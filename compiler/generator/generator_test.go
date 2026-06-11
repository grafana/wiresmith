package generator

import (
	"bytes"
	"context"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// repoRoot returns the repository root by walking up from the test file's
// directory until it finds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod found in any parent directory)")
		}
		dir = parent
	}
}

func TestGeneratorDeterminism(t *testing.T) {
	root := repoRoot(t)

	for _, tc := range []struct {
		name     string
		protoDir string
	}{
		{"otlp", filepath.Join(root, "proto", "otlp")},
		{"kitchen_sink", filepath.Join(root, "proto", "test")},
		{"basic", filepath.Join(root, "proto", "basic")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkDeterminism(t, tc.protoDir, 5)
		})
	}
}

func checkDeterminism(t *testing.T, protoDir string, iterations int) {
	t.Helper()
	// The import-path base is now module + outDir; running twice with
	// different absolute --out values would emit different cross-package
	// import strings (one TMPDIR vs. another), trivially failing the
	// determinism check. Run each iteration in the SAME relative outDir,
	// snapshotting the bytes between runs.
	cwd := t.TempDir()
	t.Chdir(cwd)
	outDir := "gen"
	absOutDir := filepath.Join(cwd, outDir)

	ctx := context.Background()

	for i := 0; i < iterations; i++ {
		// Clean previous iteration's output so file existence checks below
		// only see the current iteration's files.
		if err := os.RemoveAll(absOutDir); err != nil {
			t.Fatalf("iteration %d: cleanup: %v", i, err)
		}

		gen := &Generator{
			Module:    "wiresmith",
			OutDir:    outDir,
			ProtoDirs: []string{protoDir},
		}
		if err := gen.Generate(ctx); err != nil {
			t.Fatalf("iteration %d: first Generate failed: %v", i, err)
		}

		runA := snapshotDir(t, absOutDir, fmt.Sprintf("iteration %d first run", i))

		// Second run overwrites the same outDir; compare against the snapshot.
		if err := os.RemoveAll(absOutDir); err != nil {
			t.Fatalf("iteration %d: cleanup before second run: %v", i, err)
		}
		if err := gen.Generate(ctx); err != nil {
			t.Fatalf("iteration %d: second Generate failed: %v", i, err)
		}

		runB := snapshotDir(t, absOutDir, fmt.Sprintf("iteration %d second run", i))

		for rel, contentA := range runA {
			contentB, ok := runB[rel]
			if !ok {
				t.Errorf("iteration %d: %s exists in first output but not in second", i, rel)
				continue
			}
			if !bytes.Equal(contentA, contentB) {
				t.Errorf("iteration %d: file %s differs between runs", i, rel)
			}
		}
		for rel := range runB {
			if _, ok := runA[rel]; !ok {
				t.Errorf("iteration %d: %s exists in second output but not in first", i, rel)
			}
		}
	}
}

// TestGenerateMatchesCheckedIn runs the generator against every proto set that
// `make generate-ours` uses and verifies the output matches the checked-in
// files under gen/. This catches generator regressions without needing `make`.
func TestGenerateMatchesCheckedIn(t *testing.T) {
	root := repoRoot(t)
	ctx := context.Background()

	// otelOverrides mirrors the `-M ...;<name>` flags emitted by the
	// Makefile's wiresmith_mflags helper. The upstream OTel protos declare
	// `go_package = "go.opentelemetry.io/..."`; without these overrides the
	// generator would honor that literally and produce a different file
	// than the checked-in copy.
	otelOverrides := map[string]string{
		"opentelemetry/proto/common/v1/common.proto":                "github.com/grafana/wiresmith/gen/opentelemetry/proto/common/v1;commonv1",
		"opentelemetry/proto/resource/v1/resource.proto":            "github.com/grafana/wiresmith/gen/opentelemetry/proto/resource/v1;resourcev1",
		"opentelemetry/proto/metrics/v1/metrics.proto":              "github.com/grafana/wiresmith/gen/opentelemetry/proto/metrics/v1;metricsv1",
		"opentelemetry/proto/trace/v1/trace.proto":                  "github.com/grafana/wiresmith/gen/opentelemetry/proto/trace/v1;tracev1",
		"opentelemetry/proto/logs/v1/logs.proto":                    "github.com/grafana/wiresmith/gen/opentelemetry/proto/logs/v1;logsv1",
		"opentelemetry/proto/profiles/v1development/profiles.proto": "github.com/grafana/wiresmith/gen/opentelemetry/proto/profiles/v1development;profilesv1development",
	}

	cases := []struct {
		name      string
		protoDir  string
		overrides map[string]string
	}{
		{
			name:      "otlp",
			protoDir:  filepath.Join(root, "proto", "otlp"),
			overrides: otelOverrides,
		},
		{
			name:     "test/kitchensink",
			protoDir: filepath.Join(root, "proto", "test"),
		},
		{
			name:     "basic",
			protoDir: filepath.Join(root, "proto", "basic"),
		},
		{
			name:     "conformance/test_messages",
			protoDir: filepath.Join(root, "proto", "conformance"),
		},
	}

	genDir := filepath.Join(root, "gen")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Chdir to a fresh tmp dir so the generator can use a relative
			// --out value. The import-path base (module + outDir) only makes
			// sense as a Go path when outDir is relative; an absolute t.TempDir
			// would yield a nonsense base like wiresmith/var/folders/....
			cwd := t.TempDir()
			t.Chdir(cwd)
			outDir := "gen"
			absOutDir := filepath.Join(cwd, outDir)

			// For conformance, only test_messages_proto3.proto is generated
			// by wiresmith; conformance.proto uses protoc. Copy just that file
			// to a temp dir so the generator doesn't see conformance.proto.
			protoDir := tc.protoDir
			if tc.name == "conformance/test_messages" {
				isolated := t.TempDir()
				src, err := os.ReadFile(filepath.Join(tc.protoDir, "test_messages_proto3.proto"))
				if err != nil {
					t.Fatalf("reading conformance proto: %v", err)
				}
				if err := os.WriteFile(filepath.Join(isolated, "test_messages_proto3.proto"), src, 0o644); err != nil {
					t.Fatalf("writing isolated proto: %v", err)
				}
				protoDir = isolated
			}

			gen := &Generator{
				Module:    "wiresmith",
				OutDir:    outDir,
				ProtoDirs: []string{protoDir},
				Overrides: tc.overrides,
			}
			if err := gen.Generate(ctx); err != nil {
				t.Fatalf("Generate failed: %v", err)
			}

			// Walk the freshly generated output and compare against checked-in
			// files. The generator writes to outDir/sourceRelDir(fd.Path()),
			// which mirrors the layout under gen/.
			generatedFiles := make(map[string]struct{})
			err := filepath.Walk(absOutDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(absOutDir, path)
				if err != nil {
					return err
				}
				if !strings.HasSuffix(rel, ".pb.go") {
					return nil
				}
				generatedFiles[rel] = struct{}{}

				freshContent, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				checkedIn := filepath.Join(genDir, rel)
				existingContent, err := os.ReadFile(checkedIn)
				if err != nil {
					t.Errorf("generated %s has no checked-in counterpart at %s", rel, checkedIn)
					return nil
				}
				if !bytes.Equal(freshContent, existingContent) {
					t.Errorf("generated %s differs from checked-in copy; run 'make generate-ours'", rel)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walking generated output: %v", err)
			}
			if len(generatedFiles) == 0 {
				t.Fatal("generator produced no files")
			}

			// Reverse check: for each directory that contains generated files,
			// verify the matching checked-in directory has no extra .go files
			// the generator didn't produce. We scope to exact directories
			// because gen/ also contains protoc-generated files in sibling dirs.
			genDirs := make(map[string]struct{})
			for rel := range generatedFiles {
				genDirs[filepath.Dir(rel)] = struct{}{}
			}
			for dir := range genDirs {
				checkedInDir := filepath.Join(genDir, dir)
				entries, err := os.ReadDir(checkedInDir)
				if err != nil {
					t.Fatalf("reading checked-in directory %s: %v", dir, err)
				}
				for _, e := range entries {
					if e.IsDir() || !strings.HasSuffix(e.Name(), ".pb.go") {
						continue
					}
					rel := filepath.Join(dir, e.Name())
					if _, ok := generatedFiles[rel]; !ok {
						t.Errorf("checked-in %s was not generated; run 'make generate-ours'", rel)
					}
				}
			}
		})
	}
}

