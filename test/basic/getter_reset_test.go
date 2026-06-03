package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ks "github.com/grafana/wiresmith/gen/test/kitchensink/v1"
)

// Compile-time checks that ProtoMessage and Reset exist with correct signatures.
var _ interface{ ProtoMessage() } = (*ks.AllScalars)(nil)
var _ interface{ Reset() } = (*ks.AllScalars)(nil)

func TestReset_ClearsFieldsAndPresence(t *testing.T) {
	msg := &ks.AllScalars{
		FieldString: "hello",
		FieldInt32:  42,
		FieldBool:   true,
	}
	b, err := msg.Marshal()
	require.NoError(t, err)

	var decoded ks.AllScalars
	require.NoError(t, decoded.Unmarshal(b))

	assert.True(t, decoded.HasFieldString())
	assert.True(t, decoded.HasFieldInt32())

	decoded.Reset()

	assert.Equal(t, "", decoded.FieldString)
	assert.Equal(t, int32(0), decoded.FieldInt32)
	assert.False(t, decoded.FieldBool)
	assert.False(t, decoded.HasFieldString(), "presence must be cleared by Reset")
	assert.False(t, decoded.HasFieldInt32(), "presence must be cleared by Reset")
	assert.False(t, decoded.HasFieldBool(), "presence must be cleared by Reset")
}

func TestReset_NilReceiver(t *testing.T) {
	var m *ks.AllScalars
	assert.NotPanics(t, func() { m.Reset() })

	var o *ks.Outer
	assert.NotPanics(t, func() { o.Reset() })
}

// Size and the Marshal family must be nil-safe to match the contract pinned by
// Get*/Has*/Equal/Reset/String. CLAUDE.md "nil-safety on all generated receiver
// methods" treats the surface as uniform; a nil panic from Size() while Get()
// returns zero would surprise callers.
func TestSize_NilReceiver(t *testing.T) {
	var m *ks.AllScalars
	assert.Equal(t, 0, m.Size())

	var o *ks.Outer
	assert.Equal(t, 0, o.Size())
}

func TestMarshal_NilReceiver(t *testing.T) {
	var m *ks.AllScalars
	b, err := m.Marshal()
	assert.NoError(t, err)
	assert.Nil(t, b)

	var o *ks.Outer
	b, err = o.Marshal()
	assert.NoError(t, err)
	assert.Nil(t, b)
}

