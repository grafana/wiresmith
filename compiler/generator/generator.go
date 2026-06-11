package generator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/grafana/wiresmith/compiler/types"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/reporter"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

type Generator struct {
	Module   string
	OutDir   string
	ProtoDir string

	// Files optionally restricts emission to a subset of `.proto` files in
	// ProtoDir. Entries are filesystem paths (relative to cwd or absolute),
	// matching protoc's positional-argument convention. Files outside
	// ProtoDir are rejected. An empty slice keeps the default "walk
	// --proto_path and emit everything" behavior; cross-file imports are
	// always resolved against the full walk regardless of this filter.
	Files []string

	// Overrides maps an import-mapping key (the same fd.Path() string
	// buildImportMapping produces) to a Go import path, with an optional
	// `;name` suffix matching go_package syntax. Set from the CLI's
	// repeatable `-M source=dest` flag and wins over in-source go_package
	// during destination resolution — matches protoc's `M<source>=<dest>`
	// semantics. Useful when a vendored .proto declares a go_package that
	// doesn't match the generator's target tree.
	Overrides map[string]string

	// goPackages maps a proto package name to the raw value of its
	// `option go_package`. Populated during Generate after compilation.
	goPackages map[string]string

	// destinations maps fd.Path() to its resolved Go destination. Built
	// once, after collectGoPackages, by walking compiled files (including
	// transitively-imported well-known files) and feeding each through
	// destFor / destForReachable. ImportTracker reads from this map to
	// resolve cross-package references; keying by fd.Path() (rather than
	// proto package) is what disambiguates the well-known case where one
	// proto package (`google.protobuf`) spans multiple Go destinations
	// — descriptorpb, timestamppb, durationpb — for distinct files.
	destinations map[string]goDest

	// emitFilter is the set of fd.Path() values to emit, derived from Files
	// at the start of Generate. Nil means "emit every shouldGenerateFile
	// candidate" (the empty-Files default).
	emitFilter map[string]bool

	// options is the registered set of (wiresmith.options.*) custom field
	// options. Initialised by newOptionRegistry on first Generate; the same
	// slice is shared with every FileGenerator. Resolved once after Compile
	// (each option binds its linked extension descriptor) then walked again
	// to validate placements, then consulted per-field during emission to
	// dispatch FieldType / GoFieldType overrides.
	options []FieldOption

	// jsontagExt is the linked extension descriptor for
	// `(wiresmith.options.jsontag)`. jsontag doesn't influence FieldType /
	// GoFieldType, so it sits outside the registry — its resolve+validate
	// run as two inline calls in Generate.
	jsontagExt protoreflect.FieldDescriptor

	// noPresenceExt / noPresenceAllExt are the linked extension descriptors
	// for the message-level `(wiresmith.options.no_presence)` and the
	// file-level `(wiresmith.options.no_presence_all)` options. Like
	// jsontag they sit outside the FieldOption registry (they annotate
	// messages and files, not fields). Consulted by
	// FileGenerator.hasNoPresence via fieldsForPresence.
	noPresenceExt    protoreflect.FieldDescriptor
	noPresenceAllExt protoreflect.FieldDescriptor

	// enumNoPrefixExt / enumNoPrefixAllExt are the linked extension
	// descriptors for the enum-level `(wiresmith.options.enum_no_prefix)`
	// and file-level `enum_no_prefix_all` options. Same shape as the
	// no_presence pair. Consulted by FileGenerator.hasEnumNoPrefix.
	enumNoPrefixExt    protoreflect.FieldDescriptor
	enumNoPrefixAllExt protoreflect.FieldDescriptor

	// outputs accumulates the formatted Go files produced by writeFormatted
	// during a Generate run. Two callers harvest it: Generate writes them
	// to disk; GenerateFromDescriptors returns them to a protoc plugin
	// caller that hands placement to buf. Reset on every Generate call so a
	// reused Generator can't carry a prior run's outputs into a new one.
	outputs []GeneratedFile
}

// GeneratedFile is one output produced by the generator. Path is the
// file's intended location: in source mode (Generate), it includes g.OutDir
// and is ready for os.WriteFile; in plugin mode (GenerateFromDescriptors),
// it is source-relative and meant to be handed to
// protogen.Plugin.NewGeneratedFile.
type GeneratedFile struct {
	Path    string
	Content []byte
}

// FileGenerator collects emitted code for one proto source file. It owns
// TWO output buffers, not one — see the long comment on `reflectBody` for the
// performance reason. Emitters route their output to one or the other based
// on whether the code they emit is part of the marshal/unmarshal hot path or
// part of the (cold) protoreflect-registration scaffolding.
type FileGenerator struct {
	fd     protoreflect.FileDescriptor
	module string

	// body / imports hold the "main" .pb.go file: struct definitions, oneof
	// variants, Reset/Has/Get/Size/Marshal/Unmarshal/Equal methods, enum
	// constants + name/value maps + String(), and the ProtoMessage() marker.
	// Everything called on every Marshal/Unmarshal/Size lives here.
	imports *ImportTracker
	body    *bytes.Buffer

	// reflectBody / reflectImports hold the companion `_reflect.pb.go` file:
	// per-message ProtoReflect() methods, per-enum Descriptor()/Type()/Number()
	// methods, the embedded `file_*_rawDesc` byte blob, MessageInfo/EnumInfo
	// arrays, and the init() that wires everything into
	// google.golang.org/protobuf's global registries.
	//
	// Why split? google.golang.org/protobuf/types/descriptorpb,
	// google.golang.org/protobuf/reflect/protoreflect, and the descriptor
	// blobs together add ~377KB to __TEXT and ~144KB of new symbols (with
	// descriptorpb alone contributing ~64KB of code never touched by a hot
	// Marshal/Unmarshal call). Co-mingling that code with the hot paths in a
	// single .pb.go file pushed the hot functions onto different cache sets
	// / pages in the linked binary and produced a measured +7-14% slowdown on
	// otlp Marshal/Unmarshal benchmarks (UnmarshalProfiles regressed by
	// +12.6%; benchmark numbers in compiler/generator/emit_registration.go).
	//
	// By emitting the reflection glue into a SEPARATE compilation unit, we
	// give the linker freedom to place the rarely-called scaffolding away from
	// the hot Marshal/Unmarshal code. Same code, same binary size, same
	// exported API — but the hot inner loops keep their icache/iTLB locality.
	reflectImports *ImportTracker
	reflectBody    *bytes.Buffer

	// equalBody / equalImports hold the companion `_equal.pb.go` file:
	// per-message Equal() methods. Equal is never called from
	// Marshal/Unmarshal/Size, so moving it out of the main .pb.go follows the
	// same icache/iTLB-locality rationale as the reflect split — see the
	// long comment on reflectBody above and the benchmark numbers in the
	// header comment of emit_equal.go.
	equalImports *ImportTracker
	equalBody    *bytes.Buffer

	// fileVarName is a sanitized proto file path used as prefix for
	// file-level variables (descriptor, MessageInfo/EnumInfo arrays).
	fileVarName   string
	nextMsgIndex  int
	nextEnumIndex int

	// options is the shared field-option registry, copied by reference from
	// the parent Generator. Each FileGenerator emit path (fieldType,
	// goFieldType, the goFieldName / has*Option helpers) consults this
	// slice rather than reaching back up to the Generator.
	options []FieldOption

	// jsontagExt is the cached jsontag-option extension descriptor. jsontag
	// is the lone option still outside the registry (it has no FieldType /
	// GoFieldType behavior), so the descriptor still rides through as a
	// dedicated field rather than via the registry.
	jsontagExt protoreflect.FieldDescriptor

	// noPresenceExt / noPresenceAllExt are the cached message-level and
	// file-level no_presence extension descriptors — non-field options, so
	// outside the registry for the same reason as jsontagExt. Consulted by
	// hasNoPresence (option_no_presence.go).
	noPresenceExt    protoreflect.FieldDescriptor
	noPresenceAllExt protoreflect.FieldDescriptor

	// enumNoPrefixExt / enumNoPrefixAllExt — same shape, for the
	// enum_no_prefix pair (option_enum_no_prefix.go).
	enumNoPrefixExt    protoreflect.FieldDescriptor
	enumNoPrefixAllExt protoreflect.FieldDescriptor

	// compareBody / compareImports hold a second companion `_compare.pb.go`
	// file: just the per-message Compare(other interface{}) int methods.
	//
	// Why split? Compare is never called on the marshal/unmarshal/size hot
	// path, but emitting it next to the hot functions in the main `.pb.go`
	// pushes them onto different cache sets and produced a measured +9%
	// geomean regression on OTel benchmarks (UnmarshalMap +14%,
	// MarshalSingleSpan +13%) — same icache-pressure failure mode as the
	// reflectBody split documented above. Keeping Compare in its own
	// compilation unit gives the linker freedom to place the cold half
	// away from the hot half and restores baseline throughput. We pay for
	// it once, and callers don't have to opt in.
	compareImports *ImportTracker
	compareBody    *bytes.Buffer
}

