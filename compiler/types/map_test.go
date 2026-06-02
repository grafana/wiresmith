package types

import (
	"strings"
	"testing"
)

func TestMapField_EmitSize_KeyValBothVariable(t *testing.T) {
	e := &captureEmitter{}
	(&MapField{Key: StringType{}, Val: StringType{}}).EmitSize(e, "m.M", 1)
	got := e.buf.String()
	// Both string: both variable-size → range over both.
	if !strings.Contains(got, "for k, v := range m.M {") {
		t.Errorf("EmitSize: expected `for k, v := range m.M`:\n%s", got)
	}
	// entrySize accumulator drives the outer SizeVarint add.
	if !strings.Contains(got, "entrySize := 0") {
		t.Errorf("EmitSize: missing entrySize local:\n%s", got)
	}
	if !strings.Contains(got, "n += 1 + protowire.SizeVarint(uint64(entrySize)) + entrySize") {
		t.Errorf("EmitSize: missing outer entry-length add:\n%s", got)
	}
}

func TestMapField_EmitSize_OnlyKeyVariable(t *testing.T) {
	e := &captureEmitter{}
	// Fixed64-valued map → only key is variable; range collapses to `for k`.
	(&MapField{Key: StringType{}, Val: Fixed64Type}).EmitSize(e, "m.M", 1)
	got := e.buf.String()
	if !strings.Contains(got, "for k := range m.M {") {
		t.Errorf("EmitSize: expected `for k := range m.M` (value is fixed-size):\n%s", got)
	}
}

func TestMapField_EmitSize_OnlyValVariable(t *testing.T) {
	e := &captureEmitter{}
	(&MapField{Key: Fixed64Type, Val: StringType{}}).EmitSize(e, "m.M", 1)
	got := e.buf.String()
	if !strings.Contains(got, "for _, v := range m.M {") {
		t.Errorf("EmitSize: expected `for _, v := range m.M` (key is fixed-size):\n%s", got)
	}
}

func TestMapField_EmitSize_BothFixed(t *testing.T) {
	e := &captureEmitter{}
	(&MapField{Key: Fixed64Type, Val: Fixed64Type}).EmitSize(e, "m.M", 1)
	got := e.buf.String()
	if !strings.Contains(got, "for range m.M {") {
		t.Errorf("EmitSize: expected `for range m.M` (both keys/values fixed-size):\n%s", got)
	}
}

// Reverse-write order: value (field 2) is written before key (field 1) so the
// final buffer has key-then-value when read forward. The `baseI` snapshot
// captures the post-value `i` so the entry-length varint can be computed.
func TestMapField_EmitMarshal_ValueBeforeKey(t *testing.T) {
	e := &captureEmitter{}
	(&MapField{Key: StringType{}, Val: StringType{}}).EmitMarshal(e, "m.M", 1)
	got := e.buf.String()

	idxBase := strings.Index(got, "baseI := i")
	idxVal := strings.Index(got, "copy(dAtA[i:], v)")
	idxKey := strings.Index(got, "copy(dAtA[i:], k)")
	idxLen := strings.Index(got, "i = protohelpers.EncodeVarint(dAtA, i, uint64(baseI-i))")

	if idxBase < 0 || idxVal < 0 || idxKey < 0 || idxLen < 0 {
		t.Fatalf("EmitMarshal missing one of baseI/val-copy/key-copy/entry-len:\n%s", got)
	}
	// Reverse-write order: baseI → value → key → entry-length varint.
	if idxBase >= idxVal || idxVal >= idxKey || idxKey >= idxLen {
		t.Errorf("EmitMarshal order wrong (want baseI < value < key < entry-len): base=%d val=%d key=%d len=%d\n%s",
			idxBase, idxVal, idxKey, idxLen, got)
	}
}

// proto3 map duplicate-key semantics are REPLACE / last-write-wins:
// EmitUnmarshal must finish each map entry by overwriting `m[mapkey]`
// unconditionally — no `mapValueBytes` capture, no `existing` merge call.
// wiresmith-05d backs out the prior merge branch (originally added to
// implement "merge on duplicate key", which protobuf-go and proto3 spec
// disagree with) and the mapValueStart machinery that fed it.
func TestMapField_EmitUnmarshal_MessageValueReplaces(t *testing.T) {
	e := &captureEmitter{}
	mf := &MapField{
		Key:       StringType{},
		Val:       &MessageType{},
		MapType:   "map[string]Resource",
		KeyGoType: "string",
		ValGoType: "Resource",
		ValCtx:    FieldContext{MessageType: "Resource", IsSamePackage: true},
	}
	mf.EmitUnmarshal(e, "m.M", FieldContext{})
	got := e.buf.String()

	if !strings.Contains(got, "m.M[mapkey] = mapvalue") {
		t.Errorf("EmitUnmarshal must finish each entry with `m.M[mapkey] = mapvalue` (REPLACE):\n%s", got)
	}
	if strings.Contains(got, "mapValueBytes") {
		t.Errorf("EmitUnmarshal must not emit mapValueBytes — merge is gone:\n%s", got)
	}
	if strings.Contains(got, "existing.unmarshal(") || strings.Contains(got, "existing.Unmarshal(") || strings.Contains(got, "existing.UnmarshalWithDepth(") {
		t.Errorf("EmitUnmarshal must not emit any `existing.*marshal*` merge call — merge is gone:\n%s", got)
	}
	if strings.Contains(got, "} else if !ok {") {
		t.Errorf("EmitUnmarshal must not branch on existing key — REPLACE is unconditional:\n%s", got)
	}
}