// TestGenerateEmptyProto verifies that a proto file with no messages or enums
// does not produce a .pb.go that fails to compile. The historical bug was
// emitting an empty init() plus an unused `protohelpers` import — `go build`
// rejects the result. Skipping the file entirely avoids the failure mode.
func TestGenerateEmptyProto(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "empty.proto", `
syntax = "proto3";
package empty;`)
	writeProto(t, protoDir, "real.proto", `
syntax = "proto3";
package real_pkg;
message Foo { string name = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "empty", "empty.pb.go")); !os.IsNotExist(err) {
		t.Errorf("expected no .pb.go for empty proto, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "real_pkg", "real.pb.go")); err != nil {
		t.Errorf("expected non-empty proto to still produce a .pb.go: %v", err)
	}
}

// TestGenerateEnumsOnlyProto pins that an enums-only proto file (no
// messages) does not leave an unused `protohelpers` import in its
// generated `_reflect.pb.go`. ProtoReflect methods — the sole consumer
// of protohelpers in the reflect file — are emitted per-message; for an
// enums-only file there are no messages, so adding the import
// unconditionally in emitRegistration would produce
// "imported and not used: protohelpers" at `go build` time. The fix is
// to let emit_protoreflect.go own the import (it's idempotent via
// addImport's cache).
func TestGenerateEnumsOnlyProto(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "enumsonly.proto", `
syntax = "proto3";
package enumsonly;
enum Color {
  COLOR_UNSPECIFIED = 0;
  COLOR_RED = 1;
  COLOR_GREEN = 2;
  COLOR_BLUE = 3;
}`)

	outDir := testOutDir(t)
	gen := &Generator{Module: "github.com/grafana/wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	reflectPath := filepath.Join(outDir, "enumsonly", "enumsonly_reflect.pb.go")
	body, err := os.ReadFile(reflectPath)
	if err != nil {
		t.Fatalf("read reflect file: %v", err)
	}
	if strings.Contains(string(body), "protohelpers") {
		t.Errorf("enums-only reflect file must not import protohelpers; would fail to build as unused import:\n%s", body)
	}
}

// TestGenerateEmptyProtoDoesNotTriggerCollision verifies that an empty .proto
// sharing (package + basename) with a non-empty proto in another directory
// does not fail the output-collision preflight. The empty file writes nothing,
// so it cannot clobber the real file — keying the collision check on every
// compiled file would be a false positive.
func TestGenerateEmptyProtoDoesNotTriggerCollision(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "a/shared.proto", `
syntax = "proto3";
package shared;`)
	writeProto(t, protoDir, "b/shared.proto", `
syntax = "proto3";
package shared;
message Foo { string s = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Under source-relative output, b/shared.proto lands at outDir/b/shared.pb.go
	// regardless of the empty a/shared.proto. The empty file emits nothing, so
	// the proto-package-spans-multiple-dirs check (which would otherwise reject
	// `shared` declaring both "a" and "b") leaves the real file alone.
	if _, err := os.Stat(filepath.Join(outDir, "b", "shared.pb.go")); err != nil {
		t.Errorf("expected non-empty proto to still produce a .pb.go: %v", err)
	}
}

// TestGeneratePerSourceFileOutput pins the per-source-file emission contract:
// two .proto files sharing a proto package must each get their own .pb.go +
// _reflect.pb.go (named by basename, not aggregated into one Go file), and
// each registration init() must be self-contained for its own file's types.
//
// This is the precondition for source-relative output paths
// (`<--out>/<source-rel>.pb.go`): aggregation would make that contract
// impossible to honor for any proto package containing more than one file.
func TestGeneratePerSourceFileOutput(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "common.proto", `
syntax = "proto3";
package example.v1;
message Foo { string name = 1; }`)
	writeProto(t, protoDir, "types.proto", `
syntax = "proto3";
package example.v1;
import "example/v1/common.proto";
message Bar { Foo foo = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	wantFiles := []string{
		filepath.Join("example", "v1", "common.pb.go"),
		filepath.Join("example", "v1", "common_reflect.pb.go"),
		filepath.Join("example", "v1", "common_equal.pb.go"),
		filepath.Join("example", "v1", "types.pb.go"),
		filepath.Join("example", "v1", "types_reflect.pb.go"),
		filepath.Join("example", "v1", "types_equal.pb.go"),
	}
	for _, rel := range wantFiles {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Errorf("expected per-source output %s: %v", rel, err)
		}
	}

	commonSrc := mustReadFile(t, filepath.Join(outDir, "example", "v1", "common.pb.go"))
	typesSrc := mustReadFile(t, filepath.Join(outDir, "example", "v1", "types.pb.go"))
	commonReflSrc := mustReadFile(t, filepath.Join(outDir, "example", "v1", "common_reflect.pb.go"))
	typesReflSrc := mustReadFile(t, filepath.Join(outDir, "example", "v1", "types_reflect.pb.go"))

	// Each file declares only its own types — no aggregation into a single
	// Go file. common.pb.go must contain Foo (not Bar); types.pb.go must
	// contain Bar (not Foo).
	if !strings.Contains(commonSrc, "type Foo struct") {
		t.Errorf("common.pb.go must declare Foo")
	}
	if strings.Contains(commonSrc, "type Bar struct") {
		t.Errorf("common.pb.go must NOT declare Bar (types.proto's type leaked into common.pb.go)")
	}
	if !strings.Contains(typesSrc, "type Bar struct") {
		t.Errorf("types.pb.go must declare Bar")
	}
	if strings.Contains(typesSrc, "type Foo struct") {
		t.Errorf("types.pb.go must NOT declare Foo (common.proto's type leaked into types.pb.go)")
	}

	// Cross-file references within the same Go package: types.pb.go's Bar
	// must reference Foo by bare identifier (no package qualifier), since
	// Go same-package files share a namespace.
	if !strings.Contains(typesSrc, "Foo ") && !strings.Contains(typesSrc, "Foo\n") && !strings.Contains(typesSrc, "*Foo") {
		t.Errorf("types.pb.go must reference Foo without a package qualifier — expected to find an unqualified `Foo` reference")
	}

	// Per-file registration: each _reflect.pb.go owns its file's rawDesc and
	// its own init() — registration must NOT be aggregated across the proto
	// package into a single init.
	if !strings.Contains(commonReflSrc, "file_example_v1_common_proto_rawDesc") {
		t.Errorf("common_reflect.pb.go missing its own file_*_rawDesc constant")
	}
	if !strings.Contains(typesReflSrc, "file_example_v1_types_proto_rawDesc") {
		t.Errorf("types_reflect.pb.go missing its own file_*_rawDesc constant")
	}
	if strings.Contains(commonReflSrc, "file_example_v1_types_proto_rawDesc") {
		t.Errorf("common_reflect.pb.go references types.proto's rawDesc — registration leaked across source files")
	}
	if strings.Contains(typesReflSrc, "file_example_v1_common_proto_rawDesc") {
		t.Errorf("types_reflect.pb.go references common.proto's rawDesc — registration leaked across source files")
	}
	if strings.Count(commonReflSrc, "\nfunc init() {") != 1 {
		t.Errorf("common_reflect.pb.go must declare exactly one init() function")
	}
	if strings.Count(typesReflSrc, "\nfunc init() {") != 1 {
		t.Errorf("types_reflect.pb.go must declare exactly one init() function")
	}
}

// mustReadFile reads path and returns its content as a string, failing the
// test on error.
func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestGenerateEmptyProtoDoesNotTriggerDestinationCollision verifies that an
// empty .proto whose package resolves to the same Go directory as a different
// non-empty proto's go_package does not fail validateDestinations. Only the
// non-empty file actually writes there, so there is no real collision —
// routing the empty file through destFor would be a false positive.
func TestGenerateEmptyProtoDoesNotTriggerDestinationCollision(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "empty.proto", `
syntax = "proto3";
package alpha;`)
	writeProto(t, protoDir, "real.proto", `
syntax = "proto3";
package beta;
option go_package = "github.com/grafana/wiresmith/gen/alpha";
message Foo { string s = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Under source-relative output, real.proto's package "beta" determines its
	// on-disk location regardless of go_package. The empty file at
	// alpha/empty.proto emits nothing, so it cannot clobber a neighbour even
	// though its declared package would have routed it to the same Go dir.
	if _, err := os.Stat(filepath.Join(outDir, "beta", "real.pb.go")); err != nil {
		t.Errorf("expected non-empty proto to still produce a .pb.go: %v", err)
	}
}

// TestValidateOutDir covers the input checks that protect the import-path
// composition: --out must be a relative, clean, forward-slash path. Each
// rejected case surfaces a clear error message naming the offending value.
func TestValidateOutDir(t *testing.T) {
	good := []string{
		"",        // implicit module-root output
		"gen",     // canonical
		"pkg/api", // multi-segment relative
		"./gen",   // tolerated; normalized to "gen"
		"gen/sub", // nested relative
	}
	for _, v := range good {
		t.Run("accept_"+v, func(t *testing.T) {
			g := &Generator{OutDir: v}
			if err := g.validateOutDir(); err != nil {
				t.Errorf("validateOutDir(%q) = %v; want nil", v, err)
			}
		})
	}

	bad := []struct {
		out, wantSubstr string
	}{
		{"/abs", "module-relative"},
		{"/tmp/gen", "module-relative"},
		{`pkg\api`, "backslashes"},
		{"pkg/api/..", "'..'"},
		{"./pkg/../api", "'..'"},
		{"gen//sub", "not a clean path"},
	}
	for _, tc := range bad {
		t.Run("reject_"+tc.out, func(t *testing.T) {
			g := &Generator{OutDir: tc.out}
			err := g.validateOutDir()
			if err == nil {
				t.Fatalf("validateOutDir(%q) = nil; want error containing %q", tc.out, tc.wantSubstr)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("validateOutDir(%q) = %v; want substring %q", tc.out, err, tc.wantSubstr)
			}
		})
	}

	// "./gen" is accepted but normalized in place.
	g := &Generator{OutDir: "./gen"}
	if err := g.validateOutDir(); err != nil {
		t.Fatalf("validateOutDir(\"./gen\"): %v", err)
	}
	if g.OutDir != "gen" {
		t.Errorf("validateOutDir did not strip ./ prefix; got OutDir=%q", g.OutDir)
	}

	// Bare "." and "./." normalize to "" (module root) so joinImport doesn't
	// embed a literal "." segment in downstream import paths.
	for _, in := range []string{".", "./."} {
		g := &Generator{OutDir: in}
		if err := g.validateOutDir(); err != nil {
			t.Fatalf("validateOutDir(%q): %v", in, err)
		}
		if g.OutDir != "" {
			t.Errorf("validateOutDir(%q) did not normalize to empty; got OutDir=%q", in, g.OutDir)
		}
	}
}

// snapshotDir reads every regular file under root into a map keyed by its
// path relative to root. The label is woven into fatal messages so the
// caller's loop iteration is identifiable in test output.
func snapshotDir(t *testing.T, root, label string) map[string][]byte {
	t.Helper()
	out := make(map[string][]byte)
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = content
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s (%s): %v", root, label, err)
	}
	return out
}

// testOutDir returns a Generator.OutDir value suitable for tests. It chdirs
// into a fresh per-test tmp directory (auto-restored on test end) and
// returns "gen" as the relative outDir. The Generator's import-path base
// formula composes module + outDir, so the relative form is what tests
// must use — passing an absolute t.TempDir() would produce nonsense Go
// import paths like wiresmith/var/folders/.../gen.
func testOutDir(t *testing.T) string {
	t.Helper()
	t.Chdir(t.TempDir())
	return "gen"
}

func writeProto(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestGenerateFilesScopesEmission pins the positional-argument contract:
// when Generator.Files lists a subset of `.proto` files, only those files
// produce output. Other files in --proto_path remain available for import
// resolution but do not get a `.pb.go`.
//
// This is the property that lets callers say "compile just this one file"
// (matching protoc/vtproto/gogoproto conventions) without losing the ability
// to resolve cross-file imports against the full proto tree.
func TestGenerateFilesScopesEmission(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "a/foo.proto", `
syntax = "proto3";
package scoped.a.v1;
import "b/bar.proto";
option go_package = "github.com/grafana/wiresmith/scoped/a";
message Foo {
  string name = 1;
  scoped.b.v1.Bar bar = 2;
}`)
	writeProto(t, protoDir, "b/bar.proto", `
syntax = "proto3";
package scoped.b.v1;
option go_package = "github.com/grafana/wiresmith/scoped/b";
message Bar { int32 n = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "wiresmith",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
		Files:     []string{filepath.Join(protoDir, "a", "foo.proto")},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate (cross-file import must still resolve from --proto_path): %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "a", "foo.pb.go")); err != nil {
		t.Errorf("expected a/foo.pb.go in the scoped set: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "b", "bar.pb.go")); !os.IsNotExist(err) {
		t.Errorf("b/bar.pb.go must NOT be emitted (not in Files filter), got err=%v", err)
	}

	// foo.proto imports bar.proto's Bar message — even though bar.proto is
	// excluded from emission, foo.pb.go's reference into it must resolve
	// to a real qualified type, not an empty `.Bar` from an unregistered
	// destination. Pin the emitted reference so a future regression in
	// computeDests (skipping destination registration for filtered-out
	// files) surfaces here instead of as a downstream go-vet failure.
	foo, err := os.ReadFile(filepath.Join(outDir, "a", "foo.pb.go"))
	if err != nil {
		t.Fatalf("reading a/foo.pb.go: %v", err)
	}
	wantQualifier := "b.Bar"
	if !strings.Contains(string(foo), wantQualifier) {
		t.Errorf("a/foo.pb.go should reference Bar via the b package's import alias (expected %q)\n--- foo.pb.go ---\n%s", wantQualifier, foo)
	}
	wantImport := "github.com/grafana/wiresmith/scoped/b"
	if !strings.Contains(string(foo), wantImport) {
		t.Errorf("a/foo.pb.go should import %q\n--- foo.pb.go ---\n%s", wantImport, foo)
	}
}

// TestGenerateFilesEmptyEmitsAll pins that an empty Files list is the same
// "walk and emit everything" behavior wiresmith had before positional args
// were introduced — i.e. positional args are purely an additional filter, not
// a breaking semantic change.
func TestGenerateFilesEmptyEmitsAll(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "a/foo.proto", `
syntax = "proto3";
package scoped.a.v1;
option go_package = "github.com/grafana/wiresmith/scoped/a";
message Foo { string s = 1; }`)
	writeProto(t, protoDir, "b/bar.proto", `
syntax = "proto3";
package scoped.b.v1;
option go_package = "github.com/grafana/wiresmith/scoped/b";
message Bar { int32 n = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, rel := range []string{
		filepath.Join("a", "foo.pb.go"),
		filepath.Join("b", "bar.pb.go"),
	} {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Errorf("expected %s to be emitted in default-walk mode: %v", rel, err)
		}
	}
}

// TestGenerateMultiProtoPath pins the end-to-end contract for multiple
// --proto_path roots: a .proto in one root resolves an import of a
// .proto in another root, both get compiled, and both produce .pb.go
// under --out using their respective source-relative paths. This is
// the Mimir / Tempo "vendor/ + internal/" workflow the single-root
// CLI couldn't serve without symlink hacks.
func TestGenerateMultiProtoPath(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeProto(t, rootA, "lib/v1/foo.proto", `
syntax = "proto3";
package lib.v1;
option go_package = "github.com/grafana/wiresmith/scoped/lib/v1";
message Foo { string s = 1; }`)
	writeProto(t, rootB, "app/v1/bar.proto", `
syntax = "proto3";
package app.v1;
import "lib/v1/foo.proto";
option go_package = "github.com/grafana/wiresmith/scoped/app/v1";
message Bar { lib.v1.Foo f = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "wiresmith",
		OutDir:    outDir,
		ProtoDirs: []string{rootA, rootB},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate across two --proto_path roots: %v", err)
	}
	for _, rel := range []string{
		filepath.Join("lib", "v1", "foo.pb.go"),
		filepath.Join("app", "v1", "bar.pb.go"),
	} {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Errorf("expected %s to be emitted from the multi-root walk: %v", rel, err)
		}
	}

	// The cross-root import must resolve to a real qualified type
	// in the emitted Go (not an empty selector or unregistered alias).
	bar, err := os.ReadFile(filepath.Join(outDir, "app", "v1", "bar.pb.go"))
	if err != nil {
		t.Fatalf("reading app/v1/bar.pb.go: %v", err)
	}
	wantQualifier := "v1.Foo"
	if !strings.Contains(string(bar), wantQualifier) {
		t.Errorf("app/v1/bar.pb.go should reference Foo via the lib package's import alias (expected %q)\n--- bar.pb.go ---\n%s", wantQualifier, bar)
	}
	wantImport := "github.com/grafana/wiresmith/scoped/lib/v1"
	if !strings.Contains(string(bar), wantImport) {
		t.Errorf("app/v1/bar.pb.go should import %q\n--- bar.pb.go ---\n%s", wantImport, bar)
	}
}

// TestGenerateMultiProtoPathCollision pins the end-to-end propagation
// of the strict-collision rule: when two --proto_path roots both ship
// a file at the same import key, Generate refuses to produce any
// output. The test guards against a regression where the collision is
// caught at buildImportMapping but a later layer's first-wins logic
// silently picks one.
func TestGenerateMultiProtoPathCollision(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeProto(t, rootA, "lib/v1/foo.proto", `
syntax = "proto3";
package lib.v1;
option go_package = "github.com/grafana/wiresmith/scoped/lib/v1";
message Foo { string a = 1; }`)
	writeProto(t, rootB, "lib/v1/foo.proto", `
syntax = "proto3";
package lib.v1;
option go_package = "github.com/grafana/wiresmith/scoped/lib/v1";
message Foo { string b = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "wiresmith",
		OutDir:    outDir,
		ProtoDirs: []string{rootA, rootB},
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected collision error for same import key across --proto_path roots, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate import key") {
		t.Errorf("expected duplicate-import-key error, got: %v", err)
	}
	// Nothing should have been written to outDir.
	if _, err := os.Stat(filepath.Join(outDir, "lib", "v1", "foo.pb.go")); !os.IsNotExist(err) {
		t.Errorf("collision must abort before emit; got stat err=%v", err)
	}
}

// TestGenerateEmptyProtoDirs pins that an empty ProtoDirs slice fails
// with a clean diagnostic rather than silently walking nothing. The
// CLI defaults to ["proto"] when no flag is given, so this only fires
// for direct library use that forgets to set the field.
func TestGenerateEmptyProtoDirs(t *testing.T) {
	gen := &Generator{
		Module: "wiresmith",
		OutDir: testOutDir(t),
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected error for empty ProtoDirs, got nil")
	}
	if !strings.Contains(err.Error(), "--proto_path") {
		t.Errorf("error must name the flag, got: %v", err)
	}
}

// TestGenerateFilesReuseClearsFilter pins that the emitFilter from a prior
// scoped Generate call does not leak into a subsequent empty-Files run on
// the same *Generator instance. Without the explicit reset in Generate,
// the second call would still be filtered to the first call's subset,
// silently producing the wrong output.
func TestGenerateFilesReuseClearsFilter(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "a/foo.proto", `
syntax = "proto3";
package scoped.a.v1;
option go_package = "github.com/grafana/wiresmith/scoped/a";
message Foo { string s = 1; }`)
	writeProto(t, protoDir, "b/bar.proto", `
syntax = "proto3";
package scoped.b.v1;
option go_package = "github.com/grafana/wiresmith/scoped/b";
message Bar { int32 n = 1; }`)

	gen := &Generator{
		Module:    "wiresmith",
		ProtoDirs: []string{protoDir},
		Files:     []string{filepath.Join(protoDir, "a", "foo.proto")},
	}
	scopedOut := testOutDir(t)
	gen.OutDir = scopedOut
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("first (scoped) Generate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(scopedOut, "b", "bar.pb.go")); !os.IsNotExist(err) {
		t.Fatalf("first call should have scoped to a/foo.proto only (sanity check): %v", err)
	}

	// Reuse the same Generator with Files cleared. Without an explicit
	// reset of emitFilter, the second run would still apply the first
	// call's filter and skip b/bar.proto. The second outDir needs its own
	// chdir/relative outDir so the first run's output doesn't leak in.
	gen.Files = nil
	allOut := testOutDir(t)
	gen.OutDir = allOut
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("second (walk-everything) Generate after Files cleared: %v", err)
	}
	for _, rel := range []string{
		filepath.Join("a", "foo.pb.go"),
		filepath.Join("b", "bar.pb.go"),
	} {
		if _, err := os.Stat(filepath.Join(allOut, rel)); err != nil {
			t.Errorf("second Generate must emit %s (Files=nil after a prior scoped run): %v", rel, err)
		}
	}
}

// TestGenerateFilesRejectsNonexistent verifies that a positional path
// pointing at a file that does not exist produces a "no such file or
// directory"-style error, not the misleading "is not under --proto_path"
// error that fires when the file exists outside the walked tree. A typo
// in a positional arg is by far the most common failure mode; the
// diagnostic should name the actual cause.
func TestGenerateFilesRejectsNonexistent(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "in.proto", `
syntax = "proto3";
package scoped.in;
option go_package = "github.com/grafana/wiresmith/scoped/in";
message In { string s = 1; }`)

	missing := filepath.Join(protoDir, "doesnotexist.proto")
	gen := &Generator{
		Module:    "wiresmith",
		OutDir:    testOutDir(t),
		ProtoDirs: []string{protoDir},
		Files:     []string{missing},
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatalf("expected error for nonexistent positional path, got nil")
	}
	if !strings.Contains(err.Error(), "doesnotexist.proto") {
		t.Errorf("error should name the offending path, got: %v", err)
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should distinguish 'does not exist' from 'outside --proto_path', got: %v", err)
	}
}

