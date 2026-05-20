package types

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// fixed32Base provides shared implementation for 4-byte fixed-width types
// (Fixed32, Sfixed32, Float).
type fixed32Base struct {
	// putExpr: format for the PutUint32 argument. One %s for access.
	// Fixed32: "%s", Sfixed32: "uint32(%s)", Float: "math.Float32bits(%s)"
	putExpr string
	// getExpr: format to convert decoded uint32 to target Go type.
	// Fixed32: "%s", Sfixed32: "int32(%s)", Float: "math.Float32frombits(%s)"
	getExpr string
	imports []string
}

func (fixed32Base) WireType() string  { return "protowire.Fixed32Type" }
func (fixed32Base) IsPackable() bool  { return true }
func (fixed32Base) IsFixed32() bool   { return true }
func (fixed32Base) IsFixed64() bool   { return false }
func (fixed32Base) FixedSize() int    { return 4 }
func (fixed32Base) SizeByIndex() bool { return false }
func (f fixed32Base) RequiredImports() []string {
	return f.imports
}

func (fixed32Base) OptionalAccess(access string) string {
	return "*" + access
}

func (fixed32Base) VarintSizeExpr(access string) string {
	panic("VarintSizeExpr called on fixed32 type")
}

// --- Size ---

func (fixed32Base) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tif %s != 0 {\n\t\tn += %d\n\t}\n", access, tagSize+4)
}

func (fixed32Base) EmitValueSize(e Emitter, indent, access string, tagSize int, target string) {
	e.Writef("%s%s += %d\n", indent, target, tagSize+4)
}

// --- Marshal ---

func (f fixed32Base) EmitMarshal(e Emitter, access string, num protowire.Number) {
	e.Writef("\tif %s != 0 {\n", access)
	e.Writef("\t\ti -= 4\n\t\tbinary.LittleEndian.PutUint32(dAtA[i:], %s)\n", f.put(access))
	e.ReverseTag("\t\t", num, protowire.Fixed32Type)
	e.Writef("\t}\n")
}

func (f fixed32Base) EmitEncode(e Emitter, indent, access string) {
	e.Writef("%si -= 4\n%sbinary.LittleEndian.PutUint32(dAtA[i:], %s)\n", indent, indent, f.put(access))
}

func (f fixed32Base) EmitValueMarshal(e Emitter, indent, access string, num protowire.Number) {
	f.EmitEncode(e, indent, access)
	e.ReverseTag(indent, num, protowire.Fixed32Type)
}

// --- Unmarshal ---

func (fixed32Base) EmitConsume(e Emitter) { emitConsumeFixed32(e) }

func (f fixed32Base) CastExpr(varName string, ctx FieldContext) string {
	return f.get(varName)
}

func (f fixed32Base) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeFixed32(e)
	e.Writef("\t\t\t%s = %s\n", access, f.get("v"))
}

func (f fixed32Base) EmitMapEntryUnmarshal(e Emitter, varName, indent string, ctx FieldContext) {
	emitConsumeFixed32At(e, indent)
	e.Writef("%s%s = %s\n", indent, varName, f.get("v"))
}

func (f fixed32Base) put(access string) string {
	return fmt.Sprintf(f.putExpr, access)
}

func (f fixed32Base) get(varName string) string {
	return fmt.Sprintf(f.getExpr, varName)
}

func (fixed32Base) ZeroLiteral() string { return "0" }

func (fixed32Base) EmitEqual(e Emitter, indent, lhs, rhs string) {
	scalarNotEqualGuard(e, indent, lhs, rhs)
}

// Fixed32Type is the Type for protoreflect.Fixed32Kind.
var Fixed32Type = &fixed32Base{
	putExpr: "%s",
	getExpr: "%s",
	imports: []string{"encoding/binary"},
}

func init() {
	register(protoreflect.Fixed32Kind, Fixed32Type)
}
