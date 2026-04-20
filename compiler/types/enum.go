package types

import (
	"google.golang.org/protobuf/reflect/protoreflect"
)

// EnumType extends varintBase with enum-specific unmarshal (uses FieldContext.EnumType).
type EnumType struct {
	varintBase
}

// Override CastExpr and EmitUnmarshal to use ctx.EnumType for casting.

func (EnumType) CastExpr(varName string, ctx FieldContext) string {
	return ctx.EnumType + "(" + varName + ")"
}

func (EnumType) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeVarint(e)
	e.Writef("\t\t\t%s = %s(v)\n", access, ctx.EnumType)
}

func (EnumType) EmitMapEntryUnmarshal(e Emitter, varName, indent string, ctx FieldContext) {
	emitConsumeVarintAt(e, indent)
	e.Writef("%s%s = %s(v)\n", indent, varName, ctx.EnumType)
}

func init() {
	register(protoreflect.EnumKind, &EnumType{varintBase{unmarshalCast: "%s"}})
}