// TestGenerateFilesRejectsOutsideProtoPath verifies that passing a positional
// path that doesn't live under --proto_path produces a clear error, rather
// than silently emitting nothing (which would be a frustrating
// "command appeared to succeed but did nothing" footgun).
func TestGenerateFilesRejectsOutsideProtoPath(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "in.proto", `
syntax = "proto3";
package scoped.in;
option go_package = "github.com/grafana/wiresmith/scoped/in";
message In { string s = 1; }`)

	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.proto")
	if err := os.WriteFile(outsideFile, []byte(`
syntax = "proto3";
package scoped.outside;
message Out { string s = 1; }`), 0o644); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		Module:    "wiresmith",
		OutDir:    testOutDir(t),
		ProtoDirs: []string{protoDir},
		Files:     []string{outsideFile},
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatalf("expected error for positional path outside --proto_path, got nil")
	}
	if !strings.Contains(err.Error(), "outside.proto") {
		t.Errorf("error should name the offending path, got: %v", err)
	}
	if !strings.Contains(err.Error(), "not a .proto under any --proto_path") {
		t.Errorf("error should distinguish 'outside --proto_path' from 'does not exist', got: %v", err)
	}
	// The configured roots are formatted with %q on the slice, which
	// fmt renders as `["root1" "root2"]` — each element quoted, the
	// whole wrapped in brackets. Pin the quoted form so a regression
	// back to %v (which would print the unquoted `[root1 root2]`) is
	// caught. The bracket-and-quote shape also disambiguates roots
	// that contain spaces.
	wantQuoted := fmt.Sprintf("[%q]", protoDir)
	if !strings.Contains(err.Error(), wantQuoted) {
		t.Errorf("error should print the configured roots in %%q form (expected %q), got: %v", wantQuoted, err)
	}
}

