// Package plugin_test exercises the protoc-gen-wiresmith plugin through a
// real protoc invocation. Where cmd/protoc-gen-wiresmith/main_test.go feeds
// the plugin a synthetic CodeGeneratorRequest, this test goes all the way
// out to a `protoc --plugin=...` subprocess so we catch any difference
// between protogen-built descriptors and what protoc actually delivers on
// the wire (varying field defaults, descriptor encoding round-trips, etc.).
//
// The test is skipped when `protoc` isn't on PATH so a developer without it
// can still run `make test`.
package plugin_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestProtocInvokesPluginEndToEnd builds the plugin, hands it a tiny .proto
// fixture via protoc, and checks the resulting .pb.go set:
//   - the three files we expect (main + util + compare) exist;
//   - the main file declares the struct in the expected Go package;
//   - the output compiles with `go build` (executed inside a temp module).
//
// Catching the build step matters: a regression that produced syntactically
// valid Go but mis-named a type would slip past pure substring checks.
func TestProtocInvokesPluginEndToEnd(t *testing.T) {
	protoc, err := exec.LookPath("protoc")
	if err != nil {
		t.Skip("protoc not found on PATH; skipping integration test")
	}
	goExe, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not found on PATH; skipping integration test")
	}

	tmp := t.TempDir()

	pluginBin := filepath.Join(tmp, "protoc-gen-wiresmith")
	build := exec.Command(goExe, "build", "-o", pluginBin, "./cmd/protoc-gen-wiresmith")
	build.Dir = repoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building plugin: %v\n%s", err, out)
	}

	protoDir := filepath.Join(tmp, "proto", "pluginsmoke", "v1")
	if err := os.MkdirAll(protoDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const protoSrc = `syntax = "proto3";

package pluginsmoke.v1;

option go_package = "example.com/pluginsmoke/v1;pluginsmoke";

message Greeting {
  string text = 1;
  int32 count = 2;
  repeated string tags = 3;
}
`
	protoPath := filepath.Join(protoDir, "greeting.proto")
	if err := os.WriteFile(protoPath, []byte(protoSrc), 0o644); err != nil {
		t.Fatalf("write proto: %v", err)
	}

	outDir := filepath.Join(tmp, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out: %v", err)
	}

	cmd := exec.Command(
		protoc,
		"--plugin=protoc-gen-wiresmith="+pluginBin,
		"--proto_path="+filepath.Join(tmp, "proto"),
		"--wiresmith_out="+outDir,
		"pluginsmoke/v1/greeting.proto",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("protoc: %v\n%s", err, out)
	}

	expected := []string{
		"pluginsmoke/v1/greeting.pb.go",
		"pluginsmoke/v1/greeting_util.pb.go",
		"pluginsmoke/v1/greeting_compare.pb.go",
	}
	for _, rel := range expected {
		full := filepath.Join(outDir, rel)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}

	main, err := os.ReadFile(filepath.Join(outDir, "pluginsmoke/v1/greeting.pb.go"))
	if err != nil {
		t.Fatalf("read main: %v", err)
	}
	wantSubstrs := []string{
		"package pluginsmoke",
		"type Greeting struct",
		"func (m *Greeting) Marshal()",
		"func (m *Greeting) Unmarshal(",
	}
	for _, want := range wantSubstrs {
		if !strings.Contains(string(main), want) {
			t.Errorf("greeting.pb.go missing %q", want)
		}
	}

	// `go build` the generated package inside a throwaway module to catch
	// any regression that produced parseable-but-uncompilable Go. The
	// protohelpers package lives in this repo; wire it in as a replace so
	// the synthetic module can import it without going through a proxy. Pin
	// the temp module's `go` directive to whatever the repo's own go.mod
	// declares so the build runs under the same language version that
	// `make build` would — a stale literal here once masked the difference
	// between Go 1.24 and 1.26 toolchain semantics.
	goVer := readGoModVersion(t, filepath.Join(repoRoot(t), "go.mod"))
	modDir := filepath.Join(outDir, "pluginsmoke")
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), []byte(
		"module example.com/pluginsmoke\n\ngo "+goVer+"\n\nrequire github.com/grafana/wiresmith v0.0.0-00010101000000-000000000000\n\nreplace github.com/grafana/wiresmith => "+repoRoot(t)+"\n",
	), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	// `go mod tidy` would touch the network unless GOFLAGS=-mod=mod and
	// GOPROXY=off; skip it and rely on the replace directive to satisfy
	// the wiresmith import. Pull the wiresmith module's go.sum into ours
	// so `go build` doesn't fail integrity checking on transitive deps.
	if data, err := os.ReadFile(filepath.Join(repoRoot(t), "go.sum")); err == nil {
		if err := os.WriteFile(filepath.Join(modDir, "go.sum"), data, 0o644); err != nil {
			t.Fatalf("copy go.sum: %v", err)
		}
	}
	goBuild := exec.Command(goExe, "build", "./...")
	goBuild.Dir = modDir
	goBuild.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if out, err := goBuild.CombinedOutput(); err != nil {
		t.Fatalf("go build of generated package failed: %v\n%s", err, out)
	}
}

// readGoModVersion returns the `go <ver>` directive from a go.mod file.
// Used so the temp module the integration test writes tracks the same Go
// language version the rest of the repo builds under.
func readGoModVersion(t *testing.T, goModPath string) string {
	t.Helper()
	data, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("read %s: %v", goModPath, err)
	}
	m := goDirectiveRE.FindStringSubmatch(string(data))
	if m == nil {
		t.Fatalf("no `go <version>` directive in %s", goModPath)
	}
	return m[1]
}

// goDirectiveRE captures the language version from a `go <ver>` directive
// on its own line. Multiline mode matches the directive wherever it
// appears in go.mod (always near the top in practice).
var goDirectiveRE = regexp.MustCompile(`(?m)^go\s+(\S+)\s*$`)

// repoRoot returns the absolute path to this repository's root by walking
// up from the test binary's CWD until it finds go.mod. Computed lazily so
// the test still works when invoked from arbitrary working directories.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod walking up from %q", dir)
		}
		dir = parent
	}
}
