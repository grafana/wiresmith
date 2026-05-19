package types

import "google.golang.org/protobuf/encoding/protowire"

// PointerField wraps a Type to emit a singular pointer-shaped field.
//
// In v1 callers only construct PointerField{Inner: MessageType{}} — the option
// is restricted to message fields by validatePointerOptions. The shape mirrors
// OptionalField so the two singular-pointer composites sit side by side, but
// PointerField has its own message-aware unmarshal block: OptionalField cannot
// handle MessageKind because MessageType.CastExpr panics.
type PointerField struct {
	Inner Type
}

func (p *PointerField) RequiredImports() []string { return p.Inner.RequiredImports() }

// EmitSize emits a nil-guarded size accumulator. For the singular-message case
// the body delegates to MessageType.EmitValueSize via OptionalAccess, which
// produces `(*access).Size()` — Go auto-derefs for method calls, but the
// parenthesized form is what the size template expects.
func (p *PointerField) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tif %s != nil {\n", access)
	p.Inner.EmitValueSize(e, "\t\t", p.Inner.OptionalAccess(access), tagSize, "n")
	e.Writef("\t}\n")
}

// EmitMarshal mirrors EmitSize: nil-guard then dereferenced value marshal.
func (p *PointerField) EmitMarshal(e Emitter, access string, num protowire.Number) {
	AddTypeImports(e, p.Inner)
	e.Writef("\tif %s != nil {\n", access)
	p.Inner.EmitValueMarshal(e, "\t\t", p.Inner.OptionalAccess(access), num)
	e.Writef("\t}\n")
}

// EmitUnmarshal consumes the length-delimited header, allocates a new inner
// message when the field is nil, then unmarshals into the pointer. Mirrors the
// inline optional-message block in emit_unmarshal.go but lives here so the
// pointer-option dispatch in the generator stays a single composite-emit call.
func (p *PointerField) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	p.Inner.EmitConsume(e)
	e.Writef("\t\t\tif %s == nil {\n\t\t\t\t%s = new(%s)\n\t\t\t}\n", access, access, ctx.MessageType)
	emitUnmarshalCall(e, access, ctx.IsSamePackage)
	e.Writef("\t\t\tiNdEx = postIndex\n")
}
