package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitSkipFieldHelper() {
	fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")
	fg.imports.addImport("fmt", "")

	fmt.Fprintf(fg.body, "func skipField(b []byte, num protowire.Number, typ protowire.Type) (int, error) {\n")
	fmt.Fprintf(fg.body, "\tswitch typ {\n")
	fmt.Fprintf(fg.body, "\tcase protowire.VarintType:\n")
	fmt.Fprintf(fg.body, "\t\t_, n := protowire.ConsumeVarint(b)\n")
	fmt.Fprintf(fg.body, "\t\tif n < 0 {\n\t\t\treturn 0, fmt.Errorf(\"invalid varint\")\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t\treturn n, nil\n")
	fmt.Fprintf(fg.body, "\tcase protowire.Fixed32Type:\n")
	fmt.Fprintf(fg.body, "\t\tif len(b) < 4 {\n\t\t\treturn 0, fmt.Errorf(\"truncated fixed32\")\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t\treturn 4, nil\n")
	fmt.Fprintf(fg.body, "\tcase protowire.Fixed64Type:\n")
	fmt.Fprintf(fg.body, "\t\tif len(b) < 8 {\n\t\t\treturn 0, fmt.Errorf(\"truncated fixed64\")\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t\treturn 8, nil\n")
	fmt.Fprintf(fg.body, "\tcase protowire.BytesType:\n")
	fmt.Fprintf(fg.body, "\t\t_, n := protowire.ConsumeBytes(b)\n")
	fmt.Fprintf(fg.body, "\t\tif n < 0 {\n\t\t\treturn 0, fmt.Errorf(\"invalid bytes\")\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t\treturn n, nil\n")
	fmt.Fprintf(fg.body, "\tcase protowire.StartGroupType:\n")
	fmt.Fprintf(fg.body, "\t\t_, n := protowire.ConsumeGroup(num, b)\n")
	fmt.Fprintf(fg.body, "\t\tif n < 0 {\n\t\t\treturn 0, fmt.Errorf(\"invalid group\")\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t\treturn n, nil\n")
	fmt.Fprintf(fg.body, "\tdefault:\n")
	fmt.Fprintf(fg.body, "\t\treturn 0, fmt.Errorf(\"unknown wire type %%d\", typ)\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

func (fg *FileGenerator) emitUnmarshal(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")
	fg.imports.addImport("fmt", "")

	fmt.Fprintf(fg.body, "func (m *%s) UnmarshalProto(b []byte) error {\n", name)
	fmt.Fprintf(fg.body, "\tfor len(b) > 0 {\n")
	fmt.Fprintf(fg.body, "\t\tnum, typ, tagLen := protowire.ConsumeTag(b)\n")
	fmt.Fprintf(fg.body, "\t\tif tagLen < 0 {\n\t\t\treturn fmt.Errorf(\"invalid tag\")\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t\tb = b[tagLen:]\n")
	fmt.Fprintf(fg.body, "\t\tswitch num {\n")

	// Collect all field numbers including oneof fields
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		fg.emitFieldUnmarshal(md, fd)
	}

	fmt.Fprintf(fg.body, "\t\tdefault:\n")
	fmt.Fprintf(fg.body, "\t\t\tn, err := skipField(b, num, typ)\n")
	fmt.Fprintf(fg.body, "\t\t\tif err != nil {\n\t\t\t\treturn err\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tb = b[n:]\n")
	fmt.Fprintf(fg.body, "\t\t}\n") // end switch
	fmt.Fprintf(fg.body, "\t}\n")   // end for
	fmt.Fprintf(fg.body, "\treturn nil\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

func (fg *FileGenerator) emitFieldUnmarshal(md protoreflect.MessageDescriptor, fd protoreflect.FieldDescriptor) {
	goName := snakeToPascal(string(fd.Name()))
	fieldNum := fd.Number()
	kind := fd.Kind()

	fmt.Fprintf(fg.body, "\t\tcase %d: // %s\n", fieldNum, fd.Name())

	if fd.IsList() {
		fg.emitRepeatedFieldUnmarshal(goName, fd)
		return
	}

	if isRealOneof(fd) {
		fg.emitOneofFieldUnmarshal(md, fd)
		return
	}

	if fd.HasOptionalKeyword() {
		fg.emitOptionalFieldUnmarshal(goName, fd)
		return
	}

	fg.emitSingularFieldUnmarshal(goName, fd, kind)
}

func (fg *FileGenerator) emitSingularFieldUnmarshal(goName string, fd protoreflect.FieldDescriptor, kind protoreflect.Kind) {
	access := "m." + goName

	switch kind {
	case protoreflect.BoolKind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = v != 0\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Int32Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = int32(v)\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Sint32Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = int32(protowire.DecodeZigZag(v))\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Uint32Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = uint32(v)\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Int64Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = int64(v)\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Sint64Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = int64(protowire.DecodeZigZag(v))\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Uint64Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = v\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.EnumKind:
		fg.emitConsumeVarint()
		enumType := fg.imports.goEnumType(fd.Enum())
		fmt.Fprintf(fg.body, "\t\t\t%s = %s(v)\n", access, enumType)
		fg.emitAdvanceBytes()

	case protoreflect.Fixed32Kind:
		fg.emitConsumeFixed32()
		fmt.Fprintf(fg.body, "\t\t\t%s = v\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Sfixed32Kind:
		fg.emitConsumeFixed32()
		fmt.Fprintf(fg.body, "\t\t\t%s = int32(v)\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.FloatKind:
		fg.imports.addImport("math", "")
		fg.emitConsumeFixed32()
		fmt.Fprintf(fg.body, "\t\t\t%s = math.Float32frombits(v)\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Fixed64Kind:
		fg.emitConsumeFixed64()
		fmt.Fprintf(fg.body, "\t\t\t%s = v\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Sfixed64Kind:
		fg.emitConsumeFixed64()
		fmt.Fprintf(fg.body, "\t\t\t%s = int64(v)\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.DoubleKind:
		fg.imports.addImport("math", "")
		fg.emitConsumeFixed64()
		fmt.Fprintf(fg.body, "\t\t\t%s = math.Float64frombits(v)\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.StringKind:
		fg.emitConsumeString()
		fmt.Fprintf(fg.body, "\t\t\t%s = v\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.BytesKind:
		fg.emitConsumeBytes()
		fmt.Fprintf(fg.body, "\t\t\t%s = append(%s[:0], v...)\n", access, access)
		fg.emitAdvanceBytes()

	case protoreflect.MessageKind:
		msgType := fg.imports.goMessageType(fd.Message())
		_ = msgType
		fg.emitConsumeBytes()
		fmt.Fprintf(fg.body, "\t\t\tif err := %s.UnmarshalProto(v); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", access)
		fg.emitAdvanceBytes()
	}
}

func (fg *FileGenerator) emitOptionalFieldUnmarshal(goName string, fd protoreflect.FieldDescriptor) {
	access := "m." + goName
	kind := fd.Kind()

	switch kind {
	case protoreflect.BoolKind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\ttmp := v != 0\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Int32Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\ttmp := int32(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Sint32Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\ttmp := int32(protowire.DecodeZigZag(v))\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Uint32Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\ttmp := uint32(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Int64Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\ttmp := int64(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Sint64Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\ttmp := int64(protowire.DecodeZigZag(v))\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Uint64Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\ttmp := v\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Fixed32Kind:
		fg.emitConsumeFixed32()
		fmt.Fprintf(fg.body, "\t\t\t%s = &v\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Sfixed32Kind:
		fg.emitConsumeFixed32()
		fmt.Fprintf(fg.body, "\t\t\ttmp := int32(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.FloatKind:
		fg.imports.addImport("math", "")
		fg.emitConsumeFixed32()
		fmt.Fprintf(fg.body, "\t\t\ttmp := math.Float32frombits(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Fixed64Kind:
		fg.emitConsumeFixed64()
		fmt.Fprintf(fg.body, "\t\t\t%s = &v\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Sfixed64Kind:
		fg.emitConsumeFixed64()
		fmt.Fprintf(fg.body, "\t\t\ttmp := int64(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.DoubleKind:
		fg.imports.addImport("math", "")
		fg.emitConsumeFixed64()
		fmt.Fprintf(fg.body, "\t\t\ttmp := math.Float64frombits(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.StringKind:
		fg.emitConsumeString()
		fmt.Fprintf(fg.body, "\t\t\t%s = &v\n", access)
		fg.emitAdvanceBytes()
	}
}

func (fg *FileGenerator) emitRepeatedFieldUnmarshal(goName string, fd protoreflect.FieldDescriptor) {
	access := "m." + goName
	kind := fd.Kind()

	switch {
	case kind == protoreflect.MessageKind:
		msgType := fg.imports.goSingularType(fd)
		fg.emitConsumeBytes()
		fmt.Fprintf(fg.body, "\t\t\tvar elem %s\n", msgType)
		fmt.Fprintf(fg.body, "\t\t\tif err := elem.UnmarshalProto(v); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = append(%s, elem)\n", access, access)
		fg.emitAdvanceBytes()

	case kind == protoreflect.StringKind:
		fg.emitConsumeString()
		fmt.Fprintf(fg.body, "\t\t\t%s = append(%s, v)\n", access, access)
		fg.emitAdvanceBytes()

	case kind == protoreflect.BytesKind:
		fg.emitConsumeBytes()
		fmt.Fprintf(fg.body, "\t\t\t%s = append(%s, append([]byte(nil), v...))\n", access, access)
		fg.emitAdvanceBytes()

	case isPackable(kind):
		fg.emitPackedFieldUnmarshal(access, fd, kind)
	}
}

func (fg *FileGenerator) emitPackedFieldUnmarshal(access string, fd protoreflect.FieldDescriptor, kind protoreflect.Kind) {
	// Handle both packed and unpacked encoding
	fmt.Fprintf(fg.body, "\t\t\tif typ == protowire.BytesType {\n")
	fmt.Fprintf(fg.body, "\t\t\t\tdata, n := protowire.ConsumeBytes(b)\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif n < 0 {\n\t\t\t\t\treturn fmt.Errorf(\"invalid packed field\")\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tfor len(data) > 0 {\n")

	switch {
	case isFixed64Kind(kind):
		fmt.Fprintf(fg.body, "\t\t\t\t\tv, vn := protowire.ConsumeFixed64(data)\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif vn < 0 {\n\t\t\t\t\t\treturn fmt.Errorf(\"invalid packed fixed64\")\n\t\t\t\t\t}\n")
		fg.emitPackedElementDecode(access, fd, kind, "v")
		fmt.Fprintf(fg.body, "\t\t\t\t\tdata = data[vn:]\n")
	case isFixed32Kind(kind):
		fmt.Fprintf(fg.body, "\t\t\t\t\tv, vn := protowire.ConsumeFixed32(data)\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif vn < 0 {\n\t\t\t\t\t\treturn fmt.Errorf(\"invalid packed fixed32\")\n\t\t\t\t\t}\n")
		fg.emitPackedElementDecode(access, fd, kind, "v")
		fmt.Fprintf(fg.body, "\t\t\t\t\tdata = data[vn:]\n")
	default: // varint
		fmt.Fprintf(fg.body, "\t\t\t\t\tv, vn := protowire.ConsumeVarint(data)\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif vn < 0 {\n\t\t\t\t\t\treturn fmt.Errorf(\"invalid packed varint\")\n\t\t\t\t\t}\n")
		fg.emitPackedElementDecode(access, fd, kind, "v")
		fmt.Fprintf(fg.body, "\t\t\t\t\tdata = data[vn:]\n")
	}

	fmt.Fprintf(fg.body, "\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tb = b[n:]\n")
	fmt.Fprintf(fg.body, "\t\t\t} else {\n")

	// Non-packed: single element
	switch {
	case isFixed64Kind(kind):
		fmt.Fprintf(fg.body, "\t\t\t\tv, n := protowire.ConsumeFixed64(b)\n")
		fmt.Fprintf(fg.body, "\t\t\t\tif n < 0 {\n\t\t\t\t\treturn fmt.Errorf(\"invalid fixed64\")\n\t\t\t\t}\n")
		fg.emitPackedElementDecode(access, fd, kind, "v")
		fmt.Fprintf(fg.body, "\t\t\t\tb = b[n:]\n")
	case isFixed32Kind(kind):
		fmt.Fprintf(fg.body, "\t\t\t\tv, n := protowire.ConsumeFixed32(b)\n")
		fmt.Fprintf(fg.body, "\t\t\t\tif n < 0 {\n\t\t\t\t\treturn fmt.Errorf(\"invalid fixed32\")\n\t\t\t\t}\n")
		fg.emitPackedElementDecode(access, fd, kind, "v")
		fmt.Fprintf(fg.body, "\t\t\t\tb = b[n:]\n")
	default:
		fmt.Fprintf(fg.body, "\t\t\t\tv, n := protowire.ConsumeVarint(b)\n")
		fmt.Fprintf(fg.body, "\t\t\t\tif n < 0 {\n\t\t\t\t\treturn fmt.Errorf(\"invalid varint\")\n\t\t\t\t}\n")
		fg.emitPackedElementDecode(access, fd, kind, "v")
		fmt.Fprintf(fg.body, "\t\t\t\tb = b[n:]\n")
	}

	fmt.Fprintf(fg.body, "\t\t\t}\n")
}

func (fg *FileGenerator) emitPackedElementDecode(access string, fd protoreflect.FieldDescriptor, kind protoreflect.Kind, varName string) {
	switch kind {
	case protoreflect.BoolKind:
		fmt.Fprintf(fg.body, "\t\t\t\t%s = append(%s, %s != 0)\n", access, access, varName)
	case protoreflect.Int32Kind:
		fmt.Fprintf(fg.body, "\t\t\t\t%s = append(%s, int32(%s))\n", access, access, varName)
	case protoreflect.Sint32Kind:
		fmt.Fprintf(fg.body, "\t\t\t\t%s = append(%s, int32(protowire.DecodeZigZag(%s)))\n", access, access, varName)
	case protoreflect.Uint32Kind:
		fmt.Fprintf(fg.body, "\t\t\t\t%s = append(%s, uint32(%s))\n", access, access, varName)
	case protoreflect.Int64Kind:
		fmt.Fprintf(fg.body, "\t\t\t\t%s = append(%s, int64(%s))\n", access, access, varName)
	case protoreflect.Sint64Kind:
		fmt.Fprintf(fg.body, "\t\t\t\t%s = append(%s, int64(protowire.DecodeZigZag(%s)))\n", access, access, varName)
	case protoreflect.Uint64Kind:
		fmt.Fprintf(fg.body, "\t\t\t\t%s = append(%s, %s)\n", access, access, varName)
	case protoreflect.EnumKind:
		enumType := fg.imports.goEnumType(fd.Enum())
		fmt.Fprintf(fg.body, "\t\t\t\t%s = append(%s, %s(%s))\n", access, access, enumType, varName)
	case protoreflect.Fixed32Kind:
		fmt.Fprintf(fg.body, "\t\t\t\t%s = append(%s, %s)\n", access, access, varName)
	case protoreflect.Sfixed32Kind:
		fmt.Fprintf(fg.body, "\t\t\t\t%s = append(%s, int32(%s))\n", access, access, varName)
	case protoreflect.FloatKind:
		fg.imports.addImport("math", "")
		fmt.Fprintf(fg.body, "\t\t\t\t%s = append(%s, math.Float32frombits(%s))\n", access, access, varName)
	case protoreflect.Fixed64Kind:
		fmt.Fprintf(fg.body, "\t\t\t\t%s = append(%s, %s)\n", access, access, varName)
	case protoreflect.Sfixed64Kind:
		fmt.Fprintf(fg.body, "\t\t\t\t%s = append(%s, int64(%s))\n", access, access, varName)
	case protoreflect.DoubleKind:
		fg.imports.addImport("math", "")
		fmt.Fprintf(fg.body, "\t\t\t\t%s = append(%s, math.Float64frombits(%s))\n", access, access, varName)
	}
}

func (fg *FileGenerator) emitOneofFieldUnmarshal(md protoreflect.MessageDescriptor, fd protoreflect.FieldDescriptor) {
	oo := fd.ContainingOneof()
	ooFieldName := snakeToPascal(string(oo.Name()))
	variantName := oneofVariantName(md, fd)
	fieldName := snakeToPascal(string(fd.Name()))
	kind := fd.Kind()

	switch kind {
	case protoreflect.BoolKind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: v != 0}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()

	case protoreflect.Int32Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: int32(v)}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()

	case protoreflect.Sint32Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: int32(protowire.DecodeZigZag(v))}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()

	case protoreflect.Uint32Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: uint32(v)}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()

	case protoreflect.Int64Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: int64(v)}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()

	case protoreflect.Sint64Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: int64(protowire.DecodeZigZag(v))}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()

	case protoreflect.Uint64Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: v}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()

	case protoreflect.EnumKind:
		enumType := fg.imports.goEnumType(fd.Enum())
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: %s(v)}\n", ooFieldName, variantName, fieldName, enumType)
		fg.emitAdvanceBytes()

	case protoreflect.Fixed32Kind:
		fg.emitConsumeFixed32()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: v}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()

	case protoreflect.Sfixed32Kind:
		fg.emitConsumeFixed32()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: int32(v)}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()

	case protoreflect.FloatKind:
		fg.imports.addImport("math", "")
		fg.emitConsumeFixed32()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: math.Float32frombits(v)}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()

	case protoreflect.Fixed64Kind:
		fg.emitConsumeFixed64()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: v}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()

	case protoreflect.Sfixed64Kind:
		fg.emitConsumeFixed64()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: int64(v)}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()

	case protoreflect.DoubleKind:
		fg.imports.addImport("math", "")
		fg.emitConsumeFixed64()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: math.Float64frombits(v)}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()

	case protoreflect.StringKind:
		fg.emitConsumeString()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: v}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()

	case protoreflect.BytesKind:
		fg.emitConsumeBytes()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: append([]byte(nil), v...)}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()

	case protoreflect.MessageKind:
		fg.emitConsumeBytes()
		msgType := fg.imports.goSingularType(fd)
		fmt.Fprintf(fg.body, "\t\t\tvar msg %s\n", msgType)
		fmt.Fprintf(fg.body, "\t\t\tif err := msg.UnmarshalProto(v); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: msg}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceBytes()
	}
}

// Helper methods for consuming wire values

func (fg *FileGenerator) emitConsumeVarint() {
	fmt.Fprintf(fg.body, "\t\t\tv, n := protowire.ConsumeVarint(b)\n")
	fmt.Fprintf(fg.body, "\t\t\tif n < 0 {\n\t\t\t\treturn fmt.Errorf(\"invalid varint\")\n\t\t\t}\n")
}

func (fg *FileGenerator) emitConsumeFixed32() {
	fmt.Fprintf(fg.body, "\t\t\tv, n := protowire.ConsumeFixed32(b)\n")
	fmt.Fprintf(fg.body, "\t\t\tif n < 0 {\n\t\t\t\treturn fmt.Errorf(\"invalid fixed32\")\n\t\t\t}\n")
}

func (fg *FileGenerator) emitConsumeFixed64() {
	fmt.Fprintf(fg.body, "\t\t\tv, n := protowire.ConsumeFixed64(b)\n")
	fmt.Fprintf(fg.body, "\t\t\tif n < 0 {\n\t\t\t\treturn fmt.Errorf(\"invalid fixed64\")\n\t\t\t}\n")
}

func (fg *FileGenerator) emitConsumeBytes() {
	fmt.Fprintf(fg.body, "\t\t\tv, n := protowire.ConsumeBytes(b)\n")
	fmt.Fprintf(fg.body, "\t\t\tif n < 0 {\n\t\t\t\treturn fmt.Errorf(\"invalid bytes\")\n\t\t\t}\n")
}

func (fg *FileGenerator) emitConsumeString() {
	fmt.Fprintf(fg.body, "\t\t\tv, n := protowire.ConsumeString(b)\n")
	fmt.Fprintf(fg.body, "\t\t\tif n < 0 {\n\t\t\t\treturn fmt.Errorf(\"invalid string\")\n\t\t\t}\n")
}

func (fg *FileGenerator) emitAdvanceBytes() {
	fmt.Fprintf(fg.body, "\t\t\tb = b[n:]\n")
}
