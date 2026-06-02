package types

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// fixed64Base provides shared implementation for 8-byte fixed-width types
// (Fixed64, Sfixed64, Double).
type fixed64Base struct {
	// putExpr: format for the PutUint64 argument. One %s for access.
	// Fixed64: "%s", Sfixed64: "uint64(%s)", Double: "math.Float64bits(%s)"
	putExpr string
	// getExpr: format to convert decoded uint64 to target Go type.
	// Fixed64: "%s", Sfixed64: "int64(%s)", Double: "math.Float64frombits(%s)"
	getExpr string
	// nonzeroExpr: format for the "skip-on-default" predicate. One %s for access.
	// Defaults to "%s != 0". Double overrides to "math.Float64bits(%s) != 0" so
	// that -0.0 (which compares equal to +0.0 in Go) survives marshal, matching
	// google.golang.org/protobuf (vtproto and gogoproto silently strip -0.0).
	nonzeroExpr string
	// equalCastExpr: format applied to each side of the Equal `!=` guard. One
	// %s for access. Defaults to "%s" (no cast). Double overrides to
	// "math.Float64bits(%s)" so NaN payloads compare equal to themselves (Go's
	// `==` says `NaN != NaN`) and -0.0/+0.0 compare unequal — matching both
	// google.golang.org/protobuf proto.Equal and the marshal path's bit-exact
	// preservation contract.
	equalCastExpr string
	imports       []string
}

func (f fixed64Base) nonzero(access string) string {
	expr := f.nonzeroExpr
	if expr == "" {
		expr = "%s != 0"
	}
	return fmt.Sprintf(expr, access)
}

func (fixed64Base) WireType() string  { return "protowire.Fixed64Type" }
func (fixed64Base) IsPackable() bool  { return true }
func (fixed64Base) IsFixed32() bool   { return false }
func (fixed64Base) IsFixed64() bool   { return true }
func (fixed64Base) FixedSize() int    { return 8 }
func (fixed64Base) SizeByIndex() bool { return false }
func (f fixed64Base) RequiredImports() []string {
	return f.imports
}

func (fixed64Base) OptionalAccess(access string) string {
	return "*" + access
}

func (fixed64Base) VarintSizeExpr(access string) string {
	panic("VarintSizeExpr called on fixed64 type")
}

// --- Size ---

func (f fixed64Base) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tif %s {\n\t\tn += %d\n\t}\n", f.nonzero(access), tagSize+8)
}

func (fixed64Base) EmitValueSize(e Emitter, indent, access string, tagSize int, target string) {
	e.Writef("%s%s += %d\n", indent, target, tagSize+8)
}

// --- Marshal ---

func (f fixed64Base) EmitMarshal(e Emitter, access string, num protowire.Number) {
	if f.nonzeroExpr != "" {
		// Cache the bits in a local so we only compute math.Float64bits once
		// (used by both the predicate and PutUint64). Without this, the Go
		// compiler does not common-subexpression-eliminate the second call.
		e.Writef("\tif v := %s; v != 0 {\n", f.put(access))
		e.Writef("\t\ti -= 8\n\t\tbinary.LittleEndian.PutUint64(dAtA[i:], v)\n")
	} else {
		e.Writef("\tif %s != 0 {\n", access)
		e.Writef("\t\ti -= 8\n\t\tbinary.LittleEndian.PutUint64(dAtA[i:], %s)\n", f.put(access))
	}
	e.ReverseTag("\t\t", num, protowire.Fixed64Type)
	e.Writef("\t}\n")
}

func (f fixed64Base) EmitEncode(e Emitter, indent, access string) {
	e.Writef("%si -= 8\n%sbinary.LittleEndian.PutUint64(dAtA[i:], %s)\n", indent, indent, f.put(access))
}

func (f fixed64Base) EmitValueMarshal(e Emitter, indent, access string, num protowire.Number) {
	f.EmitEncode(e, indent, access)
	e.ReverseTag(indent, num, protowire.Fixed64Type)
}

// --- Unmarshal ---

func (fixed64Base) EmitConsume(e Emitter) { emitConsumeFixed64(e) }

func (f fixed64Base) CastExpr(varName string, ctx FieldContext) string {
	return f.get(varName)
}

func (f fixed64Base) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeFixed64(e)
	e.Writef("\t\t\t%s = %s\n", access, f.get("v"))
}

func (f fixed64Base) EmitMapEntryUnmarshal(e Emitter, varName, indent string, ctx FieldContext) {
	emitConsumeFixed64At(e, indent)
	e.Writef("%s%s = %s\n", indent, varName, f.get("v"))
}

func (f fixed64Base) put(access string) string {
	return fmt.Sprintf(f.putExpr, access)
}

func (f fixed64Base) get(varName string) string {
	return fmt.Sprintf(f.getExpr, varName)
}

func (fixed64Base) ZeroLiteral() string { return "0" }

func (f fixed64Base) equalCast(access string) string {
	if f.equalCastExpr == "" {
		return access
	}
	return fmt.Sprintf(f.equalCastExpr, access)
}

func (f fixed64Base) EmitEqual(e Emitter, indent, lhs, rhs string) {
	if f.equalCastExpr != "" {
		// The only registered fixed64Base with an equalCastExpr is Double,
		// whose cast names math.Float64bits. Register the import here so the
		// companion _equal.pb.go file (which doesn't share imports with the
		// main .pb.go) compiles.
		e.AddImport("math", "")
	}
	scalarNotEqualGuard(e, indent, f.equalCast(lhs), f.equalCast(rhs))
}

// EmitCompare emits a bit-exact ordered comparison via equalCast. For Double
// that routes both sides through math.Float64bits so NaN payloads order
// stably; fixed/sfixed keep natural unsigned/signed `<` semantics. "math"
// is added lazily here (parallel to fixed32Base.EmitCompare) so the
// _compare.pb.go file doesn't pick up Marshal-only imports it never uses.
func (f fixed64Base) EmitCompare(e Emitter, indent, lhs, rhs string) {
	if f.equalCastExpr != "" {
		e.AddImport("math", "")
	}
	orderedScalarCompareGuard(e, indent, f.equalCast(lhs), f.equalCast(rhs))
}

// Fixed64Type is the Type for protoreflect.Fixed64Kind.
var Fixed64Type = &fixed64Base{
	putExpr: "%s",
	getExpr: "%s",
	imports: []string{"encoding/binary"},
}

func init() {
	register(protoreflect.Fixed64Kind, Fixed64Type)
}
