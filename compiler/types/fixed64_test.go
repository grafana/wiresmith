package types

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestFixed64Type_Classification(t *testing.T) {
	f := Fixed64Type
	if got := f.WireType(); got != "protowire.Fixed64Type" {
		t.Errorf("WireType() = %q, want protowire.Fixed64Type", got)
	}
	if !f.IsPackable() {
		t.Error("IsPackable() = false, want true")
	}
	if f.IsFixed32() || !f.IsFixed64() {
		t.Error("classification: want IsFixed32=false, IsFixed64=true")
	}
	if got := f.FixedSize(); got != 8 {
		t.Errorf("FixedSize() = %d, want 8", got)
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

func TestFixed64Type_VarintSizeExpr_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic, got none")
		}
	}()
	Fixed64Type.VarintSizeExpr("x")
}

func TestFixed64Type_EmitMarshal_PlainPredicate(t *testing.T) {
	e := &captureEmitter{}
	Fixed64Type.EmitMarshal(e, "m.X", 1)
	got := e.buf.String()
	if !strings.Contains(got, "if m.X != 0 {") {
		t.Errorf("EmitMarshal: missing plain non-zero predicate:\n%s", got)
	}
	if strings.Contains(got, "math.Float64bits") {
		t.Errorf("EmitMarshal on bare fixed64 must not reference math.Float64bits:\n%s", got)
	}
}

func TestFixed64Type_EmitSize_PlainPredicate(t *testing.T) {
	e := &captureEmitter{}
	Fixed64Type.EmitSize(e, "m.X", 1)
	want := "\tif m.X != 0 {\n\t\tn += 9\n\t}\n"
	if got := e.buf.String(); got != want {
		t.Errorf("EmitSize:\n got: %q\nwant: %q", got, want)
	}
}

func TestFixed64Type_EmitValueSize(t *testing.T) {
	e := &captureEmitter{}
	Fixed64Type.EmitValueSize(e, "\t\t", "v", 1, "entrySize")
	want := "\t\tentrySize += 9\n"
	if got := e.buf.String(); got != want {
		t.Errorf("EmitValueSize:\n got: %q\nwant: %q", got, want)
	}
}

func TestSfixed64_RegisteredUnmarshalCast(t *testing.T) {
	sf64, ok := Get(protoreflect.Sfixed64Kind).(*fixed64Base)
	if !ok {
		t.Fatalf("Sfixed64Kind not registered as *fixed64Base: %T", Get(protoreflect.Sfixed64Kind))
	}
	e := &captureEmitter{}
	sf64.EmitUnmarshal(e, "m.X", FieldContext{})
	if got := e.buf.String(); !strings.Contains(got, "m.X = int64(v)") {
		t.Errorf("sfixed64 EmitUnmarshal: missing int64 cast:\n%s", got)
	}
}

func TestSfixed64_RegisteredCastExpr(t *testing.T) {
	sf64 := Get(protoreflect.Sfixed64Kind)
	if got := sf64.CastExpr("v", FieldContext{}); got != "int64(v)" {
		t.Errorf("sfixed64 CastExpr = %q, want %q", got, "int64(v)")
	}
}
