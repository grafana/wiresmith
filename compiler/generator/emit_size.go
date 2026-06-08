package generator

import (
	"fmt"

	"github.com/grafana/wiresmith/compiler/types"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitSize(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")

	fmt.Fprintf(fg.body, "func (m *%s) Size() int {\n", name)
	fmt.Fprintf(fg.body, "\tif m == nil {\n\t\treturn 0\n\t}\n")
	fmt.Fprintf(fg.body, "\tvar n int\n")

	pm := fg.presenceMap(md)
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
		// Singular message fields with presence bitmap: account for
		// tag + length-0 when the field was set to an empty message.
		// Customtype-annotated message fields opt out — the user type
		// owns presence via SizeWiresmith(), so they take the regular
		// emitFieldSize -> fieldType -> CustomType.EmitSize path.
		if bitIndex, ok := pm[fd.Number()]; ok && fd.Kind() == protoreflect.MessageKind && !fg.hasCustomtypeOption(fd) {
			fg.emitMessageSizeWithPresence(fd, bitIndex)
			continue
		}
		fg.emitFieldSize(fd)
	}

	fmt.Fprintf(fg.body, "\treturn n\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

// emitMessageSizeWithPresence emits size code for a singular message field
// with presence bitmap. Adds tag + 1 byte (varint 0) when the field was
// present but the nested message is empty.
func (fg *FileGenerator) emitMessageSizeWithPresence(fd protoreflect.FieldDescriptor, bitIndex int) {
	goName := fg.goFieldName(fd)
	access := "m." + goName
	tagSize := protowire.SizeTag(protowire.Number(fd.Number()))

	fmt.Fprintf(fg.body, "\t{\n\t\ts := %s.Size()\n", access)
	fmt.Fprintf(fg.body, "\t\tif s > 0 {\n\t\t\tn += %d + protowire.SizeVarint(uint64(s)) + s\n\t\t}", tagSize)
	fmt.Fprintf(fg.body, " else if %s {\n\t\t\tn += %d\n\t\t}\n", presenceCheck(bitIndex), tagSize+1)
	fmt.Fprintf(fg.body, "\t}\n")
}

func (fg *FileGenerator) emitFieldSize(fd protoreflect.FieldDescriptor) {
	goName := fg.goFieldName(fd)
	access := "m." + goName
	tagSize := protowire.SizeTag(protowire.Number(fd.Number()))

	ft := fg.fieldType(fd)
	types.AddTypeImports(fg, ft)
	ft.EmitSize(fg, access, tagSize)
}

func (fg *FileGenerator) emitOneofSize(md protoreflect.MessageDescriptor, oo protoreflect.OneofDescriptor) {
	ooFieldName := snakeToPascal(string(oo.Name()))
	fmt.Fprintf(fg.body, "\tswitch v := m.%s.(type) {\n", ooFieldName)

	for i := 0; i < oo.Fields().Len(); i++ {
		fd := oo.Fields().Get(i)
		variantName := oneofVariantName(md, fd)
		fieldName := fg.goFieldName(fd)
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
