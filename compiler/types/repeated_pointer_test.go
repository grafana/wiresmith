package types

import (
	"strings"
	"testing"
)

// Nil-element semantics: nil entries are skipped during Size/Marshal (matching
// gogoproto's []*Msg behaviour). The skip is via `continue`, not panic.
func TestRepeatedPointer_EmitSize_SkipsNilEntries(t *testing.T) {
	e := &captureEmitter{}
	(&RepeatedPointer{Inner: &MessageType{}}).EmitSize(e, "m.Items", 1)
	got := e.buf.String()
	if !strings.Contains(got, "for i := range m.Items {") {
		t.Errorf("EmitSize: missing index loop:\n%s", got)
	}
	if !strings.Contains(got, "if m.Items[i] == nil {\n\t\t\tcontinue") {
		t.Errorf("EmitSize: must skip nil entries with continue:\n%s", got)
	}
	if !strings.Contains(got, "s := (*m.Items[i]).Size()") {
		t.Errorf("EmitSize: must call Size() on dereferenced element:\n%s", got)
	}
}

func TestRepeatedPointer_EmitMarshal_ReverseSkipsNil(t *testing.T) {
	e := &captureEmitter{}
	(&RepeatedPointer{Inner: &MessageType{}}).EmitMarshal(e, "m.Items", 1)
	got := e.buf.String()
	if !strings.Contains(got, "for iNdEx := len(m.Items) - 1; iNdEx >= 0; iNdEx-- {") {
		t.Errorf("EmitMarshal: must reverse-iterate for end-to-start fill:\n%s", got)
	}
	if !strings.Contains(got, "if m.Items[iNdEx] == nil {\n\t\t\tcontinue") {
		t.Errorf("EmitMarshal: must skip nil entries:\n%s", got)
	}
}

// Unmarshal always appends a non-nil pointer (`&Resource{}`).
func TestRepeatedPointer_EmitUnmarshal_AppendsNonNilPointer(t *testing.T) {
	e := &captureEmitter{}
	ctx := FieldContext{MessageType: "Resource", IsSamePackage: true}
	(&RepeatedPointer{Inner: &MessageType{}}).EmitUnmarshal(e, "m.Items", ctx)
	got := e.buf.String()
	if !strings.Contains(got, "m.Items = append(m.Items, &Resource{})") {
		t.Errorf("EmitUnmarshal: must append &Resource{} (non-nil):\n%s", got)
	}
	if !strings.Contains(got, "m.Items[len(m.Items)-1].unmarshal(") {
		t.Errorf("EmitUnmarshal: must unmarshal into the newly appended slot:\n%s", got)
	}
}

// Cross-package repeated pointer threads depth through UnmarshalWithDepth.
func TestRepeatedPointer_EmitUnmarshal_CrossPackage(t *testing.T) {
	e := &captureEmitter{}
	ctx := FieldContext{MessageType: "external.Resource", IsSamePackage: false}
	(&RepeatedPointer{Inner: &MessageType{}}).EmitUnmarshal(e, "m.Items", ctx)
	got := e.buf.String()
	if !strings.Contains(got, "UnmarshalWithDepth(dAtA[iNdEx:postIndex], depth+1)") {
		t.Errorf("EmitUnmarshal cross-package: must use UnmarshalWithDepth:\n%s", got)
	}
}

// Validation invariant: RepeatedPointer with a non-message inner must panic
// loudly. Generator validation rejects this before emit, but the panic is
// the explicit invariant.
func TestRepeatedPointer_EmitUnmarshal_PanicsWithoutMessageType(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when ctx.MessageType is empty, got none")
		}
	}()
	(&RepeatedPointer{Inner: &MessageType{}}).EmitUnmarshal(&captureEmitter{}, "m.Items", FieldContext{})
}
