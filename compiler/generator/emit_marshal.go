package generator

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitMarshal(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	fg.imports.addImport(fg.module+"/gen/protohelpers", "")

	// MarshalProto allocates and returns the encoded bytes.
	fmt.Fprintf(fg.body, "func (m *%s) MarshalProto() ([]byte, error) {\n", name)
	fmt.Fprintf(fg.body, "\tsize := m.SizeProto()\n")
	fmt.Fprintf(fg.body, "\tif size == 0 {\n\t\treturn nil, nil\n\t}\n")
	fmt.Fprintf(fg.body, "\tdAtA := make([]byte, size)\n")
	fmt.Fprintf(fg.body, "\tn := m.MarshalToSizedBuffer(dAtA)\n")
	fmt.Fprintf(fg.body, "\treturn dAtA[:n], nil\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// MarshalToSizedBuffer writes the message backwards into dAtA.
	// Returns the number of bytes written.
	fmt.Fprintf(fg.body, "func (m *%s) MarshalToSizedBuffer(dAtA []byte) int {\n", name)
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

	fmt.Fprintf(fg.body, "\treturn len(dAtA) - i\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

func (fg *FileGenerator) emitFieldMarshalReverse(fd protoreflect.FieldDescriptor) {
	goName := snakeToPascal(string(fd.Name()))
	access := "m." + goName

	if fd.IsList() {
		fg.emitRepeatedFieldMarshalReverse(access, fd)
		return
	}
	if fd.HasOptionalKeyword() {
		fg.emitOptionalFieldMarshalReverse(access, fd)
		return
	}
	fg.emitSingularFieldMarshalReverse(access, fd)
}

// reverseTag writes tag bytes in reverse order into the buffer.
func (fg *FileGenerator) reverseTag(indent string, num protowire.Number, wt protowire.Type) {
	tag := computeTagBytes(num, wt)
	for j := len(tag) - 1; j >= 0; j-- {
		fmt.Fprintf(fg.body, "%si--\n%sdAtA[i] = 0x%02x\n", indent, indent, tag[j])
	}
}

func (fg *FileGenerator) needsBinaryImport(kind protoreflect.Kind) {
	if isFixed32Kind(kind) || isFixed64Kind(kind) {
		fg.imports.addImport("encoding/binary", "")
	}
}

func (fg *FileGenerator) emitSingularFieldMarshalReverse(access string, fd protoreflect.FieldDescriptor) {
	kind := fd.Kind()
	num := protowire.Number(fd.Number())
	fg.needsBinaryImport(kind)

	switch kind {
	case protoreflect.BoolKind:
		fmt.Fprintf(fg.body, "\tif %s {\n", access)
		fmt.Fprintf(fg.body, "\t\ti--\n\t\tif %s {\n\t\t\tdAtA[i] = 1\n\t\t} else {\n\t\t\tdAtA[i] = 0\n\t\t}\n", access)
		fg.reverseTag("\t\t", num, protowire.VarintType)
		fmt.Fprintf(fg.body, "\t}\n")

	case protoreflect.Int32Kind, protoreflect.Uint32Kind, protoreflect.Int64Kind, protoreflect.Uint64Kind, protoreflect.EnumKind:
		fmt.Fprintf(fg.body, "\tif %s != 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(%s))\n", access)
		fg.reverseTag("\t\t", num, protowire.VarintType)
		fmt.Fprintf(fg.body, "\t}\n")

	case protoreflect.Sint32Kind:
		fmt.Fprintf(fg.body, "\tif %s != 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(uint32(%s<<1)^uint32(int32(%s)>>31)))\n", access, access)
		fg.reverseTag("\t\t", num, protowire.VarintType)
		fmt.Fprintf(fg.body, "\t}\n")

	case protoreflect.Sint64Kind:
		fmt.Fprintf(fg.body, "\tif %s != 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(uint64(%s<<1)^uint64(int64(%s)>>63)))\n", access, access)
		fg.reverseTag("\t\t", num, protowire.VarintType)
		fmt.Fprintf(fg.body, "\t}\n")

	case protoreflect.Fixed32Kind:
		fmt.Fprintf(fg.body, "\tif %s != 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\ti -= 4\n\t\tbinary.LittleEndian.PutUint32(dAtA[i:], %s)\n", access)
		fg.reverseTag("\t\t", num, protowire.Fixed32Type)
		fmt.Fprintf(fg.body, "\t}\n")

	case protoreflect.Sfixed32Kind:
		fmt.Fprintf(fg.body, "\tif %s != 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\ti -= 4\n\t\tbinary.LittleEndian.PutUint32(dAtA[i:], uint32(%s))\n", access)
		fg.reverseTag("\t\t", num, protowire.Fixed32Type)
		fmt.Fprintf(fg.body, "\t}\n")

	case protoreflect.FloatKind:
		fg.imports.addImport("math", "")
		fmt.Fprintf(fg.body, "\tif %s != 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\ti -= 4\n\t\tbinary.LittleEndian.PutUint32(dAtA[i:], math.Float32bits(%s))\n", access)
		fg.reverseTag("\t\t", num, protowire.Fixed32Type)
		fmt.Fprintf(fg.body, "\t}\n")

	case protoreflect.Fixed64Kind:
		fmt.Fprintf(fg.body, "\tif %s != 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\ti -= 8\n\t\tbinary.LittleEndian.PutUint64(dAtA[i:], %s)\n", access)
		fg.reverseTag("\t\t", num, protowire.Fixed64Type)
		fmt.Fprintf(fg.body, "\t}\n")

	case protoreflect.Sfixed64Kind:
		fmt.Fprintf(fg.body, "\tif %s != 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\ti -= 8\n\t\tbinary.LittleEndian.PutUint64(dAtA[i:], uint64(%s))\n", access)
		fg.reverseTag("\t\t", num, protowire.Fixed64Type)
		fmt.Fprintf(fg.body, "\t}\n")

	case protoreflect.DoubleKind:
		fg.imports.addImport("math", "")
		fmt.Fprintf(fg.body, "\tif %s != 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\ti -= 8\n\t\tbinary.LittleEndian.PutUint64(dAtA[i:], math.Float64bits(%s))\n", access)
		fg.reverseTag("\t\t", num, protowire.Fixed64Type)
		fmt.Fprintf(fg.body, "\t}\n")

	case protoreflect.StringKind:
		fmt.Fprintf(fg.body, "\tif len(%s) > 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\ti -= len(%s)\n\t\tcopy(dAtA[i:], %s)\n", access, access)
		fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(len(%s)))\n", access)
		fg.reverseTag("\t\t", num, protowire.BytesType)
		fmt.Fprintf(fg.body, "\t}\n")

	case protoreflect.BytesKind:
		fmt.Fprintf(fg.body, "\tif len(%s) > 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\ti -= len(%s)\n\t\tcopy(dAtA[i:], %s)\n", access, access)
		fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(len(%s)))\n", access)
		fg.reverseTag("\t\t", num, protowire.BytesType)
		fmt.Fprintf(fg.body, "\t}\n")

	case protoreflect.MessageKind:
		fmt.Fprintf(fg.body, "\t{\n")
		fmt.Fprintf(fg.body, "\t\tsize := %s.MarshalToSizedBuffer(dAtA[:i])\n", access)
		fmt.Fprintf(fg.body, "\t\tif size > 0 {\n")
		fmt.Fprintf(fg.body, "\t\t\ti -= size\n")
		fmt.Fprintf(fg.body, "\t\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(size))\n")
		fg.reverseTag("\t\t\t", num, protowire.BytesType)
		fmt.Fprintf(fg.body, "\t\t}\n")
		fmt.Fprintf(fg.body, "\t}\n")
	}
}

func (fg *FileGenerator) emitOptionalFieldMarshalReverse(access string, fd protoreflect.FieldDescriptor) {
	kind := fd.Kind()
	num := protowire.Number(fd.Number())
	fg.needsBinaryImport(kind)

	fmt.Fprintf(fg.body, "\tif %s != nil {\n", access)

	switch kind {
	case protoreflect.BoolKind:
		fmt.Fprintf(fg.body, "\t\ti--\n\t\tif *%s {\n\t\t\tdAtA[i] = 1\n\t\t} else {\n\t\t\tdAtA[i] = 0\n\t\t}\n", access)
		fg.reverseTag("\t\t", num, protowire.VarintType)

	case protoreflect.Int32Kind, protoreflect.Uint32Kind, protoreflect.Int64Kind, protoreflect.Uint64Kind, protoreflect.EnumKind:
		fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(*%s))\n", access)
		fg.reverseTag("\t\t", num, protowire.VarintType)

	case protoreflect.Sint32Kind:
		fmt.Fprintf(fg.body, "\t\tv := *%s\n", access)
		fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(uint32(v<<1)^uint32(int32(v)>>31)))\n")
		fg.reverseTag("\t\t", num, protowire.VarintType)

	case protoreflect.Sint64Kind:
		fmt.Fprintf(fg.body, "\t\tv := *%s\n", access)
		fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(uint64(v<<1)^uint64(int64(v)>>63)))\n")
		fg.reverseTag("\t\t", num, protowire.VarintType)

	case protoreflect.Fixed32Kind:
		fmt.Fprintf(fg.body, "\t\ti -= 4\n\t\tbinary.LittleEndian.PutUint32(dAtA[i:], *%s)\n", access)
		fg.reverseTag("\t\t", num, protowire.Fixed32Type)

	case protoreflect.Sfixed32Kind:
		fmt.Fprintf(fg.body, "\t\ti -= 4\n\t\tbinary.LittleEndian.PutUint32(dAtA[i:], uint32(*%s))\n", access)
		fg.reverseTag("\t\t", num, protowire.Fixed32Type)

	case protoreflect.FloatKind:
		fg.imports.addImport("math", "")
		fmt.Fprintf(fg.body, "\t\ti -= 4\n\t\tbinary.LittleEndian.PutUint32(dAtA[i:], math.Float32bits(*%s))\n", access)
		fg.reverseTag("\t\t", num, protowire.Fixed32Type)

	case protoreflect.Fixed64Kind:
		fmt.Fprintf(fg.body, "\t\ti -= 8\n\t\tbinary.LittleEndian.PutUint64(dAtA[i:], *%s)\n", access)
		fg.reverseTag("\t\t", num, protowire.Fixed64Type)

	case protoreflect.Sfixed64Kind:
		fmt.Fprintf(fg.body, "\t\ti -= 8\n\t\tbinary.LittleEndian.PutUint64(dAtA[i:], uint64(*%s))\n", access)
		fg.reverseTag("\t\t", num, protowire.Fixed64Type)

	case protoreflect.DoubleKind:
		fg.imports.addImport("math", "")
		fmt.Fprintf(fg.body, "\t\ti -= 8\n\t\tbinary.LittleEndian.PutUint64(dAtA[i:], math.Float64bits(*%s))\n", access)
		fg.reverseTag("\t\t", num, protowire.Fixed64Type)

	case protoreflect.StringKind:
		fmt.Fprintf(fg.body, "\t\ti -= len(*%s)\n\t\tcopy(dAtA[i:], *%s)\n", access, access)
		fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(len(*%s)))\n", access)
		fg.reverseTag("\t\t", num, protowire.BytesType)
	}

	fmt.Fprintf(fg.body, "\t}\n")
}

