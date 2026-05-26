package generator

import (
	"bytes"
	"context"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"wiresmith/compiler/types"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
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

	// goPackages maps a proto package name to the raw value of its
	// `option go_package`. Populated during Generate after compilation.
	goPackages map[string]string

	// emitFilter is the set of fd.Path() values to emit, derived from Files
	// at the start of Generate. Nil means "emit every shouldGenerateFile
	// candidate" (the empty-Files default).
	emitFilter map[string]bool

	// pointerExt is the linked extension descriptor for
	// `(wiresmith.options.pointer)`, resolved once after Compile and consulted
	// by hasPointerOption. It is always non-nil after a successful Compile
	// because the embedded `wiresmith/options.proto` is always part of the
	// input set.
	pointerExt protoreflect.FieldDescriptor
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

	// fileVarName is a sanitized proto file path used as prefix for
	// file-level variables (descriptor, MessageInfo/EnumInfo arrays).
	fileVarName   string
	nextMsgIndex  int
	nextEnumIndex int

	// pointerExt is the cached pointer-option extension descriptor copied from
	// the parent Generator. Plumbed through so the per-field option lookup
	// doesn't have to reach back up to the Generator.
	pointerExt protoreflect.FieldDescriptor
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

// fieldContext builds a FieldContext from a field descriptor.
func (fg *FileGenerator) fieldContext(fd protoreflect.FieldDescriptor) types.FieldContext {
	ctx := types.FieldContext{}
	if fd.Kind() == protoreflect.EnumKind {
		ctx.EnumType = fg.imports.goEnumType(fd.Enum())
	}
	if fd.Kind() == protoreflect.MessageKind {
		ctx.MessageType = fg.imports.goSingularType(fd)
		msgPkg := string(fd.Message().ParentFile().Package())
		ctx.IsSamePackage = (msgPkg == fg.imports.selfPkg)
	}
	return ctx
}

func (g *Generator) Generate(ctx context.Context) error {
	mapping, importPaths, pathToKey, err := buildImportMapping(g.ProtoDir)
	if err != nil {
		return fmt.Errorf("building import mapping: %w", err)
	}

	// Reset on every call so a reused Generator can't carry an emitFilter
	// from a prior scoped run into a subsequent walk-everything one.
	g.emitFilter = nil
	if len(g.Files) > 0 {
		g.emitFilter = make(map[string]bool, len(g.Files))
		for _, src := range g.Files {
			abs, err := filepath.Abs(src)
			if err != nil {
				return fmt.Errorf("resolving %q: %w", src, err)
			}
			key, ok := pathToKey[abs]
			if !ok {
				return fmt.Errorf("file %q is not under --proto_path=%q (or is not a .proto file in the walked tree)", src, g.ProtoDir)
			}
			g.emitFilter[key] = true
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
		return fmt.Errorf("user proto at %q conflicts with the embedded wiresmith schema — remove the on-disk file; wiresmith serves it from its own embed", embeddedOptionsPath)
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

	results, err := compiler.Compile(ctx, importPaths...)
	if err != nil {
		return fmt.Errorf("compiling protos: %w", err)
	}

	if err := g.resolvePointerExtension(results); err != nil {
		return err
	}
	if err := g.validatePointerOptions(results); err != nil {
		return err
	}
	if err := g.validateNoValueCycles(results); err != nil {
		return err
	}

	if err := g.collectGoPackages(results); err != nil {
		return err
	}
	if err := g.validateDestinations(results); err != nil {
		return err
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
	// Each proto emits two outputs (the main .pb.go and the companion
	// _reflect.pb.go), so an input like foo_reflect.proto would generate
	// foo_reflect.pb.go that collides with foo.proto's reflect companion.
	// Check both paths against the same map.
	outputs := make(map[string]string, 2*len(results))
	for _, fd := range results {
		if !g.shouldEmit(fd) {
			continue
		}
		for _, outPath := range []string{g.outputPathFor(fd), g.outputReflectPathFor(fd)} {
			if prev, exists := outputs[outPath]; exists {
				return fmt.Errorf("output collision at %s: %q and %q both write to this path (proto package %q)",
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
			return fmt.Errorf("generating %s: %w", fd.Path(), err)
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

// shouldGenerateFile reports whether fd contributes a .pb.go to the output.
// Internal schema files (wiresmith.options) and proto files with no messages
// or enums are skipped — the latter would emit only an empty init() plus
// unused imports, which go build rejects.
func shouldGenerateFile(fd protoreflect.FileDescriptor) bool {
	if isInternalSchemaFile(fd) {
		return false
	}
	return fd.Messages().Len() > 0 || fd.Enums().Len() > 0
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
// The directory is taken straight from destFor — the single source of truth
// the import tracker and consumers also use, so they can't disagree.
func (g *Generator) outputPathFor(fd protoreflect.FileDescriptor) string {
	dest := destFor(g.Module, string(fd.Package()), g.goPackages)
	base := strings.TrimSuffix(filepath.Base(fd.Path()), ".proto") + ".pb.go"
	return filepath.Join(g.OutDir, dest.relDir, base)
}

// collectGoPackages records every file's `option go_package` value, keyed
// by proto package. Every file belonging to one proto package must declare
// the same value — including "unset". An asymmetric mix (file A sets it,
// file B in the same package omits it) is rejected too: silently treating
// the unset file as if it inherited A's value would contradict the
// upfront-agreement contract and could move generated files unexpectedly.
//
// The path-traversal check sits here because it only makes sense against
// the raw go_package string; cross-mode destination collisions are caught
// later in validateDestinations against the resolved goDest.
func (g *Generator) collectGoPackages(results linker.Files) error {
	g.goPackages = make(map[string]string)
	base := effectiveBase(g.Module)

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

		if goPkg == "" {
			continue
		}
		g.goPackages[pkg] = goPkg

		// Reject `..` segments in go_package values that fall under our
		// base. Without this, filepath.Join(g.OutDir, relDir) would
		// silently write outside the configured output directory.
		importPath, _ := parseGoPackage(goPkg)
		if importPath != base && !strings.HasPrefix(importPath, base+"/") {
			continue
		}
		for _, seg := range strings.Split(importPath, "/") {
			if seg == ".." {
				return fmt.Errorf("invalid go_package %q in %s: path contains '..' segment",
					goPkg, fd.Path())
			}
		}
	}
	return nil
}

// validateDestinations runs destFor for every compiled file and rejects the
// case where two distinct proto packages resolve to the same Go directory.
// This catches three failure modes at once: two protos with the same in-base
// go_package, a go_package shadowing the default mapping of another package,
// and a default mapping shadowing an explicit go_package. Routing every file
// through destFor (not just those with go_package set) is the part that
// makes the cross-mode case visible.
//
// Files that won't emit are skipped: an empty .proto cannot collide on disk
// with a non-empty proto resolving to the same Go dir, because it writes
// nothing there. Including it would produce a false-positive collision error.
func (g *Generator) validateDestinations(results linker.Files) error {
	dirOwner := make(map[string]string)
	for _, fd := range results {
		if !g.shouldEmit(fd) {
			continue
		}
		protoPkg := string(fd.Package())
		dest := destFor(g.Module, protoPkg, g.goPackages)
		if owner, ok := dirOwner[dest.relDir]; ok && owner != protoPkg {
			return fmt.Errorf("destination %q is claimed by both proto packages %q and %q (check go_package options)",
				dest.relDir, owner, protoPkg)
		}
		dirOwner[dest.relDir] = protoPkg
	}
	return nil
}

func (g *Generator) generateFile(fd protoreflect.FileDescriptor) error {
	fg := &FileGenerator{
		fd:             fd,
		module:         g.Module,
		imports:        newImportTracker(g.Module, string(fd.Package()), g.goPackages),
		body:           &bytes.Buffer{},
		reflectImports: newImportTracker(g.Module, string(fd.Package()), g.goPackages),
		reflectBody:    &bytes.Buffer{},
		fileVarName:    sanitizeFileVarName(fd.Path()),
		pointerExt:     g.pointerExt,
	}

	// Main file: hot paths and the user-facing API. These emitters write to
	// fg.body / fg.imports.
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

	// Write the main .pb.go file.
	var mainOut bytes.Buffer
	fg.emitHeader(&mainOut, fg.imports)
	mainOut.Write(fg.body.Bytes())
	if err := g.writeFormatted(g.outputPathFor(fd), mainOut.Bytes(), fd.Path()); err != nil {
		return err
	}

	// Write the companion _reflect.pb.go file. Skip if the proto declared no
	// messages and no enums — there's nothing to register and emitting a file
	// with just a package clause would be misleading.
	if fg.reflectBody.Len() == 0 {
		return nil
	}
	var reflOut bytes.Buffer
	fg.emitHeader(&reflOut, fg.reflectImports)
	fg.emitReflectFileBanner(&reflOut)
	reflOut.Write(fg.reflectBody.Bytes())
	return g.writeFormatted(g.outputReflectPathFor(fd), reflOut.Bytes(), fd.Path())
}

// writeFormatted gofmt-formats src and writes it to outPath. If gofmt fails
// (typically a generator bug producing invalid Go), the unformatted bytes
// are written instead so the broken output is visible for debugging.
func (g *Generator) writeFormatted(outPath string, src []byte, sourceProto string) error {
	formatted, err := format.Source(src)
	if err != nil {
		formatted = src
		fmt.Fprintf(os.Stderr, "warning: format error for %s -> %s: %v\n", sourceProto, outPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, formatted, 0o644)
}

// outputReflectPathFor returns the path for the companion `_reflect.pb.go`
// file. It sits next to the main `.pb.go` so callers find both files in the
// same package directory; the `_reflect` suffix is conventional and matches
// the protoc-gen-go-impl convention where extra generated material lives in
// `<name>_<flavor>.pb.go`.
func (g *Generator) outputReflectPathFor(fd protoreflect.FileDescriptor) string {
	dest := destFor(g.Module, string(fd.Package()), g.goPackages)
	base := strings.TrimSuffix(filepath.Base(fd.Path()), ".proto") + "_reflect.pb.go"
	return filepath.Join(g.OutDir, dest.relDir, base)
}

// emitHeader writes the "Code generated by ..." banner, the package clause,
// and the import block for one output file. The tracker argument selects
// which set of imports gets emitted (main file vs. companion reflect file).
// Each output file in the package needs its own import block — the Go
// compiler can't deduce that a `protoreflect` symbol used in foo_reflect.pb.go
// satisfies the import in foo.pb.go.
func (fg *FileGenerator) emitHeader(out *bytes.Buffer, tracker *ImportTracker) {
	pkg := string(fg.fd.Package())
	fmt.Fprintf(out, "// Code generated by wiresmith. DO NOT EDIT.\n")
	fmt.Fprintf(out, "// source: %s\n\n", fg.fd.Path())
	fmt.Fprintf(out, "package %s\n\n", tracker.resolvePkgName(pkg))

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
	// Emit the max recursion depth constant and skip helpers once per file.
	fmt.Fprintf(fg.body, "const maxUnmarshalDepth = 10000\n\n")
	fg.emitSkipValueHelper()
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
