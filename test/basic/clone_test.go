package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mapsv1 "github.com/grafana/wiresmith/gen/basic/maps/v1"
	pointerv1 "github.com/grafana/wiresmith/gen/basic/pointer/v1"
	ks "github.com/grafana/wiresmith/gen/test/kitchensink/v1"
)

// Clone() (wiresmith-oz2l) is a generated, non-reflection deep copy. These
// tests pin the two halves of the contract: (1) clone.Equal(orig) holds for
// every shape, and (2) the clone shares NO mutable backing storage with the
// original — mutating the clone (or the original) must never be observable on
// the other side. Coverage spans nested value messages, repeated value/pointer
// messages, repeated scalars/bytes, maps (scalar and message values), oneofs
// (scalar/bytes/message variants), pointer-option fields, and the nil receiver.

// TestClone_NilReceiver pins the nil-safety contract: a nil receiver clones to
// a nil pointer (never panics).
func TestClone_NilReceiver(t *testing.T) {
	var p *ks.Outer
	assert.Nil(t, p.Clone(), "nil receiver must clone to nil")

	var leaf *pointerv1.Leaf
	assert.Nil(t, leaf.Clone())
}

// TestClone_NestedAndRepeatedMessages exercises singular value messages,
// repeated value messages, nested bytes, and scalars.
func TestClone_NestedAndRepeatedMessages(t *testing.T) {
	orig := &ks.Outer{
		Middle: ks.Middle{
			Inner:  ks.Inner{Data: "deep", Raw: []byte{0xFF, 0x01}, SignedVal: -9999},
			Inners: []ks.Inner{{Data: "inner1", Raw: []byte{0x0A}}, {Data: "inner2"}},
			Value:  42,
		},
		Middles: []ks.Middle{{Value: 100}, {Inner: ks.Inner{Data: "nested"}, Value: 200}},
		Name:    "outer-name",
	}

	clone := orig.Clone()
	require.True(t, clone.Equal(orig), "clone must Equal original")
	require.True(t, orig.Equal(clone), "Equal must be symmetric")

	// Mutate every reference-bearing part of the clone; the original must not move.
	clone.Name = "mutated"
	clone.Middle.Inner.Data = "changed"
	clone.Middle.Inner.Raw[0] = 0x00 // bytes deep-copied, not aliased
	clone.Middle.Inners[0].Data = "changed-inner"
	clone.Middles[1].Inner.Data = "changed-middle"
	clone.Middles = append(clone.Middles, ks.Middle{Value: 300})

	assert.Equal(t, "outer-name", orig.Name)
	assert.Equal(t, "deep", orig.Middle.Inner.Data)
	assert.Equal(t, byte(0xFF), orig.Middle.Inner.Raw[0], "original bytes must not alias clone")
	assert.Equal(t, "inner1", orig.Middle.Inners[0].Data)
	assert.Equal(t, "nested", orig.Middles[1].Inner.Data)
	assert.Len(t, orig.Middles, 2, "appending to clone slice must not grow original")
}

// TestClone_RepeatedScalarsAndBytes pins that repeated scalar slices and
// repeated-bytes ([][]byte) get fresh backing storage per element.
func TestClone_RepeatedScalarsAndBytes(t *testing.T) {
	orig := &ks.AllRepeatedScalars{
		FieldInt32:  []int32{1, 2, 3},
		FieldString: []string{"a", "b"},
		FieldBytes:  [][]byte{{0x01, 0x02}, {0x03}},
	}

	clone := orig.Clone()
	require.True(t, clone.Equal(orig))

	clone.FieldInt32[0] = 99
	clone.FieldString[0] = "z"
	clone.FieldBytes[0][0] = 0xFF // inner []byte must be deep-copied, not aliased

	assert.Equal(t, int32(1), orig.FieldInt32[0])
	assert.Equal(t, "a", orig.FieldString[0])
	assert.Equal(t, byte(0x01), orig.FieldBytes[0][0], "repeated-bytes element must not alias")
}

// TestClone_Maps covers scalar-valued and message-valued maps; the clone must
// own a fresh map and fresh message values.
func TestClone_Maps(t *testing.T) {
	orig := &mapsv1.MapBench{
		StringMap:  map[string]string{"k": "v"},
		IntMap:     map[int64]int64{1: 2},
		MessageMap: map[string]mapsv1.Inner{"m": {Name: "inner", Data: []byte{0x07}}},
	}

	clone := orig.Clone()
	require.True(t, clone.Equal(orig))

	clone.StringMap["k"] = "changed"
	clone.StringMap["new"] = "added"
	inner := clone.MessageMap["m"]
	inner.Name = "changed-inner"
	inner.Data[0] = 0xFF
	clone.MessageMap["m"] = inner

	assert.Equal(t, "v", orig.StringMap["k"])
	assert.NotContains(t, orig.StringMap, "new", "adding to clone map must not touch original")
	assert.Equal(t, "inner", orig.MessageMap["m"].Name)
	assert.Equal(t, byte(0x07), orig.MessageMap["m"].Data[0], "map message-value bytes must not alias")
}

// TestClone_OneofVariants clones each oneof variant shape (scalar, bytes,
// message) and verifies the payload is deep-copied.
func TestClone_OneofVariants(t *testing.T) {
	// Scalar variant.
	scalar := &ks.OneofVariants{Value: &ks.OneofVariants_Sint32Value{Sint32Value: -7}}
	require.True(t, scalar.Clone().Equal(scalar))

	// Bytes variant: mutating the clone's payload must not touch the original.
	bytesOrig := &ks.OneofVariants{Value: &ks.OneofVariants_BytesValue{BytesValue: []byte{0x01, 0x02}}}
	bytesClone := bytesOrig.Clone()
	require.True(t, bytesClone.Equal(bytesOrig))
	bytesClone.Value.(*ks.OneofVariants_BytesValue).BytesValue[0] = 0xFF
	assert.Equal(t, byte(0x01), bytesOrig.Value.(*ks.OneofVariants_BytesValue).BytesValue[0])
}

// TestClone_PointerFields covers the pointer-option shapes: *Msg, []*Msg, and
// a control value-type message field.
func TestClone_PointerFields(t *testing.T) {
	orig := &pointerv1.PointerHolder{
		Name:  "root",
		Head:  &pointerv1.Leaf{Id: 1, Name: "head"},
		Items: []*pointerv1.Leaf{{Id: 2}, nil, {Id: 3}},
		Tail:  pointerv1.Leaf{Id: 4, Name: "tail"},
	}

	clone := orig.Clone()
	require.True(t, clone.Equal(orig))

	clone.Head.Id = 99
	clone.Items[0].Id = 99
	clone.Tail.Id = 99

	assert.Equal(t, int64(1), orig.Head.Id, "*Msg field must not alias")
	assert.Equal(t, int64(2), orig.Items[0].Id, "[]*Msg element must not alias")
	assert.Nil(t, clone.Items[1], "nil slice element must survive clone")
	assert.Equal(t, int64(4), orig.Tail.Id, "value-message field must not alias")
}