// Emitter interface implementation for FileGenerator.

func (fg *FileGenerator) Writef(format string, args ...any) {
	fmt.Fprintf(fg.body, format, args...)
}

func (fg *FileGenerator) ReverseTag(indent string, num protowire.Number, wt protowire.Type) {
	fg.reverseTag(indent, num, wt)
}

func (fg *FileGenerator) AddImport(path, alias string) {
	fg.imports.addImport(path, alias)
}

// compareEmitter wraps a FileGenerator so Type.EmitCompare / FieldType.EmitCompare
// implementations route their Writef calls into fg.compareBody and their
// AddImport calls into fg.compareImports without each call site having to
// remember which buffer it should be targeting. ReverseTag panics because
// Compare never emits marshal bytes; a stray call is a generator bug worth
// catching loudly.
type compareEmitter struct{ fg *FileGenerator }

func (ce *compareEmitter) Writef(format string, args ...any) {
	fmt.Fprintf(ce.fg.compareBody, format, args...)
}

func (ce *compareEmitter) ReverseTag(indent string, num protowire.Number, wt protowire.Type) {
	panic("compareEmitter.ReverseTag: Compare emission must not call ReverseTag")
}

func (ce *compareEmitter) AddImport(path, alias string) {
	ce.fg.compareImports.addImport(path, alias)
}

// equalEmitter wraps a FileGenerator so that types.EmitEqual callbacks route
// their writes and import registrations into the companion `_equal.pb.go`
// file instead of the main .pb.go. Equal paths never emit wire tags, so
// ReverseTag is unreachable and panics if anything ever reaches it.
type equalEmitter struct{ fg *FileGenerator }

func (e equalEmitter) Writef(format string, args ...any) {
	fmt.Fprintf(e.fg.equalBody, format, args...)
}

func (e equalEmitter) AddImport(path, alias string) {
	e.fg.equalImports.addImport(path, alias)
}

func (e equalEmitter) ReverseTag(indent string, num protowire.Number, wt protowire.Type) {
	panic("equalEmitter.ReverseTag: Equal must not emit wire tags")
}

// fieldContext builds a FieldContext from a field descriptor.
//
// Stdtime / stdduration fields are special-cased: they have MessageKind on
// the wire but the Go field is a stdlib value type (`time.Time` /
// `time.Duration`), so we skip the MessageType lookup (which would
// register a timestamppb/durationpb import the main `.pb.go` never uses).
func (fg *FileGenerator) fieldContext(fd protoreflect.FieldDescriptor) types.FieldContext {
	ctx := types.FieldContext{}
	if fd.Kind() == protoreflect.EnumKind {
		ctx.EnumType = fg.imports.goEnumType(fd.Enum())
	}
	if fd.Kind() == protoreflect.MessageKind && !fg.suppressMessageType(fd) {
		ctx.MessageType = fg.imports.goSingularType(fd)
		ctx.IsSamePackage = fg.imports.isSelfDest(fd.Message().ParentFile().Path())
	}
	return ctx
}

// Generate compiles .proto sources from g.ProtoDir and writes the
// resulting Go files under g.OutDir. CLI entry point.
func (g *Generator) Generate(ctx context.Context) error {
	files, emit, err := g.compileSources(ctx)
	if err != nil {
		return err
	}
	outputs, err := g.generateFromFiles(files, emit)
	if err != nil {
		return err
	}
	return writeOutputsToDisk(outputs)
}

// GenerateFromDescriptors emits Go files from a pre-linked descriptor set
// and returns them in memory rather than writing to disk. Used by
// `cmd/protoc-gen-wiresmith` (and by tests that want to assert on the
// generated bytes without touching the filesystem).
//
// `files` is the full set of files visible to the generator — the union of
// the files to emit and their transitive dependencies (so cross-file
// imports resolve). `emit` (if non-nil) restricts emission to files whose
// fd.Path() is a key in the map; nil means "emit every shouldGenerateFile
// candidate in files".
//
// The caller must arrange for the embedded `wiresmith/options.proto` to
// appear in `files` when any user file imports it — Generate handles that
// from the embed; in plugin mode the wiresmith schema must be on the
// invoking proto compiler's import path so the protoc/buf request brings
// the descriptors in transitively.
func (g *Generator) GenerateFromDescriptors(ctx context.Context, files []protoreflect.FileDescriptor, emit map[string]bool) ([]GeneratedFile, error) {
	_ = ctx
	if err := g.validateOutDir(); err != nil {
		return nil, err
	}
	return g.generateFromFiles(files, emit)
}

