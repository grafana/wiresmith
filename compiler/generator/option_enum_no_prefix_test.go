package generator

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// generateEnumProto writes protoBody as test.proto, runs the generator, and
// returns the main .pb.go source. The proto must declare `package
// enumpfx.v1` so the output lands at a known path.
func generateEnumProto(t *testing.T, protoBody string) string {
	t.Helper()
	protoDir := t.TempDir()
	outDir := testOutDir(t)
	writeProto(t, protoDir, "test.proto", protoBody)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDir: protoDir}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return mustReadFile(t, filepath.Join(outDir, "enumpfx", "v1", "test.pb.go"))
}

// TestEnumNoPrefixOption pins the per-enum opt-out: the annotated enum's
// constants drop the EnumName_ prefix while an unannotated sibling keeps
// it, and the name/value maps stay on bare proto names for both.
func TestEnumNoPrefixOption(t *testing.T) {
	src := generateEnumProto(t, `
syntax = "proto3";
package enumpfx.v1;
option go_package = "wiresmith/gen/enumpfx/v1";
import "wiresmith/options.proto";
enum MetricType {
  option (wiresmith.options.enum_no_prefix) = true;
  UNKNOWN = 0;
  COUNTER = 1;
}
enum Color {
  COLOR_RED = 0;
}`)

	if !strings.Contains(src, "\tUNKNOWN MetricType = 0\n") {
		t.Errorf("expected bare constant UNKNOWN, got:\n%.600s", src)
	}
	if !strings.Contains(src, "\tCOUNTER MetricType = 1\n") {
		t.Errorf("expected bare constant COUNTER")
	}
	if strings.Contains(src, "MetricType_UNKNOWN") {
		t.Errorf("prefixed MetricType_UNKNOWN must not be emitted under enum_no_prefix")
	}
	// Unannotated sibling keeps the default prefix.
	if !strings.Contains(src, "\tColor_COLOR_RED Color = 0\n") {
		t.Errorf("unannotated enum must keep the prefixed constant")
	}
	// Maps and String() always use bare proto names — unchanged either way.
	if !strings.Contains(src, `"UNKNOWN": 0,`) {
		t.Errorf("value map must keep bare proto names")
	}
}

// TestEnumNoPrefixFileOption pins the file default plus per-enum override,
// including a nested enum.
func TestEnumNoPrefixFileOption(t *testing.T) {
	src := generateEnumProto(t, `
syntax = "proto3";
package enumpfx.v1;
option go_package = "wiresmith/gen/enumpfx/v1";
import "wiresmith/options.proto";
option (wiresmith.options.enum_no_prefix_all) = true;
enum Mode {
  MODE_A = 0;
}
message Holder {
  enum Inner {
    INNER_X = 0;
  }
  Inner inner = 1;
}
enum OptedBackIn {
  option (wiresmith.options.enum_no_prefix) = false;
  OBI_VAL = 0;
}`)

	if !strings.Contains(src, "\tMODE_A Mode = 0\n") {
		t.Errorf("top-level enum must drop the prefix under enum_no_prefix_all")
	}
	if !strings.Contains(src, "\tINNER_X Holder_Inner = 0\n") {
		t.Errorf("nested enum must drop the (parent-chain) prefix under enum_no_prefix_all, got:\n%.800s", src)
	}
	if !strings.Contains(src, "\tOptedBackIn_OBI_VAL OptedBackIn = 0\n") {
		t.Errorf("enum_no_prefix = false must override the file default back to prefixed")
	}
}
