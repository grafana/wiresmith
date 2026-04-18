package types

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Sint32Type implements Type for proto sint32 fields (zigzag-encoded varint).
type Sint32Type struct{}

func (Sint32Type) WireType() string          { return "protowire.VarintType" }
func (Sint32Type) IsPackable() bool          { return true }
func (Sint32Type) IsFixed32() bool           { return false }
func (Sint32Type) IsFixed64() bool           { return false }
func (Sint32Type) FixedSize() int            { return 0 }
func (Sint32Type) SizeByIndex() bool         { return false }
func (Sint32Type) RequiredImports() []string { return nil }
func (Sint32Type) OptionalAccess(access string) string {
	return "*" + access
}

func (Sint32Type) VarintSizeExpr(access string) string {
	return fmt.Sprintf("protowire.SizeVarint(protowire.EncodeZigZag(int64(%s)))", access)
}

// --- Size ---

func (Sint32Type) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tif %s != 0 {\n\t\tn += %d + protowire.SizeVarint(protowire.EncodeZigZag(int64(%s)))\n\t}\n", access, tagSize, access)
}

func (Sint32Type) EmitValueSize(e Emitter, indent, access string, tagSize int, target string) {
	e.Writef("%s%s += %d + protowire.SizeVarint(protowire.EncodeZigZag(int64(%s)))\n", indent, target, tagSize, access)
}

// --- Marshal ---

func (Sint32Type) EmitMarshal(e Emitter, access string, num protowire.Number) {
	e.Writef("\tif %s != 0 {\n", access)
	e.Writef("\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(uint32(%s<<1)^uint32(int32(%s)>>31)))\n", access, access)
	e.ReverseTag("\t\t", num, protowire.VarintType)
	e.Writef("\t}\n")
}

func (Sint32Type) EmitEncode(e Emitter, indent, access string) {
	if strings.HasPrefix(access, "*") {
		e.Writef("%sv := %s\n", indent, access)
		access = "v"
	}
	e.Writef("%si = protohelpers.EncodeVarint(dAtA, i, uint64(uint32(%s<<1)^uint32(int32(%s)>>31)))\n", indent, access, access)
}

func (s Sint32Type) EmitValueMarshal(e Emitter, indent, access string, num protowire.Number) {
	s.EmitEncode(e, indent, access)
	e.ReverseTag(indent, num, protowire.VarintType)
}

// --- Unmarshal ---

func (Sint32Type) EmitConsume(e Emitter) { emitConsumeVarint(e) }

func (Sint32Type) CastExpr(varName string, ctx FieldContext) string {
	return fmt.Sprintf("int32(protowire.DecodeZigZag(%s))", varName)
}

func (Sint32Type) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeVarint(e)
	e.Writef("\t\t\t%s = int32(protowire.DecodeZigZag(v))\n", access)
	emitAdvanceBytes(e)
}

func (Sint32Type) EmitMapEntryUnmarshal(e Emitter, varName, indent string, ctx FieldContext) {
	e.Writef("%stmpVal, tmpN := protowire.ConsumeVarint(entryData)\n", indent)
	e.Writef("%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid varint\")\n%s}\n", indent, indent, indent)
	e.Writef("%s%s = int32(protowire.DecodeZigZag(tmpVal))\n", indent, varName)
	e.Writef("%sentryData = entryData[tmpN:]\n", indent)
}

func init() {
	register(protoreflect.Sint32Kind, &Sint32Type{})
}