// compileSources runs the source-based front half of Generate: read
// .proto files from g.ProtoDir, build the import-map emit filter from
// g.Files, inject the embedded wiresmith schema, and invoke protocompile.
// Split out so GenerateFromDescriptors can skip it.
func (g *Generator) compileSources(ctx context.Context) ([]protoreflect.FileDescriptor, map[string]bool, error) {
	// --out flows into the Go import-path base (module + outDir), so it has to
	// be a clean, module-relative, forward-slash path. An absolute, '..'-
	// containing, or backslash-separated value would emit import paths that
	// fail Go's directory-equals-import-path rule at build time.
	if err := g.validateOutDir(); err != nil {
		return nil, nil, err
	}

	// Probe the proto_path root explicitly before WalkDir runs so a missing
	// or wrong-shape flag value gets a clean, user-facing diagnostic.
	// WalkDir surfaces the underlying lstat error verbatim, which would
	// otherwise leak "lstat ... no such file or directory" to end users.
	// Probing here also keeps the narrower "missing inside the tree" case
	// (broken symlink, file deleted mid-walk) on its own error path with
	// the lstat context preserved — only the proto_path root itself
	// triggers the clean message.
	info, err := os.Stat(g.ProtoDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, fmt.Errorf("--proto_path %q: directory does not exist", g.ProtoDir)
		}
		return nil, nil, fmt.Errorf("--proto_path %q: %w", g.ProtoDir, err)
	}
	if !info.IsDir() {
		// Without this check WalkDir on a regular file walks nothing and
		// the generator silently succeeds with an empty output set — a
		// confusing "no proto files found" outcome for what is really a
		// flag-value typo.
		return nil, nil, fmt.Errorf("--proto_path %q: not a directory", g.ProtoDir)
	}

	mapping, importPaths, pathToKey, err := buildImportMapping(g.ProtoDir)
	if err != nil {
		return nil, nil, fmt.Errorf("building import mapping: %w", err)
	}

	var emit map[string]bool
	if len(g.Files) > 0 {
		emit = make(map[string]bool, len(g.Files))
		for _, src := range g.Files {
			abs, err := filepath.Abs(src)
			if err != nil {
				return nil, nil, fmt.Errorf("resolving %q: %w", src, err)
			}
			if key, ok := pathToKey[abs]; ok {
				emit[key] = true
				continue
			}
			// Distinguish "file doesn't exist" (typo, far more common) from
			// "file exists but is outside the walked tree". The first message
			// blamed --proto_path even when the user just mistyped a filename,
			// which made typos confusing to diagnose.
			if _, statErr := os.Stat(src); os.IsNotExist(statErr) {
				return nil, nil, fmt.Errorf("file %q does not exist", src)
			}
			return nil, nil, fmt.Errorf("file %q is not a .proto under --proto_path=%q", src, g.ProtoDir)
		}
	}

	// Always inject the embedded `wiresmith/options.proto` into the input set
	// so its extension descriptor ends up in the linked results — that's how
	// hasPointerOption finds the extension type later. Users `import
	// "wiresmith/options.proto"` from their own .proto files; the memResolver
	// serves it from the embed. A user file at the canonical path would
	// silently shadow the embedded schema — reject that explicitly rather
	// than guessing intent.
	if _, ok := mapping[embeddedOptionsPath]; ok {
		return nil, nil, fmt.Errorf("user proto at %q conflicts with the embedded wiresmith schema — remove the on-disk file; wiresmith serves it from its own embed", embeddedOptionsPath)
	}
	mapping[embeddedOptionsPath] = embeddedOptionsProto
	importPaths = append(importPaths, embeddedOptionsPath)
	// buildImportMapping returns importPaths sorted for determinism; restore
	// that invariant after the in-place append.
	sort.Strings(importPaths)

	// WithStandardImports satisfies imports for the well-known protos
	// (`google/protobuf/descriptor.proto` and friends) that the embedded
	// options file depends on.
	resolver := protocompile.WithStandardImports(&memResolver{files: mapping})
	compiler := protocompile.Compiler{
		Resolver:       resolver,
		SourceInfoMode: protocompile.SourceInfoStandard,
		Reporter: reporter.NewReporter(
			func(err reporter.ErrorWithPos) error { return err },
			nil,
		),
	}

	// With positional files, compile only those (plus the embedded options
	// schema); the resolver pulls transitive imports from the full mapping
	// on demand. Unrelated siblings in the walked tree are never parsed —
	// a staged migration can generate one file from a tree whose other
	// protos don't compile (gogo annotations without the gogo schema on
	// the path). Without positional files, the whole walk compiles, as
	// before. Validation (go_package consistency, destination collisions)
	// therefore covers the compiled subgraph only in scoped runs — the
	// same trade protoc makes.
	roots := importPaths
	if len(emit) > 0 {
		roots = make([]string, 0, len(emit)+1)
		for key := range emit {
			roots = append(roots, key)
		}
		sort.Strings(roots)
		roots = append(roots, embeddedOptionsPath)
	}
	linked, err := compiler.Compile(ctx, roots...)
	if err != nil {
		return nil, nil, fmt.Errorf("compiling protos: %w", err)
	}
	files := make([]protoreflect.FileDescriptor, len(linked))
	for i, r := range linked {
		files[i] = r
	}
	return files, emit, nil
}

// generateFromFiles is the shared post-compile pipeline used by both
// source mode (Generate) and descriptor mode (GenerateFromDescriptors):
// bind and validate options, resolve destinations, detect output-path
// collisions, and emit one .pb.go set per file. Writes are captured into
// g.outputs and returned; the caller decides whether to flush to disk or
// hand off to a plugin response.
func (g *Generator) generateFromFiles(results []protoreflect.FileDescriptor, emit map[string]bool) ([]GeneratedFile, error) {
	// Reset on every call so a reused Generator never carries an emitFilter
	// or outputs slice from a prior run into a new one.
	g.emitFilter = emit
	g.outputs = nil

	// Bind every registered FieldOption to its linked extension descriptor,
	// then run their placement validators. Two passes (rather than one
	// combined pass) so a Validate implementation can look up a peer
	// option's descriptor — stdtime's Validate consults the pointer option
	// to surface a clearer "stdtime cannot combine with pointer" error.
	g.options = newOptionRegistry()
	for _, opt := range g.options {
		// ext may be nil when wiresmith/options.proto is not part of the
		// compiled set (e.g. plugin mode without a user-side import). The
		// option helpers already short-circuit on nil descriptors and return
		// false from Has, so an unbound option becomes a silent no-op for
		// files that don't use it. Source mode always injects the embedded
		// schema, so a nil ext there would only surface if the embed itself
		// is broken — caught by every options-using test rather than by a
		// here-and-now panic.
		opt.Resolve(findExtension(results, opt.Name()))
	}
	for _, opt := range g.options {
		if err := opt.Validate(g, results); err != nil {
			return nil, err
		}
	}
	if err := g.resolveJsontagExtension(results); err != nil {
		return nil, err
	}
	g.noPresenceExt = findExtension(results, noPresenceExtName)
	g.noPresenceAllExt = findExtension(results, noPresenceAllExtName)
	g.enumNoPrefixExt = findExtension(results, enumNoPrefixExtName)
	g.enumNoPrefixAllExt = findExtension(results, enumNoPrefixAllExtName)
	if err := g.validateJsontagOptions(results); err != nil {
		return nil, err
	}
	if err := g.validateNoValueCycles(results); err != nil {
		return nil, err
	}

	if err := g.collectGoPackages(results); err != nil {
		return nil, err
	}
	if err := g.computeDests(results); err != nil {
		return nil, err
	}
	if err := g.validateDestinations(results); err != nil {
		return nil, err
	}

	// Detect output-path collisions up front, before writing any files. Two
	// protos in different directories with the same package and same basename
	// would otherwise silently clobber each other on disk — recursive scanning
	// makes this collision possible where flat layouts could not produce it.
	//
	// Only consider files we'll actually emit (shouldGenerateFile filters out
	// internal schemas and empty protos that emit no output, so they can't
	// clobber a neighbour).
	//
	// Each proto emits up to five outputs (the main .pb.go and the
	// companion _reflect.pb.go, _compare.pb.go, _equal.pb.go, and — when
	// the proto declares services — _grpc.pb.go), so an input like
	// foo_reflect.proto / foo_compare.proto / foo_equal.proto / foo_grpc.proto
	// would generate a file that collides with foo.proto's companion.
	// Check all five paths against the same map.
	outputs := make(map[string]string, 5*len(results))
	for _, fd := range results {
		if !g.shouldEmit(fd) {
			continue
		}
		for _, outPath := range []string{g.outputPathFor(fd), g.outputReflectPathFor(fd), g.outputComparePathFor(fd), g.outputEqualPathFor(fd), g.outputGrpcPathFor(fd)} {
			if prev, exists := outputs[outPath]; exists {
				return nil, fmt.Errorf("output collision at %s: %q and %q both write to this path (proto package %q)",
					outPath, prev, fd.Path(), fd.Package())
			}
			outputs[outPath] = fd.Path()
		}
	}

	for _, fd := range results {
		if !g.shouldEmit(fd) {
			continue
		}
		if err := g.generateFile(fd); err != nil {
			return nil, fmt.Errorf("generating %s: %w", fd.Path(), err)
		}
	}
	return g.outputs, nil
}

