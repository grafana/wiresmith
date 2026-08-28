package types

import (
	"google.golang.org/protobuf/encoding/protowire"
)

// StdDurationType is the FieldType for a singular `google.protobuf.Duration`
// field annotated with `(wiresmith.options.stdduration) = true`. The Go-side
// field is a stdlib `time.Duration`; the wire format remains the standard
// Duration sub-message (int64 seconds field 1, int32 nanos field 2).
//
// Presence semantics: `time.Duration(0)` is treated as "not set" — Size
// returns 0, Marshal skips the field entirely, and Unmarshal leaves the
// value at zero when no Duration tag is seen. Unlike stdtime, there is no
// "explicit zero vs unset" distinction: `time.Duration` has only one zero
// value, so a wire payload encoding seconds=0 nanos=0 round-trips as the
// same zero a never-set field would produce.
//
// Decoded values saturate on overflow at math.MaxInt64 / math.MinInt64
// nanoseconds — `time.Duration` is int64 nanoseconds (~292 years max),
// while proto Duration permits up to ~10000 years. See protohelpers
// `DecodeStdDuration`.
//
// The Size/Marshal/Unmarshal call sites are slim and route into the
// SizeStdDuration / EncodeStdDuration / DecodeStdDuration helpers in
// protohelpers, same shape as StdtimeType.
type StdDurationType struct{}

// RequiredImports declares "time" (for time.Duration access) at every
// stdduration call site. protohelpers is imported by the surrounding
// marshal/unmarshal emit path.
func (StdDurationType) RequiredImports() []string {
	return []string{"time"}
}

// EmitSize emits the proto-wrapper size accumulator for a singular
// stdduration field. The inner Duration payload size is computed by
// protohelpers.SizeStdDuration; the `!= 0` gate makes `time.Duration(0)`
// round-trip as "field absent", matching proto3 default-suppression.
func (StdDurationType) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tif %s != 0 {\n", access)
	e.Writef("\t\tinner := protohelpers.SizeStdDuration(%s)\n", access)
	e.Writef("\t\tn += %d + protowire.SizeVarint(uint64(inner)) + inner\n", tagSize)
	e.Writef("\t}\n")
}

// EmitMarshal writes the Duration envelope and its two inner fields via
// protohelpers.EncodeStdDuration, then the outer length prefix and the
// outer field tag. Skips the entire envelope when the Go value is zero.
//
// `start := i` snapshots the reverse-write cursor so the post-encode size
// can be recovered as `start - i` without recomputing SizeStdDuration.
func (StdDurationType) EmitMarshal(e Emitter, access string, num protowire.Number) {
	e.Writef("\tif %s != 0 {\n", access)
	e.Writef("\t\tstart := i\n")
	e.Writef("\t\ti = protohelpers.EncodeStdDuration(dAtA, i, %s)\n", access)
	e.Writef("\t\tinner := start - i\n")
	e.Writef("\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(inner))\n")
	e.ReverseTag("\t\t", num, protowire.BytesType)
	e.Writef("\t}\n")
}

// EmitUnmarshal consumes the outer length-delimited header, slices out the
// payload, and hands it to protohelpers.DecodeStdDuration. The helper is
// responsible for bounding inner-tag reads to the slice and saturating on
// overflow; the call site keeps the outer iNdEx/dAtA/l invariants intact.
func (StdDurationType) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeBytesLen(e)
	e.Writef("\t\t\tstddurationVal, err := protohelpers.DecodeStdDuration(dAtA[iNdEx:postIndex])\n")
	e.Writef("\t\t\tif err != nil {\n\t\t\t\treturn err\n\t\t\t}\n")
	e.Writef("\t\t\t%s = stddurationVal\n", access)
	e.Writef("\t\t\tiNdEx = postIndex\n")
}

// EmitEqual compares two time.Duration values via plain `==`. time.Duration
// is int64 nanoseconds, so identity-by-value matches the canonical "same
// elapsed time" semantics — there's no zone or normalisation to worry about
// like time.Time has.
func (StdDurationType) EmitEqual(e Emitter, indent, lhs, rhs string) {
	e.Writef("%sif %s != %s {\n%s\treturn false\n%s}\n", indent, lhs, rhs, indent, indent)
}