func TestMarshalTo_NilReceiver(t *testing.T) {
	var m *ks.AllScalars
	buf := make([]byte, 16)
	n, err := m.MarshalTo(buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestMarshalToSizedBuffer_NilReceiver(t *testing.T) {
	var m *ks.AllScalars
	buf := make([]byte, 16)
	n, err := m.MarshalToSizedBuffer(buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestReset_NestedMessage(t *testing.T) {
	msg := &ks.Outer{
		Middle: ks.Middle{Value: 99, Inner: ks.Inner{Data: "deep"}},
		Name:   "test",
	}
	b, err := msg.Marshal()
	require.NoError(t, err)

	var decoded ks.Outer
	require.NoError(t, decoded.Unmarshal(b))
	decoded.Reset()

	assert.Equal(t, ks.Outer{}, decoded)
	assert.False(t, decoded.HasMiddle())
	assert.False(t, decoded.HasName())
}

// --- Getter tests ---

func TestGetters_ScalarNilReceiver(t *testing.T) {
	var m *ks.AllScalars
	assert.Equal(t, float64(0), m.GetFieldDouble())
	assert.Equal(t, float32(0), m.GetFieldFloat())
	assert.Equal(t, int32(0), m.GetFieldInt32())
	assert.Equal(t, int64(0), m.GetFieldInt64())
	assert.Equal(t, uint32(0), m.GetFieldUint32())
	assert.Equal(t, uint64(0), m.GetFieldUint64())
	assert.Equal(t, int32(0), m.GetFieldSint32())
	assert.Equal(t, int64(0), m.GetFieldSint64())
	assert.Equal(t, uint32(0), m.GetFieldFixed32())
	assert.Equal(t, uint64(0), m.GetFieldFixed64())
	assert.Equal(t, int32(0), m.GetFieldSfixed32())
	assert.Equal(t, int64(0), m.GetFieldSfixed64())
	assert.Equal(t, false, m.GetFieldBool())
	assert.Equal(t, "", m.GetFieldString())
	assert.Nil(t, m.GetFieldBytes())
}

func TestGetters_ScalarValues(t *testing.T) {
	msg := &ks.AllScalars{
		FieldDouble: 3.14,
		FieldString: "hello",
		FieldBool:   true,
		FieldInt32:  -42,
		FieldBytes:  []byte{0xCA, 0xFE},
	}
	assert.Equal(t, 3.14, msg.GetFieldDouble())
	assert.Equal(t, "hello", msg.GetFieldString())
	assert.Equal(t, true, msg.GetFieldBool())
	assert.Equal(t, int32(-42), msg.GetFieldInt32())
	assert.Equal(t, []byte{0xCA, 0xFE}, msg.GetFieldBytes())
}

func TestGetters_MessageFieldBitmapAware(t *testing.T) {
	// Go-constructed struct: bitmap not set, getter returns nil.
	msg := &ks.Outer{
		Middle: ks.Middle{Value: 42},
		Name:   "test",
	}
	assert.Nil(t, msg.GetMiddle(), "getter must return nil when bitmap is not set")
	assert.Equal(t, "test", msg.GetName())

	// After unmarshal: bitmap is set, getter returns pointer.
	b, err := msg.Marshal()
	require.NoError(t, err)

	var decoded ks.Outer
	require.NoError(t, decoded.Unmarshal(b))
	got := decoded.GetMiddle()
	require.NotNil(t, got, "getter must return non-nil after Unmarshal")
	assert.Equal(t, int64(42), got.Value)
}

func TestGetters_MessageFieldNilReceiver(t *testing.T) {
	var m *ks.Outer
	assert.Nil(t, m.GetMiddle())
	assert.Equal(t, "", m.GetName())
	assert.Nil(t, m.GetMiddles())
}

func TestGetters_OptionalFields(t *testing.T) {
	// All nil.
	msg := &ks.AllOptionalScalars{}
	assert.Equal(t, float64(0), msg.GetFieldDouble())
	assert.Equal(t, float32(0), msg.GetFieldFloat())
	assert.Equal(t, int32(0), msg.GetFieldInt32())
	assert.Equal(t, int64(0), msg.GetFieldInt64())
	assert.Equal(t, uint32(0), msg.GetFieldUint32())
	assert.Equal(t, uint64(0), msg.GetFieldUint64())
	assert.Equal(t, int32(0), msg.GetFieldSint32())
	assert.Equal(t, int64(0), msg.GetFieldSint64())
	assert.Equal(t, uint32(0), msg.GetFieldFixed32())
	assert.Equal(t, uint64(0), msg.GetFieldFixed64())
	assert.Equal(t, int32(0), msg.GetFieldSfixed32())
	assert.Equal(t, int64(0), msg.GetFieldSfixed64())
	assert.Equal(t, false, msg.GetFieldBool())
	assert.Equal(t, "", msg.GetFieldString())
	assert.Nil(t, msg.GetFieldBytes())

	// Set to non-zero.
	d := 1.23
	s := "opt"
	msg2 := &ks.AllOptionalScalars{FieldDouble: &d, FieldString: &s}
	assert.Equal(t, 1.23, msg2.GetFieldDouble())
	assert.Equal(t, "opt", msg2.GetFieldString())

	// Nil receiver.
	var nilMsg *ks.AllOptionalScalars
	assert.Equal(t, float64(0), nilMsg.GetFieldDouble())
	assert.Equal(t, "", nilMsg.GetFieldString())
}

func TestGetters_RepeatedFields(t *testing.T) {
	msg := &ks.AllRepeatedScalars{
		FieldString: []string{"a", "b"},
		FieldInt32:  []int32{1, 2, 3},
	}
	assert.Equal(t, []string{"a", "b"}, msg.GetFieldString())
	assert.Equal(t, []int32{1, 2, 3}, msg.GetFieldInt32())

	empty := &ks.AllRepeatedScalars{}
	assert.Nil(t, empty.GetFieldString())
	assert.Nil(t, empty.GetFieldInt32())
}

func TestGetters_MapFields(t *testing.T) {
	msg := &ks.AllMaps{
		MapStringString: map[string]string{"k": "v"},
	}
	assert.Equal(t, map[string]string{"k": "v"}, msg.GetMapStringString())
	assert.Nil(t, msg.GetMapInt32Int32())

	var nilMsg *ks.AllMaps
	assert.Nil(t, nilMsg.GetMapStringString())
}

func TestGetters_OneofFields(t *testing.T) {
	msg := &ks.OneofVariants{
		Value: &ks.OneofVariants_StringValue{StringValue: "hello"},
	}
	assert.Equal(t, "hello", msg.GetStringValue())
	assert.Equal(t, float64(0), msg.GetDoubleValue())
	assert.Nil(t, msg.GetBytesValue())

	msg2 := &ks.OneofVariants{
		Value: &ks.OneofVariants_DoubleValue{DoubleValue: 3.14},
	}
	assert.Equal(t, 3.14, msg2.GetDoubleValue())
	assert.Equal(t, "", msg2.GetStringValue())

	// Nil receiver.
	var nilMsg *ks.OneofVariants
	assert.Nil(t, nilMsg.GetValue())
	assert.Equal(t, float64(0), nilMsg.GetDoubleValue())

	// Nil variant.
	msg3 := &ks.OneofVariants{}
	assert.Nil(t, msg3.GetValue())
	assert.Equal(t, int32(0), msg3.GetInt32Value())
}

func TestGetters_EnumField(t *testing.T) {
	msg := &ks.WithEnum{Color: ks.Color_COLOR_BLUE}
	assert.Equal(t, ks.Color_COLOR_BLUE, msg.GetColor())

	var nilMsg *ks.WithEnum
	assert.Equal(t, ks.Color(0), nilMsg.GetColor())
}