// writeOutputsToDisk persists a generator run's captured outputs to disk.
// Used by the source-mode CLI path; plugin mode hands the slice to
// protogen instead.
func writeOutputsToDisk(outputs []GeneratedFile) error {
	for _, o := range outputs {
		if err := os.MkdirAll(filepath.Dir(o.Path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(o.Path, o.Content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// shouldEmit reports whether fd should produce output. It combines the
// content-based shouldGenerateFile check (skips internal schemas and
// empty protos) with the optional Files-positional-args filter — the
// latter narrows emission to a caller-listed subset while leaving import
// resolution unrestricted.
func (g *Generator) shouldEmit(fd protoreflect.FileDescriptor) bool {
	if !shouldGenerateFile(fd) {
		return false
	}
	if g.emitFilter != nil && !g.emitFilter[fd.Path()] {
		return false
	}
	return true
}

// shouldGenerateFile reports whether fd contributes any Go output. Internal
// schema files (wiresmith.options) are always skipped. Files with no
// messages, enums, or services emit nothing — the main .pb.go would be
// header-only, the reflect/compare/equal companions have nothing to write,
// and the gRPC stub generator has nothing to do.
//
// Service-only files (no messages, no enums, but `service` declarations)
// produce both the FileDescriptor-registration companion (`_reflect.pb.go`)
// and the gRPC stub companion (`_grpc.pb.go`); without them the gRPC bridge
// would never reach a file whose only Go output is its stub set.
func shouldGenerateFile(fd protoreflect.FileDescriptor) bool {
	if isInternalSchemaFile(fd) {
		return false
	}
	return fd.Messages().Len() > 0 || fd.Enums().Len() > 0 || fd.Services().Len() > 0
}

// isInternalSchemaFile reports whether a compiled file is wiresmith-internal
// metadata — currently just the embedded `wiresmith/options.proto`. Identified
// by canonical import path so a user file that happens to declare the same
// proto package is never mistaken for the embedded schema (and is therefore
// not skipped by validation, codegen, or output-collision checks).
func isInternalSchemaFile(fd protoreflect.FileDescriptor) bool {
	return fd.Path() == embeddedOptionsPath
}

// outputPathFor returns the output path for the .pb.go file produced for fd.
// The directory is purely source-relative — the dir of fd.Path() under
// --out — matching the `paths=source_relative` contract.
func (g *Generator) outputPathFor(fd protoreflect.FileDescriptor) string {
	base := strings.TrimSuffix(filepath.Base(fd.Path()), ".proto") + ".pb.go"
	return filepath.Join(g.OutDir, sourceRelDir(fd.Path()), base)
}

// collectGoPackages records every file's `option go_package` value, keyed
// by proto package. Every file belonging to one proto package must declare
// the same value — including "unset". An asymmetric mix (file A sets it,
// file B in the same package omits it) is rejected too: silently treating
// the unset file as if it inherited A's value would contradict the
// upfront-agreement contract and could move generated files unexpectedly.
//
// No validation is performed on the go_package string itself: disk
// destinations are source-relative (independent of go_package), so a
// malformed value can only produce a malformed import-path string in the
// generated file — which fails loudly at `go build` with a clear error.
// This matches protoc-gen-go.
//
// Only `results` (files explicitly in the compile set) feed this map.
// Well-known proto files share the proto package `google.protobuf` across
// several distinct Go destinations (descriptor.proto → descriptorpb,
// timestamp.proto → timestamppb, etc.); enforcing the one-go_package-per-
// proto-package rule on those would reject every user proto that imports
// more than one well-known. Transitive imports get their dest resolved
// directly from their own FileDescriptor in computeDests instead.
func (g *Generator) collectGoPackages(results []protoreflect.FileDescriptor) error {
	g.goPackages = make(map[string]string)

	// sighting captures the first go_package value (possibly empty) we saw
	// for a proto pkg and the file we saw it in, so a later disagreement
	// can be reported with both endpoints.
	type sighting struct{ value, path string }
	seen := make(map[string]sighting)

	for _, fd := range results {
		// The embedded options schema is generator-internal — it has no
		// go_package and shouldn't seed the seen-map or constrain a
		// user file that legitimately declares package wiresmith.options.
		if isInternalSchemaFile(fd) {
			continue
		}
		// A file pinned via -M resolves its destination from the override
		// (destForPath checks Overrides first), so it neither seeds the
		// agreement map nor conflicts with it. This is the escape hatch for
		// proto packages that legitimately span multiple Go packages (e.g.
		// Loki's `package logproto` split between a standalone push module
		// and pkg/logproto): pin the odd files out with -M and the rest of
		// the package still has to agree among itself. validateDestinations
		// separately rejects an override that would split a single output
		// directory across two Go packages.
		if override, ok := g.Overrides[fd.Path()]; ok && override != "" {
			continue
		}
		// GetGoPackage is nil-safe — it returns "" if the cast fails or
		// the option is unset.
		opts, _ := fd.Options().(*descriptorpb.FileOptions)
		goPkg := opts.GetGoPackage()
		pkg := string(fd.Package())

		if prev, ok := seen[pkg]; ok {
			if prev.value != goPkg {
				return fmt.Errorf("inconsistent go_package for package %q: %s declares %q but %s declares %q",
					pkg, prev.path, prev.value, fd.Path(), goPkg)
			}
			continue
		}
		seen[pkg] = sighting{value: goPkg, path: fd.Path()}

		if goPkg != "" {
			g.goPackages[pkg] = goPkg
		}
	}
	return nil
}

// walkReachableFiles returns every FileDescriptor transitively reachable
// from `roots` via fd.Imports(). The returned slice preserves first-seen
// order and never includes a descriptor twice. Used by computeDests to pick
// up well-known imports (e.g. google/protobuf/timestamp.proto) that
// protocompile resolves via WithStandardImports but doesn't include in the
// top-level results slice.
func walkReachableFiles(roots []protoreflect.FileDescriptor) []protoreflect.FileDescriptor {
	seen := make(map[string]struct{})
	var out []protoreflect.FileDescriptor
	var visit func(fd protoreflect.FileDescriptor)
	visit = func(fd protoreflect.FileDescriptor) {
		if _, ok := seen[fd.Path()]; ok {
			return
		}
		seen[fd.Path()] = struct{}{}
		out = append(out, fd)
		imps := fd.Imports()
		for i := 0; i < imps.Len(); i++ {
			visit(imps.Get(i).FileDescriptor)
		}
	}
	for _, fd := range roots {
		visit(fd)
	}
	return out
}

// validateOutDir rejects --out values that would produce broken import paths
// when composed into module + outDir. The acceptable shape is a clean,
// module-relative, forward-slash path with no '..' segments. A leading "./"
// is tolerated and stripped — convenient for shells that expand bare ".gen"
// to "./.gen" — so the user doesn't have to learn the difference. A bare "."
// (or "./.") is treated as the module root, matching the empty-string case:
// without normalization, joinImport(module, ".") would compose to "module/."
// and downstream imports would land as "module/./<pkg>".
func (g *Generator) validateOutDir() error {
	g.OutDir = strings.TrimPrefix(g.OutDir, "./")
	if g.OutDir == "." {
		g.OutDir = ""
	}
	if g.OutDir == "" {
		return nil // empty is fine: import base is just the module
	}
	if strings.ContainsRune(g.OutDir, '\\') {
		return fmt.Errorf("--out %q contains backslashes; use forward slashes (Go import paths are always slash-separated)", g.OutDir)
	}
	if filepath.IsAbs(g.OutDir) || strings.HasPrefix(g.OutDir, "/") {
		return fmt.Errorf("--out %q must be a module-relative path, not absolute", g.OutDir)
	}
	// Check '..' on the raw segments first so the error names the actual
	// danger rather than dropping out via the "not clean" branch — both
	// `pkg/api/..` (cleans to `pkg`) and `./pkg/../api` (cleans to `api`)
	// canonicalize away the traversal silently.
	for _, seg := range strings.Split(g.OutDir, "/") {
		if seg == ".." {
			return fmt.Errorf("--out %q must not contain '..' segments", g.OutDir)
		}
	}
	if cleaned := path.Clean(g.OutDir); cleaned != g.OutDir {
		return fmt.Errorf("--out %q is not a clean path; the equivalent canonical form is %q", g.OutDir, cleaned)
	}
	return nil
}

// computeDests resolves each compiled file's Go destination once and stores
// the result keyed by fd.Path(). Cross-file import resolution in
// ImportTracker.goMessageType / goEnumType always has a parent
// FileDescriptor in hand and uses the path key directly, which is what
// disambiguates the well-known case where one proto package
// (`google.protobuf`) spans multiple Go destinations.
//
// All files in `results` that declare the same proto package must live in
// the same source-relative directory; Go's directory-equals-package rule
// would later reject the disagreement, but flagging it here gives the user
// a clearer error referencing both .proto files.
//
// The walk follows transitive `import` statements (via walkReachableFiles)
// so well-known proto files served by `protocompile.WithStandardImports`
// also land in the destinations map — emit_protoreflect's goTypes array
// needs their Go imports resolved even though they themselves don't emit.
// Transitively-imported files use destForReachable (each file's own
// go_package option) instead of the shared g.goPackages table, because
// google.protobuf's well-known files share one proto package across many
// distinct Go destinations.
func (g *Generator) computeDests(results []protoreflect.FileDescriptor) error {
	g.destinations = make(map[string]goDest)
	type sighting struct{ relDir, path string }
	seen := make(map[string]sighting)
	// importOwner tracks which source directory first claimed each Go
	// import path. Two proto packages MAY share one import path when they
	// live in the same directory (protoc parity — Loki's indexgateway.proto
	// cohabits pkg/logproto with `package logproto` files); the
	// uncompilable case is one import path written from two different
	// directories. Package-name agreement inside a directory is enforced
	// by validateDestinations, which sees the resolved pkgName.
	type pathClaim struct{ protoPkg, relDir, path string }
	importOwner := make(map[string]pathClaim)

	// inResults marks each file we'll consider an emit candidate.
	inResults := make(map[string]bool, len(results))
	for _, fd := range results {
		inResults[fd.Path()] = true
	}

	for _, fd := range walkReachableFiles(results) {
		if isInternalSchemaFile(fd) {
			continue
		}
		if !inResults[fd.Path()] {
			g.destinations[fd.Path()] = destForReachable(fd)
			continue
		}
		// Files in results but excluded from emission (positional Files
		// filter scoped the emit set, but the compile still linked the
		// import dependencies) still need a destination registered so an
		// emitted file's reference into them resolves to a real import
		// path rather than the empty alias. They skip the conflict
		// checks — those only matter for files that actually write
		// output to disk.
		if !g.shouldEmit(fd) {
			g.destinations[fd.Path()] = g.destFor(fd)
			continue
		}
		protoPkg := string(fd.Package())
		relDir := sourceRelDir(fd.Path())
		dest := g.destFor(fd)
		// An -M-pinned file opts out of the package-wide agreement (same
		// rationale as in collectGoPackages): its destination is explicit,
		// so it must not seed the seen-map (which would force dir agreement
		// on its unpinned package-mates) nor be checked against it. It still
		// claims its import path below so a different proto package can't
		// silently share it.
		if override, ok := g.Overrides[fd.Path()]; ok && override != "" {
			if owner, exists := importOwner[dest.importPath]; exists && owner.relDir != relDir {
				return fmt.Errorf("import path %q is claimed from two directories: %s (package %q) and %s (package %q) — one Go import path cannot span directories (check go_package options and -M overrides)",
					dest.importPath, owner.path, owner.protoPkg, fd.Path(), protoPkg)
			}
			importOwner[dest.importPath] = pathClaim{protoPkg: protoPkg, relDir: relDir, path: fd.Path()}
			g.destinations[fd.Path()] = dest
			continue
		}
		if prev, ok := seen[protoPkg]; ok {
			if prev.relDir != relDir {
				return fmt.Errorf("proto package %q spans multiple source-relative directories: %s declares %q but %s declares %q",
					protoPkg, prev.path, prev.relDir, fd.Path(), relDir)
			}
			// Same proto pkg, same dir — share the destination across all
			// of the package's files so cross-file references inside the
			// same Go package don't need a qualifier.
			g.destinations[fd.Path()] = dest
			continue
		}
		seen[protoPkg] = sighting{relDir: relDir, path: fd.Path()}
		if owner, exists := importOwner[dest.importPath]; exists && owner.relDir != relDir {
			return fmt.Errorf("import path %q is claimed from two directories: %s (package %q) and %s (package %q) — one Go import path cannot span directories (check go_package options)",
				dest.importPath, owner.path, owner.protoPkg, fd.Path(), protoPkg)
		}
		importOwner[dest.importPath] = pathClaim{protoPkg: protoPkg, relDir: relDir, path: fd.Path()}
		g.destinations[fd.Path()] = dest
	}
	return nil
}

// destForReachable returns the Go destination for a transitively-imported
// file (one not in the emit set) using only the file's own go_package
// option. Mirrors destFor but bypasses the shared g.goPackages table to
// avoid mis-resolving well-known files that share a proto package.
//
// Returns the parsed go_package directly. A file without go_package falls
// back to the proto-package-derived defaults — never the right answer for
// well-known types, but no worse than the previous behavior (the unaliased
// import would have resolved to an empty path either way).
func destForReachable(fd protoreflect.FileDescriptor) goDest {
	relDir := sourceRelDir(fd.Path())
	protoPkg := string(fd.Package())
	importPath := relDir
	pkgName := goPackageName(protoPkg)
	opts, _ := fd.Options().(*descriptorpb.FileOptions)
	if goPkg := opts.GetGoPackage(); goPkg != "" {
		importPath, pkgName = parseGoPackage(goPkg)
	}
	return goDest{importPath: importPath, relDir: relDir, pkgName: pkgName, protoPkg: protoPkg}
}

// validateDestinations rejects the case where two distinct proto packages
// resolve to the same source-relative Go directory. With per-source-file
// emission and source-relative output paths, two .proto files in different
// proto packages but the same directory would land in the same Go package
// — which is exactly the disagreement Go's directory-equals-package rule
// would later flag at build time, just much later and less clearly.
//
// Files that won't emit are skipped: an empty .proto cannot collide on disk
// with a non-empty proto resolving to the same Go dir, because it writes
// nothing there. Including it would produce a false-positive collision error.
//
// The unit of agreement is the file's RESOLVED Go identity — import path
// and package name — not its proto package: two proto packages may
// legitimately cohabit one directory when their go_package options agree
// (protoc parity; Loki's indexgateway.proto shares pkg/logproto with
// `package logproto` files). What one directory cannot hold is two import
// paths (an -M override pinning one file away from its dir-mates) or two
// package clauses (e.g. neither file declares go_package, so the names
// derive from the differing proto packages) — Go's directory-equals-package
// rule rejects both, just later and less clearly.
func (g *Generator) validateDestinations(results []protoreflect.FileDescriptor) error {
	type claim struct{ importPath, pkgName, path string }
	dirOwner := make(map[string]claim)
	for _, fd := range results {
		if !g.shouldEmit(fd) {
			continue
		}
		relDir := sourceRelDir(fd.Path())
		dest := g.destinations[fd.Path()]
		if owner, ok := dirOwner[relDir]; ok {
			if owner.importPath != dest.importPath {
				return fmt.Errorf("destination %q has conflicting Go import paths: %s resolves to %q but %s resolves to %q (check go_package options and -M overrides — one directory must map to one Go package)",
					relDir, owner.path, owner.importPath, fd.Path(), dest.importPath)
			}
			if owner.pkgName != dest.pkgName {
				return fmt.Errorf("destination %q has conflicting Go package names: %s resolves to %q but %s resolves to %q (files sharing a directory must agree on the Go package name — set matching go_package options)",
					relDir, owner.path, owner.pkgName, fd.Path(), dest.pkgName)
			}
			continue
		}
		dirOwner[relDir] = claim{importPath: dest.importPath, pkgName: dest.pkgName, path: fd.Path()}
	}
	return nil
}

func (g *Generator) generateFile(fd protoreflect.FileDescriptor) error {
	selfDest := g.destinations[fd.Path()]
	fg := &FileGenerator{
		fd:                 fd,
		module:             g.Module,
		imports:            newImportTracker(g.Module, selfDest, g.destinations),
		body:               &bytes.Buffer{},
		reflectImports:     newImportTracker(g.Module, selfDest, g.destinations),
		reflectBody:        &bytes.Buffer{},
		compareImports:     newImportTracker(g.Module, selfDest, g.destinations),
		compareBody:        &bytes.Buffer{},
		equalImports:       newImportTracker(g.Module, selfDest, g.destinations),
		equalBody:          &bytes.Buffer{},
		fileVarName:        sanitizeFileVarName(fd.Path()),
		options:            g.options,
		jsontagExt:         g.jsontagExt,
		noPresenceExt:      g.noPresenceExt,
		noPresenceAllExt:   g.noPresenceAllExt,
		enumNoPrefixExt:    g.enumNoPrefixExt,
		enumNoPrefixAllExt: g.enumNoPrefixAllExt,
	}

	// Hot-path emitters target the main .pb.go: structs, oneof variants,
	// Reset/Has/Get/Size/Marshal/Unmarshal all write to fg.body / fg.imports.
	// emitAllEqualMethods and emitAllCompareMethods are intentionally driven
	// from this same pass, but route their output into fg.equalBody /
	// fg.compareBody (via equalEmitter / compareEmitter) so Equal and Compare
	// land in their own companion files for the same icache/iTLB rationale as
	// the reflect split — see the FileGenerator field comments for details.
	fg.emitAllEnums(fd)
	fg.emitAllOneofs(fd)
	fg.emitAllStructs(fd)
	fg.emitAllResetMethods(fd)
	fg.emitAllHasMethods(fd)
	fg.emitAllGetterMethods(fd)
	fg.emitAllSizeMethods(fd)
	fg.emitAllMarshalMethods(fd)
	fg.emitAllUnmarshalMethods(fd)
	fg.emitAllEqualMethods(fd)
	fg.emitAllCompareMethods(fd)

	// Companion file: reflection/registration glue. These emitters write to
	// fg.reflectBody / fg.reflectImports. The two passes below MUST iterate
	// in TypeBuilder's flattened ordering (via flattenedEnums and
	// flattenedMessages), because they assign the same indices that
	// emitRegistration uses for the per-file _enumTypes / _msgTypes arrays.
	// Changing one without the other will silently produce a binary where
	// message N's ProtoReflect() returns the descriptor for message N+k.
	fg.emitAllEnumReflectMethods(fd)
	fg.emitAllProtoReflectMethods(fd)
	fg.emitRegistration(fd)

	// Record the main .pb.go file. Skip when fg.body is empty — that's the
	// service-only case (no enums, oneofs, structs, accessors, or marshal
	// methods to emit), and emitting a header-only file would just produce
	// unused imports / no useful Go output. Mirrors the per-companion
	// conditional writes for _reflect / _compare / _equal below.
	if fg.body.Len() > 0 {
		var mainOut bytes.Buffer
		fg.emitHeader(&mainOut, fg.imports)
		mainOut.Write(fg.body.Bytes())
		g.writeFormatted(g.outputPathFor(fd), mainOut.Bytes(), fd.Path())
	}

	// Record the companion _reflect.pb.go file. Skip if the proto declared no
	// messages and no enums — there's nothing to register and emitting a file
	// with just a package clause would be misleading.
	if fg.reflectBody.Len() > 0 {
		var reflOut bytes.Buffer
		fg.emitHeader(&reflOut, fg.reflectImports)
		fg.emitReflectFileBanner(&reflOut)
		reflOut.Write(fg.reflectBody.Bytes())
		g.writeFormatted(g.outputReflectPathFor(fd), reflOut.Bytes(), fd.Path())
	}

	// Record the companion _compare.pb.go file. Same split rationale as
	// _reflect.pb.go: Compare is a cold method but emitting it next to the
	// hot Marshal/Unmarshal in the main file shifts hot code onto different
	// cache sets (measured: +9% geomean on OTel hot paths). Skip when the
	// proto declared no messages — Compare is per-message so an enum-only
	// or empty file has nothing to emit.
	if fg.compareBody.Len() > 0 {
		var cmpOut bytes.Buffer
		fg.emitHeader(&cmpOut, fg.compareImports)
		fg.emitCompareFileBanner(&cmpOut)
		cmpOut.Write(fg.compareBody.Bytes())
		g.writeFormatted(g.outputComparePathFor(fd), cmpOut.Bytes(), fd.Path())
	}

	// Record the companion _equal.pb.go file. Skip if no messages contributed
	// an Equal method (file with only enums, or filtered to none).
	if fg.equalBody.Len() > 0 {
		var eqOut bytes.Buffer
		fg.emitHeader(&eqOut, fg.equalImports)
		fg.emitEqualFileBanner(&eqOut)
		eqOut.Write(fg.equalBody.Bytes())
		g.writeFormatted(g.outputEqualPathFor(fd), eqOut.Bytes(), fd.Path())
	}

	// Record the companion _grpc.pb.go file. Skip when fd declares no
	// services — emitGRPC short-circuits the same case but checking
	// inline keeps the order-of-files generated visible in this loop.
	return g.emitGRPC(fd)
}

// writeFormatted gofmt-formats src and records it as one of this run's
// outputs. The disk write — for source-mode callers — happens later in
// writeOutputsToDisk; plugin-mode callers harvest g.outputs directly. If
// gofmt fails (typically a generator bug producing invalid Go), the
// unformatted bytes are recorded instead so the broken output is visible
// for debugging.
func (g *Generator) writeFormatted(outPath string, src []byte, sourceProto string) {
	formatted, err := format.Source(src)
	if err != nil {
		formatted = src
		fmt.Fprintf(os.Stderr, "warning: format error for %s -> %s: %v\n", sourceProto, outPath, err)
	}
	g.outputs = append(g.outputs, GeneratedFile{Path: outPath, Content: formatted})
}

// outputReflectPathFor returns the path for the companion `_reflect.pb.go`
// file. It sits next to the main `.pb.go` so callers find both files in the
// same package directory; the `_reflect` suffix is conventional and matches
// the protoc-gen-go-impl convention where extra generated material lives in
// `<name>_<flavor>.pb.go`.
func (g *Generator) outputReflectPathFor(fd protoreflect.FileDescriptor) string {
	base := strings.TrimSuffix(filepath.Base(fd.Path()), ".proto") + "_reflect.pb.go"
	return filepath.Join(g.OutDir, sourceRelDir(fd.Path()), base)
}

// outputComparePathFor returns the path for the third companion file,
// `<name>_compare.pb.go`, which carries the per-message Compare methods.
// Same source-relative directory as the main and reflect files.
func (g *Generator) outputComparePathFor(fd protoreflect.FileDescriptor) string {
	base := strings.TrimSuffix(filepath.Base(fd.Path()), ".proto") + "_compare.pb.go"
	return filepath.Join(g.OutDir, sourceRelDir(fd.Path()), base)
}

// outputEqualPathFor returns the path for the companion `_equal.pb.go` file
// that holds per-message Equal() methods, split out for the same icache /
// iTLB locality reasons as `_reflect.pb.go`.
func (g *Generator) outputEqualPathFor(fd protoreflect.FileDescriptor) string {
	base := strings.TrimSuffix(filepath.Base(fd.Path()), ".proto") + "_equal.pb.go"
	return filepath.Join(g.OutDir, sourceRelDir(fd.Path()), base)
}

// outputGrpcPathFor returns the path for the companion `_grpc.pb.go` file
// that holds the gRPC client/server stubs emitted by the vendored
// protoc-gen-go-grpc generator (compiler/generator/grpc). The path is
// reserved unconditionally so collision detection can flag a hypothetical
// `<name>_grpc.proto` even when fd itself has no services; the actual file
// is only written when fd.Services().Len() > 0 (see emit_grpc.go).
func (g *Generator) outputGrpcPathFor(fd protoreflect.FileDescriptor) string {
	base := strings.TrimSuffix(filepath.Base(fd.Path()), ".proto") + "_grpc.pb.go"
	return filepath.Join(g.OutDir, sourceRelDir(fd.Path()), base)
}

// emitHeader writes the "Code generated by ..." banner, the package clause,
// and the import block for one output file. The tracker argument selects
// which set of imports gets emitted (main file vs. companion reflect file).
// Each output file in the package needs its own import block — the Go
// compiler can't deduce that a `protoreflect` symbol used in foo_reflect.pb.go
// satisfies the import in foo.pb.go.
func (fg *FileGenerator) emitHeader(out *bytes.Buffer, tracker *ImportTracker) {
	fmt.Fprintf(out, "// Code generated by wiresmith. DO NOT EDIT.\n")
	fmt.Fprintf(out, "// source: %s\n\n", fg.fd.Path())
	fmt.Fprintf(out, "package %s\n\n", tracker.selfDest.pkgName)

	type imp struct {
		path    string
		alias   string
		natural string
	}
	var imps []imp
	for p, e := range tracker.imports {
		// Pre-reserved entries that no emitter actually requested would
		// produce "imported and not used" if emitted — skip them. They
		// existed only so aliasInUse could see their natural names.
		if !e.requested {
			continue
		}
		imps = append(imps, imp{p, e.alias, e.naturalName})
	}
	sort.Slice(imps, func(i, j int) bool { return imps[i].path < imps[j].path })

	if len(imps) == 0 {
		return
	}
	fmt.Fprintf(out, "import (\n")
	for _, i := range imps {
		// Elide the alias only when Go would arrive at the same identifier
		// anyway — that is, when the alias matches the imported file's
		// declared `package` clause. Using path.Base instead would be wrong
		// for go_package's `;name` form, where the declared name and the
		// path's last segment can differ.
		if i.alias == "" || i.alias == i.natural {
			fmt.Fprintf(out, "\t%q\n", i.path)
		} else {
			fmt.Fprintf(out, "\t%s %q\n", i.alias, i.path)
		}
	}
	fmt.Fprintf(out, ")\n\n")
}

// emitReflectFileBanner writes a comment block below the standard generated
// header in every `_reflect.pb.go` file explaining (a) what's in this file,
// (b) why it isn't just appended to the main `.pb.go`, and (c) what to grep for
// if you want to undo the split. Documentation lives in the generated artifact
// (not just in the generator) because future maintainers will encounter the
// file before they encounter the generator.
func (fg *FileGenerator) emitReflectFileBanner(out *bytes.Buffer) {
	fmt.Fprintf(out, "// Reflection / registration glue for %s.\n", fg.fd.Path())
	fmt.Fprintf(out, "//\n")
	fmt.Fprintf(out, "// This file holds the per-message ProtoReflect() methods, the per-enum\n")
	fmt.Fprintf(out, "// Descriptor()/Type()/Number() methods, the embedded FileDescriptorProto\n")
	fmt.Fprintf(out, "// blob, the file_*_msgTypes / file_*_enumTypes arrays, and the init()\n")
	fmt.Fprintf(out, "// that registers everything with protoregistry.GlobalFiles and\n")
	fmt.Fprintf(out, "// protoregistry.GlobalTypes. None of these are called on the marshal /\n")
	fmt.Fprintf(out, "// unmarshal / size hot path.\n")
	fmt.Fprintf(out, "//\n")
	fmt.Fprintf(out, "// Why a separate file? Putting this code (plus its descriptorpb /\n")
	fmt.Fprintf(out, "// protoreflect / protoimpl imports — ~64KB of descriptorpb alone, ~377KB\n")
	fmt.Fprintf(out, "// added to __TEXT overall) next to the hot Marshal/Unmarshal functions\n")
	fmt.Fprintf(out, "// caused a measured +7–14%% regression on otlp benchmarks (UnmarshalProfiles\n")
	fmt.Fprintf(out, "// regressed by +12.6%%) due to icache / iTLB / BTB pressure: the hot\n")
	fmt.Fprintf(out, "// loops themselves were unchanged, but cold reflection code interleaved\n")
	fmt.Fprintf(out, "// in the same compilation unit shifted hot functions onto different\n")
	fmt.Fprintf(out, "// cache sets and pushed them ~131KB further into the binary. Emitting\n")
	fmt.Fprintf(out, "// the cold half here, in its own .o, lets the linker place it away\n")
	fmt.Fprintf(out, "// from the hot half and recovers that throughput.\n")
	fmt.Fprintf(out, "//\n")
	fmt.Fprintf(out, "// See compiler/generator/emit_registration.go for the full rationale\n")
	fmt.Fprintf(out, "// and the benchmark methodology. DO NOT inline this file's contents\n")
	fmt.Fprintf(out, "// back into the main .pb.go without re-measuring.\n\n")
}

// emitCompareFileBanner is the parallel of emitReflectFileBanner for the
// per-message Compare methods. Same icache-pressure rationale: Compare is
// cold (never called from Marshal/Unmarshal/Size), so emitting it next to
// the hot path was measured to cost ~9% geomean on OTel hot benchmarks
// even though the hot code itself didn't change a byte. Splitting it into
// its own compilation unit lets the linker keep the cold half away from
// the hot half.
func (fg *FileGenerator) emitCompareFileBanner(out *bytes.Buffer) {
	fmt.Fprintf(out, "// Per-message Compare() methods for %s.\n", fg.fd.Path())
	fmt.Fprintf(out, "//\n")
	fmt.Fprintf(out, "// Compare returns -1/0/+1 like bytes.Compare with the gogoproto.compare\n")
	fmt.Fprintf(out, "// nil/wrong-type preamble. Always emitted on every message; callers that\n")
	fmt.Fprintf(out, "// don't use it can rely on Go's dead-code elimination to drop the body.\n")
	fmt.Fprintf(out, "//\n")
	fmt.Fprintf(out, "// Why a separate file? Compare is never called from Marshal/Unmarshal/Size,\n")
	fmt.Fprintf(out, "// but emitting it next to those hot functions in the main .pb.go pushed\n")
	fmt.Fprintf(out, "// them onto different cache sets and produced a measured ~9%% geomean\n")
	fmt.Fprintf(out, "// regression on OTel benchmarks (UnmarshalMap +14%%, MarshalSingleSpan +13%%)\n")
	fmt.Fprintf(out, "// purely from icache / iTLB / BTB pressure. Splitting Compare into its own\n")
	fmt.Fprintf(out, "// compilation unit gives the linker freedom to place the cold half away\n")
	fmt.Fprintf(out, "// from the hot half — same trick the _reflect.pb.go split uses.\n")
	fmt.Fprintf(out, "//\n")
	fmt.Fprintf(out, "// See compiler/generator/emit_compare.go for the full rationale and the\n")
	fmt.Fprintf(out, "// benchmark methodology. DO NOT inline this file's contents back into\n")
	fmt.Fprintf(out, "// the main .pb.go without re-measuring.\n\n")
}

// emitEqualFileBanner writes a comment block at the top of every
// `_equal.pb.go` file explaining what's here, why it isn't appended to the
// main `.pb.go`, and what to do if a future maintainer wants to undo the
// split. Mirrors emitReflectFileBanner — same rationale, smaller surface.
func (fg *FileGenerator) emitEqualFileBanner(out *bytes.Buffer) {
	fmt.Fprintf(out, "// Per-message Equal() methods for %s.\n", fg.fd.Path())
	fmt.Fprintf(out, "//\n")
	fmt.Fprintf(out, "// Equal is never called from Marshal / Unmarshal / Size on the hot path;\n")
	fmt.Fprintf(out, "// emitting it next to those methods in the same compilation unit shifts\n")
	fmt.Fprintf(out, "// the hot functions onto different cache lines / iTLB pages in the linked\n")
	fmt.Fprintf(out, "// binary without buying anything for the (uncommon) Equal callers.\n")
	fmt.Fprintf(out, "//\n")
	fmt.Fprintf(out, "// Splitting Equal out into its own .o gives the linker freedom to place\n")
	fmt.Fprintf(out, "// it away from the hot paths, mirroring the reflect/registration split\n")
	fmt.Fprintf(out, "// documented in the companion _reflect.pb.go banner and in\n")
	fmt.Fprintf(out, "// compiler/generator/emit_registration.go. DO NOT inline this file's\n")
	fmt.Fprintf(out, "// contents back into the main .pb.go without re-measuring.\n\n")
}

func (fg *FileGenerator) emitAllEnums(fd protoreflect.FileDescriptor) {
	for i := 0; i < fd.Enums().Len(); i++ {
		fg.emitEnum(fd.Enums().Get(i))
	}
	forEachMessage(fd, func(md protoreflect.MessageDescriptor) {
		for i := 0; i < md.Enums().Len(); i++ {
			fg.emitEnum(md.Enums().Get(i))
		}
	})
}

// emitAllEnumReflectMethods emits the Descriptor()/Type()/Number() methods
// for every enum into the companion reflect file in TypeBuilder's flattened
// order (file-level enums first, then for each message in flattenedMessages
// order its direct nested enums). The `nextEnumIndex` values assigned here
// match the positions Build() will populate in `file_*_enumTypes`.
//
// `emitAllEnums` (which emits enum type + constants + name maps + String
// into the main .pb.go) uses a different order — that's fine because the
// main file's enum emissions don't index into the EnumInfo array. Only the
// reflection methods need this strict ordering.
func (fg *FileGenerator) emitAllEnumReflectMethods(fd protoreflect.FileDescriptor) {
	for _, ed := range flattenedEnums(fd) {
		fg.emitEnumReflect(ed)
	}
}

func (fg *FileGenerator) emitAllOneofs(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, func(md protoreflect.MessageDescriptor) {
		for i := 0; i < md.Oneofs().Len(); i++ {
			oo := md.Oneofs().Get(i)
			if oo.IsSynthetic() {
				continue
			}
			fg.emitOneof(md, oo)
		}
	})
}

func (fg *FileGenerator) emitAllStructs(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, fg.emitStruct)
}

func (fg *FileGenerator) emitAllHasMethods(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, fg.emitHasMethods)
}

func (fg *FileGenerator) emitAllSizeMethods(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, fg.emitSize)
}

func (fg *FileGenerator) emitAllMarshalMethods(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, fg.emitMarshal)
}

