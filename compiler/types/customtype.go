package types

import (
	"google.golang.org/protobuf/encoding/protowire"
)

// CustomType is the FieldType for a field annotated with
// `(wiresmith.options.customtype)`. The user-supplied Go type owns its own
// wire encoding: SizeWiresmith reports the encoded length, MarshalWiresmith
// writes bytes forward into a slice the generator pre-sizes, and
// UnmarshalWiresmith reads them back. EqualWiresmith is consulted from the
// generated Equal method.
//
// v1 scope: only constructed for singular bytes/string fields — validation in
// compiler/generator/option_customtype.go enforces that. The wire format the
// custom type writes is whatever it likes; the proto wrapper is fixed
// length-delimited (tag + varint length + payload).
type CustomType struct {
	// GoType is the field type as it appears in generated code, including
	// any package alias resolved by the ImportTracker (e.g. "labels.LabelPairs"
	// or "MyLocalType" for same-package). Populated by the FileGenerator
	// during fieldType() dispatch — keeping the resolved form here means
	// emit_size / emit_marshal don't have to thread it through the call.
	GoType string
}

// RequiredImports declares "fmt" because EmitMarshal asserts the user's
// MarshalWiresmith returned exactly SizeWiresmith() bytes via fmt.Errorf.
// Other generated paths (Stringer, unmarshal error wrapping) usually pull
// fmt in already, but declaring it here keeps the import sound for any
// future file where customtype is the only fmt user.
func (c *CustomType) RequiredImports() []string { return []string{"fmt"} }

// EmitSize emits the proto-wrapper size accumulator. The customtype's
// SizeWiresmith() is called once and reused for the conditional and the size
// add — the convention with bytes/string is "skip when zero", and the
// customtype mirrors that so the generated layout stays consistent across
// the two field kinds the option targets.
func (c *CustomType) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tif s := %s.SizeWiresmith(); s > 0 {\n", access)
	e.Writef("\t\tn += %d + protowire.SizeVarint(uint64(s)) + s\n", tagSize)
	e.Writef("\t}\n")
}

// EmitMarshal reserves SizeWiresmith() bytes in the reverse-write buffer,
// hands that exact slice to the user's MarshalWiresmith for forward-write,
// then prepends the varint length and the field tag. The two-call pattern
// (Size then Marshal) matches gogo's MarshalTo/Size shape but with
// wiresmith-specific method names to make the contract unambiguous.
//
// We assert MarshalWiresmith wrote exactly SizeWiresmith() bytes. A short
// write would leave uninitialised tail bytes inside the length-delimited
// payload (the slice was carved out of the reverse-write scratch buffer,
// which is reused across messages), producing a corrupt wire payload
// without any error from the user's implementation. The check turns that
// silent corruption into a marshal-time error.
func (c *CustomType) EmitMarshal(e Emitter, access string, num protowire.Number) {
	e.Writef("\tif s := %s.SizeWiresmith(); s > 0 {\n", access)
	e.Writef("\t\ti -= s\n")
	e.Writef("\t\tn, err := %s.MarshalWiresmith(dAtA[i : i+s])\n", access)
	e.Writef("\t\tif err != nil {\n")
	e.Writef("\t\t\treturn 0, err\n")
	e.Writef("\t\t}\n")
	e.Writef("\t\tif n != s {\n")
	e.Writef("\t\t\treturn 0, fmt.Errorf(\"%s.MarshalWiresmith returned %%d bytes, expected %%d\", n, s)\n", access)
	e.Writef("\t\t}\n")
	e.Writef("\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(s))\n")
	e.ReverseTag("\t\t", num, protowire.BytesType)
	e.Writef("\t}\n")
}

// EmitUnmarshal consumes the length-delimited header and hands the exact
// payload window to the user's UnmarshalWiresmith. The field is addressable
// (it's a struct field reached via the *Holder receiver), so a pointer-
// receiver UnmarshalWiresmith just works without an explicit address-of.
func (c *CustomType) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeBytesLen(e)
	e.Writef("\t\t\tif err := %s.UnmarshalWiresmith(dAtA[iNdEx:postIndex]); err != nil {\n", access)
	e.Writef("\t\t\t\treturn err\n")
	e.Writef("\t\t\t}\n")
	e.Writef("\t\t\tiNdEx = postIndex\n")
}

// EmitEqual delegates to the user type's EqualWiresmith — wiresmith doesn't
// know the type's invariants, so structural comparison is impossible
// here. Passing the value by value matches how the rest of the Equal
// emitters access fields.
func (c *CustomType) EmitEqual(e Emitter, indent, lhs, rhs string) {
	e.Writef("%sif !%s.EqualWiresmith(%s) {\n%s\treturn false\n%s}\n", indent, lhs, rhs, indent, indent)
}

// EmitCompare delegates to the user type's CompareWiresmith, which must
// return -1/0/+1 like bytes.Compare or strings.Compare. The wiresmith
// generator doesn't know the type's ordering semantics, so it can't synthesise
// the comparison itself — the contract here is symmetric with EmitEqual.
//
// On the early-exit short-circuit shape: the Compare emitter pattern is
// "if non-zero, return", matching what MessageType.EmitCompare and friends
// do.
func (c *CustomType) EmitCompare(e Emitter, indent, lhs, rhs string) {
	e.Writef("%sif c := %s.CompareWiresmith(%s); c != 0 {\n", indent, lhs, rhs)
	e.Writef("%s\treturn c\n", indent)
	e.Writef("%s}\n", indent)
}
