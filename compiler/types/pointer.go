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

// EmitEqual is structurally identical to the optional-message branch — nil
// mismatch is a difference, otherwise delegate to the receiver's nil-safe
// Equal. The `lhs != nil` guard is symmetric with the optional path; the
// generated Equal is nil-safe so the guard could be elided, but keeping it
// makes the "nil vs &Msg{}" distinction explicit at the call site.
func (p *PointerField) EmitEqual(e Emitter, indent, lhs, rhs string) {
	e.Writef("%sif (%s == nil) != (%s == nil) {\n%s\treturn false\n%s}\n", indent, lhs, rhs, indent, indent)
	e.Writef("%sif %s != nil && !%s.Equal(%s) {\n%s\treturn false\n%s}\n", indent, lhs, lhs, rhs, indent, indent)
}

// EmitCompare orders nil < non-nil, then delegates to the receiver's
// nil-safe Compare method. Mirrors the optional-message branch — same
// pointer shape, same dispatch.
func (p *PointerField) EmitCompare(e Emitter, indent, lhs, rhs string) {
	emitNilPairOrdering(e, indent, lhs, rhs)
	e.Writef("%sif %s != nil {\n", indent, lhs)
	p.Inner.EmitCompare(e, indent+"\t", lhs, rhs)
	e.Writef("%s}\n", indent)
}
