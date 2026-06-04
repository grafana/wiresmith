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

// protohelpersImport is the canonical import path for wiresmith's runtime
// helpers. Generated code references it verbatim; the package lives at
// protohelpers/ in the wiresmith repo and is exported as part of the
// public Go module. Hardcoded here so the --module flag (which controls
// where generated proto packages land) cannot accidentally change the
// helpers import path for downstream consumers.
const protohelpersImport = "github.com/grafana/wiresmith/protohelpers"

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
	protohelpersImport,
}

type ImportTracker struct {
	module       string
	selfDest     goDest
	destinations map[string]goDest // fd.Path() -> resolved Go destination
	imports      map[string]importEntry
}

// newImportTracker constructs an ImportTracker for one output file.
// selfDest is the destination of the file being emitted — its pkgName is
// what the generated `package` clause should use and what we compare
// against when deciding whether a cross-file type reference needs an
// import qualifier. destinations is the project-wide fd.Path() →
// destination map; cross-file lookups (goMessageType / goEnumType) read
// from it directly, so the tracker shares the same map by reference for
// every output file in the generator run.
func newImportTracker(module string, selfDest goDest, destinations map[string]goDest) *ImportTracker {
	it := &ImportTracker{
		module:       module,
		selfDest:     selfDest,
		destinations: destinations,
		imports:      make(map[string]importEntry),
	}
	for _, p := range reservedStdlibImports {
		it.imports[p] = importEntry{naturalName: path.Base(p)}
	}
	return it
}

// addImport registers a non-proto import path with the given alias. The
// natural name defaults to path.Base, which is correct for stdlib and for
// every external package wiresmith currently uses. Repeated calls with the
// same path keep the first registration — callers don't need to coordinate.
func (it *ImportTracker) addImport(importPath, alias string) string {
	return it.register(importPath, alias, path.Base(importPath))
}

// addExplicitAliasImport registers importPath with a collision-free explicit
// alias derived from path.Base, and returns that alias for use as the Go
// qualifier in generated code. Unlike addImport, the alias is always spelled
// out in the emitted import block — for paths supplied by users (e.g.
// (wiresmith.options.customtype)) we can't trust that the upstream package's
// `package` declaration matches path.Base (think module major-version paths
// like `.../foo/v2`, or any package whose directory name differs from its
// declared name), so an unaliased import could bind a different identifier
// than the generated code references.
//
// The empty naturalName triggers emit_header's "always spell out the alias"
// branch (i.alias != i.natural), and effectiveName falls back to the alias
// for aliasInUse comparisons against later imports.
func (it *ImportTracker) addExplicitAliasImport(importPath string) string {
	if e, ok := it.imports[importPath]; ok && e.requested {
		return e.alias
	}
	alias := it.uniqueAlias(path.Base(importPath), importPath, it.selfDest.pkgName)
	it.imports[importPath] = importEntry{
		alias:     alias,
		requested: true,
	}
	return alias
}

func (it *ImportTracker) register(importPath, alias, naturalName string) string {
	if e, ok := it.imports[importPath]; ok && e.requested {
		return e.alias
	}
	it.imports[importPath] = importEntry{alias: alias, naturalName: naturalName, requested: true}
	return alias
}

// addProtoImport registers the import for a cross-package proto reference.
// fdPath is the importing file's Path() — it identifies the target's Go
// destination unambiguously, including the well-known case where one proto
// package (`google.protobuf`) spans multiple Go destinations (descriptorpb,
// timestamppb, durationpb …).
//
// Callers all route through goMessageType / goEnumType, which both supply
// the parent file's Path(). A miss in destinations means the file wasn't
// reached during computeDests' walk — a generator bug rather than a
// user-recoverable condition, so this returns the empty alias and lets
// the downstream "imported and not used" / "undefined identifier"
// compiler error surface where the bad reference is emitted.
func (it *ImportTracker) addProtoImport(fdPath string) string {
	dest := it.destinations[fdPath]

	// Prefer the destination's declared pkgName as the local alias so the
	// generated code reads naturally. On collision (with our own pkg name
	// or with another import), fall back to the proto-package-derived
	// alias; uniqueAlias then appends a numeric suffix if even that
	// collides. Matches the protogen/gogoproto disambiguation scheme.
	alias := dest.pkgName
	selfName := it.selfDest.pkgName
	if alias == selfName || it.aliasInUse(alias, dest.importPath) {
		alias = goPackageName(dest.protoPkg)
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
	parent := md.ParentFile()
	typeName := goMessageTypeName(md)
	if it.isSelfDest(parent.Path()) {
		return typeName
	}
	alias := it.addProtoImport(parent.Path())
	return alias + "." + typeName
}

func (it *ImportTracker) goEnumType(ed protoreflect.EnumDescriptor) string {
	parent := ed.ParentFile()
	typeName := goEnumTypeName(ed)
	if it.isSelfDest(parent.Path()) {
		return typeName
	}
	alias := it.addProtoImport(parent.Path())
	return alias + "." + typeName
}

// isSelfDest reports whether fdPath resolves to the same Go destination as
// the tracker's own. Two .proto files declaring the same proto package land
// in the same Go directory (enforced by validateDestinations) and therefore
// the same import path — references between them don't need a qualifier.
// Comparing destinations directly (rather than proto packages) keeps the
// answer correct even when one proto package spans multiple Go destinations
// (the well-known google.protobuf case) — references resolve to the type's
// actual destination, not to whichever destination won the proto-package
// tiebreaker.
func (it *ImportTracker) isSelfDest(fdPath string) bool {
	dest, ok := it.destinations[fdPath]
	if !ok {
		return false
	}
	return dest.importPath == it.selfDest.importPath
}
