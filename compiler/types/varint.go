package types

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

// varintBase provides shared implementation for standard varint-encoded types
// (Int32, Uint32, Int64, Uint64). Enum extends this with custom unmarshal.
// Sint32/Sint64 are standalone due to zigzag encoding differences.
type varintBase struct {
	// unmarshalCast: format to cast decoded varint to target Go type.
	// "int32(%s)" for Int32, "uint32(%s)" for Uint32, "int64(%s)" for Int64, "%s" for Uint64.
	unmarshalCast string
}

func (varintBase) WireType() string          { return "protowire.VarintType" }
func (varintBase) IsPackable() bool          { return true }
func (varintBase) IsFixed32() bool           { return false }
func (varintBase) IsFixed64() bool           { return false }
func (varintBase) FixedSize() int            { return 0 }
func (varintBase) SizeByIndex() bool         { return false }
func (varintBase) RequiredImports() []string { return nil }
func (varintBase) OptionalAccess(access string) string {
	return "*" + access
}

func (varintBase) VarintSizeExpr(access string) string {
	return fmt.Sprintf("protowire.SizeVarint(uint64(%s))", access)
}

// --- Size ---

func (varintBase) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tif %s != 0 {\n\t\tn += %d + protowire.SizeVarint(uint64(%s))\n\t}\n", access, tagSize, access)
}

func (varintBase) EmitValueSize(e Emitter, indent, access string, tagSize int, target string) {
	e.Writef("%s%s += %d + protowire.SizeVarint(uint64(%s))\n", indent, target, tagSize, access)
}

// --- Marshal ---

func (varintBase) EmitMarshal(e Emitter, access string, num protowire.Number) {
	e.Writef("\tif %s != 0 {\n", access)
	e.Writef("\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(%s))\n", access)
	e.ReverseTag("\t\t", num, protowire.VarintType)
	e.Writef("\t}\n")
}

func (varintBase) EmitEncode(e Emitter, indent, access string) {
	e.Writef("%si = protohelpers.EncodeVarint(dAtA, i, uint64(%s))\n", indent, access)
}

func (v varintBase) EmitValueMarshal(e Emitter, indent, access string, num protowire.Number) {
	v.EmitEncode(e, indent, access)
	e.ReverseTag(indent, num, protowire.VarintType)
}

// --- Unmarshal ---

func (varintBase) EmitConsume(e Emitter) { emitConsumeVarint(e) }

func (v varintBase) CastExpr(varName string, ctx FieldContext) string {
	return v.cast(varName)
}

func (v varintBase) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeVarint(e)
	e.Writef("\t\t\t%s = %s\n", access, v.cast("v"))
}

func (v varintBase) EmitMapEntryUnmarshal(e Emitter, varName, indent string, ctx FieldContext) {
	emitConsumeVarintAt(e, indent)
	e.Writef("%s%s = %s\n", indent, varName, v.cast("v"))
}

func (v varintBase) cast(varName string) string {
	return fmt.Sprintf(v.unmarshalCast, varName)
}

func (varintBase) ZeroLiteral() string { return "0" }

func (varintBase) EmitEqual(e Emitter, indent, lhs, rhs string) {
	scalarNotEqualGuard(e, indent, lhs, rhs)
}
