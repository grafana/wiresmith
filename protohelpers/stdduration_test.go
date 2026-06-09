package protohelpers

import (
	"errors"
	"io"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

// encodeDuration is a self-contained Size+Encode wrapper that the round-
// trip tests use as a stand-in for the generated marshaler: it reserves
// SizeStdDuration(d) bytes, writes via EncodeStdDuration, and returns the
// payload in forward (network) order. Mirrors the encodeTimestamp helper
// stdtime tests use.
func encodeDuration(d time.Duration) []byte {
	size := SizeStdDuration(d)
	buf := make([]byte, size)
	offset := EncodeStdDuration(buf, size, d)
	return buf[offset:]
}

// TestSizeStdDuration_ZeroIsZero locks down the gogoproto-compatible
// presence rule: time.Duration(0) reports SizeStdDuration == 0 so the
// caller can skip the entire envelope. Unlike time.Time, time.Duration
// has only one zero value, so this is also the only "absent" payload.
func TestSizeStdDuration_ZeroIsZero(t *testing.T) {
	assert.Equal(t, 0, SizeStdDuration(0))
}

// TestSizeStdDuration_KnownSizes pins concrete byte counts so wire-format
// changes show up here, not as a downstream test failure.
func TestSizeStdDuration_KnownSizes(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
		want int
	}{
		{"one_second", time.Second, 2},                // tag (1) + varint (1)
		{"one_nano", time.Nanosecond, 2},              // tag (1) + varint (1)
		{"one_second_one_nano", time.Second + 1, 4},   // 2 + 2
		{"negative_one_second", -time.Second, 1 + 10}, // negative varint = 10 bytes
		{"big_seconds", time.Duration(1<<32) * time.Second, 1 + 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, SizeStdDuration(tc.d))
		})
	}
}

// TestStdDurationRoundTrip walks a representative cross-section of
// durations through SizeStdDuration → EncodeStdDuration → DecodeStdDuration
// and asserts the value is preserved.
func TestStdDurationRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
	}{
		{"one_second", time.Second},
		{"one_nano", time.Nanosecond},
		{"complex", 5*time.Hour + 23*time.Minute + 17*time.Second + 123456789*time.Nanosecond},
		{"negative", -42 * time.Second},
		{"negative_complex", -(2*time.Hour + 30*time.Minute + 1*time.Nanosecond)},
		{"max_int64", time.Duration(math.MaxInt64)},
		{"min_int64", time.Duration(math.MinInt64)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := encodeDuration(tc.d)
			got, err := DecodeStdDuration(payload)
			require.NoError(t, err)
			assert.Equal(t, tc.d, got, "value mismatch")
		})
	}
}

// TestDecodeStdDuration_Empty pins the empty-payload contract: an
// explicitly present but inner-zero Duration envelope (e.g. all defaults
// suppressed) decodes to time.Duration(0), which is the same as "absent".
// This is the price of using zero as the presence sentinel — explicit
// zero on the wire collapses with absent.
func TestDecodeStdDuration_Empty(t *testing.T) {
	got, err := DecodeStdDuration(nil)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), got)
}

// TestDecodeStdDuration_OverflowSaturates pins the saturation contract.
// A proto Duration with seconds far past math.MaxInt64 nanoseconds must
// decode to math.MaxInt64 rather than wrap silently — matches
// (*durationpb.Duration).AsDuration().
func TestDecodeStdDuration_OverflowSaturates(t *testing.T) {
	// Build a payload with seconds = math.MaxInt64 (well past what fits
	// in time.Duration's nanosecond budget).
	payload := []byte{0x08} // field 1, varint
	payload = appendVarint(payload, uint64(math.MaxInt64))
	got, err := DecodeStdDuration(payload)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(math.MaxInt64), got, "positive overflow must saturate to MaxInt64")
}