// EmitCompare returns -1/0/+1 from a numeric compare on the int64-backed
// time.Duration. Stays consistent with bytes.Compare / time.Time.Compare's
// contract so the generated Compare method's overall ordering is uniform
// across field kinds.
func (StdDurationType) EmitCompare(e Emitter, indent, lhs, rhs string) {
	e.Writef("%sif %s < %s {\n%s\treturn -1\n%s} else if %s > %s {\n%s\treturn 1\n%s}\n",
		indent, lhs, rhs, indent, indent, lhs, rhs, indent, indent)
}

// OptionalStdDurationType is the FieldType for a proto3 `optional
// google.protobuf.Duration` field annotated with
// `(wiresmith.options.stdduration) = true`. The Go-side field is
// `*time.Duration`; presence is real proto3 optional semantics (nil pointer
// means "not set"), unlike the non-optional StdDurationType where
// `time.Duration(0)` doubles as the presence sentinel. This lets a caller
// distinguish "duration explicitly set to zero" from "unset" — the one
// distinction plain StdDurationType cannot make.
//
// Wire format is unchanged from StdDurationType: the standard Duration
// sub-message envelope, same overflow-saturation behavior on decode.
type OptionalStdDurationType struct{}

// RequiredImports declares "time" for *time.Duration access.
func (OptionalStdDurationType) RequiredImports() []string {
	return []string{"time"}
}

// EmitSize emits a nil-guarded size accumulator — presence is the pointer,
// so a non-nil zero duration is sized and emitted as an explicit envelope.
func (OptionalStdDurationType) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tif %s != nil {\n", access)
	e.Writef("\t\tinner := protohelpers.SizeStdDuration(*%s)\n", access)
	e.Writef("\t\tn += %d + protowire.SizeVarint(uint64(inner)) + inner\n", tagSize)
	e.Writef("\t}\n")
}

// EmitMarshal mirrors EmitSize: nil-guard then dereferenced encode.
func (OptionalStdDurationType) EmitMarshal(e Emitter, access string, num protowire.Number) {
	e.Writef("\tif %s != nil {\n", access)
	e.Writef("\t\tstart := i\n")
	e.Writef("\t\ti = protohelpers.EncodeStdDuration(dAtA, i, *%s)\n", access)
	e.Writef("\t\tinner := start - i\n")
	e.Writef("\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(inner))\n")
	e.ReverseTag("\t\t", num, protowire.BytesType)
	e.Writef("\t}\n")
}

// EmitUnmarshal decodes the Duration envelope into a freshly allocated
// *time.Duration, so a decoded field is always non-nil.
func (OptionalStdDurationType) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeBytesLen(e)
	e.Writef("\t\t\tstddurationVal, err := protohelpers.DecodeStdDuration(dAtA[iNdEx:postIndex])\n")
	e.Writef("\t\t\tif err != nil {\n\t\t\t\treturn err\n\t\t\t}\n")
	e.Writef("\t\t\t%s = &stddurationVal\n", access)
	e.Writef("\t\t\tiNdEx = postIndex\n")
}

// EmitEqual: nil-pair mismatch is a difference, otherwise dereference and
// compare by value.
func (OptionalStdDurationType) EmitEqual(e Emitter, indent, lhs, rhs string) {
	e.Writef("%sif (%s == nil) != (%s == nil) {\n%s\treturn false\n%s}\n", indent, lhs, rhs, indent, indent)
	e.Writef("%sif %s != nil && *%s != *%s {\n%s\treturn false\n%s}\n", indent, lhs, lhs, rhs, indent, indent)
}

// EmitCompare: nil < non-nil, then numeric compare on the dereferenced
// int64-backed durations.
func (OptionalStdDurationType) EmitCompare(e Emitter, indent, lhs, rhs string) {
	emitNilPairOrdering(e, indent, lhs, rhs)
	e.Writef("%sif %s != nil {\n", indent, lhs)
	e.Writef("%s\tif *%s < *%s {\n%s\t\treturn -1\n%s\t} else if *%s > *%s {\n%s\t\treturn 1\n%s\t}\n",
		indent, lhs, rhs, indent, indent, lhs, rhs, indent, indent)
	e.Writef("%s}\n", indent)
}