func (fg *FileGenerator) emitRepeatedFieldMarshalReverse(access string, fd protoreflect.FieldDescriptor) {
	kind := fd.Kind()
	num := protowire.Number(fd.Number())

	switch {
	case kind == protoreflect.MessageKind:
		fmt.Fprintf(fg.body, "\tfor iNdEx := len(%s) - 1; iNdEx >= 0; iNdEx-- {\n", access)
		fmt.Fprintf(fg.body, "\t\tsize := %s[iNdEx].MarshalToSizedBuffer(dAtA[:i])\n", access)
		fmt.Fprintf(fg.body, "\t\ti -= size\n")
		fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(size))\n")
		fg.reverseTag("\t\t", num, protowire.BytesType)
		fmt.Fprintf(fg.body, "\t}\n")

	case kind == protoreflect.StringKind:
		fmt.Fprintf(fg.body, "\tfor iNdEx := len(%s) - 1; iNdEx >= 0; iNdEx-- {\n", access)
		fmt.Fprintf(fg.body, "\t\ti -= len(%s[iNdEx])\n\t\tcopy(dAtA[i:], %s[iNdEx])\n", access, access)
		fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(len(%s[iNdEx])))\n", access)
		fg.reverseTag("\t\t", num, protowire.BytesType)
		fmt.Fprintf(fg.body, "\t}\n")

	case kind == protoreflect.BytesKind:
		fmt.Fprintf(fg.body, "\tfor iNdEx := len(%s) - 1; iNdEx >= 0; iNdEx-- {\n", access)
		fmt.Fprintf(fg.body, "\t\ti -= len(%s[iNdEx])\n\t\tcopy(dAtA[i:], %s[iNdEx])\n", access, access)
		fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(len(%s[iNdEx])))\n", access)
		fg.reverseTag("\t\t", num, protowire.BytesType)
		fmt.Fprintf(fg.body, "\t}\n")

	case isPackable(kind):
		fmt.Fprintf(fg.body, "\tif len(%s) > 0 {\n", access)

		if isFixed64Kind(kind) {
			fmt.Fprintf(fg.body, "\t\tfor iNdEx := len(%s) - 1; iNdEx >= 0; iNdEx-- {\n", access)
			fg.emitPackedElementReverse("\t\t\t", kind, access+"[iNdEx]")
			fmt.Fprintf(fg.body, "\t\t}\n")
			fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(len(%s)*8))\n", access)
		} else if isFixed32Kind(kind) {
			fmt.Fprintf(fg.body, "\t\tfor iNdEx := len(%s) - 1; iNdEx >= 0; iNdEx-- {\n", access)
			fg.emitPackedElementReverse("\t\t\t", kind, access+"[iNdEx]")
			fmt.Fprintf(fg.body, "\t\t}\n")
			fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(len(%s)*4))\n", access)
		} else {
			// Varint packed: write elements backwards, measure size from position difference
			fmt.Fprintf(fg.body, "\t\tvar j int\n")
			fmt.Fprintf(fg.body, "\t\tpStart := i\n")
			fmt.Fprintf(fg.body, "\t\tfor j = len(%s) - 1; j >= 0; j-- {\n", access)
			fg.emitPackedVarintElementReverse("\t\t\t", kind, access+"[j]")
			fmt.Fprintf(fg.body, "\t\t}\n")
			fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(pStart-i))\n")
		}

		fg.reverseTag("\t\t", num, protowire.BytesType)
		fmt.Fprintf(fg.body, "\t}\n")
	}
}