// Cross-package map<K, Msg> companion. Same REPLACE assertion: no
// `existing` merge call (even the depth-threading
// `existing.UnmarshalWithDepth(mapValueBytes, depth+1)` form added in
// wiresmith-1c0). The depth-threading still lives in the *initial* value
// decode at `MessageType.EmitMapEntryUnmarshal` — that's what
// `TestMessageType_EmitMapEntryUnmarshal_CrossPackageThreadsDepth` (in
// message_test.go) pins; this test owns the post-loop side.
func TestMapField_EmitUnmarshal_CrossPackageMessageReplaces(t *testing.T) {
	e := &captureEmitter{}
	mf := &MapField{
		Key:       StringType{},
		Val:       &MessageType{},
		MapType:   "map[string]external.Resource",
		KeyGoType: "string",
		ValGoType: "external.Resource",
		ValCtx:    FieldContext{MessageType: "external.Resource", IsSamePackage: false},
	}
	mf.EmitUnmarshal(e, "m.M", FieldContext{})
	got := e.buf.String()

	if !strings.Contains(got, "m.M[mapkey] = mapvalue") {
		t.Errorf("EmitUnmarshal must finish each entry with `m.M[mapkey] = mapvalue`:\n%s", got)
	}
	if strings.Contains(got, "mapValueBytes") {
		t.Errorf("EmitUnmarshal must not emit mapValueBytes — merge is gone:\n%s", got)
	}
	if strings.Contains(got, "existing.UnmarshalWithDepth(") || strings.Contains(got, "existing.Unmarshal(") {
		t.Errorf("EmitUnmarshal must not emit any `existing.*` merge call:\n%s", got)
	}
}

// Non-message (scalar) map values share the same shape: unconditional
// `m[mapkey] = mapvalue` post-loop, no mapValueBytes/merge machinery.
func TestMapField_EmitUnmarshal_ScalarValueAssigns(t *testing.T) {
	e := &captureEmitter{}
	mf := &MapField{
		Key:       StringType{},
		Val:       varintBase{unmarshalCast: "int32(%s)"},
		MapType:   "map[string]int32",
		KeyGoType: "string",
		ValGoType: "int32",
	}
	mf.EmitUnmarshal(e, "m.M", FieldContext{})
	got := e.buf.String()

	if strings.Contains(got, "mapValueBytes") {
		t.Errorf("Scalar-valued map must not emit mapValueBytes:\n%s", got)
	}
	if !strings.Contains(got, "m.M[mapkey] = mapvalue") {
		t.Errorf("Scalar-valued map must assign mapvalue directly:\n%s", got)
	}
}

// Map entry tags are decoded inline (no protowire.ConsumeTag call) and must
// be dispatched by field number: case 1 = key, case 2 = value.
func TestMapField_EmitUnmarshal_KeyAndValueDispatch(t *testing.T) {
	e := &captureEmitter{}
	mf := &MapField{
		Key:       StringType{},
		Val:       StringType{},
		MapType:   "map[string]string",
		KeyGoType: "string",
		ValGoType: "string",
	}
	mf.EmitUnmarshal(e, "m.M", FieldContext{})
	got := e.buf.String()

	idxKey := strings.Index(got, "case 1:")
	idxVal := strings.Index(got, "case 2:")
	if idxKey < 0 || idxVal < 0 {
		t.Fatalf("EmitUnmarshal missing case 1 / case 2 dispatch:\n%s", got)
	}
	if idxKey > idxVal {
		t.Errorf("EmitUnmarshal: case 1 (key) must precede case 2 (value); got key@%d val@%d", idxKey, idxVal)
	}
}

func TestMapField_RequiredImports_ConcatsKeyAndVal(t *testing.T) {
	mf := &MapField{Key: floatType(t), Val: doubleType(t)}
	got := mf.RequiredImports()
	if len(got) == 0 {
		t.Fatal("RequiredImports() = empty; want key+val imports concatenated")
	}
	hasMath, hasBinary := 0, 0
	for _, imp := range got {
		switch imp {
		case "math":
			hasMath++
		case "encoding/binary":
			hasBinary++
		}
	}
	// Both Float and Double pull in math + encoding/binary, so each must
	// appear at least twice (once per side); the concrete count depends
	// on slice concatenation order but presence is the invariant.
	if hasMath < 2 || hasBinary < 2 {
		t.Errorf("RequiredImports() = %v; want math and encoding/binary from BOTH key and value", got)
	}
}
