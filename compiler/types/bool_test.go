package types

import (
	"strings"
	"testing"
)

func TestBoolType_Classification(t *testing.T) {
	b := BoolType{}
	if got := b.WireType(); got != "protowire.VarintType" {
		t.Errorf("WireType() = %q, want protowire.VarintType", got)
	}
	if !b.IsPackable() {
		t.Error("IsPackable() = false, want true (bool is packable like other varints)")
	}
	if b.IsFixed32() || b.IsFixed64() {
		t.Error("classification: bool is not fixed-width despite FixedSize()==1")
	}
	// Bool's "fixed size" is 1 byte: SizeVarint(0)==SizeVarint(1)==1. This
	// lets RepeatedField use the constant-size encoding path.
	if got := b.FixedSize(); got != 1 {
		t.Errorf("FixedSize() = %d, want 1 (bool wire form is always 1 byte)", got)
	}
	if b.SizeByIndex() {
		t.Error("SizeByIndex() = true, want false")
	}
	if got := b.ZeroLiteral(); got != "false" {
		t.Errorf("ZeroLiteral() = %q, want %q", got, "false")
	}
	if got := b.OptionalAccess("x"); got != "*x" {
		t.Errorf("OptionalAccess(%q) = %q, want %q", "x", got, "*x")
	}
	if got := b.VarintSizeExpr("anything"); got != "1" {
		t.Errorf("VarintSizeExpr returns constant %q, want %q (bool is always 1 byte)", got, "1")
	}
}

func TestBoolType_EmitSize(t *testing.T) {
	e := &captureEmitter{}
	BoolType{}.EmitSize(e, "m.X", 1)
	want := "\tif m.X {\n\t\tn += 2\n\t}\n"
	if got := e.buf.String(); got != want {
		t.Errorf("EmitSize:\n got: %q\nwant: %q", got, want)
	}
}

// EmitMarshal wraps the body in `if m.X { ... }`, so a false bool field is
// elided from the wire. The inner `else { dAtA[i] = 0 }` that EmitEncode
// generates is unreachable for the singular case — we intentionally do NOT
// assert the false-byte branch here, so dead-code cleanup remains free.
// The false branch is locked down separately via EmitEncode (which is the
// public surface called from packed-repeated contexts where it is reachable).
func TestBoolType_EmitMarshal_TruePath(t *testing.T) {
	e := &captureEmitter{}
	BoolType{}.EmitMarshal(e, "m.X", 1)
	got := e.buf.String()
	for _, sub := range []string{
		"if m.X {",
		"i--",
		"dAtA[i] = 1",
	} {
		if !strings.Contains(got, sub) {
			t.Errorf("EmitMarshal: missing %q in:\n%s", sub, got)
		}
	}
}

// EmitEncode is the bare encoding form (no zero-guard) used by packed loops.
// Both branches of the bool-to-byte conversion are reachable here.
func TestBoolType_EmitEncode_BothBranches(t *testing.T) {
	e := &captureEmitter{}
	BoolType{}.EmitEncode(e, "\t", "m.X")
	got := e.buf.String()
	for _, sub := range []string{
		"i--",
		"if m.X {",
		"dAtA[i] = 1",
		"dAtA[i] = 0",
	} {
		if !strings.Contains(got, sub) {
			t.Errorf("EmitEncode: missing %q in:\n%s", sub, got)
		}
	}
}

// Decode reads a varint into v, and bool stores `v != 0`. Even a multi-byte
// varint with non-zero high bits must end up `true`.
func TestBoolType_EmitUnmarshal(t *testing.T) {
	e := &captureEmitter{}
	BoolType{}.EmitUnmarshal(e, "m.X", FieldContext{})
	got := e.buf.String()
	if !strings.Contains(got, "m.X = v != 0") {
		t.Errorf("EmitUnmarshal: missing `m.X = v != 0`:\n%s", got)
	}
}

func TestBoolType_CastExpr(t *testing.T) {
	got := BoolType{}.CastExpr("v", FieldContext{})
	if got != "v != 0" {
		t.Errorf("CastExpr = %q, want %q", got, "v != 0")
	}
}
