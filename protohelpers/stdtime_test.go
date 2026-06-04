package protohelpers

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// encodeTimestamp is a self-contained Size+Encode wrapper that the round-
// trip tests use as a stand-in for the generated marshaler: it reserves
// SizeStdTime(t) bytes, writes via EncodeStdTime, and returns the payload
// in forward (network) order. Mirrors what the generated EmitMarshal path
// does for a singular stdtime field, minus the outer envelope.
func encodeTimestamp(t time.Time) []byte {
	size := SizeStdTime(t)
	buf := make([]byte, size)
	offset := EncodeStdTime(buf, size, t)
	return buf[offset:]
}

// TestSizeStdTime_ZeroIsZero locks down the gogoproto-compatible presence
// rule: a Go-zero time.Time{} reports SizeStdTime == 0 so the caller can
// skip the entire envelope. Year 1 (Go's zero) is the only value where
// IsZero() returns true.
func TestSizeStdTime_ZeroIsZero(t *testing.T) {
	assert.Equal(t, 0, SizeStdTime(time.Time{}))
}

// TestSizeStdTime_KnownSizes pins concrete byte counts so wire-format
// changes show up here, not as a downstream test failure.
func TestSizeStdTime_KnownSizes(t *testing.T) {
	cases := []struct {
		name string
		t    time.Time
		want int
	}{
		// Unix epoch is suppressed inner-field-wise (seconds=0 nanos=0),
		// but the outer envelope still exists — that's the caller's call.
		{"epoch", time.Unix(0, 0).UTC(), 0},
		{"seconds_only", time.Unix(1, 0).UTC(), 2},          // tag (1) + varint (1)
		{"nanos_only", time.Unix(0, 1).UTC(), 2},            // tag (1) + varint (1)
		{"seconds_and_nanos", time.Unix(1, 1).UTC(), 4},     // 2 + 2
		{"big_seconds", time.Unix(1<<32, 0).UTC(), 1 + 5},   // tag + 5-byte varint
		{"big_nanos", time.Unix(0, 999999999).UTC(), 1 + 5}, // tag + 5-byte varint (30-bit value)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, SizeStdTime(tc.t))
		})
	}
}

// TestRoundTrip walks a representative cross-section of timestamps through
// SizeStdTime → EncodeStdTime → DecodeStdTime and asserts the instant is
// preserved. UTC normalisation is part of the DecodeStdTime contract; the
// constructed time.Time's zone is irrelevant after decode.
func TestRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		t    time.Time
	}{
		{"epoch_explicit", time.Unix(0, 0).UTC()},
		{"recent", time.Date(2026, 6, 3, 12, 30, 45, 123456789, time.UTC)},
		{"year_2", time.Date(2, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"year_9999", time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC)},
		{"negative_unix", time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"max_nanos", time.Unix(1, 999999999).UTC()},
		{"min_positive_nanos", time.Unix(0, 1).UTC()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := encodeTimestamp(tc.t)
			got, err := DecodeStdTime(payload)
			require.NoError(t, err)
			assert.True(t, got.Equal(tc.t), "instant mismatch: got %v want %v", got, tc.t)
			assert.Equal(t, time.UTC, got.Location(), "decoded time must be UTC")
		})
	}
}

// TestDecodeStdTime_Empty pins the empty-payload contract: an explicitly
// present but inner-zero Timestamp envelope (e.g. time.Unix(0,0)) decodes
// to time.Unix(0, 0).UTC(), which is the epoch — distinct from the
// IsZero() Go-zero.
func TestDecodeStdTime_Empty(t *testing.T) {
	got, err := DecodeStdTime(nil)
	require.NoError(t, err)
	assert.True(t, got.Equal(time.Unix(0, 0).UTC()))
	assert.False(t, got.IsZero(), "empty payload decodes as Unix epoch, not Go zero")
}

