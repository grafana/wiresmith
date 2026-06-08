package basic

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ctm "github.com/grafana/wiresmith/gen/basic/customtype_message/v1"
	"github.com/grafana/wiresmith/test/customtypes"
)

// TestCustomTypeMessage_FieldTypesSwapped pins the Go-side shape of the
// customtype-annotated fields. Singular message customtype must surface as
// the user type (not `*Label`); repeated as `[]LabelAdapter`. Controls
// retain the wiresmith-default shape.
func TestCustomTypeMessage_FieldTypesSwapped(t *testing.T) {
	holderType := reflect.TypeFor[ctm.CustomTypeMessageHolder]()
	cases := []struct {
		field    string
		wantType string
	}{
		{"Primary", "customtypes.LabelAdapter"},
		{"Labels", "[]customtypes.LabelAdapter"},
		{"ControlSingular", "v1.Label"},
		{"ControlRepeated", "[]v1.Label"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			f, ok := holderType.FieldByName(tc.field)
			require.True(t, ok)
			assert.Equal(t, tc.wantType, f.Type.String())
		})
	}
}

// TestCustomTypeMessage_RoundTrip confirms wire format is unchanged across
// the singular and repeated message customtype dispatch. The customtype's
// MarshalWiresmith writes the inner submessage's wire payload, which the
// wrapper bookends with tag + length.
func TestCustomTypeMessage_RoundTrip(t *testing.T) {
	msg := &ctm.CustomTypeMessageHolder{
		Primary: customtypes.LabelAdapter{Name: "service", Value: "api"},
		Labels: []customtypes.LabelAdapter{
			{Name: "env", Value: "prod"},
			{Name: "region", Value: "us-east-1"},
		},
		ControlSingular: ctm.Label{Name: "ctrl", Value: "single"},
		ControlRepeated: []ctm.Label{
			{Name: "ctrl0", Value: "v0"},
			{Name: "ctrl1", Value: "v1"},
		},
	}
	roundTrip(t, msg)
}

// TestCustomTypeMessage_WireCompatWithControl confirms the wire bytes a
// customtype-annotated field produces are byte-identical to the bytes a
// non-customtype field of the equivalent inner shape would produce. The
// payload owned by the user type must round-trip cleanly through wiresmith
// reading the message back as a plain Label.
func TestCustomTypeMessage_WireCompatWithControl(t *testing.T) {
	// Holder A: writes Primary via LabelAdapter (the customtype path).
	a := &ctm.CustomTypeMessageHolder{
		Primary: customtypes.LabelAdapter{Name: "n", Value: "v"},
	}
	bA, err := a.Marshal()
	require.NoError(t, err)

	// Holder B: writes ControlSingular via the wiresmith-default Label.
	// Field number is different (3 vs 1), but we want the *payload* shape
	// to match. Strip the tag and compare the length-delimited payload.
	b := &ctm.CustomTypeMessageHolder{
		ControlSingular: ctm.Label{Name: "n", Value: "v"},
	}
	bB, err := b.Marshal()
	require.NoError(t, err)

	require.Len(t, bA, len(bB), "customtype and control wire byte counts must match for equivalent content")
	// Both should encode as tag + length + (name field + value field). The
	// only allowable difference is the leading tag byte (field number 1 vs
	// 3). Confirm the rest is byte-identical.
	assert.Equal(t, bA[1:], bB[1:], "payload bytes after the tag must be identical between customtype and control")
}

// TestCustomTypeMessage_EmptyRoundTrip exercises the SizeWiresmith=0 gate.
// An empty LabelAdapter reports size 0, the wrapper skips the tag, and an
// empty Labels slice produces zero wire bytes.
func TestCustomTypeMessage_EmptyRoundTrip(t *testing.T) {
	msg := &ctm.CustomTypeMessageHolder{}
	b, err := msg.Marshal()
	require.NoError(t, err)
	assert.Empty(t, b, "all-zero holder must produce no wire bytes")

	dst := &ctm.CustomTypeMessageHolder{}
	require.NoError(t, dst.Unmarshal(b))
	assert.Equal(t, customtypes.LabelAdapter{}, dst.Primary)
	assert.Nil(t, dst.Labels)
}