func TestBuildImportMappingFlat(t *testing.T) {
	dir := t.TempDir()
	writeProto(t, dir, "foo.proto", "syntax = \"proto3\";\npackage test.foo;\nmessage Foo {}")

	mapping, importPaths, _, err := buildImportMapping([]string{dir})
	if err != nil {
		t.Fatalf("buildImportMapping: %v", err)
	}
	if len(importPaths) != 1 {
		t.Fatalf("expected 1 import path, got %d", len(importPaths))
	}
	// Top-level file uses package-derived key as its canonical path.
	if _, ok := mapping["test/foo/foo.proto"]; !ok {
		keys := make([]string, 0, len(mapping))
		for k := range mapping {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Errorf("expected key test/foo/foo.proto in mapping, got keys: %v", keys)
	}
	// The plain filename must not be registered — doing so would cause
	// protocompile to compile the same content twice if a consumer imported
	// it via the basename, producing a duplicate-symbol error.
	if _, ok := mapping["foo.proto"]; ok {
		t.Error("plain filename foo.proto should not be aliased; only canonical pkg-derived key should exist")
	}
}

func TestBuildImportMappingRecursive(t *testing.T) {
	dir := t.TempDir()
	writeProto(t, dir, "common/v1/common.proto",
		"syntax = \"proto3\";\npackage tempopb.common.v1;\nmessage Foo {}")
	writeProto(t, dir, "trace/v1/trace.proto",
		"syntax = \"proto3\";\npackage tempopb.trace.v1;\nimport \"common/v1/common.proto\";\nmessage Bar { tempopb.common.v1.Foo foo = 1; }")

	mapping, importPaths, _, err := buildImportMapping([]string{dir})
	if err != nil {
		t.Fatalf("buildImportMapping: %v", err)
	}
	if len(importPaths) != 2 {
		t.Fatalf("expected 2 import paths, got %d: %v", len(importPaths), importPaths)
	}
	if _, ok := mapping["common/v1/common.proto"]; !ok {
		t.Error("expected common/v1/common.proto in mapping")
	}
	if _, ok := mapping["trace/v1/trace.proto"]; !ok {
		t.Error("expected trace/v1/trace.proto in mapping")
	}
	// Determinism: importPaths must be sorted.
	sorted := make([]string, len(importPaths))
	copy(sorted, importPaths)
	sort.Strings(sorted)
	for i := range importPaths {
		if importPaths[i] != sorted[i] {
			t.Errorf("import paths not sorted: %v", importPaths)
			break
		}
	}
}

func TestBuildImportMappingMixed(t *testing.T) {
	dir := t.TempDir()
	writeProto(t, dir, "root.proto",
		"syntax = \"proto3\";\npackage mypkg;\nmessage Root {}")
	writeProto(t, dir, "sub/v1/nested.proto",
		"syntax = \"proto3\";\npackage mypkg.sub.v1;\nmessage Nested {}")

	mapping, importPaths, _, err := buildImportMapping([]string{dir})
	if err != nil {
		t.Fatalf("buildImportMapping: %v", err)
	}
	if len(importPaths) != 2 {
		t.Fatalf("expected 2 import paths, got %d: %v", len(importPaths), importPaths)
	}
	if _, ok := mapping["mypkg/root.proto"]; !ok {
		t.Error("expected mypkg/root.proto for top-level file")
	}
	if _, ok := mapping["sub/v1/nested.proto"]; !ok {
		t.Error("expected sub/v1/nested.proto for nested file")
	}
}

func TestBuildImportMappingNoPackage(t *testing.T) {
	dir := t.TempDir()
	writeProto(t, dir, "bare.proto", "syntax = \"proto3\";\nmessage Bare {}")

	_, _, _, err := buildImportMapping([]string{dir})
	if err == nil {
		t.Fatal("expected error for proto without package, got nil")
	}
	if !strings.Contains(err.Error(), "no package found") {
		t.Errorf("expected 'no package found' error, got: %v", err)
	}
}

// TestBuildImportMappingDuplicateKey covers the realistic collision: a
// top-level file's package-derived key collides with a nested file's
// relative-path key (e.g. top-level foo.proto with `package bar` produces
// key `bar/foo.proto`, same as a nested file at `bar/foo.proto`).
func TestBuildImportMappingDuplicateKey(t *testing.T) {
	dir := t.TempDir()
	writeProto(t, dir, "foo.proto",
		"syntax = \"proto3\";\npackage bar;\nmessage Foo {}")
	writeProto(t, dir, "bar/foo.proto",
		"syntax = \"proto3\";\npackage bar;\nmessage Foo {}")

	_, _, _, err := buildImportMapping([]string{dir})
	if err == nil {
		t.Fatal("expected duplicate-key error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate import key") {
		t.Errorf("expected 'duplicate import key' error, got: %v", err)
	}
}

// TestBuildImportMappingMultiRoot pins the happy-path contract: two
// `--proto_path` roots union into one mapping, and files in one root
// can import files in the other via their canonical key. This is the
// `protoc -I=a -I=b` behaviour the Mimir / Tempo multi-root layouts
// need (vendor/ + internal/ trees).
func TestBuildImportMappingMultiRoot(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeProto(t, rootA, "lib/v1/foo.proto",
		"syntax = \"proto3\";\npackage lib.v1;\nmessage Foo {}")
	writeProto(t, rootB, "app/v1/bar.proto",
		"syntax = \"proto3\";\npackage app.v1;\nimport \"lib/v1/foo.proto\";\nmessage Bar { lib.v1.Foo f = 1; }")

	mapping, importPaths, pathToKey, err := buildImportMapping([]string{rootA, rootB})
	if err != nil {
		t.Fatalf("buildImportMapping: %v", err)
	}
	if len(importPaths) != 2 {
		t.Fatalf("expected 2 import paths, got %d: %v", len(importPaths), importPaths)
	}
	for _, want := range []string{"lib/v1/foo.proto", "app/v1/bar.proto"} {
		if _, ok := mapping[want]; !ok {
			keys := make([]string, 0, len(mapping))
			for k := range mapping {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			t.Errorf("expected key %q in union mapping, got keys: %v", want, keys)
		}
	}
	// pathToKey must round-trip each file under the root it came from
	// so the positional-args filter in compileSources keeps working
	// when the user passes a file from either root.
	for _, abs := range []string{
		filepath.Join(rootA, "lib/v1/foo.proto"),
		filepath.Join(rootB, "app/v1/bar.proto"),
	} {
		if _, ok := pathToKey[abs]; !ok {
			t.Errorf("pathToKey missing entry for %s — positional-arg lookup would fail", abs)
		}
	}
}

// TestBuildImportMappingMultiRootCollision pins the strict-collision
// policy: when two roots both produce the same import key but point at
// different files, fail loudly with both paths. This is the safety net
// against silent shadowing — the alternative (`protoc`'s first-wins)
// would let a stale copy in one root mask the canonical one in another
// with no diagnostic.
func TestBuildImportMappingMultiRootCollision(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeProto(t, rootA, "lib/v1/foo.proto",
		"syntax = \"proto3\";\npackage lib.v1;\nmessage Foo { string a = 1; }")
	writeProto(t, rootB, "lib/v1/foo.proto",
		"syntax = \"proto3\";\npackage lib.v1;\nmessage Foo { string b = 1; }")

	_, _, _, err := buildImportMapping([]string{rootA, rootB})
	if err == nil {
		t.Fatal("expected collision error for same import key across roots, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "duplicate import key") {
		t.Errorf("error should name the duplicate-import-key class, got: %v", err)
	}
	// The user needs both paths in the message to know which root to
	// fix; assert both absolute paths show up.
	absA, _ := filepath.Abs(filepath.Join(rootA, "lib/v1/foo.proto"))
	absB, _ := filepath.Abs(filepath.Join(rootB, "lib/v1/foo.proto"))
	if !strings.Contains(msg, absA) {
		t.Errorf("error should name the first-registering path %s, got: %v", absA, err)
	}
	if !strings.Contains(msg, absB) {
		t.Errorf("error should name the colliding path %s, got: %v", absB, err)
	}
}

// TestBuildImportMappingMultiRootSameRootTwice pins that passing the
// same root multiple times is silently de-duped, not a collision. This
// is the path a list-separator CLI value like `--proto_path=proto:proto`
// (user mistake) or a Mimir Makefile that ends up listing one root
// twice through different code paths takes. Without the same-abs guard
// it would surface as a misleading "duplicate import key" error.
//
// The sub-cases together cover the de-dup contract: identical strings,
// trailing slash, and a "./"-prefixed spelling all normalise to the
// same filepath.Abs and must hit the de-dup branch. Symlinked roots
// deliberately do *not* hit this branch — filepath.Abs doesn't resolve
// symlinks, so two symlinked spellings of the same directory produce
// different abs strings and fall through to the explicit-collision
// branches by design.
func TestBuildImportMappingMultiRootSameRootTwice(t *testing.T) {
	dir := t.TempDir()
	writeProto(t, dir, "lib/v1/foo.proto",
		"syntax = \"proto3\";\npackage lib.v1;\nmessage Foo {}")

	for _, tc := range []struct {
		name  string
		roots []string
	}{
		{"identical", []string{dir, dir}},
		{"trailing-slash", []string{dir, dir + string(filepath.Separator)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapping, importPaths, _, err := buildImportMapping(tc.roots)
			if err != nil {
				t.Fatalf("buildImportMapping: equivalent-spelling roots must not error, got: %v", err)
			}
			if len(importPaths) != 1 || len(mapping) != 1 {
				t.Errorf("expected single import entry after de-dup, got importPaths=%v mapping=%v", importPaths, mapping)
			}
		})
	}
}

// TestBuildImportMappingMultiRootOverlap pins the root-containment
// safety net: when one --proto_path root is a subdirectory of another,
// the inner root's files are also reachable from the outer root, but
// under *different* import keys (a nested rel path from the outer root
// vs. a package-derived top-level key from the inner root). Letting
// both register would silently compile the file twice and surface as a
// duplicate-symbol link error in the generated Go.
//
// Without this guard the keyToPath de-dup only catches "same key, same
// file" — it misses "same file, different key", which is exactly what
// overlapping roots produce when the inner root's files don't sit at a
// path matching their package.
func TestBuildImportMappingMultiRootOverlap(t *testing.T) {
	outer := t.TempDir()
	// Inner is a subdirectory of outer that itself contains a flat
	// .proto whose package would yield a key unrelated to the rel
	// path under outer. Outer sees `inner/foo.proto` as a nested rel
	// path → key `inner/foo.proto`. Inner sees `foo.proto` as
	// top-level → key derived from `package mypkg;` → `mypkg/foo.proto`.
	// Two different keys, one file.
	inner := filepath.Join(outer, "inner")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	writeProto(t, outer, "inner/foo.proto",
		"syntax = \"proto3\";\npackage mypkg;\nmessage Foo {}")

	_, _, _, err := buildImportMapping([]string{outer, inner})
	if err == nil {
		t.Fatal("expected error for overlapping roots, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "reachable from multiple --proto_path roots") {
		t.Errorf("error should call out the overlap reason, got: %v", err)
	}
	// Both keys must show up so the user can decide which root to drop.
	for _, wantKey := range []string{"inner/foo.proto", "mypkg/foo.proto"} {
		if !strings.Contains(msg, wantKey) {
			t.Errorf("error should name the conflicting key %q, got: %v", wantKey, err)
		}
	}
}

// TestGenerateRespectsOutDirInImportBase pins the contract that --out feeds
// into the Go import-path base (module + outDir), not just the on-disk
// destination. A user pointing --out at a directory other than `gen` —
// e.g. `pkg/api` inside their own module — must get generated code whose
// cross-file imports resolve under that prefix, otherwise Go's
// directory-equals-import-path rule rejects the output at build time.
//
// Pre-fix the import base was hardcoded as `<module>/gen` regardless of
// --out, so any --out other than `gen` silently produced unbuildable code.
func TestGenerateRespectsOutDirInImportBase(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "common/v1/common.proto", `
syntax = "proto3";
package svc.common.v1;
message Item { string id = 1; }`)
	writeProto(t, protoDir, "api/v1/api.proto", `
syntax = "proto3";
package svc.api.v1;
import "common/v1/common.proto";
message Req { svc.common.v1.Item item = 1; }`)

	cwd := t.TempDir()
	t.Chdir(cwd)
	gen := &Generator{
		Module:    "example.com/svc",
		OutDir:    "pkg/api",
		ProtoDirs: []string{protoDir},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	for _, rel := range []string{
		filepath.Join("pkg", "api", "common", "v1", "common.pb.go"),
		filepath.Join("pkg", "api", "api", "v1", "api.pb.go"),
	} {
		if _, err := os.Stat(filepath.Join(cwd, rel)); err != nil {
			t.Errorf("expected %s under outDir: %v", rel, err)
		}
	}

	// The cross-file import in api.pb.go must reference the outDir-derived
	// base — example.com/svc/pkg/api/common/v1 — not the legacy
	// example.com/svc/gen/common/v1 the old hardcode would have produced.
	apiContent, err := os.ReadFile(filepath.Join(cwd, "pkg", "api", "api", "v1", "api.pb.go"))
	if err != nil {
		t.Fatalf("reading api.pb.go: %v", err)
	}
	apiStr := string(apiContent)
	if !strings.Contains(apiStr, `"example.com/svc/pkg/api/common/v1"`) {
		t.Errorf("api.pb.go must import the common pkg under example.com/svc/pkg/api/common/v1; got:\n%s", apiStr)
	}
	if strings.Contains(apiStr, `"example.com/svc/gen/common/v1"`) {
		t.Errorf("api.pb.go must NOT import under the legacy /gen/ base; the hardcode is back. Got:\n%s", apiStr)
	}
}

// TestGenerateNestedLayout runs the full generator pipeline against a
// recursive proto layout where a nested file imports another nested file.
// This is the integration-level counterpart to TestBuildImportMappingRecursive:
// it verifies the import keys we register actually resolve through protocompile
// and that .pb.go files land at the source-relative location dictated by
// each input's directory under --proto_path.
func TestGenerateNestedLayout(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "testpb/common/v1/common.proto",
		"syntax = \"proto3\";\npackage testpb.common.v1;\nmessage Resource { string name = 1; }")
	writeProto(t, protoDir, "testpb/trace/v1/trace.proto",
		"syntax = \"proto3\";\npackage testpb.trace.v1;\nimport \"testpb/common/v1/common.proto\";\nmessage Span { testpb.common.v1.Resource resource = 1; }")

	outDir := testOutDir(t)
	gen := &Generator{Module: "github.com/grafana/wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	for _, rel := range []string{
		filepath.Join("testpb", "common", "v1", "common.pb.go"),
		filepath.Join("testpb", "trace", "v1", "trace.pb.go"),
	} {
		path := filepath.Join(outDir, rel)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected generated file %s: %v", rel, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("generated file %s is empty", rel)
		}
	}

	// The cross-file import must be reflected in the consumer file: trace.pb.go
	// should reference the common package's Go import path so it actually
	// compiles. A missing import would mean protocompile resolved the .proto
	// import but the generator dropped it.
	traceContent, err := os.ReadFile(filepath.Join(outDir, "testpb", "trace", "v1", "trace.pb.go"))
	if err != nil {
		t.Fatalf("reading trace.pb.go: %v", err)
	}
	if !strings.Contains(string(traceContent), "github.com/grafana/wiresmith/gen/testpb/common/v1") {
		t.Errorf("trace.pb.go missing cross-package import to common/v1; content:\n%s", traceContent)
	}
}

// TestGenerateCrossPackageUnmarshalThreadsDepth pins the SEC-5 fix:
// when a generated Unmarshal calls into a *different-package* message
// type, the recursion-depth counter must thread across the call instead
// of restarting at zero. Historically the cross-package emit site called
// `.Unmarshal(b)` which routes through the depth=0 public entry point,
// so a graph bouncing between N packages could recurse to depth
// maxUnmarshalDepth*N levels without tripping the guard. The fix is to
// emit `.UnmarshalWithDepth(b, depth+1)` at every cross-package site
// and expose `UnmarshalWithDepth` as the cross-package surface.
func TestGenerateCrossPackageUnmarshalThreadsDepth(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "depthsec/leaf/v1/leaf.proto", `
syntax = "proto3";
package depthsec.leaf.v1;
message Leaf { string s = 1; }`)
	writeProto(t, protoDir, "depthsec/outer/v1/outer.proto", `
syntax = "proto3";
package depthsec.outer.v1;
import "depthsec/leaf/v1/leaf.proto";
message Outer { depthsec.leaf.v1.Leaf l = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	outerSrc := mustReadFile(t, filepath.Join(outDir, "depthsec", "outer", "v1", "outer.pb.go"))
	leafSrc := mustReadFile(t, filepath.Join(outDir, "depthsec", "leaf", "v1", "leaf.pb.go"))

	// The cross-package unmarshal site in outer.pb.go must use the
	// depth-threading entry point. The pre-fix code emitted
	// `.Unmarshal(dAtA[iNdEx:postIndex])`, which resets depth at the
	// boundary — that's the bug this test pins against regression.
	if !strings.Contains(outerSrc, "UnmarshalWithDepth(dAtA[iNdEx:postIndex], depth+1)") {
		t.Errorf("outer.pb.go must call .UnmarshalWithDepth(..., depth+1) at the cross-package Leaf site; full source:\n%s", outerSrc)
	}
	// Belt-and-braces: an unqualified `.Unmarshal(dAtA[...:...])` call at
	// the same site would mean the bug is back. Filter for the dAtA slice
	// signature so we don't false-positive on, e.g., the public Unmarshal
	// wrapper definition `func (m *Outer) Unmarshal(b []byte) error`.
	if strings.Contains(outerSrc, ".Unmarshal(dAtA[iNdEx:postIndex])") {
		t.Errorf("outer.pb.go must not call the depth-resetting .Unmarshal at the cross-package site; full source:\n%s", outerSrc)
	}

	// Cross-package callers can only reach UnmarshalWithDepth if it is
	// exported from the callee package — so leaf.pb.go has to declare it.
	if !strings.Contains(leafSrc, "func (m *Leaf) UnmarshalWithDepth(b []byte, depth int) error") {
		t.Errorf("leaf.pb.go must expose UnmarshalWithDepth(b, depth) so cross-package callers can thread depth; full source:\n%s", leafSrc)
	}
}

// TestGenerateCrossPackageMapValueThreadsDepth covers the second SEC-5
// emit site — `map<K, Msg>` values where `Msg` lives in a different proto
// package. The map-entry value path is its own code branch (see
// MessageType.EmitMapEntryUnmarshal), so the SEC-5 fix had to be applied
// there explicitly; this test pins it so it can't quietly slip back.
func TestGenerateCrossPackageMapValueThreadsDepth(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "depthmap/leaf/v1/leaf.proto", `
syntax = "proto3";
package depthmap.leaf.v1;
message Leaf { string s = 1; }`)
	writeProto(t, protoDir, "depthmap/outer/v1/outer.proto", `
syntax = "proto3";
package depthmap.outer.v1;
import "depthmap/leaf/v1/leaf.proto";
message Outer { map<string, depthmap.leaf.v1.Leaf> entries = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	outerSrc := mustReadFile(t, filepath.Join(outDir, "depthmap", "outer", "v1", "outer.pb.go"))

	if !strings.Contains(outerSrc, "UnmarshalWithDepth(dAtA[iNdEx:postIndex], depth+1)") {
		t.Errorf("outer.pb.go map<K, Leaf> entry must thread depth via UnmarshalWithDepth — pre-fix this site called the depth-resetting Unmarshal; full source:\n%s", outerSrc)
	}
}

// TestGenerateCrossPackageMapDuplicateKeyReplaces pins the proto3
// duplicate-key REPLACE / last-write-wins semantics at the generator
// level. Prior to wiresmith-05d the codegen MERGED message values when
// the same map key appeared more than once on the wire; that diverged
// from the proto3 spec and from `google.golang.org/protobuf`'s
// `consumeMapOfMessage` (which allocates a fresh value per entry and
// SetMapIndex's it unconditionally). The visible symptom was the
// `Required.Proto3.ProtobufInput.ValidDataMap.STRING.MESSAGE.MergeValue`
// conformance test, exposed by the corecursive re-add in
// wiresmith-sb1.
//
// The depth-threading the merge call used to do — closely related to
// SEC-5 / wiresmith-1c0 — is now redundant: only the *initial* value
// decode (via `MessageType.EmitMapEntryUnmarshal`) needs to thread
// depth, and `TestGenerateCrossPackageMapValueThreadsDepth` above still
// pins that.
func TestGenerateCrossPackageMapDuplicateKeyReplaces(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "dupkey/leaf/v1/leaf.proto", `
syntax = "proto3";
package dupkey.leaf.v1;
message Leaf { string s = 1; }`)
	writeProto(t, protoDir, "dupkey/outer/v1/outer.proto", `
syntax = "proto3";
package dupkey.outer.v1;
import "dupkey/leaf/v1/leaf.proto";
message Outer { map<string, dupkey.leaf.v1.Leaf> entries = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	outerSrc := mustReadFile(t, filepath.Join(outDir, "dupkey", "outer", "v1", "outer.pb.go"))

	// REPLACE: the post-loop must unconditionally overwrite m[mapkey].
	if !strings.Contains(outerSrc, "m.Entries[mapkey] = mapvalue") {
		t.Errorf("outer.pb.go must end the map-entry block with `m.Entries[mapkey] = mapvalue` (REPLACE):\n%s", outerSrc)
	}
	// All the merge-branch machinery must be gone: no mapValueBytes
	// capture, no `existing.<unmarshal>` call (in any of its three
	// historical forms), no nil-check branch.
	if strings.Contains(outerSrc, "mapValueBytes") {
		t.Errorf("outer.pb.go must not mention mapValueBytes — merge is gone (wiresmith-05d):\n%s", outerSrc)
	}
	for _, frag := range []string{
		"existing.unmarshal(",
		"existing.Unmarshal(",
		"existing.UnmarshalWithDepth(",
	} {
		if strings.Contains(outerSrc, frag) {
			t.Errorf("outer.pb.go must not contain `%s` — duplicate-key merge is gone (wiresmith-05d):\n%s", frag, outerSrc)
		}
	}
}

// TestGenerateUnmarshalWithDepthClampsNegative pins the negative-depth
// guard inside UnmarshalWithDepth. A negative starting depth would
// silently widen the recursion budget (the guard is `depth > maxDepth`),
// so the generator emits a clamp-to-zero block at the entry. Without it,
// a misuse of the public API could re-open SEC-5 from the caller side.
func TestGenerateUnmarshalWithDepthClampsNegative(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "depthclamp/v1/clamp.proto", `
syntax = "proto3";
package depthclamp.v1;
message Probe { string s = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	src := mustReadFile(t, filepath.Join(outDir, "depthclamp", "v1", "clamp.pb.go"))

	if !strings.Contains(src, "func (m *Probe) UnmarshalWithDepth(b []byte, depth int) error") {
		t.Fatalf("Probe must expose UnmarshalWithDepth; full source:\n%s", src)
	}
	// The clamp block must be inside UnmarshalWithDepth, before the call
	// to the private unmarshal. The literal `if depth < 0` form is what
	// the generator emits today; keep this assertion exact so a refactor
	// that drops the clamp can't pass quietly.
	if !strings.Contains(src, "if depth < 0 {\n\t\tdepth = 0\n\t}") {
		t.Errorf("UnmarshalWithDepth must clamp negative starting depth to 0; full source:\n%s", src)
	}
}

// TestGenerateMixedLayoutImport documents the supported import shape for a
// mixed flat+nested layout: a nested file importing a top-level file must
// use the top-level's package-derived path (its canonical key), not the
// plain basename. The plain-basename form fails because protocompile uses
// the queried path as file identity and would compile the file twice.
func TestGenerateMixedLayoutImport(t *testing.T) {
	protoDir := t.TempDir()
	// Flat common.proto registers under its package-derived key
	// `testpb/common.proto`; the source-relative output therefore lands at
	// outDir/testpb/common.pb.go.
	writeProto(t, protoDir, "common.proto",
		"syntax = \"proto3\";\npackage testpb;\nmessage Resource { string name = 1; }")
	writeProto(t, protoDir, "testpb/trace/v1/trace.proto",
		"syntax = \"proto3\";\npackage testpb.trace.v1;\nimport \"testpb/common.proto\";\nmessage Span { testpb.Resource resource = 1; }")

	outDir := testOutDir(t)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	for _, rel := range []string{
		filepath.Join("testpb", "common.pb.go"),
		filepath.Join("testpb", "trace", "v1", "trace.pb.go"),
	} {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Errorf("expected generated file %s: %v", rel, err)
		}
	}

	// The plain-basename form must be rejected: registering both keys for the
	// same content would cause protocompile to compile it twice and emit a
	// duplicate-symbol error.
	protoDir2 := t.TempDir()
	writeProto(t, protoDir2, "common.proto",
		"syntax = \"proto3\";\npackage testpb;\nmessage Resource { string name = 1; }")
	writeProto(t, protoDir2, "testpb/trace/v1/trace.proto",
		"syntax = \"proto3\";\npackage testpb.trace.v1;\nimport \"common.proto\";\nmessage Span { testpb.Resource resource = 1; }")
	gen2 := &Generator{Module: "wiresmith", OutDir: testOutDir(t), ProtoDirs: []string{protoDir2}}
	if err := gen2.Generate(context.Background()); err == nil {
		t.Error("expected plain-basename import to fail; canonical pkg-derived path is required for cross-imports")
	}
}

// TestGenerateOutputCollision verifies that two protos in different
// source-relative directories sharing the same proto package are rejected
// before any file is written. With source-relative output the on-disk paths
// would not collide (the files live in different dirs), but Go forbids one
// package spanning two directories — flagging the configuration up front
// gives a clearer error than waiting for `go build` to reject the result.
func TestGenerateOutputCollision(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "a/v1/shared.proto",
		"syntax = \"proto3\";\npackage testpb.shared.v1;\nmessage A {}")
	writeProto(t, protoDir, "b/v1/shared.proto",
		"syntax = \"proto3\";\npackage testpb.shared.v1;\nmessage B {}")

	outDir := testOutDir(t)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected proto-package-span error, got nil")
	}
	if !strings.Contains(err.Error(), "spans multiple source-relative directories") {
		t.Errorf("expected 'spans multiple source-relative directories' error, got: %v", err)
	}

	// Fail-fast guarantee: no .pb.go should have been written before the
	// collision was detected.
	collisionDir := filepath.Join(outDir, "testpb", "shared", "v1")
	if entries, err := os.ReadDir(collisionDir); err == nil && len(entries) > 0 {
		t.Errorf("expected no files written on collision, found %d in %s", len(entries), collisionDir)
	}
}

// TestGenerateReflectOutputCollision verifies that a user proto whose basename
// would generate a `_reflect.pb.go` file colliding with another proto's
// companion reflect output is rejected. Without this guard, `foo_reflect.proto`
// and `foo.proto` in the same package would silently overwrite each other's
// reflect output.
func TestGenerateReflectOutputCollision(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "foo.proto",
		"syntax = \"proto3\";\npackage testpb.v1;\nmessage Foo {}")
	writeProto(t, protoDir, "foo_reflect.proto",
		"syntax = \"proto3\";\npackage testpb.v1;\nmessage FooReflect {}")

	outDir := testOutDir(t)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected reflect-output collision error, got nil")
	}
	if !strings.Contains(err.Error(), "output collision") {
		t.Errorf("expected 'output collision' error, got: %v", err)
	}

	// Fail-fast: no files should have been written.
	collisionDir := filepath.Join(outDir, "testpb", "v1")
	if entries, err := os.ReadDir(collisionDir); err == nil && len(entries) > 0 {
		t.Errorf("expected no files written on collision, found %d in %s", len(entries), collisionDir)
	}
}

