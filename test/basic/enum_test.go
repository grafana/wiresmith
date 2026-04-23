package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"

	enumpb "wiresmith/gen/basic/enum/v1"
)

func TestAliasedPriority_NameValueMaps(t *testing.T) {
	// First name for a numeric value wins in the name map.
	assert.Equal(t, "ALIASED_PRIORITY_NORMAL", enumpb.AliasedPriority_name[2])
	assert.Equal(t, "ALIASED_PRIORITY_HIGH", enumpb.AliasedPriority_name[3])

	// All aliases present in the value map.
	assert.Equal(t, int32(2), enumpb.AliasedPriority_value["ALIASED_PRIORITY_NORMAL"])
	assert.Equal(t, int32(2), enumpb.AliasedPriority_value["ALIASED_PRIORITY_DEFAULT"])
	assert.Equal(t, int32(3), enumpb.AliasedPriority_value["ALIASED_PRIORITY_HIGH"])
	assert.Equal(t, int32(3), enumpb.AliasedPriority_value["ALIASED_PRIORITY_CRITICAL"])
}

func TestSignedEnum_NegativeValue(t *testing.T) {
	assert.Equal(t, "SIGNED_NEGATIVE", enumpb.SignedEnum(-1).String())
	assert.Equal(t, int32(-1), enumpb.SignedEnum_value["SIGNED_NEGATIVE"])
}

func TestWithNestedEnum_RoundTrip(t *testing.T) {
	msg := &enumpb.WithNestedEnum{
		Priority: enumpb.PRIORITY_HIGH,
		Priorities: []enumpb.WithNestedEnum_Priority{
			enumpb.PRIORITY_LOW,
			enumpb.PRIORITY_MEDIUM,
			enumpb.PRIORITY_HIGH,
		},
		Name: "test",
	}
	roundTrip(t, msg)
}

func TestEnumContainer_FullyPopulated(t *testing.T) {
	optSigned := enumpb.SIGNED_NEGATIVE
	msg := &enumpb.EnumContainer{
		Aliased:        enumpb.ALIASED_PRIORITY_DEFAULT,
		Signed:         enumpb.SIGNED_NEGATIVE,
		OptionalSigned: &optSigned,
		RepeatedAliased: []enumpb.AliasedPriority{
			enumpb.ALIASED_PRIORITY_LOW,
			enumpb.ALIASED_PRIORITY_CRITICAL,
		},
		SignedMap: map[string]enumpb.SignedEnum{
			"pos": enumpb.SIGNED_POSITIVE,
			"neg": enumpb.SIGNED_NEGATIVE,
		},
	}
	mapRoundTrip(t, msg)
}

func TestEnumContainer_NilOptional(t *testing.T) {
	msg := &enumpb.EnumContainer{
		Signed: enumpb.SIGNED_POSITIVE,
	}
	roundTrip(t, msg)
}
