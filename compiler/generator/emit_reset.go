package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitAllResetMethods(fd protoreflect.FileDescriptor) {
	for i := 0; i < fd.Messages().Len(); i++ {
		fg.emitResetMethods(fd.Messages().Get(i))
	}
}

func (fg *FileGenerator) emitResetMethods(md protoreflect.MessageDescriptor) {
	for i := 0; i < md.Messages().Len(); i++ {
		nested := md.Messages().Get(i)
		if nested.IsMapEntry() {
			continue
		}
		fg.emitResetMethods(nested)
	}
	fg.emitReset(md)
}

func (fg *FileGenerator) emitReset(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	fmt.Fprintf(fg.body, "func (m *%s) Reset()      { *m = %s{} }\n", name, name)
	fmt.Fprintf(fg.body, "func (*%s) ProtoMessage() {}\n", name)
	fmt.Fprintf(fg.body, "func (m *%s) String() string { return fmt.Sprintf(\"%%v\", *m) }\n\n", name)
	fg.imports.addImport("fmt", "")
}
