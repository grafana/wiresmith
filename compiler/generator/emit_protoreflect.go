package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitAllProtoReflectMethods(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, fg.emitProtoReflect)
}

func (fg *FileGenerator) emitProtoReflect(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	idx := fg.nextMsgIndex
	fg.nextMsgIndex++

	fmt.Fprintf(fg.body, "func (x *%s) ProtoReflect() protoreflect.Message {\n", name)
	fmt.Fprintf(fg.body, "\t%s_init()\n", fg.fileVarName)
	fmt.Fprintf(fg.body, "\treturn protohelpers.NewMessageReflect(&%s_msgTypes[%d], x)\n",
		fg.fileVarName, idx)
	fmt.Fprintf(fg.body, "}\n\n")
	fg.imports.addImport("google.golang.org/protobuf/reflect/protoreflect", "")
	fg.imports.addImport(fg.module+"/gen/protohelpers", "")
}
