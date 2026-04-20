package types

import (
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// StringType implements Type for proto string fields.
// Length-delimited, non-packable.
type StringType struct{}

func (StringType) WireType() string          { return "protowire.BytesType" }
func (StringType) IsPackable() bool          { return false }
func (StringType) IsFixed32() bool           { return false }
func (StringType) IsFixed64() bool           { return false }
func (StringType) FixedSize() int            { return 0 }
func (StringType) SizeByIndex() bool         { return false }
func (StringType) RequiredImports() []string { return nil }
func (StringType) OptionalAccess(access string) string {
	return "*" + access
}

func (StringType) VarintSizeExpr(access string) string {
	panicNotPackable("VarintSizeExpr")
	return ""
}

// --- Size ---

func (StringType) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tif len(%s) > 0 {\n\t\tn += %d + protowire.SizeVarint(uint64(len(%s))) + len(%s)\n\t}\n", access, tagSize, access, access)
}

func (StringType) EmitValueSize(e Emitter, indent, access string, tagSize int, target string) {
	e.Writef("%s%s += %d + protowire.SizeVarint(uint64(len(%s))) + len(%s)\n", indent, target, tagSize, access, access)
}

// --- Marshal ---

func (StringType) EmitMarshal(e Emitter, access string, num protowire.Number) {
	e.Writef("\tif len(%s) > 0 {\n", access)
	e.Writef("\t\ti -= len(%s)\n\t\tcopy(dAtA[i:], %s)\n", access, access)
	e.Writef("\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(len(%s)))\n", access)
	e.ReverseTag("\t\t", num, protowire.BytesType)
	e.Writef("\t}\n")
}

func (StringType) EmitEncode(e Emitter, indent, access string) {
	e.Writef("%si -= len(%s)\n%scopy(dAtA[i:], %s)\n", indent, access, indent, access)
	e.Writef("%si = protohelpers.EncodeVarint(dAtA, i, uint64(len(%s)))\n", indent, access)
}

func (s StringType) EmitValueMarshal(e Emitter, indent, access string, num protowire.Number) {
	s.EmitEncode(e, indent, access)
	e.ReverseTag(indent, num, protowire.BytesType)
}

// --- Unmarshal ---

func (StringType) EmitConsume(e Emitter) { emitConsumeBytesLen(e) }
func (StringType) CastExpr(varName string, ctx FieldContext) string {
	return "string(" + varName + ")"
}

func (StringType) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeBytesLen(e)
	e.Writef("\t\t\t%s = string(dAtA[iNdEx:postIndex])\n", access)
	e.Writef("\t\t\tiNdEx = postIndex\n")
}

func (StringType) EmitMapEntryUnmarshal(e Emitter, varName, indent string, ctx FieldContext) {
	e.Writef("%stmpVal, tmpN := protowire.ConsumeString(entryData)\n", indent)
	e.Writef("%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid string\")\n%s}\n", indent, indent, indent)
	e.Writef("%s%s = tmpVal\n", indent, varName)
	e.Writef("%sentryData = entryData[tmpN:]\n", indent)
}

func init() {
	register(protoreflect.StringKind, &StringType{})
}
