package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protowire"

	commonv1 "wiresmith/gen/opentelemetry/proto/common/v1"
)

// SEC-4: 10th-byte varint overflow rejection. A 10-byte varint whose final
// byte has any data bit above bit 0 set (or its continuation bit set) is
// malformed: the spec only allows bit 0 of the 10th byte to contribute to
// uint64 bit 63. Without the new shift==63 guard the inline decoder silently
// dropped the upper bits, producing a parseable but wrong value.

// tenByteVarint returns a 10-byte varint where bytes 1-9 are bare continuation
// (no data bits) and byte 10 holds finalByte. By choosing finalByte the caller
// drives whether the value is well-formed or overflows past uint64.
func tenByteVarint(finalByte byte) []byte {
	out := make([]byte, 10)
	for i := range 9 {
		out[i] = 0x80
	}
	out[9] = finalByte
	return out
}

func TestUnmarshalRejectsVarintOverflow(t *testing.T) {
	t.Run("varint scalar field: 10th-byte data bits > 1", func(t *testing.T) {
		// AnyValue.int_value is field 3, wire type 0 (varint). Bytes 1-9 set
		// no data, byte 10 has data 0x7F (>1). Pre-fix this would silently
		// shift those bits past uint64 and accept the message.
		var b []byte
		b = protowire.AppendTag(b, 3, protowire.VarintType)
		b = append(b, tenByteVarint(0x7F)...)

		var av commonv1.AnyValue
		err := av.Unmarshal(b)
		assert.Error(t, err, "expected error for varint with overflow in 10th byte")
	})

	t.Run("varint scalar field: 11-byte varint (10th byte continuation)", func(t *testing.T) {
		// Byte 10 = 0x80 → indicates an 11th byte exists. This case is caught
		// by the existing `shift >= 64` guard on the next iteration, not by
		// the new 10th-byte data-bit guard (which only runs on the terminator
		// path). Kept here as a defence-in-depth check that 11-byte varints
		// stay rejected end-to-end.
		var b []byte
		b = protowire.AppendTag(b, 3, protowire.VarintType)
		b = append(b, tenByteVarint(0x80)...)

		var av commonv1.AnyValue
		err := av.Unmarshal(b)
		assert.Error(t, err, "expected error for 11-byte varint")
	})

	t.Run("length-delimited prefix: 10th-byte data bits > 1", func(t *testing.T) {
		// AnyValue.string_value is field 1, wire type 2. Use 0x02 in the 10th
		// byte: bit 1 shifted by 63 truncates entirely, so the corrupted
		// length is 0 — well below the MaxInt guard and trivially satisfying
		// postIndex<=l with no payload. Pre-fix this would silently accept
		// an empty string; post-fix the new break-path guard rejects it.
		// 0x7F here would just trip MaxInt and pass for the wrong reason.
		var b []byte
		b = protowire.AppendTag(b, 1, protowire.BytesType)
		b = append(b, tenByteVarint(0x02)...)

		var av commonv1.AnyValue
		err := av.Unmarshal(b)
		assert.Error(t, err, "expected error for length varint with overflow in 10th byte")
	})

	t.Run("unknown field length via skipValue: 10th-byte overflow", func(t *testing.T) {
		// Field 99 routes through skipValue case 2 (length-delimited). Same
		// reasoning as the length-prefix subtest above: 0x02 truncates to a
		// zero length pre-fix, so the new guard is the only thing rejecting
		// it. No payload byte is appended — length is 0.
		var b []byte
		b = protowire.AppendTag(b, 99, protowire.BytesType)
		b = append(b, tenByteVarint(0x02)...)

		var av commonv1.AnyValue
		err := av.Unmarshal(b)
		assert.Error(t, err, "expected error for unknown-field length varint overflow")
	})

	t.Run("well-formed 10-byte varint with 10th byte == 0x01 is accepted", func(t *testing.T) {
		// Boundary case: 10th byte == 0x01 is the largest legal 10th byte
		// (b > 1 is the guard, strict inequality). The value is bit 63 set
		// only → MinInt64 when interpreted as int64. Must NOT error.
		var b []byte
		b = protowire.AppendTag(b, 3, protowire.VarintType)
		b = append(b, tenByteVarint(0x01)...)

		var av commonv1.AnyValue
		err := av.Unmarshal(b)
		assert.NoError(t, err, "valid 10-byte varint must be accepted")
	})
}
