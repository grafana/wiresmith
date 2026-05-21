package basic

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	num "wiresmith/gen/basic/numeric/v1"
	ks "wiresmith/gen/test/kitchensink/v1"
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

// TestMixedModifiers_OptionalBytes_EmptyPresence covers a wire shape that
// fuzzing surfaced: an optional_bytes field that appears twice — first with
// a payload, then with length zero. The second pass left OptionalBytes as
// `[]byte{}` (non-nil empty), which encodes to 2 bytes (tag + len 0) but
// the naive `append(nil[:0], emptySrc...)` from a fresh message yields nil,
// so a re-unmarshal of the re-marshaled bytes dropped presence and emitted
// 0 bytes the second time.
func TestMixedModifiers_OptionalBytes_EmptyPresence(t *testing.T) {
	// Wire: optional_bytes=[0xff], then optional_bytes=[].
	wire := []byte{0x5a, 0x01, 0xff, 0x5a, 0x00}
	m1 := &num.MixedModifiers{}
	require.NoError(t, m1.Unmarshal(wire))
	require.NotNil(t, m1.OptionalBytes, "after second-pass empty append, OptionalBytes must remain present")
	assert.Equal(t, 0, len(m1.OptionalBytes))

	b1, err := m1.Marshal()
	require.NoError(t, err)

	m2 := &num.MixedModifiers{}
	require.NoError(t, m2.Unmarshal(b1))
	require.NotNil(t, m2.OptionalBytes, "second unmarshal must preserve presence")

	b2, err := m2.Marshal()
	require.NoError(t, err)
	assert.Equal(t, b1, b2, "round-trip must be byte-stable")
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

// Negative zero must survive marshal/unmarshal. In Go, -0.0 == 0.0 is true,
// so a naive `if v != 0` check elides the field and loses the sign bit.
// google.golang.org/protobuf guards this with `v == 0 && !math.Signbit(v)`;
// wiresmith uses the equivalent `math.Float{32,64}bits(v) != 0` predicate.
func TestNegativeZero_SurvivesRoundTrip(t *testing.T) {
	negZero64 := math.Copysign(0, -1)
	negZero32 := float32(math.Copysign(0, -1))

	t.Run("MixedModifiers/regular_double", func(t *testing.T) {
		src := &num.MixedModifiers{RegularDouble: negZero64}
		b, err := src.Marshal()
		require.NoError(t, err)
		require.NotEmpty(t, b, "marshal of -0.0 must emit the field")
		assert.Equal(t, src.Size(), len(b))

		dst := &num.MixedModifiers{}
		require.NoError(t, dst.Unmarshal(b))
		assert.True(t, math.Signbit(dst.RegularDouble), "negative zero sign bit must survive round-trip; got %v", dst.RegularDouble)
	})

	t.Run("AllScalars/field_double", func(t *testing.T) {
		src := &ks.AllScalars{FieldDouble: negZero64}
		b, err := src.Marshal()
		require.NoError(t, err)
		require.NotEmpty(t, b)
		assert.Equal(t, src.Size(), len(b))

		dst := &ks.AllScalars{}
		require.NoError(t, dst.Unmarshal(b))
		assert.True(t, math.Signbit(dst.FieldDouble), "negative zero sign bit must survive round-trip; got %v", dst.FieldDouble)
	})

	t.Run("AllScalars/field_float", func(t *testing.T) {
		src := &ks.AllScalars{FieldFloat: negZero32}
		b, err := src.Marshal()
		require.NoError(t, err)
		require.NotEmpty(t, b)
		assert.Equal(t, src.Size(), len(b))

		dst := &ks.AllScalars{}
		require.NoError(t, dst.Unmarshal(b))
		assert.True(t, math.Signbit(float64(dst.FieldFloat)), "negative zero sign bit must survive round-trip; got %v", dst.FieldFloat)
	})
}

// Positive zero must continue to be elided. Field-presence semantics for
// singular non-optional doubles/floats are "default value is the zero wire
// representation"; +0.0 must not appear on the wire.
func TestPositiveZero_StillElided(t *testing.T) {
	t.Run("MixedModifiers/regular_double", func(t *testing.T) {
		src := &num.MixedModifiers{RegularDouble: 0.0}
		b, err := src.Marshal()
		require.NoError(t, err)
		assert.Empty(t, b, "marshal of +0.0 must elide the field")
		assert.Equal(t, 0, src.Size())
	})

	t.Run("AllScalars/field_float", func(t *testing.T) {
		src := &ks.AllScalars{FieldFloat: 0.0}
		b, err := src.Marshal()
		require.NoError(t, err)
		assert.Empty(t, b)
		assert.Equal(t, 0, src.Size())
	})
}
