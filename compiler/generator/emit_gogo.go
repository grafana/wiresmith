package generator

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// emitAllGogoMethods generates gogo protobuf compatibility methods for all messages.
// This includes Reset(), ProtoMessage(), String(), Descriptor(), and XXX_* methods.
func (fg *FileGenerator) emitAllGogoMethods(fd protoreflect.FileDescriptor) {
	for i := 0; i < fd.Messages().Len(); i++ {
		fg.emitGogoMethods(fd.Messages().Get(i))
	}
}

func (fg *FileGenerator) emitGogoMethods(md protoreflect.MessageDescriptor) {
	for i := 0; i < md.Messages().Len(); i++ {
		fg.emitGogoMethods(md.Messages().Get(i))
	}

	name := goMessageTypeName(md)

	// Reset zeroes the struct.
	fmt.Fprintf(fg.body, "func (m *%s) Reset() { *m = %s{} }\n", name, name)

	// ProtoMessage is a marker method.
	fmt.Fprintf(fg.body, "func (*%s) ProtoMessage() {}\n", name)

	// String returns a text representation.
	fg.imports.addStdImport("fmt")
	fmt.Fprintf(fg.body, "func (m *%s) String() string {\n", name)
	fmt.Fprintf(fg.body, "\tif m == nil {\n\t\treturn \"nil\"\n\t}\n")
	fmt.Fprintf(fg.body, "\treturn fmt.Sprintf(\"%%v\", *m)\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// XXX_Unmarshal delegates to Unmarshal.
	fmt.Fprintf(fg.body, "func (m *%s) XXX_Unmarshal(b []byte) error {\n", name)
	fmt.Fprintf(fg.body, "\treturn m.Unmarshal(b)\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// XXX_Marshal delegates to MarshalToSizedBuffer.
	fg.imports.addImport("github.com/gogo/protobuf/proto", "proto")
	fmt.Fprintf(fg.body, "func (m *%s) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {\n", name)
	fmt.Fprintf(fg.body, "\tif deterministic {\n")
	fmt.Fprintf(fg.body, "\t\treturn xxx_messageInfo_%s.Marshal(b, m, deterministic)\n", name)
	fmt.Fprintf(fg.body, "\t} else {\n")
	fmt.Fprintf(fg.body, "\t\tb = b[:cap(b)]\n")
	fmt.Fprintf(fg.body, "\t\tn, err := m.MarshalToSizedBuffer(b)\n")
	fmt.Fprintf(fg.body, "\t\tif err != nil {\n\t\t\treturn nil, err\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t\treturn b[:n], nil\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// XXX_Merge delegates to InternalMessageInfo.
	fmt.Fprintf(fg.body, "func (m *%s) XXX_Merge(src proto.Message) {\n", name)
	fmt.Fprintf(fg.body, "\txxx_messageInfo_%s.Merge(m, src)\n", name)
	fmt.Fprintf(fg.body, "}\n\n")

	// XXX_Size delegates to Size.
	fmt.Fprintf(fg.body, "func (m *%s) XXX_Size() int {\n", name)
	fmt.Fprintf(fg.body, "\treturn m.Size()\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// XXX_DiscardUnknown delegates to InternalMessageInfo.
	fmt.Fprintf(fg.body, "func (m *%s) XXX_DiscardUnknown() {\n", name)
	fmt.Fprintf(fg.body, "\txxx_messageInfo_%s.DiscardUnknown(m)\n", name)
	fmt.Fprintf(fg.body, "}\n\n")

	// InternalMessageInfo variable.
	fmt.Fprintf(fg.body, "var xxx_messageInfo_%s proto.InternalMessageInfo\n\n", name)
}

// emitRegistration generates init() functions for proto.RegisterType and proto.RegisterFile.
func (fg *FileGenerator) emitRegistration(fd protoreflect.FileDescriptor) {
	hasRegistrations := fd.Messages().Len() > 0 || fd.Enums().Len() > 0
	if hasRegistrations {
		fg.imports.addImport("github.com/gogo/protobuf/proto", "proto")
	}

	// RegisterType for each message.
	for i := 0; i < fd.Messages().Len(); i++ {
		fg.emitRegisterType(fd.Messages().Get(i), fd)
	}

	// RegisterEnum for each enum.
	for i := 0; i < fd.Enums().Len(); i++ {
		ed := fd.Enums().Get(i)
		fg.emitRegisterEnum(ed)
	}
	for i := 0; i < fd.Messages().Len(); i++ {
		fg.emitNestedRegisterEnums(fd.Messages().Get(i))
	}
}

func (fg *FileGenerator) emitRegisterType(md protoreflect.MessageDescriptor, fd protoreflect.FileDescriptor) {
	name := goMessageTypeName(md)
	fullName := string(md.FullName())

	fmt.Fprintf(fg.body, "func init() {\n")
	fmt.Fprintf(fg.body, "\tproto.RegisterType((*%s)(nil), %q)\n", name, fullName)
	fmt.Fprintf(fg.body, "}\n\n")

	// Recurse for nested messages.
	for i := 0; i < md.Messages().Len(); i++ {
		fg.emitRegisterType(md.Messages().Get(i), fd)
	}
}

func (fg *FileGenerator) emitRegisterEnum(ed protoreflect.EnumDescriptor) {
	typeName := goEnumTypeName(ed)
	fullName := string(ed.FullName())

	// Build name/value maps.
	fmt.Fprintf(fg.body, "var %s_name = map[int32]string{\n", typeName)
	for i := 0; i < ed.Values().Len(); i++ {
		v := ed.Values().Get(i)
		fmt.Fprintf(fg.body, "\t%d: %q,\n", v.Number(), string(v.Name()))
	}
	fmt.Fprintf(fg.body, "}\n\n")

	fmt.Fprintf(fg.body, "var %s_value = map[string]int32{\n", typeName)
	for i := 0; i < ed.Values().Len(); i++ {
		v := ed.Values().Get(i)
		fmt.Fprintf(fg.body, "\t%q: %d,\n", string(v.Name()), v.Number())
	}
	fmt.Fprintf(fg.body, "}\n\n")

	fmt.Fprintf(fg.body, "func (%s) EnumDescriptor() ([]byte, []int) {\n", typeName)
	fmt.Fprintf(fg.body, "\treturn nil, nil\n")
	fmt.Fprintf(fg.body, "}\n\n")

	fmt.Fprintf(fg.body, "func (x %s) String() string {\n", typeName)
	fmt.Fprintf(fg.body, "\tif name, ok := %s_name[int32(x)]; ok {\n", typeName)
	fmt.Fprintf(fg.body, "\t\treturn name\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\treturn fmt.Sprintf(\"%%d\", int32(x))\n")
	fmt.Fprintf(fg.body, "}\n\n")

	fmt.Fprintf(fg.body, "func init() {\n")
	fmt.Fprintf(fg.body, "\tproto.RegisterEnum(%q, %s_name, %s_value)\n", fullName, typeName, typeName)
	fmt.Fprintf(fg.body, "}\n\n")
}

func (fg *FileGenerator) emitNestedRegisterEnums(md protoreflect.MessageDescriptor) {
	for i := 0; i < md.Enums().Len(); i++ {
		fg.emitRegisterEnum(md.Enums().Get(i))
	}
	for i := 0; i < md.Messages().Len(); i++ {
		fg.emitNestedRegisterEnums(md.Messages().Get(i))
	}
}

// emitAllStringMethods generates custom String() methods for messages in gogo compat mode.
func (fg *FileGenerator) emitAllStringMethods(fd protoreflect.FileDescriptor) {
	// String methods are already emitted in emitGogoMethods.
	// This is a no-op placeholder for the generator flow.
}

// emitAllGoStringMethods generates GoString() methods for all messages.
func (fg *FileGenerator) emitAllGoStringMethods(fd protoreflect.FileDescriptor) {
	for i := 0; i < fd.Messages().Len(); i++ {
		fg.emitGoStringMethods(fd.Messages().Get(i))
	}
}

func (fg *FileGenerator) emitGoStringMethods(md protoreflect.MessageDescriptor) {
	for i := 0; i < md.Messages().Len(); i++ {
		fg.emitGoStringMethods(md.Messages().Get(i))
	}
	fg.emitGoString(md)
}

func (fg *FileGenerator) emitGoString(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	pkgName := goFilePackage(fg.fd)
	fg.imports.addStdImport("fmt")
	fg.imports.addStdImport("strings")

	fmt.Fprintf(fg.body, "func (this *%s) GoString() string {\n", name)
	fmt.Fprintf(fg.body, "\tif this == nil {\n\t\treturn \"nil\"\n\t}\n")

	// Count fields for initial slice capacity.
	fieldCount := md.Fields().Len()
	fmt.Fprintf(fg.body, "\ts := make([]string, 0, %d)\n", fieldCount+4)
	fmt.Fprintf(fg.body, "\ts = append(s, \"&%s.%s{\")\n", pkgName, name)

	seenOneofs := map[string]bool{}
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)

		if oo := fd.ContainingOneof(); oo != nil && !oo.IsSynthetic() {
			ooName := string(oo.Name())
			if !seenOneofs[ooName] {
				seenOneofs[ooName] = true
				goName := snakeToPascal(ooName)
				fmt.Fprintf(fg.body, "\tif this.%s != nil {\n", goName)
				fmt.Fprintf(fg.body, "\t\ts = append(s, \"%s: \"+fmt.Sprintf(\"%%#v\", this.%s)+\",\\n\")\n", goName, goName)
				fmt.Fprintf(fg.body, "\t}\n")
			}
			continue
		}

		goName := snakeToPascal(string(fd.Name()))
		fmt.Fprintf(fg.body, "\ts = append(s, \"%s: \"+fmt.Sprintf(\"%%#v\", this.%s)+\",\\n\")\n", goName, goName)
	}

	fmt.Fprintf(fg.body, "\ts = append(s, \"}\")\n")
	fmt.Fprintf(fg.body, "\treturn strings.Join(s, \"\")\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

// protoFieldName returns the original proto field name.
func protoFieldName(fd protoreflect.FieldDescriptor) string {
	return string(fd.Name())
}

// protoFullName returns the full proto name for a message including package.
func protoFullName(md protoreflect.MessageDescriptor) string {
	parts := []string{string(md.Name())}
	parent := md.Parent()
	for {
		pm, ok := parent.(protoreflect.MessageDescriptor)
		if !ok {
			break
		}
		parts = append([]string{string(pm.Name())}, parts...)
		parent = pm.Parent()
	}
	return string(md.ParentFile().Package()) + "." + strings.Join(parts, ".")
}
