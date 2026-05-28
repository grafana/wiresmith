package types

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// floatType returns the Float kind's registered Type (a *fixed32Base with
// float-specific put/get/nonzero/equal overrides).
func floatType(t *testing.T) *fixed32Base {
	t.Helper()
	f, ok := Get(protoreflect.FloatKind).(*fixed32Base)
	if !ok {
		t.Fatalf("FloatKind not registered as *fixed32Base: %T", Get(protoreflect.FloatKind))
	}
	return f
}

func TestFloat_RequiredImports(t *testing.T) {
	f := floatType(t)
	imps := f.RequiredImports()
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

// CR-2 (wiresmith-wl6): a singular float field whose value is -0.0 must
// survive marshal. Go's `v != 0` is false for -0.0, so the emitter must use
// bit-level comparison via math.Float32bits — otherwise round-trip silently
// strips the sign bit. The bit-aware predicate also subsumes the EmitMarshal
// CSE local `v := math.Float32bits(...)`, so we check both.
func TestFloat_EmitMarshal_SignbitAware(t *testing.T) {
	e := &captureEmitter{}
	floatType(t).EmitMarshal(e, "m.X", 1)
	got := e.buf.String()
	if !strings.Contains(got, "v := math.Float32bits(m.X); v != 0") {
		t.Errorf("EmitMarshal must compare math.Float32bits against 0 (preserves -0.0):\n%s", got)
	}
	// PutUint32 must consume the cached bit-pattern local, not re-call Float32bits.
	if !strings.Contains(got, "binary.LittleEndian.PutUint32(dAtA[i:], v)") {
		t.Errorf("EmitMarshal: PutUint32 must consume cached `v` from the predicate:\n%s", got)
	}
	if strings.Count(got, "math.Float32bits") != 1 {
		t.Errorf("EmitMarshal: math.Float32bits should appear exactly once (CSE local); got:\n%s", got)
	}
}

func TestFloat_EmitSize_SignbitAware(t *testing.T) {
	e := &captureEmitter{}
	floatType(t).EmitSize(e, "m.X", 1)
	got := e.buf.String()
	want := "\tif math.Float32bits(m.X) != 0 {\n\t\tn += 5\n\t}\n"
	if got != want {
		t.Errorf("EmitSize:\n got: %q\nwant: %q", got, want)
	}
}

// CR-5 (wiresmith-tqo): Equal must compare bit patterns so that NaN==NaN
// and -0.0 != +0.0 match google.golang.org/protobuf's proto.Equal.
func TestFloat_EmitEqual_BitExact(t *testing.T) {
	e := &captureEmitter{}
	floatType(t).EmitEqual(e, "\t", "a", "b")
	want := "\tif math.Float32bits(a) != math.Float32bits(b) {\n\t\treturn false\n\t}\n"
	if got := e.buf.String(); got != want {
		t.Errorf("EmitEqual:\n got: %q\nwant: %q", got, want)
	}
}

func TestFloat_EmitUnmarshal_UsesFloat32frombits(t *testing.T) {
	e := &captureEmitter{}
	floatType(t).EmitUnmarshal(e, "m.X", FieldContext{})
	got := e.buf.String()
	if !strings.Contains(got, "m.X = math.Float32frombits(v)") {
		t.Errorf("EmitUnmarshal: missing math.Float32frombits cast:\n%s", got)
	}
}

func TestFloat_CastExpr(t *testing.T) {
	got := floatType(t).CastExpr("v", FieldContext{})
	want := "math.Float32frombits(v)"
	if got != want {
		t.Errorf("CastExpr = %q, want %q", got, want)
	}
}
