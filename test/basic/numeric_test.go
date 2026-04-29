package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	num "wiresmith/gen/basic/numeric/v1"
)

func TestUnpackedScalars_RoundTrip(t *testing.T) {
	msg := &num.UnpackedScalars{
		FieldDouble:   []float64{1.1, 2.2, 3.3},
		FieldFloat:    []float32{1.0, 2.0},
		FieldInt32:    []int32{-1, 0, 1, 2147483647},
		FieldInt64:    []int64{-1, 0, 9223372036854775807},
		FieldUint32:   []uint32{0, 4294967295},
		FieldUint64:   []uint64{0, 18446744073709551615},
		FieldSint32:   []int32{-2147483648, 0, 2147483647},
		FieldSint64:   []int64{-9223372036854775808, 0, 9223372036854775807},
		FieldFixed32:  []uint32{0, 4294967295},
		FieldFixed64:  []uint64{0, 18446744073709551615},
		FieldSfixed32: []int32{-2147483648, 2147483647},
		FieldSfixed64: []int64{-9223372036854775808, 9223372036854775807},
		FieldBool:     []bool{true, false, true},
	}
	b := roundTrip(t, msg)
	assert.Greater(t, len(b), 0)
}

func TestUnpackedScalars_Empty(t *testing.T) {
	roundTrip(t, &num.UnpackedScalars{})
}

func TestMixedModifiers_FullyPopulated(t *testing.T) {
	optInt := int64(42)
	optDbl := 3.14
	optStr := "hello"
	optBytes := []byte("world")
	msg := &num.MixedModifiers{
		RegularInt:     1,
		OptionalInt:    &optInt,
		RepeatedInt:    []int64{10, 20, 30},
		RegularDouble:  2.718,
		OptionalDouble: &optDbl,
		RepeatedDouble: []float64{1.1, 2.2},
		RegularString:  "regular",
		OptionalString: &optStr,
		RepeatedString: []string{"a", "b", "c"},
		RegularBytes:   []byte("bytes"),
		OptionalBytes:  optBytes,
		RepeatedBytes:  [][]byte{[]byte("x"), []byte("y")},
	}
	roundTrip(t, msg)
}

func TestMixedModifiers_ZeroValueOptionals(t *testing.T) {
	optInt := int64(0)
	optDbl := 0.0
	optStr := ""
	msg := &num.MixedModifiers{
		OptionalInt:    &optInt,
		OptionalDouble: &optDbl,
		OptionalString: &optStr,
	}
	roundTrip(t, msg)
}

func TestWideFields_BitmapBoundary(t *testing.T) {
	msg := &num.WideFields{
		F1:  "first",
		F64: true,
		F65: true,
		F70: true,
	}
	b, err := msg.Marshal()
	require.NoError(t, err)
	assert.Equal(t, msg.Size(), len(b))

	dst := &num.WideFields{}
	require.NoError(t, dst.Unmarshal(b))

	// Fields across the uint64 word boundary must be tracked independently.
	assert.True(t, dst.HasF1())
	assert.False(t, dst.HasF2())
	assert.True(t, dst.HasF64())
	assert.True(t, dst.HasF65())
	assert.False(t, dst.HasF69())
	assert.True(t, dst.HasF70())
}

func TestWideFields_FullyPopulated(t *testing.T) {
	msg := &num.WideFields{}
	msg.F1 = "1"
	msg.F20 = "20"
	msg.F21 = 21
	msg.F40 = 40
	msg.F41 = 41
	msg.F50 = 50
	msg.F51 = 51.0
	msg.F60 = 60.0
	msg.F61 = true
	msg.F70 = true
	roundTrip(t, msg)
}
