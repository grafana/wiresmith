package basic

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	ks "github.com/grafana/wiresmith/gen/test/kitchensink/v1"
)

// --- Float bit-exact equality (CR-5) ---
//
// Equal() compares float fields by their bit pattern, not by Go's `==`
// operator. This matches the official google.golang.org/protobuf proto.Equal
// contract — and also wiresmith's own marshal path, which preserves -0.0 and
// arbitrary NaN bit patterns instead of canonicalizing them.

func TestEqual_NaNFloat64_SameBits(t *testing.T) {
	// Two messages with the same NaN bit pattern must compare equal.
	// Without bit-exact comparison, `NaN != NaN` (per IEEE 754) makes the
	// guard always fire — even when both sides are the same NaN value.
	bits := uint64(0x7ff8000000000001) // a quiet NaN with payload 1
	a := &ks.AllScalars{FieldDouble: math.Float64frombits(bits)}
	b := &ks.AllScalars{FieldDouble: math.Float64frombits(bits)}
	assert.True(t, a.Equal(b), "two messages with the same NaN bit pattern must be equal")
}

func TestEqual_NaNFloat32_SameBits(t *testing.T) {
	bits := uint32(0x7fc00001)
	a := &ks.AllScalars{FieldFloat: math.Float32frombits(bits)}
	b := &ks.AllScalars{FieldFloat: math.Float32frombits(bits)}
	assert.True(t, a.Equal(b), "two messages with the same float32 NaN bit pattern must be equal")
}

func TestEqual_NaNFloat64_DifferentBits(t *testing.T) {
	// Different NaN bit patterns must NOT be equal under bit-exact comparison.
	a := &ks.AllScalars{FieldDouble: math.Float64frombits(0x7ff8000000000001)}
	b := &ks.AllScalars{FieldDouble: math.Float64frombits(0x7ff8000000000002)}
	assert.False(t, a.Equal(b), "NaNs with distinct bit patterns must not be equal")
}

func TestEqual_NaNFloat32_DifferentBits(t *testing.T) {
	a := &ks.AllScalars{FieldFloat: math.Float32frombits(0x7fc00001)}
	b := &ks.AllScalars{FieldFloat: math.Float32frombits(0x7fc00002)}
	assert.False(t, a.Equal(b), "float32 NaNs with distinct bit patterns must not be equal")
}

func TestEqual_NegativeZeroFloat64(t *testing.T) {
	// -0.0 and +0.0 compare equal under IEEE 754 `==`, but bit-exact
	// comparison distinguishes them. This matches the marshal path: -0.0
	// roundtrips as -0.0 while +0.0 is omitted as a default, so they cannot
	// be considered equal without breaking the marshal/Equal contract.
	a := &ks.AllScalars{FieldDouble: math.Copysign(0, -1)}
	b := &ks.AllScalars{FieldDouble: 0}
	assert.False(t, a.Equal(b), "-0.0 and +0.0 must compare unequal (bit-exact)")
}

func TestEqual_NegativeZeroFloat32(t *testing.T) {
	a := &ks.AllScalars{FieldFloat: float32(math.Copysign(0, -1))}
	b := &ks.AllScalars{FieldFloat: 0}
	assert.False(t, a.Equal(b), "-0.0 and +0.0 (float32) must compare unequal (bit-exact)")
}

func TestEqual_NilReceiver(t *testing.T) {
	var a *ks.AllScalars
	var b *ks.AllScalars
	assert.True(t, a.Equal(b), "two nil pointers must be equal")
	assert.True(t, a.Equal(nil), "nil pointer and nil interface must be equal")
}

func TestEqual_NilVsZero(t *testing.T) {
	var a *ks.AllScalars
	b := &ks.AllScalars{}
	assert.False(t, a.Equal(b), "nil pointer must not equal zero value")
	assert.False(t, b.Equal(a), "zero value must not equal nil pointer")
	assert.False(t, b.Equal(nil), "zero value must not equal nil interface")
}

func TestEqual_PointerAndValue(t *testing.T) {
	a := &ks.AllScalars{FieldInt32: 42}
	b := ks.AllScalars{FieldInt32: 42}
	assert.True(t, a.Equal(b), "pointer and value with same fields must be equal")
}

func TestEqual_WrongType(t *testing.T) {
	a := &ks.AllScalars{FieldInt32: 42}
	assert.False(t, a.Equal("not a proto"), "must return false for wrong type")
	assert.False(t, a.Equal(42), "must return false for wrong type")
}

func TestEqual_DifferentScalar(t *testing.T) {
	a := &ks.AllScalars{FieldInt32: 1}
	b := &ks.AllScalars{FieldInt32: 2}
	assert.False(t, a.Equal(b))
}

