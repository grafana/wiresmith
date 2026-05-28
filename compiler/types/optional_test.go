package types

import (
	"strings"
	"testing"
)

// EmitSize wraps the inner's value-size in a nil-guard. OptionalAccess on a
// scalar dereferences (e.g. `*m.X`) because the field is a *T.
func TestOptionalField_EmitSize_ScalarNilGuard(t *testing.T) {
	e := &captureEmitter{}
	(&OptionalField{Inner: varintBase{}}).EmitSize(e, "m.X", 1)
	got := e.buf.String()
	if !strings.Contains(got, "if m.X != nil {") {
		t.Errorf("EmitSize: missing nil-guard:\n%s", got)
	}
	// Inner.EmitValueSize sees the dereferenced access.
	if !strings.Contains(got, "n += 1 + protowire.SizeVarint(uint64(*m.X))") {
		t.Errorf("EmitSize: missing dereferenced value-size add:\n%s", got)
	}
}

// Bytes is "already nullable" — OptionalAccess returns access unchanged.
// EmitSize still nil-guards; the value path sees raw []byte.
func TestOptionalField_EmitSize_BytesNoDereference(t *testing.T) {
	e := &captureEmitter{}
	(&OptionalField{Inner: &BytesType{}}).EmitSize(e, "m.X", 1)
	got := e.buf.String()
	if !strings.Contains(got, "if m.X != nil {") {
		t.Errorf("EmitSize: missing nil-guard for bytes:\n%s", got)
	}
	// No dereference: bytes already-nullable means OptionalAccess is identity.
	if strings.Contains(got, "*m.X") {
		t.Errorf("EmitSize: bytes optional must NOT dereference:\n%s", got)
	}
}

// EmitMarshal must register the inner's imports on the emitter.
func TestOptionalField_EmitMarshal_RegistersInnerImports(t *testing.T) {
	e := &captureEmitter{}
	(&OptionalField{Inner: Fixed64Type}).EmitMarshal(e, "m.X", 1)
	if len(e.imports) == 0 {
		t.Fatalf("EmitMarshal: expected inner imports to be registered, got none")
	}
}

// Unmarshal for already-nullable types (bytes) routes through inner.EmitUnmarshal
// directly. For optional bytes, the generator also normalises an empty result
// to `[]byte{}` so presence survives the marshal round-trip — without this,
// `append([]byte(nil)[:0], ...)` evaluates to nil and looks indistinguishable
// from "field absent".
func TestOptionalField_EmitUnmarshal_BytesNormalizesEmpty(t *testing.T) {
	e := &captureEmitter{}
	(&OptionalField{Inner: &BytesType{}}).EmitUnmarshal(e, "m.X", FieldContext{})
	got := e.buf.String()
	if !strings.Contains(got, "if m.X == nil {") {
		t.Errorf("EmitUnmarshal: missing post-decode nil-normalisation guard:\n%s", got)
	}
	if !strings.Contains(got, "m.X = []byte{}") {
		t.Errorf("EmitUnmarshal: must normalise nil to empty slice for presence preservation:\n%s", got)
	}
}

// Value-typed scalar (varint) optional: CastExpr returns identity ("v"), so
// the generator emits `m.X = &v` directly (no temp local).
func TestOptionalField_EmitUnmarshal_ScalarIdentityCast(t *testing.T) {
	e := &captureEmitter{}
	(&OptionalField{Inner: varintBase{unmarshalCast: "%s"}}).EmitUnmarshal(e, "m.X", FieldContext{})
	got := e.buf.String()
	if !strings.Contains(got, "m.X = &v") {
		t.Errorf("EmitUnmarshal: identity cast must emit `m.X = &v`:\n%s", got)
	}
	if strings.Contains(got, "tmp := v") {
		t.Errorf("EmitUnmarshal: identity cast must not allocate `tmp`:\n%s", got)
	}
}

// Non-identity cast (int32 etc.) must bind a tmp local because `&int32(v)`
// is not addressable in Go.
func TestOptionalField_EmitUnmarshal_ScalarCastUsesTmp(t *testing.T) {
	e := &captureEmitter{}
	(&OptionalField{Inner: varintBase{unmarshalCast: "int32(%s)"}}).EmitUnmarshal(e, "m.X", FieldContext{})
	got := e.buf.String()
	if !strings.Contains(got, "tmp := int32(v)") {
		t.Errorf("EmitUnmarshal: non-identity cast must declare `tmp`:\n%s", got)
	}
	if !strings.Contains(got, "m.X = &tmp") {
		t.Errorf("EmitUnmarshal: must assign &tmp:\n%s", got)
	}
}

// Length-delimited optional (e.g. optional string): the post-consume slice
// is dAtA[iNdEx:postIndex], cast (here string(...)) and stored via &tmp.
func TestOptionalField_EmitUnmarshal_StringUsesPostIndexSlice(t *testing.T) {
	e := &captureEmitter{}
	(&OptionalField{Inner: StringType{}}).EmitUnmarshal(e, "m.X", FieldContext{})
	got := e.buf.String()
	if !strings.Contains(got, "tmp := string(dAtA[iNdEx:postIndex])") {
		t.Errorf("EmitUnmarshal: string optional must use postIndex slice:\n%s", got)
	}
	if !strings.Contains(got, "iNdEx = postIndex") {
		t.Errorf("EmitUnmarshal: missing iNdEx advance:\n%s", got)
	}
}

func TestOptionalField_RequiredImports_PassesThrough(t *testing.T) {
	got := (&OptionalField{Inner: Fixed64Type}).RequiredImports()
	if len(got) == 0 {
		t.Fatalf("RequiredImports() = empty; want inner imports")
	}
}
