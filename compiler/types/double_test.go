package types

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func doubleType(t *testing.T) *fixed64Base {
	t.Helper()
	d, ok := Get(protoreflect.DoubleKind).(*fixed64Base)
	if !ok {
		t.Fatalf("DoubleKind not registered as *fixed64Base: %T", Get(protoreflect.DoubleKind))
	}
	return d
}

func TestDouble_RequiredImports(t *testing.T) {
	d := doubleType(t)
	imps := d.RequiredImports()
	got := make(map[string]bool, len(imps))
	for _, imp := range imps {
		got[imp] = true
	}
	for _, want := range []string{"encoding/binary", "math"} {
		if !got[want] {
			t.Errorf("RequiredImports() missing %q (got %v)", want, imps)
		}
	}
}

// Mirror of TestFloat_EmitMarshal_SignbitAware for double-precision: -0.0
// is suppressed by `v != 0` because Go's `==` treats it as +0.0.
func TestDouble_EmitMarshal_SignbitAware(t *testing.T) {
	e := &captureEmitter{}
	doubleType(t).EmitMarshal(e, "m.X", 1)
	got := e.buf.String()
	if !strings.Contains(got, "v := math.Float64bits(m.X); v != 0") {
		t.Errorf("EmitMarshal must compare math.Float64bits against 0:\n%s", got)
	}
	if !strings.Contains(got, "binary.LittleEndian.PutUint64(dAtA[i:], v)") {
		t.Errorf("EmitMarshal: PutUint64 must consume cached `v`:\n%s", got)
	}
	if strings.Count(got, "math.Float64bits") != 1 {
		t.Errorf("EmitMarshal: math.Float64bits should appear exactly once; got:\n%s", got)
	}
}

func TestDouble_EmitSize_SignbitAware(t *testing.T) {
	e := &captureEmitter{}
	doubleType(t).EmitSize(e, "m.X", 1)
	got := e.buf.String()
	want := "\tif math.Float64bits(m.X) != 0 {\n\t\tn += 9\n\t}\n"
	if got != want {
		t.Errorf("EmitSize:\n got: %q\nwant: %q", got, want)
	}
}

func TestDouble_EmitEqual_BitExact(t *testing.T) {
	e := &captureEmitter{}
	doubleType(t).EmitEqual(e, "\t", "a", "b")
	want := "\tif math.Float64bits(a) != math.Float64bits(b) {\n\t\treturn false\n\t}\n"
	if got := e.buf.String(); got != want {
		t.Errorf("EmitEqual:\n got: %q\nwant: %q", got, want)
	}
}

func TestDouble_EmitUnmarshal_UsesFloat64frombits(t *testing.T) {
	e := &captureEmitter{}
	doubleType(t).EmitUnmarshal(e, "m.X", FieldContext{})
	got := e.buf.String()
	if !strings.Contains(got, "m.X = math.Float64frombits(v)") {
		t.Errorf("EmitUnmarshal: missing math.Float64frombits cast:\n%s", got)
	}
}

func TestDouble_CastExpr(t *testing.T) {
	got := doubleType(t).CastExpr("v", FieldContext{})
	want := "math.Float64frombits(v)"
	if got != want {
		t.Errorf("CastExpr = %q, want %q", got, want)
	}
}