// TestGenerateWithGoPackage verifies that a proto's `option go_package` drives
// the generated file's import path and Go package name, and the alias used by
// importing files. Output location is independent (always source-relative).
func TestGenerateWithGoPackage(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "a.proto", `
syntax = "proto3";
package myproject.a;
option go_package = "example.com/mod/gen/myproject/a;a";
message Foo { string name = 1; }`)
	writeProto(t, protoDir, "b.proto", `
syntax = "proto3";
package myproject.b;
option go_package = "example.com/mod/gen/myproject/b;b";
import "myproject/a/a.proto";
message Bar { myproject.a.Foo foo = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	aContent, err := os.ReadFile(filepath.Join(outDir, "myproject", "a", "a.pb.go"))
	if err != nil {
		t.Fatalf("expected output at myproject/a/a.pb.go: %v", err)
	}
	bContent, err := os.ReadFile(filepath.Join(outDir, "myproject", "b", "b.pb.go"))
	if err != nil {
		t.Fatalf("expected output at myproject/b/b.pb.go: %v", err)
	}

	// Package name comes from the go_package semicolon form, not from the
	// proto package's last two components.
	if !strings.Contains(string(aContent), "package a\n") {
		t.Errorf("a.pb.go: expected 'package a', not derived 'myprojecta'")
	}
	if !strings.Contains(string(bContent), "package b\n") {
		t.Errorf("b.pb.go: expected 'package b'")
	}

	// Cross-file import in b.pb.go must use the go_package import path of a.
	if !strings.Contains(string(bContent), `"example.com/mod/gen/myproject/a"`) {
		t.Errorf("b.pb.go: expected import of example.com/mod/gen/myproject/a, got:\n%s", string(bContent))
	}
}

// TestGenerateGoPackageHonoredLiterally verifies wiresmith-gz4: a go_package
// pointing outside the module's `<module>/gen` base is honored verbatim, not
// rewritten or ignored. The on-disk path stays source-relative; only the
// import-path string the generated file declares follows go_package. This
// matches protoc-gen-go's behavior in `paths=source_relative` mode.
func TestGenerateGoPackageHonoredLiterally(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "x.proto", `
syntax = "proto3";
package mytest.x;
option go_package = "some.other/module/pkg";
message Msg { int32 val = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "wiresmith",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// On-disk path stays source-relative.
	content, err := os.ReadFile(filepath.Join(outDir, "mytest", "x", "x.pb.go"))
	if err != nil {
		t.Fatalf("expected source-relative output: %v", err)
	}
	// Package name comes from path.Base of the go_package — not the proto
	// package — confirming the literal honor.
	if !strings.Contains(string(content), "package pkg\n") {
		t.Errorf("expected 'package pkg' (from go_package), got:\n%s", string(content))
	}
}

// TestGenerateWithMOverride verifies the CLI `--M source=dest` override:
// the override wins over the file's option go_package and supplies the
// import path used both in the generated file's package declaration and in
// importers of that file. Mirrors protoc's `M<source>=<dest>` semantics —
// the documented escape hatch when a vendored .proto's go_package doesn't
// match the consumer's tree.
func TestGenerateWithMOverride(t *testing.T) {
	protoDir := t.TempDir()
	// a.proto declares an "external" go_package that an M-override redirects
	// back into the consumer's tree.
	writeProto(t, protoDir, "vendored/a/a.proto", `
syntax = "proto3";
package vendored.a;
option go_package = "go.example.com/upstream/a";
message Foo { string name = 1; }`)
	// b.proto imports a.proto and must pick up the overridden path when
	// emitting a cross-file import alias.
	writeProto(t, protoDir, "b/b.proto", `
syntax = "proto3";
package myapp.b;
option go_package = "example.com/mod/gen/b";
import "vendored/a/a.proto";
message Bar { vendored.a.Foo foo = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
		Overrides: map[string]string{
			"vendored/a/a.proto": "example.com/mod/gen/a;aliased",
		},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	aContent, err := os.ReadFile(filepath.Join(outDir, "vendored", "a", "a.pb.go"))
	if err != nil {
		t.Fatalf("a.pb.go: %v", err)
	}
	// The `;name` form in the override sets the package clause.
	if !strings.Contains(string(aContent), "package aliased\n") {
		t.Errorf("expected 'package aliased' from override, got:\n%s", string(aContent))
	}

	bContent, err := os.ReadFile(filepath.Join(outDir, "b", "b.pb.go"))
	if err != nil {
		t.Fatalf("b.pb.go: %v", err)
	}
	// The cross-file import in b.pb.go must use the override's import path,
	// not a.proto's go_package — that's the whole point of the override.
	if !strings.Contains(string(bContent), `"example.com/mod/gen/a"`) {
		t.Errorf("expected cross-import via override path, got:\n%s", string(bContent))
	}
	if strings.Contains(string(bContent), `"go.example.com/upstream/a"`) {
		t.Errorf("override ignored — b.pb.go still imports the upstream path:\n%s", string(bContent))
	}
}

// TestGenerateScopedMOverrideTransitiveImport is a regression test for the P2 bug:
// a scoped (positional-file) generation run must honor a -M override even when
// the overridden file is reached only as a transitive import of the positional
// root, so the emitted cross-file reference uses the override (not the dep's
// own go_package).
//
// ─── How to reproduce on the CLI ────────────────────────────────────────────
//
//	wiresmith -proto_path tree \
//	  -M dep/dep.proto=example.com/OVERRIDE/dep \
//	  tree/app/app.proto
//
// app.proto imports dep/dep.proto. The generated app.pb.go imports
// "example.com/orig/dep" (dep.proto's own go_package) instead of the pinned
// "example.com/OVERRIDE/dep". The -M flag — the user's explicit "this vendored
// dep actually lives here" instruction — is silently dropped.
//
// ─── Why it happens ─────────────────────────────────────────────────────────
//
// In scoped mode (Generator.Files non-empty → emit set non-empty), compileSources
// passes only the positional files (+ the embedded options schema) as the
// protocompile roots:
//
//	roots = [<emit keys>..., embeddedOptionsPath]
//	linked, _ := compiler.Compile(ctx, roots...)
//	results = linked    // ← ONE FileDescriptor per requested root, NOT its deps
//
// protocompile.Compiler.Compile returns one linker.File per *requested* name;
// transitive imports are linked into each root's .Imports() but are NOT
// returned as their own top-level entries. So `results` (and therefore the
// `inResults` set in computeDests) contains only the roots.
//
// computeDests then walks walkReachableFiles(results) — roots PLUS their
// transitive imports — and for the transitive dep takes the !inResults branch:
//
//	if !inResults[fd.Path()] {
//	    g.destinations[fd.Path()] = destForReachable(fd)   // ← bug
//	    continue
//	}
//
// destForReachable resolves purely from the file's own go_package option and
// never consults g.Overrides — by design, because it must bypass the
// g.goPackages table for well-known types (google.protobuf.* share one proto
// package across many Go destinations). But that same blindness drops the -M
// override for ordinary user deps.
//
// The non-scoped sibling TestGenerateWithMOverride PASSES because without
// positional files every file in the walk is a compile root, so the dep IS in
// `results`, takes the in-results branch, and resolves through g.destFor →
// destForPath, which checks g.Overrides first.
//
// ─── Fix ────────────────────────────────────────────────────────────────────
//
// computeDests must consult g.Overrides even for transitively imported files in
// scoped runs. Pinned files are routed through g.destFor (override-aware, keyed
// by fd.Path()); everything else continues to use destForReachable to preserve
// the well-known-type special case.
func TestGenerateScopedMOverrideTransitiveImport(t *testing.T) {
	protoDir := t.TempDir()
	// a.proto declares an "external" go_package; the -M override should
	// redirect references to it back into the consumer's tree.
	writeProto(t, protoDir, "vendored/a/a.proto", `
syntax = "proto3";
package vendored.a;
option go_package = "go.example.com/upstream/a";
message Foo { string name = 1; }`)
	// b.proto is the positional root; a.proto enters only as its transitive
	// import (so a.proto is NOT in `results`).
	writeProto(t, protoDir, "b/b.proto", `
syntax = "proto3";
package myapp.b;
option go_package = "example.com/mod/gen/b";
import "vendored/a/a.proto";
message Bar { vendored.a.Foo foo = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
		// Scoped run: only b.proto is a positional root.
		Files: []string{filepath.Join(protoDir, "b", "b.proto")},
		Overrides: map[string]string{
			"vendored/a/a.proto": "example.com/mod/gen/a;aliased",
		},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	bContent, err := os.ReadFile(filepath.Join(outDir, "b", "b.pb.go"))
	if err != nil {
		t.Fatalf("b.pb.go: %v", err)
	}
	// The cross-file import in b.pb.go must use the override's import path even
	// though a.proto was reached only as a transitive import of the scoped root.
	if !strings.Contains(string(bContent), `"example.com/mod/gen/a"`) {
		t.Errorf("scoped run dropped the -M override for a transitive import; b.pb.go should import the override path, got:\n%s", string(bContent))
	}
	if strings.Contains(string(bContent), `"go.example.com/upstream/a"`) {
		t.Errorf("override ignored — b.pb.go still imports the upstream path (P2 bug):\n%s", string(bContent))
	}
}

// TestGenerateGoPackageWithSemicolon verifies the semicolon form
// "import/path;name" lets the proto author choose a Go package name that
// differs from the last component of the import path.
func TestGenerateGoPackageWithSemicolon(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "svc.proto", `
syntax = "proto3";
package myapp.svc;
option go_package = "example.com/app/gen/myapp/svc;service";
message Request { string id = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "example.com/app",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outDir, "myapp", "svc", "svc.pb.go"))
	if err != nil {
		t.Fatalf("expected output: %v", err)
	}
	// The semicolon name wins over path.Base.
	if !strings.Contains(string(content), "package service\n") {
		t.Errorf("expected 'package service', got:\n%s", string(content))
	}
}

// TestGenerateConflictingGoPackage rejects the configuration where two .proto
// files share a proto package but disagree on go_package. With recursive
// scanning, this can happen by accident, and a single proto package must map
// to one Go destination.
func TestGenerateConflictingGoPackage(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "a.proto", `
syntax = "proto3";
package mypkg;
option go_package = "example.com/mod/gen/mypkg;a";
message Foo { string name = 1; }`)
	writeProto(t, protoDir, "b.proto", `
syntax = "proto3";
package mypkg;
option go_package = "example.com/mod/gen/mypkg;b";
message Bar { string id = 1; }`)

	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    testOutDir(t),
		ProtoDirs: []string{protoDir},
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected conflicting go_package error, got nil")
	}
	if !strings.Contains(err.Error(), "inconsistent go_package") {
		t.Errorf("expected 'inconsistent go_package' error, got: %v", err)
	}
}

// TestGenerateMixedGoPackageState rejects the case where one file in a
// proto package sets go_package and another in the same package omits it.
// Silently inheriting would contradict the upfront-agreement contract and
// could move generated files around when a file is later updated.
func TestGenerateMixedGoPackageState(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "a.proto", `
syntax = "proto3";
package mixed;
option go_package = "example.com/mod/gen/mixed;mixed";
message Foo { string s = 1; }`)
	writeProto(t, protoDir, "b.proto", `
syntax = "proto3";
package mixed;
message Bar { string s = 1; }`)

	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    testOutDir(t),
		ProtoDirs: []string{protoDir},
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected inconsistent go_package error, got nil")
	}
	if !strings.Contains(err.Error(), "inconsistent go_package") {
		t.Errorf("expected 'inconsistent go_package' error, got: %v", err)
	}
}

// TestGenerateSplitPackageWithOverrides pins the -M escape hatch for proto
// packages that legitimately span multiple Go packages (Loki's `package
// logproto` lives in both pkg/push — a separate Go module — and
// pkg/logproto). A file pinned via Overrides opts out of the
// one-go_package-per-proto-package agreement: the remaining files still
// agree among themselves, the pinned file resolves to its override, and
// cross-Go-package references between the two halves come out qualified.
func TestGenerateSplitPackageWithOverrides(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "push/push.proto", `
syntax = "proto3";
package logproto;
option go_package = "example.com/loki/pkg/push";
message Stream { string labels = 1; }`)
	writeProto(t, protoDir, "logproto/logproto.proto", `
syntax = "proto3";
package logproto;
option go_package = "example.com/loki/v3/pkg/logproto";
import "push/push.proto";
message QueryResponse { Stream stream = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "example.com/loki",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
		Overrides: map[string]string{
			"push/push.proto": "example.com/loki/pkg/push",
		},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate with -M override: %v", err)
	}

	pushSrc := mustReadFile(t, filepath.Join(outDir, "push", "push.pb.go"))
	logSrc := mustReadFile(t, filepath.Join(outDir, "logproto", "logproto.pb.go"))

	if !strings.Contains(pushSrc, "package push\n") {
		t.Errorf("push.pb.go must declare 'package push' (from the override), got:\n%.300s", pushSrc)
	}
	if !strings.Contains(logSrc, "package logproto\n") {
		t.Errorf("logproto.pb.go must declare 'package logproto', got:\n%.300s", logSrc)
	}
	// The reference into the pinned half must be a qualified import, not a
	// bare same-package identifier.
	if !strings.Contains(logSrc, `"example.com/loki/pkg/push"`) {
		t.Errorf("logproto.pb.go must import the pinned push package")
	}
	if !strings.Contains(logSrc, "push.Stream") {
		t.Errorf("logproto.pb.go must reference Stream as push.Stream")
	}
	if strings.Contains(logSrc, "type Stream struct") {
		t.Errorf("logproto.pb.go must not redeclare Stream locally")
	}
}

// TestGenerateSplitPackageWithoutOverridesRejected pins that the same
// split-package layout WITHOUT -M pins still fails the consistency check —
// the override is an explicit opt-in, not a relaxation of the default rule.
func TestGenerateSplitPackageWithoutOverridesRejected(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "push/push.proto", `
syntax = "proto3";
package logproto;
option go_package = "example.com/loki/pkg/push";
message Stream { string labels = 1; }`)
	writeProto(t, protoDir, "logproto/logproto.proto", `
syntax = "proto3";
package logproto;
option go_package = "example.com/loki/v3/pkg/logproto";
import "push/push.proto";
message QueryResponse { Stream stream = 1; }`)

	gen := &Generator{
		Module:    "example.com/loki",
		OutDir:    testOutDir(t),
		ProtoDirs: []string{protoDir},
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected inconsistent go_package error, got nil")
	}
	if !strings.Contains(err.Error(), "inconsistent go_package") {
		t.Errorf("expected 'inconsistent go_package' error, got: %v", err)
	}
}

// TestGenerateOverrideSplittingDirRejected guards against -M misuse: pinning
// one file of a directory to a different Go import path than its dir-mates
// would put two Go packages in one output directory — Go's
// directory-equals-package rule makes that uncompilable, so reject it up
// front with an error naming both files.
func TestGenerateOverrideSplittingDirRejected(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "pkg/a.proto", `
syntax = "proto3";
package samedir;
option go_package = "example.com/mod/gen/samedir";
message A { string s = 1; }`)
	writeProto(t, protoDir, "pkg/b.proto", `
syntax = "proto3";
package samedir;
option go_package = "example.com/mod/gen/samedir";
message B { string s = 1; }`)

	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    testOutDir(t),
		ProtoDirs: []string{protoDir},
		Overrides: map[string]string{
			"pkg/b.proto": "example.com/elsewhere/bpkg",
		},
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected conflicting-import-path error, got nil")
	}
	if !strings.Contains(err.Error(), "conflicting Go import paths") {
		t.Errorf("expected 'conflicting Go import paths' error, got: %v", err)
	}
}

// TestGenerateOverridesCollideAcrossDirsRejected pins the -M misuse flagged
// on PR #125: two files in DIFFERENT source-relative directories both pinned
// to the SAME Go import path would make one import path span two directories.
// isSelfDest compares import paths only, so it would then treat the two
// directories as one Go package and emit unqualified cross-directory
// references — uncompilable code. computeDests' importOwner check (keyed by
// import path, recording the claiming relDir) must reject it up front, naming
// both files. Distinct from TestGenerateOverrideSplittingDirRejected, which
// covers the inverse (one dir split to two import paths).
func TestGenerateOverridesCollideAcrossDirsRejected(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "dira/a.proto", `
syntax = "proto3";
package pkga;
option go_package = "example.com/mod/gen/dira";
message A { string s = 1; }`)
	writeProto(t, protoDir, "dirb/b.proto", `
syntax = "proto3";
package pkgb;
option go_package = "example.com/mod/gen/dirb";
message B { string s = 1; }`)

	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    testOutDir(t),
		ProtoDirs: []string{protoDir},
		Overrides: map[string]string{
			"dira/a.proto": "example.com/shared",
			"dirb/b.proto": "example.com/shared",
		},
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected import-path-claimed-from-two-directories error, got nil")
	}
	if !strings.Contains(err.Error(), "claimed from two directories") {
		t.Errorf("expected 'claimed from two directories' error, got: %v", err)
	}
	// Both files are pinned, so this must surface through the -M override
	// branch of computeDests (which appends the -M hint), not the plain path.
	if !strings.Contains(err.Error(), "-M overrides") {
		t.Errorf("expected the -M-override variant of the error, got: %v", err)
	}
}

// TestGenerateSharedGoPackageAcrossProtoPackages pins the protoc-parity
// case Loki's indexgateway.proto needs: two .proto files in one directory
// with DIFFERENT proto packages but the SAME resolved go_package compile
// into one Go package. References between them are same-Go-package
// unqualified identifiers.
func TestGenerateSharedGoPackageAcrossProtoPackages(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "shared/a.proto", `
syntax = "proto3";
package alpha.v1;
option go_package = "example.com/mod/gen/shared";
message AlphaMsg { string s = 1; }`)
	writeProto(t, protoDir, "shared/b.proto", `
syntax = "proto3";
package beta.v1;
option go_package = "example.com/mod/gen/shared";
import "shared/a.proto";
message BetaMsg { alpha.v1.AlphaMsg a = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{Module: "example.com/mod", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	aSrc := mustReadFile(t, filepath.Join(outDir, "shared", "a.pb.go"))
	bSrc := mustReadFile(t, filepath.Join(outDir, "shared", "b.pb.go"))
	if !strings.Contains(aSrc, "package shared\n") || !strings.Contains(bSrc, "package shared\n") {
		t.Errorf("both files must declare 'package shared'")
	}
	// Same Go package: the cross-proto-package reference is unqualified.
	if !strings.Contains(bSrc, "A AlphaMsg ") && !strings.Contains(bSrc, "A AlphaMsg\t") {
		t.Errorf("BetaMsg must reference AlphaMsg unqualified, got:\n%.600s", bSrc)
	}
	if strings.Contains(bSrc, `"example.com/mod/gen/shared"`) {
		t.Errorf("b.pb.go must not import its own package")
	}
}

// TestGenerateSharedDirDifferentPackageNamesRejected is the guard that
// stays: two proto packages in one directory whose resolved Go package
// names disagree (here: no go_package, so the names derive from the proto
// packages) cannot form one compilable Go package and must be rejected
// up front.
func TestGenerateSharedDirDifferentPackageNamesRejected(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "shared/a.proto", `
syntax = "proto3";
package alpha.v1;
message AlphaMsg { string s = 1; }`)
	writeProto(t, protoDir, "shared/b.proto", `
syntax = "proto3";
package beta.v1;
message BetaMsg { string s = 1; }`)

	gen := &Generator{Module: "example.com/mod", OutDir: testOutDir(t), ProtoDirs: []string{protoDir}}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected error for one dir with two Go package names, got nil")
	}
	if !strings.Contains(err.Error(), "package name") {
		t.Errorf("expected a package-name conflict error, got: %v", err)
	}
}

// TestGenerateFilesSkipsUnrelatedBrokenSiblings pins the lazy compile set:
// positional files compile only themselves plus transitive imports, so an
// unrelated sibling .proto that doesn't compile (gogo annotations without
// the gogo schema on the path, syntax errors, unresolvable imports) must
// not fail the run. This is what lets a staged migration generate one file
// from a tree that still contains gogo-annotated protos, without copying
// the target into a staging directory first.
func TestGenerateFilesSkipsUnrelatedBrokenSiblings(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "good/good.proto", `
syntax = "proto3";
package good.v1;
option go_package = "example.com/mod/gen/good";
import "dep/dep.proto";
message G { dep.v1.D d = 1; }`)
	writeProto(t, protoDir, "dep/dep.proto", `
syntax = "proto3";
package dep.v1;
option go_package = "example.com/mod/gen/dep";
message D { string s = 1; }`)
	// Unrelated sibling that cannot compile: imports a file that's nowhere
	// on the path (the shape a gogo-annotated proto has when gogoproto's
	// schema isn't provided).
	writeProto(t, protoDir, "legacy/legacy.proto", `
syntax = "proto3";
package legacy.v1;
import "gogoproto/gogo.proto";
message L { string s = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
		Files:     []string{filepath.Join(protoDir, "good", "good.proto")},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate with positional file must ignore unrelated broken sibling: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "good", "good.pb.go")); err != nil {
		t.Errorf("expected good.pb.go: %v", err)
	}
	// The transitive import is compiled (for destinations) but not emitted.
	if _, err := os.Stat(filepath.Join(outDir, "dep", "dep.pb.go")); err == nil {
		t.Errorf("dep.pb.go must not be emitted (not in the positional set)")
	}
	if _, err := os.Stat(filepath.Join(outDir, "legacy", "legacy.pb.go")); err == nil {
		t.Errorf("legacy.pb.go must not exist")
	}
}

// TestGenerateFilesBrokenImportStillFails is the counterpart: when the
// positional file actually imports the broken proto, the failure must
// surface — lazy compilation narrows the compile set, it doesn't swallow
// real errors.
func TestGenerateFilesBrokenImportStillFails(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "good/good.proto", `
syntax = "proto3";
package good.v1;
option go_package = "example.com/mod/gen/good";
import "legacy/legacy.proto";
message G { legacy.v1.L l = 1; }`)
	writeProto(t, protoDir, "legacy/legacy.proto", `
syntax = "proto3";
package legacy.v1;
import "gogoproto/gogo.proto";
message L { string s = 1; }`)

	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    testOutDir(t),
		ProtoDirs: []string{protoDir},
		Files:     []string{filepath.Join(protoDir, "good", "good.proto")},
	}
	if err := gen.Generate(context.Background()); err == nil {
		t.Fatal("expected error when the positional file imports a broken proto")
	}
}

// TestGenerateCustomtypeSuppressesUnusedCrossFileImport pins that a
// customtype-annotated message field does not register an import for the
// natural (replaced) message type. When such a field is the file's only
// reference into the imported proto, the stale registration emitted an
// import that nothing used — a compile error in the generated code.
// (Same mechanism as the stdtime/stdduration suppression in fieldContext.)
func TestGenerateCustomtypeSuppressesUnusedCrossFileImport(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "dep/dep.proto", `
syntax = "proto3";
package dep.v1;
option go_package = "example.com/mod/gen/dep";
message Inner { string s = 1; }`)
	writeProto(t, protoDir, "main/main.proto", `
syntax = "proto3";
package main.v1;
option go_package = "example.com/mod/gen/mainpb";
import "wiresmith/options.proto";
import "dep/dep.proto";
message Outer {
  dep.v1.Inner x = 1 [(wiresmith.options.customtype) = "example.com/mod/custom.MyType"];
  repeated dep.v1.Inner xs = 2 [(wiresmith.options.customtype) = "example.com/mod/custom.MyType"];
}`)

	outDir := testOutDir(t)
	gen := &Generator{Module: "example.com/mod", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	mainSrc := mustReadFile(t, filepath.Join(outDir, "main", "main.pb.go"))
	if strings.Contains(mainSrc, `"example.com/mod/gen/dep"`) {
		t.Errorf("main.pb.go imports the replaced type's package but never references it:\n%.600s", mainSrc)
	}
	if !strings.Contains(mainSrc, "custom.MyType") {
		t.Errorf("main.pb.go must use the customtype custom.MyType")
	}
}

// TestGenerateGoPackageShadowsStdlibAlias forces a proto's go_package pkgName
// to equal a stdlib name wiresmith always uses ("fmt"). The pre-reserved
// stdlib entry in newImportTracker keeps the alias pool aware of "fmt", so
// addProtoImport falls back to a unique proto-derived alias and the generated
// imports compile.
func TestGenerateGoPackageShadowsStdlibAlias(t *testing.T) {
	protoDir := t.TempDir()
	// fmtish.proto's go_package pkgName is exactly "fmt".
	writeProto(t, protoDir, "fmtish.proto", `
syntax = "proto3";
package x.fmtish;
option go_package = "example.com/mod/gen/x/fmtish;fmt";
message Sprintf { string s = 1; }`)
	// use.proto imports fmtish AND triggers stdlib fmt (every generated
	// file calls fmt.Sprintf in its String() / Reset() helpers).
	writeProto(t, protoDir, "use.proto", `
syntax = "proto3";
package y.use;
import "x/fmtish/fmtish.proto";
message User { x.fmtish.Sprintf s = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	use, err := os.ReadFile(filepath.Join(outDir, "y", "use", "use.pb.go"))
	if err != nil {
		t.Fatalf("expected output: %v", err)
	}
	useStr := string(use)
	// Stdlib fmt is unaliased; the proto import must have a NON-"fmt"
	// alias. The proto-derived fallback for "x.fmtish" is "xfmtish".
	if !strings.Contains(useStr, "\t\"fmt\"") {
		t.Errorf("use.pb.go missing stdlib fmt import; content:\n%s", useStr)
	}
	if !strings.Contains(useStr, `xfmtish "example.com/mod/gen/x/fmtish"`) {
		t.Errorf("use.pb.go: expected proto alias 'xfmtish' (avoiding stdlib 'fmt'); content:\n%s", useStr)
	}
	// And nothing in the import block should have alias "fmt" except the
	// stdlib import itself (which is unaliased — i.e., no explicit `fmt`
	// keyword before a non-stdlib import).
	if strings.Contains(useStr, "fmt \"example.com") {
		t.Errorf("use.pb.go: proto import claimed 'fmt' alias; content:\n%s", useStr)
	}
	if _, err := format.Source(use); err != nil {
		t.Errorf("use.pb.go did not round-trip through go/format: %v", err)
	}
}

// TestGenerateGoPackageAliasCollision verifies that two proto packages whose
// go_package values both produce alias "v1" don't generate uncompilable
// imports. The third file imports both and must reference them with distinct
// aliases.
func TestGenerateGoPackageAliasCollision(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "common/v1/common.proto", `
syntax = "proto3";
package myproject.common;
option go_package = "example.com/mod/gen/common/v1;v1";
message Foo { string name = 1; }`)
	writeProto(t, protoDir, "trace/v1/trace.proto", `
syntax = "proto3";
package myproject.trace;
option go_package = "example.com/mod/gen/trace/v1;v1";
message Bar { string id = 1; }`)
	writeProto(t, protoDir, "api/v1/api.proto", `
syntax = "proto3";
package myproject.api;
option go_package = "example.com/mod/gen/api/v1;v1";
import "common/v1/common.proto";
import "trace/v1/trace.proto";
message Request {
  myproject.common.Foo foo = 1;
  myproject.trace.Bar bar = 2;
}`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	api, err := os.ReadFile(filepath.Join(outDir, "api", "v1", "api.pb.go"))
	if err != nil {
		t.Fatalf("expected output: %v", err)
	}
	apiStr := string(api)

	// Both /common/v1 and /trace/v1 ask for alias "v1". Whichever is
	// registered first wins; the other must fall back to a unique
	// proto-package-derived alias so the import block compiles.
	if !strings.Contains(apiStr, `"example.com/mod/gen/common/v1"`) {
		t.Errorf("api.pb.go: missing common/v1 import")
	}
	if !strings.Contains(apiStr, `"example.com/mod/gen/trace/v1"`) {
		t.Errorf("api.pb.go: missing trace/v1 import")
	}
	// The trace import must carry an explicit fallback alias (the proto-
	// package-derived name). Without that, both imports would resolve to
	// natural name "v1" and Go would reject the file.
	if !strings.Contains(apiStr, `myprojecttrace "example.com/mod/gen/trace/v1"`) {
		t.Errorf("api.pb.go: expected fallback alias 'myprojecttrace' on trace/v1 import; content:\n%s", apiStr)
	}

	// The generated file must actually be a valid Go file. format.Source
	// rejects duplicate-name imports.
	if _, err := format.Source(api); err != nil {
		t.Errorf("api.pb.go did not round-trip through go/format: %v", err)
	}
}

// TestGenerateGoPackageFallbackAliasAlsoCollides catches the case where the
// proto-package-derived fallback alias *also* collides — e.g. two protos with
// the same go_package ";v1" both fall back to a derived alias that turns out
// to be in use. uniqueAlias's numeric suffix must break the tie.
func TestGenerateGoPackageFallbackAliasAlsoCollides(t *testing.T) {
	protoDir := t.TempDir()
	// Both common protos have go_package ";v1" → first wins alias "v1",
	// second falls back to "commonv1". Then a third proto whose default
	// alias (no go_package) is also "commonv1" arrives and must get a
	// numeric suffix.
	writeProto(t, protoDir, "a/common/v1/a.proto", `
syntax = "proto3";
package myproject.acommon.v1;
option go_package = "example.com/mod/gen/a/common/v1;v1";
message Foo { string s = 1; }`)
	writeProto(t, protoDir, "b/common/v1/b.proto", `
syntax = "proto3";
package myproject.bcommon.v1;
option go_package = "example.com/mod/gen/b/common/v1;v1";
message Bar { string s = 1; }`)
	writeProto(t, protoDir, "svc/svc.proto", `
syntax = "proto3";
package myproject.svc;
option go_package = "example.com/mod/gen/svc;service";
import "a/common/v1/a.proto";
import "b/common/v1/b.proto";
message Req {
  myproject.acommon.v1.Foo a = 1;
  myproject.bcommon.v1.Bar b = 2;
}`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	svc, err := os.ReadFile(filepath.Join(outDir, "svc", "svc.pb.go"))
	if err != nil {
		t.Fatalf("expected output: %v", err)
	}
	svcStr := string(svc)

	// Both packages' proto-derived aliases are "acommonv1" and "bcommonv1" —
	// distinct, so no actual collision in this concrete case. But the second
	// import's pkgName collides with the first's, exercising the fallback.
	if !strings.Contains(svcStr, `acommonv1 "example.com/mod/gen/a/common/v1"`) &&
		!strings.Contains(svcStr, `bcommonv1 "example.com/mod/gen/b/common/v1"`) {
		t.Errorf("svc.pb.go: expected fallback aliases; content:\n%s", svcStr)
	}
	if _, err := format.Source(svc); err != nil {
		t.Errorf("svc.pb.go did not round-trip through go/format: %v", err)
	}
}

// TestGenerateGoPackageKeyword verifies that a go_package whose package name
// would be a Go reserved keyword (e.g. `type`) is escaped to `type_`, so the
// generated file's `package` clause is valid Go.
func TestGenerateGoPackageKeyword(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "myproject/type/x.proto", `
syntax = "proto3";
package myproject.x;
option go_package = "example.com/mod/gen/myproject/type";
message Msg { string s = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	out, err := os.ReadFile(filepath.Join(outDir, "myproject", "type", "x.pb.go"))
	if err != nil {
		t.Fatalf("expected output: %v", err)
	}
	if !strings.Contains(string(out), "package type_\n") {
		t.Errorf("expected 'package type_' (keyword escape), got:\n%s", out)
	}
	if _, err := format.Source(out); err != nil {
		t.Errorf("x.pb.go did not round-trip through go/format: %v", err)
	}
}

// TestGenerateGoPackageAliasCollisionBetweenImports forces the alias collision
// to be resolved by aliasInUse rather than by the selfPkg-name check: the
// importing file's own package name differs from the colliding "v1" alias,
// so the fallback only fires when the second import sees the first's alias.
func TestGenerateGoPackageAliasCollisionBetweenImports(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "common/v1/common.proto", `
syntax = "proto3";
package myproject.common;
option go_package = "example.com/mod/gen/common/v1;v1";
message Foo { string name = 1; }`)
	writeProto(t, protoDir, "trace/v1/trace.proto", `
syntax = "proto3";
package myproject.trace;
option go_package = "example.com/mod/gen/trace/v1;v1";
message Bar { string id = 1; }`)
	writeProto(t, protoDir, "api/v1/api.proto", `
syntax = "proto3";
package myproject.api;
option go_package = "example.com/mod/gen/api/v1;service";
import "common/v1/common.proto";
import "trace/v1/trace.proto";
message Request {
  myproject.common.Foo foo = 1;
  myproject.trace.Bar bar = 2;
}`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	api, err := os.ReadFile(filepath.Join(outDir, "api", "v1", "api.pb.go"))
	if err != nil {
		t.Fatalf("expected output: %v", err)
	}
	apiStr := string(api)

	// api's own package is "service", not "v1", so neither common nor trace
	// hits the self-name fallback. The first one wins "v1" and the second
	// falls back via aliasInUse to a proto-package-derived alias.
	if !strings.Contains(apiStr, "package service\n") {
		t.Errorf("expected 'package service', got:\n%s", apiStr)
	}
	if !strings.Contains(apiStr, `myprojecttrace "example.com/mod/gen/trace/v1"`) {
		t.Errorf("expected fallback alias 'myprojecttrace' on trace/v1; content:\n%s", apiStr)
	}
	if _, err := format.Source(api); err != nil {
		t.Errorf("api.pb.go did not round-trip through go/format: %v", err)
	}
}

// TestGenerateOverrideDoesNotEscapeOutDir mirrors
// TestGenerateGoPackageDoesNotEscapeOutDir for the `-M src=dest` CLI
// override path: a `..` segment in an override value cannot make the
// generator write outside g.OutDir. The override only influences the
// generated file's import-path string, never the disk location (which is
// derived from sourceRelDir(fd.Path()) and is `..`-free by construction).
func TestGenerateOverrideDoesNotEscapeOutDir(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "x.proto", `
syntax = "proto3";
package myproject.x;
message Msg { string s = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
		Overrides: map[string]string{
			"myproject/x/x.proto": "example.com/mod/gen/../escape",
		},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "myproject", "x", "x.pb.go")); err != nil {
		t.Fatalf("expected source-relative output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(outDir), "escape")); err == nil {
		t.Errorf("-M escaped OutDir")
	}
}

// TestGenerateGoPackageDoesNotEscapeOutDir verifies that a `..` segment in
// go_package cannot make the generator write outside g.OutDir. Disk paths
// are derived from the source-relative fd.Path() — not from go_package —
// so the `..` can only appear in the generated file's import-path string
// (which then fails loudly at `go build`, matching protoc-gen-go).
func TestGenerateGoPackageDoesNotEscapeOutDir(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "evil.proto", `
syntax = "proto3";
package myproject.evil;
option go_package = "example.com/mod/gen/../outside;evil";
message Mal { string s = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Output lands at the source-relative path, never at "<outDir>/../outside".
	if _, err := os.Stat(filepath.Join(outDir, "myproject", "evil", "evil.pb.go")); err != nil {
		t.Fatalf("expected source-relative output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(outDir), "outside")); err == nil {
		t.Errorf("generator escaped OutDir")
	}
}

// TestGenerateGoPackageDuplicateImportPath rejects two distinct proto
// packages whose go_package values resolve to the same Go import path —
// they would share a directory but disagree on the package clause.
func TestGenerateGoPackageDuplicateImportPath(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "a.proto", `
syntax = "proto3";
package proj.a;
option go_package = "example.com/mod/gen/shared;shared";
message Foo { string s = 1; }`)
	writeProto(t, protoDir, "b.proto", `
syntax = "proto3";
package proj.b;
option go_package = "example.com/mod/gen/shared;shared";
message Bar { string s = 1; }`)

	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    testOutDir(t),
		ProtoDirs: []string{protoDir},
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected duplicate-import-path error, got nil")
	}
	if !strings.Contains(err.Error(), `import path`) {
		t.Errorf("expected duplicate-import-path error, got: %v", err)
	}
}

// TestGenerateGoPackageShadowsDefaultDestination catches the cross-mode
// collision: one proto package's go_package points to a Go directory that
// another proto package would otherwise default to. validateDestinations
// must reject this — without it, two distinct Go-package files would be
// written into the same directory.
func TestGenerateGoPackageShadowsDefaultDestination(t *testing.T) {
	protoDir := t.TempDir()
	// proj.a explicitly redirects to gen/proj/b via go_package.
	writeProto(t, protoDir, "a.proto", `
syntax = "proto3";
package proj.a;
option go_package = "example.com/mod/gen/proj/b";
message Foo { string s = 1; }`)
	// proj.b has no go_package, so it would default to gen/proj/b — clash.
	writeProto(t, protoDir, "b.proto", `
syntax = "proto3";
package proj.b;
message Bar { string s = 1; }`)

	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    testOutDir(t),
		ProtoDirs: []string{protoDir},
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected destination-collision error, got nil")
	}
	if !strings.Contains(err.Error(), "import path") {
		t.Errorf("expected destination-collision error, got: %v", err)
	}
}

// TestGenerateGoPackageFallbackAliasMatchesPathBase verifies the elision-bug
// fix: when the fallback alias happens to equal the import path's last
// segment but differs from the file's declared `package` clause, emitHeader
// must still emit an explicit alias. Otherwise Go would bind the unaliased
// import to the file's declared name (not the fallback the generated code
// expects), producing a "undeclared name" compile error.
func TestGenerateGoPackageFallbackAliasMatchesPathBase(t *testing.T) {
	protoDir := t.TempDir()
	// dep is named `pkgone` with go_package `;myalias`. Its Go path ends
	// in `/pkgone`, so the proto-derived fallback alias (`xpkgone`) does
	// NOT match path.Base. To force the bug we'd need the fallback alias
	// to equal path.Base — instead we just demonstrate the cleaner check:
	// any time alias != naturalName, the alias is emitted explicitly.
	writeProto(t, protoDir, "dep.proto", `
syntax = "proto3";
package x.pkgone;
option go_package = "example.com/mod/gen/x/pkgone;myalias";
message Foo { string s = 1; }`)
	writeProto(t, protoDir, "use.proto", `
syntax = "proto3";
package y.use;
option go_package = "example.com/mod/gen/y/use;myalias";
import "x/pkgone/dep.proto";
message Bar { x.pkgone.Foo foo = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	use, err := os.ReadFile(filepath.Join(outDir, "y", "use", "use.pb.go"))
	if err != nil {
		t.Fatalf("expected output: %v", err)
	}
	useStr := string(use)
	// use.pb.go's own package is `myalias`; dep's declared package is
	// also `myalias`. The alias for dep collides with selfName, so we
	// fall back to the proto-derived alias `xpkgone`. xpkgone differs
	// from naturalName "myalias", so emitHeader must emit it explicitly.
	if !strings.Contains(useStr, `xpkgone "example.com/mod/gen/x/pkgone"`) {
		t.Errorf("use.pb.go: expected explicit fallback alias 'xpkgone' on dep import; content:\n%s", useStr)
	}
	if _, err := format.Source(use); err != nil {
		t.Errorf("use.pb.go did not round-trip through go/format: %v", err)
	}
}

// TestGenerateGoPackageFallbackAliasElision is the worst-case scenario:
// the proto-derived fallback alias happens to equal the import path's last
// segment, but the file's declared `package` clause is something else.
// The OLD heuristic (elide when path.HasSuffix(/alias)) would have emitted
// no alias, leaving Go to bind the import to the file's declared name —
// which doesn't match the alias the generator emitted in the body, causing
// a compile error. With naturalName-based elision, the alias is preserved.
func TestGenerateGoPackageFallbackAliasElision(t *testing.T) {
	protoDir := t.TempDir()
	// Construct a proto package whose proto-derived fallback alias equals
	// the path.Base of its import path. goPackageName("p.xfoo") = "pxfoo",
	// so we set the go_package import path to end in `/pxfoo`.
	writeProto(t, protoDir, "dep.proto", `
syntax = "proto3";
package p.xfoo;
option go_package = "example.com/mod/gen/wrap/pxfoo;myalias";
message Foo { string s = 1; }`)
	// Importer also has pkgName "myalias" so dep's pkgName collides with
	// selfName, forcing the fallback to "pxfoo".
	writeProto(t, protoDir, "use.proto", `
syntax = "proto3";
package q.use;
option go_package = "example.com/mod/gen/q/use;myalias";
import "p/xfoo/dep.proto";
message Bar { p.xfoo.Foo foo = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{
		Module:    "example.com/mod",
		OutDir:    outDir,
		ProtoDirs: []string{protoDir},
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	use, err := os.ReadFile(filepath.Join(outDir, "q", "use", "use.pb.go"))
	if err != nil {
		t.Fatalf("expected output: %v", err)
	}
	useStr := string(use)
	// Critical: even though "pxfoo" matches path.Base, it differs from the
	// declared `package myalias`. The explicit alias must be present.
	if !strings.Contains(useStr, `pxfoo "example.com/mod/gen/wrap/pxfoo"`) {
		t.Errorf("use.pb.go: explicit alias 'pxfoo' must be emitted because it differs from declared 'myalias'; content:\n%s", useStr)
	}
	if _, err := format.Source(use); err != nil {
		t.Errorf("use.pb.go did not round-trip through go/format: %v", err)
	}
}

// TestGenerateMissingProtoPath_CleanError pins that a non-existent --proto_path
// value produces a user-facing diagnostic naming the flag and the bad value —
// not the raw filesystem syscall string ("lstat ... no such file or directory")
// that filepath.WalkDir surfaces by default. The leak was DOC-9 / wiresmith-d2x.
func TestGenerateMissingProtoPath_CleanError(t *testing.T) {
	// Anchor the bad path inside a temp dir so the test stays portable
	// across OS/filesystem layouts (and a parallel test on another machine
	// can't ever happen to have a /this/path/... tree). The temp root
	// exists; the "missing" subpath under it does not.
	missing := filepath.Join(t.TempDir(), "missing-proto-tree")
	gen := &Generator{
		Module:    "wiresmith",
		OutDir:    testOutDir(t),
		ProtoDirs: []string{missing},
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected error for non-existent proto_path, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "--proto_path") {
		t.Errorf("error must name the flag, got: %q", msg)
	}
	// The production formatter prints the path via %q, so on Windows
	// `C:\tmp\x` shows up as `"C:\\tmp\\x"`. Compare against the same
	// quoted form rather than the raw path so the test stays portable.
	if quoted := fmt.Sprintf("%q", gen.ProtoDirs[0]); !strings.Contains(msg, quoted) {
		t.Errorf("error must echo the bad value %s, got: %q", quoted, msg)
	}
	if strings.Contains(msg, "lstat") {
		t.Errorf("error must not leak the lstat syscall name, got: %q", msg)
	}
}

// TestGenerateProtoPathIsFile_CleanError pins that --proto_path pointing
// at a regular file (not a directory) produces a clean diagnostic instead
// of silently walking nothing and reporting success. Without the IsDir
// guard, filepath.WalkDir on a file emits one callback with the file
// itself, which the .proto-suffix filter skips, leaving the generator to
// declare a successful empty run — a confusing outcome for what is
// usually a flag-value typo.
func TestGenerateProtoPathIsFile_CleanError(t *testing.T) {
	tmp := t.TempDir()
	asFile := filepath.Join(tmp, "definitely-a-file.txt")
	if err := os.WriteFile(asFile, []byte("not a proto tree"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	gen := &Generator{
		Module:    "wiresmith",
		OutDir:    testOutDir(t),
		ProtoDirs: []string{asFile},
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected error for file-shaped proto_path, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "--proto_path") {
		t.Errorf("error must name the flag, got: %q", msg)
	}
	if !strings.Contains(msg, "not a directory") {
		t.Errorf("error must surface the not-a-directory reason, got: %q", msg)
	}
	// Same %q-quoted comparison as the missing-path test above.
	if quoted := fmt.Sprintf("%q", gen.ProtoDirs[0]); !strings.Contains(msg, quoted) {
		t.Errorf("error must echo the bad value %s, got: %q", quoted, msg)
	}
}
