package types

import (
	"strings"
	"testing"
)

// Non-packable types take the per-element loop in EmitSize. Messages use
// index access (SizeByIndex=true) to avoid the per-iteration struct copy.
func TestRepeatedField_EmitSize_MessageSliceUsesIndex(t *testing.T) {
	e := &captureEmitter{}
	(&RepeatedField{Inner: &MessageType{}, IsPacked: false}).EmitSize(e, "m.Items", 1)
	got := e.buf.String()
	if !strings.Contains(got, "for i := range m.Items {") {
		t.Errorf("EmitSize: message slice must iterate by index (SizeByIndex=true):\n%s", got)
	}
	if !strings.Contains(got, "m.Items[i]") {
		t.Errorf("EmitSize: message slice must access via [i]:\n%s", got)
	}
}

// Strings/bytes are non-packable but variable-size with SizeByIndex=false →
// per-element loop with value access.
func TestRepeatedField_EmitSize_StringSliceUsesValueLoop(t *testing.T) {
	e := &captureEmitter{}
	(&RepeatedField{Inner: StringType{}, IsPacked: false}).EmitSize(e, "m.Items", 1)
	got := e.buf.String()
	if !strings.Contains(got, "for _, v := range m.Items {") {
		t.Errorf("EmitSize: string slice must iterate by value:\n%s", got)
	}
}

// Packed fixed-size scalars (fixed32/sfixed32/float) skip the SizeVarint loop
// — total payload is `len(slice) * elementSize`.
func TestRepeatedField_EmitSize_PackedFixed32_ConstantDataLen(t *testing.T) {
	e := &captureEmitter{}
	(&RepeatedField{Inner: Fixed32Type, IsPacked: true}).EmitSize(e, "m.Items", 1)
	got := e.buf.String()
	if !strings.Contains(got, "dataLen := len(m.Items) * 4") {
		t.Errorf("EmitSize packed fixed32: must compute dataLen as len*4:\n%s", got)
	}
}

// Packed fixed-size 64-bit: len * 8.
func TestRepeatedField_EmitSize_PackedFixed64_ConstantDataLen(t *testing.T) {
	e := &captureEmitter{}
	(&RepeatedField{Inner: Fixed64Type, IsPacked: true}).EmitSize(e, "m.Items", 1)
	got := e.buf.String()
	if !strings.Contains(got, "dataLen := len(m.Items) * 8") {
		t.Errorf("EmitSize packed fixed64: must compute dataLen as len*8:\n%s", got)
	}
}

// Packed bool: len * 1 (bool wire encoding is always one byte).
func TestRepeatedField_EmitSize_PackedBool_ConstantDataLen(t *testing.T) {
	e := &captureEmitter{}
	(&RepeatedField{Inner: BoolType{}, IsPacked: true}).EmitSize(e, "m.Items", 1)
	got := e.buf.String()
	if !strings.Contains(got, "dataLen := len(m.Items)") {
		t.Errorf("EmitSize packed bool: must compute dataLen as len:\n%s", got)
	}
}

// Packed variable-width (varint, sint*): must sum VarintSizeExpr over the slice.
func TestRepeatedField_EmitSize_PackedVarint_SumLoop(t *testing.T) {
	e := &captureEmitter{}
	(&RepeatedField{Inner: varintBase{}, IsPacked: true}).EmitSize(e, "m.Items", 1)
	got := e.buf.String()
	if !strings.Contains(got, "for _, v := range m.Items {") {
		t.Errorf("EmitSize packed varint: must sum SizeVarint over slice:\n%s", got)
	}
	if !strings.Contains(got, "dataLen += protowire.SizeVarint(uint64(v))") {
		t.Errorf("EmitSize packed varint: must accumulate SizeVarint:\n%s", got)
	}
}

// Unpacked fixed-size adds (tag+size) per element — multiplied, not looped.
// This is the per-field [packed=false] case.
func TestRepeatedField_EmitSize_UnpackedFixed32_Multiplied(t *testing.T) {
	e := &captureEmitter{}
	(&RepeatedField{Inner: Fixed32Type, IsPacked: false}).EmitSize(e, "m.Items", 1)
	got := e.buf.String()
	want := "\tn += len(m.Items) * 5\n"
	if !strings.Contains(got, want) {
		t.Errorf("EmitSize unpacked fixed32:\n got: %q\nwant containing: %q", got, want)
	}
}

// Reverse-write iteration: marshaling walks from end-to-start so the
// generated assembly fills the pre-sized buffer back-to-front.
func TestRepeatedField_EmitMarshal_ReverseLoop(t *testing.T) {
	e := &captureEmitter{}
	(&RepeatedField{Inner: StringType{}, IsPacked: false}).EmitMarshal(e, "m.Items", 1)
	got := e.buf.String()
	if !strings.Contains(got, "for iNdEx := len(m.Items) - 1; iNdEx >= 0; iNdEx-- {") {
		t.Errorf("EmitMarshal must use reverse-iteration loop:\n%s", got)
	}
}

// Packed unmarshal must accept both wire forms (wireType==2 for packed,
// native wireType for non-packed) so protos that emit one element per tag
// still decode correctly.
func TestRepeatedField_EmitUnmarshal_AcceptsBothWireTypes(t *testing.T) {
	e := &captureEmitter{}
	ctx := FieldContext{SliceType: "[]int32"}
	(&RepeatedField{Inner: varintBase{unmarshalCast: "int32(%s)"}, IsPacked: true}).EmitUnmarshal(e, "m.Items", ctx)
	got := e.buf.String()
	if !strings.Contains(got, "if wireType == 2 {") {
		t.Errorf("EmitUnmarshal: missing packed wire form (wireType==2):\n%s", got)
	}
	if !strings.Contains(got, "else if wireType == 0 {") {
		t.Errorf("EmitUnmarshal: missing native varint wire form (wireType==0):\n%s", got)
	}
}

// Packed unmarshal pre-allocates the destination slice with exact capacity to
// avoid the per-append growth in tight loops.
func TestRepeatedField_EmitUnmarshal_PackedFixed32_PreAllocates(t *testing.T) {
	e := &captureEmitter{}
	ctx := FieldContext{SliceType: "[]uint32"}
	(&RepeatedField{Inner: Fixed32Type, IsPacked: true}).EmitUnmarshal(e, "m.Items", ctx)
	got := e.buf.String()
	if !strings.Contains(got, "elementCount := len(data) / 4") {
		t.Errorf("EmitUnmarshal: packed fixed32 must size from data/4:\n%s", got)
	}
	if !strings.Contains(got, "m.Items = make([]uint32, 0, elementCount)") {
		t.Errorf("EmitUnmarshal: must pre-allocate with exact element count:\n%s", got)
	}
}

// Non-packable message slice unmarshal appends a fresh element then unmarshals
// into the new slot.
func TestRepeatedField_EmitUnmarshal_MessageAppendsFresh(t *testing.T) {
	e := &captureEmitter{}
	ctx := FieldContext{MessageType: "Resource", IsSamePackage: true}
	(&RepeatedField{Inner: &MessageType{}, IsPacked: false}).EmitUnmarshal(e, "m.Items", ctx)
	got := e.buf.String()
	if !strings.Contains(got, "m.Items = append(m.Items, Resource{})") {
		t.Errorf("EmitUnmarshal: must append a fresh Resource{}:\n%s", got)
	}
	if !strings.Contains(got, "m.Items[len(m.Items)-1].unmarshal(") {
		t.Errorf("EmitUnmarshal: must unmarshal into the newly appended slot:\n%s", got)
	}
}
