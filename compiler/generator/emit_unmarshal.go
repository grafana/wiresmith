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

// fieldsForPreScan returns fields whose element count can be determined by
// counting wire-format field-number occurrences. This includes repeated
// message/string/bytes fields (packed scalars are excluded because one wire
// occurrence contains many elements) and map fields (each wire occurrence
// is one map entry).
func fieldsForPreScan(md protoreflect.MessageDescriptor) []protoreflect.FieldDescriptor {
	var fields []protoreflect.FieldDescriptor
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if fd.IsMap() {
			fields = append(fields, fd)
			continue
		}
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
// slices during the main unmarshal loop. Gated on len(b) to skip small messages
// where the overhead isn't justified.
func (fg *FileGenerator) emitPreScan(md protoreflect.MessageDescriptor) {
	fields := fieldsForPreScan(md)
	if len(fields) == 0 {
		return
	}

	fmt.Fprintf(fg.body, "\tif len(b) >= %d {\n", preScanMinBytes)
	fmt.Fprintf(fg.body, "\t\ttmp := b\n")
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
		goType := fg.imports.goType(fd)
		fmt.Fprintf(fg.body, "\t\tif field%dcount > 0 {\n", fd.Number())
		if fd.IsMap() {
			fmt.Fprintf(fg.body, "\t\t\tm.%s = make(%s, field%dcount)\n", goName, goType, fd.Number())
		} else {
			fmt.Fprintf(fg.body, "\t\t\tm.%s = make(%s, 0, field%dcount)\n", goName, goType, fd.Number())
		}
		fmt.Fprintf(fg.body, "\t\t}\n")
	}

	fmt.Fprintf(fg.body, "\t}\n")
}

