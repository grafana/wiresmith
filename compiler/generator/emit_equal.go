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
	out := fg.equalBody
	em := equalEmitter{fg: fg}

	fmt.Fprintf(out, "func (this *%s) Equal(that interface{}) bool {\n", name)
	fmt.Fprintf(out, "\tif that == nil {\n\t\treturn this == nil\n\t}\n\n")

	fmt.Fprintf(out, "\tthat1, ok := that.(*%s)\n", name)
	fmt.Fprintf(out, "\tif !ok {\n")
	fmt.Fprintf(out, "\t\tthat2, ok := that.(%s)\n", name)
	fmt.Fprintf(out, "\t\tif ok {\n\t\t\tthat1 = &that2\n\t\t} else {\n\t\t\treturn false\n\t\t}\n")
	fmt.Fprintf(out, "\t}\n")
	fmt.Fprintf(out, "\tif that1 == nil {\n\t\treturn this == nil\n\t} else if this == nil {\n\t\treturn false\n\t}\n")

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
		ft.EmitEqual(em, "\t", "this."+goName, "that1."+goName)
	}

	fmt.Fprintf(out, "\treturn true\n")
	fmt.Fprintf(out, "}\n\n")
}

func (fg *FileGenerator) emitOneofEqual(md protoreflect.MessageDescriptor, oo protoreflect.OneofDescriptor) {
	goName := snakeToPascal(string(oo.Name()))
	out := fg.equalBody
	em := equalEmitter{fg: fg}

	fmt.Fprintf(out, "\tif (this.%s == nil) != (that1.%s == nil) {\n", goName, goName)
	fmt.Fprintf(out, "\t\treturn false\n\t}\n")
	fmt.Fprintf(out, "\tif this.%s != nil {\n", goName)
	fmt.Fprintf(out, "\t\tswitch v := this.%s.(type) {\n", goName)

	for i := 0; i < oo.Fields().Len(); i++ {
		fd := oo.Fields().Get(i)
		variantType := oneofVariantName(md, fd)
		fieldName := snakeToPascal(string(fd.Name()))

		fmt.Fprintf(out, "\t\tcase *%s:\n", variantType)
		fmt.Fprintf(out, "\t\t\tv2, ok := that1.%s.(*%s)\n", goName, variantType)
		fmt.Fprintf(out, "\t\t\tif !ok {\n\t\t\t\treturn false\n\t\t\t}\n")

		// Per-variant comparison: scalars/string/enum → `!=`, bytes →
		// bytes.Equal (and lazy import), message → `.Equal()`.
		types.Get(fd.Kind()).EmitEqual(em, "\t\t\t", "v."+fieldName, "v2."+fieldName)
	}

	fmt.Fprintf(out, "\t\tdefault:\n\t\t\treturn false\n")
	fmt.Fprintf(out, "\t\t}\n")
	fmt.Fprintf(out, "\t}\n")
}
