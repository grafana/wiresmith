package generator

import (
	"bytes"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitRegistration(fd protoreflect.FileDescriptor) {
	rawDesc := serializeFileDescriptor(fd)
	prefix := fg.fileVarName

	// Emit raw descriptor const.
	fg.body.WriteString("const ")
	fg.body.WriteString(prefix)
	fg.body.WriteString("_rawDesc = \"\" +\n")
	encodeRawDescriptor(fg.body, rawDesc)
	fg.body.WriteString("\n\n")

	// Emit var block.
	fmt.Fprintf(fg.body, "var (\n")
	fmt.Fprintf(fg.body, "\t%s_fd protoreflect.FileDescriptor\n", prefix)
	if fg.nextMsgIndex > 0 {
		fmt.Fprintf(fg.body, "\t%s_msgTypes [%d]protoimpl.MessageInfo\n", prefix, fg.nextMsgIndex)
	}
	if fg.nextEnumIndex > 0 {
		fmt.Fprintf(fg.body, "\t%s_enumTypes [%d]protoimpl.EnumInfo\n", prefix, fg.nextEnumIndex)
	}
	fmt.Fprintf(fg.body, ")\n\n")

	// Emit init function.
	fmt.Fprintf(fg.body, "func %s_init() {\n", prefix)
	fmt.Fprintf(fg.body, "\tif %s_fd != nil {\n\t\treturn\n\t}\n", prefix)
	fmt.Fprintf(fg.body, "\tfdp := new(descriptorpb.FileDescriptorProto)\n")
	fmt.Fprintf(fg.body, "\tif err := proto.Unmarshal([]byte(%s_rawDesc), fdp); err != nil {\n", prefix)
	fmt.Fprintf(fg.body, "\t\tpanic(err)\n\t}\n")
	fmt.Fprintf(fg.body, "\tfd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)\n")
	fmt.Fprintf(fg.body, "\tif err != nil {\n")
	fmt.Fprintf(fg.body, "\t\tvar findErr error\n")
	fmt.Fprintf(fg.body, "\t\tfd, findErr = protoregistry.GlobalFiles.FindFileByPath(fdp.GetName())\n")
	fmt.Fprintf(fg.body, "\t\tif findErr != nil {\n")
	fmt.Fprintf(fg.body, "\t\t\tpanic(err)\n")
	fmt.Fprintf(fg.body, "\t\t}\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\t%s_fd = fd\n", prefix)
	fmt.Fprintf(fg.body, "\tprotoregistry.GlobalFiles.RegisterFile(fd)\n\n")

	// Register messages in the same order as emitAllProtoReflectMethods.
	msgIdx := 0
	forEachMessage(fd, func(md protoreflect.MessageDescriptor) {
		typeName := goMessageTypeName(md)
		fmt.Fprintf(fg.body, "\t%s_msgTypes[%d].GoReflectType = reflect.TypeOf((*%s)(nil))\n",
			prefix, msgIdx, typeName)
		fmt.Fprintf(fg.body, "\t%s_msgTypes[%d].Desc = protohelpers.FindMessageDescriptor(fd, %q)\n",
			prefix, msgIdx, md.FullName())

		// Collect oneof wrappers.
		var wrappers []string
		for i := 0; i < md.Oneofs().Len(); i++ {
			oo := md.Oneofs().Get(i)
			if oo.IsSynthetic() {
				continue
			}
			for j := 0; j < oo.Fields().Len(); j++ {
				variantName := oneofVariantName(md, oo.Fields().Get(j))
				wrappers = append(wrappers, fmt.Sprintf("(*%s)(nil)", variantName))
			}
		}
		if len(wrappers) > 0 {
			fmt.Fprintf(fg.body, "\t%s_msgTypes[%d].OneofWrappers = []any{%s}\n",
				prefix, msgIdx, strings.Join(wrappers, ", "))
		}

		fmt.Fprintf(fg.body, "\tprotoregistry.GlobalTypes.RegisterMessage(&%s_msgTypes[%d])\n",
			prefix, msgIdx)
		msgIdx++
	})

	// Register enums in the same order as emitAllEnums.
	enumIdx := 0
	emitEnumReg := func(ed protoreflect.EnumDescriptor) {
		typeName := goEnumTypeName(ed)
		fmt.Fprintf(fg.body, "\t%s_enumTypes[%d].GoReflectType = reflect.TypeOf(%s(0))\n",
			prefix, enumIdx, typeName)
		fmt.Fprintf(fg.body, "\t%s_enumTypes[%d].Desc = protohelpers.FindEnumDescriptor(fd, %q)\n",
			prefix, enumIdx, ed.FullName())
		fmt.Fprintf(fg.body, "\tprotoregistry.GlobalTypes.RegisterEnum(&%s_enumTypes[%d])\n",
			prefix, enumIdx)
		enumIdx++
	}

	for i := 0; i < fd.Enums().Len(); i++ {
		emitEnumReg(fd.Enums().Get(i))
	}
	forEachMessage(fd, func(md protoreflect.MessageDescriptor) {
		for i := 0; i < md.Enums().Len(); i++ {
			emitEnumReg(md.Enums().Get(i))
		}
	})

	fmt.Fprintf(fg.body, "}\n\n")
	fmt.Fprintf(fg.body, "func init() { %s_init() }\n", prefix)

	// Add imports needed by the generated registration code.
	fg.imports.addImport("reflect", "")
	fg.imports.addImport("google.golang.org/protobuf/proto", "")
	fg.imports.addImport("google.golang.org/protobuf/reflect/protodesc", "")
	fg.imports.addImport("google.golang.org/protobuf/reflect/protoreflect", "")
	fg.imports.addImport("google.golang.org/protobuf/reflect/protoregistry", "")
	fg.imports.addImport("google.golang.org/protobuf/runtime/protoimpl", "")
	fg.imports.addImport("google.golang.org/protobuf/types/descriptorpb", "")
	fg.imports.addImport(fg.module+"/gen/protohelpers", "")
}

// serializeFileDescriptor converts a protoreflect.FileDescriptor to raw proto bytes.
func serializeFileDescriptor(fd protoreflect.FileDescriptor) []byte {
	fdp := protodesc.ToFileDescriptorProto(fd)
	fdp.SourceCodeInfo = nil
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(fdp)
	if err != nil {
		panic(fmt.Sprintf("marshaling file descriptor for %s: %v", fd.Path(), err))
	}
	return b
}

// encodeRawDescriptor writes bytes as a Go string literal with \x hex escapes.
func encodeRawDescriptor(buf *bytes.Buffer, b []byte) {
	const lineLen = 40
	for i := 0; i < len(b); i += lineLen {
		end := i + lineLen
		if end > len(b) {
			end = len(b)
		}
		buf.WriteString("\t\"")
		for _, c := range b[i:end] {
			fmt.Fprintf(buf, "\\x%02x", c)
		}
		buf.WriteString("\"")
		if end < len(b) {
			buf.WriteString(" +\n")
		} else {
			buf.WriteString("\n")
		}
	}
}

// sanitizeFileVarName converts a proto file path to a valid Go identifier prefix.
func sanitizeFileVarName(path string) string {
	var b strings.Builder
	b.WriteString("file_")
	for _, c := range path {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
