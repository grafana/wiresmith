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
//
// requested distinguishes entries the emit_*.go code actually asked for
// from entries pre-reserved by newImportTracker. emitHeader only emits
// requested entries; aliasInUse considers all of them, so addProtoImport
// can't hand out an alias that a known stdlib import will later claim.
type importEntry struct {
	alias       string
	naturalName string
	requested   bool
}

// reservedStdlibImports lists every import path the generated code might
// pull in via the emit_*.go helpers. Pre-registering them keeps their
// natural names out of the alias pool, so a proto with go_package like
// `;fmt` can't pick "fmt" and then collide with the stdlib fmt at emit
// time. Keep this in sync with the addImport calls in compiler/generator
// and compiler/types — a name we forget here is a name a malicious or
// careless go_package can shadow.
var reservedStdlibImports = []string{
	"bytes",
	"encoding/binary",
	"fmt",
	"io",
	"math",
	"reflect",
	"strconv",
	"unsafe",
	"google.golang.org/protobuf/encoding/protowire",
}

type ImportTracker struct {
	module  string
	selfPkg string
	dests   map[string]goDest // proto pkg -> resolved Go destination
	imports map[string]importEntry
}

func newImportTracker(module, selfPkg string, dests map[string]goDest) *ImportTracker {
	it := &ImportTracker{
		module:  module,
		selfPkg: selfPkg,
		dests:   dests,
		imports: make(map[string]importEntry),
	}
	for _, p := range reservedStdlibImports {
		it.imports[p] = importEntry{naturalName: path.Base(p)}
	}
	// protohelpers is generated under the module, so we can't list it as
	// a static path. Reserve it dynamically.
	helpers := module + "/gen/protohelpers"
	it.imports[helpers] = importEntry{naturalName: path.Base(helpers)}
	return it
}

// addImport registers a non-proto import path with the given alias. The
// natural name defaults to path.Base, which is correct for stdlib and for
// every external package wiresmith currently uses. Repeated calls with the
// same path keep the first registration — callers don't need to coordinate.
func (it *ImportTracker) addImport(importPath, alias string) string {
	return it.register(importPath, alias, path.Base(importPath))
}

func (it *ImportTracker) register(importPath, alias, naturalName string) string {
	if e, ok := it.imports[importPath]; ok && e.requested {
		return e.alias
	}
	it.imports[importPath] = importEntry{alias: alias, naturalName: naturalName, requested: true}
	return alias
}

// resolvePkgName returns the Go package name for protoPkg.
func (it *ImportTracker) resolvePkgName(protoPkg string) string {
	return it.dests[protoPkg].pkgName
}

func (it *ImportTracker) addProtoImport(protoPkg string) string {
	selfName := it.resolvePkgName(it.selfPkg)
	dest := it.dests[protoPkg]

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

// aliasInUse reports whether some other registered import already occupies
// the identifier `alias` in the current file's scope. An unaliased import
// occupies its naturalName — that's the package's declared name, which is
// what Go binds to the unaliased path. Comparing against effectiveName
// covers both explicit-alias and unaliased imports, so a proto whose
// pkgName is "fmt" can't quietly shadow the stdlib fmt import.
func (it *ImportTracker) aliasInUse(alias, forPath string) bool {
	if alias == "" {
		return false
	}
	for p, e := range it.imports {
		if p == forPath {
			continue
		}
		if e.effectiveName() == alias {
			return true
		}
	}
	return false
}

// effectiveName returns the identifier this import occupies in the current
// file's scope: the explicit alias when set, otherwise the imported file's
// declared package name.
func (e importEntry) effectiveName() string {
	if e.alias != "" {
		return e.alias
	}
	return e.naturalName
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
