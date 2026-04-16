package generator

import (
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
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
		elemType := it.goSingularType(fd)
		// In gogo compat mode, repeated message fields default to pointer slices
		// (matching gogoslick behavior). Use value types only when
		// (gogoproto.nullable) = false is set.
		if it.gen != nil && it.gen.GogoCompat && fd.Kind() == protoreflect.MessageKind && isFieldNullable(fd) {
			return "[]*" + elemType
		}
		return "[]" + elemType
	}
	if fd.HasOptionalKeyword() {
		return it.goOptionalType(fd)
	}
	// In gogo compat mode, singular message fields default to pointers
	// (matching gogoslick behavior). Use value types only when
	// (gogoproto.nullable) = false is set.
	if it.gen != nil && it.gen.GogoCompat && fd.Kind() == protoreflect.MessageKind && isFieldNullable(fd) {
		return "*" + it.goSingularType(fd)
	}
	return it.goSingularType(fd)
}

// isGogoPointerField returns true if the field should be a pointer in gogo compat mode.
func isGogoPointerField(gen *Generator, fd protoreflect.FieldDescriptor) bool {
	return gen != nil && gen.GogoCompat && fd.Kind() == protoreflect.MessageKind && isFieldNullable(fd)
}

// isFieldNullable checks the gogoproto.nullable field option (extension 65001).
// Returns true (the gogoproto default) if the option is not set or set to true.
// Returns false only when explicitly set to (gogoproto.nullable) = false.
func isFieldNullable(fd protoreflect.FieldDescriptor) bool {
	opts, ok := fd.Options().(*descriptorpb.FieldOptions)
	if !ok || opts == nil {
		return true
	}
	// gogoproto.nullable is extension field 65001 on FieldOptions (varint type).
	// Serialize the options and scan for the field tag.
	b, err := proto.Marshal(opts)
	if err != nil {
		return true
	}
	return !containsVarintField(b, 65001, 0)
}

// containsVarintField scans serialized proto bytes for a varint field with the
// given number and value.
func containsVarintField(b []byte, fieldNum protoreflect.FieldNumber, val uint64) bool {
	for len(b) > 0 {
		num, typ, tagLen := protowire.ConsumeTag(b)
		if tagLen < 0 {
			return false
		}
		b = b[tagLen:]
		switch typ {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return false
			}
			if num == protowire.Number(fieldNum) && v == val {
				return true
			}
			b = b[n:]
		case protowire.Fixed32Type:
			b = b[4:]
		case protowire.Fixed64Type:
			b = b[8:]
		case protowire.BytesType:
			_, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return false
			}
			b = b[n:]
		default:
			return false
		}
	}
	return false
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
