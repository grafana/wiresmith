package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitEnum(ed protoreflect.EnumDescriptor) {
	typeName := goEnumTypeName(ed)

	if c := leadingComment(ed); c != "" {
		fg.body.WriteString(c)
	}

	fmt.Fprintf(fg.body, "type %s int32\n\n", typeName)
	fmt.Fprintf(fg.body, "const (\n")
	for i := 0; i < ed.Values().Len(); i++ {
		v := ed.Values().Get(i)
		if c := leadingComment(v); c != "" {
			fg.body.WriteString(indentComment(c))
		}
		fmt.Fprintf(fg.body, "\t%s %s = %d\n", string(v.Name()), typeName, v.Number())
	}
	fmt.Fprintf(fg.body, ")\n\n")
}
