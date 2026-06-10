package generator

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// generateNoPresenceProto writes protoBody as test.proto, runs the generator,
// and returns the main .pb.go source. The proto must declare `package
// nopresence.v1` so the output lands at a known path.
func generateNoPresenceProto(t *testing.T, protoBody string) string {
	t.Helper()
	protoDir := t.TempDir()
	outDir := testOutDir(t)
	writeProto(t, protoDir, "test.proto", protoBody)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDir: protoDir}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return mustReadFile(t, filepath.Join(outDir, "nopresence", "v1", "test.pb.go"))
}

// TestNoPresenceMessageOption pins the per-message opt-out: the annotated
// message loses its fieldsPresent bitmap, its bitmap-backed Has methods,
// and every bitmap operation in Marshal/Size/Unmarshal — while an
// unannotated sibling in the same file keeps all of them.
func TestNoPresenceMessageOption(t *testing.T) {
	src := generateNoPresenceProto(t, `
syntax = "proto3";
package nopresence.v1;
option go_package = "wiresmith/gen/nopresence/v1";
import "wiresmith/options.proto";
message Inner { string s = 1; }
message Bare {
  option (wiresmith.options.no_presence) = true;
  Inner child = 1;
  int64 num = 2;
}
message Tracked {
  Inner child = 1;
  int64 num = 2;
}`)

	// The annotated message: plain struct, no bitmap, no Has.
	if got := structBody(t, src, "Bare"); strings.Contains(got, "fieldsPresent") {
		t.Errorf("Bare must not carry a fieldsPresent bitmap, got:\n%s", got)
	}
	if strings.Contains(src, "func (m *Bare) HasChild") {
		t.Errorf("Bare must not have a bitmap-backed HasChild method")
	}
	if strings.Contains(src, "func (m *Bare) HasNum") {
		t.Errorf("Bare must not have a bitmap-backed HasNum method")
	}

	// The unannotated sibling keeps the default behavior.
	if got := structBody(t, src, "Tracked"); !strings.Contains(got, "fieldsPresent") {
		t.Errorf("Tracked must keep its fieldsPresent bitmap, got:\n%s", got)
	}
	if !strings.Contains(src, "func (m *Tracked) HasChild") {
		t.Errorf("Tracked must keep HasChild")
	}
}

// TestNoPresenceFileOption pins the file-level default plus the per-message
// override: no_presence_all strips the bitmap everywhere (nested messages
// included), and an explicit no_presence = false on one message re-enables
// it for that message only.
func TestNoPresenceFileOption(t *testing.T) {
	src := generateNoPresenceProto(t, `
syntax = "proto3";
package nopresence.v1;
option go_package = "wiresmith/gen/nopresence/v1";
import "wiresmith/options.proto";
option (wiresmith.options.no_presence_all) = true;
message Inner { string s = 1; }
message Outer {
  Inner child = 1;
  message Nested { Inner child = 1; }
  Nested nested = 2;
}
message OptedBackIn {
  option (wiresmith.options.no_presence) = false;
  Inner child = 1;
}`)

	for _, name := range []string{"Outer", "Outer_Nested"} {
		if got := structBody(t, src, name); strings.Contains(got, "fieldsPresent") {
			t.Errorf("%s must not carry a fieldsPresent bitmap under no_presence_all, got:\n%s", name, got)
		}
	}
	if strings.Contains(src, "func (m *Outer) HasChild") {
		t.Errorf("Outer must not have HasChild under no_presence_all")
	}
	if got := structBody(t, src, "OptedBackIn"); !strings.Contains(got, "fieldsPresent") {
		t.Errorf("OptedBackIn (no_presence = false) must override the file default and keep its bitmap, got:\n%s", got)
	}
	if !strings.Contains(src, "func (m *OptedBackIn) HasChild") {
		t.Errorf("OptedBackIn must keep HasChild")
	}
}

// TestNoPresenceKeepsOptionalHas pins that proto3 `optional` fields — whose
// presence is the pointer's nil-ness, not the bitmap — keep their Has
// accessor under no_presence.
func TestNoPresenceKeepsOptionalHas(t *testing.T) {
	src := generateNoPresenceProto(t, `
syntax = "proto3";
package nopresence.v1;
option go_package = "wiresmith/gen/nopresence/v1";
import "wiresmith/options.proto";
message Bare {
  option (wiresmith.options.no_presence) = true;
  optional int64 maybe = 1;
}`)

	if got := structBody(t, src, "Bare"); strings.Contains(got, "fieldsPresent") {
		t.Errorf("Bare must not carry a fieldsPresent bitmap, got:\n%s", got)
	}
	if !strings.Contains(src, "func (m *Bare) HasMaybe") {
		t.Errorf("optional field must keep its nil-check HasMaybe under no_presence")
	}
}

// TestNoPresenceMarshalDropsEmptyMessage pins the documented semantic
// trade-off: without the bitmap there is no "present but empty" state, so
// Marshal/Size must not contain any bitmap consultation for the message
// field — the generated code skips an empty child entirely.
func TestNoPresenceMarshalDropsEmptyMessage(t *testing.T) {
	src := generateNoPresenceProto(t, `
syntax = "proto3";
package nopresence.v1;
option go_package = "wiresmith/gen/nopresence/v1";
import "wiresmith/options.proto";
option (wiresmith.options.no_presence_all) = true;
message Inner { string s = 1; }
message Bare {
  Inner child = 1;
}`)

	if strings.Contains(src, "m.fieldsPresent") {
		t.Errorf("generated code for a no_presence-only file must never touch m.fieldsPresent:\n%s", src)
	}
}

// structBody returns the source text of `type <name> struct { ... }` from
// src, failing the test when the declaration is missing.
func structBody(t *testing.T, src, name string) string {
	t.Helper()
	marker := "type " + name + " struct {"
	start := strings.Index(src, marker)
	if start < 0 {
		t.Fatalf("missing struct %s in generated source", name)
	}
	end := strings.Index(src[start:], "\n}\n")
	if end < 0 {
		t.Fatalf("unterminated struct %s", name)
	}
	return src[start : start+end]
}
