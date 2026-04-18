package types

import "google.golang.org/protobuf/encoding/protowire"

// MapField is a composite type for proto map fields.
// Key/Val are the scalar types for the map's key and value.
// The caller must set MapType, KeyGoType, ValGoType, KeyCtx, ValCtx
// before calling EmitUnmarshal.
type MapField struct {
	Key, Val  Type
	MapType   string       // full Go map type, e.g. "map[string]Resource"
	KeyGoType string       // Go type for key, e.g. "string"
	ValGoType string       // Go type for value, e.g. "Resource"
	KeyCtx    FieldContext // FieldContext for key unmarshal
	ValCtx    FieldContext // FieldContext for value unmarshal
}

func (m *MapField) RequiredImports() []string {
	var imps []string
	imps = append(imps, m.Key.RequiredImports()...)
	imps = append(imps, m.Val.RequiredImports()...)
	return imps
}

func (m *MapField) EmitSize(e Emitter, access string, tagSize int) {
	keyTagSize := protowire.SizeTag(1)
	valTagSize := protowire.SizeTag(2)

	keyUsed := m.Key.FixedSize() == 0
	valUsed := m.Val.FixedSize() == 0
	switch {
	case keyUsed && valUsed:
		e.Writef("\tfor k, v := range %s {\n", access)
	case keyUsed:
		e.Writef("\tfor k := range %s {\n", access)
	case valUsed:
		e.Writef("\tfor _, v := range %s {\n", access)
	default:
		e.Writef("\tfor range %s {\n", access)
	}
	e.Writef("\t\tentrySize := 0\n")

	m.Key.EmitValueSize(e, "\t\t", "k", keyTagSize, "entrySize")
	m.Val.EmitValueSize(e, "\t\t", "v", valTagSize, "entrySize")

	e.Writef("\t\tn += %d + protowire.SizeVarint(uint64(entrySize)) + entrySize\n", tagSize)
	e.Writef("\t}\n")
}

func (m *MapField) EmitMarshal(e Emitter, access string, num protowire.Number) {
	AddTypeImports(e, m.Key)
	AddTypeImports(e, m.Val)

	e.Writef("\tfor k, v := range %s {\n", access)

	// Reverse-write: value first (field 2), then key (field 1),
	// then entry length, then map field tag.
	e.Writef("\t\tbaseI := i\n")

	// Write value (field 2)
	m.Val.EmitValueMarshal(e, "\t\t", "v", 2)
	// Write key (field 1)
	m.Key.EmitValueMarshal(e, "\t\t", "k", 1)

	// Entry length and map field tag
	e.Writef("\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(baseI-i))\n")
	e.ReverseTag("\t\t", num, protowire.BytesType)
	e.Writef("\t}\n")
}

func (m *MapField) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeBytes(e)

	e.Writef("\t\t\tif %s == nil {\n", access)
	e.Writef("\t\t\t\t%s = make(%s)\n", access, m.MapType)
	e.Writef("\t\t\t}\n")
	e.Writef("\t\t\tvar mapkey %s\n", m.KeyGoType)
	e.Writef("\t\t\tvar mapvalue %s\n", m.ValGoType)
	e.Writef("\t\t\tentryData := v\n")
	e.Writef("\t\t\tfor len(entryData) > 0 {\n")
	e.Writef("\t\t\t\tentryNum, entryTyp, entryTagLen := protowire.ConsumeTag(entryData)\n")
	e.Writef("\t\t\t\tif entryTagLen < 0 {\n\t\t\t\t\treturn fmt.Errorf(\"invalid map entry tag\")\n\t\t\t\t}\n")
	e.Writef("\t\t\t\tentryData = entryData[entryTagLen:]\n")
	e.Writef("\t\t\t\tswitch entryNum {\n")

	// Key (field 1)
	e.Writef("\t\t\t\tcase 1:\n")
	m.emitMapEntryWireTypeCheck(e, m.Key.WireType())
	m.Key.EmitMapEntryUnmarshal(e, "mapkey", "\t\t\t\t\t", m.KeyCtx)

	// Value (field 2)
	e.Writef("\t\t\t\tcase 2:\n")
	m.emitMapEntryWireTypeCheck(e, m.Val.WireType())
	m.Val.EmitMapEntryUnmarshal(e, "mapvalue", "\t\t\t\t\t", m.ValCtx)

	// Unknown fields
	e.Writef("\t\t\t\tdefault:\n")
	e.Writef("\t\t\t\t\tskipN, skipErr := skipField(entryData, entryNum, entryTyp)\n")
	e.Writef("\t\t\t\t\tif skipErr != nil {\n\t\t\t\t\t\treturn skipErr\n\t\t\t\t\t}\n")
	e.Writef("\t\t\t\t\tentryData = entryData[skipN:]\n")

	e.Writef("\t\t\t\t}\n") // end switch
	e.Writef("\t\t\t}\n")   // end for
	e.Writef("\t\t\t%s[mapkey] = mapvalue\n", access)

	emitAdvanceBytes(e)
}

func (m *MapField) emitMapEntryWireTypeCheck(e Emitter, wt string) {
	e.Writef("\t\t\t\t\tif entryTyp != %s {\n", wt)
	e.Writef("\t\t\t\t\t\tskipN, skipErr := skipField(entryData, entryNum, entryTyp)\n")
	e.Writef("\t\t\t\t\t\tif skipErr != nil {\n\t\t\t\t\t\t\treturn skipErr\n\t\t\t\t\t\t}\n")
	e.Writef("\t\t\t\t\t\tentryData = entryData[skipN:]\n")
	e.Writef("\t\t\t\t\t\tcontinue\n")
	e.Writef("\t\t\t\t\t}\n")
}
