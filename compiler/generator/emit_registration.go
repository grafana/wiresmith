package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitRegistration(fd protoreflect.FileDescriptor) {
	fg.body.WriteString("func init() {\n")
	fg.emitEnumRegistrations(fd)
	fg.emitMessageRegistrations(fd)
	fg.body.WriteString("}\n")

	fg.imports.addImport(fg.module+"/gen/protohelpers", "")
}

func (fg *FileGenerator) emitEnumRegistrations(parent protoreflect.Descriptor) {
	var enums protoreflect.EnumDescriptors
	switch p := parent.(type) {
	case protoreflect.FileDescriptor:
		enums = p.Enums()
	case protoreflect.MessageDescriptor:
		enums = p.Enums()
	}
	for i := 0; i < enums.Len(); i++ {
		ed := enums.Get(i)
		typeName := goEnumTypeName(ed)
		fmt.Fprintf(fg.body, "\tprotohelpers.RegisterEnum(%q, %s_name, %s_value)\n",
			ed.FullName(), typeName, typeName)
	}

	// Recurse into nested messages
	var msgs protoreflect.MessageDescriptors
	switch p := parent.(type) {
	case protoreflect.FileDescriptor:
		msgs = p.Messages()
	case protoreflect.MessageDescriptor:
		msgs = p.Messages()
	}
	for i := 0; i < msgs.Len(); i++ {
		md := msgs.Get(i)
		if md.IsMapEntry() {
			continue
		}
		fg.emitEnumRegistrations(md)
	}
}

func (fg *FileGenerator) emitMessageRegistrations(parent protoreflect.Descriptor) {
	var msgs protoreflect.MessageDescriptors
	switch p := parent.(type) {
	case protoreflect.FileDescriptor:
		msgs = p.Messages()
	case protoreflect.MessageDescriptor:
		msgs = p.Messages()
	}
	for i := 0; i < msgs.Len(); i++ {
		md := msgs.Get(i)
		if md.IsMapEntry() {
			continue
		}
		typeName := goMessageTypeName(md)
		fmt.Fprintf(fg.body, "\tprotohelpers.RegisterType((*%s)(nil), %q)\n",
			typeName, md.FullName())
		fg.emitMessageRegistrations(md)
	}
}
