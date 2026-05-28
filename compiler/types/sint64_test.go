package types

import (
	"strings"
	"testing"
)

func TestSint64Type_Classification(t *testing.T) {
	s := Sint64Type{}
	if got := s.WireType(); got != "protowire.VarintType" {
		t.Errorf("WireType() = %q, want protowire.VarintType", got)
	}
	if !s.IsPackable() {
		t.Error("IsPackable() = false, want true")
	}
	if got := s.FixedSize(); got != 0 {
		t.Errorf("FixedSize() = %d, want 0", got)
	}
	if got := s.ZeroLiteral(); got != "0" {
		t.Errorf("ZeroLiteral() = %q, want %q", got, "0")
	}
}

func TestSint64Type_VarintSizeExpr_Zigzag(t *testing.T) {
	got := Sint64Type{}.VarintSizeExpr("m.X")
	want := "protowire.SizeVarint(protowire.EncodeZigZag(m.X))"
	if got != want {
		t.Errorf("VarintSizeExpr = %q, want %q", got, want)
	}
}

func TestSint64Type_EmitMarshal_InlineZigzag(t *testing.T) {
	e := &captureEmitter{}
	Sint64Type{}.EmitMarshal(e, "m.X", 1)
	got := e.buf.String()
	want := "uint64(uint64(m.X<<1)^uint64(int64(m.X)>>63))"
	if !strings.Contains(got, want) {
		t.Errorf("EmitMarshal: missing inline zigzag %q:\n%s", want, got)
	}
}

func TestSint64Type_EmitUnmarshal_InlineZigzagDecode(t *testing.T) {
	e := &captureEmitter{}
	Sint64Type{}.EmitUnmarshal(e, "m.X", FieldContext{})
	got := e.buf.String()
	want := "m.X = int64(v>>1) ^ int64(v)<<63>>63"
	if !strings.Contains(got, want) {
		t.Errorf("EmitUnmarshal: missing inline zigzag decode %q:\n%s", want, got)
	}
}

func TestSint64Type_CastExpr(t *testing.T) {
	got := Sint64Type{}.CastExpr("v", FieldContext{})
	want := "int64(v>>1) ^ int64(v)<<63>>63"
	if got != want {
		t.Errorf("CastExpr = %q, want %q", got, want)
	}
}

func TestSint64Type_EmitEncode_PointerAccess_BindsLocal(t *testing.T) {
	e := &captureEmitter{}
	Sint64Type{}.EmitEncode(e, "\t", "*p")
	got := e.buf.String()
	if !strings.Contains(got, "v := *p") {
		t.Errorf("EmitEncode: pointer access must bind a local to avoid double-deref:\n%s", got)
	}
}
