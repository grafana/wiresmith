package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ptr "wiresmith/gen/basic/pointer/v1"
)

func TestPointerHolder_RoundTrip(t *testing.T) {
	msg := &ptr.PointerHolder{
		Name:  "holder",
		Head:  &ptr.Leaf{Id: 1, Name: "head"},
		Items: []*ptr.Leaf{{Id: 10, Name: "a"}, {Id: 20, Name: "b"}},
		Tail:  ptr.Leaf{Id: 99, Name: "tail"},
	}
	roundTrip(t, msg)
}

// Singular pointer-message: nil means absent — no tag emitted, decode yields nil.
func TestPointerHolder_NilHead(t *testing.T) {
	msg := &ptr.PointerHolder{Name: "holder"}
	b, err := msg.Marshal()
	require.NoError(t, err)

	dst := &ptr.PointerHolder{}
	require.NoError(t, dst.Unmarshal(b))
	assert.Nil(t, dst.Head)
	assert.Nil(t, dst.GetHead())
	assert.Empty(t, dst.Items)
}

// Singular pointer-message: an empty message *is* present and round-trips as
// a non-nil zero-value Leaf. This is the same boundary case as proto3
// `optional` with a zero-length nested message.
func TestPointerHolder_EmptyHead(t *testing.T) {
	msg := &ptr.PointerHolder{Head: &ptr.Leaf{}}
	b, err := msg.Marshal()
	require.NoError(t, err)
	require.NotEmpty(t, b, "empty submessage with present pointer must emit a tag+length-0")

	dst := &ptr.PointerHolder{}
	require.NoError(t, dst.Unmarshal(b))
	require.NotNil(t, dst.Head)
	assert.Equal(t, int64(0), dst.Head.Id)
}

// Repeated pointer: nil elements are skipped, matching gogoproto nullable=true.
func TestPointerHolder_NilElementSkipped(t *testing.T) {
	msg := &ptr.PointerHolder{
		Items: []*ptr.Leaf{
			{Id: 1, Name: "a"},
			nil,
			{Id: 2, Name: "b"},
			nil,
		},
	}
	b, err := msg.Marshal()
	require.NoError(t, err)

	dst := &ptr.PointerHolder{}
	require.NoError(t, dst.Unmarshal(b))
	require.Len(t, dst.Items, 2, "nil entries must not appear on the wire")
	assert.Equal(t, int64(1), dst.Items[0].Id)
	assert.Equal(t, int64(2), dst.Items[1].Id)
}

// Size and len(Marshal()) must agree even when nil elements are present, so
// the size precomputation can't overcount nil slots either.
func TestPointerHolder_SizeMatchesMarshalWithNils(t *testing.T) {
	msg := &ptr.PointerHolder{
		Items: []*ptr.Leaf{nil, {Id: 7}, nil},
	}
	b, err := msg.Marshal()
	require.NoError(t, err)
	assert.Equal(t, msg.Size(), len(b))
}

// Equal: nil vs non-nil head must be distinguishable; otherwise Equal would
// mask a difference that the wire format records.
func TestPointerHolder_EqualNilVsPresent(t *testing.T) {
	a := &ptr.PointerHolder{Head: &ptr.Leaf{Id: 1}}
	b := &ptr.PointerHolder{Head: nil}
	assert.False(t, a.Equal(b))
	assert.False(t, b.Equal(a))

	c := &ptr.PointerHolder{Head: &ptr.Leaf{Id: 1}}
	assert.True(t, a.Equal(c))
}

// Equal on []*Leaf: nil-in-one, value-in-other at the same index must be
// unequal. Per-element Equal would NPE without the explicit nil-guard.
func TestPointerHolder_EqualRepeatedNilMismatch(t *testing.T) {
	a := &ptr.PointerHolder{Items: []*ptr.Leaf{{Id: 1}, nil}}
	b := &ptr.PointerHolder{Items: []*ptr.Leaf{{Id: 1}, {Id: 2}}}
	assert.False(t, a.Equal(b))
	assert.False(t, b.Equal(a))

	c := &ptr.PointerHolder{Items: []*ptr.Leaf{{Id: 1}, nil}}
	assert.True(t, a.Equal(c))
}

// Get on a nil receiver must not panic. CLAUDE.md "Common review caveats"
// flags this; a focused test pins the contract.
func TestPointerHolder_GetOnNilReceiver(t *testing.T) {
	var m *ptr.PointerHolder
	assert.Nil(t, m.GetHead())
	assert.Nil(t, m.GetItems())
	assert.Nil(t, m.GetTail())
}

// The value-type "Tail" field side by side with the pointer-shape fields
// confirms the option is local to the annotated field — the surrounding
// presence-bitmap path is untouched.
func TestPointerHolder_ValueTypeTailStillUsesBitmap(t *testing.T) {
	// Explicit set: zero-value Tail with HasTail() == true after round-trip.
	src := &ptr.PointerHolder{Tail: ptr.Leaf{}}
	// HasTail is false because the bitmap is only set during Unmarshal,
	// matching existing value-type-message presence semantics.
	assert.False(t, src.HasTail())

	// Now marshal & unmarshal a holder with a non-empty Tail so the bitmap
	// flips on the decode side.
	src2 := &ptr.PointerHolder{Tail: ptr.Leaf{Id: 5}}
	b, err := src2.Marshal()
	require.NoError(t, err)

	dst := &ptr.PointerHolder{}
	require.NoError(t, dst.Unmarshal(b))
	assert.True(t, dst.HasTail(), "non-empty value-type Tail should be marked present after Unmarshal")
}
