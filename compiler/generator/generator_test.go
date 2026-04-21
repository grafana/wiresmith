package generator

import (
	"bytes"
	"context"
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkDeterminism(t, tc.protoDir, 5)
		})
	}
}

func checkDeterminism(t *testing.T, protoDir string, iterations int) {
	t.Helper()

	for i := 0; i < iterations; i++ {
		dirA := t.TempDir()
		dirB := t.TempDir()

		genA := &Generator{
			Module:   "wiresmith",
			OutDir:   dirA,
			ProtoDir: protoDir,
		}
		genB := &Generator{
			Module:   "wiresmith",
			OutDir:   dirB,
			ProtoDir: protoDir,
		}

		ctx := context.Background()

		if err := genA.Generate(ctx); err != nil {
			t.Fatalf("iteration %d: first Generate failed: %v", i, err)
		}
		if err := genB.Generate(ctx); err != nil {
			t.Fatalf("iteration %d: second Generate failed: %v", i, err)
		}

		// Walk dirA and compare every file with its counterpart in dirB.
		err := filepath.Walk(dirA, func(pathA string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			rel, err := filepath.Rel(dirA, pathA)
			if err != nil {
				return err
			}
			pathB := filepath.Join(dirB, rel)

			contentA, err := os.ReadFile(pathA)
			if err != nil {
				return err
			}
			contentB, err := os.ReadFile(pathB)
			if err != nil {
				t.Errorf("iteration %d: file %s exists in first output but not in second", i, rel)
				return nil
			}

			if !bytes.Equal(contentA, contentB) {
				t.Errorf("iteration %d: file %s differs between runs", i, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("iteration %d: walking output directory: %v", i, err)
		}

		// Also walk dirB to catch files that exist only in the second output.
		err = filepath.Walk(dirB, func(pathB string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(dirB, pathB)
			if err != nil {
				return err
			}
			pathA := filepath.Join(dirA, rel)
			if _, err := os.Stat(pathA); os.IsNotExist(err) {
				t.Errorf("iteration %d: file %s exists in second output but not in first", i, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("iteration %d: walking second output directory: %v", i, err)
		}
	}
}

// TestGenerateMatchesCheckedIn runs the generator against every proto set that
// `make generate-ours` uses and verifies the output matches the checked-in
// files under gen/. This catches generator regressions without needing `make`.
func TestGenerateMatchesCheckedIn(t *testing.T) {
	root := repoRoot(t)
	ctx := context.Background()

	cases := []struct {
		name     string
		protoDir string
	}{
		{
			name:     "otlp",
			protoDir: filepath.Join(root, "proto", "otlp"),
		},
		{
			name:     "test/kitchensink",
			protoDir: filepath.Join(root, "proto", "test"),
		},
		{
			name:     "bench/maps",
			protoDir: filepath.Join(root, "proto", "bench"),
		},
		{
			name:     "conformance/test_messages",
			protoDir: filepath.Join(root, "proto", "conformance"),
		},
	}

	genDir := filepath.Join(root, "gen")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()

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
				Module:   "wiresmith",
				OutDir:   tmpDir,
				ProtoDir: protoDir,
			}
			if err := gen.Generate(ctx); err != nil {
				t.Fatalf("Generate failed: %v", err)
			}

			// Walk the freshly generated output and compare against checked-in
			// files. The generator writes to outDir/goPackageDir(pkg), which
			// mirrors the layout under gen/.
			generatedFiles := make(map[string]struct{})
			err := filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(tmpDir, path)
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

func TestBuildImportMappingFlat(t *testing.T) {
	dir := t.TempDir()
	writeProto(t, dir, "foo.proto", "syntax = \"proto3\";\npackage test.foo;\nmessage Foo {}")

	mapping, importPaths, err := buildImportMapping(dir)
	if err != nil {
		t.Fatalf("buildImportMapping: %v", err)
	}
	if len(importPaths) != 1 {
		t.Fatalf("expected 1 import path, got %d", len(importPaths))
	}
	// Top-level file uses package-derived key.
	if _, ok := mapping["test/foo/foo.proto"]; !ok {
		t.Errorf("expected key test/foo/foo.proto, got keys: %v", importPaths)
	}
}

func TestBuildImportMappingRecursive(t *testing.T) {
	dir := t.TempDir()
	writeProto(t, dir, "common/v1/common.proto",
		"syntax = \"proto3\";\npackage tempopb.common.v1;\nmessage Foo {}")
	writeProto(t, dir, "trace/v1/trace.proto",
		"syntax = \"proto3\";\npackage tempopb.trace.v1;\nimport \"common/v1/common.proto\";\nmessage Bar { tempopb.common.v1.Foo foo = 1; }")

	mapping, importPaths, err := buildImportMapping(dir)
	if err != nil {
		t.Fatalf("buildImportMapping: %v", err)
	}
	if len(importPaths) != 2 {
		t.Fatalf("expected 2 import paths, got %d: %v", len(importPaths), importPaths)
	}
	// Nested files use relative paths.
	if _, ok := mapping["common/v1/common.proto"]; !ok {
		t.Error("expected common/v1/common.proto in mapping")
	}
	if _, ok := mapping["trace/v1/trace.proto"]; !ok {
		t.Error("expected trace/v1/trace.proto in mapping")
	}
	// Must be sorted for determinism.
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

	mapping, importPaths, err := buildImportMapping(dir)
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

func TestGenerateWithStripPrefix(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "common/v1/common.proto", `
syntax = "proto3";
package tempopb.common.v1;
message KeyValue {
  string key = 1;
  string value = 2;
}`)
	writeProto(t, protoDir, "trace/v1/trace.proto", `
syntax = "proto3";
package tempopb.trace.v1;
import "common/v1/common.proto";
message Span {
  string name = 1;
  repeated tempopb.common.v1.KeyValue attributes = 2;
}`)

	outDir := t.TempDir()
	g := &Generator{
		Module:        "github.com/grafana/tempo",
		OutDir:        outDir,
		ProtoDir:      protoDir,
		StripPrefix:   "tempopb",
		ImportBase:    "github.com/grafana/tempo/pkg/tempopb",
		HelpersImport: "github.com/grafana/tempo/pkg/tempopb/protohelpers",
	}
	if err := g.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	commonFile := filepath.Join(outDir, "common", "v1", "common.pb.go")
	traceFile := filepath.Join(outDir, "trace", "v1", "trace.pb.go")

	commonContent, err := os.ReadFile(commonFile)
	if err != nil {
		t.Fatalf("expected output at %s: %v", commonFile, err)
	}
	traceContent, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("expected output at %s: %v", traceFile, err)
	}

	common := string(commonContent)
	trace := string(traceContent)

	// Package name: last component after stripping prefix.
	if !strings.Contains(common, "package v1\n") {
		t.Error("common: expected 'package v1'")
	}
	if !strings.Contains(trace, "package v1\n") {
		t.Error("trace: expected 'package v1'")
	}

	// Custom helpers import.
	if !strings.Contains(common, `"github.com/grafana/tempo/pkg/tempopb/protohelpers"`) {
		t.Error("common: expected custom helpers import path")
	}

	// Cross-file import uses import base.
	if !strings.Contains(trace, `"github.com/grafana/tempo/pkg/tempopb/common/v1"`) {
		t.Error("trace: expected import of common/v1 package")
	}
	// Alias must not be bare "v1" (collides with own package name).
	if strings.Contains(trace, "\tv1 \"") {
		t.Error("trace: alias 'v1' collides with package name, should be disambiguated")
	}
}

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

	outDir := t.TempDir()
	g := &Generator{
		Module:   "example.com/mod",
		OutDir:   outDir,
		ProtoDir: protoDir,
	}
	if err := g.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	aFile := filepath.Join(outDir, "myproject", "a", "a.pb.go")
	bFile := filepath.Join(outDir, "myproject", "b", "b.pb.go")

	aContent, err := os.ReadFile(aFile)
	if err != nil {
		t.Fatalf("expected output at %s: %v", aFile, err)
	}
	bContent, err := os.ReadFile(bFile)
	if err != nil {
		t.Fatalf("expected output at %s: %v", bFile, err)
	}

	// Package name from go_package semicolon form.
	if !strings.Contains(string(aContent), "package a\n") {
		t.Errorf("a.pb.go: expected 'package a', got:\n%s", string(aContent)[:min(200, len(aContent))])
	}
	if !strings.Contains(string(bContent), "package b\n") {
		t.Errorf("b.pb.go: expected 'package b', got:\n%s", string(bContent)[:min(200, len(bContent))])
	}

	// Cross-file import uses go_package import path.
	if !strings.Contains(string(bContent), `"example.com/mod/gen/myproject/a"`) {
		t.Error("b.pb.go: expected import of example.com/mod/gen/myproject/a")
	}
}

func TestGenerateGoPackageFallback(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "x.proto", `
syntax = "proto3";
package mytest.x;
option go_package = "some.other/module/pkg";
message Msg { int32 val = 1; }`)

	outDir := t.TempDir()
	g := &Generator{
		Module:   "wiresmith",
		OutDir:   outDir,
		ProtoDir: protoDir,
	}
	if err := g.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	outFile := filepath.Join(outDir, "mytest", "x", "x.pb.go")
	content, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("expected fallback output at %s: %v", outFile, err)
	}

	if !strings.Contains(string(content), "package mytestx\n") {
		t.Errorf("expected 'package mytestx', got:\n%s", string(content)[:min(200, len(content))])
	}
}

func TestGenerateGoPackageWithSemicolon(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "svc.proto", `
syntax = "proto3";
package myapp.svc;
option go_package = "example.com/app/gen/myapp/svc;service";
message Request { string id = 1; }`)

	outDir := t.TempDir()
	g := &Generator{
		Module:   "example.com/app",
		OutDir:   outDir,
		ProtoDir: protoDir,
	}
	if err := g.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	outFile := filepath.Join(outDir, "myapp", "svc", "svc.pb.go")
	content, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("expected output at %s: %v", outFile, err)
	}

	if !strings.Contains(string(content), "package service\n") {
		t.Errorf("expected 'package service', got:\n%s", string(content)[:min(200, len(content))])
	}
}

