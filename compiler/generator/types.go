package generator

import (
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

type ImportTracker struct {
	gen     *Generator
	module  string
	selfPkg string
	imports map[string]string // import path -> alias
}

func newImportTracker(gen *Generator, module string, selfPkg string) *ImportTracker {
	return &ImportTracker{
		gen:     gen,
		module:  module,
		selfPkg: selfPkg,
		imports: make(map[string]string),
	}
}

func (it *ImportTracker) addImport(importPath, alias string) string {
	it.imports[importPath] = alias
	return alias
}

func (it *ImportTracker) addProtoImport(protoPkg string) string {
	alias := goPackageName(protoPkg)
	importPath := goImportPath(it.module, protoPkg)
	return it.addImport(importPath, alias)
}

// addProtoFileImport adds an import for a dependency proto file, using its
// file descriptor to resolve the correct Go import path.
//
// In gogo compat mode, the Go import path is derived from the proto file's
// import path (e.g., "github.com/grafana/mimir/pkg/foo/bar/baz.proto"
// → "github.com/grafana/mimir/pkg/foo/bar"). This matches how protoc places
// generated files alongside the .proto sources.
//
// In default (OTel) mode, the import path is derived from the proto package
// name using the module-relative gen/ directory.
func (it *ImportTracker) addProtoFileImport(fd protoreflect.FileDescriptor) string {
	protoPkg := string(fd.Package())

	if it.gen != nil && it.gen.GogoCompat {
		// In gogo compat mode, derive Go import path from proto file path.
		// E.g., "github.com/grafana/mimir/pkg/planning/core/core.proto"
		// → import "github.com/grafana/mimir/pkg/planning/core"
		protoPath := fd.Path()
		importPath := filepath.Dir(protoPath)

		// Determine alias: use go_package if set, otherwise derive from package name.
		alias := goPackageName(protoPkg)
		if opts, ok := fd.Options().(*descriptorpb.FileOptions); ok && opts != nil && opts.GoPackage != nil {
			goPkg := opts.GetGoPackage()
			if !strings.Contains(goPkg, "/") {
				alias = goPkg
			} else if idx := strings.LastIndex(goPkg, "/"); idx >= 0 {
				alias = goPkg[idx+1:]
			}
		}

		return it.addImport(importPath, alias)
	}

	return it.addProtoImport(protoPkg)
}

func (it *ImportTracker) addStdImport(path string) {
	// For standard library imports, alias is empty
	it.imports[path] = ""
}

func (it *ImportTracker) goType(fd protoreflect.FieldDescriptor) string {
	if fd.IsList() {
		return "[]" + it.goSingularType(fd)
	}
	if fd.HasOptionalKeyword() {
		return it.goOptionalType(fd)
	}
	return it.goSingularType(fd)
}

func (it *ImportTracker) goSingularType(fd protoreflect.FieldDescriptor) string {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return "bool"
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return "int32"
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return "int64"
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return "uint32"
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return "uint64"
	case protoreflect.FloatKind:
		return "float32"
	case protoreflect.DoubleKind:
		return "float64"
	case protoreflect.StringKind:
		return "string"
	case protoreflect.BytesKind:
		return "[]byte"
	case protoreflect.MessageKind:
		return it.goMessageType(fd.Message())
	case protoreflect.EnumKind:
		return it.goEnumType(fd.Enum())
	default:
		return "interface{}"
	}
}

func (it *ImportTracker) goOptionalType(fd protoreflect.FieldDescriptor) string {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return "*bool"
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return "*int32"
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return "*int64"
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return "*uint32"
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return "*uint64"
	case protoreflect.FloatKind:
		return "*float32"
	case protoreflect.DoubleKind:
		return "*float64"
	case protoreflect.StringKind:
		return "*string"
	default:
		return it.goSingularType(fd)
	}
}

func (it *ImportTracker) goMessageType(md protoreflect.MessageDescriptor) string {
	msgPkg := string(md.ParentFile().Package())
	typeName := goMessageTypeName(md)
	if msgPkg == it.selfPkg {
		return typeName
	}
	alias := it.addProtoFileImport(md.ParentFile())
	return alias + "." + typeName
}

func (it *ImportTracker) goEnumType(ed protoreflect.EnumDescriptor) string {
	enumPkg := string(ed.ParentFile().Package())
	typeName := goEnumTypeName(ed)
	if enumPkg == it.selfPkg {
		return typeName
	}
	alias := it.addProtoFileImport(ed.ParentFile())
	return alias + "." + typeName
}
