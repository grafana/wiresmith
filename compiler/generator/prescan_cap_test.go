package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPreScanEmitsCapClamp is the meaningful regression test for SEC-1
// (wiresmith-bmp). The test/basic-side TestPreScanCapBoundedByPayload only
// asserts an invariant (`cap ≤ len/2`) that a *well-formed* payload of
// length-delimited entries satisfies regardless of whether the generator
// emits the cap or not — so it cannot catch a regression that removes the
// cap. This test pins the generator output directly: any change that drops
// `preCapMax := l / 2` or the per-field clamp from the emitted pre-scan
// makes this fail.
func TestPreScanEmitsCapClamp(t *testing.T) {
	// A proto with a repeated *message* field forces emitPreScan to fire
	// (repeated message/string/bytes/map are the only kinds the pre-scan
	// tracks). Keep the schema minimal so the generated file is small and
	// the asserted substrings are obvious in test failure output.
	const protoBody = `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
message Item {
  int32 id = 1;
}
message Container {
  repeated Item items = 1;
}
`

	protoDir := t.TempDir()
	outDir := testOutDir(t)
	if err := os.WriteFile(filepath.Join(protoDir, "test.proto"), []byte(protoBody), 0o644); err != nil {
		t.Fatalf("writing proto: %v", err)
	}

	g := &Generator{
		Module:   "wiresmith",
		OutDir:   outDir,
		ProtoDir: protoDir,
	}
	if err := g.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	srcBytes, err := os.ReadFile(filepath.Join(outDir, "test", "v1", "test.pb.go"))
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	src := string(srcBytes)

	// The cap is the entire point of the SEC-1 fix. Missing any of these
	// substrings means a future generator edit silently re-introduced the
	// amplification primitive.
	required := []string{
		"preCapMax := l / 2",           // shared bound, derived from payload length
		"if c := field1count; c > 0 {", // scoped local + zero-guard, per pre-scan field
		"if c > preCapMax {",           // the clamp itself
		"c = preCapMax",                // clamp assignment
		"make([]Item, 0, c)",           // make() uses the clamped capacity, not the raw count
	}
	for _, want := range required {
		if !strings.Contains(src, want) {
			t.Errorf("generated pre-scan missing %q\n\nfull source:\n%s", want, src)
		}
	}

	// Also assert the raw count is *not* fed into make(): an accidental
	// fall-back to `make([]Item, 0, field1count)` would re-introduce the
	// vulnerability without tripping any of the substrings above.
	if strings.Contains(src, "make([]Item, 0, field1count)") {
		t.Errorf("generated pre-scan uses raw count in make() — SEC-1 cap was bypassed\n\nfull source:\n%s", src)
	}
}
