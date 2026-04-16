package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// emitAllEqualMethods generates Equal() methods for all messages.
func (fg *FileGenerator) emitAllEqualMethods(fd protoreflect.FileDescriptor) {
	for i := 0; i < fd.Messages().Len(); i++ {
		fg.emitEqualMethods(fd.Messages().Get(i))
	}
}

func (fg *FileGenerator) emitEqualMethods(md protoreflect.MessageDescriptor) {
	for i := 0; i < md.Messages().Len(); i++ {
		fg.emitEqualMethods(md.Messages().Get(i))
	}
	fg.emitEqual(md)
}

func (fg *FileGenerator) emitEqual(md protoreflect.MessageDescriptor) {
	// Skip Equal for messages with (gogoproto.equal) = false.
	if isMessageOptionFalse(md, 64013) {
		return
	}

	name := goMessageTypeName(md)

	// Only import "bytes" if there are []byte fields.
	if needsBytesImportForEqual(md) {
		fg.imports.addStdImport("bytes")
	}

	fmt.Fprintf(fg.body, "func (this *%s) Equal(that interface{}) bool {\n", name)
	fmt.Fprintf(fg.body, "\tif that == nil {\n\t\treturn this == nil\n\t}\n\n")

	// Type assertion
	fmt.Fprintf(fg.body, "\tthat1, ok := that.(*%s)\n", name)
	fmt.Fprintf(fg.body, "\tif !ok {\n")
	fmt.Fprintf(fg.body, "\t\tthat2, ok := that.(%s)\n", name)
	fmt.Fprintf(fg.body, "\t\tif ok {\n\t\t\tthat1 = &that2\n\t\t} else {\n\t\t\treturn false\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\tif that1 == nil {\n\t\treturn this == nil\n\t} else if this == nil {\n\t\treturn false\n\t}\n")

	// Compare fields
	seenOneofs := map[string]bool{}
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)

		if isRealOneof(fd) {
			oo := fd.ContainingOneof()
			ooName := string(oo.Name())
			if !seenOneofs[ooName] {
				seenOneofs[ooName] = true
				goName := snakeToPascal(ooName)
				// For oneofs, we need to compare the interface values.
				// This is a simplified comparison; a full implementation
				// would do type switches.
				fmt.Fprintf(fg.body, "\tif this.%s != that1.%s {\n", goName, goName)
				fmt.Fprintf(fg.body, "\t\treturn false\n\t}\n")
			}
			continue
		}

		goName := snakeToPascal(string(fd.Name()))
		fg.emitFieldEqual(fd, goName)
	}

	fmt.Fprintf(fg.body, "\treturn true\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

func (fg *FileGenerator) emitFieldEqual(fd protoreflect.FieldDescriptor, goName string) {
	if fd.IsList() {
		fg.emitRepeatedFieldEqual(fd, goName)
		return
	}
	if fd.HasOptionalKeyword() {
		fg.emitOptionalFieldEqual(fd, goName)
		return
	}

	switch fd.Kind() {
	case protoreflect.BytesKind:
		fmt.Fprintf(fg.body, "\tif !bytes.Equal(this.%s, that1.%s) {\n\t\treturn false\n\t}\n", goName, goName)
	case protoreflect.MessageKind:
		if isGogoPointerField(fg.gen, fd) {
			// Pointer message field: nil check then Equal.
			fmt.Fprintf(fg.body, "\tif !this.%s.Equal(that1.%s) {\n\t\treturn false\n\t}\n", goName, goName)
		} else {
			// Value message field.
			fmt.Fprintf(fg.body, "\tif !this.%s.Equal(that1.%s) {\n\t\treturn false\n\t}\n", goName, goName)
		}
	default:
		fmt.Fprintf(fg.body, "\tif this.%s != that1.%s {\n\t\treturn false\n\t}\n", goName, goName)
	}
}

func (fg *FileGenerator) emitOptionalFieldEqual(fd protoreflect.FieldDescriptor, goName string) {
	// For optional (pointer) fields, check nil first then compare values.
	fmt.Fprintf(fg.body, "\tif this.%s != that1.%s {\n", goName, goName)
	fmt.Fprintf(fg.body, "\t\tif this.%s == nil || that1.%s == nil {\n\t\t\treturn false\n\t\t}\n", goName, goName)
	fmt.Fprintf(fg.body, "\t\tif *this.%s != *that1.%s {\n\t\t\treturn false\n\t\t}\n", goName, goName)
	fmt.Fprintf(fg.body, "\t}\n")
}

// needsBytesImportForEqual checks if any field in the message uses []byte.
func needsBytesImportForEqual(md protoreflect.MessageDescriptor) bool {
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if fd.Kind() == protoreflect.BytesKind {
			return true
		}
	}
	return false
}

func (fg *FileGenerator) emitRepeatedFieldEqual(fd protoreflect.FieldDescriptor, goName string) {
	fmt.Fprintf(fg.body, "\tif len(this.%s) != len(that1.%s) {\n\t\treturn false\n\t}\n", goName, goName)
	fmt.Fprintf(fg.body, "\tfor i := range this.%s {\n", goName)

	switch fd.Kind() {
	case protoreflect.BytesKind:
		fmt.Fprintf(fg.body, "\t\tif !bytes.Equal(this.%s[i], that1.%s[i]) {\n\t\t\treturn false\n\t\t}\n", goName, goName)
	case protoreflect.MessageKind:
		fmt.Fprintf(fg.body, "\t\tif !this.%s[i].Equal(that1.%s[i]) {\n\t\t\treturn false\n\t\t}\n", goName, goName)
	default:
		fmt.Fprintf(fg.body, "\t\tif this.%s[i] != that1.%s[i] {\n\t\t\treturn false\n\t\t}\n", goName, goName)
	}

	fmt.Fprintf(fg.body, "\t}\n")
}