func TestGeneratorDeterminismRecursive(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "common/v1/common.proto",
		"syntax = \"proto3\";\npackage ns.common.v1;\nmessage Foo { string name = 1; }")
	writeProto(t, protoDir, "svc/v1/svc.proto",
		"syntax = \"proto3\";\npackage ns.svc.v1;\nimport \"common/v1/common.proto\";\nmessage Bar { ns.common.v1.Foo foo = 1; }")

	for i := 0; i < 5; i++ {
		dirA := t.TempDir()
		dirB := t.TempDir()

		genA := &Generator{Module: "testmod", OutDir: dirA, ProtoDir: protoDir}
		genB := &Generator{Module: "testmod", OutDir: dirB, ProtoDir: protoDir}

		ctx := context.Background()
		if err := genA.Generate(ctx); err != nil {
			t.Fatalf("iteration %d: first Generate: %v", i, err)
		}
		if err := genB.Generate(ctx); err != nil {
			t.Fatalf("iteration %d: second Generate: %v", i, err)
		}

		err := filepath.Walk(dirA, func(pathA string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(dirA, pathA)
			contentA, _ := os.ReadFile(pathA)
			contentB, err := os.ReadFile(filepath.Join(dirB, rel))
			if err != nil {
				t.Errorf("iteration %d: %s missing in second output", i, rel)
				return nil
			}
			if !bytes.Equal(contentA, contentB) {
				t.Errorf("iteration %d: %s differs between runs", i, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("iteration %d: walk: %v", i, err)
		}
	}
}
