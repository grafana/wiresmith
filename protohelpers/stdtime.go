package protohelpers

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

// SizeStdTime returns the wire size of t encoded as a Timestamp payload
// (the bytes inside the outer length-delimited header). Returns 0 for the
// Go zero time, which is treated as "not set" by the
// (wiresmith.options.stdtime) contract.
//
// Skips zero seconds and zero nanos individually, matching proto3 default-
// suppression rules on the inner Timestamp fields. The result lets the
// caller reserve exactly the right number of bytes ahead of EncodeStdTime.
func SizeStdTime(t time.Time) int {
	if t.IsZero() {
		return 0
	}
	return sizeStdTimeFields(t)
}

// SizeStdTimeAlways is SizeStdTime without the whole-value "IsZero means
// absent" special case, for callers where presence is tracked separately
// (a `*time.Time`, as in the proto3 `optional` stdtime field type) rather
// than by treating the Go zero time as a sentinel. This matters because
// `time.Time{}` (year 1) is not the same instant as the Unix epoch: its
// Unix-seconds value is a large non-zero number, so — unlike
// `time.Duration(0)`, whose seconds/nanos are always both exactly zero —
// it does NOT fall out of the per-field zero-suppression in
// sizeStdTimeFields on its own. A caller that reserves SizeStdTime(t) bytes
// (0, via the IsZero short-circuit) but then unconditionally encodes via
// EncodeStdTime would write past the reserved buffer for exactly this
// value; SizeStdTimeAlways/EncodeStdTime is the correct size/encode pair
// for a caller that always encodes when non-nil.
func SizeStdTimeAlways(t time.Time) int {
	return sizeStdTimeFields(t)
}

// sizeStdTimeFields computes the per-field (seconds, nanos) wire size with
// proto3 default-suppression, shared by SizeStdTime and SizeStdTimeAlways.
func sizeStdTimeFields(t time.Time) int {
	seconds := t.Unix()
	nanos := int32(t.Nanosecond())
	n := 0
	if seconds != 0 {
		n += 1 + SizeOfVarint(uint64(seconds))
	}
	if nanos != 0 {
		n += 1 + SizeOfVarint(uint64(nanos))
	}
	return n
}

// EncodeStdTime writes the Timestamp payload for t in reverse-write order
// into dAtA, ending at offset. Caller must reserve SizeStdTime(t) bytes
// before offset and must check t.IsZero() before invoking — EncodeStdTime
// does not gate the envelope itself.
//
// nanos (field 2) is written first so the resulting bytes appear in
// ascending tag order — the canonical proto3 emit order — once the caller
// flips the slice around via the reverse-write convention.
func EncodeStdTime(dAtA []byte, offset int, t time.Time) int {
	seconds := t.Unix()
	nanos := int32(t.Nanosecond())
	if nanos != 0 {
		offset = EncodeVarint(dAtA, offset, uint64(nanos))
		offset--
		dAtA[offset] = 0x10
	}
	if seconds != 0 {
		offset = EncodeVarint(dAtA, offset, uint64(seconds))
		offset--
		dAtA[offset] = 0x08
	}
	return offset
}

// DecodeStdTime parses a Timestamp payload (the bytes inside the outer
// length-delimited header) and returns the corresponding UTC time.Time.
// Used by (wiresmith.options.stdtime) field unmarshalers.
//
// UTC is the canonical Timestamp zone in the proto spec; even malformed-
// but-valid wire data that came from a non-UTC source decodes consistently
// because seconds/nanos carry no zone information themselves.
//
// Unknown inner fields and wire-type mismatches on the two known fields
// are skipped via protowire.ConsumeFieldValue — a Timestamp encoded by a
// future schema with extra inner fields stays decodable, in keeping with
// the rest of the generated unmarshal's tolerance to forward-compatible
// additions.
func DecodeStdTime(b []byte) (time.Time, error) {
	var seconds int64
	var nanos int32
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return time.Time{}, protowire.ParseError(n)
		}
		if num < 1 {
			return time.Time{}, fmt.Errorf("proto: invalid field number")
		}
		b = b[n:]
		switch {
		case num == 1 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return time.Time{}, protowire.ParseError(n)
			}
			seconds = int64(v)
			b = b[n:]
		case num == 2 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return time.Time{}, protowire.ParseError(n)
			}
			nanos = int32(v)
			b = b[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return time.Time{}, protowire.ParseError(n)
			}
			b = b[n:]
		}
	}
	return time.Unix(seconds, int64(nanos)).UTC(), nil
}
