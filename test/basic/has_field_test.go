package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"

	num "github.com/grafana/wiresmith/gen/basic/numeric/v1"
	ks "github.com/grafana/wiresmith/gen/test/kitchensink/v1"
)

func TestHasField_ScalarsAllSet(t *testing.T) {
	msg := &ks.AllScalars{
		FieldDouble:   3.14,
		FieldFloat:    2.71,
		FieldInt32:    -42,
		FieldInt64:    -100000,
		FieldUint32:   42,
		FieldUint64:   100000,
		FieldSint32:   -12345,
		FieldSint64:   -9876543210,
		FieldFixed32:  0xDEAD,
		FieldFixed64:  0xDEADBEEFCAFE,
		FieldSfixed32: -99999,
		FieldSfixed64: -1234567890123,
		FieldBool:     true,
		FieldString:   "hello",
		FieldBytes:    []byte{0xCA, 0xFE},
	}

	b, err := msg.Marshal()
	require.NoError(t, err)

	var decoded ks.AllScalars
	require.NoError(t, decoded.Unmarshal(b))

	assert.True(t, decoded.HasFieldDouble())
	assert.True(t, decoded.HasFieldFloat())
	assert.True(t, decoded.HasFieldInt32())
	assert.True(t, decoded.HasFieldInt64())
	assert.True(t, decoded.HasFieldUint32())
	assert.True(t, decoded.HasFieldUint64())
	assert.True(t, decoded.HasFieldSint32())
	assert.True(t, decoded.HasFieldSint64())
	assert.True(t, decoded.HasFieldFixed32())
	assert.True(t, decoded.HasFieldFixed64())
	assert.True(t, decoded.HasFieldSfixed32())
	assert.True(t, decoded.HasFieldSfixed64())
	assert.True(t, decoded.HasFieldBool())
	assert.True(t, decoded.HasFieldString())
	assert.True(t, decoded.HasFieldBytes())
}

func TestHasField_ScalarsNoneSet(t *testing.T) {
	// Empty message: no fields present in wire format.
	var decoded ks.AllScalars
	require.NoError(t, decoded.Unmarshal(nil))

	assert.False(t, decoded.HasFieldDouble())
	assert.False(t, decoded.HasFieldFloat())
	assert.False(t, decoded.HasFieldInt32())
	assert.False(t, decoded.HasFieldInt64())
	assert.False(t, decoded.HasFieldUint32())
	assert.False(t, decoded.HasFieldUint64())
	assert.False(t, decoded.HasFieldSint32())
	assert.False(t, decoded.HasFieldSint64())
	assert.False(t, decoded.HasFieldFixed32())
	assert.False(t, decoded.HasFieldFixed64())
	assert.False(t, decoded.HasFieldSfixed32())
	assert.False(t, decoded.HasFieldSfixed64())
	assert.False(t, decoded.HasFieldBool())
	assert.False(t, decoded.HasFieldString())
	assert.False(t, decoded.HasFieldBytes())
}

func TestHasField_ZeroValueScalarPresent(t *testing.T) {
	// Proto3 normally skips default-valued fields during marshal.
	// Construct wire bytes manually to include a zero-valued int32 (field 3).
	// Field 3, wire type 0 (varint), value 0.
	var wire []byte
	wire = protowire.AppendTag(wire, 3, protowire.VarintType)
	wire = protowire.AppendVarint(wire, 0)

	var decoded ks.AllScalars
	require.NoError(t, decoded.Unmarshal(wire))

	assert.Equal(t, int32(0), decoded.FieldInt32)
	assert.True(t, decoded.HasFieldInt32(), "zero-valued field present in wire should be tracked")
	assert.False(t, decoded.HasFieldDouble(), "field absent from wire should not be tracked")
}

func TestHasField_MessagePresent(t *testing.T) {
	msg := &ks.Outer{
		Middle: ks.Middle{Value: 42},
		Name:   "test",
	}
	b, err := msg.Marshal()
	require.NoError(t, err)

	var decoded ks.Outer
	require.NoError(t, decoded.Unmarshal(b))

	assert.True(t, decoded.HasMiddle())
	assert.True(t, decoded.HasName())
}

func TestHasField_EmptyMessagePresent(t *testing.T) {
	// An empty sub-message is still "present" if it appears in the wire.
	// Construct: field 1 (Middle), wire type 2 (bytes), length 0.
	var wire []byte
	wire = protowire.AppendTag(wire, 1, protowire.BytesType)
	wire = protowire.AppendBytes(wire, nil)

	var decoded ks.Outer
	require.NoError(t, decoded.Unmarshal(wire))

	assert.True(t, decoded.HasMiddle(), "empty sub-message in wire should be tracked as present")
	assert.False(t, decoded.HasName(), "absent field should not be tracked")
}

func TestHasField_MessageAbsent(t *testing.T) {
	// Only Name is set, Middle is absent.
	msg := &ks.Outer{Name: "test"}
	b, err := msg.Marshal()
	require.NoError(t, err)

	var decoded ks.Outer
	require.NoError(t, decoded.Unmarshal(b))

	assert.False(t, decoded.HasMiddle(), "absent message field should not be tracked")
	assert.True(t, decoded.HasName())
}

