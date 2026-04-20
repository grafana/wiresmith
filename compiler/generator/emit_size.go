package generator

import (
	"fmt"
	"wiresmith/compiler/types"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitSize(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")

	fmt.Fprintf(fg.body, "func (m *%s) Size() int {\n", name)
	fmt.Fprintf(fg.body, "\tvar n int\n")

	seenOneofs := map[string]bool{}
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if isRealOneof(fd) {
			oo := fd.ContainingOneof()
			ooName := string(oo.Name())
			if !seenOneofs[ooName] {
				seenOneofs[ooName] = true
				fg.emitOneofSize(md, oo)
			}
			continue
		}
		fg.emitFieldSize(fd)
	}

	fmt.Fprintf(fg.body, "\tn += len(m.unknownFields)\n")
	fmt.Fprintf(fg.body, "\treturn n\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

func (fg *FileGenerator) emitFieldSize(fd protoreflect.FieldDescriptor) {
	goName := snakeToPascal(string(fd.Name()))
	access := "m." + goName
	tagSize := protowire.SizeTag(protowire.Number(fd.Number()))

	ft := types.ForField(fd)
	types.AddTypeImports(fg, ft)
	ft.EmitSize(fg, access, tagSize)
}

func (fg *FileGenerator) emitOneofSize(md protoreflect.MessageDescriptor, oo protoreflect.OneofDescriptor) {
	ooFieldName := snakeToPascal(string(oo.Name()))
	fmt.Fprintf(fg.body, "\tswitch v := m.%s.(type) {\n", ooFieldName)

	for i := 0; i < oo.Fields().Len(); i++ {
		fd := oo.Fields().Get(i)
		variantName := oneofVariantName(md, fd)
		fieldName := snakeToPascal(string(fd.Name()))
		tagSize := protowire.SizeTag(protowire.Number(fd.Number()))
		access := "v." + fieldName

		fmt.Fprintf(fg.body, "\tcase *%s:\n", variantName)

		t := types.Get(fd.Kind())
		types.AddTypeImports(fg, t)
		of := &types.OneofField{Inner: t}
		of.EmitSize(fg, access, tagSize)
	}

	fmt.Fprintf(fg.body, "\t}\n")
}
