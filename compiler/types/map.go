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
	_, isMsg := m.Val.(*MessageType)

	emitConsumeBytesLen(e)

	e.Writef("\t\t\tif %s == nil {\n", access)
	e.Writef("\t\t\t\t%s = make(%s)\n", access, m.MapType)
	e.Writef("\t\t\t}\n")
	e.Writef("\t\t\tvar mapkey %s\n", m.KeyGoType)
	e.Writef("\t\t\tvar mapvalue %s\n", m.ValGoType)
	if isMsg {
		e.Writef("\t\t\tvar mapValueBytes []byte\n")
	}

	// Index-based iteration: reuse iNdEx directly instead of creating
	// an entryData sub-slice, and inline tag decode to avoid non-inlined
	// protowire.ConsumeTag calls.
	e.AddImport("io", "")
	e.Writef("\t\t\tfor iNdEx < postIndex {\n")

	EmitConsumeTagAt(e, "\t\t\t\t", "entryWire")
	e.Writef("\t\t\t\tswitch int32(entryWire >> 3) {\n")

	// Key (field 1)
	e.Writef("\t\t\t\tcase 1:\n")
	emitMapEntryWireTypeCheck(e, m.Key.WireType())
	m.Key.EmitMapEntryUnmarshal(e, "mapkey", "\t\t\t\t\t", m.KeyCtx)

	// Value (field 2)
	e.Writef("\t\t\t\tcase 2:\n")
	emitMapEntryWireTypeCheck(e, m.Val.WireType())
	m.Val.EmitMapEntryUnmarshal(e, "mapvalue", "\t\t\t\t\t", m.ValCtx)
	if isMsg {
		// Save raw value bytes for merge semantics when the same
		// key appears in multiple wire entries.
		e.Writef("\t\t\t\t\tmapValueBytes = dAtA[mapValueStart:iNdEx]\n")
	}

	// Unknown fields
	e.Writef("\t\t\t\tdefault:\n")
	e.Writef("\t\t\t\t\tn, err := skipValue(dAtA[iNdEx:], int(entryWire&0x7), int32(entryWire>>3))\n")
	e.Writef("\t\t\t\t\tif err != nil {\n\t\t\t\t\t\treturn err\n\t\t\t\t\t}\n")
	e.Writef("\t\t\t\t\tiNdEx += n\n")

	e.Writef("\t\t\t\t}\n") // end switch
	e.Writef("\t\t\t}\n")   // end for

	if isMsg {
		// Merge semantics: if the key already exists and the value field
		// was present (even if empty), merge into the existing message.
		// When value is absent (mapValueBytes == nil) and key exists,
		// preserve the existing entry per proto merge rules.
		e.Writef("\t\t\tif existing, ok := %s[mapkey]; ok && mapValueBytes != nil {\n", access)
		if m.ValCtx.IsSamePackage {
			e.Writef("\t\t\t\tif err := existing.unmarshal(mapValueBytes, depth+1); err != nil {\n\t\t\t\t\treturn err\n\t\t\t\t}\n")
		} else {
			// Cross-package merge: thread depth through UnmarshalWithDepth.
			// Calling the public Unmarshal here would reset depth at the
			// boundary, re-opening the SEC-5 hole that the rest of the
			// codegen closes for cross-package message access (and which
			// MessageType.EmitMapEntryUnmarshal closes for the *initial*
			// map-entry decode). An attacker who can place duplicate keys
			// on the wire could otherwise get a fresh depth budget per
			// merge and ping-pong indefinitely. See wiresmith-1c0.
			e.Writef("\t\t\t\tif err := existing.UnmarshalWithDepth(mapValueBytes, depth+1); err != nil {\n\t\t\t\t\treturn err\n\t\t\t\t}\n")
		}
		e.Writef("\t\t\t\t%s[mapkey] = existing\n", access)
		e.Writef("\t\t\t} else if !ok {\n")
		e.Writef("\t\t\t\t%s[mapkey] = mapvalue\n", access)
		e.Writef("\t\t\t}\n")
	} else {
		e.Writef("\t\t\t%s[mapkey] = mapvalue\n", access)
	}
	e.Writef("\t\t\tiNdEx = postIndex\n")
}

