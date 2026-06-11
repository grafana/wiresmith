package basic

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	np "github.com/grafana/wiresmith/gen/basic/nopresence/v1"
)

// Struct layout is the point of no_presence: the annotated struct must
// contain exactly the declared fields, nothing else. A hand-written mirror
// of the declared fields pins the sizeof; if a bitmap (or anything else)
// sneaks back into the generated struct this fails at compile-review time
// with a clear size delta.
func TestNoPresence_StructLayout(t *testing.T) {
	type mirror struct {
		Child np.Leaf
		Num   int64
		Label string
		Maybe *int64
	}
	assert.Equal(t, unsafe.Sizeof(mirror{}), unsafe.Sizeof(np.BareHolder{}),
		"BareHolder must have gogoproto-parity layout (declared fields only, no bitmap)")
}

func TestNoPresence_RoundTrip(t *testing.T) {
	maybe := int64(7)
	msg := &np.BareHolder{
		Child: np.Leaf{Id: 1, Name: "leaf"},
		Num:   42,
		Label: "x",
		Maybe: &maybe,
	}
	b, err := msg.Marshal()
	require.NoError(t, err)

	dst := &np.BareHolder{}
	require.NoError(t, dst.Unmarshal(b))
	assert.True(t, msg.Equal(dst))
	// With no bitmap, a hand-constructed message and its decoded round-trip
	// are directly comparable — the property Loki's and Mimir's tests rely on.
	assert.Equal(t, msg, dst)
}

// Without the bitmap there is no "present but empty" state: an empty child
// marshals to nothing and decodes as the zero value — gogoproto
// `nullable=false` semantics. The unannotated control message keeps the
// wiresmith default, where an explicitly-decoded empty child re-marshals as
// a zero-length field.
func TestNoPresence_EmptyChildDropsFromWire(t *testing.T) {
	b, err := (&np.BareHolder{Child: np.Leaf{}}).Marshal()
	require.NoError(t, err)
	assert.Empty(t, b, "empty child + zero scalars must marshal to zero bytes under no_presence")

	// Control: TrackedHolder round-trips present-but-empty via its bitmap.
	wire := []byte{0x0a, 0x00} // field 1, length-delimited, len 0
	tracked := &np.TrackedHolder{}
	require.NoError(t, tracked.Unmarshal(wire))
	require.True(t, tracked.HasChild())
	b2, err := tracked.Marshal()
	require.NoError(t, err)
	assert.Equal(t, wire, b2, "TrackedHolder must preserve present-but-empty via the bitmap")

	// BareHolder decoding the same wire accepts it but cannot preserve it.
	bare := &np.BareHolder{}
	require.NoError(t, bare.Unmarshal(wire))
	b3, err := bare.Marshal()
	require.NoError(t, err)
	assert.Empty(t, b3)
}

// Get<MsgField> on a no_presence message returns the VALUE — gogoproto
// nullable=false getter parity, so gogo-era value-getter-shaped interfaces
// (Loki's queryrange Request/Response) are satisfied directly. Optional
// fields keep their pointer-based Has accessor.
func TestNoPresence_Accessors(t *testing.T) {
	bare := &np.BareHolder{Child: np.Leaf{Id: 7}}
	// Explicit np.Leaf type is a compile-time assertion that GetChild returns
	// the value, not *np.Leaf — the no_presence value-getter contract.
	var got np.Leaf = bare.GetChild() //nolint:staticcheck // QF1011: type is intentional, see above
	assert.Equal(t, int64(7), got.Id, "GetChild must return the value under no_presence")
	assert.False(t, bare.HasMaybe())
	maybe := int64(0)
	bare.Maybe = &maybe
	assert.True(t, bare.HasMaybe(), "optional presence is the pointer's nil-ness, unaffected by no_presence")

	var nilHolder *np.BareHolder
	assert.Equal(t, np.Leaf{}, nilHolder.GetChild(), "nil receiver returns the zero value")
	assert.Zero(t, nilHolder.GetNum())
}
