package types

import "google.golang.org/protobuf/encoding/protowire"

// OneofField wraps a Type for a oneof variant.
//
// Size and marshal operate on the already-extracted variant value (access is
// e.g. `v.FieldName` from inside a type-switch), so they only need Inner.
//
// Unmarshal arrives without a variant tag, so the emitter has to wrap the
// decoded value in the variant struct (`&Span_StringValue{StringValue: ...}`).
// The wrapper's three pascal-cased names are stored on the composite so
// unmarshal doesn't need an extra method on the Type interface.
type OneofField struct {
	Inner Type

	// Unmarshal-only; zero-valued for size/marshal call sites.
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

// EmitUnmarshal consumes the wire value and wraps it in the variant struct
// assigned at access (the oneof's interface field, `m.<OneofName>`).
//
// The MessageKind branch reuses the existing variant's message when the same
// variant is hit twice — proto3 merge semantics. Depth threading and the
// same-vs-cross-package unmarshal dispatch go through emitUnmarshalCall.
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
