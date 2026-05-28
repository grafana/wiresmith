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

// Oneof unmarshal is handled by the generator (per-variant case), not by the
// composite — the composite's EmitUnmarshal must panic to catch accidental
// direct calls.
func TestOneofField_EmitUnmarshal_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on direct EmitUnmarshal call, got none")
		}
	}()
	(&OneofField{Inner: BoolType{}}).EmitUnmarshal(&captureEmitter{}, "x", FieldContext{})
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
