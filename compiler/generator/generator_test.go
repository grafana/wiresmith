package generator

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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
	protoDir := filepath.Join(root, "proto", "otlp")

	const iterations = 5
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