func (fg *FileGenerator) emitPackedElementReverse(indent string, kind protoreflect.Kind, access string) {
	fg.needsBinaryImport(kind)
	switch kind {
	case protoreflect.Fixed64Kind:
		fmt.Fprintf(fg.body, "%si -= 8\n%sbinary.LittleEndian.PutUint64(dAtA[i:], %s)\n", indent, indent, access)
	case protoreflect.Sfixed64Kind:
		fmt.Fprintf(fg.body, "%si -= 8\n%sbinary.LittleEndian.PutUint64(dAtA[i:], uint64(%s))\n", indent, indent, access)
	case protoreflect.DoubleKind:
		fg.imports.addImport("math", "")
		fmt.Fprintf(fg.body, "%si -= 8\n%sbinary.LittleEndian.PutUint64(dAtA[i:], math.Float64bits(%s))\n", indent, indent, access)
	case protoreflect.Fixed32Kind:
		fmt.Fprintf(fg.body, "%si -= 4\n%sbinary.LittleEndian.PutUint32(dAtA[i:], %s)\n", indent, indent, access)
	case protoreflect.Sfixed32Kind:
		fmt.Fprintf(fg.body, "%si -= 4\n%sbinary.LittleEndian.PutUint32(dAtA[i:], uint32(%s))\n", indent, indent, access)
	case protoreflect.FloatKind:
		fg.imports.addImport("math", "")
		fmt.Fprintf(fg.body, "%si -= 4\n%sbinary.LittleEndian.PutUint32(dAtA[i:], math.Float32bits(%s))\n", indent, indent, access)
	}
}

