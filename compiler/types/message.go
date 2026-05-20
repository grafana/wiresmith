package types

import (
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// MessageType implements Type for proto message fields.
// Length-delimited, non-packable, with recursive marshal/unmarshal.
type MessageType struct{}

func (MessageType) WireType() string          { return "protowire.BytesType" }
func (MessageType) IsPackable() bool          { return false }
func (MessageType) IsFixed32() bool           { return false }
func (MessageType) IsFixed64() bool           { return false }
func (MessageType) FixedSize() int            { return 0 }
func (MessageType) SizeByIndex() bool         { return true }
func (MessageType) RequiredImports() []string { return nil }

// OptionalAccess dereferences a *MessageType pointer for use by OptionalField's
// size/marshal helpers. Parenthesized to avoid precedence issues with method calls.
func (MessageType) OptionalAccess(access string) string {
	return "(*" + access + ")"
}

func (MessageType) VarintSizeExpr(access string) string {
	panicNotPackable("VarintSizeExpr")
	return ""
}

// --- Size ---

func (MessageType) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\t{\n\t\ts := %s.Size()\n\t\tif s > 0 {\n\t\t\tn += %d + protowire.SizeVarint(uint64(s)) + s\n\t\t}\n\t}\n", access, tagSize)
}

func (MessageType) EmitValueSize(e Emitter, indent, access string, tagSize int, target string) {
	e.Writef("%ss := %s.Size()\n", indent, access)
	e.Writef("%s%s += %d + protowire.SizeVarint(uint64(s)) + s\n", indent, target, tagSize)
}

// --- Marshal ---

func (MessageType) EmitMarshal(e Emitter, access string, num protowire.Number) {
	e.Writef("\t{\n")
	e.Writef("\t\tsize, err := %s.MarshalToSizedBuffer(dAtA[:i])\n", access)
	e.Writef("\t\tif err != nil {\n\t\t\treturn 0, err\n\t\t}\n")
	e.Writef("\t\tif size > 0 {\n")
	e.Writef("\t\t\ti -= size\n")
	e.Writef("\t\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(size))\n")
	e.ReverseTag("\t\t\t", num, protowire.BytesType)
	e.Writef("\t\t}\n")
	e.Writef("\t}\n")
}

func (MessageType) EmitValueMarshal(e Emitter, indent, access string, num protowire.Number) {
	e.Writef("%ssize, err := %s.MarshalToSizedBuffer(dAtA[:i])\n", indent, access)
	e.Writef("%sif err != nil {\n%s\treturn 0, err\n%s}\n", indent, indent, indent)
	e.Writef("%si -= size\n", indent)
	e.Writef("%si = protohelpers.EncodeVarint(dAtA, i, uint64(size))\n", indent)
	e.ReverseTag(indent, num, protowire.BytesType)
}

// --- Unmarshal ---

func (MessageType) EmitConsume(e Emitter) { emitConsumeBytesLen(e) }

func (MessageType) CastExpr(varName string, ctx FieldContext) string {
	panic("CastExpr called on message type")
}

func (MessageType) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeBytesLen(e)
	emitUnmarshalCall(e, access, ctx.IsSamePackage)
	e.Writef("\t\t\tiNdEx = postIndex\n")
}

func (MessageType) EmitMapEntryUnmarshal(e Emitter, varName, indent string, ctx FieldContext) {
	emitConsumeBytesLenAt(e, indent)
	// Save start position so the caller can capture raw bytes for merge semantics.
	e.Writef("%smapValueStart := iNdEx\n", indent)
	if ctx.IsSamePackage {
		e.Writef("%sif err := %s.unmarshal(dAtA[iNdEx:postIndex], depth+1); err != nil {\n%s\treturn err\n%s}\n", indent, varName, indent, indent)
	} else {
		e.Writef("%sif err := %s.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {\n%s\treturn err\n%s}\n", indent, varName, indent, indent)
	}
	e.Writef("%siNdEx = postIndex\n", indent)
}

// ZeroLiteral is "nil" — the conventional Go zero for a *Msg. The current
// getter emit paths never funnel a MessageKind through ZeroLiteral (they
// hardcode `return nil` for the message-pointer return shape), but the
// method is defined for completeness so Type stays uniformly answerable.
func (MessageType) ZeroLiteral() string { return "nil" }

// EmitEqual emits a deep-equality guard. Works for both value and pointer
// access forms because Go auto-addresses lhs when calling the
// pointer-receiver Equal method, and Equal accepts interface{}.
func (MessageType) EmitEqual(e Emitter, indent, lhs, rhs string) {
	e.Writef("%sif !%s.Equal(%s) {\n%s\treturn false\n%s}\n", indent, lhs, rhs, indent, indent)
}

func init() {
	register(protoreflect.MessageKind, &MessageType{})
}
