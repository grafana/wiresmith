package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// emitAllProtoReflectMethods writes a `ProtoReflect() protoreflect.Message`
// method for every (non-map-entry) message into the companion _util.pb.go
// file.
//
// These methods are the bridge between wiresmith's value-type-fields shape
// and google.golang.org/protobuf's reflection API. They are never called on
// the marshal/unmarshal hot path (wiresmith's hot path goes through the
// generated UnmarshalToSizedBuffer / MarshalToSizedBuffer methods directly) —
// only by callers that explicitly use proto.Marshal / proto.Unmarshal /
// protoregistry lookups.
//
// Iteration uses `flattenedMessages` (the TypeBuilder layered order) and
// includes map-entry messages in the index space — we skip emitting a method
// for map entries (they have no Go type) but still increment `nextMsgIndex`,
// so positions stay aligned with the `file_*_msgTypes` slots that
// `TypeBuilder.Build` populates.
func (fg *FileGenerator) emitAllProtoReflectMethods(fd protoreflect.FileDescriptor) {
	for _, md := range flattenedMessages(fd) {
		if md.IsMapEntry() {
			// No Go type → no method, but the msgTypes slot DOES exist
			// (Build() walks the raw FileDescriptorProto and allocates a
			// slot per nested message regardless of map-entry status).
			fg.nextMsgIndex++
			continue
		}
		fg.emitProtoReflect(md)
	}
}

func (fg *FileGenerator) emitProtoReflect(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	idx := fg.nextMsgIndex
	fg.nextMsgIndex++

	fmt.Fprintf(fg.utilBody, "func (x *%s) ProtoReflect() protoreflect.Message {\n", name)
	fmt.Fprintf(fg.utilBody, "\t%s_init()\n", fg.fileVarName)
	fmt.Fprintf(fg.utilBody, "\treturn protohelpers.NewMessageReflect(&%s_msgTypes[%d], x)\n",
		fg.fileVarName, idx)
	fmt.Fprintf(fg.utilBody, "}\n\n")
	fg.utilImports.addImport("google.golang.org/protobuf/reflect/protoreflect", "")
	fg.utilImports.addImport(protohelpersImport, "")
}
