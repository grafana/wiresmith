package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitAllResetMethods(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, fg.emitReset)
}

func (fg *FileGenerator) emitReset(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	fmt.Fprintf(fg.body, "func (m *%s) Reset() {\n", name)
	fmt.Fprintf(fg.body, "\tif m == nil {\n\t\treturn\n\t}\n")
	fmt.Fprintf(fg.body, "\t*m = %s{}\n}\n", name)
	fmt.Fprintf(fg.body, "func (*%s) ProtoMessage() {}\n", name)
	fmt.Fprintf(fg.body, "func (m *%s) String() string {\n", name)
	fmt.Fprintf(fg.body, "\tif m == nil {\n\t\treturn \"<nil>\"\n\t}\n")
	fmt.Fprintf(fg.body, "\treturn fmt.Sprintf(\"%%v\", *m)\n}\n\n")
	fg.imports.addImport("fmt", "")
}
