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
	if o.Inner.WireType() == "protowire.BytesType" {
		// Length-delimited: EmitConsume set postIndex.
		cast := o.Inner.CastExpr("dAtA[iNdEx:postIndex]", ctx)
		e.Writef("\t\t\ttmp := %s\n", cast)
		e.Writef("\t\t\t%s = &tmp\n", access)
		e.Writef("\t\t\tiNdEx = postIndex\n")
	} else {
		// Value types: EmitConsume set v.
		cast := o.Inner.CastExpr("v", ctx)
		if cast == "v" {
			e.Writef("\t\t\t%s = &v\n", access)
		} else {
			e.Writef("\t\t\ttmp := %s\n", cast)
			e.Writef("\t\t\t%s = &tmp\n", access)
		}
	}
}

// EmitEqual handles three optional-field shapes:
//   - Bytes ([]byte is already nullable): nil-pair check + bytes.Equal.
//   - Message (*Msg field): nil-pair check + nil-guarded deep Equal.
//   - Scalar (*T field): equal-pointers shortcut, then nil-pair + *deref compare.
//
// Bytes and message are explicit type assertions so the dispatch reads
// the same as the message branch below it; the EmitUnmarshal predicate
// above uses a different (broader) "OptionalAccess unchanged" form
// because it covers any future already-nullable type, not just bytes.
func (o *OptionalField) EmitEqual(e Emitter, indent, lhs, rhs string) {
	if _, isBytes := o.Inner.(*BytesType); isBytes {
		// Bytes: nil/non-nil mismatch is a difference even when contents match.
		e.Writef("%sif (%s == nil) != (%s == nil) {\n%s\treturn false\n%s}\n", indent, lhs, rhs, indent, indent)
		o.Inner.EmitEqual(e, indent, lhs, rhs)
		return
	}
	if _, isMsg := o.Inner.(*MessageType); isMsg {
		e.Writef("%sif (%s == nil) != (%s == nil) {\n%s\treturn false\n%s}\n", indent, lhs, rhs, indent, indent)
		e.Writef("%sif %s != nil && !%s.Equal(%s) {\n%s\treturn false\n%s}\n", indent, lhs, lhs, rhs, indent, indent)
		return
	}
	// Scalar optional: identical pointers compare equal cheaply; otherwise
	// require both non-nil and dereferenced equality.
	e.Writef("%sif %s != %s {\n", indent, lhs, rhs)
	e.Writef("%s\tif %s == nil || %s == nil {\n%s\t\treturn false\n%s\t}\n", indent, lhs, rhs, indent, indent)
	e.Writef("%s\tif *%s != *%s {\n%s\t\treturn false\n%s\t}\n", indent, lhs, rhs, indent, indent)
	e.Writef("%s}\n", indent)
}
