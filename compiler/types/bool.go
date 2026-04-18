package types

import (
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// BoolType implements Type for proto bool fields.
// Bool has unique encoding: always 1 byte, with bool-to-byte conversion.
type BoolType struct{}

func (BoolType) WireType() string          { return "protowire.VarintType" }
func (BoolType) IsPackable() bool          { return true }
func (BoolType) IsFixed32() bool           { return false }
func (BoolType) IsFixed64() bool           { return false }
func (BoolType) FixedSize() int            { return 1 }
func (BoolType) SizeByIndex() bool         { return false }
func (BoolType) RequiredImports() []string { return nil }
func (BoolType) OptionalAccess(access string) string {
	return "*" + access
}

func (BoolType) VarintSizeExpr(access string) string {
	return "1"
}

// --- Size ---

func (BoolType) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tif %s {\n\t\tn += %d\n\t}\n", access, tagSize+1)
}

func (BoolType) EmitValueSize(e Emitter, indent, access string, tagSize int, target string) {
	e.Writef("%s%s += %d\n", indent, target, tagSize+1)
}

// --- Marshal ---

func (BoolType) EmitMarshal(e Emitter, access string, num protowire.Number) {
	e.Writef("\tif %s {\n", access)
	e.Writef("\t\ti--\n\t\tif %s {\n\t\t\tdAtA[i] = 1\n\t\t} else {\n\t\t\tdAtA[i] = 0\n\t\t}\n", access)
	e.ReverseTag("\t\t", num, protowire.VarintType)
	e.Writef("\t}\n")
}

func (BoolType) EmitEncode(e Emitter, indent, access string) {
	e.Writef("%si--\n%sif %s {\n%s\tdAtA[i] = 1\n%s} else {\n%s\tdAtA[i] = 0\n%s}\n", indent, indent, access, indent, indent, indent, indent)
}

func (b BoolType) EmitValueMarshal(e Emitter, indent, access string, num protowire.Number) {
	b.EmitEncode(e, indent, access)
	e.ReverseTag(indent, num, protowire.VarintType)
}

// --- Unmarshal ---

func (BoolType) EmitConsume(e Emitter)                            { emitConsumeVarint(e) }
func (BoolType) CastExpr(varName string, ctx FieldContext) string { return varName + " != 0" }

func (BoolType) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeVarint(e)
	e.Writef("\t\t\t%s = v != 0\n", access)
	emitAdvanceBytes(e)
}

func (BoolType) EmitMapEntryUnmarshal(e Emitter, varName, indent string, ctx FieldContext) {
	e.Writef("%stmpVal, tmpN := protowire.ConsumeVarint(entryData)\n", indent)
	e.Writef("%sif tmpN < 0 {\n%s\treturn fmt.Errorf(\"invalid varint\")\n%s}\n", indent, indent, indent)
	e.Writef("%s%s = tmpVal != 0\n", indent, varName)
	e.Writef("%sentryData = entryData[tmpN:]\n", indent)
}

func init() {
	register(protoreflect.BoolKind, &BoolType{})
}