// TestDecodeStdTime_NormalisesToUTC pins the zone normalisation contract.
// The Decoder takes raw seconds/nanos and reconstructs the instant in UTC;
// callers don't need to set Location themselves.
func TestDecodeStdTime_NormalisesToUTC(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	src := time.Date(2026, 6, 3, 21, 0, 0, 0, tokyo)

	payload := encodeTimestamp(src)
	got, err := DecodeStdTime(payload)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, got.Location())
	assert.True(t, got.Equal(src), "instant must round-trip even though zone is normalised")
}

// TestDecodeStdTime_UnknownField pins forward compatibility: an inner field
// number 3 (currently unallocated by google.protobuf.Timestamp) must be
// skipped without error. This routes through protowire.ConsumeFieldValue.
func TestDecodeStdTime_UnknownField(t *testing.T) {
	// Build a payload with seconds=42, then an unknown field 3 (varint, value=99).
	// Tag for field 3 varint = (3 << 3) | 0 = 0x18.
	payload := []byte{
		0x08, 42, // field 1 (seconds), varint, 42
		0x18, 99, // field 3 (unknown), varint, 99
	}
	got, err := DecodeStdTime(payload)
	require.NoError(t, err)
	assert.True(t, got.Equal(time.Unix(42, 0).UTC()))
}

// TestDecodeStdTime_WireTypeMismatch pins behaviour when the encoder sent
// the canonical seconds/nanos numbers but with the wrong wire type — the
// decoder skips them via ConsumeFieldValue rather than crashing. Future
// schemas that change the field encoding shouldn't blow up old binaries.
func TestDecodeStdTime_WireTypeMismatch(t *testing.T) {
	// Field 1 with BytesType (wire 2) instead of VarintType: payload contains
	// a single zero-length bytes value. The decoder should skip it and leave
	// seconds at zero.
	payload := []byte{0x0a, 0x00}
	got, err := DecodeStdTime(payload)
	require.NoError(t, err)
	assert.True(t, got.Equal(time.Unix(0, 0).UTC()))
}

// TestDecodeStdTime_TruncatedVarint pins that a truncated inner varint
// produces a parse error (not a silent zero). io.ErrUnexpectedEOF is the
// canonical protowire short-buffer error.
func TestDecodeStdTime_TruncatedVarint(t *testing.T) {
	// Tag 0x08 (field 1, varint), then a continuation byte with no terminator.
	payload := []byte{0x08, 0x80}
	_, err := DecodeStdTime(payload)
	require.Error(t, err)
	assert.True(t, errors.Is(err, io.ErrUnexpectedEOF), "got %v", err)
}

// TestDecodeStdTime_InvalidFieldNumber pins that field number 0 (the
// only invalid protobuf field number) is rejected. The protowire ConsumeTag
// itself doesn't validate ranges; the helper layers that check on top.
func TestDecodeStdTime_InvalidFieldNumber(t *testing.T) {
	// Tag byte 0x00 encodes field number 0, wire type 0 — invalid per spec.
	payload := []byte{0x00, 0x00}
	_, err := DecodeStdTime(payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid field number")
}

// TestCrossLibrary pins on-wire compatibility with the canonical
// google.golang.org/protobuf timestamppb. Both libraries must produce
// byte-identical encodings of the same instant — otherwise stdtime
// silently breaks interop with the rest of the protobuf ecosystem.
func TestCrossLibrary(t *testing.T) {
	cases := []struct {
		name string
		t    time.Time
	}{
		{"recent", time.Date(2026, 6, 3, 12, 30, 45, 123456789, time.UTC)},
		{"year_2", time.Date(2, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"negative_unix", time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ours := encodeTimestamp(tc.t)
			theirs, err := proto.Marshal(timestamppb.New(tc.t))
			require.NoError(t, err)
			assert.Equal(t, theirs, ours, "wire payload must match google.golang.org/protobuf")

			// And in the other direction: decode their bytes.
			got, err := DecodeStdTime(theirs)
			require.NoError(t, err)
			assert.True(t, got.Equal(tc.t))
		})
	}
}