func TestHasField_PartialScalars(t *testing.T) {
	// Only some fields set.
	msg := &ks.AllScalars{
		FieldString: "hello",
		FieldBool:   true,
	}
	b, err := msg.Marshal()
	require.NoError(t, err)

	var decoded ks.AllScalars
	require.NoError(t, decoded.Unmarshal(b))

	assert.True(t, decoded.HasFieldString())
	assert.True(t, decoded.HasFieldBool())
	assert.False(t, decoded.HasFieldInt32())
	assert.False(t, decoded.HasFieldDouble())
}

func TestHasField_EnumPresent(t *testing.T) {
	msg := &ks.WithEnum{Color: ks.Color_COLOR_BLUE}
	b, err := msg.Marshal()
	require.NoError(t, err)

	var decoded ks.WithEnum
	require.NoError(t, decoded.Unmarshal(b))

	assert.True(t, decoded.HasColor())
}

func TestHasField_NestedBitmapIndependent(t *testing.T) {
	// Verify that nested message bitmaps are independent.
	msg := &ks.Outer{
		Middle: ks.Middle{
			Inner: ks.Inner{Data: "deep"},
			Value: 42,
		},
		Name: "test",
	}
	b, err := msg.Marshal()
	require.NoError(t, err)

	var decoded ks.Outer
	require.NoError(t, decoded.Unmarshal(b))

	// Outer level
	assert.True(t, decoded.HasMiddle())
	assert.True(t, decoded.HasName())

	// Middle level
	assert.True(t, decoded.Middle.HasInner())
	assert.True(t, decoded.Middle.HasValue())

	// Inner level
	assert.True(t, decoded.Middle.Inner.HasData())
	assert.False(t, decoded.Middle.Inner.HasRaw(), "Raw was not set")
	assert.False(t, decoded.Middle.Inner.HasSignedVal(), "SignedVal was not set")
	assert.False(t, decoded.Middle.Inner.HasFixedVal(), "FixedVal was not set")
}

func TestHasField_ZeroInitializedStructHasNothingPresent(t *testing.T) {
	// A zero-initialized struct (not from Unmarshal) should have no fields present.
	var msg ks.AllScalars
	assert.False(t, msg.HasFieldDouble())
	assert.False(t, msg.HasFieldString())
	assert.False(t, msg.HasFieldBool())
}

// TestHasField_OptionalFields pins the Has*() contract for proto3 optional
// fields. Presence is carried by the Go field's nil-ability — scalars
// surface as `*T`, `optional bytes` stays a nil-able `[]byte` slice — and
// the generated Has accessor checks `m != nil && m.F != nil` so callers
// using Has* keep the same API surface across value-type and optional
// kinds. Was missing before wiresmith-hld — users had to fall back to a
// manual `!= nil` check.
func TestHasField_OptionalFields(t *testing.T) {
	// Default-constructed: all optional fields nil → Has returns false.
	empty := &num.MixedModifiers{}
	assert.False(t, empty.HasOptionalInt())
	assert.False(t, empty.HasOptionalDouble())
	assert.False(t, empty.HasOptionalString())
	assert.False(t, empty.HasOptionalBytes())

	// Populated: optional fields set → Has returns true.
	intVal := int64(42)
	dblVal := 3.14
	strVal := "hello"
	populated := &num.MixedModifiers{
		OptionalInt:    &intVal,
		OptionalDouble: &dblVal,
		OptionalString: &strVal,
		OptionalBytes:  []byte{0x01, 0x02},
	}
	assert.True(t, populated.HasOptionalInt())
	assert.True(t, populated.HasOptionalDouble())
	assert.True(t, populated.HasOptionalString())
	assert.True(t, populated.HasOptionalBytes())
}

// TestHasField_OptionalFields_DefaultValueIsPresent pins the proto3
// optional contract: an explicitly-set field with the default value
// (`0`, `""`, empty `[]byte`) is still "present". Has returns true
// because the pointer / slice is non-nil — the underlying value being
// the zero-value does not collapse presence to false. Was a gap in the
// initial wiresmith-hld coverage: the round-trip-set test only exercised
// non-default values.
func TestHasField_OptionalFields_DefaultValueIsPresent(t *testing.T) {
	zeroInt := int64(0)
	zeroDbl := float64(0)
	zeroStr := ""
	m := &num.MixedModifiers{
		OptionalInt:    &zeroInt,
		OptionalDouble: &zeroDbl,
		OptionalString: &zeroStr,
		OptionalBytes:  []byte{}, // non-nil, empty — distinct from nil
	}
	assert.True(t, m.HasOptionalInt(), "explicit zero must compare present")
	assert.True(t, m.HasOptionalDouble(), "explicit zero must compare present")
	assert.True(t, m.HasOptionalString(), "explicit empty string must compare present")
	assert.True(t, m.HasOptionalBytes(), "non-nil empty []byte must compare present")
}

// TestHasField_OptionalFields_NilReceiver pins nil-safety. The generated
// Has*() short-circuits on a nil receiver — matches the CLAUDE.md
// "Nil-safety on all generated receiver methods" contract.
func TestHasField_OptionalFields_NilReceiver(t *testing.T) {
	var m *num.MixedModifiers
	assert.False(t, m.HasOptionalInt())
	assert.False(t, m.HasOptionalDouble())
	assert.False(t, m.HasOptionalString())
	assert.False(t, m.HasOptionalBytes())
}