func TestEqual_Bytes_NonOptional(t *testing.T) {
	// For non-optional bytes, nil and empty are both the zero value.
	a := &ks.AllScalars{FieldBytes: nil}
	b := &ks.AllScalars{FieldBytes: []byte{}}
	assert.True(t, a.Equal(b), "non-optional bytes: nil and empty are the same zero value")
}

func TestEqual_Bytes_Optional_NilVsEmpty(t *testing.T) {
	// For optional bytes, nil means "unset" and []byte{} means "set to empty".
	a := &ks.AllOptionalScalars{FieldBytes: nil}
	b := &ks.AllOptionalScalars{FieldBytes: []byte{}}
	assert.False(t, a.Equal(b), "optional bytes: nil (unset) must not equal empty (set)")
	assert.False(t, b.Equal(a))
}

func TestEqual_Bytes_Optional_BothNil(t *testing.T) {
	a := &ks.AllOptionalScalars{FieldBytes: nil}
	b := &ks.AllOptionalScalars{FieldBytes: nil}
	assert.True(t, a.Equal(b))
}

func TestEqual_Bytes_Optional_BothEmpty(t *testing.T) {
	a := &ks.AllOptionalScalars{FieldBytes: []byte{}}
	b := &ks.AllOptionalScalars{FieldBytes: []byte{}}
	assert.True(t, a.Equal(b))
}

func TestEqual_Bytes_Optional_SameContent(t *testing.T) {
	a := &ks.AllOptionalScalars{FieldBytes: []byte{0x01, 0x02}}
	b := &ks.AllOptionalScalars{FieldBytes: []byte{0x01, 0x02}}
	assert.True(t, a.Equal(b))
}

// --- Oneof Equal ---

func TestEqual_Oneof_BothUnset(t *testing.T) {
	a := &ks.OneofVariants{Value: nil}
	b := &ks.OneofVariants{Value: nil}
	assert.True(t, a.Equal(b))
}

func TestEqual_Oneof_OneUnset(t *testing.T) {
	a := &ks.OneofVariants{Value: nil}
	b := &ks.OneofVariants{Value: &ks.OneofVariants_Int32Value{Int32Value: 1}}
	assert.False(t, a.Equal(b))
	assert.False(t, b.Equal(a))
}

func TestEqual_Oneof_SameTypeSameValue(t *testing.T) {
	// Two independently allocated variants with the same value must be equal.
	a := &ks.OneofVariants{Value: &ks.OneofVariants_StringValue{StringValue: "hello"}}
	b := &ks.OneofVariants{Value: &ks.OneofVariants_StringValue{StringValue: "hello"}}
	assert.True(t, a.Equal(b), "same oneof variant type and value must be equal")
}

func TestEqual_Oneof_SameTypeDifferentValue(t *testing.T) {
	a := &ks.OneofVariants{Value: &ks.OneofVariants_Int64Value{Int64Value: 1}}
	b := &ks.OneofVariants{Value: &ks.OneofVariants_Int64Value{Int64Value: 2}}
	assert.False(t, a.Equal(b))
}

func TestEqual_Oneof_DifferentTypes(t *testing.T) {
	a := &ks.OneofVariants{Value: &ks.OneofVariants_Int32Value{Int32Value: 42}}
	b := &ks.OneofVariants{Value: &ks.OneofVariants_Int64Value{Int64Value: 42}}
	assert.False(t, a.Equal(b), "different oneof variant types must not be equal")
}

func TestEqual_Oneof_BytesVariant(t *testing.T) {
	a := &ks.OneofVariants{Value: &ks.OneofVariants_BytesValue{BytesValue: []byte{0x01}}}
	b := &ks.OneofVariants{Value: &ks.OneofVariants_BytesValue{BytesValue: []byte{0x01}}}
	assert.True(t, a.Equal(b))

	c := &ks.OneofVariants{Value: &ks.OneofVariants_BytesValue{BytesValue: []byte{0x02}}}
	assert.False(t, a.Equal(c))
}

func TestEqual_Oneof_AfterUnmarshal(t *testing.T) {
	// The core bug scenario: two messages created via independent unmarshals
	// must compare equal if they have the same oneof content.
	src := &ks.OneofVariants{Value: &ks.OneofVariants_StringValue{StringValue: "test"}}
	b, err := src.Marshal()
	assert.NoError(t, err)

	var dst1, dst2 ks.OneofVariants
	assert.NoError(t, dst1.Unmarshal(b))
	assert.NoError(t, dst2.Unmarshal(b))
	assert.True(t, dst1.Equal(&dst2), "independently unmarshaled messages must be equal")
}

