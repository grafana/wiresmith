package generator

import (
	"fmt"
	"wiresmith/compiler/types"

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
		rf := &types.RepeatedField{Inner: t, IsPacked: fd.IsPacked()}
		types.AddTypeImports(fg, rf)
		rf.EmitUnmarshal(fg, access, ctx)
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
		if kind == protoreflect.MessageKind {
			fmt.Fprintf(fg.body, "\t\t\tvar msg %s\n", ctx.MessageType)
			if ctx.IsSamePackage {
				fmt.Fprintf(fg.body, "\t\t\tif err := msg.unmarshal(v, depth+1); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n")
			} else {
				fmt.Fprintf(fg.body, "\t\t\tif err := msg.Unmarshal(v); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n")
			}
			fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: msg}\n", ooFieldName, variantName, fieldName)
		} else {
			fmt.Fprintf(fg.body, "\t\t\tm.%s = &%s{%s: %s}\n", ooFieldName, variantName, fieldName, t.CastExpr("v", ctx))
		}
		fmt.Fprintf(fg.body, "\t\t\tb = b[n:]\n")
		return
	}

	if fd.HasOptionalKeyword() {
		of := &types.OptionalField{Inner: t}
		types.AddTypeImports(fg, of)
		of.EmitUnmarshal(fg, access, ctx)
		return
	}

	types.AddTypeImports(fg, t)
	t.EmitUnmarshal(fg, access, ctx)
}

// emitWireTypeCheck emits a check that the wire type matches the expected type
// for a given proto kind, skipping the field if it doesn't match.
func (fg *FileGenerator) emitWireTypeCheck(kind protoreflect.Kind) {
	wt := types.Get(kind).WireType()
	fmt.Fprintf(fg.body, "\t\t\tif typ != %s {\n", wt)
	fmt.Fprintf(fg.body, "\t\t\t\tn, err := skipField(b, num, typ)\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif err != nil {\n\t\t\t\t\treturn err\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tb = b[n:]\n")
	fmt.Fprintf(fg.body, "\t\t\t\tcontinue\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
}
