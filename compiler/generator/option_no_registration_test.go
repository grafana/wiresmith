package generator

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// generateNoRegistrationUtil writes protoBody at noregistration/v1/test.proto
// — the path matching its `package noregistration.v1`, so the source-relative
// output lands at a known path — runs the generator, and returns the companion
// test_util.pb.go source (the registration glue lives there, not in the main
// .pb.go).
func generateNoRegistrationUtil(t *testing.T, protoBody string) string {
	t.Helper()
	protoDir := t.TempDir()
	outDir := testOutDir(t)
	writeProto(t, protoDir, "noregistration/v1/test.proto", protoBody)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return mustReadFile(t, filepath.Join(outDir, "noregistration", "v1", "test_util.pb.go"))
}

// noRegistrationMarkers are the tokens the init() emits ONLY under
// (wiresmith.options.no_registration) = true: the package-local registries and
// the DescBuilder/TypeBuilder fields that redirect registration at them.
var noRegistrationMarkers = []string{
	"new(protoregistry.Files)",
	"new(protoregistry.Types)",
	"FileRegistry:",
	"TypeRegistry:",
	"google.golang.org/protobuf/reflect/protoregistry",
}

// TestNoRegistrationFileOption pins that a file with
// (wiresmith.options.no_registration) = true redirects registration into a
// package-local protoregistry.Files / protoregistry.Types instead of the
// official globals — the _util.pb.go init() carries the local registries, the
// FileRegistry / TypeRegistry builder fields, and the protoregistry import.
func TestNoRegistrationFileOption(t *testing.T) {
	util := generateNoRegistrationUtil(t, `
syntax = "proto3";
package noregistration.v1;
option go_package = "wiresmith/gen/noregistration/v1";
option (wiresmith.options.no_registration) = true;
import "wiresmith/options.proto";
enum Color { COLOR_UNKNOWN = 0; COLOR_RED = 1; }
message Widget {
  string name = 1;
  Color color = 2;
  Part part = 3;
}
message Part { string sku = 1; }`)

	for _, marker := range noRegistrationMarkers {
		if !strings.Contains(util, marker) {
			t.Errorf("no_registration _util.pb.go must contain %q, got:\n%s", marker, util)
		}
	}
}

// TestNoRegistrationDefaultOff is the byte-identity guard at the unit level: a
// file WITHOUT the option must emit none of the no_registration markers and
// must not import protoregistry — registration stays on the official globals,
// byte-for-byte the pre-feature output.
func TestNoRegistrationDefaultOff(t *testing.T) {
	util := generateNoRegistrationUtil(t, `
syntax = "proto3";
package noregistration.v1;
option go_package = "wiresmith/gen/noregistration/v1";
import "wiresmith/options.proto";
enum Color { COLOR_UNKNOWN = 0; COLOR_RED = 1; }
message Widget {
  string name = 1;
  Color color = 2;
  Part part = 3;
}
message Part { string sku = 1; }`)

	for _, marker := range noRegistrationMarkers {
		if strings.Contains(util, marker) {
			t.Errorf("default (no option) _util.pb.go must NOT contain %q, got:\n%s", marker, util)
		}
	}
}
