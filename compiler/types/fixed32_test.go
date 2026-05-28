package types

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestFixed32Type_Classification(t *testing.T) {
	f := Fixed32Type
	if got := f.WireType(); got != "protowire.Fixed32Type" {
		t.Errorf("WireType() = %q, want protowire.Fixed32Type", got)
	}
	if !f.IsPackable() {
		t.Error("IsPackable() = false, want true (fixed32 is packable)")
	}
	if !f.IsFixed32() || f.IsFixed64() {
		t.Error("classification: want IsFixed32=true, IsFixed64=false")
	}
	if got := f.FixedSize(); got != 4 {
		t.Errorf("FixedSize() = %d, want 4", got)
	}
	if f.SizeByIndex() {
		t.Error("SizeByIndex() = true, want false")
	}
	if got := f.OptionalAccess("x"); got != "*x" {
		t.Errorf("OptionalAccess(%q) = %q, want %q", "x", got, "*x")
	}
	if got := f.ZeroLiteral(); got != "0" {
		t.Errorf("ZeroLiteral() = %q, want %q", got, "0")
	}
}

func TestFixed32Type_VarintSizeExpr_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for fixed-width VarintSizeExpr, got none")
		}
	}()
	Fixed32Type.VarintSizeExpr("x")
}

// Default fixed32 (Fixed32, Sfixed32): predicate is `access != 0`.
// Critically, float-style nonzeroExpr override must NOT leak into the bare
// fixed32 path; the bit-aware form is only correct for IEEE-754 floats.
func TestFixed32Type_EmitMarshal_PlainPredicate(t *testing.T) {
	e := &captureEmitter{}
	Fixed32Type.EmitMarshal(e, "m.X", 1)
	got := e.buf.String()
	if !strings.Contains(got, "if m.X != 0 {") {
		t.Errorf("EmitMarshal: missing plain non-zero predicate `if m.X != 0`:\n%s", got)
	}
	if strings.Contains(got, "math.Float32bits") {
		t.Errorf("EmitMarshal on bare fixed32 must not reference math.Float32bits:\n%s", got)
	}
}

func TestFixed32Type_EmitSize_PlainPredicate(t *testing.T) {
	e := &captureEmitter{}
	Fixed32Type.EmitSize(e, "m.X", 1)
	want := "\tif m.X != 0 {\n\t\tn += 5\n\t}\n"
	if got := e.buf.String(); got != want {
		t.Errorf("EmitSize:\n got: %q\nwant: %q", got, want)
	}
}

// EmitValueSize is the unconditional size add used by packed/map encoders —
// no zero-guard, just `target += tagSize+4`.
func TestFixed32Type_EmitValueSize(t *testing.T) {
	e := &captureEmitter{}
	Fixed32Type.EmitValueSize(e, "\t\t", "v", 1, "entrySize")
	want := "\t\tentrySize += 5\n"
	if got := e.buf.String(); got != want {
		t.Errorf("EmitValueSize:\n got: %q\nwant: %q", got, want)
	}
}

// Sfixed32 is registered with putExpr `uint32(%s)` and getExpr `int32(%s)`,
// so EmitUnmarshal must cast the consumed uint32 back to int32 for storage.
func TestSfixed32_RegisteredUnmarshalCast(t *testing.T) {
	sf32, ok := Get(protoreflect.Sfixed32Kind).(*fixed32Base)
	if !ok {
		t.Fatalf("Sfixed32Kind not registered as *fixed32Base: %T", Get(protoreflect.Sfixed32Kind))
	}
	e := &captureEmitter{}
	sf32.EmitUnmarshal(e, "m.X", FieldContext{})
	if got := e.buf.String(); !strings.Contains(got, "m.X = int32(v)") {
		t.Errorf("sfixed32 EmitUnmarshal: missing int32 cast:\n%s", got)
	}
}

func TestSfixed32_RegisteredCastExpr(t *testing.T) {
	sf32 := Get(protoreflect.Sfixed32Kind)
	if got := sf32.CastExpr("v", FieldContext{}); got != "int32(v)" {
		t.Errorf("sfixed32 CastExpr = %q, want %q", got, "int32(v)")
	}
}
