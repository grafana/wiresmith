package generator

import (
	"fmt"
	"wiresmith/compiler/types"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// emitSkipValueHelper emits an inline skip function that skips a field value
// given its wire type. Used by the main unmarshal loop for unknown fields and
// wire type mismatches where the tag has already been decoded.
func (fg *FileGenerator) emitSkipValueHelper() {
	fg.imports.addImport("fmt", "")
	fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")

	fmt.Fprintf(fg.body, "func skipValue(dAtA []byte, wireType int, fieldNum int32) (int, error) {\n")
	fmt.Fprintf(fg.body, "\tiNdEx := 0\n")
	fmt.Fprintf(fg.body, "\tl := len(dAtA)\n")
	fmt.Fprintf(fg.body, "\tswitch wireType {\n")
	fmt.Fprintf(fg.body, "\tcase 0:\n") // varint
	fmt.Fprintf(fg.body, "\t\tfor shift := 0; ; shift++ {\n")
	fmt.Fprintf(fg.body, "\t\t\tif shift >= 10 {\n\t\t\t\treturn 0, fmt.Errorf(\"invalid varint\")\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tif iNdEx >= l {\n\t\t\t\treturn 0, fmt.Errorf(\"invalid varint\")\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tiNdEx++\n")
	fmt.Fprintf(fg.body, "\t\t\tif dAtA[iNdEx-1] < 0x80 {\n\t\t\t\tbreak\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t}\n")
	fmt.Fprintf(fg.body, "\tcase 1:\n") // fixed64
	fmt.Fprintf(fg.body, "\t\tif (iNdEx + 8) > l {\n\t\t\treturn 0, fmt.Errorf(\"truncated fixed64\")\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t\tiNdEx += 8\n")
	fmt.Fprintf(fg.body, "\tcase 2:\n") // length-delimited
	fmt.Fprintf(fg.body, "\t\tvar length uint64\n")
	fmt.Fprintf(fg.body, "\t\tfor shift := uint(0); ; shift += 7 {\n")
	fmt.Fprintf(fg.body, "\t\t\tif shift >= 64 {\n\t\t\t\treturn 0, fmt.Errorf(\"invalid bytes\")\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tif iNdEx >= l {\n\t\t\t\treturn 0, fmt.Errorf(\"invalid bytes\")\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tb := dAtA[iNdEx]\n")
	fmt.Fprintf(fg.body, "\t\t\tiNdEx++\n")
	fmt.Fprintf(fg.body, "\t\t\tlength |= uint64(b&0x7F) << shift\n")
	fmt.Fprintf(fg.body, "\t\t\tif b < 0x80 {\n\t\t\t\tbreak\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t}\n")
	fmt.Fprintf(fg.body, "\t\tif int(length) < 0 {\n\t\t\treturn 0, fmt.Errorf(\"invalid bytes\")\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t\tiNdEx += int(length)\n")
	fmt.Fprintf(fg.body, "\t\tif iNdEx < 0 || iNdEx > l {\n\t\t\treturn 0, fmt.Errorf(\"invalid bytes\")\n\t\t}\n")
	fmt.Fprintf(fg.body, "\tcase 3:\n") // start group
	fmt.Fprintf(fg.body, "\t\t_, n := protowire.ConsumeGroup(protowire.Number(fieldNum), dAtA[iNdEx:])\n")
	fmt.Fprintf(fg.body, "\t\tif n < 0 {\n\t\t\treturn 0, fmt.Errorf(\"invalid group\")\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t\tiNdEx += n\n")
	fmt.Fprintf(fg.body, "\tcase 5:\n") // fixed32
	fmt.Fprintf(fg.body, "\t\tif (iNdEx + 4) > l {\n\t\t\treturn 0, fmt.Errorf(\"truncated fixed32\")\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t\tiNdEx += 4\n")
	fmt.Fprintf(fg.body, "\tdefault:\n")
	fmt.Fprintf(fg.body, "\t\treturn 0, fmt.Errorf(\"unknown wire type %%d\", wireType)\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\treturn iNdEx, nil\n")
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
const preScanMinBytes = 256

// emitPreScan emits a lightweight tag-scanning loop that counts occurrences of
// repeated message/string/bytes fields, then pre-allocates their slices with
// exact capacity. Uses inline varint decoding for performance.
func (fg *FileGenerator) emitPreScan(md protoreflect.MessageDescriptor) {
	fields := fieldsForPreScan(md)
	if len(fields) == 0 {
		return
	}

	fmt.Fprintf(fg.body, "\tif l >= %d {\n", preScanMinBytes)
	fmt.Fprintf(fg.body, "\t\tvar preIdx int\n")
	for _, fd := range fields {
		fmt.Fprintf(fg.body, "\t\tvar field%dcount int\n", fd.Number())
	}
	fmt.Fprintf(fg.body, "\t\tfor preIdx < l {\n")

	// Inline tag decode
	fmt.Fprintf(fg.body, "\t\t\tvar preWire uint64\n")
	fmt.Fprintf(fg.body, "\t\t\tfor shift := uint(0); ; shift += 7 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif preIdx >= l {\n\t\t\t\t\tbreak\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tb := dAtA[preIdx]\n")
	fmt.Fprintf(fg.body, "\t\t\t\tpreIdx++\n")
	fmt.Fprintf(fg.body, "\t\t\t\tpreWire |= uint64(b&0x7F) << shift\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif b < 0x80 {\n\t\t\t\t\tbreak\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tpreNum := int32(preWire >> 3)\n")
	fmt.Fprintf(fg.body, "\t\t\tpreTyp := int(preWire & 0x7)\n")

	fmt.Fprintf(fg.body, "\t\t\tswitch preNum {\n")
	for _, fd := range fields {
		fmt.Fprintf(fg.body, "\t\t\tcase %d:\n", fd.Number())
		fmt.Fprintf(fg.body, "\t\t\t\tfield%dcount++\n", fd.Number())
	}
	fmt.Fprintf(fg.body, "\t\t\t}\n")

	// Skip field value
	fmt.Fprintf(fg.body, "\t\t\tswitch preTyp {\n")
	fmt.Fprintf(fg.body, "\t\t\tcase 0:\n") // varint
	fmt.Fprintf(fg.body, "\t\t\t\tfor preIdx < l {\n\t\t\t\t\tpreIdx++\n\t\t\t\t\tif dAtA[preIdx-1] < 0x80 {\n\t\t\t\t\t\tbreak\n\t\t\t\t\t}\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tcase 1:\n") // fixed64
	fmt.Fprintf(fg.body, "\t\t\t\tpreIdx += 8\n")
	fmt.Fprintf(fg.body, "\t\t\tcase 2:\n") // bytes
	fmt.Fprintf(fg.body, "\t\t\t\tvar preLen uint64\n")
	fmt.Fprintf(fg.body, "\t\t\t\tfor shift := uint(0); ; shift += 7 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tif preIdx >= l {\n\t\t\t\t\t\tbreak\n\t\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tb := dAtA[preIdx]\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tpreIdx++\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tpreLen |= uint64(b&0x7F) << shift\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tif b < 0x80 {\n\t\t\t\t\t\tbreak\n\t\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tpreIdx += int(preLen)\n")
	fmt.Fprintf(fg.body, "\t\t\tcase 5:\n") // fixed32
	fmt.Fprintf(fg.body, "\t\t\t\tpreIdx += 4\n")
	fmt.Fprintf(fg.body, "\t\t\tdefault:\n")
	fmt.Fprintf(fg.body, "\t\t\t\tbreak\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tif preIdx < 0 || preIdx > l {\n\t\t\t\tbreak\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t}\n")

	for _, fd := range fields {
		goName := snakeToPascal(string(fd.Name()))
		// goFieldType respects (wiresmith.options.pointer) so a repeated
		// pointer-message field pre-allocates as `[]*Msg` rather than `[]Msg`.
		goType := fg.goFieldType(fd)
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
	fg.imports.addImport("fmt", "")
	fg.imports.addImport("io", "")

	// Public wrapper that starts depth tracking at zero.
	fmt.Fprintf(fg.body, "func (m *%s) Unmarshal(b []byte) error {\n", name)
	fmt.Fprintf(fg.body, "\treturn m.unmarshal(b, 0)\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// Private implementation with inline varint decoding (iNdEx/dAtA pattern).
	fmt.Fprintf(fg.body, "func (m *%s) unmarshal(dAtA []byte, depth int) error {\n", name)
	fmt.Fprintf(fg.body, "\tif depth > maxUnmarshalDepth {\n")
	fmt.Fprintf(fg.body, "\t\treturn fmt.Errorf(\"exceeded max recursion depth\")\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\tl := len(dAtA)\n")
	fmt.Fprintf(fg.body, "\tiNdEx := 0\n")

	fg.emitPreScan(md)

	// Main parse loop with inline tag decoding.
	fmt.Fprintf(fg.body, "\tfor iNdEx < l {\n")

	types.EmitConsumeTagAt(fg, "\t\t", "wire")
	fmt.Fprintf(fg.body, "\t\tfieldNum := int32(wire >> 3)\n")
	fmt.Fprintf(fg.body, "\t\twireType := int(wire & 0x7)\n")

	fmt.Fprintf(fg.body, "\t\tswitch fieldNum {\n")

	pm := fg.presenceMap(md)
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		fg.emitFieldUnmarshal(md, fd)
		if bitIndex, ok := pm[fd.Number()]; ok {
			fmt.Fprintf(fg.body, "\t\t\t%s\n", presenceSet(bitIndex))
		}
	}

	fmt.Fprintf(fg.body, "\t\tdefault:\n")
	fmt.Fprintf(fg.body, "\t\t\tn, err := skipValue(dAtA[iNdEx:], wireType, fieldNum)\n")
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
	kind := fd.Kind()
	access := "m." + goName

	fmt.Fprintf(fg.body, "\t\tcase %d: // %s\n", fd.Number(), fd.Name())

	if fd.IsMap() {
		fg.emitWireTypeCheck(protoreflect.MessageKind)
		mf := &types.MapField{
			Key:       types.Get(fd.MapKey().Kind()),
			Val:       types.Get(fd.MapValue().Kind()),
			MapType:   fg.imports.goType(fd),
			KeyGoType: fg.imports.goSingularType(fd.MapKey()),
			ValGoType: fg.imports.goSingularType(fd.MapValue()),
			KeyCtx:    fg.fieldContext(fd.MapKey()),
			ValCtx:    fg.fieldContext(fd.MapValue()),
		}
		types.AddTypeImports(fg, mf)
		mf.EmitUnmarshal(fg, access, types.FieldContext{})
		return
	}

	t := types.Get(kind)

	// Packed repeated fields handle wire type dispatch internally.
	if fd.IsList() && t.IsPackable() {
		ctx := fg.fieldContext(fd)
		ctx.SliceType = fg.imports.goType(fd)
		rf := &types.RepeatedField{Inner: t, IsPacked: fd.IsPacked()}
		types.AddTypeImports(fg, rf)
		rf.EmitUnmarshal(fg, access, ctx)
		return
	}

	fg.emitWireTypeCheck(kind)

	if fd.IsList() {
		ctx := fg.fieldContext(fd)
		// fieldType dispatches between RepeatedField and RepeatedPointer based
		// on `(wiresmith.options.pointer)`; the FieldType interface keeps the
		// call site uniform.
		ft := fg.fieldType(fd)
		types.AddTypeImports(fg, ft)
		ft.EmitUnmarshal(fg, access, ctx)
		return
	}

	ctx := fg.fieldContext(fd)

	if isRealOneof(fd) {
		oo := fd.ContainingOneof()
		ooFieldName := snakeToPascal(string(oo.Name()))
		variantName := oneofVariantName(md, fd)
		fieldName := snakeToPascal(string(fd.Name()))
		types.AddTypeImports(fg, t)
		t.EmitConsume(fg)

		switch kind {
		case protoreflect.MessageKind:
			// EmitConsume set postIndex for length-delimited types.
			// Merge semantics: if the oneof already holds the same variant,
			// unmarshal into the existing message instead of replacing it.
			fmt.Fprintf(fg.body, "\t\t\tvar msg %s\n", ctx.MessageType)
			fmt.Fprintf(fg.body, "\t\t\tif ov, ok := m.%s.(*%s); ok {\n", ooFieldName, variantName)
			fmt.Fprintf(fg.body, "\t\t\t\tmsg = ov.%s\n", fieldName)
			fmt.Fprintf(fg.body, "\t\t\t}\n")
			if ctx.IsSamePackage {
				fmt.Fprintf(fg.body, "\t\t\tif err := msg.unmarshal(dAtA[iNdEx:postIndex], depth+1); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n")
			} else {
				fmt.Fprintf(fg.body, "\t\t\tif err := msg.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n")
			}
			fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: msg}\n", ooFieldName, variantName, fieldName)
			fmt.Fprintf(fg.body, "\t\t\tiNdEx = postIndex\n")
		case protoreflect.StringKind, protoreflect.BytesKind:
			// EmitConsume set postIndex for length-delimited types.
			fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: %s}\n", ooFieldName, variantName, fieldName, t.CastExpr("dAtA[iNdEx:postIndex]", ctx))
			fmt.Fprintf(fg.body, "\t\t\tiNdEx = postIndex\n")
		default:
			// EmitConsume set v for value types (varint/fixed).
			fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: %s}\n", ooFieldName, variantName, fieldName, t.CastExpr("v", ctx))
		}
		return
	}

	if fd.HasOptionalKeyword() {
		if fd.Kind() == protoreflect.MessageKind {
			// Optional message: allocate pointer if nil, then unmarshal into it.
			types.AddTypeImports(fg, t)
			t.EmitConsume(fg)
			msgType := fg.imports.goSingularType(fd)
			fmt.Fprintf(fg.body, "\t\t\tif %s == nil {\n\t\t\t\t%s = new(%s)\n\t\t\t}\n", access, access, msgType)
			if ctx.IsSamePackage {
				fmt.Fprintf(fg.body, "\t\t\tif err := %s.unmarshal(dAtA[iNdEx:postIndex], depth+1); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", access)
			} else {
				fmt.Fprintf(fg.body, "\t\t\tif err := %s.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", access)
			}
			fmt.Fprintf(fg.body, "\t\t\tiNdEx = postIndex\n")
			return
		}
		of := &types.OptionalField{Inner: t}
		types.AddTypeImports(fg, of)
		of.EmitUnmarshal(fg, access, ctx)
		return
	}

	// Singular `(wiresmith.options.pointer) = true` on a message field is
	// dispatched through fg.fieldType — the same single entry point used by
	// emit_marshal, emit_size, and the repeated branch above. Routing it here
	// instead of inlining a second PointerField construction keeps the option
	// visible in exactly one place.
	if fg.hasPointerOption(fd) && fd.Kind() == protoreflect.MessageKind {
		pf := fg.fieldType(fd)
		types.AddTypeImports(fg, pf)
		pf.EmitUnmarshal(fg, access, ctx)
		return
	}

	types.AddTypeImports(fg, t)
	t.EmitUnmarshal(fg, access, ctx)
}

// emitWireTypeCheck emits a check that the wire type matches the expected type
// for a given proto kind, skipping the field if it doesn't match.
func (fg *FileGenerator) emitWireTypeCheck(kind protoreflect.Kind) {
	wtInt := types.WireTypeInt(kind)
	fmt.Fprintf(fg.body, "\t\t\tif wireType != %d {\n", wtInt)
	fmt.Fprintf(fg.body, "\t\t\t\tn, err := skipValue(dAtA[iNdEx:], wireType, fieldNum)\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif err != nil {\n\t\t\t\t\treturn err\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tiNdEx += n\n")
	fmt.Fprintf(fg.body, "\t\t\t\tcontinue\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
}
