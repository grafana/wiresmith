package basic

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protowire"

	commonv1 "wiresmith/gen/opentelemetry/proto/common/v1"
)

// SEC-3: 32-bit length-decode truncation. A length varint encoding a value
// above MaxInt must be rejected, not silently truncated to a small positive
// int by `int(byteLen)` on 32-bit platforms (GOARCH=386/arm/wasm).
//
// On 64-bit the new `byteLen > uint64(math.MaxInt)` guard fires only for
// values above 2^63-1; smaller values still fail via `postIndex > l`. On
// 32-bit it additionally catches values in (MaxInt32, MaxInt64] that would
// otherwise truncate and bypass the `postIndex > l` bound check entirely.
//
// `GOOS=linux GOARCH=386 go build ./...` exercises the compilation path;
// these runtime tests cover both error paths on the host architecture.

// encodeLenVarint encodes a uint64 as a 10-byte varint header for a
// length-delimited field. Used to inject lengths above MaxInt64 (which a
// well-formed encoder would never produce).
func encodeLenVarint(v uint64) []byte {
	out := make([]byte, 0, 10)
	for i := 0; i < 9; i++ {
		out = append(out, byte(v&0x7F)|0x80)
		v >>= 7
	}
	out = append(out, byte(v&0x7F))
	return out
}

func TestUnmarshalRejectsLengthAboveMaxInt(t *testing.T) {
	// 2^63 — first uint64 above MaxInt64. On amd64 this triggers the
	// new math.MaxInt guard; on 32-bit any value above MaxInt32 triggers
	// it (the same payload still works).
	const huge = uint64(1) << 63

	t.Run("known length-delimited field (string)", func(t *testing.T) {
		// AnyValue.string_value is field 1, wire type 2 (bytes/length-delimited).
		// Inject a malicious length varint, then a single payload byte.
		var b []byte
		b = protowire.AppendTag(b, 1, protowire.BytesType)
		b = append(b, encodeLenVarint(huge)...)
		b = append(b, 'x')

		var av commonv1.AnyValue
		err := av.Unmarshal(b)
		assert.Error(t, err, "expected error for length above MaxInt")
	})

	t.Run("unknown field via skipValue", func(t *testing.T) {
		// Field 99 is unknown to AnyValue, so skipValue handles it.
		// Wire type 2 routes through skipValue case 2 (length-delimited).
		var b []byte
		b = protowire.AppendTag(b, 99, protowire.BytesType)
		b = append(b, encodeLenVarint(huge)...)
		b = append(b, 'x')

		var av commonv1.AnyValue
		err := av.Unmarshal(b)
		assert.Error(t, err, "expected error for unknown-field length above MaxInt")
	})

	t.Run("length equal to MaxInt is well-defined", func(t *testing.T) {
		// MaxInt-sized lengths must still be processed (and then fail
		// via postIndex>l since the payload isn't present). The guard
		// is strict-greater-than, so this is the boundary case.
		var b []byte
		b = protowire.AppendTag(b, 1, protowire.BytesType)
		b = append(b, encodeLenVarint(uint64(math.MaxInt))...)

		var av commonv1.AnyValue
		err := av.Unmarshal(b)
		// Must error (no payload), but the error path is postIndex>l,
		// not the new guard. Either way: error, never silent truncation.
		assert.Error(t, err)
	})
}
