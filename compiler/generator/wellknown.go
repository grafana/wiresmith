package generator

import (
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Well-known-type substitution.
//
// wiresmith generates its own struct + Size/Marshal/Unmarshal/Equal/Compare
// for nested message fields and calls those wiresmith methods on the field's
// Go type. The official google.golang.org/protobuf/types/known/* types
// (anypb.Any, …) don't have those methods, so a field referencing a
// well-known type can't resolve to the official package. Instead wiresmith
// ships a drop-in replacement package and resolves every reference to the WKT
// there.
//
// The replacement package does NOT register its descriptor with the global
// proto registry: the official runtime — linked into essentially every binary
// — already registers `google/protobuf/any.proto` and the
// `google.protobuf.Any` message, and a second registration of the same file /
// message panics at init. The shipped package therefore omits the generated
// `_util.pb.go` and hand-writes a `ProtoReflect()` that delegates to the
// already-registered official descriptor (see types/known/anypb/helpers.go).

// anyProtoImportPath is the canonical import path of Google's well-known Any
// message, as written in a consumer `import "..."` and as produced by
// protocompile.WithStandardImports.
const anyProtoImportPath = "google/protobuf/any.proto"

// wktDestByImportPath maps a well-known-type import path to the
// wiresmith-shipped Go package that replaces it, in go_package `path;name`
// form. References to the WKT resolve here instead of the official
// google.golang.org/protobuf/types/known/* package. A user `-M` override for
// the same path still wins (it is checked first in destForPath).
var wktDestByImportPath = map[string]string{
	anyProtoImportPath: "github.com/grafana/wiresmith/types/known/anypb;anypb",
}

// wktNoReflectSourcePaths are the proto source paths of wiresmith's own WKT
// replacement packages. A file at one of these paths skips reflect /
// registration emission: it (re)declares a `google.protobuf.*` message whose
// descriptor the official runtime already registers, so emitting a second
// registration would panic at init. Each shipped package hand-writes its
// ProtoReflect (delegating to the official descriptor) and any typed helpers.
var wktNoReflectSourcePaths = map[string]bool{
	"types/known/anypb/any.proto": true,
}

// wktDest returns the wiresmith replacement go_package for a WKT import path,
// or ("", false) when the path is not a substituted well-known type.
func wktDest(importPath string) (string, bool) {
	dest, ok := wktDestByImportPath[importPath]
	return dest, ok
}

// skipReflectEmission reports whether fd is one of wiresmith's WKT replacement
// sources, whose reflect / registration glue must not be emitted.
func skipReflectEmission(fd protoreflect.FileDescriptor) bool {
	return wktNoReflectSourcePaths[fd.Path()]
}