func (fg *FileGenerator) emitAllUnmarshalMethods(fd protoreflect.FileDescriptor) {
	// The depth constant and skip helper live in protohelpers
	// (MaxUnmarshalDepth / SkipValue) rather than being emitted per file:
	// package-level declarations would collide when multiple .proto files
	// generate into one Go package (Tempo's tempopb, Loki's logproto).
	forEachMessage(fd, fg.emitUnmarshal)
}

// forEachMessage calls fn for every message reachable from fd, skipping
// nested map-entry messages and visiting nested messages before their
// parent (post-order).
func forEachMessage(fd protoreflect.FileDescriptor, fn func(protoreflect.MessageDescriptor)) {
	for i := 0; i < fd.Messages().Len(); i++ {
		walkMessages(fd.Messages().Get(i), fn)
	}
}

func walkMessages(md protoreflect.MessageDescriptor, fn func(protoreflect.MessageDescriptor)) {
	for i := 0; i < md.Messages().Len(); i++ {
		nested := md.Messages().Get(i)
		if nested.IsMapEntry() {
			continue
		}
		walkMessages(nested, fn)
	}
	fn(md)
}

// flattenedMessages returns every message in fd in the order that
// protoimpl.TypeBuilder / filedesc.Builder consumes them — layered per parent,
// NOT depth-first pre-order. At each parent we emit all direct children before
// recursing into any of them, matching protoc-gen-go's `walkMessages` in
// google.golang.org/protobuf/cmd/protoc-gen-go/internal_gengo/init.go.
//
// Map-entry messages are INCLUDED. Their `goTypes` slot is `nil`, but Build()
// still requires the MessageInfos slice to have one slot per message including
// map entries (filedesc.Builder.unmarshalCounts walks the raw FileDescriptorProto
// and counts every nested message regardless of map-entry status).
//
// Callers that emit per-message *Go code* (ProtoReflect methods, OneofWrappers
// assignments) must skip map-entry positions but still advance their index
// counter so the index stays aligned with the slot in `file_*_msgTypes` that
// Build() will populate.
func flattenedMessages(fd protoreflect.FileDescriptor) []protoreflect.MessageDescriptor {
	var out []protoreflect.MessageDescriptor
	var visit func(md protoreflect.MessageDescriptor)
	visit = func(md protoreflect.MessageDescriptor) {
		for i := 0; i < md.Messages().Len(); i++ {
			out = append(out, md.Messages().Get(i))
		}
		for i := 0; i < md.Messages().Len(); i++ {
			visit(md.Messages().Get(i))
		}
	}
	for i := 0; i < fd.Messages().Len(); i++ {
		out = append(out, fd.Messages().Get(i))
	}
	for i := 0; i < fd.Messages().Len(); i++ {
		visit(fd.Messages().Get(i))
	}
	return out
}

