package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// emitAllCompatMethods emits XXX_ methods for gogoproto compatibility.
// These methods allow wiresmith types to work with gogoproto's proto.Clone,
// proto.Equal, proto.Merge, and jsonpb.
func (fg *FileGenerator) emitAllCompatMethods(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, fg.emitCompat)
}

func (fg *FileGenerator) emitCompat(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	fullName := string(md.FullName())

	// XXX_Unmarshal
	fmt.Fprintf(fg.body, "func (m *%s) XXX_Unmarshal(b []byte) error {\n", name)
	fmt.Fprintf(fg.body, "\treturn m.Unmarshal(b)\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// XXX_Marshal
	fmt.Fprintf(fg.body, "func (m *%s) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {\n", name)
	fmt.Fprintf(fg.body, "\tb = b[:cap(b)]\n")
	fmt.Fprintf(fg.body, "\tn, err := m.MarshalToSizedBuffer(b)\n")
	fmt.Fprintf(fg.body, "\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(fg.body, "\treturn b[:n], nil\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// XXX_Merge — deep copy via marshal/unmarshal
	fmt.Fprintf(fg.body, "func (m *%s) XXX_Merge(src proto.Message) {\n", name)
	fmt.Fprintf(fg.body, "\tif s, ok := src.(*%s); ok {\n", name)
	fmt.Fprintf(fg.body, "\t\tb, err := s.Marshal()\n")
	fmt.Fprintf(fg.body, "\t\tif err != nil {\n\t\t\treturn\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t_ = m.Unmarshal(b)\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "}\n\n")
	fg.imports.addImport("github.com/gogo/protobuf/proto", "proto")

	// XXX_Size
	fmt.Fprintf(fg.body, "func (m *%s) XXX_Size() int {\n", name)
	fmt.Fprintf(fg.body, "\treturn m.Size()\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// XXX_DiscardUnknown — no-op for wiresmith (unknown fields are always discarded)
	fmt.Fprintf(fg.body, "func (m *%s) XXX_DiscardUnknown() {}\n\n", name)

	// XXX_MessageName for gogoproto's MessageName()
	fmt.Fprintf(fg.body, "func (m *%s) XXX_MessageName() string {\n", name)
	fmt.Fprintf(fg.body, "\treturn %q\n", fullName)
	fmt.Fprintf(fg.body, "}\n\n")
}
