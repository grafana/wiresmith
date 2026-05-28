package types

import (
	"strings"
	"testing"
)

func TestSint32Type_Classification(t *testing.T) {
	s := Sint32Type{}
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

// Zigzag size: encoded bytes track the absolute magnitude, so SizeVarint
// runs on the zigzag-encoded form, not the raw signed value. The expression
// must include `protowire.EncodeZigZag(int64(access))`.
func TestSint32Type_VarintSizeExpr_Zigzag(t *testing.T) {
	got := Sint32Type{}.VarintSizeExpr("m.X")
	want := "protowire.SizeVarint(protowire.EncodeZigZag(int64(m.X)))"
	if got != want {
		t.Errorf("VarintSizeExpr = %q, want %q", got, want)
	}
}

// Marshal zigzag: `(n<<1) ^ (n>>31)`. Done inline rather than via
// protowire.EncodeZigZag so the shift/xor stays inlinable.
func TestSint32Type_EmitMarshal_InlineZigzag(t *testing.T) {
	e := &captureEmitter{}
	Sint32Type{}.EmitMarshal(e, "m.X", 1)
	got := e.buf.String()
	want := "uint64(uint32(m.X<<1)^uint32(int32(m.X)>>31))"
	if !strings.Contains(got, want) {
		t.Errorf("EmitMarshal: missing inline zigzag %q:\n%s", want, got)
	}
}

// Unmarshal zigzag: `int32(v>>1) ^ int32(v)<<31>>31`. The trailing
// `<<31>>31` sign-extends bit 0 of v across all 32 bits.
func TestSint32Type_EmitUnmarshal_InlineZigzagDecode(t *testing.T) {
	e := &captureEmitter{}
	Sint32Type{}.EmitUnmarshal(e, "m.X", FieldContext{})
	got := e.buf.String()
	want := "m.X = int32(uint32(v)>>1) ^ int32(uint32(v))<<31>>31"
	if !strings.Contains(got, want) {
		t.Errorf("EmitUnmarshal: missing inline zigzag decode %q:\n%s", want, got)
	}
}

func TestSint32Type_CastExpr(t *testing.T) {
	got := Sint32Type{}.CastExpr("v", FieldContext{})
	want := "int32(uint32(v)>>1) ^ int32(uint32(v))<<31>>31"
	if got != want {
		t.Errorf("CastExpr = %q, want %q", got, want)
	}
}

// EmitEncode is called for packed-repeated emission. When the access form is
// a pointer dereference (`*v`), it must first bind to a local — otherwise the
// emitted expression evaluates the deref twice (once for shift, once for xor).
func TestSint32Type_EmitEncode_PointerAccess_BindsLocal(t *testing.T) {
	e := &captureEmitter{}
	Sint32Type{}.EmitEncode(e, "\t", "*p")
	got := e.buf.String()
	if !strings.Contains(got, "v := *p") {
		t.Errorf("EmitEncode: pointer access must bind a local to avoid double-deref:\n%s", got)
	}
	// Subsequent shift expressions should reference `v`, not `*p`.
	if strings.Contains(got, "*p<<1") || strings.Contains(got, "*p>>31") {
		t.Errorf("EmitEncode: must not double-evaluate pointer deref:\n%s", got)
	}
}
