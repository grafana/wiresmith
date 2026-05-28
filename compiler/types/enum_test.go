package types

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// EnumType extends varintBase but routes the cast through ctx.EnumType so the
// generated assignment matches the field's declared enum type. Without the
// override, EmitUnmarshal would emit a bare `int32(v)` and lose the enum
// type's name in the generated code.
func TestEnumType_CastExpr_UsesContext(t *testing.T) {
	got := EnumType{}.CastExpr("v", FieldContext{EnumType: "log.SeverityNumber"})
	want := "log.SeverityNumber(v)"
	if got != want {
		t.Errorf("CastExpr = %q, want %q", got, want)
	}
}

func TestEnumType_EmitUnmarshal_UsesContextEnumType(t *testing.T) {
	e := &captureEmitter{}
	EnumType{}.EmitUnmarshal(e, "m.X", FieldContext{EnumType: "log.SeverityNumber"})
	got := e.buf.String()
	if !strings.Contains(got, "m.X = log.SeverityNumber(v)") {
		t.Errorf("EmitUnmarshal: must cast to ctx.EnumType:\n%s", got)
	}
}

func TestEnumType_EmitMapEntryUnmarshal_UsesContextEnumType(t *testing.T) {
	e := &captureEmitter{}
	EnumType{}.EmitMapEntryUnmarshal(e, "mapvalue", "\t\t", FieldContext{EnumType: "log.SeverityNumber"})
	got := e.buf.String()
	if !strings.Contains(got, "mapvalue = log.SeverityNumber(v)") {
		t.Errorf("EmitMapEntryUnmarshal: must cast to ctx.EnumType:\n%s", got)
	}
}

// Sanity: registered EnumKind delegates to the EnumType implementation.
func TestEnumType_Registered(t *testing.T) {
	tp := Get(protoreflect.EnumKind)
	if _, ok := tp.(*EnumType); !ok {
		t.Fatalf("EnumKind registered as %T, want *EnumType", tp)
	}
}

// Enum keeps varintBase's classification, so the registered instance must
// still report VarintType / packable / no fixed size.
func TestEnumType_InheritsVarintClassification(t *testing.T) {
	tp := Get(protoreflect.EnumKind)
	if got := tp.WireType(); got != "protowire.VarintType" {
		t.Errorf("WireType() = %q, want protowire.VarintType", got)
	}
	if !tp.IsPackable() {
		t.Error("IsPackable() = false; enum must be packable like other varints")
	}
	if got := tp.FixedSize(); got != 0 {
		t.Errorf("FixedSize() = %d, want 0", got)
	}
}
