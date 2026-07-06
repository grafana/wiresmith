package generator

import (
	"google.golang.org/protobuf/reflect/protoreflect"
)

// The no_registration option sits outside the FieldOption registry for the
// same reason as no_presence_all / enum_no_prefix_all: it annotates the file,
// not a field. It is file-level only — registration is emitted per file (the
// whole file registers with the runtime as one unit via protoimpl.TypeBuilder),
// so there is no per-message counterpart and no *_all layering.
// hasNoRegistration is the single consumer, called from emitRegistration.
const noRegistrationExtName = "wiresmith.options.no_registration"

// hasNoRegistration reports whether fd opts out of OFFICIAL global-registry
// registration. When true, emitRegistration targets a package-local
// protoregistry.Files / protoregistry.Types instead of protoregistry.GlobalFiles
// / GlobalTypes, so the generated file mutates no global registry state and can
// coexist with another module that registers the same proto package globally
// (e.g. github.com/prometheus/client_model for io.prometheus.client). The
// reflection machinery is otherwise emitted unchanged, so the types remain
// valid proto.Messages backed by the file's local descriptor.
func (fg *FileGenerator) hasNoRegistration(fd protoreflect.FileDescriptor) bool {
	v, _ := readBoolExt(fg.noRegistrationExt, fd.Options())
	return v
}
