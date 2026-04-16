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

// repeatedFieldsForPreScan returns list fields whose element count can be
// determined by counting wire-format field-number occurrences (messages,
// strings, bytes). Packed scalars are excluded because one wire occurrence
// contains many elements.
func repeatedFieldsForPreScan(md protoreflect.MessageDescriptor) []protoreflect.FieldDescriptor {
	var fields []protoreflect.FieldDescriptor
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if !fd.IsList() {
			continue
		}
		switch fd.Kind() {
		case protoreflect.MessageKind, protoreflect.StringKind, protoreflect.BytesKind:
			fields = append(fields, fd)
		}
	}
	return fields
}

// preScanMinBytes is the minimum message size for the pre-scan to run.
// Small messages (individual spans, data points) have few repeated elements
// so the scanning overhead outweighs the pre-allocation savings. Large
// container messages (ScopeSpans with many spans) benefit significantly.
const preScanMinBytes = 256

// emitPreScan emits a lightweight tag-scanning loop that counts occurrences of
// repeated message/string/bytes fields, then pre-allocates their slices with
// exact capacity. This avoids expensive realloc+copy cycles for value-type
// slices during the main unmarshal loop. Gated on len(dAtA) to skip small messages
// where the overhead isn't justified.
func (fg *FileGenerator) emitPreScan(md protoreflect.MessageDescriptor) {
	fields := repeatedFieldsForPreScan(md)
	if len(fields) == 0 {
		return
	}

	fmt.Fprintf(fg.body, "\tif len(dAtA) >= %d {\n", preScanMinBytes)
	fmt.Fprintf(fg.body, "\t\ttmp := dAtA\n")
	for _, fd := range fields {
		fmt.Fprintf(fg.body, "\t\tvar field%dcount int\n", fd.Number())
	}
	fmt.Fprintf(fg.body, "\t\tfor len(tmp) > 0 {\n")
	fmt.Fprintf(fg.body, "\t\t\tnum, typ, tagLen := protowire.ConsumeTag(tmp)\n")
	fmt.Fprintf(fg.body, "\t\t\tif tagLen < 0 {\n\t\t\t\tbreak\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\ttmp = tmp[tagLen:]\n")

	fmt.Fprintf(fg.body, "\t\t\tswitch num {\n")
	for _, fd := range fields {
		fmt.Fprintf(fg.body, "\t\t\tcase %d:\n", fd.Number())
		fmt.Fprintf(fg.body, "\t\t\t\tfield%dcount++\n", fd.Number())
	}
	fmt.Fprintf(fg.body, "\t\t\t}\n")

	fmt.Fprintf(fg.body, "\t\t\tvar skip int\n")
	fmt.Fprintf(fg.body, "\t\t\tswitch typ {\n")
	fmt.Fprintf(fg.body, "\t\t\tcase protowire.VarintType:\n")
	fmt.Fprintf(fg.body, "\t\t\t\t_, skip = protowire.ConsumeVarint(tmp)\n")
	fmt.Fprintf(fg.body, "\t\t\tcase protowire.Fixed32Type:\n")
	fmt.Fprintf(fg.body, "\t\t\t\tskip = 4\n")
	fmt.Fprintf(fg.body, "\t\t\tcase protowire.Fixed64Type:\n")
	fmt.Fprintf(fg.body, "\t\t\t\tskip = 8\n")
	fmt.Fprintf(fg.body, "\t\t\tcase protowire.BytesType:\n")
	fmt.Fprintf(fg.body, "\t\t\t\t_, skip = protowire.ConsumeBytes(tmp)\n")
	fmt.Fprintf(fg.body, "\t\t\tcase protowire.StartGroupType:\n")
	fmt.Fprintf(fg.body, "\t\t\t\t_, skip = protowire.ConsumeGroup(num, tmp)\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tif skip < 0 || skip > len(tmp) {\n\t\t\t\tbreak\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\ttmp = tmp[skip:]\n")
	fmt.Fprintf(fg.body, "\t\t}\n")

	for _, fd := range fields {
		goName := snakeToPascal(string(fd.Name()))
		sliceType := fg.imports.goType(fd)
		fmt.Fprintf(fg.body, "\t\tif field%dcount > 0 {\n", fd.Number())
		fmt.Fprintf(fg.body, "\t\t\tm.%s = make(%s, 0, field%dcount)\n", goName, sliceType, fd.Number())
		fmt.Fprintf(fg.body, "\t\t}\n")
	}

	fmt.Fprintf(fg.body, "\t}\n")
}

