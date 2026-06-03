package types

import (
	"google.golang.org/protobuf/encoding/protowire"
)

// StdtimeType is the FieldType for a singular `google.protobuf.Timestamp`
// field annotated with `(wiresmith.options.stdtime) = true`. The Go-side
// field is a stdlib `time.Time`; the wire format remains the standard
// Timestamp sub-message (int64 seconds field 1, int32 nanos field 2).
//
// Presence semantics: `time.Time{}` (Go's zero, January 1 year 1 UTC) is
// treated as "not set" — Size returns 0, Marshal skips the field entirely,
// and Unmarshal leaves the value at the Go zero when no Timestamp tag is
// seen. This mirrors gogoproto's stdtime contract; see the option doc on
// the proto extension for the rationale.
//
// Decoded times are normalised to UTC. UTC is the canonical timezone for
// protobuf Timestamp; values constructed elsewhere are silently re-zoned by
// the Unmarshal path so a round-trip never depends on the local zone.
//
// The Size/Marshal/Unmarshal call sites are slim and route into per-file
// helpers (sizeStdTime, encodeStdTime, decodeStdTime) emitted once by the
// generator when any stdtime field is present. Pushing the Timestamp envelope
// logic out of every call site keeps the main `.pb.go` readable, avoids
// duplicating the inner-tag decode loop across every stdtime field, and lets
// the encoder hot path share branch state across users of the same package.
type StdtimeType struct{}

// RequiredImports declares "time" (for time.Time access) at every stdtime
// call site. The encoder/decoder helpers themselves pull in protohelpers /
// io / fmt when the generator emits them, so the call sites don't need to
// duplicate that registration.
func (StdtimeType) RequiredImports() []string {
	return []string{"time"}
}

// EmitSize emits the proto-wrapper size accumulator for a singular stdtime
// field. The inner Timestamp payload size is computed by sizeStdTime; the
// `!access.IsZero()` gate makes `time.Time{}` round-trip as "field absent",
// matching the presence semantics documented on the option.
func (StdtimeType) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tif !%s.IsZero() {\n", access)
	e.Writef("\t\tinner := sizeStdTime(%s)\n", access)
	e.Writef("\t\tn += %d + protowire.SizeVarint(uint64(inner)) + inner\n", tagSize)
	e.Writef("\t}\n")
}

// EmitMarshal writes the Timestamp envelope and its two inner fields via
// the per-file encodeStdTime helper, then the outer length prefix and the
// outer field tag. Skips the entire envelope when the Go value is the zero
// `time.Time{}`.
//
// `start := i` snapshots the reverse-write cursor so the post-encode size
// can be recovered as `start - i` without recomputing sizeStdTime (which
// would walk seconds/nanos a second time).
func (StdtimeType) EmitMarshal(e Emitter, access string, num protowire.Number) {
	e.Writef("\tif !%s.IsZero() {\n", access)
	e.Writef("\t\tstart := i\n")
	e.Writef("\t\ti = encodeStdTime(dAtA, i, %s)\n", access)
	e.Writef("\t\tinner := start - i\n")
	e.Writef("\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(inner))\n")
	e.ReverseTag("\t\t", num, protowire.BytesType)
	e.Writef("\t}\n")
}

// EmitUnmarshal consumes the outer length-delimited header, slices out the
// payload, and hands it to decodeStdTime. The helper is responsible for
// bounding inner-tag reads to the slice; the call site keeps the outer
// iNdEx/dAtA/l invariants intact.
func (StdtimeType) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeBytesLen(e)
	e.Writef("\t\t\tstdtimeVal, err := decodeStdTime(dAtA[iNdEx:postIndex])\n")
	e.Writef("\t\t\tif err != nil {\n\t\t\t\treturn err\n\t\t\t}\n")
	e.Writef("\t\t\t%s = stdtimeVal\n", access)
	e.Writef("\t\t\tiNdEx = postIndex\n")
}

// EmitEqual delegates to time.Time.Equal, which compares by instant — the
// canonical contract: two times referring to the same wall-clock instant
// compare equal regardless of how they were constructed. After decode we
// always produce UTC values, so timezone semantics never differ between
// the two sides of the comparison.
func (StdtimeType) EmitEqual(e Emitter, indent, lhs, rhs string) {
	e.Writef("%sif !%s.Equal(%s) {\n%s\treturn false\n%s}\n", indent, lhs, rhs, indent, indent)
}

// EmitCompare uses time.Time.Compare (Go 1.20+), which returns -1/0/+1 by
// instant — the same contract bytes.Compare uses, so the generated Compare
// method's overall ordering stays consistent across field kinds.
func (StdtimeType) EmitCompare(e Emitter, indent, lhs, rhs string) {
	e.Writef("%sif c := %s.Compare(%s); c != 0 {\n", indent, lhs, rhs)
	e.Writef("%s\treturn c\n", indent)
	e.Writef("%s}\n", indent)
}
