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

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/reporter"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

type Generator struct {
	Module        string
	OutDir        string
	HelpersImport string
	GogoCompat    bool

	// ProtoPaths lists directories to search for .proto files.
	// When ProtoFiles is empty, the first path is scanned for .proto files to compile.
	// All paths are used for resolving imports.
	ProtoPaths []string

	// ProtoFiles lists specific .proto files to compile (positional args).
	// When set, only these files are compiled; the first proto path is NOT scanned.
	ProtoFiles []string

	// Deprecated: use ProtoPaths instead. Kept for backward compatibility.
	ProtoDir string
}

type FileGenerator struct {
	fd      protoreflect.FileDescriptor
	gen     *Generator
	module  string
	imports *ImportTracker
	body    *bytes.Buffer
}

func (g *Generator) helpersImportPath() string {
	if g.HelpersImport != "" {
		return g.HelpersImport
	}
	return g.Module + "/gen/protohelpers"
}

func (g *Generator) protoPaths() []string {
	if len(g.ProtoPaths) > 0 {
		return g.ProtoPaths
	}
	if g.ProtoDir != "" {
		return []string{g.ProtoDir}
	}
	return []string{"proto"}
}

func (g *Generator) Generate(ctx context.Context) error {
	paths := g.protoPaths()

	var mapping map[string][]byte
	var importPaths []string
	var err error

	if len(g.ProtoFiles) > 0 {
		// Compile only the explicitly named proto files.
		mapping, importPaths, err = buildImportMappingFromFiles(g.ProtoFiles)
	} else {
		// Scan the first proto path directory.
		mapping, importPaths, err = buildImportMapping(paths[0])
	}
	if err != nil {
		return fmt.Errorf("building import mapping: %w", err)
	}

	resolver := &multiPathResolver{
		files: mapping,
		paths: paths,
	}
	compiler := protocompile.Compiler{
		Resolver: resolver,
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

	for _, fd := range results {
		if err := g.generateFile(fd); err != nil {
			return fmt.Errorf("generating %s: %w", fd.Path(), err)
		}
	}
	return nil
}

// goFilePackage returns the Go package name for a file descriptor,
// respecting the go_package option if set.
func goFilePackage(fd protoreflect.FileDescriptor) string {
	opts, ok := fd.Options().(*descriptorpb.FileOptions)
	if ok && opts != nil && opts.GoPackage != nil {
		goPkg := opts.GetGoPackage()
		// go_package can be "path;name" or just "name" or "full/import/path"
		if idx := strings.LastIndex(goPkg, ";"); idx >= 0 {
			return goPkg[idx+1:]
		}
		if idx := strings.LastIndex(goPkg, "/"); idx >= 0 {
			return goPkg[idx+1:]
		}
		return goPkg
	}
	return goPackageName(string(fd.Package()))
}

func (g *Generator) generateFile(fd protoreflect.FileDescriptor) error {
	fg := &FileGenerator{
		fd:      fd,
		gen:     g,
		module:  g.Module,
		imports: newImportTracker(g, g.Module, string(fd.Package())),
		body:    &bytes.Buffer{},
	}

	fg.emitAllEnums(fd)
	fg.emitAllOneofs(fd)
	fg.emitAllStructs(fd)
	fg.emitAllSizeMethods(fd)
	fg.emitAllMarshalMethods(fd)
	fg.emitAllUnmarshalMethods(fd)

	if g.GogoCompat {
		if !isFileOptionFalse(fd, 63001) { // gogoproto.goproto_getters_all
			fg.emitAllGetters(fd)
		}
		if !isFileOptionFalse(fd, 63013) { // gogoproto.equal_all
			fg.emitAllEqualMethods(fd)
		}
		fg.emitAllStringMethods(fd)
		fg.emitAllGogoMethods(fd)
		fg.emitRegistration(fd)
	}

	var out bytes.Buffer
	fg.emitHeader(&out)
	out.Write(fg.body.Bytes())

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		// Write unformatted for debugging
		formatted = out.Bytes()
		fmt.Fprintf(os.Stderr, "warning: format error for %s: %v\n", fd.Path(), err)
	}

	// In gogo compat mode, use go_package for the output directory
	// (e.g., go_package="alertspb" → <outDir>/alertspb/).
	// In default mode, derive from proto package name.
	var subDir string
	if g.GogoCompat {
		subDir = goFilePackage(fd)
	} else {
		subDir = goPackageDir(string(fd.Package()))
	}
	dir := filepath.Join(g.OutDir, subDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Use the proto filename without extension as the Go filename.
	// In gogo compat mode, use .pb.go suffix to match protoc convention
	// and avoid conflicts with hand-written .go files.
	base := filepath.Base(fd.Path())
	if g.GogoCompat {
		base = strings.TrimSuffix(base, ".proto") + ".pb.go"
	} else {
		base = strings.TrimSuffix(base, ".proto") + ".go"
	}
	outPath := filepath.Join(dir, base)

	return os.WriteFile(outPath, formatted, 0o644)
}

func (fg *FileGenerator) emitHeader(out *bytes.Buffer) {
	pkgName := goFilePackage(fg.fd)
	fmt.Fprintf(out, "// Code generated by wiresmith. DO NOT EDIT.\n")
	fmt.Fprintf(out, "// source: %s\n\n", fg.fd.Path())
	fmt.Fprintf(out, "package %s\n\n", pkgName)

	// Collect all imports
	type imp struct {
		path  string
		alias string
	}
	var imps []imp
	for path, alias := range fg.imports.imports {
		imps = append(imps, imp{path, alias})
	}
	sort.Slice(imps, func(i, j int) bool { return imps[i].path < imps[j].path })

	if len(imps) > 0 {
		fmt.Fprintf(out, "import (\n")
		for _, i := range imps {
			if i.alias != "" && !strings.HasSuffix(i.path, "/"+i.alias) {
				fmt.Fprintf(out, "\t%s %q\n", i.alias, i.path)
			} else {
				fmt.Fprintf(out, "\t%q\n", i.path)
			}
		}
		fmt.Fprintf(out, ")\n\n")
	}
}

func (fg *FileGenerator) emitAllEnums(fd protoreflect.FileDescriptor) {
	for i := 0; i < fd.Enums().Len(); i++ {
		fg.emitEnum(fd.Enums().Get(i))
	}
	for i := 0; i < fd.Messages().Len(); i++ {
		fg.emitNestedEnums(fd.Messages().Get(i))
	}
}

func (fg *FileGenerator) emitNestedEnums(md protoreflect.MessageDescriptor) {
	for i := 0; i < md.Enums().Len(); i++ {
		fg.emitEnum(md.Enums().Get(i))
	}
	for i := 0; i < md.Messages().Len(); i++ {
		fg.emitNestedEnums(md.Messages().Get(i))
	}
}

func (fg *FileGenerator) emitAllOneofs(fd protoreflect.FileDescriptor) {
	for i := 0; i < fd.Messages().Len(); i++ {
		fg.emitMessageOneofs(fd.Messages().Get(i))
	}
}

func (fg *FileGenerator) emitMessageOneofs(md protoreflect.MessageDescriptor) {
	for i := 0; i < md.Oneofs().Len(); i++ {
		oo := md.Oneofs().Get(i)
		if oo.IsSynthetic() {
			continue
		}
		fg.emitOneof(md, oo)
	}
	for i := 0; i < md.Messages().Len(); i++ {
		fg.emitMessageOneofs(md.Messages().Get(i))
	}
}

func (fg *FileGenerator) emitAllStructs(fd protoreflect.FileDescriptor) {
	for i := 0; i < fd.Messages().Len(); i++ {
		fg.emitMessageStructs(fd.Messages().Get(i))
	}
}

func (fg *FileGenerator) emitMessageStructs(md protoreflect.MessageDescriptor) {
	// Nested messages first
	for i := 0; i < md.Messages().Len(); i++ {
		fg.emitMessageStructs(md.Messages().Get(i))
	}
	fg.emitStruct(md)
}

func (fg *FileGenerator) emitAllSizeMethods(fd protoreflect.FileDescriptor) {
	for i := 0; i < fd.Messages().Len(); i++ {
		fg.emitSizeMethods(fd.Messages().Get(i))
	}
}

func (fg *FileGenerator) emitSizeMethods(md protoreflect.MessageDescriptor) {
	for i := 0; i < md.Messages().Len(); i++ {
		fg.emitSizeMethods(md.Messages().Get(i))
	}
	fg.emitSize(md)
}

func (fg *FileGenerator) emitAllMarshalMethods(fd protoreflect.FileDescriptor) {
	for i := 0; i < fd.Messages().Len(); i++ {
		fg.emitMarshalMethods(fd.Messages().Get(i))
	}
}

func (fg *FileGenerator) emitMarshalMethods(md protoreflect.MessageDescriptor) {
	for i := 0; i < md.Messages().Len(); i++ {
		fg.emitMarshalMethods(md.Messages().Get(i))
	}
	fg.emitMarshal(md)
}

func (fg *FileGenerator) emitAllUnmarshalMethods(fd protoreflect.FileDescriptor) {
	// Emit the skipField helper once per package
	fg.emitSkipFieldHelper()
	for i := 0; i < fd.Messages().Len(); i++ {
		fg.emitUnmarshalMethods(fd.Messages().Get(i))
	}
}

func (fg *FileGenerator) emitUnmarshalMethods(md protoreflect.MessageDescriptor) {
	for i := 0; i < md.Messages().Len(); i++ {
		fg.emitUnmarshalMethods(md.Messages().Get(i))
	}
	fg.emitUnmarshal(md)
}

// isMessageOptionFalse checks if a boolean message-level option is explicitly set to false.
func isMessageOptionFalse(md protoreflect.MessageDescriptor, fieldNum protoreflect.FieldNumber) bool {
	opts, ok := md.Options().(*descriptorpb.MessageOptions)
	if !ok || opts == nil {
		return false
	}
	b, err := proto.Marshal(opts)
	if err != nil {
		return false
	}
	return containsVarintField(b, fieldNum, 0)
}

// isFileOptionFalse checks if a boolean file-level option (by field number) is
// explicitly set to false. Used for gogoproto file options like equal_all (65017),
// goproto_getters_all (65018), etc.
func isFileOptionFalse(fd protoreflect.FileDescriptor, fieldNum protoreflect.FieldNumber) bool {
	opts, ok := fd.Options().(*descriptorpb.FileOptions)
	if !ok || opts == nil {
		return false
	}
	b, err := proto.Marshal(opts)
	if err != nil {
		return false
	}
	return containsVarintField(b, fieldNum, 0)
}

// leadingComment returns the leading comment for a descriptor, formatted as
// Go comment lines. Returns empty string if no comment exists.
func leadingComment(d protoreflect.Descriptor) string {
	loc := d.ParentFile().SourceLocations().ByDescriptor(d)
	comment := strings.TrimSpace(loc.LeadingComments)
	if comment == "" {
		return ""
	}
	var b strings.Builder
	for _, line := range strings.Split(comment, "\n") {
		// Protobuf source info preserves leading whitespace from the .proto file;
		// strip it so the Go comment is clean.
		line = strings.TrimSpace(line)
		if line == "" {
			b.WriteString("//\n")
		} else {
			b.WriteString("// ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
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

// buildImportMapping reads proto files from the primary directory and builds
// a mapping from import paths to file contents.
func buildImportMapping(protoDir string) (map[string][]byte, []string, error) {
	entries, err := os.ReadDir(protoDir)
	if err != nil {
		return nil, nil, err
	}

	mapping := make(map[string][]byte)
	var importPaths []string
	pkgRE := regexp.MustCompile(`(?m)^package\s+([\w.]+)\s*;`)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".proto") {
			continue
		}
		fullPath := filepath.Join(protoDir, entry.Name())
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, nil, err
		}

		m := pkgRE.FindSubmatch(content)
		if m == nil {
			return nil, nil, fmt.Errorf("no package found in %s", entry.Name())
		}

		pkg := string(m[1])
		importPath := strings.ReplaceAll(pkg, ".", "/") + "/" + entry.Name()
		mapping[importPath] = content
		importPaths = append(importPaths, importPath)
	}

	return mapping, importPaths, nil
}

// buildImportMappingFromFiles reads specific proto files and builds a mapping
// from import paths to file contents.
func buildImportMappingFromFiles(files []string) (map[string][]byte, []string, error) {
	mapping := make(map[string][]byte)
	var importPaths []string
	pkgRE := regexp.MustCompile(`(?m)^package\s+([\w.]+)\s*;`)

	for _, filePath := range files {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, nil, err
		}

		m := pkgRE.FindSubmatch(content)
		if m == nil {
			return nil, nil, fmt.Errorf("no package found in %s", filePath)
		}

		pkg := string(m[1])
		base := filepath.Base(filePath)
		importPath := strings.ReplaceAll(pkg, ".", "/") + "/" + base
		mapping[importPath] = content
		importPaths = append(importPaths, importPath)
	}

	return mapping, importPaths, nil
}

// multiPathResolver resolves proto files from memory first, then from
// filesystem paths. This allows primary proto files to be compiled while
// their dependencies (e.g. gogoproto/gogo.proto) are resolved from disk.
type multiPathResolver struct {
	files map[string][]byte
	paths []string
}

func (r *multiPathResolver) FindFileByPath(path string) (protocompile.SearchResult, error) {
	// Check in-memory files first (primary protos).
	if content, ok := r.files[path]; ok {
		return protocompile.SearchResult{
			Source: bytes.NewReader(content),
		}, nil
	}

	// Search additional proto paths on disk.
	for _, dir := range r.paths {
		fullPath := filepath.Join(dir, path)
		content, err := os.ReadFile(fullPath)
		if err == nil {
			return protocompile.SearchResult{
				Source: bytes.NewReader(content),
			}, nil
		}
	}

	return protocompile.SearchResult{}, os.ErrNotExist
}
