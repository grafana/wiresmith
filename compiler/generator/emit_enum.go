package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitEnum(ed protoreflect.EnumDescriptor) {
	typeName := goEnumTypeName(ed)

	// Prefix for enum value constants: parent message names joined with "_".
	// e.g. Span.SpanKind.SPAN_KIND_CLIENT → Span_SPAN_KIND_CLIENT
	valuePrefix := goEnumValuePrefix(ed)

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
		fmt.Fprintf(fg.body, "\t%s%s %s = %d\n", valuePrefix, string(v.Name()), typeName, v.Number())
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

	// UnmarshalJSON: accepts both string names and integer values.
	fmt.Fprintf(fg.body, "func (x *%s) UnmarshalJSON(data []byte) error {\n", typeName)
	fmt.Fprintf(fg.body, "\tif len(data) > 0 && data[0] == '\"' {\n")
	fmt.Fprintf(fg.body, "\t\tvar s string\n")
	fmt.Fprintf(fg.body, "\t\tif err := json.Unmarshal(data, &s); err != nil {\n")
	fmt.Fprintf(fg.body, "\t\t\treturn err\n")
	fmt.Fprintf(fg.body, "\t\t}\n")
	fmt.Fprintf(fg.body, "\t\tif v, ok := %s_value[s]; ok {\n", typeName)
	fmt.Fprintf(fg.body, "\t\t\t*x = %s(v)\n", typeName)
	fmt.Fprintf(fg.body, "\t\t\treturn nil\n")
	fmt.Fprintf(fg.body, "\t\t}\n")
	fmt.Fprintf(fg.body, "\t\treturn fmt.Errorf(\"unknown enum value %%q for %s\", s)\n", typeName)
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\tvar v int32\n")
	fmt.Fprintf(fg.body, "\tif err := json.Unmarshal(data, &v); err != nil {\n")
	fmt.Fprintf(fg.body, "\t\treturn err\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\t*x = %s(v)\n", typeName)
	fmt.Fprintf(fg.body, "\treturn nil\n")
	fmt.Fprintf(fg.body, "}\n\n")
	fg.imports.addImport("encoding/json", "")

	// MarshalJSON: emit string name.
	fmt.Fprintf(fg.body, "func (x %s) MarshalJSON() ([]byte, error) {\n", typeName)
	fmt.Fprintf(fg.body, "\treturn json.Marshal(x.String())\n")
	fmt.Fprintf(fg.body, "}\n\n")
}