func (fg *FileGenerator) emitUnmarshal(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")
	fg.imports.addImport("io", "")

	fmt.Fprintf(fg.body, "func (m *%s) Unmarshal(dAtA []byte) error {\n", name)
	fg.emitPreScan(md)

	// Index-based main loop with inlined tag decoding
	fmt.Fprintf(fg.body, "\tl := len(dAtA)\n")
	fmt.Fprintf(fg.body, "\tiNdEx := 0\n")
	fmt.Fprintf(fg.body, "\tfor iNdEx < l {\n")

	// Inline tag varint decode
	fmt.Fprintf(fg.body, "\t\tvar wire uint64\n")
	fmt.Fprintf(fg.body, "\t\tfor shift := uint(0); ; shift += 7 {\n")
	fmt.Fprintf(fg.body, "\t\t\tif shift >= 64 {\n\t\t\t\treturn protohelpers.ErrIntOverflow\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tif iNdEx >= l {\n\t\t\t\treturn io.ErrUnexpectedEOF\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tb := dAtA[iNdEx]\n")
	fmt.Fprintf(fg.body, "\t\t\tiNdEx++\n")
	fmt.Fprintf(fg.body, "\t\t\twire |= uint64(b&0x7F) << shift\n")
	fmt.Fprintf(fg.body, "\t\t\tif b < 0x80 {\n\t\t\t\tbreak\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t}\n")
	fmt.Fprintf(fg.body, "\t\tfieldNum := int32(wire >> 3)\n")
	fmt.Fprintf(fg.body, "\t\twireType := int(wire & 0x7)\n")
	fmt.Fprintf(fg.body, "\t\tif fieldNum == 0 {\n\t\t\treturn fmt.Errorf(\"proto: illegal tag 0\")\n\t\t}\n")

	fmt.Fprintf(fg.body, "\t\tswitch fieldNum {\n")

	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		fg.emitFieldUnmarshal(md, fd)
	}

	fmt.Fprintf(fg.body, "\t\tdefault:\n")
	fmt.Fprintf(fg.body, "\t\t\tn, err := skipField(dAtA[iNdEx:], protowire.Number(fieldNum), protowire.Type(wireType))\n")
	fmt.Fprintf(fg.body, "\t\t\tif err != nil {\n\t\t\t\treturn err\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tiNdEx += n\n")
	fmt.Fprintf(fg.body, "\t\t}\n") // end switch
	fmt.Fprintf(fg.body, "\t}\n")   // end for
	fmt.Fprintf(fg.body, "\tif iNdEx > l {\n\t\treturn io.ErrUnexpectedEOF\n\t}\n")
	fmt.Fprintf(fg.body, "\treturn nil\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

func (fg *FileGenerator) emitFieldUnmarshal(md protoreflect.MessageDescriptor, fd protoreflect.FieldDescriptor) {
	goName := snakeToPascal(string(fd.Name()))
	fieldNum := fd.Number()
	kind := fd.Kind()

	fmt.Fprintf(fg.body, "\t\tcase %d: // %s\n", fieldNum, fd.Name())

	// Packed repeated fields handle wire type dispatch internally
	// (packed=BytesType vs unpacked=native type).
	if fd.IsList() && isPackable(kind) {
		fg.emitRepeatedFieldUnmarshal(goName, fd)
		return
	}

	// All other fields have exactly one expected wire type.
	fg.emitWireTypeCheck(kind)

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

// wireTypeInt returns the protobuf wire type number for a given proto kind.
func wireTypeInt(kind protoreflect.Kind) int {
	switch kind {
	case protoreflect.BoolKind, protoreflect.EnumKind,
		protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Uint32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Uint64Kind:
		return 0 // VarintType
	case protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind, protoreflect.FloatKind:
		return 5 // Fixed32Type
	case protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind, protoreflect.DoubleKind:
		return 1 // Fixed64Type
	case protoreflect.StringKind, protoreflect.BytesKind, protoreflect.MessageKind:
		return 2 // BytesType
	default:
		return 2
	}
}

// emitWireTypeCheck emits a check that the wire type matches the expected type
// for a given proto kind, skipping the field if it doesn't match.
func (fg *FileGenerator) emitWireTypeCheck(kind protoreflect.Kind) {
	wt := wireTypeInt(kind)
	fmt.Fprintf(fg.body, "\t\t\tif wireType != %d {\n", wt)
	fmt.Fprintf(fg.body, "\t\t\t\tn, err := skipField(dAtA[iNdEx:], protowire.Number(fieldNum), protowire.Type(wireType))\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif err != nil {\n\t\t\t\t\treturn err\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tiNdEx += n\n")
	fmt.Fprintf(fg.body, "\t\t\t\tcontinue\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
}

func (fg *FileGenerator) emitSingularFieldUnmarshal(goName string, fd protoreflect.FieldDescriptor, kind protoreflect.Kind) {
	access := "m." + goName

	switch kind {
	case protoreflect.BoolKind:
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = v != 0\n", access)

	case protoreflect.Int32Kind:
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = int32(v)\n", access)

	case protoreflect.Sint32Kind:
		fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = int32(protowire.DecodeZigZag(v))\n", access)

	case protoreflect.Uint32Kind:
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = uint32(v)\n", access)

	case protoreflect.Int64Kind:
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = int64(v)\n", access)

	case protoreflect.Sint64Kind:
		fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = int64(protowire.DecodeZigZag(v))\n", access)

	case protoreflect.Uint64Kind:
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = v\n", access)

	case protoreflect.EnumKind:
		fg.emitInlineVarint()
		enumType := fg.imports.goEnumType(fd.Enum())
		fmt.Fprintf(fg.body, "\t\t\t%s = %s(v)\n", access, enumType)

	case protoreflect.Fixed32Kind:
		fg.emitInlineFixed32()
		fmt.Fprintf(fg.body, "\t\t\t%s = v\n", access)

	case protoreflect.Sfixed32Kind:
		fg.emitInlineFixed32()
		fmt.Fprintf(fg.body, "\t\t\t%s = int32(v)\n", access)

	case protoreflect.FloatKind:
		fg.imports.addImport("math", "")
		fg.emitInlineFixed32()
		fmt.Fprintf(fg.body, "\t\t\t%s = math.Float32frombits(v)\n", access)

	case protoreflect.Fixed64Kind:
		fg.emitInlineFixed64()
		fmt.Fprintf(fg.body, "\t\t\t%s = v\n", access)

	case protoreflect.Sfixed64Kind:
		fg.emitInlineFixed64()
		fmt.Fprintf(fg.body, "\t\t\t%s = int64(v)\n", access)

	case protoreflect.DoubleKind:
		fg.imports.addImport("math", "")
		fg.emitInlineFixed64()
		fmt.Fprintf(fg.body, "\t\t\t%s = math.Float64frombits(v)\n", access)

	case protoreflect.StringKind:
		fg.emitInlineBytesLen()
		fmt.Fprintf(fg.body, "\t\t\t%s = string(dAtA[iNdEx:postIndex])\n", access)
		fg.emitAdvanceToPostIndex()

	case protoreflect.BytesKind:
		fg.emitInlineBytesLen()
		fmt.Fprintf(fg.body, "\t\t\t%s = append(%s[:0], dAtA[iNdEx:postIndex]...)\n", access, access)
		fg.emitAdvanceToPostIndex()

	case protoreflect.MessageKind:
		fg.emitInlineBytesLen()
		fmt.Fprintf(fg.body, "\t\t\tif err := %s.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", access)
		fg.emitAdvanceToPostIndex()
	}
}

func (fg *FileGenerator) emitOptionalFieldUnmarshal(goName string, fd protoreflect.FieldDescriptor) {
	access := "m." + goName
	kind := fd.Kind()

	switch kind {
	case protoreflect.BoolKind:
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\ttmp := v != 0\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)

	case protoreflect.Int32Kind:
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\ttmp := int32(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)

	case protoreflect.Sint32Kind:
		fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\ttmp := int32(protowire.DecodeZigZag(v))\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)

	case protoreflect.Uint32Kind:
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\ttmp := uint32(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)

	case protoreflect.Int64Kind:
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\ttmp := int64(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)

	case protoreflect.Sint64Kind:
		fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\ttmp := int64(protowire.DecodeZigZag(v))\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)

	case protoreflect.Uint64Kind:
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\ttmp := v\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)

	case protoreflect.Fixed32Kind:
		fg.emitInlineFixed32()
		fmt.Fprintf(fg.body, "\t\t\t%s = &v\n", access)

	case protoreflect.Sfixed32Kind:
		fg.emitInlineFixed32()
		fmt.Fprintf(fg.body, "\t\t\ttmp := int32(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)

	case protoreflect.FloatKind:
		fg.imports.addImport("math", "")
		fg.emitInlineFixed32()
		fmt.Fprintf(fg.body, "\t\t\ttmp := math.Float32frombits(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)

	case protoreflect.Fixed64Kind:
		fg.emitInlineFixed64()
		fmt.Fprintf(fg.body, "\t\t\t%s = &v\n", access)

	case protoreflect.Sfixed64Kind:
		fg.emitInlineFixed64()
		fmt.Fprintf(fg.body, "\t\t\ttmp := int64(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)

	case protoreflect.DoubleKind:
		fg.imports.addImport("math", "")
		fg.emitInlineFixed64()
		fmt.Fprintf(fg.body, "\t\t\ttmp := math.Float64frombits(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)

	case protoreflect.StringKind:
		fg.emitInlineBytesLen()
		fmt.Fprintf(fg.body, "\t\t\ttmp := string(dAtA[iNdEx:postIndex])\n")
		fmt.Fprintf(fg.body, "\t\t\t%s = &tmp\n", access)
		fg.emitAdvanceToPostIndex()
	}
}

func (fg *FileGenerator) emitRepeatedFieldUnmarshal(goName string, fd protoreflect.FieldDescriptor) {
	access := "m." + goName
	kind := fd.Kind()

	switch {
	case kind == protoreflect.MessageKind:
		msgType := fg.imports.goSingularType(fd)
		fg.emitInlineBytesLen()
		fmt.Fprintf(fg.body, "\t\t\t%s = append(%s, %s{})\n", access, access, msgType)
		fmt.Fprintf(fg.body, "\t\t\tif err := %s[len(%s)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", access, access)
		fg.emitAdvanceToPostIndex()

	case kind == protoreflect.StringKind:
		fg.emitInlineBytesLen()
		fmt.Fprintf(fg.body, "\t\t\t%s = append(%s, string(dAtA[iNdEx:postIndex]))\n", access, access)
		fg.emitAdvanceToPostIndex()

	case kind == protoreflect.BytesKind:
		fg.emitInlineBytesLen()
		fmt.Fprintf(fg.body, "\t\t\t%s = append(%s, append([]byte(nil), dAtA[iNdEx:postIndex]...))\n", access, access)
		fg.emitAdvanceToPostIndex()

	case isPackable(kind):
		fg.emitPackedFieldUnmarshal(access, fd, kind)
	}
}

func (fg *FileGenerator) emitPackedFieldUnmarshal(access string, fd protoreflect.FieldDescriptor, kind protoreflect.Kind) {
	sliceType := fg.imports.goType(fd)

	// Packed encoding (wireType == 2)
	fmt.Fprintf(fg.body, "\t\t\tif wireType == 2 {\n")

	// Inline length varint
	fmt.Fprintf(fg.body, "\t\t\t\tvar packedLen int\n")
	fmt.Fprintf(fg.body, "\t\t\t\tfor shift := uint(0); ; shift += 7 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tif shift >= 64 {\n\t\t\t\t\t\treturn protohelpers.ErrIntOverflow\n\t\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tif iNdEx >= l {\n\t\t\t\t\t\treturn io.ErrUnexpectedEOF\n\t\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tb := dAtA[iNdEx]\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tiNdEx++\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tpackedLen |= int(b&0x7F) << shift\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tif b < 0x80 {\n\t\t\t\t\t\tbreak\n\t\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif packedLen < 0 {\n\t\t\t\t\treturn protohelpers.ErrInvalidLength\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tpostIndex := iNdEx + packedLen\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif postIndex < 0 {\n\t\t\t\t\treturn protohelpers.ErrInvalidLength\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif postIndex > l {\n\t\t\t\t\treturn io.ErrUnexpectedEOF\n\t\t\t\t}\n")

	// Pre-allocate with exact capacity
	if isFixed64Kind(kind) {
		fmt.Fprintf(fg.body, "\t\t\t\tif elementCount := (postIndex - iNdEx) / 8; elementCount != 0 && len(%s) == 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\t\t\t\t%s = make(%s, 0, elementCount)\n", access, sliceType)
		fmt.Fprintf(fg.body, "\t\t\t\t}\n")
	} else if isFixed32Kind(kind) {
		fmt.Fprintf(fg.body, "\t\t\t\tif elementCount := (postIndex - iNdEx) / 4; elementCount != 0 && len(%s) == 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\t\t\t\t%s = make(%s, 0, elementCount)\n", access, sliceType)
		fmt.Fprintf(fg.body, "\t\t\t\t}\n")
	} else if kind == protoreflect.BoolKind {
		fmt.Fprintf(fg.body, "\t\t\t\tif elementCount := postIndex - iNdEx; elementCount != 0 && len(%s) == 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\t\t\t\t%s = make(%s, 0, elementCount)\n", access, sliceType)
		fmt.Fprintf(fg.body, "\t\t\t\t}\n")
	} else {
		// Varint kinds: count elements by scanning for terminator bytes (< 128)
		fmt.Fprintf(fg.body, "\t\t\t\tvar elementCount int\n")
		fmt.Fprintf(fg.body, "\t\t\t\tfor _, b := range dAtA[iNdEx:postIndex] {\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif b < 128 {\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\t\telementCount++\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\tif elementCount != 0 && len(%s) == 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\t\t\t\t%s = make(%s, 0, elementCount)\n", access, sliceType)
		fmt.Fprintf(fg.body, "\t\t\t\t}\n")
	}

	// Inner decode loop
	fmt.Fprintf(fg.body, "\t\t\t\tfor iNdEx < postIndex {\n")
	switch {
	case isFixed64Kind(kind):
		fg.imports.addImport("encoding/binary", "")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif (iNdEx + 8) > postIndex {\n\t\t\t\t\t\treturn io.ErrUnexpectedEOF\n\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tv := binary.LittleEndian.Uint64(dAtA[iNdEx:])\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tiNdEx += 8\n")
		fg.emitPackedElementDecode(access, fd, kind, "v")
	case isFixed32Kind(kind):
		fg.imports.addImport("encoding/binary", "")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif (iNdEx + 4) > postIndex {\n\t\t\t\t\t\treturn io.ErrUnexpectedEOF\n\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tv := binary.LittleEndian.Uint32(dAtA[iNdEx:])\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tiNdEx += 4\n")
		fg.emitPackedElementDecode(access, fd, kind, "v")
	default: // varint
		fmt.Fprintf(fg.body, "\t\t\t\t\tvar v uint64\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tfor shift := uint(0); ; shift += 7 {\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\t\tif shift >= 64 {\n\t\t\t\t\t\t\treturn protohelpers.ErrIntOverflow\n\t\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\t\tif iNdEx >= postIndex {\n\t\t\t\t\t\t\treturn io.ErrUnexpectedEOF\n\t\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\t\tb := dAtA[iNdEx]\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\t\tiNdEx++\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\t\tv |= uint64(b&0x7F) << shift\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\t\tif b < 0x80 {\n\t\t\t\t\t\t\tbreak\n\t\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\t}\n")
		fg.emitPackedElementDecode(access, fd, kind, "v")
	}
	fmt.Fprintf(fg.body, "\t\t\t\t}\n") // end inner for

	// Unpacked encoding (single element with native wire type)
	nativeWt := wireTypeInt(kind)
	fmt.Fprintf(fg.body, "\t\t\t} else if wireType == %d {\n", nativeWt)

	switch {
	case isFixed64Kind(kind):
		fg.imports.addImport("encoding/binary", "")
		fmt.Fprintf(fg.body, "\t\t\t\tif (iNdEx + 8) > l {\n\t\t\t\t\treturn io.ErrUnexpectedEOF\n\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\tv := binary.LittleEndian.Uint64(dAtA[iNdEx:])\n")
		fmt.Fprintf(fg.body, "\t\t\t\tiNdEx += 8\n")
		fg.emitPackedElementDecode(access, fd, kind, "v")
	case isFixed32Kind(kind):
		fg.imports.addImport("encoding/binary", "")
		fmt.Fprintf(fg.body, "\t\t\t\tif (iNdEx + 4) > l {\n\t\t\t\t\treturn io.ErrUnexpectedEOF\n\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\tv := binary.LittleEndian.Uint32(dAtA[iNdEx:])\n")
		fmt.Fprintf(fg.body, "\t\t\t\tiNdEx += 4\n")
		fg.emitPackedElementDecode(access, fd, kind, "v")
	default:
		fmt.Fprintf(fg.body, "\t\t\t\tvar v uint64\n")
		fmt.Fprintf(fg.body, "\t\t\t\tfor shift := uint(0); ; shift += 7 {\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif shift >= 64 {\n\t\t\t\t\t\treturn protohelpers.ErrIntOverflow\n\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif iNdEx >= l {\n\t\t\t\t\t\treturn io.ErrUnexpectedEOF\n\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tb := dAtA[iNdEx]\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tiNdEx++\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tv |= uint64(b&0x7F) << shift\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif b < 0x80 {\n\t\t\t\t\t\tbreak\n\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t}\n")
		fg.emitPackedElementDecode(access, fd, kind, "v")
	}

	// Skip unexpected wire types
	fmt.Fprintf(fg.body, "\t\t\t} else {\n")
	fmt.Fprintf(fg.body, "\t\t\t\tn, err := skipField(dAtA[iNdEx:], protowire.Number(fieldNum), protowire.Type(wireType))\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif err != nil {\n\t\t\t\t\treturn err\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tiNdEx += n\n")
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
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: v != 0}\n", ooFieldName, variantName, fieldName)

	case protoreflect.Int32Kind:
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: int32(v)}\n", ooFieldName, variantName, fieldName)

	case protoreflect.Sint32Kind:
		fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: int32(protowire.DecodeZigZag(v))}\n", ooFieldName, variantName, fieldName)

	case protoreflect.Uint32Kind:
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: uint32(v)}\n", ooFieldName, variantName, fieldName)

	case protoreflect.Int64Kind:
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: int64(v)}\n", ooFieldName, variantName, fieldName)

	case protoreflect.Sint64Kind:
		fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: int64(protowire.DecodeZigZag(v))}\n", ooFieldName, variantName, fieldName)

	case protoreflect.Uint64Kind:
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: v}\n", ooFieldName, variantName, fieldName)

	case protoreflect.EnumKind:
		enumType := fg.imports.goEnumType(fd.Enum())
		fg.emitInlineVarint()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: %s(v)}\n", ooFieldName, variantName, fieldName, enumType)

	case protoreflect.Fixed32Kind:
		fg.emitInlineFixed32()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: v}\n", ooFieldName, variantName, fieldName)

	case protoreflect.Sfixed32Kind:
		fg.emitInlineFixed32()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: int32(v)}\n", ooFieldName, variantName, fieldName)

	case protoreflect.FloatKind:
		fg.imports.addImport("math", "")
		fg.emitInlineFixed32()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: math.Float32frombits(v)}\n", ooFieldName, variantName, fieldName)

	case protoreflect.Fixed64Kind:
		fg.emitInlineFixed64()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: v}\n", ooFieldName, variantName, fieldName)

	case protoreflect.Sfixed64Kind:
		fg.emitInlineFixed64()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: int64(v)}\n", ooFieldName, variantName, fieldName)

	case protoreflect.DoubleKind:
		fg.imports.addImport("math", "")
		fg.emitInlineFixed64()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: math.Float64frombits(v)}\n", ooFieldName, variantName, fieldName)

	case protoreflect.StringKind:
		fg.emitInlineBytesLen()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: string(dAtA[iNdEx:postIndex])}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceToPostIndex()

	case protoreflect.BytesKind:
		fg.emitInlineBytesLen()
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: append([]byte(nil), dAtA[iNdEx:postIndex]...)}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceToPostIndex()

	case protoreflect.MessageKind:
		fg.emitInlineBytesLen()
		msgType := fg.imports.goSingularType(fd)
		fmt.Fprintf(fg.body, "\t\t\tvar msg %s\n", msgType)
		fmt.Fprintf(fg.body, "\t\t\tif err := msg.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: msg}\n", ooFieldName, variantName, fieldName)
		fg.emitAdvanceToPostIndex()
	}
}