// flattenedEnums returns every enum in TypeBuilder ordering: file-level enums
// first (in declaration order), then for each message visited in
// `flattenedMessages` order, the message's direct nested enums (declaration
// order) before recursing into the message's nested messages.
func flattenedEnums(fd protoreflect.FileDescriptor) []protoreflect.EnumDescriptor {
	out := make([]protoreflect.EnumDescriptor, 0, fd.Enums().Len())
	for i := 0; i < fd.Enums().Len(); i++ {
		out = append(out, fd.Enums().Get(i))
	}
	for _, md := range flattenedMessages(fd) {
		for i := 0; i < md.Enums().Len(); i++ {
			out = append(out, md.Enums().Get(i))
		}
	}
	return out
}

// isRealOneof returns true if the field belongs to a non-synthetic oneof.
func isRealOneof(fd protoreflect.FieldDescriptor) bool {
	oo := fd.ContainingOneof()
	return oo != nil && !oo.IsSynthetic()
}

// oneofInterfaceName returns the Go interface name for a oneof.
func oneofInterfaceName(md protoreflect.MessageDescriptor, oo protoreflect.OneofDescriptor) string {
	return goMessageTypeName(md) + "_" + snakeToPascal(string(oo.Name()))
}

// oneofVariantName returns the Go struct name for a oneof variant.
func oneofVariantName(md protoreflect.MessageDescriptor, fd protoreflect.FieldDescriptor) string {
	return goMessageTypeName(md) + "_" + snakeToPascal(string(fd.Name()))
}

