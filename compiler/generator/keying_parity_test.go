package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateFlatSiblingBareImport pins the protoc/buf path-parity contract at
// the generator level: two .proto files sitting directly at the --proto_path
// root (the Prometheus prompb shape) key by their bare filenames, so one can
// `import "types.proto"` the other by basename and it resolves. Output is
// source-relative for flat files — the bare basename lands at the outDir root,
// never under a package-derived directory.
func TestGenerateFlatSiblingBareImport(t *testing.T) {
	protoDir := t.TempDir()
	writeProto(t, protoDir, "types.proto", `
syntax = "proto3";
package flatpkg;
message Sample { double value = 1; }`)
	writeProto(t, protoDir, "api.proto", `
syntax = "proto3";
package flatpkg;
import "types.proto";
message WriteRequest { repeated Sample samples = 1; }`)

	outDir := testOutDir(t)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: a bare-filename import between flat siblings must resolve: %v", err)
	}

	// Source-relative flat output: bare basenames at the outDir root.
	for _, rel := range []string{"types.pb.go", "api.pb.go"} {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Errorf("expected flat output %s at outDir root: %v", rel, err)
		}
	}
	// The pre-parity package-derived directory must NOT appear.
	if _, err := os.Stat(filepath.Join(outDir, "flatpkg")); !os.IsNotExist(err) {
		t.Errorf("flat file must not emit under a package-derived directory (found flatpkg/), err=%v", err)
	}
	// Same Go package: the cross-file reference is unqualified.
	apiSrc := mustReadFile(t, filepath.Join(outDir, "api.pb.go"))
	if !strings.Contains(apiSrc, "Sample") {
		t.Errorf("api.pb.go must reference Sample from the bare-imported sibling, got:\n%s", apiSrc)
	}
}

// TestVendoredOptionsProtoByteIdenticalAccepted covers Fix B: a consumer that
// vendors wiresmith/options.proto on disk (required for buf/protoc, which can't
// read the compiler's embed) is accepted silently when the bytes match the
// embedded schema. The vendored file emits no Go output, and the options still
// apply to user files that import it.
func TestVendoredOptionsProtoByteIdenticalAccepted(t *testing.T) {
	protoDir := t.TempDir()
	// Byte-identical vendored copy at the canonical import path.
	writeProto(t, protoDir, embeddedOptionsPath, string(embeddedOptionsProto))
	writeProto(t, protoDir, "user/v1/user.proto", `
syntax = "proto3";
package user.v1;
option go_package = "wiresmith/gen/user/v1";
import "wiresmith/options.proto";
message M {
  string id = 1 [(wiresmith.options.customname) = "ID"];
}`)

	outDir := testOutDir(t)
	gen := &Generator{Module: "wiresmith", OutDir: outDir, ProtoDirs: []string{protoDir}}
	if err := gen.Generate(context.Background()); err != nil {
		t.Fatalf("byte-identical vendored options.proto must be accepted: %v", err)
	}

	// The user file compiles and the imported option applies.
	userSrc := mustReadFile(t, filepath.Join(outDir, "user", "v1", "user.pb.go"))
	if !strings.Contains(userSrc, "ID ") && !strings.Contains(userSrc, "ID\t") {
		t.Errorf("customname option from the vendored-options build did not apply, got:\n%s", userSrc)
	}
	// The vendored schema itself must not emit Go output.
	if _, err := os.Stat(filepath.Join(outDir, "wiresmith")); !os.IsNotExist(err) {
		t.Errorf("vendored wiresmith/options.proto must not emit output, found wiresmith/ (err=%v)", err)
	}
}

// TestVendoredOptionsProtoByteMismatchRejected covers the other half of Fix B:
// an on-disk wiresmith/options.proto whose bytes differ from the embedded
// schema is rejected up front, with an error that calls out the mismatch (so a
// consumer whose vendored copy drifted from the pinned compiler gets a clear
// signal rather than a divergent schema silently shadowing the embed).
func TestVendoredOptionsProtoByteMismatchRejected(t *testing.T) {
	protoDir := t.TempDir()
	// A divergent copy at the canonical path (still a valid proto, but not the
	// embed byte-for-byte). Fix B compares bytes before compiling.
	writeProto(t, protoDir, embeddedOptionsPath, string(embeddedOptionsProto)+"\n// vendored copy drifted from the pinned compiler\n")
	writeProto(t, protoDir, "user/v1/user.proto", `
syntax = "proto3";
package user.v1;
message M { string id = 1; }`)

	gen := &Generator{Module: "wiresmith", OutDir: testOutDir(t), ProtoDirs: []string{protoDir}}
	err := gen.Generate(context.Background())
	if err == nil {
		t.Fatal("byte-divergent vendored options.proto must be rejected, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "embedded wiresmith schema") || !strings.Contains(msg, "differ") {
		t.Errorf("error must call out the byte mismatch against the embedded schema, got: %v", err)
	}
}
