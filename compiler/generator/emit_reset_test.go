package generator

import (
	"strings"
	"testing"
)

// TestEmitReset_NilGuard pins the nil-receiver guard added for CR-1
// (wiresmith-v9y). The Reset() body must short-circuit on a nil receiver
// before dereferencing *m, otherwise calling Reset on a nil pointer panics —
// which one OTel consumer was observed to do via a generic post-unmarshal
// reset path.
func TestEmitReset_NilGuard(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
message M {
  int32 x = 1;
}
`))
	fg.emitReset(messageByName(t, fg.fd, "M"))
	body := fg.body.String()
	assertContains(t, body, "func (m *M) Reset() {")
	assertContains(t, body, "if m == nil {")
	assertContains(t, body, "return\n\t}")
	// The zero-assignment must come AFTER the nil check or the guard is
	// pointless. Locate both substrings and verify ordering.
	guardIdx := strings.Index(body, "if m == nil")
	zeroIdx := strings.Index(body, "*m = M{}")
	if guardIdx == -1 || zeroIdx == -1 || guardIdx >= zeroIdx {
		t.Errorf("nil guard must precede *m = M{} in Reset; guard@%d zero@%d\nbody:\n%s", guardIdx, zeroIdx, body)
	}
}

// TestEmitReset_StringIsNilSafe locks in the matching nil-safe contract on
// String(). The ReviewCaveats section of CLAUDE.md flags this as a recurring
// review hit: every pointer-receiver method must tolerate nil.
func TestEmitReset_StringIsNilSafe(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
message M {}
`))
	fg.emitReset(messageByName(t, fg.fd, "M"))
	body := fg.body.String()
	assertContains(t, body, "func (m *M) String() string {")
	assertContains(t, body, "if m == nil {")
	assertContains(t, body, `return "<nil>"`)
}

// TestEmitReset_EmitsProtoMessageMarker confirms the ProtoMessage() marker
// remains on a value receiver so it can be invoked via both pointer and value
// expressions — protobuf reflection sometimes hands a non-addressable copy.
func TestEmitReset_EmitsProtoMessageMarker(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
message Outer {
  message Inner {}
}
`))
	fg.emitAllResetMethods(fg.fd)
	body := fg.body.String()
	// Pointer-typed nil-receiver protocol applies to the type-level marker too:
	// it must be a value receiver, no nil check.
	assertContains(t, body, "func (*Outer) ProtoMessage() {}")
	assertContains(t, body, "func (*Outer_Inner) ProtoMessage() {}")
}
