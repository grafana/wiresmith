package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nest "wiresmith/gen/basic/nesting/v1"
)

func TestLevel0_FullDepth(t *testing.T) {
	msg := &nest.Level0{
		Label: "root",
		Child: nest.Level0_Level1{
			Value: "l1",
			Child: nest.Level0_Level1_Level2{
				Value: "l2",
				Child: nest.Level0_Level1_Level2_Level3{
					DeepValue: "leaf",
					Depth:     3,
				},
			},
			Extras: []nest.Level0_Level1_Level2{
				{Value: "extra1", Child: nest.Level0_Level1_Level2_Level3{DeepValue: "e1"}},
				{Value: "extra2"},
			},
		},
	}
	roundTrip(t, msg)
}

func TestLevel0_Empty(t *testing.T) {
	roundTrip(t, &nest.Level0{})
}

func TestCrossRef_ReferencesNestedTypes(t *testing.T) {
	msg := &nest.CrossRef{
		Tag: "cross",
		NestedRef: nest.Level0_Level1_Level2{
			Value: "nested",
			Child: nest.Level0_Level1_Level2_Level3{DeepValue: "deep"},
		},
		DeepRef: nest.Level0_Level1_Level2_Level3{
			DeepValue: "direct-deep",
			Depth:     99,
		},
	}
	roundTrip(t, msg)
}

func TestLevel0_GeneratedTypeNames(t *testing.T) {
	// Verify the generated type names follow the Parent_Child convention.
	var _ nest.Level0_Level1
	var _ nest.Level0_Level1_Level2
	var _ nest.Level0_Level1_Level2_Level3

	// Verify the deepest level has expected fields.
	l3 := nest.Level0_Level1_Level2_Level3{DeepValue: "v", Depth: 1}
	b, err := l3.Marshal()
	require.NoError(t, err)

	dst := &nest.Level0_Level1_Level2_Level3{}
	require.NoError(t, dst.Unmarshal(b))
	assert.Equal(t, "v", dst.DeepValue)
	assert.Equal(t, int64(1), dst.Depth)
}
