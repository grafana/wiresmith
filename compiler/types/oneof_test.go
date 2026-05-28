package types

import (
	"slices"
	"strings"
	"testing"
)

// EmitSize on a oneof variant must NOT emit a zero-guard: the caller already
// proved the variant is selected by type-switch. There are three code paths:
//   - FixedSize != 0 (bool, fixed32/64, sfixed*, float, double): emit
//     `_ = access` (so the variant is observed) then unconditional add.
//   - BytesType wire, !packable, !SizeByIndex (string, bytes): emit a `l`
//     local to share between SizeVarint and the length add.
//   - Everything else (varint scalars, sint*, enum, message): delegate to
//     EmitValueSize without the temp.
func TestOneofField_EmitSize_Bool_FixedSize(t *testing.T) {
	e := &captureEmitter{}
	(&OneofField{Inner: BoolType{}}).EmitSize(e, "x.Value", 1)
	got := e.buf.String()
	// FixedSize=1 branch: discard access (so `x.Value` is referenced even
	// though the size is constant), then `n += tagSize+1`.
	if !strings.Contains(got, "_ = x.Value") {
		t.Errorf("FixedSize branch must emit access-discard:\n%s", got)
	}
	if !strings.Contains(got, "n += 2") {
		t.Errorf("FixedSize branch: expected `n += 2` (tagSize=1 + size=1):\n%s", got)
	}
}

func TestOneofField_EmitSize_Fixed32_FixedSize(t *testing.T) {
	e := &captureEmitter{}
	(&OneofField{Inner: Fixed32Type}).EmitSize(e, "x.Value", 1)
	got := e.buf.String()
	if !strings.Contains(got, "_ = x.Value") {
		t.Errorf("FixedSize branch must emit access-discard:\n%s", got)
	}
	if !strings.Contains(got, "n += 5") {
		t.Errorf("FixedSize branch: expected `n += 5` (tagSize=1 + size=4):\n%s", got)
	}
}

func TestOneofField_EmitSize_Fixed64_FixedSize(t *testing.T) {
	e := &captureEmitter{}
	(&OneofField{Inner: Fixed64Type}).EmitSize(e, "x.Value", 1)
	got := e.buf.String()
	if !strings.Contains(got, "_ = x.Value") {
		t.Errorf("FixedSize branch must emit access-discard:\n%s", got)
	}
	if !strings.Contains(got, "n += 9") {
		t.Errorf("FixedSize branch: expected `n += 9` (tagSize=1 + size=8):\n%s", got)
	}
}

// String oneof: BytesType-wire, non-packable, !SizeByIndex → temp-local branch.
func TestOneofField_EmitSize_String_BytesTypeBranch(t *testing.T) {
	e := &captureEmitter{}
	(&OneofField{Inner: StringType{}}).EmitSize(e, "x.Value", 1)
	got := e.buf.String()
	want := "\t\tl := len(x.Value)\n\t\tn += 1 + protowire.SizeVarint(uint64(l)) + l\n"
	if got != want {
		t.Errorf("StringType branch:\n got: %q\nwant: %q", got, want)
	}
}

// Bytes oneof: same temp-local branch as string.
func TestOneofField_EmitSize_Bytes_BytesTypeBranch(t *testing.T) {
	e := &captureEmitter{}
	(&OneofField{Inner: &BytesType{}}).EmitSize(e, "x.Value", 1)
	got := e.buf.String()
	want := "\t\tl := len(x.Value)\n\t\tn += 1 + protowire.SizeVarint(uint64(l)) + l\n"
	if got != want {
		t.Errorf("BytesType branch:\n got: %q\nwant: %q", got, want)
	}
}

// MessageType is BytesType-wire but SizeByIndex=true, so it must NOT take
// the temp-local branch — it falls through to EmitValueSize, which uses
// `s := access.Size()`.
func TestOneofField_EmitSize_Message_FallthroughBranch(t *testing.T) {
	e := &captureEmitter{}
	(&OneofField{Inner: &MessageType{}}).EmitSize(e, "x.Value", 1)
	got := e.buf.String()
	if !strings.Contains(got, "s := x.Value.Size()") {
		t.Errorf("MessageType must take the EmitValueSize fall-through (no temp-local):\n%s", got)
	}
	if strings.Contains(got, "l := len(x.Value)") {
		t.Errorf("MessageType must NOT use the bytes-temp branch (SizeByIndex=true):\n%s", got)
	}
}

// Varint scalars (int32, uint64, etc.) fall through to EmitValueSize because
// FixedSize==0 AND they aren't BytesType-wire.
func TestOneofField_EmitSize_Varint_FallthroughBranch(t *testing.T) {
	e := &captureEmitter{}
	(&OneofField{Inner: varintBase{unmarshalCast: "int32(%s)"}}).EmitSize(e, "x.Value", 1)
	got := e.buf.String()
	want := "\t\tn += 1 + protowire.SizeVarint(uint64(x.Value))\n"
	if got != want {
		t.Errorf("varint fallthrough:\n got: %q\nwant: %q", got, want)
	}
}

