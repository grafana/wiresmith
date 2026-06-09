package basic

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ct "github.com/grafana/wiresmith/gen/basic/customtype/v1"
	"github.com/grafana/wiresmith/test/customtypes"
)

// TestRepeatedCustomType_FieldTypesSwapped pins the Go-side shape of the
// repeated customtype-annotated fields. Each element surfaces as the user
// type; controls stay as plain `[]byte` / `[]string`.
func TestRepeatedCustomType_FieldTypesSwapped(t *testing.T) {
	holderType := reflect.TypeFor[ct.RepeatedCustomTypeHolder]()
	cases := []struct {
		field    string
		wantType string
	}{
		{"Ids", "[]customtypes.UUID"},
		{"Tags", "[]customtypes.Tag"},
		{"PlainIds", "[][]uint8"},
		{"PlainTags", "[]string"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			f, ok := holderType.FieldByName(tc.field)
			require.True(t, ok)
			assert.Equal(t, tc.wantType, f.Type.String())
		})
	}
}

// TestRepeatedCustomType_RoundTrip confirms wire format is unchanged for
// the repeated bytes (UUID) and repeated string (Tag) customtype dispatch.
// Each per-element envelope stays length-delimited; the user type owns the
// payload bytes.
func TestRepeatedCustomType_RoundTrip(t *testing.T) {
	msg := &ct.RepeatedCustomTypeHolder{
		Ids: []customtypes.UUID{
			{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
			{0xff, 0xee, 0xdd, 0xcc, 0xbb, 0xaa, 0x99, 0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11, 0x00},
		},
		Tags:      []customtypes.Tag{"alpha", "beta", "gamma"},
		PlainIds:  [][]byte{{0xde, 0xad}, {0xbe, 0xef}},
		PlainTags: []string{"plain1", "plain2"},
	}
	roundTrip(t, msg)
}

// TestRepeatedCustomType_WireCompatWithControl confirms the wire bytes a
// repeated customtype produces are byte-identical to the bytes a plain
// repeated bytes/string field would produce for the same payload content.
// Each UUID element is 16 bytes; each Tag is a length-prefixed string.
func TestRepeatedCustomType_WireCompatWithControl(t *testing.T) {
	id := customtypes.UUID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}

	// Customtype path: Ids[0] via UUID; PlainIds unset.
	a := &ct.RepeatedCustomTypeHolder{Ids: []customtypes.UUID{id}}
	bA, err := a.Marshal()
	require.NoError(t, err)

	// Control path: PlainIds[0] with the same 16 payload bytes; Ids unset.
	b := &ct.RepeatedCustomTypeHolder{PlainIds: [][]byte{id[:]}}
	bB, err := b.Marshal()
	require.NoError(t, err)

	require.Len(t, bA, len(bB), "customtype and control wire byte counts must match for equivalent payload")
	// The two encodings differ only in the wire-tag byte (field number 1
	// vs 3). Strip that leading byte and confirm length-prefix + payload
	// bytes are identical between the customtype and control paths.
	assert.Equal(t, bA[1:], bB[1:], "payload bytes after the wire tag must be identical between customtype and control")
}

// TestRepeatedCustomType_EmptyRoundTrip confirms an empty customtype slice
// produces no wire bytes. Decoding such a payload leaves the slice nil.
func TestRepeatedCustomType_EmptyRoundTrip(t *testing.T) {
	msg := &ct.RepeatedCustomTypeHolder{}
	b, err := msg.Marshal()
	require.NoError(t, err)
	assert.Empty(t, b, "empty holder must produce no wire bytes")

	dst := &ct.RepeatedCustomTypeHolder{}
	require.NoError(t, dst.Unmarshal(b))
	assert.Nil(t, dst.Ids)
	assert.Nil(t, dst.Tags)
}

// TestRepeatedCustomType_PreservesEmptyElements pins that every slice
// element is emitted to the wire, including an element whose
// SizeWiresmith() returns 0 — proto3 repeated semantics preserve each
// occurrence (analogous to `repeated bytes` emitting `tag + 0` for an
// empty `[]byte` element). Skipping zero-sized elements would silently
// corrupt round-trips.
//
// Uses Tag (a string-backed customtype) rather than UUID for this
// regression: UUID is fixed-size at 16 bytes so SizeWiresmith() never
// returns 0, and a "skip on size 0" bug in the generator would slip
// through a UUID-only fixture. Tag("") has SizeWiresmith() == 0 and
// makes the gate testable.
func TestRepeatedCustomType_PreservesEmptyElements(t *testing.T) {
	msg := &ct.RepeatedCustomTypeHolder{
		Tags: []customtypes.Tag{
			"alpha", // non-empty
			"",      // size-0 — must still round-trip
			"omega", // non-empty
		},
	}
	b, err := msg.Marshal()
	require.NoError(t, err)

	dst := &ct.RepeatedCustomTypeHolder{}
	require.NoError(t, dst.Unmarshal(b))
	require.Len(t, dst.Tags, 3, "every slice element must round-trip, including the size-0 element")
	assert.Equal(t, customtypes.Tag("alpha"), dst.Tags[0])
	assert.Equal(t, customtypes.Tag(""), dst.Tags[1])
	assert.Equal(t, customtypes.Tag("omega"), dst.Tags[2])
}

// TestRepeatedCustomType_GetterOnNilReceiver pins the nil-safety contract
// for the repeated customtype getter.
func TestRepeatedCustomType_GetterOnNilReceiver(t *testing.T) {
	var m *ct.RepeatedCustomTypeHolder
	assert.Nil(t, m.GetIds())
	assert.Nil(t, m.GetTags())
}
