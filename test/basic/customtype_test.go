package basic

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ct "github.com/grafana/wiresmith/gen/basic/customtype/v1"
	"github.com/grafana/wiresmith/test/customtypes"
)

// TestCustomType_FieldTypesSwapped pins the struct field types — the
// customtype-annotated fields must surface the user's Go type, and the
// unannotated control fields must keep the proto-default shape.
func TestCustomType_FieldTypesSwapped(t *testing.T) {
	holderType := reflect.TypeFor[ct.CustomTypeHolder]()
	cases := []struct {
		field    string
		wantType string
	}{
		{"Labels", "customtypes.LabelPairs"},
		{"TenantId", "customtypes.TenantID"},
		{"PlainBytes", "[]uint8"},
		{"PlainString", "string"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			f, ok := holderType.FieldByName(tc.field)
			require.True(t, ok)
			assert.Equal(t, tc.wantType, f.Type.String())
		})
	}
}

// TestCustomType_RoundTrip confirms wire format is unchanged — the customtype
// methods write/read the same bytes a plain bytes/string field would.
func TestCustomType_RoundTrip(t *testing.T) {
	msg := &ct.CustomTypeHolder{
		Labels:      customtypes.LabelPairs{Pairs: []byte("env=prod\x00region=us-east")},
		TenantId:    customtypes.TenantID("tenant-abc-123"),
		PlainBytes:  []byte{0x01, 0x02, 0x03},
		PlainString: "plain",
	}
	roundTrip(t, msg)
}

// TestCustomType_Empty exercises the "size > 0" gate: when the customtype
// reports SizeWiresmith() == 0, no tag is emitted, and unmarshal does not
// invoke UnmarshalWiresmith.
func TestCustomType_Empty(t *testing.T) {
	msg := &ct.CustomTypeHolder{}
	b, err := msg.Marshal()
	require.NoError(t, err)
	assert.Empty(t, b, "empty customtypes must not emit any wire bytes")

	dst := &ct.CustomTypeHolder{}
	require.NoError(t, dst.Unmarshal(b))
	assert.Equal(t, customtypes.LabelPairs{}, dst.Labels)
	assert.Equal(t, customtypes.TenantID(""), dst.TenantId)
}

// TestCustomType_EqualDelegatesToUserType pins that the generated Equal
// routes through EqualWiresmith on the user type. Two holders with the
// "same" payload bytes but different LabelPairs identities must still
// compare equal because EqualWiresmith uses bytes.Equal.
func TestCustomType_EqualDelegatesToUserType(t *testing.T) {
	a := &ct.CustomTypeHolder{
		Labels:   customtypes.LabelPairs{Pairs: []byte("k=v")},
		TenantId: customtypes.TenantID("t1"),
	}
	b := &ct.CustomTypeHolder{
		Labels:   customtypes.LabelPairs{Pairs: []byte("k=v")},
		TenantId: customtypes.TenantID("t1"),
	}
	assert.True(t, a.Equal(b))

	c := &ct.CustomTypeHolder{
		Labels:   customtypes.LabelPairs{Pairs: []byte("k=w")},
		TenantId: customtypes.TenantID("t1"),
	}
	assert.False(t, a.Equal(c))
}

// TestCustomType_WireFormatMatchesPlainBytes pins on-wire identity with a
// non-customtype field. The customtype owns its payload, but the proto
// envelope is fixed length-delimited — Marshal must produce the same tag +
// varint-length + payload bytes a plain bytes/string field would. This is
// the acceptance criterion that the option doesn't break peers using the
// official protobuf library.
func TestCustomType_WireFormatMatchesPlainBytes(t *testing.T) {
	msg := &ct.CustomTypeHolder{
		Labels:   customtypes.LabelPairs{Pairs: []byte("hello")},
		TenantId: customtypes.TenantID("abc"),
	}
	b, err := msg.Marshal()
	require.NoError(t, err)
	// Field 1 bytes (tag 0x0a, length 5, "hello") followed by field 2 string
	// (tag 0x12, length 3, "abc"). Hand-rolled so a future regression in
	// either size, length-prefix encoding, or tag emission is caught.
	want := []byte{
		0x0a, 0x05, 'h', 'e', 'l', 'l', 'o',
		0x12, 0x03, 'a', 'b', 'c',
	}
	assert.Equal(t, want, b)
}

// TestCustomType_GetterOnNilReceiver pins the nil-safety contract — the
// generated Get*() must not panic on a nil receiver. The customtype-aware
// branch uses `var zero T` so the contract holds for any user type shape.
func TestCustomType_GetterOnNilReceiver(t *testing.T) {
	var m *ct.CustomTypeHolder
	assert.Equal(t, customtypes.LabelPairs{}, m.GetLabels())
	assert.Equal(t, customtypes.TenantID(""), m.GetTenantId())
	assert.Nil(t, m.GetPlainBytes())
	assert.Empty(t, m.GetPlainString())
}
