package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"

	"wiresmith/compiler/types"
)

func (fg *FileGenerator) emitAllEqualMethods(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, fg.emitEqual)
}

func (fg *FileGenerator) emitEqual(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)

	fmt.Fprintf(fg.body, "func (this *%s) Equal(that interface{}) bool {\n", name)
	fmt.Fprintf(fg.body, "\tif that == nil {\n\t\treturn this == nil\n\t}\n\n")

	fmt.Fprintf(fg.body, "\tthat1, ok := that.(*%s)\n", name)
	fmt.Fprintf(fg.body, "\tif !ok {\n")
	fmt.Fprintf(fg.body, "\t\tthat2, ok := that.(%s)\n", name)
	fmt.Fprintf(fg.body, "\t\tif ok {\n\t\t\tthat1 = &that2\n\t\t} else {\n\t\t\treturn false\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\tif that1 == nil {\n\t\treturn this == nil\n\t} else if this == nil {\n\t\treturn false\n\t}\n")

	seenOneofs := map[string]bool{}
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)

		if isRealOneof(fd) {
			oo := fd.ContainingOneof()
			ooName := string(oo.Name())
			if !seenOneofs[ooName] {
				seenOneofs[ooName] = true
				fg.emitOneofEqual(md, oo)
			}
			continue
		}

		goName := snakeToPascal(string(fd.Name()))
		ft := fg.fieldType(fd)
		ft.EmitEqual(fg, "\t", "this."+goName, "that1."+goName)
	}

	fmt.Fprintf(fg.body, "\treturn true\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

func (fg *FileGenerator) emitOneofEqual(md protoreflect.MessageDescriptor, oo protoreflect.OneofDescriptor) {
	goName := snakeToPascal(string(oo.Name()))

	fmt.Fprintf(fg.body, "\tif (this.%s == nil) != (that1.%s == nil) {\n", goName, goName)
	fmt.Fprintf(fg.body, "\t\treturn false\n\t}\n")
	fmt.Fprintf(fg.body, "\tif this.%s != nil {\n", goName)
	fmt.Fprintf(fg.body, "\t\tswitch v := this.%s.(type) {\n", goName)

	for i := 0; i < oo.Fields().Len(); i++ {
		fd := oo.Fields().Get(i)
		variantType := oneofVariantName(md, fd)
		fieldName := snakeToPascal(string(fd.Name()))

		fmt.Fprintf(fg.body, "\t\tcase *%s:\n", variantType)
		fmt.Fprintf(fg.body, "\t\t\tv2, ok := that1.%s.(*%s)\n", goName, variantType)
		fmt.Fprintf(fg.body, "\t\t\tif !ok {\n\t\t\t\treturn false\n\t\t\t}\n")

		// Per-variant comparison: scalars/string/enum → `!=`, bytes →
		// bytes.Equal (and lazy import), message → `.Equal()`.
		types.Get(fd.Kind()).EmitEqual(fg, "\t\t\t", "v."+fieldName, "v2."+fieldName)
	}

	fmt.Fprintf(fg.body, "\t\tdefault:\n\t\t\treturn false\n")
	fmt.Fprintf(fg.body, "\t\t}\n")
	fmt.Fprintf(fg.body, "\t}\n")
}
