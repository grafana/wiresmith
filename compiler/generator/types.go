package generator

import (
	"path"
	"strconv"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// importEntry holds everything we need to emit an import statement: the
// alias we chose locally and the package's natural name — the `package`
// clause declared by the imported file. Storing the natural name (instead
// of inferring it from path.Base, which lies whenever go_package uses the
// `;name` form) lets emitHeader decide elision on actual identity.
type importEntry struct {
	alias       string
	naturalName string
}

type ImportTracker struct {
	module     string
	selfPkg    string
	goPackages map[string]string // proto pkg -> raw go_package option value
	imports    map[string]importEntry
}

func newImportTracker(module, selfPkg string, goPackages map[string]string) *ImportTracker {
	return &ImportTracker{
		module:     module,
		selfPkg:    selfPkg,
		goPackages: goPackages,
		imports:    make(map[string]importEntry),
	}
}

// addImport registers a non-proto import path with the given alias. The
// natural name defaults to path.Base, which is correct for stdlib and for
// every external package wiresmith currently uses. Repeated calls with the
// same path keep the first registration — callers don't need to coordinate.
func (it *ImportTracker) addImport(importPath, alias string) string {
	return it.register(importPath, alias, path.Base(importPath))
}

func (it *ImportTracker) register(importPath, alias, naturalName string) string {
	if e, ok := it.imports[importPath]; ok {
		return e.alias
	}
	it.imports[importPath] = importEntry{alias: alias, naturalName: naturalName}
	return alias
}

// resolvePkgName returns the Go package name for protoPkg.
func (it *ImportTracker) resolvePkgName(protoPkg string) string {
	return destFor(it.module, protoPkg, it.goPackages).pkgName
}

func (it *ImportTracker) addProtoImport(protoPkg string) string {
	selfName := it.resolvePkgName(it.selfPkg)
	dest := destFor(it.module, protoPkg, it.goPackages)

	// Prefer the destination's declared pkgName as the local alias so the
	// generated code reads naturally. On collision (with our own pkg name
	// or with another import), fall back to the proto-package-derived
	// alias; uniqueAlias then appends a numeric suffix if even that
	// collides. Matches the protogen/gogoproto disambiguation scheme.
	alias := dest.pkgName
	if alias == selfName || it.aliasInUse(alias, dest.importPath) {
		alias = goPackageName(protoPkg)
	}
	alias = it.uniqueAlias(alias, dest.importPath, selfName)
	return it.register(dest.importPath, alias, dest.pkgName)
}

// uniqueAlias returns an alias guaranteed not to collide with selfName or
// with any other registered import's alias. If the desired alias is free
// it's returned as-is; otherwise a numeric suffix is appended until a
// non-colliding name is found.
func (it *ImportTracker) uniqueAlias(want, forPath, selfName string) string {
	candidate := want
	for i := 1; candidate == selfName || it.aliasInUse(candidate, forPath); i++ {
		candidate = want + strconv.Itoa(i)
	}
	return candidate
}

// aliasInUse reports whether some other registered import already uses alias.
// The forPath argument is the import path of the candidate that wants alias —
// it's excluded so repeated register calls for the same path don't self-
// report as a collision.
func (it *ImportTracker) aliasInUse(alias, forPath string) bool {
	// An empty alias is the "use the natural name" sentinel — treating it
	// as in-use would short-circuit later registrations.
	if alias == "" {
		return false
	}
	for p, e := range it.imports {
		if p != forPath && e.alias == alias {
			return true
		}
	}
	return false
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
