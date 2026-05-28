package generator

import (
	"strings"
	"testing"
)

// TestEmitUnmarshal_WireTypeMismatchDispatch confirms the per-field wire-type
// guard: when the on-wire type doesn't match what the field expects, the
// generated case skips the value via skipValue + continue rather than mis-
// interpreting the bytes as the wrong shape. Without this guard, a malformed
// payload could write an attacker-controlled value into a typed field.
func TestEmitUnmarshal_WireTypeMismatchDispatch(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
message Inner {}
message M {
  string s = 1;   // wire type 2 (LEN)
  fixed32 f = 2;  // wire type 5
  Inner nested = 3; // wire type 2 (LEN)
}
`))
	fg.emitUnmarshal(messageByName(t, fg.fd, "M"))
	body := fg.body.String()
	// Each case must guard on wireType before consuming the payload.
	// Fixed32 expects wire type 5; string/message expect wire type 2.
	assertContains(t, body, "if wireType != 2 {")
	assertContains(t, body, "if wireType != 5 {")
	// Mismatch dispatch must reuse skipValue (the shared skip helper),
	// not silently advance — that would re-interpret subsequent bytes.
	assertContains(t, body, "n, err := skipValue(dAtA[iNdEx:], wireType, fieldNum)")
	assertContains(t, body, "continue")
}

// TestEmitUnmarshal_PreScanThreshold pins the size-gated pre-scan: it is
// wrapped in `if l >= 256 {` so short messages skip the extra pass entirely.
// Without the gate, every small message pays an O(N) pass that yields zero
// savings (no slice growth to amortize), which is what motivated the
// threshold in the first place.
func TestEmitUnmarshal_PreScanThreshold(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
message Inner {}
message M {
  repeated Inner xs = 1;
}
`))
	fg.emitUnmarshal(messageByName(t, fg.fd, "M"))
	body := fg.body.String()
	assertContains(t, body, "if l >= 256 {")
	assertContains(t, body, "var preIdx int")
	// Each pre-scanned field gets a counter so post-loop allocation can size
	// the slice exactly. Field number 1 → field1count.
	assertContains(t, body, "var field1count int")
}

// TestEmitUnmarshal_PreScanOmittedWithoutCountableFields covers the other
// branch of the same gate: a message whose only fields are scalars (no
// repeated message/string/bytes, no map) gains nothing from a pre-scan.
// The emitter must not insert the `if l >= 256` block in that case.
func TestEmitUnmarshal_PreScanOmittedWithoutCountableFields(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
message M {
  int32 a = 1;
  string s = 2;
  repeated int32 nums = 3; // scalar repeated — packed, no allocation hint needed
}
`))
	fg.emitUnmarshal(messageByName(t, fg.fd, "M"))
	body := fg.body.String()
	assertNotContains(t, body, "if l >= 256 {")
	assertNotContains(t, body, "preIdx")
	// Sanity: the main loop is still there.
	assertContains(t, body, "for iNdEx < l {")
}

// TestEmitUnmarshal_FieldZeroRejected pins the inline-tag invariant: a
// decoded field number of 0 is invalid in proto3 and the generated code
// must reject it before dispatching. This was the SEC-style hole that
// motivated tightening the EmitConsumeTagAt validation; the test pins it
// at the emit level so a future refactor of that helper cannot strip the
// check.
func TestEmitUnmarshal_FieldZeroRejected(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
message M {
  int32 a = 1;
}
`))
	fg.emitUnmarshal(messageByName(t, fg.fd, "M"))
	body := fg.body.String()
	// The range check rejects field 0 (lower bound) and any value above
	// 2^29-1 (upper bound). Match both halves on the same line so a
	// reorder doesn't silently weaken the guard.
	if !strings.Contains(body, "wire>>3 < 1") || !strings.Contains(body, "wire>>3 > 0x1FFFFFFF") {
		t.Errorf("expected field-number range guard `wire>>3 < 1 || wire>>3 > 0x1FFFFFFF` in body:\n%s", body)
	}
	assertContains(t, body, `return fmt.Errorf("invalid field number")`)
}
