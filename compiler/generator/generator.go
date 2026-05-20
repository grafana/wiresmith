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

	// goPackages maps a proto package name to the raw value of its
	// `option go_package`. Populated during Generate after compilation.
	goPackages map[string]string

	// pointerExt is the linked extension descriptor for
	// `(wiresmith.options.pointer)`, resolved once after Compile and consulted
	// by hasPointerOption. It is always non-nil after a successful Compile
	// because the embedded `wiresmith/options.proto` is always part of the
	// input set.
	pointerExt protoreflect.FieldDescriptor
}

type FileGenerator struct {
	fd      protoreflect.FileDescriptor
	module  string
	imports *ImportTracker
	body    *bytes.Buffer

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
	mapping, importPaths, err := buildImportMapping(g.ProtoDir)
	if err != nil {
		return fmt.Errorf("building import mapping: %w", err)
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
	outputs := make(map[string]string, len(results))
	for _, fd := range results {
		if isInternalSchemaFile(fd) {
			continue
		}
		outPath := g.outputPathFor(fd)
		if prev, exists := outputs[outPath]; exists {
			return fmt.Errorf("output collision at %s: %q and %q produce the same file (same package %q + basename)",
				outPath, prev, fd.Path(), fd.Package())
		}
		outputs[outPath] = fd.Path()
	}

	for _, fd := range results {
		if isInternalSchemaFile(fd) {
			continue
		}
		if err := g.generateFile(fd); err != nil {
			return fmt.Errorf("generating %s: %w", fd.Path(), err)
		}
	}
	return nil
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
func (g *Generator) validateDestinations(results linker.Files) error {
	dirOwner := make(map[string]string)
	for _, fd := range results {
		// Skip the embedded options schema for the same reason
		// collectGoPackages does — it doesn't produce output and shouldn't
		// claim a destination on behalf of the wiresmith.options package.
		if isInternalSchemaFile(fd) {
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
	// A proto file with no messages and no enums has nothing to generate.
	// Emitting an empty init() leaves the unconditional `protohelpers`
	// import unused and the unmarshal skipValue helper dead code — both
	// reject go build. Skip the file rather than guarding every emitter.
	if fd.Messages().Len() == 0 && fd.Enums().Len() == 0 {
		return nil
	}

	fg := &FileGenerator{
		fd:         fd,
		module:     g.Module,
		imports:    newImportTracker(g.Module, string(fd.Package()), g.goPackages),
		body:       &bytes.Buffer{},
		pointerExt: g.pointerExt,
	}

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
	fg.emitRegistration(fd)

	var out bytes.Buffer
	fg.emitHeader(&out)
	out.Write(fg.body.Bytes())

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		// Write unformatted for debugging
		formatted = out.Bytes()
		fmt.Fprintf(os.Stderr, "warning: format error for %s: %v\n", fd.Path(), err)
	}

	outPath := g.outputPathFor(fd)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}

	return os.WriteFile(outPath, formatted, 0o644)
}

func (fg *FileGenerator) emitHeader(out *bytes.Buffer) {
	pkg := string(fg.fd.Package())
	fmt.Fprintf(out, "// Code generated by wiresmith. DO NOT EDIT.\n")
	fmt.Fprintf(out, "// source: %s\n\n", fg.fd.Path())
	fmt.Fprintf(out, "package %s\n\n", fg.imports.resolvePkgName(pkg))

	type imp struct {
		path    string
		alias   string
		natural string
	}
	var imps []imp
	for p, e := range fg.imports.imports {
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
func buildImportMapping(protoDir string) (map[string][]byte, []string, error) {
	mapping := make(map[string][]byte)
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
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	sort.Strings(importPaths)
	return mapping, importPaths, nil
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