// TestDecodeStdDuration_NegativeOverflowSaturates pins the saturation
// contract on the negative side: seconds far past math.MinInt64
// nanoseconds saturates to math.MinInt64.
func TestDecodeStdDuration_NegativeOverflowSaturates(t *testing.T) {
	// seconds = math.MinInt64 (encoded as a 10-byte varint via the
	// uint64 reinterpretation that protobuf uses for negative int64s).
	var minSec int64 = math.MinInt64
	payload := []byte{0x08}
	payload = appendVarint(payload, uint64(minSec))
	got, err := DecodeStdDuration(payload)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(math.MinInt64), got, "negative overflow must saturate to MinInt64")
}

// TestDecodeStdDuration_UnknownField pins forward compatibility: an inner
// field number 3 (currently unallocated by google.protobuf.Duration) must
// be skipped without error. Same shape as the stdtime UnknownField test.
func TestDecodeStdDuration_UnknownField(t *testing.T) {
	// seconds=42, then an unknown field 3 (varint, value=99).
	// Tag for field 3 varint = (3 << 3) | 0 = 0x18.
	payload := []byte{
		0x08, 42, // field 1 (seconds), varint, 42
		0x18, 99, // field 3 (unknown), varint, 99
	}
	got, err := DecodeStdDuration(payload)
	require.NoError(t, err)
	assert.Equal(t, 42*time.Second, got)
}

// TestDecodeStdDuration_WireTypeMismatch pins behaviour when the encoder
// sent the canonical seconds/nanos numbers but with the wrong wire type:
// the decoder skips them via ConsumeFieldValue rather than crashing.
func TestDecodeStdDuration_WireTypeMismatch(t *testing.T) {
	// Field 1 with BytesType (wire 2) instead of VarintType: payload contains
	// a single zero-length bytes value. The decoder should skip it and leave
	// seconds at zero.
	payload := []byte{0x0a, 0x00}
	got, err := DecodeStdDuration(payload)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), got)
}

// TestDecodeStdDuration_TruncatedVarint pins that a truncated inner varint
// produces a parse error (not a silent zero). io.ErrUnexpectedEOF is the
// canonical protowire short-buffer error.
func TestDecodeStdDuration_TruncatedVarint(t *testing.T) {
	// Tag 0x08 (field 1, varint), then a continuation byte with no terminator.
	payload := []byte{0x08, 0x80}
	_, err := DecodeStdDuration(payload)
	require.Error(t, err)
	assert.True(t, errors.Is(err, io.ErrUnexpectedEOF), "got %v", err)
}

// TestDecodeStdDuration_InvalidFieldNumber pins that field number 0 (the
// only invalid protobuf field number) is rejected.
func TestDecodeStdDuration_InvalidFieldNumber(t *testing.T) {
	// Tag byte 0x00 encodes field number 0, wire type 0 — invalid per spec.
	payload := []byte{0x00, 0x00}
	_, err := DecodeStdDuration(payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid field number")
}

// TestStdDurationCrossLibrary pins on-wire compatibility with the canonical
// google.golang.org/protobuf durationpb. Both libraries must produce
// byte-identical encodings of the same duration — otherwise stdduration
// silently breaks interop with the rest of the protobuf ecosystem.
func TestStdDurationCrossLibrary(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
	}{
		{"one_second", time.Second},
		{"complex", 5*time.Hour + 23*time.Minute + 17*time.Second + 123456789*time.Nanosecond},
		{"negative", -42 * time.Second},
		{"min_nano", time.Nanosecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ours := encodeDuration(tc.d)
			theirs, err := proto.Marshal(durationpb.New(tc.d))
			require.NoError(t, err)
			assert.Equal(t, theirs, ours, "wire payload must match google.golang.org/protobuf")

			// And in the other direction: decode their bytes.
			got, err := DecodeStdDuration(theirs)
			require.NoError(t, err)
			assert.Equal(t, tc.d, got)
		})
	}
}

// appendVarint is a local test helper: encodes v into the canonical varint
// shape and appends it. Avoids dragging in protowire just to build a test
// payload.
func appendVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}
