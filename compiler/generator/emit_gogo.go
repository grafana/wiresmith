package generator

import (
	"fmt"
	"path/filepath"
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

	// String returns a text representation matching gogoslick's format.
	fg.imports.addStdImport("fmt")
	fg.imports.addStdImport("strings")
	fmt.Fprintf(fg.body, "func (this *%s) String() string {\n", name)
	fmt.Fprintf(fg.body, "\tif this == nil {\n\t\treturn \"nil\"\n\t}\n")
	fmt.Fprintf(fg.body, "\ts := strings.Join([]string{`&%s{`,\n", name)
	seenOneofs := map[string]bool{}
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if oo := fd.ContainingOneof(); oo != nil && !oo.IsSynthetic() {
			ooName := string(oo.Name())
			if !seenOneofs[ooName] {
				seenOneofs[ooName] = true
				goName := snakeToPascal(ooName)
				fmt.Fprintf(fg.body, "\t\t`%s:` + fmt.Sprintf(\"%%v\", this.%s) + `,`,\n", goName, goName)
			}
			continue
		}
		goName := snakeToPascal(string(fd.Name()))
		fmt.Fprintf(fg.body, "\t\t`%s:` + fmt.Sprintf(\"%%v\", this.%s) + `,`,\n", goName, goName)
	}
	fmt.Fprintf(fg.body, "\t\t`}`,\n")
	fmt.Fprintf(fg.body, "\t}, \"\")\n")
	fmt.Fprintf(fg.body, "\treturn s\n")
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

	// Generate gogoslick-compatible helper functions.
	// Hand-written code may reference these.
	if fd.Messages().Len() > 0 {
		suffix := snakeToPascal(strings.TrimSuffix(filepath.Base(fd.Path()), ".proto"))
		fg.imports.addStdImport("math/bits")
		fg.imports.addStdImport("fmt")

		// Varint size helpers
		fmt.Fprintf(fg.body, "func sov%s(x uint64) (n int) {\n", suffix)
		fmt.Fprintf(fg.body, "\treturn (bits.Len64(x|1) + 6) / 7\n")
		fmt.Fprintf(fg.body, "}\n\n")
		fmt.Fprintf(fg.body, "func soz%s(x uint64) (n int) {\n", suffix)
		fmt.Fprintf(fg.body, "\treturn sov%s(uint64((x << 1) ^ uint64((int64(x) >> 63))))\n", suffix)
		fmt.Fprintf(fg.body, "}\n\n")

		// Varint encode helper (reverse-write)
		fmt.Fprintf(fg.body, "func encodeVarint%s(dAtA []byte, offset int, v uint64) int {\n", suffix)
		fmt.Fprintf(fg.body, "\toffset -= sov%s(v)\n", suffix)
		fmt.Fprintf(fg.body, "\tbase := offset\n")
		fmt.Fprintf(fg.body, "\tfor v >= 1<<7 {\n")
		fmt.Fprintf(fg.body, "\t\tdAtA[offset] = uint8(v&0x7f | 0x80)\n")
		fmt.Fprintf(fg.body, "\t\tv >>= 7\n")
		fmt.Fprintf(fg.body, "\t\toffset++\n")
		fmt.Fprintf(fg.body, "\t}\n")
		fmt.Fprintf(fg.body, "\tdAtA[offset] = uint8(v)\n")
		fmt.Fprintf(fg.body, "\treturn base\n")
		fmt.Fprintf(fg.body, "}\n\n")

		// Skip function alias matching gogoslick convention
		fmt.Fprintf(fg.body, "func skip%s(dAtA []byte) (n int, err error) {\n", suffix)
		fmt.Fprintf(fg.body, "\tl := len(dAtA)\n")
		fmt.Fprintf(fg.body, "\tiNdEx := 0\n")
		fmt.Fprintf(fg.body, "\tfor iNdEx < l {\n")
		fmt.Fprintf(fg.body, "\t\tvar wire uint64\n")
		fmt.Fprintf(fg.body, "\t\tfor shift := uint(0); ; shift += 7 {\n")
		fmt.Fprintf(fg.body, "\t\t\tif iNdEx >= l { return 0, fmt.Errorf(\"proto: unexpected EOF\") }\n")
		fmt.Fprintf(fg.body, "\t\t\tb := dAtA[iNdEx]; iNdEx++\n")
		fmt.Fprintf(fg.body, "\t\t\twire |= uint64(b&0x7F) << shift\n")
		fmt.Fprintf(fg.body, "\t\t\tif b < 0x80 { break }\n")
		fmt.Fprintf(fg.body, "\t\t}\n")
		fmt.Fprintf(fg.body, "\t\twireType := int(wire & 0x7)\n")
		fmt.Fprintf(fg.body, "\t\tswitch wireType {\n")
		fmt.Fprintf(fg.body, "\t\tcase 0:\n")
		fmt.Fprintf(fg.body, "\t\t\tfor shift := uint(0); ; shift += 7 {\n")
		fmt.Fprintf(fg.body, "\t\t\t\tif iNdEx >= l { return 0, fmt.Errorf(\"proto: unexpected EOF\") }\n")
		fmt.Fprintf(fg.body, "\t\t\t\tiNdEx++\n")
		fmt.Fprintf(fg.body, "\t\t\t\tif dAtA[iNdEx-1] < 0x80 { break }\n")
		fmt.Fprintf(fg.body, "\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\tcase 1: iNdEx += 8\n")
		fmt.Fprintf(fg.body, "\t\tcase 2:\n")
		fmt.Fprintf(fg.body, "\t\t\tvar length int\n")
		fmt.Fprintf(fg.body, "\t\t\tfor shift := uint(0); ; shift += 7 {\n")
		fmt.Fprintf(fg.body, "\t\t\t\tif iNdEx >= l { return 0, fmt.Errorf(\"proto: unexpected EOF\") }\n")
		fmt.Fprintf(fg.body, "\t\t\t\tb := dAtA[iNdEx]; iNdEx++\n")
		fmt.Fprintf(fg.body, "\t\t\t\tlength |= (int(b) & 0x7F) << shift\n")
		fmt.Fprintf(fg.body, "\t\t\t\tif b < 0x80 { break }\n")
		fmt.Fprintf(fg.body, "\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\tif length < 0 { return 0, ErrInvalidLength%s }\n", suffix)
		fmt.Fprintf(fg.body, "\t\t\tiNdEx += length\n")
		fmt.Fprintf(fg.body, "\t\tcase 3:\n")
		fmt.Fprintf(fg.body, "\t\t\tfor { var innerWire uint64\n")
		fmt.Fprintf(fg.body, "\t\t\t\tfor shift := uint(0); ; shift += 7 {\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif iNdEx >= l { return 0, fmt.Errorf(\"proto: unexpected EOF\") }\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tb := dAtA[iNdEx]; iNdEx++\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tinnerWire |= uint64(b&0x7F) << shift\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif b < 0x80 { break }\n")
		fmt.Fprintf(fg.body, "\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\tif int(innerWire&0x7) == 4 { break }\n")
		fmt.Fprintf(fg.body, "\t\t\t\tnext, err := skip%s(dAtA[iNdEx:])\n", suffix)
		fmt.Fprintf(fg.body, "\t\t\t\tif err != nil { return 0, err }\n")
		fmt.Fprintf(fg.body, "\t\t\t\tiNdEx += next\n")
		fmt.Fprintf(fg.body, "\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\tcase 5: iNdEx += 4\n")
		fmt.Fprintf(fg.body, "\t\tdefault: return 0, fmt.Errorf(\"proto: illegal wireType %%d\", wireType)\n")
		fmt.Fprintf(fg.body, "\t\t}\n")
		fmt.Fprintf(fg.body, "\t\tif iNdEx < 0 { return 0, ErrInvalidLength%s }\n", suffix)
		fmt.Fprintf(fg.body, "\t\treturn iNdEx, nil\n")
		fmt.Fprintf(fg.body, "\t}\n")
		fmt.Fprintf(fg.body, "\treturn 0, fmt.Errorf(\"proto: unexpected EOF\")\n")
		fmt.Fprintf(fg.body, "}\n\n")

		// Error variables
		fmt.Fprintf(fg.body, "var (\n")
		fmt.Fprintf(fg.body, "\tErrInvalidLength%s = fmt.Errorf(\"proto: negative length found during unmarshaling\")\n", suffix)
		fmt.Fprintf(fg.body, "\tErrIntOverflow%s   = fmt.Errorf(\"proto: integer overflow\")\n", suffix)
		fmt.Fprintf(fg.body, "\tErrUnexpectedEndOfGroup%s = fmt.Errorf(\"proto: unexpected end of group\")\n", suffix)
		fmt.Fprintf(fg.body, ")\n\n")
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
