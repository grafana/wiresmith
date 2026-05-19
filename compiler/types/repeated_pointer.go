package types

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

// RepeatedPointer emits a `[]*Msg`-shaped repeated message field. It is the
// repeated-position counterpart to PointerField.
//
// Why a separate composite (not a flag on RepeatedField): RepeatedField holds
// a scalar-oriented Type and dispatches on packable/packed encoding, none of
// which is relevant for messages. Keeping the two side by side means the
// existing repeated code path stays untouched and the pointer-shape
// scaffolding lives in one short file.
//
// In v1 callers only ever set Inner to MessageType. The type does not enforce
// that — the generator's validatePointerOptions guarantees the constraint.
//
// Nil-element semantics: nil entries in the input slice are skipped during
// Marshal/Size, matching gogoproto's `[]*Msg` behavior. Unmarshal always
// appends a non-nil pointer.
type RepeatedPointer struct {
	Inner Type
}

func (r *RepeatedPointer) RequiredImports() []string { return r.Inner.RequiredImports() }

func (r *RepeatedPointer) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tfor i := range %s {\n", access)
	e.Writef("\t\tif %s[i] == nil {\n\t\t\tcontinue\n\t\t}\n", access)
	r.Inner.EmitValueSize(e, "\t\t", r.Inner.OptionalAccess(access+"[i]"), tagSize, "n")
	e.Writef("\t}\n")
}

func (r *RepeatedPointer) EmitMarshal(e Emitter, access string, num protowire.Number) {
	AddTypeImports(e, r.Inner)
	e.Writef("\tfor iNdEx := len(%s) - 1; iNdEx >= 0; iNdEx-- {\n", access)
	e.Writef("\t\tif %s[iNdEx] == nil {\n\t\t\tcontinue\n\t\t}\n", access)
	r.Inner.EmitValueMarshal(e, "\t\t", r.Inner.OptionalAccess(access+"[iNdEx]"), num)
	e.Writef("\t}\n")
}

func (r *RepeatedPointer) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	r.Inner.EmitConsume(e)
	if ctx.MessageType == "" {
		// Defensive — validation rejects non-message kinds for the pointer
		// option before we get here, but the panic makes the invariant
		// explicit if a future caller bypasses validation.
		panic("RepeatedPointer requires a message-kind Inner (ctx.MessageType empty)")
	}
	e.Writef("\t\t\t%s = append(%s, &%s{})\n", access, access, ctx.MessageType)
	sliceAccess := fmt.Sprintf("%s[len(%s)-1]", access, access)
	emitUnmarshalCall(e, sliceAccess, ctx.IsSamePackage)
	e.Writef("\t\t\tiNdEx = postIndex\n")
}
