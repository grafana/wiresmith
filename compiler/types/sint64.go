package types

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Sint64Type implements Type for proto sint64 fields (zigzag-encoded varint).
type Sint64Type struct{}

func (Sint64Type) WireType() string          { return "protowire.VarintType" }
func (Sint64Type) IsPackable() bool          { return true }
func (Sint64Type) IsFixed32() bool           { return false }
func (Sint64Type) IsFixed64() bool           { return false }
func (Sint64Type) FixedSize() int            { return 0 }
func (Sint64Type) SizeByIndex() bool         { return false }
func (Sint64Type) RequiredImports() []string { return nil }
func (Sint64Type) OptionalAccess(access string) string {
	return "*" + access
}

func (Sint64Type) VarintSizeExpr(access string) string {
	return fmt.Sprintf("protowire.SizeVarint(protowire.EncodeZigZag(%s))", access)
}

// --- Size ---

func (Sint64Type) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tif %s != 0 {\n\t\tn += %d + protowire.SizeVarint(protowire.EncodeZigZag(%s))\n\t}\n", access, tagSize, access)
}

func (Sint64Type) EmitValueSize(e Emitter, indent, access string, tagSize int, target string) {
	e.Writef("%s%s += %d + protowire.SizeVarint(protowire.EncodeZigZag(%s))\n", indent, target, tagSize, access)
}

// --- Marshal ---

func (Sint64Type) EmitMarshal(e Emitter, access string, num protowire.Number) {
	e.Writef("\tif %s != 0 {\n", access)
	e.Writef("\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(uint64(%s<<1)^uint64(int64(%s)>>63)))\n", access, access)
	e.ReverseTag("\t\t", num, protowire.VarintType)
	e.Writef("\t}\n")
}

func (Sint64Type) EmitEncode(e Emitter, indent, access string) {
	if strings.HasPrefix(access, "*") {
		e.Writef("%sv := %s\n", indent, access)
		access = "v"
	}
	e.Writef("%si = protohelpers.EncodeVarint(dAtA, i, uint64(uint64(%s<<1)^uint64(int64(%s)>>63)))\n", indent, access, access)
}

func (s Sint64Type) EmitValueMarshal(e Emitter, indent, access string, num protowire.Number) {
	s.EmitEncode(e, indent, access)
	e.ReverseTag(indent, num, protowire.VarintType)
}

// --- Unmarshal ---

func (Sint64Type) EmitConsume(e Emitter) { emitConsumeVarint(e) }

func (Sint64Type) CastExpr(varName string, ctx FieldContext) string {
	return fmt.Sprintf("int64(%s>>1) ^ int64(%s)<<63>>63", varName, varName)
}

func (Sint64Type) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeVarint(e)
	e.Writef("\t\t\t%s = int64(v>>1) ^ int64(v)<<63>>63\n", access)
}

func (Sint64Type) EmitMapEntryUnmarshal(e Emitter, varName, indent string, ctx FieldContext) {
	emitConsumeVarintAt(e, indent)
	e.Writef("%s%s = int64(v>>1) ^ int64(v)<<63>>63\n", indent, varName)
}

func (Sint64Type) ZeroLiteral() string { return "0" }

func (Sint64Type) EmitEqual(e Emitter, indent, lhs, rhs string) {
	scalarNotEqualGuard(e, indent, lhs, rhs)
}

func (Sint64Type) EmitCompare(e Emitter, indent, lhs, rhs string) {
	orderedScalarCompareGuard(e, indent, lhs, rhs)
}

func init() {
	register(protoreflect.Sint64Kind, &Sint64Type{})
}
