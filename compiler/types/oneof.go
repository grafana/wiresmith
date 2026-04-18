package types

import "google.golang.org/protobuf/encoding/protowire"

// OneofField wraps a Type to handle oneof field semantics.
// Oneof size/marshal operate on the already-extracted variant value,
// while unmarshal is handled by the caller via Inner.EmitOneofUnmarshal.
type OneofField struct {
	Inner Type
}

func (f *OneofField) RequiredImports() []string { return f.Inner.RequiredImports() }

func (f *OneofField) EmitSize(e Emitter, access string, tagSize int) {
	t := f.Inner
	if t.FixedSize() != 0 {
		e.Writef("\t\t_ = %s\n", access)
		t.EmitValueSize(e, "\t\t", access, tagSize, "n")
		return
	}
	// String/bytes oneof size uses a temp variable for len().
	// Matched by: BytesType wire, not packable, not index-accessed (excludes message).
	if t.WireType() == "protowire.BytesType" && !t.IsPackable() && !t.SizeByIndex() {
		e.Writef("\t\tl := len(%s)\n\t\tn += %d + protowire.SizeVarint(uint64(l)) + l\n", access, tagSize)
		return
	}
	t.EmitValueSize(e, "\t\t", access, tagSize, "n")
}

func (f *OneofField) EmitMarshal(e Emitter, access string, num protowire.Number) {
	AddTypeImports(e, f.Inner)
	f.Inner.EmitValueMarshal(e, "\t\t", access, num)
}

func (f *OneofField) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	panic("OneofField.EmitUnmarshal should not be called directly")
}
