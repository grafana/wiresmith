package types

import (
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// BytesType implements Type for proto bytes fields.
// Length-delimited, non-packable. Similar to StringType but uses []byte
// copy semantics and ConsumeBytes instead of ConsumeString.
type BytesType struct{}

func (BytesType) WireType() string          { return "protowire.BytesType" }
func (BytesType) IsPackable() bool          { return false }
func (BytesType) IsFixed32() bool           { return false }
func (BytesType) IsFixed64() bool           { return false }
func (BytesType) FixedSize() int            { return 0 }
func (BytesType) SizeByIndex() bool         { return false }
func (BytesType) RequiredImports() []string { return nil }

// OptionalAccess returns access unchanged because []byte is already nullable.
func (BytesType) OptionalAccess(access string) string {
	return access
}

func (BytesType) VarintSizeExpr(access string) string {
	panicNotPackable("VarintSizeExpr")
	return ""
}

// --- Size ---

func (BytesType) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tif len(%s) > 0 {\n\t\tn += %d + protowire.SizeVarint(uint64(len(%s))) + len(%s)\n\t}\n", access, tagSize, access, access)
}

func (BytesType) EmitValueSize(e Emitter, indent, access string, tagSize int, target string) {
	e.Writef("%s%s += %d + protowire.SizeVarint(uint64(len(%s))) + len(%s)\n", indent, target, tagSize, access, access)
}

// --- Marshal ---

func (BytesType) EmitMarshal(e Emitter, access string, num protowire.Number) {
	e.Writef("\tif len(%s) > 0 {\n", access)
	e.Writef("\t\ti -= len(%s)\n\t\tcopy(dAtA[i:], %s)\n", access, access)
	e.Writef("\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(len(%s)))\n", access)
	e.ReverseTag("\t\t", num, protowire.BytesType)
	e.Writef("\t}\n")
}

func (BytesType) EmitEncode(e Emitter, indent, access string) {
	e.Writef("%si -= len(%s)\n%scopy(dAtA[i:], %s)\n", indent, access, indent, access)
	e.Writef("%si = protohelpers.EncodeVarint(dAtA, i, uint64(len(%s)))\n", indent, access)
}

func (b BytesType) EmitValueMarshal(e Emitter, indent, access string, num protowire.Number) {
	b.EmitEncode(e, indent, access)
	e.ReverseTag(indent, num, protowire.BytesType)
}

// --- Unmarshal ---

func (BytesType) EmitConsume(e Emitter) { emitConsumeBytesLen(e) }

func (BytesType) CastExpr(varName string, ctx FieldContext) string {
	return "append([]byte(nil), " + varName + "...)"
}

func (BytesType) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeBytesLen(e)
	e.Writef("\t\t\t%s = append(%s[:0], dAtA[iNdEx:postIndex]...)\n", access, access)
	e.Writef("\t\t\tiNdEx = postIndex\n")
}

func (BytesType) EmitMapEntryUnmarshal(e Emitter, varName, indent string, ctx FieldContext) {
	emitConsumeBytesLenAt(e, indent)
	e.Writef("%s%s = append([]byte(nil), dAtA[iNdEx:postIndex]...)\n", indent, varName)
	e.Writef("%siNdEx = postIndex\n", indent)
}

func (BytesType) ZeroLiteral() string { return "nil" }

// EmitEqual registers the bytes import lazily so callers don't need a
// separate pre-scan to decide whether the generated Equal body uses
// bytes.Equal.
func (BytesType) EmitEqual(e Emitter, indent, lhs, rhs string) {
	e.AddImport("bytes", "")
	e.Writef("%sif !bytes.Equal(%s, %s) {\n%s\treturn false\n%s}\n", indent, lhs, rhs, indent, indent)
}

func init() {
	register(protoreflect.BytesKind, &BytesType{})
}
