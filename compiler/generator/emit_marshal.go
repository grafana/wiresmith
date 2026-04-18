package generator

import (
	"fmt"
	"sort"
	"wiresmith/compiler/types"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitMarshal(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	fg.imports.addImport(fg.module+"/gen/protohelpers", "")

	// Marshal allocates and returns the encoded bytes.
	fmt.Fprintf(fg.body, "func (m *%s) Marshal() (dAtA []byte, err error) {\n", name)
	fmt.Fprintf(fg.body, "\tsize := m.Size()\n")
	fmt.Fprintf(fg.body, "\tif size == 0 {\n\t\treturn nil, nil\n\t}\n")
	fmt.Fprintf(fg.body, "\tdAtA = make([]byte, size)\n")
	fmt.Fprintf(fg.body, "\tn, err := m.MarshalToSizedBuffer(dAtA[:size])\n")
	fmt.Fprintf(fg.body, "\tif err != nil {\n\t\treturn nil, err\n\t}\n")
	fmt.Fprintf(fg.body, "\treturn dAtA[:n], nil\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// MarshalTo writes the message into dAtA.
	fmt.Fprintf(fg.body, "func (m *%s) MarshalTo(dAtA []byte) (int, error) {\n", name)
	fmt.Fprintf(fg.body, "\tsize := m.Size()\n")
	fmt.Fprintf(fg.body, "\treturn m.MarshalToSizedBuffer(dAtA[:size])\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// MarshalToSizedBuffer writes the message backwards into dAtA.
	// Returns the number of bytes written.
	fmt.Fprintf(fg.body, "func (m *%s) MarshalToSizedBuffer(dAtA []byte) (int, error) {\n", name)
	fmt.Fprintf(fg.body, "\ti := len(dAtA)\n")

	// Collect fields sorted by number descending for reverse-write.
	var fields []protoreflect.FieldDescriptor
	for i := 0; i < md.Fields().Len(); i++ {
		fields = append(fields, md.Fields().Get(i))
	}
	sort.Slice(fields, func(a, b int) bool {
		return fields[a].Number() > fields[b].Number()
	})

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
		fg.emitFieldMarshalReverse(fd)
	}

	fmt.Fprintf(fg.body, "\treturn len(dAtA) - i, nil\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

func (fg *FileGenerator) emitFieldMarshalReverse(fd protoreflect.FieldDescriptor) {
	goName := snakeToPascal(string(fd.Name()))
	access := "m." + goName
	num := protowire.Number(fd.Number())

	ft := types.ForField(fd)
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
		fieldName := snakeToPascal(string(fd.Name()))
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
