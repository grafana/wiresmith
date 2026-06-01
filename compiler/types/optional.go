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
		// For optional bytes, `append([]byte(nil)[:0], ...empty)` evaluates
		// to nil — indistinguishable from "field absent" at re-marshal time.
		// Normalize to a non-nil empty slice so presence survives a round
		// trip when the wire payload is zero-length.
		if _, isBytes := o.Inner.(*BytesType); isBytes {
			e.Writef("\t\t\tif %s == nil {\n\t\t\t\t%s = []byte{}\n\t\t\t}\n", access, access)
		}
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

// EmitCompare mirrors EmitEqual's three-shape dispatch:
//   - Bytes ([]byte already nullable): nil-pair ordering (nil < non-nil),
//     then delegate to BytesType.EmitCompare which uses bytes.Compare.
//   - Message (*Msg): nil-pair ordering, then delegate to the nil-safe
//     Compare method (the inner method already accepts a nil receiver).
//   - Scalar (*T): nil-pair ordering, then dereference and compare.
//
// "nil < non-nil" matches gogo's convention and the bead's nil-safety rule
// (`nil.Compare(non-nil) == -1`).
func (o *OptionalField) EmitCompare(e Emitter, indent, lhs, rhs string) {
	if _, isBytes := o.Inner.(*BytesType); isBytes {
		emitNilPairOrdering(e, indent, lhs, rhs)
		o.Inner.EmitCompare(e, indent, lhs, rhs)
		return
	}
	if _, isMsg := o.Inner.(*MessageType); isMsg {
		emitNilPairOrdering(e, indent, lhs, rhs)
		e.Writef("%sif %s != nil {\n", indent, lhs)
		o.Inner.EmitCompare(e, indent+"\t", lhs, rhs)
		e.Writef("%s}\n", indent)
		return
	}
	// Scalar optional: nil < non-nil; identical-pointer shortcut elided
	// because the cost of dereferencing is the same as the pointer-compare
	// branch and the codegen is simpler.
	emitNilPairOrdering(e, indent, lhs, rhs)
	e.Writef("%sif %s != nil {\n", indent, lhs)
	o.Inner.EmitCompare(e, indent+"\t", "*"+lhs, "*"+rhs)
	e.Writef("%s}\n", indent)
}

// emitNilPairOrdering writes the standard nil/non-nil 3-way ordering: both
// nil falls through, lhs nil returns -1, rhs nil returns +1. Used by every
// pointer-shaped composite (Optional, Pointer, RepeatedPointer per-element).
func emitNilPairOrdering(e Emitter, indent, lhs, rhs string) {
	e.Writef("%sif (%s == nil) != (%s == nil) {\n", indent, lhs, rhs)
	e.Writef("%s\tif %s == nil {\n%s\t\treturn -1\n%s\t}\n", indent, lhs, indent, indent)
	e.Writef("%s\treturn 1\n%s}\n", indent, indent)
}