// buildImportMapping reads proto files (recursively) and builds a mapping
// from import paths to file contents. Top-level files are registered under
// the package-derived path so existing flat layouts keep working; nested
// files use their on-disk relative path as the import key.
//
// Each file is registered under exactly one canonical path. Imports across
// files must use that canonical form: package-derived for top-level files,
// relative-path for nested files. Registering the same content under two
// keys would cause protocompile to compile it twice and emit duplicate-symbol
// errors when a consumer imports it via the non-canonical name.
func buildImportMapping(protoDir string) (map[string][]byte, []string, map[string]string, error) {
	mapping := make(map[string][]byte)
	pathToKey := make(map[string]string)
	var importPaths []string
	pkgRE := regexp.MustCompile(`(?m)^package\s+([\w.]+)\s*;`)

	err := filepath.WalkDir(protoDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".proto") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		m := pkgRE.FindSubmatch(content)
		if m == nil {
			return fmt.Errorf("no package found in %s", path)
		}

		rel, err := filepath.Rel(protoDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		var key string
		if strings.Contains(rel, "/") {
			key = rel
		} else {
			pkg := string(m[1])
			key = strings.ReplaceAll(pkg, ".", "/") + "/" + d.Name()
		}
		if _, exists := mapping[key]; exists {
			return fmt.Errorf("duplicate import key %q (from %s)", key, path)
		}
		mapping[key] = content
		importPaths = append(importPaths, key)

		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		pathToKey[abs] = key
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}

	sort.Strings(importPaths)
	return mapping, importPaths, pathToKey, nil
}

// memResolver serves proto file content from memory.
type memResolver struct {
	files map[string][]byte
}

func (r *memResolver) FindFileByPath(path string) (protocompile.SearchResult, error) {
	content, ok := r.files[path]
	if !ok {
		return protocompile.SearchResult{}, os.ErrNotExist
	}
	return protocompile.SearchResult{
		Source: bytes.NewReader(content),
	}, nil
}