func (fg *FileGenerator) emitUnmarshal(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")
	fg.imports.addImport("fmt", "")

	// Public wrapper that starts depth tracking at zero.
	fmt.Fprintf(fg.body, "func (m *%s) Unmarshal(b []byte) error {\n", name)
	fmt.Fprintf(fg.body, "\treturn m.unmarshal(b, 0)\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// Private implementation with recursion depth limit.
	fmt.Fprintf(fg.body, "func (m *%s) unmarshal(b []byte, depth int) error {\n", name)
	fmt.Fprintf(fg.body, "\tif depth > maxUnmarshalDepth {\n")
	fmt.Fprintf(fg.body, "\t\treturn fmt.Errorf(\"exceeded max recursion depth\")\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fg.emitPreScan(md)
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

	if fd.IsMap() {
		fg.emitWireTypeCheck(protoreflect.MessageKind)
		fg.emitMapFieldUnmarshal(goName, fd)
		return
	}

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

// emitWireTypeCheck emits a check that the wire type matches the expected type
// for a given proto kind, skipping the field if it doesn't match.
func (fg *FileGenerator) emitWireTypeCheck(kind protoreflect.Kind) {
	wt := expectedWireType(kind)
	fmt.Fprintf(fg.body, "\t\t\tif typ != %s {\n", wt)
	fmt.Fprintf(fg.body, "\t\t\t\tn, err := skipField(b, num, typ)\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif err != nil {\n\t\t\t\t\treturn err\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tb = b[n:]\n")
	fmt.Fprintf(fg.body, "\t\t\t\tcontinue\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
}

// expectedWireType returns the protowire constant string for the expected wire
// type of a proto kind.
func expectedWireType(kind protoreflect.Kind) string {
	switch kind {
	case protoreflect.BoolKind, protoreflect.EnumKind,
		protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Uint32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Uint64Kind:
		return "protowire.VarintType"
	case protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind, protoreflect.FloatKind:
		return "protowire.Fixed32Type"
	case protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind, protoreflect.DoubleKind:
		return "protowire.Fixed64Type"
	case protoreflect.StringKind, protoreflect.BytesKind, protoreflect.MessageKind:
		return "protowire.BytesType"
	default:
		return "protowire.BytesType"
	}
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
		fg.emitUnmarshalCall(access, fd.Message())
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

	case protoreflect.BytesKind:
		// optional bytes is []byte (not *[]byte), so we copy the value directly.
		fg.emitConsumeBytes()
		fmt.Fprintf(fg.body, "\t\t\t%s = append(%s[:0], v...)\n", access, access)
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
		fmt.Fprintf(fg.body, "\t\t\t%s = append(%s, %s{})\n", access, access, msgType)
		sliceAccess := fmt.Sprintf("%s[len(%s)-1]", access, access)
		fg.emitUnmarshalCall(sliceAccess, fd.Message())
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
	// Pre-allocate with exact capacity for fixed-size packed fields
	sliceType := fg.imports.goType(fd)
	if isFixed64Kind(kind) {
		fmt.Fprintf(fg.body, "\t\t\t\tif elementCount := len(data) / 8; elementCount != 0 && len(%s) == 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\t\t\t\t%s = make(%s, 0, elementCount)\n", access, sliceType)
		fmt.Fprintf(fg.body, "\t\t\t\t}\n")
	} else if isFixed32Kind(kind) {
		fmt.Fprintf(fg.body, "\t\t\t\tif elementCount := len(data) / 4; elementCount != 0 && len(%s) == 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\t\t\t\t%s = make(%s, 0, elementCount)\n", access, sliceType)
		fmt.Fprintf(fg.body, "\t\t\t\t}\n")
	} else if kind == protoreflect.BoolKind {
		fmt.Fprintf(fg.body, "\t\t\t\tif elementCount := len(data); elementCount != 0 && len(%s) == 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\t\t\t\t%s = make(%s, 0, elementCount)\n", access, sliceType)
		fmt.Fprintf(fg.body, "\t\t\t\t}\n")
	} else {
		// Varint kinds: count elements by scanning for terminator bytes (< 128)
		fmt.Fprintf(fg.body, "\t\t\t\tvar elementCount int\n")
		fmt.Fprintf(fg.body, "\t\t\t\tfor _, b := range data {\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif b < 128 {\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\t\telementCount++\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\tif elementCount != 0 && len(%s) == 0 {\n", access)
		fmt.Fprintf(fg.body, "\t\t\t\t\t%s = make(%s, 0, elementCount)\n", access, sliceType)
		fmt.Fprintf(fg.body, "\t\t\t\t}\n")
	}
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
	nativeWt := expectedWireType(kind)
	fmt.Fprintf(fg.body, "\t\t\t} else if typ == %s {\n", nativeWt)

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

	// Skip unexpected wire types
	fmt.Fprintf(fg.body, "\t\t\t} else {\n")
	fmt.Fprintf(fg.body, "\t\t\t\tn, err := skipField(b, num, typ)\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif err != nil {\n\t\t\t\t\treturn err\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tb = b[n:]\n")
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

func (fg *FileGenerator) emitMapFieldUnmarshal(goName string, fd protoreflect.FieldDescriptor) {
	keyFd := fd.MapKey()
	valFd := fd.MapValue()
	access := "m." + goName
	mapType := fg.imports.goType(fd)

	fg.emitConsumeBytes()

	fmt.Fprintf(fg.body, "\t\t\tif %s == nil {\n", access)
	fmt.Fprintf(fg.body, "\t\t\t\t%s = make(%s)\n", access, mapType)
	fmt.Fprintf(fg.body, "\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tvar mapkey %s\n", fg.imports.goSingularType(keyFd))
	fmt.Fprintf(fg.body, "\t\t\tvar mapvalue %s\n", fg.imports.goSingularType(valFd))
	fmt.Fprintf(fg.body, "\t\t\tentryData := v\n")
	fmt.Fprintf(fg.body, "\t\t\tfor len(entryData) > 0 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\tentryNum, entryTyp, entryTagLen := protowire.ConsumeTag(entryData)\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif entryTagLen < 0 {\n\t\t\t\t\treturn fmt.Errorf(\"invalid map entry tag\")\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tentryData = entryData[entryTagLen:]\n")
	fmt.Fprintf(fg.body, "\t\t\t\tswitch entryNum {\n")

	fmt.Fprintf(fg.body, "\t\t\t\tcase 1:\n")
	fg.emitMapEntryWireTypeCheck(keyFd.Kind())
	fg.emitMapEntryFieldUnmarshal("mapkey", keyFd, "\t\t\t\t\t")

	fmt.Fprintf(fg.body, "\t\t\t\tcase 2:\n")
	fg.emitMapEntryWireTypeCheck(valFd.Kind())
	fg.emitMapEntryFieldUnmarshal("mapvalue", valFd, "\t\t\t\t\t")

	// Unknown fields — skip
	fmt.Fprintf(fg.body, "\t\t\t\tdefault:\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tskipN, skipErr := skipField(entryData, entryNum, entryTyp)\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tif skipErr != nil {\n\t\t\t\t\t\treturn skipErr\n\t\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tentryData = entryData[skipN:]\n")

	fmt.Fprintf(fg.body, "\t\t\t\t}\n") // end switch
	fmt.Fprintf(fg.body, "\t\t\t}\n")   // end for
	fmt.Fprintf(fg.body, "\t\t\t%s[mapkey] = mapvalue\n", access)

	fg.emitAdvanceBytes()
}

// emitMapEntryWireTypeCheck emits a wire-type check for a map entry field,
// skipping on mismatch to match the behavior of non-map field parsing.
func (fg *FileGenerator) emitMapEntryWireTypeCheck(kind protoreflect.Kind) {
	wt := expectedWireType(kind)
	fmt.Fprintf(fg.body, "\t\t\t\t\tif entryTyp != %s {\n", wt)
	fmt.Fprintf(fg.body, "\t\t\t\t\t\tskipN, skipErr := skipField(entryData, entryNum, entryTyp)\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\t\tif skipErr != nil {\n\t\t\t\t\t\t\treturn skipErr\n\t\t\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\t\tentryData = entryData[skipN:]\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\t\tcontinue\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\t}\n")
}

func (fg *FileGenerator) emitMapEntryFieldUnmarshal(varName string, fd protoreflect.FieldDescriptor, indent string) {
	kind := fd.Kind()
	switch kind {
	case protoreflect.BoolKind:
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeVarint(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid varint\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = tmpVal != 0\n", indent, varName)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.Int32Kind:
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeVarint(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid varint\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = int32(tmpVal)\n", indent, varName)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.Sint32Kind:
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeVarint(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid varint\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = int32(protowire.DecodeZigZag(tmpVal))\n", indent, varName)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.Uint32Kind:
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeVarint(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid varint\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = uint32(tmpVal)\n", indent, varName)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.Int64Kind:
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeVarint(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid varint\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = int64(tmpVal)\n", indent, varName)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.Sint64Kind:
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeVarint(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid varint\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = int64(protowire.DecodeZigZag(tmpVal))\n", indent, varName)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.Uint64Kind:
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeVarint(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid varint\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = tmpVal\n", indent, varName)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.EnumKind:
		enumType := fg.imports.goEnumType(fd.Enum())
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeVarint(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid varint\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = %s(tmpVal)\n", indent, varName, enumType)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.Fixed32Kind:
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeFixed32(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid fixed32\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = tmpVal\n", indent, varName)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.Sfixed32Kind:
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeFixed32(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid fixed32\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = int32(tmpVal)\n", indent, varName)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.FloatKind:
		fg.imports.addImport("math", "")
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeFixed32(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid fixed32\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = math.Float32frombits(tmpVal)\n", indent, varName)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.Fixed64Kind:
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeFixed64(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid fixed64\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = tmpVal\n", indent, varName)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.Sfixed64Kind:
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeFixed64(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid fixed64\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = int64(tmpVal)\n", indent, varName)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.DoubleKind:
		fg.imports.addImport("math", "")
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeFixed64(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid fixed64\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = math.Float64frombits(tmpVal)\n", indent, varName)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.StringKind:
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeString(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid string\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = tmpVal\n", indent, varName)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.BytesKind:
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeBytes(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid bytes\")\n%s}\n", indent, indent, indent)
		fmt.Fprintf(fg.body, "%s%s = append(%s[:0], tmpVal...)\n", indent, varName, varName)
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)

	case protoreflect.MessageKind:
		fmt.Fprintf(fg.body, "%stmpVal, tmpN := protowire.ConsumeBytes(entryData)\n", indent)
		fmt.Fprintf(fg.body, "%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid bytes\")\n%s}\n", indent, indent, indent)
		msgPkg := string(fd.Message().ParentFile().Package())
		if msgPkg == fg.imports.selfPkg {
			fmt.Fprintf(fg.body, "%sif err := %s.unmarshal(tmpVal, depth+1); err != nil {\n%s\treturn err\n%s}\n", indent, varName, indent, indent)
		} else {
			fmt.Fprintf(fg.body, "%sif err := %s.Unmarshal(tmpVal); err != nil {\n%s\treturn err\n%s}\n", indent, varName, indent, indent)
		}
		fmt.Fprintf(fg.body, "%sentryData = entryData[tmpN:]\n", indent)
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
		fg.emitUnmarshalCall("msg", fd.Message())
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

// emitUnmarshalCall emits the correct recursive unmarshal call for a message
// field. Same-package types use the private unmarshal(b, depth+1) to propagate
// the depth counter; cross-package types use the public Unmarshal(b) because
// the private method is not exported.
func (fg *FileGenerator) emitUnmarshalCall(access string, md protoreflect.MessageDescriptor) {
	msgPkg := string(md.ParentFile().Package())
	if msgPkg == fg.imports.selfPkg {
		fmt.Fprintf(fg.body, "\t\t\tif err := %s.unmarshal(v, depth+1); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", access)
	} else {
		fmt.Fprintf(fg.body, "\t\t\tif err := %s.Unmarshal(v); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", access)
	}
}
