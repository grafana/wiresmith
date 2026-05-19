package generator

import (
	"google.golang.org/protobuf/reflect/protoreflect"
)

type ImportTracker struct {
	module     string
	selfPkg    string
	goPackages map[string]string // proto pkg -> raw go_package option value
	imports    map[string]string // import path -> alias
}

func newImportTracker(module, selfPkg string, goPackages map[string]string) *ImportTracker {
	return &ImportTracker{
		module:     module,
		selfPkg:    selfPkg,
		goPackages: goPackages,
		imports:    make(map[string]string),
	}
}

// addImport registers an import path with the given alias. Repeated calls
// with the same path keep the first alias — callers don't need to coordinate
// the order of helper-emitter calls.
func (it *ImportTracker) addImport(importPath, alias string) string {
	if existing, ok := it.imports[importPath]; ok {
		return existing
	}
	it.imports[importPath] = alias
	return alias
}

// resolvePkgName returns the Go package name for protoPkg, preferring the
// go_package option when it falls under our import base and falling back to
// the default scheme otherwise.
func (it *ImportTracker) resolvePkgName(protoPkg string) string {
	if _, _, pkgName, ok := resolveGoPackage(protoPkg, it.goPackages, effectiveBase(it.module)); ok {
		return pkgName
	}
	return goPackageName(protoPkg)
}

func (it *ImportTracker) addProtoImport(protoPkg string) string {
	if importPath, _, pkgName, ok := resolveGoPackage(protoPkg, it.goPackages, effectiveBase(it.module)); ok {
		return it.addImport(importPath, pkgName)
	}
	return it.addImport(goImportPath(it.module, protoPkg), goPackageName(protoPkg))
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
	case protoreflect.EnumKind:
		return "*" + it.goEnumType(fd.Enum())
	case protoreflect.MessageKind:
		return "*" + it.goMessageType(fd.Message())
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
