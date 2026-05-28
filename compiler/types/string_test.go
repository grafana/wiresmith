package types

import (
	"strings"
	"testing"
)

func TestStringType_Classification(t *testing.T) {
	s := StringType{}
	if got := s.WireType(); got != "protowire.BytesType" {
		t.Errorf("WireType() = %q, want protowire.BytesType", got)
	}
	if s.IsPackable() {
		t.Error("IsPackable() = true, want false (length-delimited types are not packable)")
	}
	if s.IsFixed32() || s.IsFixed64() {
		t.Error("string must not classify as fixed-width")
	}
	if got := s.FixedSize(); got != 0 {
		t.Errorf("FixedSize() = %d, want 0", got)
	}
	if s.SizeByIndex() {
		t.Error("SizeByIndex() = true, want false")
	}
	if got := s.ZeroLiteral(); got != `""` {
		t.Errorf("ZeroLiteral() = %q, want %q", got, `""`)
	}
	if got := s.OptionalAccess("x"); got != "*x" {
		t.Errorf("OptionalAccess(%q) = %q, want %q", "x", got, "*x")
	}
}

func TestStringType_VarintSizeExpr_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic, got none")
		}
	}()
	StringType{}.VarintSizeExpr("x")
}

func TestStringType_EmitSize(t *testing.T) {
	e := &captureEmitter{}
	StringType{}.EmitSize(e, "m.S", 1)
	want := "\tif len(m.S) > 0 {\n\t\tn += 1 + protowire.SizeVarint(uint64(len(m.S))) + len(m.S)\n\t}\n"
	if got := e.buf.String(); got != want {
		t.Errorf("EmitSize:\n got: %q\nwant: %q", got, want)
	}
}

func TestStringType_EmitMarshal(t *testing.T) {
	e := &captureEmitter{}
	StringType{}.EmitMarshal(e, "m.S", 1)
	got := e.buf.String()
	for _, sub := range []string{
		"if len(m.S) > 0 {",
		"i -= len(m.S)",
		"copy(dAtA[i:], m.S)",
		"i = protohelpers.EncodeVarint(dAtA, i, uint64(len(m.S)))",
	} {
		if !strings.Contains(got, sub) {
			t.Errorf("EmitMarshal: missing %q in:\n%s", sub, got)
		}
	}
}

// Decode aliases the input byte slice via `string(dAtA[iNdEx:postIndex])`.
// Go's runtime makes the conversion a copy, so there's no aliasing risk
// here (unlike bytes which keeps backing-array identity).
func TestStringType_EmitUnmarshal_DirectStringConversion(t *testing.T) {
	e := &captureEmitter{}
	StringType{}.EmitUnmarshal(e, "m.S", FieldContext{})
	got := e.buf.String()
	if !strings.Contains(got, "m.S = string(dAtA[iNdEx:postIndex])") {
		t.Errorf("EmitUnmarshal: missing string(...) conversion:\n%s", got)
	}
}

func TestStringType_CastExpr(t *testing.T) {
	got := StringType{}.CastExpr("v", FieldContext{})
	if got != "string(v)" {
		t.Errorf("CastExpr = %q, want %q", got, "string(v)")
	}
}
