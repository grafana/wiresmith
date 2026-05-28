package types

import (
	"strings"
	"testing"
)

// EmitSize: nil-guard, then delegate to MessageType.EmitValueSize via
// OptionalAccess which parenthesises to `(*access)`. The `.Size()` template
// then reads `(*access).Size()`.
func TestPointerField_EmitSize_NilGuardedDeref(t *testing.T) {
	e := &captureEmitter{}
	(&PointerField{Inner: &MessageType{}}).EmitSize(e, "m.X", 1)
	got := e.buf.String()
	if !strings.Contains(got, "if m.X != nil {") {
		t.Errorf("EmitSize: missing nil-guard:\n%s", got)
	}
	if !strings.Contains(got, "s := (*m.X).Size()") {
		t.Errorf("EmitSize: must call Size() on dereferenced inner:\n%s", got)
	}
}

func TestPointerField_EmitMarshal_NilGuardedDeref(t *testing.T) {
	e := &captureEmitter{}
	(&PointerField{Inner: &MessageType{}}).EmitMarshal(e, "m.X", 1)
	got := e.buf.String()
	if !strings.Contains(got, "if m.X != nil {") {
		t.Errorf("EmitMarshal: missing nil-guard:\n%s", got)
	}
	if !strings.Contains(got, "size, err := (*m.X).MarshalToSizedBuffer(dAtA[:i])") {
		t.Errorf("EmitMarshal: must call MarshalToSizedBuffer on dereferenced inner:\n%s", got)
	}
}

// EmitUnmarshal: lazily allocate a fresh inner message via `new(MessageType)`
// when the field is nil, then unmarshal into it. This mirrors the inline
// optional-message block in the generator but lives on the composite.
func TestPointerField_EmitUnmarshal_LazyAllocate(t *testing.T) {
	e := &captureEmitter{}
	ctx := FieldContext{MessageType: "Resource", IsSamePackage: true}
	(&PointerField{Inner: &MessageType{}}).EmitUnmarshal(e, "m.X", ctx)
	got := e.buf.String()
	if !strings.Contains(got, "if m.X == nil {") {
		t.Errorf("EmitUnmarshal: missing nil-check before allocation:\n%s", got)
	}
	if !strings.Contains(got, "m.X = new(Resource)") {
		t.Errorf("EmitUnmarshal: must `new(Resource)` when nil:\n%s", got)
	}
	if !strings.Contains(got, "m.X.unmarshal(dAtA[iNdEx:postIndex], depth+1)") {
		t.Errorf("EmitUnmarshal (same-package): must use private unmarshal:\n%s", got)
	}
}

// Cross-package must thread depth through UnmarshalWithDepth — see SEC-5.
func TestPointerField_EmitUnmarshal_CrossPackageUsesUnmarshalWithDepth(t *testing.T) {
	e := &captureEmitter{}
	ctx := FieldContext{MessageType: "external.Resource", IsSamePackage: false}
	(&PointerField{Inner: &MessageType{}}).EmitUnmarshal(e, "m.X", ctx)
	got := e.buf.String()
	if !strings.Contains(got, "m.X.UnmarshalWithDepth(dAtA[iNdEx:postIndex], depth+1)") {
		t.Errorf("EmitUnmarshal (cross-package): must use UnmarshalWithDepth:\n%s", got)
	}
}
