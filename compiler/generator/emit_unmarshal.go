package generator

import (
	"fmt"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) skipFieldFuncName() string {
	if fg.gen != nil && fg.gen.GogoCompat {
		// In gogo compat mode, use file-specific names to avoid redeclaration
		// when multiple .pb.go files share a Go package.
		base := filepath.Base(fg.fd.Path())
		base = strings.TrimSuffix(base, ".proto")
		return "skipField_" + snakeToPascal(base)
	}
	return "skipField"
}

func (fg *FileGenerator) emitSkipFieldHelper() {
	fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")
	fg.imports.addImport("fmt", "")

	funcName := fg.skipFieldFuncName()
	fmt.Fprintf(fg.body, "func %s(b []byte, num protowire.Number, typ protowire.Type) (int, error) {\n", funcName)
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
// slices during the main unmarshal loop. Gated on len(b) to skip small messages
// where the overhead isn't justified.
func (fg *FileGenerator) emitPreScan(md protoreflect.MessageDescriptor) {
	fields := repeatedFieldsForPreScan(md)
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
	fg.imports.addImport("fmt", "")

	fmt.Fprintf(fg.body, "func (m *%s) Unmarshal(b []byte) error {\n", name)
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
	fmt.Fprintf(fg.body, "\t\t\tn, err := %s(b, num, typ)\n", fg.skipFieldFuncName())
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

	// Packed repeated fields handle wire type dispatch internally
	// (packed=BytesType vs unpacked=native type).
	if fd.IsList() && isPackable(kind) {
		fg.emitRepeatedFieldUnmarshal(goName, fd)
		return
	}

	// Map fields.
	if fd.IsMap() {
		fg.emitWireTypeCheck(protoreflect.MessageKind)
		fg.emitMapUnmarshal("m."+goName, fd)
		return
	}

	// stdduration/stdtime fields use gogo helper unmarshal functions.
	if isStdDuration(fd) || isStdTime(fd) {
		fg.emitWireTypeCheck(protoreflect.MessageKind)
		fg.emitStdWKTUnmarshal("m."+goName, fd)
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
	fmt.Fprintf(fg.body, "\t\t\t\tn, err := %s(b, num, typ)\n", fg.skipFieldFuncName())
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

	// For casttype, wrap the value cast with the custom type.
	castPrefix := ""
	castSuffix := ""
	if ct := getCastType(fd); ct != "" {
		goType := fg.imports.resolveGoTypePath(ct)
		castPrefix = goType + "("
		castSuffix = ")"
	}

	switch kind {
	case protoreflect.BoolKind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = v != 0\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Int32Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = %sint32(v)%s\n", access, castPrefix, castSuffix)
		fg.emitAdvanceBytes()

	case protoreflect.Sint32Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = int32(protowire.DecodeZigZag(v))\n", access)
		fg.emitAdvanceBytes()

	case protoreflect.Uint32Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = %suint32(v)%s\n", access, castPrefix, castSuffix)
		fg.emitAdvanceBytes()

	case protoreflect.Int64Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = %sint64(v)%s\n", access, castPrefix, castSuffix)
		fg.emitAdvanceBytes()

	case protoreflect.Sint64Kind:
		fg.emitConsumeVarint()
		fmt.Fprintf(fg.body, "\t\t\t%s = %sint64(protowire.DecodeZigZag(v))%s\n", access, castPrefix, castSuffix)
		fg.emitAdvanceBytes()

	case protoreflect.Uint64Kind:
		fg.emitConsumeVarint()
		if castPrefix != "" {
			fmt.Fprintf(fg.body, "\t\t\t%s = %sv%s\n", access, castPrefix, castSuffix)
		} else {
			fmt.Fprintf(fg.body, "\t\t\t%s = v\n", access)
		}
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
		var elemType string
		if ct := getCustomType(fd); ct != "" {
			elemType = fg.imports.resolveGoTypePath(ct)
		} else {
			elemType = fg.imports.goSingularType(fd)
		}
		fg.emitConsumeBytes()
		if isGogoPointerField(fg.gen, fd) || isGogoPointerCustomType(fg.gen, fd) {
			fmt.Fprintf(fg.body, "\t\t\tif %s == nil {\n\t\t\t\t%s = &%s{}\n\t\t\t}\n", access, access, elemType)
		}
		fmt.Fprintf(fg.body, "\t\t\tif err := %s.Unmarshal(v); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", access)
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
		// For customtype fields, use the custom type for allocation.
		var elemType string
		if ct := getCustomType(fd); ct != "" {
			elemType = fg.imports.resolveGoTypePath(ct)
		} else {
			elemType = fg.imports.goSingularType(fd)
		}
		fg.emitConsumeBytes()
		// In gogo compat mode, repeated message fields default to pointer slices
		// unless (gogoproto.nullable) = false.
		usePtr := fg.gen != nil && fg.gen.GogoCompat && isFieldNullable(fd) && getCustomType(fd) == ""
		if usePtr {
			fmt.Fprintf(fg.body, "\t\t\t%s = append(%s, &%s{})\n", access, access, elemType)
		} else {
			fmt.Fprintf(fg.body, "\t\t\t%s = append(%s, %s{})\n", access, access, elemType)
		}
		fmt.Fprintf(fg.body, "\t\t\tif err := %s[len(%s)-1].Unmarshal(v); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", access, access)
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
	fmt.Fprintf(fg.body, "\t\t\t\tn, err := %s(b, num, typ)\n", fg.skipFieldFuncName())
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
		fmt.Fprintf(fg.body, "\t\t\tif err := msg.Unmarshal(v); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n")
		// In gogo compat mode, oneof message fields are pointers.
		if fg.gen != nil && fg.gen.GogoCompat {
			fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: &msg}\n", ooFieldName, variantName, fieldName)
		} else {
			fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: msg}\n", ooFieldName, variantName, fieldName)
		}
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

// emitStdWKTUnmarshal emits unmarshal code for stdduration/stdtime fields.
func (fg *FileGenerator) emitStdWKTUnmarshal(access string, fd protoreflect.FieldDescriptor) {
	gogoTypes := fg.imports.addImport("github.com/gogo/protobuf/types", "types")

	var unmarshalFunc string
	if isStdDuration(fd) {
		unmarshalFunc = gogoTypes + ".StdDurationUnmarshal"
	} else {
		unmarshalFunc = gogoTypes + ".StdTimeUnmarshal"
	}

	nullable := isFieldNullable(fd)

	fg.emitConsumeBytes()
	if nullable {
		fg.imports.addStdImport("time")
		if isStdTime(fd) {
			fmt.Fprintf(fg.body, "\t\t\tvar t time.Time\n")
			fmt.Fprintf(fg.body, "\t\t\tif err := %s(&t, v); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", unmarshalFunc)
			fmt.Fprintf(fg.body, "\t\t\t%s = &t\n", access)
		} else {
			fmt.Fprintf(fg.body, "\t\t\tvar d time.Duration\n")
			fmt.Fprintf(fg.body, "\t\t\tif err := %s(&d, v); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", unmarshalFunc)
			fmt.Fprintf(fg.body, "\t\t\t%s = &d\n", access)
		}
	} else {
		fmt.Fprintf(fg.body, "\t\t\tif err := %s(&%s, v); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", unmarshalFunc, access)
	}
	fg.emitAdvanceBytes()
}

// emitMapUnmarshal emits unmarshal code for map fields.
func (fg *FileGenerator) emitMapUnmarshal(access string, fd protoreflect.FieldDescriptor) {
	keyFd := fd.MapKey()
	valFd := fd.Message().Fields().ByNumber(2)

	fg.emitConsumeBytes()

	// Initialize map if nil
	fmt.Fprintf(fg.body, "\t\t\tif %s == nil {\n\t\t\t\t%s = make(%s)\n\t\t\t}\n", access, access, fg.imports.goType(fd))

	// Declare key and value variables
	switch keyFd.Kind() {
	case protoreflect.StringKind:
		fmt.Fprintf(fg.body, "\t\t\tvar mapkey string\n")
	default:
		fmt.Fprintf(fg.body, "\t\t\tvar mapkey %s\n", fg.imports.goSingularType(keyFd))
	}

	switch valFd.Kind() {
	case protoreflect.MessageKind:
		valType := fg.imports.goSingularType(valFd)
		fmt.Fprintf(fg.body, "\t\t\tvar mapvalue *%s\n", valType)
	case protoreflect.StringKind:
		fmt.Fprintf(fg.body, "\t\t\tvar mapvalue string\n")
	default:
		fmt.Fprintf(fg.body, "\t\t\tvar mapvalue %s\n", fg.imports.goSingularType(valFd))
	}

	// Parse the entry bytes
	fmt.Fprintf(fg.body, "\t\t\tfor len(v) > 0 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\tentryNum, entryTyp, entryTagLen := protowire.ConsumeTag(v)\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif entryTagLen < 0 {\n\t\t\t\t\treturn fmt.Errorf(\"invalid map entry tag\")\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tv = v[entryTagLen:]\n")
	fmt.Fprintf(fg.body, "\t\t\t\tswitch entryNum {\n")

	// Key (field 1)
	fmt.Fprintf(fg.body, "\t\t\t\tcase 1:\n")
	switch keyFd.Kind() {
	case protoreflect.StringKind:
		fmt.Fprintf(fg.body, "\t\t\t\t\tval, valLen := protowire.ConsumeString(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif valLen < 0 {\n\t\t\t\t\t\treturn fmt.Errorf(\"invalid map key\")\n\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tmapkey = val\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tv = v[valLen:]\n")
	default:
		fmt.Fprintf(fg.body, "\t\t\t\t\tval, valLen := protowire.ConsumeVarint(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif valLen < 0 {\n\t\t\t\t\t\treturn fmt.Errorf(\"invalid map key\")\n\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tmapkey = %s(val)\n", fg.imports.goSingularType(keyFd))
		fmt.Fprintf(fg.body, "\t\t\t\t\tv = v[valLen:]\n")
	}

	// Value (field 2)
	fmt.Fprintf(fg.body, "\t\t\t\tcase 2:\n")
	switch valFd.Kind() {
	case protoreflect.MessageKind:
		valType := fg.imports.goSingularType(valFd)
		fmt.Fprintf(fg.body, "\t\t\t\t\tdata, dataLen := protowire.ConsumeBytes(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif dataLen < 0 {\n\t\t\t\t\t\treturn fmt.Errorf(\"invalid map value\")\n\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tmapvalue = &%s{}\n", valType)
		fmt.Fprintf(fg.body, "\t\t\t\t\tif err := mapvalue.Unmarshal(data); err != nil {\n\t\t\t\t\t\treturn err\n\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tv = v[dataLen:]\n")
	case protoreflect.StringKind:
		fmt.Fprintf(fg.body, "\t\t\t\t\tval, valLen := protowire.ConsumeString(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif valLen < 0 {\n\t\t\t\t\t\treturn fmt.Errorf(\"invalid map value\")\n\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tmapvalue = val\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tv = v[valLen:]\n")
	default:
		fmt.Fprintf(fg.body, "\t\t\t\t\tval, valLen := protowire.ConsumeVarint(v)\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tif valLen < 0 {\n\t\t\t\t\t\treturn fmt.Errorf(\"invalid map value\")\n\t\t\t\t\t}\n")
		fmt.Fprintf(fg.body, "\t\t\t\t\tmapvalue = %s(val)\n", fg.imports.goSingularType(valFd))
		fmt.Fprintf(fg.body, "\t\t\t\t\tv = v[valLen:]\n")
	}

	// Skip unknown fields
	fmt.Fprintf(fg.body, "\t\t\t\tdefault:\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tskipN, err := %s(v, entryNum, entryTyp)\n", fg.skipFieldFuncName())
	fmt.Fprintf(fg.body, "\t\t\t\t\tif err != nil {\n\t\t\t\t\t\treturn err\n\t\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tv = v[skipN:]\n")
	fmt.Fprintf(fg.body, "\t\t\t\t}\n") // end switch
	fmt.Fprintf(fg.body, "\t\t\t}\n")   // end for

	fmt.Fprintf(fg.body, "\t\t\t%s[mapkey] = mapvalue\n", access)
	fg.emitAdvanceBytes()
}
