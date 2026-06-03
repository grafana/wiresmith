package generator

import (
	"fmt"
	"sort"
	"github.com/grafana/wiresmith/compiler/types"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitMarshal(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	fg.imports.addImport(protohelpersImport, "")

	// Marshal allocates and returns the encoded bytes.
	fmt.Fprintf(fg.body, "func (m *%s) Marshal() (dAtA []byte, err error) {\n", name)
	fmt.Fprintf(fg.body, "\tif m == nil {\n\t\treturn nil, nil\n\t}\n")
	fmt.Fprintf(fg.body, "\tsize := m.Size()\n")
	fmt.Fprintf(fg.body, "\tdAtA = make([]byte, size)\n")
	fmt.Fprintf(fg.body, "\tif size == 0 {\n\t\treturn dAtA, nil\n\t}\n")
	fmt.Fprintf(fg.body, "\tn, err := m.MarshalToSizedBuffer(dAtA[:size])\n")
	fmt.Fprintf(fg.body, "\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(fg.body, "\treturn dAtA[:n], nil\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// MarshalTo writes the message into dAtA.
	fmt.Fprintf(fg.body, "func (m *%s) MarshalTo(dAtA []byte) (int, error) {\n", name)
	fmt.Fprintf(fg.body, "\tif m == nil {\n\t\treturn 0, nil\n\t}\n")
	fmt.Fprintf(fg.body, "\tsize := m.Size()\n")
	fmt.Fprintf(fg.body, "\treturn m.MarshalToSizedBuffer(dAtA[:size])\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// MarshalToSizedBuffer writes the message backwards into dAtA.
	// Returns the number of bytes written.
	fmt.Fprintf(fg.body, "func (m *%s) MarshalToSizedBuffer(dAtA []byte) (int, error) {\n", name)
	fmt.Fprintf(fg.body, "\tif m == nil {\n\t\treturn 0, nil\n\t}\n")
	fmt.Fprintf(fg.body, "\ti := len(dAtA)\n")

	// Collect fields sorted by number descending for reverse-write.
	var fields []protoreflect.FieldDescriptor
	for i := 0; i < md.Fields().Len(); i++ {
		fields = append(fields, md.Fields().Get(i))
	}
	sort.Slice(fields, func(a, b int) bool {
		return fields[a].Number() > fields[b].Number()
	})

	pm := fg.presenceMap(md)
	seenOneofs := map[string]bool{}
	for _, fd := range fields {
		if isRealOneof(fd) {
			oo := fd.ContainingOneof()
			ooName := string(oo.Name())
			if seenOneofs[ooName] {
				continue
			}
			seenOneofs[ooName] = true
			fg.emitOneofMarshalReverse(md, oo)
			continue
		}
		// Singular message fields with presence bitmap: emit the field
		// even when empty (size 0) if the bitmap says it was set.
		if bitIndex, ok := pm[fd.Number()]; ok && fd.Kind() == protoreflect.MessageKind {
			fg.emitMessageMarshalWithPresence(fd, bitIndex)
			continue
		}
		fg.emitFieldMarshalReverse(fd)
	}

	fmt.Fprintf(fg.body, "\treturn len(dAtA) - i, nil\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

func (fg *FileGenerator) emitFieldMarshalReverse(fd protoreflect.FieldDescriptor) {
	goName := fg.goFieldName(fd)
	access := "m." + goName
	num := protowire.Number(fd.Number())

	ft := fg.fieldType(fd)
	types.AddTypeImports(fg, ft)
	ft.EmitMarshal(fg, access, num)
}

// reverseTag writes tag bytes in reverse order into the buffer.
func (fg *FileGenerator) reverseTag(indent string, num protowire.Number, wt protowire.Type) {
	tag := computeTagBytes(num, wt)
	for j := len(tag) - 1; j >= 0; j-- {
		fmt.Fprintf(fg.body, "%si--\n%sdAtA[i] = 0x%02x\n", indent, indent, tag[j])
	}
}

// emitMessageMarshalWithPresence emits marshal code for a singular message
// field that uses the presence bitmap. When the nested message is non-empty it
// is written normally; when it is empty but the bitmap says it was present on
// the wire, a zero-length field (tag + varint 0) is emitted.
func (fg *FileGenerator) emitMessageMarshalWithPresence(fd protoreflect.FieldDescriptor, bitIndex int) {
	goName := fg.goFieldName(fd)
	access := "m." + goName
	num := protowire.Number(fd.Number())

	fg.imports.addImport(protohelpersImport, "")
	fmt.Fprintf(fg.body, "\t{\n")
	fmt.Fprintf(fg.body, "\t\tsize, err := %s.MarshalToSizedBuffer(dAtA[:i])\n", access)
	fmt.Fprintf(fg.body, "\t\tif err != nil {\n\t\t\treturn 0, err\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t\tif size > 0 {\n")
	fmt.Fprintf(fg.body, "\t\t\ti -= size\n")
	fmt.Fprintf(fg.body, "\t\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(size))\n")
	fg.reverseTag("\t\t\t", num, protowire.BytesType)
	fmt.Fprintf(fg.body, "\t\t} else if %s {\n", presenceCheck(bitIndex))
	fmt.Fprintf(fg.body, "\t\t\ti--\n")
	fmt.Fprintf(fg.body, "\t\t\tdAtA[i] = 0\n")
	fg.reverseTag("\t\t\t", num, protowire.BytesType)
	fmt.Fprintf(fg.body, "\t\t}\n")
	fmt.Fprintf(fg.body, "\t}\n")
}

func (fg *FileGenerator) emitOneofMarshalReverse(md protoreflect.MessageDescriptor, oo protoreflect.OneofDescriptor) {
	ooFieldName := snakeToPascal(string(oo.Name()))
	fmt.Fprintf(fg.body, "\tswitch v := m.%s.(type) {\n", ooFieldName)

	// Sort variants by field number descending
	var variants []protoreflect.FieldDescriptor
	for i := 0; i < oo.Fields().Len(); i++ {
		variants = append(variants, oo.Fields().Get(i))
	}
	sort.Slice(variants, func(a, b int) bool {
		return variants[a].Number() > variants[b].Number()
	})

	for _, fd := range variants {
		variantName := oneofVariantName(md, fd)
		fieldName := fg.goFieldName(fd)
		access := "v." + fieldName
		num := protowire.Number(fd.Number())

		fmt.Fprintf(fg.body, "\tcase *%s:\n", variantName)

		t := types.Get(fd.Kind())
		types.AddTypeImports(fg, t)
		of := &types.OneofField{Inner: t}
		of.EmitMarshal(fg, access, num)
	}

	fmt.Fprintf(fg.body, "\t}\n")
}