// --- Nested message Equal ---

func TestEqual_NestedMessage(t *testing.T) {
	a := &ks.Outer{
		Middle: ks.Middle{Inner: ks.Inner{Data: "same"}},
		Name:   "outer",
	}
	b := &ks.Outer{
		Middle: ks.Middle{Inner: ks.Inner{Data: "same"}},
		Name:   "outer",
	}
	assert.True(t, a.Equal(b))

	c := &ks.Outer{
		Middle: ks.Middle{Inner: ks.Inner{Data: "different"}},
		Name:   "outer",
	}
	assert.False(t, a.Equal(c))
}

// --- Repeated Equal ---

func TestEqual_RepeatedLength(t *testing.T) {
	a := &ks.OnlyRepeated{Names: []string{"a", "b"}}
	b := &ks.OnlyRepeated{Names: []string{"a"}}
	assert.False(t, a.Equal(b))
}

func TestEqual_RepeatedContent(t *testing.T) {
	a := &ks.OnlyRepeated{Names: []string{"a", "b"}}
	b := &ks.OnlyRepeated{Names: []string{"a", "c"}}
	assert.False(t, a.Equal(b))
}

func TestEqual_RepeatedNilVsEmpty(t *testing.T) {
	a := &ks.OnlyRepeated{Names: nil}
	b := &ks.OnlyRepeated{Names: []string{}}
	assert.True(t, a.Equal(b), "nil and empty slice are both zero value for repeated")
}

// --- Map Equal ---

func TestEqual_MapDifferentKeys(t *testing.T) {
	a := &ks.AllMaps{MapStringString: map[string]string{"a": "1"}}
	b := &ks.AllMaps{MapStringString: map[string]string{"b": "1"}}
	assert.False(t, a.Equal(b))
}

func TestEqual_MapDifferentValues(t *testing.T) {
	a := &ks.AllMaps{MapStringString: map[string]string{"a": "1"}}
	b := &ks.AllMaps{MapStringString: map[string]string{"a": "2"}}
	assert.False(t, a.Equal(b))
}

func TestEqual_MapSame(t *testing.T) {
	a := &ks.AllMaps{MapStringString: map[string]string{"a": "1", "b": "2"}}
	b := &ks.AllMaps{MapStringString: map[string]string{"b": "2", "a": "1"}}
	assert.True(t, a.Equal(b))
}

func TestEqual_MapNilVsEmpty(t *testing.T) {
	a := &ks.AllMaps{MapStringString: nil}
	b := &ks.AllMaps{MapStringString: map[string]string{}}
	assert.True(t, a.Equal(b), "nil and empty map are both zero value")
}

func TestEqual_MapMessageValue(t *testing.T) {
	a := &ks.AllMaps{MapStringMessage: map[string]ks.Inner{
		"k": {Data: "v", SignedVal: 1},
	}}
	b := &ks.AllMaps{MapStringMessage: map[string]ks.Inner{
		"k": {Data: "v", SignedVal: 1},
	}}
	assert.True(t, a.Equal(b))

	c := &ks.AllMaps{MapStringMessage: map[string]ks.Inner{
		"k": {Data: "v", SignedVal: 2},
	}}
	assert.False(t, a.Equal(c))
}

func TestEqual_MapBytesValue(t *testing.T) {
	a := &ks.AllMaps{MapStringBytes: map[string][]byte{"k": {0x01}}}
	b := &ks.AllMaps{MapStringBytes: map[string][]byte{"k": {0x01}}}
	assert.True(t, a.Equal(b))

	c := &ks.AllMaps{MapStringBytes: map[string][]byte{"k": {0x02}}}
	assert.False(t, a.Equal(c))
}

// --- Empty message Equal ---

func TestEqual_Empty(t *testing.T) {
	a := &ks.Empty{}
	b := &ks.Empty{}
	assert.True(t, a.Equal(b))
	assert.True(t, a.Equal(ks.Empty{}), "pointer vs value")
}

// --- Container with oneof ---

func TestEqual_ContainerOneof(t *testing.T) {
	a := &ks.Container{
		Variant: ks.OneofVariants{Value: &ks.OneofVariants_BoolValue{BoolValue: true}},
	}
	b := &ks.Container{
		Variant: ks.OneofVariants{Value: &ks.OneofVariants_BoolValue{BoolValue: true}},
	}
	assert.True(t, a.Equal(b))

	c := &ks.Container{
		Variant: ks.OneofVariants{Value: &ks.OneofVariants_BoolValue{BoolValue: false}},
	}
	assert.False(t, a.Equal(c))
}