// EmitEqual emits a len-check + range over lhs with per-key lookup in rhs,
// delegating per-value comparison to Val.EmitEqual. Message values use
// `.Equal()`, bytes use `bytes.Equal`, scalars use `!=`.
func (m *MapField) EmitEqual(e Emitter, indent, lhs, rhs string) {
	e.Writef("%sif len(%s) != len(%s) {\n%s\treturn false\n%s}\n", indent, lhs, rhs, indent, indent)
	e.Writef("%sfor k, v := range %s {\n", indent, lhs)
	e.Writef("%s\tv2, ok := %s[k]\n", indent, rhs)
	e.Writef("%s\tif !ok {\n%s\t\treturn false\n%s\t}\n", indent, indent, indent)
	m.Val.EmitEqual(e, indent+"\t", "v", "v2")
	e.Writef("%s}\n", indent)
}

// EmitCompare gives maps a total ordering by walking both sides in sorted
// key order:
//
//  1. Length wins (shorter sorts first).
//  2. Build sorted key slices for both lhs and rhs.
//  3. At each position compare the keys first — if they differ, that's the
//     ordering (the side with the earlier-sorting key sorts first).
//  4. If the keys match, compare the corresponding values.
//
// Wrapped in `{ ... }` so the `ks1`/`ks2`/`v1`/`v2` locals don't collide
// with anything in the enclosing Compare body or a sibling map field. Map
// values are extracted into locals because map-indexed expressions aren't
// addressable, so a pointer-receiver `.Compare` on a message-by-value
// would otherwise refuse to compile.
func (m *MapField) EmitCompare(e Emitter, indent, lhs, rhs string) {
	emitLenOrderingGuard(e, indent, lhs, rhs)
	e.AddImport("sort", "")
	lessFn := "func(i, j int) bool { return ks%s[i] < ks%s[j] }"
	if _, isBool := m.Key.(*BoolType); isBool {
		// Go forbids `<` on bool; emit the equivalent ordering (false < true)
		// as `!a && b` so map<bool, V> still gets a total order.
		lessFn = "func(i, j int) bool { return !ks%s[i] && ks%s[j] }"
	}
	e.Writef("%s{\n", indent)
	e.Writef("%s\tks1 := make([]%s, 0, len(%s))\n", indent, m.KeyGoType, lhs)
	e.Writef("%s\tfor k := range %s {\n%s\t\tks1 = append(ks1, k)\n%s\t}\n", indent, lhs, indent, indent)
	e.Writef("%s\tsort.Slice(ks1, "+lessFn+")\n", indent, "1", "1")
	e.Writef("%s\tks2 := make([]%s, 0, len(%s))\n", indent, m.KeyGoType, rhs)
	e.Writef("%s\tfor k := range %s {\n%s\t\tks2 = append(ks2, k)\n%s\t}\n", indent, rhs, indent, indent)
	e.Writef("%s\tsort.Slice(ks2, "+lessFn+")\n", indent, "2", "2")
	e.Writef("%s\tfor i := range ks1 {\n", indent)
	m.Key.EmitCompare(e, indent+"\t\t", "ks1[i]", "ks2[i]")
	e.Writef("%s\t\tv1 := %s[ks1[i]]\n", indent, lhs)
	e.Writef("%s\t\tv2 := %s[ks2[i]]\n", indent, rhs)
	m.Val.EmitCompare(e, indent+"\t\t", "v1", "v2")
	e.Writef("%s\t}\n", indent)
	e.Writef("%s}\n", indent)
}

// emitMapEntryWireTypeCheck emits a wire type guard for a map entry field.
// Uses entryWire (from inline tag decode) and skipValue (index-based skip).
func emitMapEntryWireTypeCheck(e Emitter, wt string) {
	// Extract wire type constant's numeric value for comparison
	e.Writef("\t\t\t\t\tif int(entryWire&0x7) != int(%s) {\n", wt)
	e.Writef("\t\t\t\t\t\tn, err := skipValue(dAtA[iNdEx:], int(entryWire&0x7), int32(entryWire>>3))\n")
	e.Writef("\t\t\t\t\t\tif err != nil {\n\t\t\t\t\t\t\treturn err\n\t\t\t\t\t\t}\n")
	e.Writef("\t\t\t\t\t\tiNdEx += n\n")
	e.Writef("\t\t\t\t\t\tcontinue\n")
	e.Writef("\t\t\t\t\t}\n")
}
