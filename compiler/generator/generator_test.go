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
		{"basic", filepath.Join(root, "proto", "basic")},
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

	mapping, importPaths, err := buildImportMapping(dir)
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

func TestBuildImportMappingNoPackage(t *testing.T) {
	dir := t.TempDir()
	writeProto(t, dir, "bare.proto", "syntax = \"proto3\";\nmessage Bare {}")

	_, _, err := buildImportMapping(dir)
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

	_, _, err := buildImportMapping(dir)
	if err == nil {
		t.Fatal("expected duplicate-key error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate import key") {
		t.Errorf("expected 'duplicate import key' error, got: %v", err)
	}
}

// TestGenerateNestedLayout runs the full generator pipeline against a
// recursive proto layout where a nested file imports another nested file.
// This is the integration-level counterpart to TestBuildImportMappingRecursive:
// it verifies the import keys we register actually resolve through protocompile
// and that .pb.go files land at the expected goPackageDir locations.
func TestGenerateNestedLayout(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "common/v1/common.proto",
		"syntax = \"proto3\";\npackage testpb.common.v1;\nmessage Resource { string name = 1; }")
	writeProto(t, protoDir, "trace/v1/trace.proto",
		"syntax = \"proto3\";\npackage testpb.trace.v1;\nimport \"common/v1/common.proto\";\nmessage Span { testpb.common.v1.Resource resource = 1; }")

	outDir := t.TempDir()
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDir: protoDir}
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
	if !strings.Contains(string(traceContent), "wiresmith/gen/testpb/common/v1") {
		t.Errorf("trace.pb.go missing cross-package import to common/v1; content:\n%s", traceContent)
	}
}

// TestGenerateMixedLayoutImport documents the supported import shape for a
// mixed flat+nested layout: a nested file importing a top-level file must
// use the top-level's package-derived path (its canonical key), not the
// plain basename. The plain-basename form fails because protocompile uses
// the queried path as file identity and would compile the file twice.
func TestGenerateMixedLayoutImport(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "common.proto",
		"syntax = \"proto3\";\npackage testpb;\nmessage Resource { string name = 1; }")
	writeProto(t, protoDir, "trace/v1/trace.proto",
		"syntax = \"proto3\";\npackage testpb.trace.v1;\nimport \"testpb/common.proto\";\nmessage Span { testpb.Resource resource = 1; }")

	outDir := t.TempDir()
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDir: protoDir}
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
	writeProto(t, protoDir2, "trace/v1/trace.proto",
		"syntax = \"proto3\";\npackage testpb.trace.v1;\nimport \"common.proto\";\nmessage Span { testpb.Resource resource = 1; }")
	gen2 := &Generator{Module: "wiresmith", OutDir: t.TempDir(), ProtoDir: protoDir2}
	if err := gen2.Generate(context.Background()); err == nil {
		t.Error("expected plain-basename import to fail; canonical pkg-derived path is required for cross-imports")
	}
}

// TestGenerateOutputCollision verifies that two protos in different
// subdirectories sharing the same package and basename are rejected
// before any file is written. Without this guard, recursive scanning
// would silently clobber the first .pb.go with the second.
func TestGenerateOutputCollision(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "a/v1/shared.proto",
		"syntax = \"proto3\";\npackage testpb.shared.v1;\nmessage A {}")
	writeProto(t, protoDir, "b/v1/shared.proto",
		"syntax = \"proto3\";\npackage testpb.shared.v1;\nmessage B {}")

	outDir := t.TempDir()
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDir: protoDir}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected output-collision error, got nil")
	}
	if !strings.Contains(err.Error(), "output collision") {
		t.Errorf("expected 'output collision' error, got: %v", err)
	}

	// Fail-fast guarantee: no .pb.go should have been written before the
	// collision was detected.
	collisionDir := filepath.Join(outDir, "testpb", "shared", "v1")
	if entries, err := os.ReadDir(collisionDir); err == nil && len(entries) > 0 {
		t.Errorf("expected no files written on collision, found %d in %s", len(entries), collisionDir)
	}
}
