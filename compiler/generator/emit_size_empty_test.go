package generator

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// generateEmptyMsgMain writes protoBody at emptymsg/v1/test.proto, runs the
// full generator (Generate → the file-assembly + import-prune in generateFile,
// which is where the protowire prune runs — a fixture-only helper that skips
// file assembly would not exercise it), and returns the MAIN test.pb.go source.
func generateEmptyMsgMain(t *testing.T, protoBody string) string {
	t.Helper()
	protoDir := t.TempDir()
	outDir := testOutDir(t)
	writeProto(t, protoDir, "emptymsg/v1/test.proto", protoBody)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return mustReadFile(t, filepath.Join(outDir, "emptymsg", "v1", "test.pb.go"))
}

// TestFieldlessMessagePrunesProtowireImport pins the fix: a file whose messages
// are ALL field-less emits Size/Marshal/unmarshal bodies that never reference
// protowire, so the assembled main .pb.go must neither import protowire nor use
// the "protowire." token — otherwise it fails to compile with "imported and not
// used". emitSize adds the import per message unconditionally; the file-assembly
// prune drops it when the body has no protowire reference.
func TestFieldlessMessagePrunesProtowireImport(t *testing.T) {
	src := generateEmptyMsgMain(t, `
syntax = "proto3";
package emptymsg.v1;
option go_package = "wiresmith/gen/emptymsg/v1";
message E { reserved 1; }
message F {}`)

	if strings.Contains(src, "google.golang.org/protobuf/encoding/protowire") {
		t.Errorf("all-field-less file must NOT import protowire, got:\n%s", src)
	}
	if strings.Contains(src, "protowire.") {
		t.Errorf("all-field-less file must NOT reference the protowire. token, got:\n%s", src)
	}
}

// TestFieldedMessageKeepsProtowireImport is the over-pruning guard: a message
// with a real scalar field DOES emit protowire references (the varint tag
// write), so the prune must leave the import in place.
func TestFieldedMessageKeepsProtowireImport(t *testing.T) {
	src := generateEmptyMsgMain(t, `
syntax = "proto3";
package emptymsg.v1;
option go_package = "wiresmith/gen/emptymsg/v1";
message Counted { uint64 n = 1; }`)

	if !strings.Contains(src, "google.golang.org/protobuf/encoding/protowire") {
		t.Errorf("a message with a scalar field must keep the protowire import, got:\n%s", src)
	}
	if !strings.Contains(src, "protowire.") {
		t.Errorf("a message with a scalar field must reference protowire., got:\n%s", src)
	}
}
