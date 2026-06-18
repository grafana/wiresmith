package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitAllResetMethods(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, fg.emitReset)
}

// emitReset emits Reset() and the ProtoMessage() marker into the hot main
// .pb.go. String() is NOT emitted here — it moved to the cold companion
// `<name>_util.pb.go` (see emit_string.go), so the deterministic per-field
// formatter does not bloat the hot path.
func (fg *FileGenerator) emitReset(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	fmt.Fprintf(fg.body, "func (m *%s) Reset() {\n", name)
	fmt.Fprintf(fg.body, "\tif m == nil {\n\t\treturn\n\t}\n")
	fmt.Fprintf(fg.body, "\t*m = %s{}\n}\n", name)
	fmt.Fprintf(fg.body, "func (*%s) ProtoMessage() {}\n\n", name)
}
