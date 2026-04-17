package generator

import (
	"google.golang.org/protobuf/reflect/protoreflect"
)

type ImportTracker struct {
	module  string
	selfPkg string
	imports map[string]string // import path -> alias
}

func newImportTracker(module string, selfPkg string) *ImportTracker {
	return &ImportTracker{
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

func (it *ImportTracker) goType(fd protoreflect.FieldDescriptor) string {
	if fd.IsMap() {
		return it.goMapType(fd)
	}
	if fd.IsList() {
		return "[]" + it.goSingularType(fd)
	}
	if fd.HasOptionalKeyword() {
		return it.goOptionalType(fd)
	}
	return it.goSingularType(fd)
}

func (it *ImportTracker) goMapType(fd protoreflect.FieldDescriptor) string {
	keyFd := fd.MapKey()
	valFd := fd.MapValue()
	return "map[" + it.goSingularType(keyFd) + "]" + it.goSingularType(valFd)
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
	alias := it.addProtoImport(msgPkg)
	return alias + "." + typeName
}

func (it *ImportTracker) goEnumType(ed protoreflect.EnumDescriptor) string {
	enumPkg := string(ed.ParentFile().Package())
	typeName := goEnumTypeName(ed)
	if enumPkg == it.selfPkg {
		return typeName
	}
	alias := it.addProtoImport(enumPkg)
	return alias + "." + typeName
}
