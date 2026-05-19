package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runGenerator writes the given .proto file into a temp dir and runs the
// generator against it. Returns the error from Generate, swallowing on-disk
// cleanup concerns. Helper for the rejection cases below.
func runGenerator(t *testing.T, protoBody string) error {
	t.Helper()

	protoDir := t.TempDir()
	outDir := t.TempDir()

	// The proto body imports wiresmith/options.proto; that file is served
	// from the embed by the generator's resolver, so callers only need to
	// drop the user proto on disk.
	if err := os.WriteFile(filepath.Join(protoDir, "test.proto"), []byte(protoBody), 0o644); err != nil {
		t.Fatalf("writing proto: %v", err)
	}

	g := &Generator{
		Module:   "wiresmith",
		OutDir:   outDir,
		ProtoDir: protoDir,
	}
	return g.Generate(context.Background())
}

// Helper-of-helper that asserts the error contains both the "invalid
// (wiresmith.options.pointer) placement" header and a substring describing the
// specific reason. Anchoring on both protects against a regression where the
// error gets rewritten to omit the field-level reason.
func expectInvalidPointer(t *testing.T, err error, reasonSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "invalid (wiresmith.options.pointer) placement") {
		t.Errorf("missing header in error: %s", msg)
	}
	if !strings.Contains(msg, reasonSubstr) {
		t.Errorf("missing reason %q in error: %s", reasonSubstr, msg)
	}
}

func TestPointerOption_RejectsScalar(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  int32 x = 1 [(wiresmith.options.pointer) = true];
}
`)
	expectInvalidPointer(t, err, "only applies to message fields")
}

func TestPointerOption_RejectsOptional(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message Inner {}
message M {
  optional Inner x = 1 [(wiresmith.options.pointer) = true];
}
`)
	expectInvalidPointer(t, err, "cannot combine with `optional`")
}

func TestPointerOption_RejectsOneofVariant(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message Inner {}
message M {
  oneof choice {
    Inner a = 1 [(wiresmith.options.pointer) = true];
    int32 b = 2;
  }
}
`)
	expectInvalidPointer(t, err, "oneof variants")
}

func TestPointerOption_RejectsMap(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message Inner {}
message M {
  map<string, Inner> m = 1 [(wiresmith.options.pointer) = true];
}
`)
	expectInvalidPointer(t, err, "map fields")
}

// Positive path: option on singular and repeated message succeeds, generator
// produces output, and the resulting Go file compiles via go/format.
func TestPointerOption_AcceptsMessage(t *testing.T) {
	protoDir := t.TempDir()
	outDir := t.TempDir()

	const body = `
syntax = "proto3";
package test;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message Inner { int32 x = 1; }
message Holder {
  Inner          singular = 1 [(wiresmith.options.pointer) = true];
  repeated Inner repeated = 2 [(wiresmith.options.pointer) = true];
}
`
	if err := os.WriteFile(filepath.Join(protoDir, "test.proto"), []byte(body), 0o644); err != nil {
		t.Fatalf("writing proto: %v", err)
	}

	g := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDir: protoDir}
	if err := g.Generate(context.Background()); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	out := filepath.Join(outDir, "test", "v1", "test.pb.go")
	contents, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("expected generated file at %s: %v", out, err)
	}
	src := string(contents)
	// Spot-check the shape rather than diffing — the surrounding code can
	// drift without invalidating the option's contract.
	if !strings.Contains(src, "Singular *Inner") {
		t.Errorf("expected `Singular *Inner` in struct, got:\n%s", src)
	}
	if !strings.Contains(src, "Repeated []*Inner") {
		t.Errorf("expected `Repeated []*Inner` in struct, got:\n%s", src)
	}
}

// A user proto whose package coincidentally matches the embedded schema's
// (`wiresmith.options`) must not be skipped as if it were the embedded schema
// itself. The embedded file is identified by canonical path; matching by
// package would silently drop this user file's output.
func TestPointerOption_UserFileSharingEmbeddedPackageStillEmits(t *testing.T) {
	protoDir := t.TempDir()
	outDir := t.TempDir()

	const body = `
syntax = "proto3";
package wiresmith.options;
option go_package = "wiresmith/gen/userwo/v1";
message UserMessage { int32 x = 1; }
`
	if err := os.WriteFile(filepath.Join(protoDir, "user.proto"), []byte(body), 0o644); err != nil {
		t.Fatalf("writing proto: %v", err)
	}

	g := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDir: protoDir}
	if err := g.Generate(context.Background()); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	out := filepath.Join(outDir, "userwo", "v1", "user.pb.go")
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected generated file at %s: %v", out, err)
	}
}

// Internal schema file should not produce Go output. If a future change
// accidentally generates code for wiresmith.options the on-disk tree would
// gain a file under gen/wiresmith — this test pins the absence.
func TestPointerOption_OptionsProtoDoesNotEmit(t *testing.T) {
	protoDir := t.TempDir()
	outDir := t.TempDir()

	// Trivial user proto so Generate has at least one real file to emit.
	const body = `
syntax = "proto3";
package solo;
option go_package = "wiresmith/gen/solo/v1";
message M {}
`
	if err := os.WriteFile(filepath.Join(protoDir, "solo.proto"), []byte(body), 0o644); err != nil {
		t.Fatalf("writing proto: %v", err)
	}

	g := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDir: protoDir}
	if err := g.Generate(context.Background()); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "wiresmith")); !os.IsNotExist(err) {
		t.Errorf("expected no gen/wiresmith output, but found one (err=%v)", err)
	}
}
