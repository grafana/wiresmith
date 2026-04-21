package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitRegistration(fd protoreflect.FileDescriptor) {
	fg.body.WriteString("func init() {\n")

	// File-level enums.
	for i := 0; i < fd.Enums().Len(); i++ {
		fg.emitEnumRegistration(fd.Enums().Get(i))
	}

	// Per-message enums and message registration.
	forEachMessage(fd, func(md protoreflect.MessageDescriptor) {
		for i := 0; i < md.Enums().Len(); i++ {
			fg.emitEnumRegistration(md.Enums().Get(i))
		}
		typeName := goMessageTypeName(md)
		fmt.Fprintf(fg.body, "\tprotohelpers.RegisterType((*%s)(nil), %q)\n",
			typeName, md.FullName())
	})

	fg.body.WriteString("}\n")
	fg.imports.addImport(fg.module+"/gen/protohelpers", "")
}

func (fg *FileGenerator) emitEnumRegistration(ed protoreflect.EnumDescriptor) {
	typeName := goEnumTypeName(ed)
	fmt.Fprintf(fg.body, "\tprotohelpers.RegisterEnum(%q, %s_name, %s_value)\n",
		ed.FullName(), typeName, typeName)
}