// Inline decode helpers — generate code that reads from dAtA[iNdEx:] and
// advances iNdEx past the consumed bytes.

// emitInlineVarint generates an inline varint decode loop.
// After execution: v contains the decoded uint64, iNdEx is past the varint.
func (fg *FileGenerator) emitInlineVarint() {
	fg.imports.addImport("io", "")
	fmt.Fprintf(fg.body, "\t\t\tvar v uint64\n")
	fmt.Fprintf(fg.body, "\t\t\tfor shift := uint(0); ; shift += 7 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif shift >= 64 {\n\t\t\t\t\treturn protohelpers.ErrIntOverflow\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif iNdEx >= l {\n\t\t\t\t\treturn io.ErrUnexpectedEOF\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tb := dAtA[iNdEx]\n")
	fmt.Fprintf(fg.body, "\t\t\t\tiNdEx++\n")
	fmt.Fprintf(fg.body, "\t\t\t\tv |= uint64(b&0x7F) << shift\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif b < 0x80 {\n\t\t\t\t\tbreak\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
}

// emitInlineBytesLen generates an inline length-prefix decode (varint length +
// bounds checking). After execution: iNdEx points to start of data, postIndex
// points past the data. Caller must set iNdEx = postIndex after using the data.
func (fg *FileGenerator) emitInlineBytesLen() {
	fg.imports.addImport("io", "")
	fmt.Fprintf(fg.body, "\t\t\tvar byteLen int\n")
	fmt.Fprintf(fg.body, "\t\t\tfor shift := uint(0); ; shift += 7 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif shift >= 64 {\n\t\t\t\t\treturn protohelpers.ErrIntOverflow\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif iNdEx >= l {\n\t\t\t\t\treturn io.ErrUnexpectedEOF\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tb := dAtA[iNdEx]\n")
	fmt.Fprintf(fg.body, "\t\t\t\tiNdEx++\n")
	fmt.Fprintf(fg.body, "\t\t\t\tbyteLen |= int(b&0x7F) << shift\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif b < 0x80 {\n\t\t\t\t\tbreak\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tif byteLen < 0 {\n\t\t\t\treturn protohelpers.ErrInvalidLength\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tpostIndex := iNdEx + byteLen\n")
	fmt.Fprintf(fg.body, "\t\t\tif postIndex < 0 {\n\t\t\t\treturn protohelpers.ErrInvalidLength\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tif postIndex > l {\n\t\t\t\treturn io.ErrUnexpectedEOF\n\t\t\t}\n")
}

// emitInlineFixed32 generates an inline fixed32 decode.
// After execution: v contains the decoded uint32, iNdEx is advanced by 4.
func (fg *FileGenerator) emitInlineFixed32() {
	fg.imports.addImport("io", "")
	fg.imports.addImport("encoding/binary", "")
	fmt.Fprintf(fg.body, "\t\t\tif (iNdEx + 4) > l {\n\t\t\t\treturn io.ErrUnexpectedEOF\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tv := binary.LittleEndian.Uint32(dAtA[iNdEx:])\n")
	fmt.Fprintf(fg.body, "\t\t\tiNdEx += 4\n")
}

// emitInlineFixed64 generates an inline fixed64 decode.
// After execution: v contains the decoded uint64, iNdEx is advanced by 8.
func (fg *FileGenerator) emitInlineFixed64() {
	fg.imports.addImport("io", "")
	fg.imports.addImport("encoding/binary", "")
	fmt.Fprintf(fg.body, "\t\t\tif (iNdEx + 8) > l {\n\t\t\t\treturn io.ErrUnexpectedEOF\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tv := binary.LittleEndian.Uint64(dAtA[iNdEx:])\n")
	fmt.Fprintf(fg.body, "\t\t\tiNdEx += 8\n")
}

// emitAdvanceToPostIndex generates iNdEx = postIndex.
func (fg *FileGenerator) emitAdvanceToPostIndex() {
	fmt.Fprintf(fg.body, "\t\t\tiNdEx = postIndex\n")
}
