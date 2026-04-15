package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitEnum(ed protoreflect.EnumDescriptor) {
	typeName := goEnumTypeName(ed)

	fmt.Fprintf(fg.body, "type %s int32\n\n", typeName)
	fmt.Fprintf(fg.body, "const (\n")
	for i := 0; i < ed.Values().Len(); i++ {
		v := ed.Values().Get(i)
		fmt.Fprintf(fg.body, "\t%s %s = %d\n", string(v.Name()), typeName, v.Number())
	}
	fmt.Fprintf(fg.body, ")\n\n")
}
