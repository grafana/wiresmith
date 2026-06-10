package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitStruct(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)

	if c := leadingComment(md); c != "" {
		fg.body.WriteString(c)
	}

	fmt.Fprintf(fg.body, "type %s struct {\n", name)

	seenOneofs := map[string]bool{}
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)

		if fd.IsMap() {
			if c := leadingComment(fd); c != "" {
				fg.body.WriteString(indentComment(c))
			}
			goName := fg.goFieldName(fd)
			goType := fg.imports.goType(fd)
			fmt.Fprintf(fg.body, "\t%s %s %s\n", goName, goType, fg.mapFieldTag(fd))
			continue
		}

		if oo := fd.ContainingOneof(); oo != nil && !oo.IsSynthetic() {
			ooName := string(oo.Name())
			if !seenOneofs[ooName] {
				seenOneofs[ooName] = true
				if c := leadingComment(oo); c != "" {
					fg.body.WriteString(indentComment(c))
				}
				goName := snakeToPascal(ooName)
				ifaceName := oneofInterfaceName(md, oo)
				fmt.Fprintf(fg.body, "\t%s %s %s\n", goName, ifaceName, oneofInterfaceTag(oo))
			}
			continue
		}

		if c := leadingComment(fd); c != "" {
			fg.body.WriteString(indentComment(c))
		}
		goName := fg.goFieldName(fd)
		goType := fg.goFieldType(fd)
		fmt.Fprintf(fg.body, "\t%s %s %s\n", goName, goType, fg.fieldTag(fd))
	}

	if words := fg.presenceBitmapWords(md); words > 0 {
		fmt.Fprintf(fg.body, "\n\tXXX_fieldsPresent [%d]uint64\n", words)
	}

	fmt.Fprintf(fg.body, "}\n\n")
}
