package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"

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
