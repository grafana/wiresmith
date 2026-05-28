package types

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestVarintBase_Classification(t *testing.T) {
	v := varintBase{}
	if got := v.WireType(); got != "protowire.VarintType" {
		t.Errorf("WireType() = %q, want protowire.VarintType", got)
	}
	if !v.IsPackable() {
		t.Error("IsPackable() = false, want true")
	}
	if v.IsFixed32() || v.IsFixed64() {
		t.Error("varint must not classify as fixed-width")
	}
	if got := v.FixedSize(); got != 0 {
		t.Errorf("FixedSize() = %d, want 0", got)
	}
	if v.SizeByIndex() {
		t.Error("SizeByIndex() = true, want false")
	}
	if got := v.ZeroLiteral(); got != "0" {
		t.Errorf("ZeroLiteral() = %q, want %q", got, "0")
	}
	if got := v.OptionalAccess("x"); got != "*x" {
		t.Errorf("OptionalAccess(%q) = %q, want %q", "x", got, "*x")
	}
	if got := v.VarintSizeExpr("x"); got != "protowire.SizeVarint(uint64(x))" {
		t.Errorf("VarintSizeExpr = %q, want %q", got, "protowire.SizeVarint(uint64(x))")
	}
}

func TestVarintBase_EmitSize(t *testing.T) {
	e := &captureEmitter{}
	varintBase{}.EmitSize(e, "m.X", 1)
	want := "\tif m.X != 0 {\n\t\tn += 1 + protowire.SizeVarint(uint64(m.X))\n\t}\n"
	if got := e.buf.String(); got != want {
		t.Errorf("EmitSize:\n got: %q\nwant: %q", got, want)
	}
}

func TestVarintBase_EmitMarshal(t *testing.T) {
	e := &captureEmitter{}
	varintBase{}.EmitMarshal(e, "m.X", 1)
	got := e.buf.String()
	for _, sub := range []string{
		"if m.X != 0 {",
		"i = protohelpers.EncodeVarint(dAtA, i, uint64(m.X))",
	} {
		if !strings.Contains(got, sub) {
			t.Errorf("EmitMarshal missing %q:\n%s", sub, got)
		}
	}
}

// Each varint kind registers its own cast format. EmitUnmarshal must use it.
func TestVarintKinds_RegisteredCasts(t *testing.T) {
	cases := []struct {
		kind protoreflect.Kind
		want string // CastExpr result on "v"
	}{
		{protoreflect.Int32Kind, "int32(v)"},
		{protoreflect.Int64Kind, "int64(v)"},
		{protoreflect.Uint32Kind, "uint32(v)"},
		{protoreflect.Uint64Kind, "v"},
	}
	for _, c := range cases {
		t.Run(c.kind.String(), func(t *testing.T) {
			tp := Get(c.kind)
			if got := tp.CastExpr("v", FieldContext{}); got != c.want {
				t.Errorf("%v CastExpr = %q, want %q", c.kind, got, c.want)
			}
			e := &captureEmitter{}
			tp.EmitUnmarshal(e, "m.X", FieldContext{})
			if assign := "m.X = " + c.want + "\n"; !strings.HasSuffix(e.buf.String(), assign) {
				t.Errorf("%v EmitUnmarshal: missing trailing assign %q in:\n%s", c.kind, assign, e.buf.String())
			}
		})
	}
}

// Uint64 uses the identity cast — no `uint64(...)` wrapping. Locked down so
// future "make all casts explicit" cleanups don't reintroduce the redundant
// conversion that go vet flags.
func TestUint64_IdentityCast(t *testing.T) {
	u := Get(protoreflect.Uint64Kind)
	if got := u.CastExpr("v", FieldContext{}); got != "v" {
		t.Errorf("uint64 CastExpr = %q, want plain %q (no redundant uint64() wrap)", got, "v")
	}
}
