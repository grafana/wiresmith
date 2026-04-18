package types

import "google.golang.org/protobuf/encoding/protowire"

// OptionalField wraps a Type to handle proto3 optional field semantics.
// Emits nil-guard before delegating to the inner type's value-level methods.
type OptionalField struct {
	Inner Type
}

func (o *OptionalField) RequiredImports() []string { return o.Inner.RequiredImports() }

func (o *OptionalField) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tif %s != nil {\n", access)
	o.Inner.EmitValueSize(e, "\t\t", o.Inner.OptionalAccess(access), tagSize, "n")
	e.Writef("\t}\n")
}

func (o *OptionalField) EmitMarshal(e Emitter, access string, num protowire.Number) {
	AddTypeImports(e, o.Inner)
	e.Writef("\tif %s != nil {\n", access)
	o.Inner.EmitValueMarshal(e, "\t\t", o.Inner.OptionalAccess(access), num)
	e.Writef("\t}\n")
}

func (o *OptionalField) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	// Types where optional is same as singular (bytes: []byte is nullable).
	if o.Inner.OptionalAccess("x") == "x" {
		o.Inner.EmitUnmarshal(e, access, ctx)
		return
	}
	o.Inner.EmitConsume(e)
	cast := o.Inner.CastExpr("v", ctx)
	if cast == "v" {
		e.Writef("\t\t\t%s = &v\n", access)
	} else {
		e.Writef("\t\t\ttmp := %s\n", cast)
		e.Writef("\t\t\t%s = &tmp\n", access)
	}
	emitAdvanceBytes(e)
}