// TestCustomTypeMessage_RepeatedAppend pins that successive Unmarshal calls
// each append one element to the customtype slice — never overwrite. This
// is the proto3 repeated-field convention and the natural shape of the
// `append(slice, T{})` generator emit.
func TestCustomTypeMessage_RepeatedAppend(t *testing.T) {
	msg := &ctm.CustomTypeMessageHolder{
		Labels: []customtypes.LabelAdapter{
			{Name: "k0", Value: "v0"},
			{Name: "k1", Value: "v1"},
			{Name: "k2", Value: "v2"},
		},
	}
	b, err := msg.Marshal()
	require.NoError(t, err)

	dst := &ctm.CustomTypeMessageHolder{}
	require.NoError(t, dst.Unmarshal(b))
	require.Len(t, dst.Labels, 3)
	assert.Equal(t, "k0", dst.Labels[0].Name)
	assert.Equal(t, "v1", dst.Labels[1].Value)
	assert.Equal(t, "k2", dst.Labels[2].Name)
}

// TestCustomTypeMessage_EqualDelegatesToUserType pins that Equal routes
// through EqualWiresmith on the customtype, both for the singular case and
// for the repeated case (element-wise).
func TestCustomTypeMessage_EqualDelegatesToUserType(t *testing.T) {
	a := &ctm.CustomTypeMessageHolder{
		Primary: customtypes.LabelAdapter{Name: "n", Value: "v"},
		Labels:  []customtypes.LabelAdapter{{Name: "a", Value: "1"}, {Name: "b", Value: "2"}},
	}
	b := &ctm.CustomTypeMessageHolder{
		Primary: customtypes.LabelAdapter{Name: "n", Value: "v"},
		Labels:  []customtypes.LabelAdapter{{Name: "a", Value: "1"}, {Name: "b", Value: "2"}},
	}
	assert.True(t, a.Equal(b))

	c := &ctm.CustomTypeMessageHolder{
		Primary: customtypes.LabelAdapter{Name: "n", Value: "v"},
		Labels:  []customtypes.LabelAdapter{{Name: "a", Value: "1"}, {Name: "b", Value: "DIFFERENT"}},
	}
	assert.False(t, a.Equal(c))
}

// TestCustomTypeMessage_NoHasMethod pins that a singular message
// customtype field does NOT get a Has<Name>() accessor — the user's
// SizeWiresmith() carries presence, mirroring stdtime's
// !t.IsZero() pattern. Without this exclusion the bitmap would say
// "present" for a `tag+0` wire payload but the customtype marshal path
// (which skips SizeWiresmith()==0) would silently drop the field on
// re-marshal — the bug copilot flagged on PR #117.
func TestCustomTypeMessage_NoHasMethod(t *testing.T) {
	holderType := reflect.TypeFor[ctm.CustomTypeMessageHolder]()
	// HasControlSingular should exist (default singular-message presence
	// bitmap), HasPrimary should NOT (singular customtype message opts out).
	_, ctrlHas := reflect.PointerTo(holderType).MethodByName("HasControlSingular")
	assert.True(t, ctrlHas, "default singular message field should have a Has<Name>() method")
	_, primaryHas := reflect.PointerTo(holderType).MethodByName("HasPrimary")
	assert.False(t, primaryHas, "singular customtype message field must not emit Has<Name>() — user type owns presence")
}

// TestCustomTypeMessage_GetterOnNilReceiver pins the nil-safety contract
// for the swapped accessors — the singular returns `var zero T` (avoiding
// the `*Msg` shape the non-customtype message getter uses), the repeated
// returns a nil slice.
func TestCustomTypeMessage_GetterOnNilReceiver(t *testing.T) {
	var m *ctm.CustomTypeMessageHolder
	assert.Equal(t, customtypes.LabelAdapter{}, m.GetPrimary())
	assert.Nil(t, m.GetLabels())
}
