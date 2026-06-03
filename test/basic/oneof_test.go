package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	oopb "github.com/grafana/wiresmith/gen/basic/oneof/v1"
)

func TestMultiOneof_BothSet(t *testing.T) {
	msg := &oopb.MultiOneof{
		Primary:      &oopb.MultiOneof_StrVal{StrVal: "hello"},
		Secondary:    &oopb.MultiOneof_MsgVal{MsgVal: oopb.Payload{Data: "payload", Number: 42}},
		RegularField: "regular",
	}
	roundTrip(t, msg)
}

func TestMultiOneof_OnlyPrimary(t *testing.T) {
	msg := &oopb.MultiOneof{
		Primary: &oopb.MultiOneof_IntVal{IntVal: 99},
	}
	roundTrip(t, msg)
}

func TestMultiOneof_OnlySecondary(t *testing.T) {
	msg := &oopb.MultiOneof{
		Secondary: &oopb.MultiOneof_BytesVal{BytesVal: []byte("raw")},
	}
	roundTrip(t, msg)
}

func TestMultiOneof_NeitherSet(t *testing.T) {
	roundTrip(t, &oopb.MultiOneof{})
}

func TestOneofWithTypes_StringVariant(t *testing.T) {
	msg := &oopb.OneofWithTypes{
		Value: &oopb.OneofWithTypes_StrVal{StrVal: "text"},
	}
	roundTrip(t, msg)
}

func TestOneofWithTypes_MessageVariant(t *testing.T) {
	msg := &oopb.OneofWithTypes{
		Value: &oopb.OneofWithTypes_MsgVal{MsgVal: oopb.Payload{Data: "nested", Number: 7}},
	}
	roundTrip(t, msg)
}

func TestOneofWithTypes_EnumVariant(t *testing.T) {
	msg := &oopb.OneofWithTypes{
		Value: &oopb.OneofWithTypes_EnumVal{EnumVal: oopb.Shape_SHAPE_TRIANGLE},
	}
	roundTrip(t, msg)
}

func TestOneofWithTypes_MessageEqual(t *testing.T) {
	a := &oopb.OneofWithTypes{
		Value: &oopb.OneofWithTypes_MsgVal{MsgVal: oopb.Payload{Data: "same", Number: 1}},
	}
	b := &oopb.OneofWithTypes{
		Value: &oopb.OneofWithTypes_MsgVal{MsgVal: oopb.Payload{Data: "same", Number: 1}},
	}
	assert.True(t, a.Equal(b), "independently constructed messages with same payload must be Equal")

	c := &oopb.OneofWithTypes{
		Value: &oopb.OneofWithTypes_MsgVal{MsgVal: oopb.Payload{Data: "different", Number: 2}},
	}
	assert.False(t, a.Equal(c))
}

func TestOneofPlusEverything_FullyPopulated(t *testing.T) {
	score := 9.5
	msg := &oopb.OneofPlusEverything{
		Name:   "test",
		Values: []int64{1, 2, 3},
		Score:  &score,
		Labels: map[string]string{"env": "prod", "team": "core"},
		Payload: &oopb.OneofPlusEverything_Structured{
			Structured: oopb.Payload{Data: "structured", Number: 100},
		},
	}
	mapRoundTrip(t, msg)
}

func TestOneofPlusEverything_TextPayload(t *testing.T) {
	msg := &oopb.OneofPlusEverything{
		Name:    "minimal",
		Payload: &oopb.OneofPlusEverything_Text{Text: "just text"},
	}
	roundTrip(t, msg)
}

func TestOneofPlusEverything_NoOneofSet(t *testing.T) {
	msg := &oopb.OneofPlusEverything{
		Name:   "no-oneof",
		Values: []int64{42},
	}

	b, err := msg.Marshal()
	require.NoError(t, err)

	dst := &oopb.OneofPlusEverything{}
	require.NoError(t, dst.Unmarshal(b))
	assert.Nil(t, dst.Payload)
}