func (fg *FileGenerator) emitPackedVarintElementReverse(indent string, kind protoreflect.Kind, access string) {
	switch kind {
	case protoreflect.BoolKind:
		fmt.Fprintf(fg.body, "%si--\n%sif %s {\n%s\tdAtA[i] = 1\n%s} else {\n%s\tdAtA[i] = 0\n%s}\n",
			indent, indent, access, indent, indent, indent, indent)
	case protoreflect.Sint32Kind:
		fmt.Fprintf(fg.body, "%si = protohelpers.EncodeVarint(dAtA, i, uint64(uint32(%s<<1)^uint32(int32(%s)>>31)))\n",
			indent, access, access)
	case protoreflect.Sint64Kind:
		fmt.Fprintf(fg.body, "%si = protohelpers.EncodeVarint(dAtA, i, uint64(uint64(%s<<1)^uint64(int64(%s)>>63)))\n",
			indent, access, access)
	default:
		fmt.Fprintf(fg.body, "%si = protohelpers.EncodeVarint(dAtA, i, uint64(%s))\n", indent, access)
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
		fg.needsBinaryImport(fd.Kind())

		fmt.Fprintf(fg.body, "\tcase *%s:\n", variantName)

		switch fd.Kind() {
		case protoreflect.BoolKind:
			fmt.Fprintf(fg.body, "\t\ti--\n\t\tif %s {\n\t\t\tdAtA[i] = 1\n\t\t} else {\n\t\t\tdAtA[i] = 0\n\t\t}\n", access)
			fg.reverseTag("\t\t", num, protowire.VarintType)

		case protoreflect.Int32Kind, protoreflect.Uint32Kind, protoreflect.Int64Kind, protoreflect.Uint64Kind, protoreflect.EnumKind:
			fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(%s))\n", access)
			fg.reverseTag("\t\t", num, protowire.VarintType)

		case protoreflect.Sint32Kind:
			fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(uint32(%s<<1)^uint32(int32(%s)>>31)))\n", access, access)
			fg.reverseTag("\t\t", num, protowire.VarintType)

		case protoreflect.Sint64Kind:
			fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(uint64(%s<<1)^uint64(int64(%s)>>63)))\n", access, access)
			fg.reverseTag("\t\t", num, protowire.VarintType)

		case protoreflect.Fixed32Kind:
			fmt.Fprintf(fg.body, "\t\ti -= 4\n\t\tbinary.LittleEndian.PutUint32(dAtA[i:], %s)\n", access)
			fg.reverseTag("\t\t", num, protowire.Fixed32Type)

		case protoreflect.Sfixed32Kind:
			fmt.Fprintf(fg.body, "\t\ti -= 4\n\t\tbinary.LittleEndian.PutUint32(dAtA[i:], uint32(%s))\n", access)
			fg.reverseTag("\t\t", num, protowire.Fixed32Type)

		case protoreflect.FloatKind:
			fg.imports.addImport("math", "")
			fmt.Fprintf(fg.body, "\t\ti -= 4\n\t\tbinary.LittleEndian.PutUint32(dAtA[i:], math.Float32bits(%s))\n", access)
			fg.reverseTag("\t\t", num, protowire.Fixed32Type)

		case protoreflect.Fixed64Kind:
			fmt.Fprintf(fg.body, "\t\ti -= 8\n\t\tbinary.LittleEndian.PutUint64(dAtA[i:], %s)\n", access)
			fg.reverseTag("\t\t", num, protowire.Fixed64Type)

		case protoreflect.Sfixed64Kind:
			fmt.Fprintf(fg.body, "\t\ti -= 8\n\t\tbinary.LittleEndian.PutUint64(dAtA[i:], uint64(%s))\n", access)
			fg.reverseTag("\t\t", num, protowire.Fixed64Type)

		case protoreflect.DoubleKind:
			fg.imports.addImport("math", "")
			fmt.Fprintf(fg.body, "\t\ti -= 8\n\t\tbinary.LittleEndian.PutUint64(dAtA[i:], math.Float64bits(%s))\n", access)
			fg.reverseTag("\t\t", num, protowire.Fixed64Type)

		case protoreflect.StringKind:
			fmt.Fprintf(fg.body, "\t\ti -= len(%s)\n\t\tcopy(dAtA[i:], %s)\n", access, access)
			fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(len(%s)))\n", access)
			fg.reverseTag("\t\t", num, protowire.BytesType)

		case protoreflect.BytesKind:
			fmt.Fprintf(fg.body, "\t\ti -= len(%s)\n\t\tcopy(dAtA[i:], %s)\n", access, access)
			fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(len(%s)))\n", access)
			fg.reverseTag("\t\t", num, protowire.BytesType)

		case protoreflect.MessageKind:
			fmt.Fprintf(fg.body, "\t\tsize := %s.MarshalToSizedBuffer(dAtA[:i])\n", access)
			fmt.Fprintf(fg.body, "\t\ti -= size\n")
			fmt.Fprintf(fg.body, "\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(size))\n")
			fg.reverseTag("\t\t", num, protowire.BytesType)
		}
	}

	fmt.Fprintf(fg.body, "\t}\n")
}