// EmitMarshal delegates to the inner's EmitValueMarshal. The inner's
// imports must also be registered on the emitter.
func TestOneofField_EmitMarshal_DelegatesAndRegistersImports(t *testing.T) {
	e := &captureEmitter{}
	(&OneofField{Inner: Fixed64Type}).EmitMarshal(e, "x.Value", 1)
	got := e.buf.String()
	if !strings.Contains(got, "binary.LittleEndian.PutUint64(dAtA[i:], x.Value)") {
		t.Errorf("EmitMarshal must delegate to inner.EmitValueMarshal:\n%s", got)
	}
	if !slices.Contains(e.imports, "encoding/binary") {
		t.Errorf("EmitMarshal must register inner's imports; got %v", e.imports)
	}
}

// EmitUnmarshal value-type branch (varint, fixed): cast EmitConsume's `v`
// local and assign through the variant wrapper. No iNdEx advance — the
// inline consume helpers already advanced it.
func TestOneofField_EmitUnmarshal_ValueType(t *testing.T) {
	e := &captureEmitter{}
	of := &OneofField{
		Inner:       BoolType{},
		OneofName:   "Status",
		VariantName: "Span_BoolValue",
		FieldName:   "BoolValue",
	}
	of.EmitUnmarshal(e, "m.Status", FieldContext{})
	got := e.buf.String()
	want := "m.Status = &Span_BoolValue{BoolValue: v != 0}"
	if !strings.Contains(got, want) {
		t.Errorf("value-type oneof: missing variant wrapping %q in:\n%s", want, got)
	}
	if strings.Contains(got, "iNdEx = postIndex") {
		t.Errorf("value-type oneof must not advance iNdEx to postIndex (no length-delim payload):\n%s", got)
	}
}

// EmitUnmarshal length-delimited non-message branch (string, bytes): cast
// the dAtA[iNdEx:postIndex] payload, assign into variant, then advance iNdEx.
func TestOneofField_EmitUnmarshal_StringVariant(t *testing.T) {
	e := &captureEmitter{}
	of := &OneofField{
		Inner:       StringType{},
		OneofName:   "Status",
		VariantName: "Span_StringValue",
		FieldName:   "StringValue",
	}
	of.EmitUnmarshal(e, "m.Status", FieldContext{})
	got := e.buf.String()
	if !strings.Contains(got, "m.Status = &Span_StringValue{StringValue: string(dAtA[iNdEx:postIndex])}") {
		t.Errorf("string oneof: missing slice cast / variant wrap:\n%s", got)
	}
	if !strings.Contains(got, "iNdEx = postIndex") {
		t.Errorf("string oneof: must advance iNdEx after consuming payload:\n%s", got)
	}
}

// EmitUnmarshal message branch: reuse-or-replace merge into the existing
// variant. Same-package callees use the private `.unmarshal(..., depth+1)`
// so the SEC-5 depth counter threads through.
func TestOneofField_EmitUnmarshal_MessageVariant_SamePackage(t *testing.T) {
	e := &captureEmitter{}
	of := &OneofField{
		Inner:       &MessageType{},
		OneofName:   "Body",
		VariantName: "LogRecord_Message",
		FieldName:   "Message",
	}
	of.EmitUnmarshal(e, "m.Body", FieldContext{MessageType: "InnerMsg", IsSamePackage: true})
	got := e.buf.String()
	// Reuse-or-replace merge: pull existing variant's message if same variant.
	if !strings.Contains(got, "var msg InnerMsg") {
		t.Errorf("message oneof: missing reusable msg local:\n%s", got)
	}
	if !strings.Contains(got, "if ov, ok := m.Body.(*LogRecord_Message); ok {") {
		t.Errorf("message oneof: missing reuse-variant type switch:\n%s", got)
	}
	if !strings.Contains(got, "msg = ov.Message") {
		t.Errorf("message oneof: must copy existing variant's message into local:\n%s", got)
	}
	if !strings.Contains(got, "msg.unmarshal(dAtA[iNdEx:postIndex], depth+1)") {
		t.Errorf("message oneof same-package: must call private unmarshal w/ depth:\n%s", got)
	}
	if !strings.Contains(got, "m.Body = &LogRecord_Message{Message: msg}") {
		t.Errorf("message oneof: missing final variant assignment:\n%s", got)
	}
}

// Cross-package message oneof variant threads depth via UnmarshalWithDepth.
func TestOneofField_EmitUnmarshal_MessageVariant_CrossPackage(t *testing.T) {
	e := &captureEmitter{}
	of := &OneofField{
		Inner:       &MessageType{},
		OneofName:   "Body",
		VariantName: "LogRecord_Message",
		FieldName:   "Message",
	}
	of.EmitUnmarshal(e, "m.Body", FieldContext{MessageType: "external.InnerMsg", IsSamePackage: false})
	got := e.buf.String()
	if !strings.Contains(got, "msg.UnmarshalWithDepth(dAtA[iNdEx:postIndex], depth+1)") {
		t.Errorf("message oneof cross-package: must call UnmarshalWithDepth:\n%s", got)
	}
	if strings.Contains(got, "msg.unmarshal(") {
		t.Errorf("message oneof cross-package: must NOT call private unmarshal:\n%s", got)
	}
}

func TestOneofField_RequiredImports_PassesThrough(t *testing.T) {
	got := (&OneofField{Inner: Fixed64Type}).RequiredImports()
	if len(got) == 0 {
		t.Fatalf("RequiredImports() = empty; want inner imports")
	}
	if !slices.Contains(got, "encoding/binary") {
		t.Errorf("RequiredImports() = %v; want to include encoding/binary", got)
	}
}
