package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// emitEnum emits the enum's type, named constants, name/value maps, and
// String() method into the MAIN .pb.go file. The protoreflect-shaped
// Descriptor() / Type() / Number() methods are emitted separately by
// emitEnumReflect — see the file split rationale on FileGenerator.reflectBody.
func (fg *FileGenerator) emitEnum(ed protoreflect.EnumDescriptor) {
	typeName := goEnumTypeName(ed)
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
		fmt.Fprintf(fg.body, "\t%s_%s %s = %d\n", valuePrefix, string(v.Name()), typeName, v.Number())
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

// emitEnumReflect emits the protoreflect-shaped Descriptor() / Type() /
// Number() methods for one enum into the COMPANION _reflect.pb.go file.
//
// MUST be called in the same iteration order as emitEnum, because both this
// method and emitRegistration index into the shared `file_*_enumTypes` array
// by `fg.nextEnumIndex` — they need to see the same enums in the same order
// or the registered descriptors won't match the bodies that read from the
// array. (emitAllEnumReflectMethods enforces this by mirroring emitAllEnums'
// traversal.)
func (fg *FileGenerator) emitEnumReflect(ed protoreflect.EnumDescriptor) {
	typeName := goEnumTypeName(ed)

	idx := fg.nextEnumIndex
	fg.nextEnumIndex++

	fmt.Fprintf(fg.reflectBody, "func (x %s) Descriptor() protoreflect.EnumDescriptor {\n", typeName)
	fmt.Fprintf(fg.reflectBody, "\t%s_init()\n", fg.fileVarName)
	fmt.Fprintf(fg.reflectBody, "\treturn %s_enumTypes[%d].Desc\n", fg.fileVarName, idx)
	fmt.Fprintf(fg.reflectBody, "}\n\n")

	fmt.Fprintf(fg.reflectBody, "func (x %s) Type() protoreflect.EnumType {\n", typeName)
	fmt.Fprintf(fg.reflectBody, "\t%s_init()\n", fg.fileVarName)
	fmt.Fprintf(fg.reflectBody, "\treturn &%s_enumTypes[%d]\n", fg.fileVarName, idx)
	fmt.Fprintf(fg.reflectBody, "}\n\n")

	fmt.Fprintf(fg.reflectBody, "func (x %s) Number() protoreflect.EnumNumber {\n", typeName)
	fmt.Fprintf(fg.reflectBody, "\treturn protoreflect.EnumNumber(x)\n")
	fmt.Fprintf(fg.reflectBody, "}\n\n")

	fg.reflectImports.addImport("google.golang.org/protobuf/reflect/protoreflect", "")
}
