package generator

import "testing"

// TestEmitOneof_VariantInterfaceName pins the naming contract:
// the iface is "<Message>_<OneofName>" and the per-variant marker method
// is "is<Iface>". Consumers (anyone matching on the marker via the gogo-
// style switch pattern) break if this drifts.
func TestEmitOneof_VariantInterfaceName(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
message M {
  oneof choice {
    string s = 1;
    int32 n = 2;
  }
}
`))
	md := messageByName(t, fg.fd, "M")
	fg.emitOneof(md, md.Oneofs().Get(0))
	body := fg.body.String()
	assertContains(t, body, "type M_Choice interface {")
	assertContains(t, body, "isM_Choice()")
	assertContains(t, body, "type M_S struct {")
	assertContains(t, body, "type M_N struct {")
	assertContains(t, body, "func (*M_S) isM_Choice() {}")
	assertContains(t, body, "func (*M_N) isM_Choice() {}")
}

// TestEmitOneof_PayloadSwitch covers scalar / bytes / message variants in
// the same oneof. Each must declare its own struct with the proper Go
// type — string/bytes byte-slice/message pointer — so the wire-level
// dispatch on assignment works.
func TestEmitOneof_PayloadSwitch(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
message Inner {}
message M {
  oneof choice {
    string  s = 1;
    bytes   b = 2;
    Inner   i = 3;
  }
}
`))
	md := messageByName(t, fg.fd, "M")
	fg.emitOneof(md, md.Oneofs().Get(0))
	body := fg.body.String()
	// Scalar/string: bare string field
	assertContains(t, body, "S string ")
	// Bytes: []byte field
	assertContains(t, body, "B []byte ")
	// Message: value-typed by default. Pointer-shaped message fields are
	// rejected at validation time on oneof variants (see
	// TestPointerOption_RejectsOneofVariant in option_pointer_test.go), so
	// the only shape that can reach emit is the value-typed one.
	assertContains(t, body, "I Inner ")
}

// TestEmitOneof_SingleVariant locks down the degenerate case: a oneof with
// exactly one field is still emitted as an interface + a single variant
// struct. Skipping the interface in this case would force callers to
// special-case 1-variant oneofs.
func TestEmitOneof_SingleVariant(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
message M {
  oneof only {
    int32 x = 1;
  }
}
`))
	md := messageByName(t, fg.fd, "M")
	fg.emitOneof(md, md.Oneofs().Get(0))
	body := fg.body.String()
	assertContains(t, body, "type M_Only interface {")
	assertContains(t, body, "isM_Only()")
	assertContains(t, body, "type M_X struct {")
	assertContains(t, body, "func (*M_X) isM_Only() {}")
}

// TestEmitOneof_SyntheticIgnored — proto3 `optional` is implemented as a
// synthetic oneof under the descriptor model, but wiresmith handles it via
// the optional pointer shape, not as a true oneof. emitAllOneofs must skip
// synthetic oneofs; emitting them would create a parallel API surface
// (M_Maybe interface + M_X variant) that consumers don't expect.
func TestEmitOneof_SyntheticIgnored(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
message M {
  optional int32 maybe = 1;
}
`))
	fg.emitAllOneofs(fg.fd)
	body := fg.body.String()
	assertNotContains(t, body, "interface {")
	assertNotContains(t, body, "M_Maybe")
}
