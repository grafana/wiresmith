package types

import (
	"strings"
	"testing"
)

func TestMessageType_Classification(t *testing.T) {
	m := MessageType{}
	if got := m.WireType(); got != "protowire.BytesType" {
		t.Errorf("WireType() = %q, want protowire.BytesType (length-delimited)", got)
	}
	if m.IsPackable() {
		t.Error("IsPackable() = true, want false (messages are not packable)")
	}
	if m.IsFixed32() || m.IsFixed64() {
		t.Error("message must not classify as fixed-width")
	}
	if got := m.FixedSize(); got != 0 {
		t.Errorf("FixedSize() = %d, want 0", got)
	}
	// Repeated messages can't use a `for _, v := range slice` because that
	// copies the message; SizeByIndex=true routes RepeatedField through
	// `for i := range` + `slice[i]` to avoid the copy.
	if !m.SizeByIndex() {
		t.Error("SizeByIndex() = false, want true (avoids per-iteration message copy)")
	}
}

func TestMessageType_VarintSizeExpr_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for non-packable VarintSizeExpr, got none")
		}
	}()
	MessageType{}.VarintSizeExpr("x")
}

// CastExpr is meaningless for messages — unmarshal uses an Unmarshal call,
// not a value conversion. The panic catches accidental delegation through
// the scalar code path.
func TestMessageType_CastExpr_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on CastExpr for message kind")
		}
	}()
	MessageType{}.CastExpr("v", FieldContext{})
}

// OptionalAccess parenthesises the deref so size templates that follow
// the access with `.Size()` produce `(*p).Size()` rather than `*p.Size()`,
// which Go would parse as `*(p.Size())`.
func TestMessageType_OptionalAccess_Parenthesised(t *testing.T) {
	got := MessageType{}.OptionalAccess("p")
	want := "(*p)"
	if got != want {
		t.Errorf("OptionalAccess = %q, want %q", got, want)
	}
}

// EmitSize emits a scoped block with `s := access.Size()` so the size local
// is reused for the length-varint and the payload add — preventing a second
// Size() call. Empty messages (size==0) are elided from the wire.
func TestMessageType_EmitSize_ReusesSizeLocal(t *testing.T) {
	e := &captureEmitter{}
	MessageType{}.EmitSize(e, "m.Inner", 1)
	got := e.buf.String()
	if !strings.Contains(got, "s := m.Inner.Size()") {
		t.Errorf("EmitSize: missing `s := m.Inner.Size()` local:\n%s", got)
	}
	if !strings.Contains(got, "if s > 0 {") {
		t.Errorf("EmitSize: missing `if s > 0` guard (empty messages should not be encoded):\n%s", got)
	}
	if !strings.Contains(got, "n += 1 + protowire.SizeVarint(uint64(s)) + s") {
		t.Errorf("EmitSize: missing length-varint + payload add referencing `s`:\n%s", got)
	}
}

// Marshal calls MarshalToSizedBuffer which returns the bytes-written count
// (`size`), avoiding the extra Size() call before encoding.
func TestMessageType_EmitMarshal_UsesMarshalToSizedBuffer(t *testing.T) {
	e := &captureEmitter{}
	MessageType{}.EmitMarshal(e, "m.Inner", 1)
	got := e.buf.String()
	if !strings.Contains(got, "size, err := m.Inner.MarshalToSizedBuffer(dAtA[:i])") {
		t.Errorf("EmitMarshal: must call MarshalToSizedBuffer:\n%s", got)
	}
	if !strings.Contains(got, "if size > 0 {") {
		t.Errorf("EmitMarshal: empty messages must be skipped:\n%s", got)
	}
	if !strings.Contains(got, "i = protohelpers.EncodeVarint(dAtA, i, uint64(size))") {
		t.Errorf("EmitMarshal: missing length varint:\n%s", got)
	}
}

// Same-package vs cross-package unmarshal: same-package uses the private
// `unmarshal(b, depth+1)` form so the recursion-depth counter survives; the
// public Unmarshal(b) would reset depth to 0 and re-open the SEC-5 hole.
func TestMessageType_EmitUnmarshal_SamePackagePrivateCall(t *testing.T) {
	e := &captureEmitter{}
	MessageType{}.EmitUnmarshal(e, "m.Inner", FieldContext{IsSamePackage: true})
	got := e.buf.String()
	if !strings.Contains(got, "m.Inner.unmarshal(dAtA[iNdEx:postIndex], depth+1)") {
		t.Errorf("Same-package: must call private `unmarshal(..., depth+1)`:\n%s", got)
	}
	if strings.Contains(got, "UnmarshalWithDepth") {
		t.Errorf("Same-package must not call UnmarshalWithDepth:\n%s", got)
	}
}

func TestMessageType_EmitUnmarshal_CrossPackageWithDepth(t *testing.T) {
	e := &captureEmitter{}
	MessageType{}.EmitUnmarshal(e, "m.Inner", FieldContext{IsSamePackage: false})
	got := e.buf.String()
	if !strings.Contains(got, "m.Inner.UnmarshalWithDepth(dAtA[iNdEx:postIndex], depth+1)") {
		t.Errorf("Cross-package: must call UnmarshalWithDepth(..., depth+1):\n%s", got)
	}
}

// EmitMapEntryUnmarshal must capture iNdEx as `mapValueStart` BEFORE calling
// unmarshal. The map's outer EmitUnmarshal uses the [mapValueStart:iNdEx]
// slice to detect "value field was present" for merge semantics.
func TestMessageType_EmitMapEntryUnmarshal_CapturesStart(t *testing.T) {
	e := &captureEmitter{}
	MessageType{}.EmitMapEntryUnmarshal(e, "mapvalue", "\t\t", FieldContext{IsSamePackage: true})
	got := e.buf.String()
	if !strings.Contains(got, "mapValueStart := iNdEx") {
		t.Errorf("EmitMapEntryUnmarshal: must declare mapValueStart for merge detection:\n%s", got)
	}
}
