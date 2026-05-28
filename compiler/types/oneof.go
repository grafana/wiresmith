package types

import "google.golang.org/protobuf/encoding/protowire"

// OneofField wraps a Type to handle oneof field semantics.
//
// Size and marshal operate on the already-extracted variant value (access is
// e.g. `v.FieldName` from inside a type-switch), so they only need Inner.
//
// Unmarshal is the reverse direction: the wire bytes arrive without knowing
// which variant they belong to, so the emitter has to wrap them in the
// variant struct (`&Span_StringValue{StringValue: decoded}`). That dispatch
// needs the oneof's pascal-cased field name, the variant wrapper struct
// name, and the inner field name on that struct. Those three strings come
// from descriptors at codegen time and are stored here so the composite
// alone is enough to drive emit (no extra `EmitOneofVariantUnmarshal`
// method on the Type interface, per ARCH-1).
type OneofField struct {
	Inner Type

	// Unmarshal-only metadata. Empty for size/marshal call sites.
	OneofName   string // pascal-cased oneof name (e.g. "Status")
	VariantName string // wrapper struct (e.g. "Span_StringValue")
	FieldName   string // inner field on the wrapper (e.g. "StringValue")
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

// EmitUnmarshal consumes the wire value and assigns it into the oneof field
// wrapped in the appropriate variant struct. The caller passes the access
// expression for the oneof's interface field (`m.<OneofName>`).
//
// Three branches mirror the three classes of inner type:
//   - MessageKind: length-delimited. The merge contract reuses the existing
//     variant's message when the same variant is hit twice; otherwise a
//     fresh zero value is unmarshaled into. Cross-package callees go
//     through UnmarshalWithDepth so the SEC-5 depth counter survives the
//     package boundary.
//   - Length-delimited non-message (string, bytes): cast the
//     dAtA[iNdEx:postIndex] slice directly into the field.
//   - Value types (varint, fixed): cast the v local that EmitConsume set.
func (f *OneofField) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	AddTypeImports(e, f.Inner)
	f.Inner.EmitConsume(e)

	if _, isMsg := f.Inner.(*MessageType); isMsg {
		e.Writef("\t\t\tvar msg %s\n", ctx.MessageType)
		e.Writef("\t\t\tif ov, ok := %s.(*%s); ok {\n", access, f.VariantName)
		e.Writef("\t\t\t\tmsg = ov.%s\n", f.FieldName)
		e.Writef("\t\t\t}\n")
		emitUnmarshalCall(e, "msg", ctx.IsSamePackage)
		e.Writef("\t\t\t%s = &%s{%s: msg}\n", access, f.VariantName, f.FieldName)
		e.Writef("\t\t\tiNdEx = postIndex\n")
		return
	}
	if f.Inner.WireType() == "protowire.BytesType" {
		e.Writef("\t\t\t%s = &%s{%s: %s}\n", access, f.VariantName, f.FieldName, f.Inner.CastExpr("dAtA[iNdEx:postIndex]", ctx))
		e.Writef("\t\t\tiNdEx = postIndex\n")
		return
	}
	e.Writef("\t\t\t%s = &%s{%s: %s}\n", access, f.VariantName, f.FieldName, f.Inner.CastExpr("v", ctx))
}
