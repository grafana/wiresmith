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

	// Name map: int32 → string (first name wins for aliased values)
	fmt.Fprintf(fg.body, "var %s_name = map[int32]string{\n", typeName)
	seenNumbers := make(map[int32]bool)
	for i := 0; i < ed.Values().Len(); i++ {
		v := ed.Values().Get(i)
		num := int32(v.Number())
		if seenNumbers[num] {
			continue
		}
		seenNumbers[num] = true
		fmt.Fprintf(fg.body, "\t%d: %q,\n", num, string(v.Name()))
	}
	fmt.Fprintf(fg.body, "}\n\n")

	// Value map: string → int32
	fmt.Fprintf(fg.body, "var %s_value = map[string]int32{\n", typeName)
	for i := 0; i < ed.Values().Len(); i++ {
		v := ed.Values().Get(i)
		fmt.Fprintf(fg.body, "\t%q: %d,\n", string(v.Name()), v.Number())
	}
	fmt.Fprintf(fg.body, "}\n\n")

	// String() method
	fmt.Fprintf(fg.body, "func (x %s) String() string {\n", typeName)
	fmt.Fprintf(fg.body, "\tif name, ok := %s_name[int32(x)]; ok {\n", typeName)
	fmt.Fprintf(fg.body, "\t\treturn name\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\treturn strconv.FormatInt(int64(x), 10)\n")
	fmt.Fprintf(fg.body, "}\n\n")
	fg.imports.addImport("strconv", "")
}
