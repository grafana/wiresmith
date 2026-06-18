package generator

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestEmitString_ProtoTextShape pins the hand-rolled proto-text String()
// emitter (wiresmith-qcp1): the method goes into the string companion (not the
// hot main body), starts with the nil-receiver guard returning "<nil>", builds
// via strings.Builder, omits zero scalars, dereferences optional pointers, and
// never emits a bare-pointer %v that would leak a heap address.
func TestEmitString_ProtoTextShape(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message M {
  int32 plain = 1;
  optional int32 opt = 2;
  string name = 3;
}
`))
	fg.emitString(messageByName(t, fg.fd, "M"))

	body := fg.stringBody.String()
	assertContains(t, body, "func (m *M) String() string {")
	assertContains(t, body, "if m == nil {")
	assertContains(t, body, `return "<nil>"`)
	assertContains(t, body, "var b strings.Builder")
	assertContains(t, body, "strings.TrimSpace(b.String())")
	// proto names + proto-text "name: " labels (not Go field names).
	assertContains(t, body, `b.WriteString("plain: ")`)
	assertContains(t, body, `b.WriteString("opt: ")`)
	assertContains(t, body, `b.WriteString("name: ")`)
	// proto3 zero-omission guards.
	assertContains(t, body, "if m.Plain != 0 {")
	assertContains(t, body, "if len(m.Name) > 0 {")
	// Optional scalar: nil-guard then dereference, never a raw pointer %v.
	assertContains(t, body, "if m.Opt != nil {")
	assertContains(t, body, `fmt.Fprintf(&b, "%v", (*m.Opt))`)
	// Strings are quoted (proto-text).
	assertContains(t, body, "strconv.Quote(m.Name)")
	// The emitter writes only to the string companion, not the hot main body.
	if fg.body.Len() != 0 {
		t.Errorf("emitString must not write to the main body; got:\n%s", fg.body.String())
	}
	// No raw-pointer %v on the optional (that is the bug being fixed).
	if strings.Contains(body, `"%v", m.Opt)`) {
		t.Errorf("optional scalar must be dereferenced, found raw pointer %%v:\n%s", body)
	}
}

// TestEmitString_EnumByName confirms enum fields render via the value name
// (the enum's String()), not the raw integer.
func TestEmitString_EnumByName(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
enum Color { COLOR_UNKNOWN = 0; COLOR_RED = 1; }
message M { Color c = 1; }
`))
	fg.emitString(messageByName(t, fg.fd, "M"))

	body := fg.stringBody.String()
	assertContains(t, body, `b.WriteString("c: ")`)
	assertContains(t, body, "b.WriteString(m.C.String())")
}

// TestEmitString_Oneof confirms a oneof renders only the set variant by its
// proto field name via a type-switch (no unset/default output, no address).
func TestEmitString_Oneof(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message M {
  oneof value {
    string s = 1;
    int64 n = 2;
  }
}
`))
	fg.emitString(messageByName(t, fg.fd, "M"))

	body := fg.stringBody.String()
	assertContains(t, body, "switch v := m.Value.(type) {")
	assertContains(t, body, "case *M_S:")
	assertContains(t, body, "case *M_N:")
	assertContains(t, body, `b.WriteString("s: ")`)
	assertContains(t, body, `b.WriteString("n: ")`)
	// No default branch: an unset oneof renders nothing.
	assertNotContains(t, body, "default:")
}

// TestEmitString_NestedMessageBraces confirms a singular value message recurses
// through its child String() wrapped in proto-text braces.
func TestEmitString_NestedMessageBraces(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message Inner { int64 v = 1; }
message M { Inner inner = 1; }
`))
	fg.emitString(messageByName(t, fg.fd, "M"))

	body := fg.stringBody.String()
	assertContains(t, body, "if m.Inner.Size() > 0")
	assertContains(t, body, `b.WriteString("inner: ")`)
	assertContains(t, body, `b.WriteString("{")`)
	assertContains(t, body, "b.WriteString(m.Inner.String())")
	assertContains(t, body, `b.WriteString("}")`)
}

// TestEmitString_PointerOptionMessage confirms a singular message with the
// (wiresmith.options.pointer) annotation is treated as a pointer shape:
// nil-guarded, recursing via the field's own String() in braces.
func TestEmitString_PointerOptionMessage(t *testing.T) {
	files := compileAllFixture(t, `
syntax = "proto3";
package test.v1;
import "wiresmith/options.proto";
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message Leaf { int64 id = 1; }
message M {
  Leaf head = 1 [(wiresmith.options.pointer) = true];
}
`)
	var fd protoreflect.FileDescriptor
	for _, f := range files {
		if f.Path() == "test.proto" {
			fd = f
		}
	}
	if fd == nil {
		t.Fatal("test.proto missing from compile results")
	}
	fg := newFixtureGeneratorWith(t, fd, files)
	fg.emitString(messageByName(t, fg.fd, "M"))

	body := fg.stringBody.String()
	assertContains(t, body, "if m.Head != nil {")
	assertContains(t, body, "b.WriteString(m.Head.String())")
}
