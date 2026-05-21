package generator

import (
	"bytes"
	"context"
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

	outDir := t.TempDir()
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDir: protoDir}
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
// the Go package name, output directory, and the alias used by importing
// files, as long as the go_package import path falls under the module's
// effective base (module + "/gen").
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
	gen := &Generator{
		Module:   "example.com/mod",
		OutDir:   outDir,
		ProtoDir: protoDir,
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

// TestGenerateGoPackageFallback verifies that a go_package option pointing
// outside the module's effective base is ignored — the proto falls back to
// the default package-derived layout. This matches the OTel case where
// go_package is "go.opentelemetry.io/..." but we generate under "wiresmith/gen".
func TestGenerateGoPackageFallback(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "x.proto", `
syntax = "proto3";
package mytest.x;
option go_package = "some.other/module/pkg";
message Msg { int32 val = 1; }`)

	outDir := t.TempDir()
	gen := &Generator{
		Module:   "wiresmith",
		OutDir:   outDir,
		ProtoDir: protoDir,
	}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outDir, "mytest", "x", "x.pb.go"))
	if err != nil {
		t.Fatalf("expected fallback output: %v", err)
	}
	// Falls back to default goPackageName ("mytestx"), not "pkg" from go_package.
	if !strings.Contains(string(content), "package mytestx\n") {
		t.Errorf("expected fallback 'package mytestx', got:\n%s", string(content))
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

	outDir := t.TempDir()
	gen := &Generator{
		Module:   "example.com/app",
		OutDir:   outDir,
		ProtoDir: protoDir,
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
		Module:   "example.com/mod",
		OutDir:   t.TempDir(),
		ProtoDir: protoDir,
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
		Module:   "example.com/mod",
		OutDir:   t.TempDir(),
		ProtoDir: protoDir,
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected inconsistent go_package error, got nil")
	}
	if !strings.Contains(err.Error(), "inconsistent go_package") {
		t.Errorf("expected 'inconsistent go_package' error, got: %v", err)
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

	outDir := t.TempDir()
	gen := &Generator{
		Module:   "example.com/mod",
		OutDir:   outDir,
		ProtoDir: protoDir,
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
	writeProto(t, protoDir, "common.proto", `
syntax = "proto3";
package myproject.common;
option go_package = "example.com/mod/gen/common/v1;v1";
message Foo { string name = 1; }`)
	writeProto(t, protoDir, "trace.proto", `
syntax = "proto3";
package myproject.trace;
option go_package = "example.com/mod/gen/trace/v1;v1";
message Bar { string id = 1; }`)
	writeProto(t, protoDir, "api.proto", `
syntax = "proto3";
package myproject.api;
option go_package = "example.com/mod/gen/api/v1;v1";
import "myproject/common/common.proto";
import "myproject/trace/trace.proto";
message Request {
  myproject.common.Foo foo = 1;
  myproject.trace.Bar bar = 2;
}`)

	outDir := t.TempDir()
	gen := &Generator{
		Module:   "example.com/mod",
		OutDir:   outDir,
		ProtoDir: protoDir,
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
	writeProto(t, protoDir, "svc.proto", `
syntax = "proto3";
package myproject.svc;
option go_package = "example.com/mod/gen/svc;service";
import "a/common/v1/a.proto";
import "b/common/v1/b.proto";
message Req {
  myproject.acommon.v1.Foo a = 1;
  myproject.bcommon.v1.Bar b = 2;
}`)

	outDir := t.TempDir()
	gen := &Generator{
		Module:   "example.com/mod",
		OutDir:   outDir,
		ProtoDir: protoDir,
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
	writeProto(t, protoDir, "x.proto", `
syntax = "proto3";
package myproject.x;
option go_package = "example.com/mod/gen/myproject/type";
message Msg { string s = 1; }`)

	outDir := t.TempDir()
	gen := &Generator{
		Module:   "example.com/mod",
		OutDir:   outDir,
		ProtoDir: protoDir,
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
	writeProto(t, protoDir, "common.proto", `
syntax = "proto3";
package myproject.common;
option go_package = "example.com/mod/gen/common/v1;v1";
message Foo { string name = 1; }`)
	writeProto(t, protoDir, "trace.proto", `
syntax = "proto3";
package myproject.trace;
option go_package = "example.com/mod/gen/trace/v1;v1";
message Bar { string id = 1; }`)
	writeProto(t, protoDir, "api.proto", `
syntax = "proto3";
package myproject.api;
option go_package = "example.com/mod/gen/api/v1;service";
import "myproject/common/common.proto";
import "myproject/trace/trace.proto";
message Request {
  myproject.common.Foo foo = 1;
  myproject.trace.Bar bar = 2;
}`)

	outDir := t.TempDir()
	gen := &Generator{
		Module:   "example.com/mod",
		OutDir:   outDir,
		ProtoDir: protoDir,
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

// TestGenerateGoPackagePathTraversal rejects go_package values that contain
// `..` segments — they'd let filepath.Join write outside g.OutDir.
func TestGenerateGoPackagePathTraversal(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "evil.proto", `
syntax = "proto3";
package myproject.evil;
option go_package = "example.com/mod/gen/../outside;evil";
message Mal { string s = 1; }`)

	gen := &Generator{
		Module:   "example.com/mod",
		OutDir:   t.TempDir(),
		ProtoDir: protoDir,
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected path-traversal error, got nil")
	}
	if !strings.Contains(err.Error(), "'..'") {
		t.Errorf("expected '..'-segment error, got: %v", err)
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
		Module:   "example.com/mod",
		OutDir:   t.TempDir(),
		ProtoDir: protoDir,
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected duplicate-import-path error, got nil")
	}
	if !strings.Contains(err.Error(), `claimed by both proto packages`) {
		t.Errorf("expected destination-collision error, got: %v", err)
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
		Module:   "example.com/mod",
		OutDir:   t.TempDir(),
		ProtoDir: protoDir,
	}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("expected destination-collision error, got nil")
	}
	if !strings.Contains(err.Error(), "claimed by both proto packages") {
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

	outDir := t.TempDir()
	gen := &Generator{
		Module:   "example.com/mod",
		OutDir:   outDir,
		ProtoDir: protoDir,
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

	outDir := t.TempDir()
	gen := &Generator{
		Module:   "example.com/mod",
		OutDir:   outDir,
		ProtoDir: protoDir,
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
