package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/reporter"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// TestRunBasic drives the plugin's run() with a synthetic CodeGeneratorRequest
// and checks the response carries the four generated files we expect for a
// trivial single-message proto (main + reflect + compare + equal). The proto
// is compiled via protocompile so the FileDescriptorProto reaching protogen
// has the same shape protoc would have emitted.
func TestRunBasic(t *testing.T) {
	const src = `
syntax = "proto3";
package wsplugin.v1;
option go_package = "example.com/wsplugin/v1;wsplugin";
message Greeting {
  string text = 1;
  int32 count = 2;
  repeated string tags = 3;
}
`
	resp := runPluginOnSrc(t, "wsplugin/v1/greeting.proto", src, pluginOpts{})

	if got := resp.GetError(); got != "" {
		t.Fatalf("response error: %s", got)
	}
	files := resp.GetFile()
	if len(files) != 4 {
		var names []string
		for _, f := range files {
			names = append(names, f.GetName())
		}
		t.Fatalf("expected 4 files (main+reflect+compare+equal), got %d: %v", len(files), names)
	}

	mainOut := findFile(t, files, "wsplugin/v1/greeting.pb.go")
	wantSubstrs := []string{
		"package wsplugin",
		"type Greeting struct",
		"Text  string",
		"Count int32",
		"Tags  []string",
		"func (m *Greeting) Marshal() (dAtA []byte, err error)",
		"func (m *Greeting) Unmarshal(b []byte) error",
	}
	for _, want := range wantSubstrs {
		if !strings.Contains(mainOut.GetContent(), want) {
			t.Errorf("greeting.pb.go missing %q", want)
		}
	}
	// Sanity: companion files should be wired to the same package.
	for _, suffix := range []string{"_reflect.pb.go", "_compare.pb.go", "_equal.pb.go"} {
		path := "wsplugin/v1/greeting" + suffix
		companion := findFile(t, files, path)
		if !strings.Contains(companion.GetContent(), "package wsplugin") {
			t.Errorf("%s missing package wsplugin", path)
		}
	}
}

// TestRunSkipsNonGenerateImports verifies the FilesToGenerate filter: a
// dependency that's part of the request but not listed for generation
// (matching how buf delivers transitive imports) must not produce output.
func TestRunSkipsNonGenerateImports(t *testing.T) {
	const importedSrc = `
syntax = "proto3";
package wsplugin.dep;
option go_package = "example.com/wsplugin/dep;dep";
message Dep { string s = 1; }
`
	const mainSrc = `
syntax = "proto3";
package wsplugin.v1;
import "wsplugin/dep/dep.proto";
option go_package = "example.com/wsplugin/v1;wsplugin";
message Uses { wsplugin.dep.Dep d = 1; }
`
	files := map[string]string{
		"wsplugin/dep/dep.proto": importedSrc,
		"wsplugin/v1/uses.proto": mainSrc,
	}
	resp := runPluginOnFiles(t, files, []string{"wsplugin/v1/uses.proto"}, pluginOpts{})
	if got := resp.GetError(); got != "" {
		t.Fatalf("response error: %s", got)
	}
	for _, f := range resp.GetFile() {
		if strings.HasPrefix(f.GetName(), "wsplugin/dep/") {
			t.Errorf("plugin emitted %q for a transitive dep that was not in FilesToGenerate", f.GetName())
		}
	}
	// And the requested file did emit.
	findFile(t, resp.GetFile(), "wsplugin/v1/uses.pb.go")
}

// TestRunEmptyRequest checks that the plugin succeeds (returning an empty
// response) when nothing is requested for generation. Matches protoc-gen-go's
// behaviour for the same input.
func TestRunEmptyRequest(t *testing.T) {
	req := &pluginpb.CodeGeneratorRequest{}
	plugin, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatalf("protogen.New: %v", err)
	}
	if err := run(plugin, pluginOpts{}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if files := plugin.Response().GetFile(); len(files) != 0 {
		t.Fatalf("expected no generated files, got %d", len(files))
	}
}

// TestRunSurfacesGeneratorErrors confirms a generator-level failure (here:
// an unresolvable `paths=source_relative` conflict where two protos in
// different packages would collide on disk) propagates back to the plugin
// response as an error rather than crashing.
func TestRunSurfacesGeneratorErrors(t *testing.T) {
	// Two .proto files in different proto packages that source-relative to
	// the same directory — caught by validateDestinations.
	files := map[string]string{
		"shared/a.proto": `
syntax = "proto3";
package one;
option go_package = "example.com/one;one";
message A { string s = 1; }
`,
		"shared/b.proto": `
syntax = "proto3";
package two;
option go_package = "example.com/two;two";
message B { string s = 1; }
`,
	}
	err := runPluginOnFilesExpectingError(t, files, []string{"shared/a.proto", "shared/b.proto"}, pluginOpts{})
	if err == nil {
		t.Fatal("expected run() to return an error for conflicting destinations")
	}
	if !strings.Contains(err.Error(), "shared") {
		t.Fatalf("error %q should mention the conflicting directory", err)
	}
}

// runPluginOnFilesExpectingError mirrors runPluginOnFiles but returns the
// error from run() instead of unconditionally t.Fatal-ing on it. Used by
// tests that exercise generator-level failure paths.
func runPluginOnFilesExpectingError(t *testing.T, sources map[string]string, toGenerate []string, opts pluginOpts) error {
	t.Helper()
	plugin := buildPlugin(t, sources, toGenerate)
	return run(plugin, opts)
}

// runPluginOnSrc compiles a single .proto source string into a
// CodeGeneratorRequest and drives the plugin against it. Returns the raw
// response so each test can assert its own expectations.
func runPluginOnSrc(t *testing.T, path, src string, opts pluginOpts) *pluginpb.CodeGeneratorResponse {
	t.Helper()
	return runPluginOnFiles(t, map[string]string{path: src}, []string{path}, opts)
}

// runPluginOnFiles is the multi-file form: compile every entry in `sources`
// and ask the plugin to generate `toGenerate`. The compile resolver pulls in
// google.protobuf well-knowns via WithStandardImports, matching how a real
// protoc invocation would have populated the request.
func runPluginOnFiles(t *testing.T, sources map[string]string, toGenerate []string, opts pluginOpts) *pluginpb.CodeGeneratorResponse {
	t.Helper()
	plugin := buildPlugin(t, sources, toGenerate)
	if err := run(plugin, opts); err != nil {
		t.Fatalf("run: %v", err)
	}
	return plugin.Response()
}

// buildPlugin compiles `sources` with protocompile (so the FileDescriptorProtos
// reaching protogen carry the SourceInfo and dependency closure protoc would
// have populated), then constructs a protogen.Plugin for the given
// FilesToGenerate list. Shared by the response-asserting helpers and the
// error-asserting helpers.
func buildPlugin(t *testing.T, sources map[string]string, toGenerate []string) *protogen.Plugin {
	t.Helper()
	contents := make(map[string][]byte, len(sources))
	for k, v := range sources {
		contents[k] = []byte(v)
	}
	compiler := protocompile.Compiler{
		Resolver:       protocompile.WithStandardImports(&inMemoryResolver{files: contents}),
		SourceInfoMode: protocompile.SourceInfoStandard,
		Reporter: reporter.NewReporter(
			func(err reporter.ErrorWithPos) error { return err },
			nil,
		),
	}
	paths := make([]string, 0, len(sources))
	for p := range sources {
		paths = append(paths, p)
	}
	linked, err := compiler.Compile(context.Background(), paths...)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	// protogen wants the full transitive closure as FileDescriptorProtos.
	// Walk every reachable file (depth-first via imports) and serialise.
	seen := map[string]bool{}
	var protos []*descriptorpb.FileDescriptorProto
	var visit func(fd protoreflect.FileDescriptor)
	visit = func(fd protoreflect.FileDescriptor) {
		if seen[fd.Path()] {
			return
		}
		seen[fd.Path()] = true
		imps := fd.Imports()
		for i := 0; i < imps.Len(); i++ {
			visit(imps.Get(i).FileDescriptor)
		}
		protos = append(protos, protodesc.ToFileDescriptorProto(fd))
	}
	for _, fd := range linked {
		visit(fd)
	}

	req := &pluginpb.CodeGeneratorRequest{
		ProtoFile:      protos,
		FileToGenerate: append([]string(nil), toGenerate...),
	}
	plugin, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatalf("protogen.New: %v", err)
	}
	return plugin
}

// inMemoryResolver implements protocompile.Resolver for tests by serving
// hand-coded sources from a map.
type inMemoryResolver struct {
	files map[string][]byte
}

func (r *inMemoryResolver) FindFileByPath(path string) (protocompile.SearchResult, error) {
	content, ok := r.files[path]
	if !ok {
		return protocompile.SearchResult{}, &fileNotFoundErr{path: path}
	}
	return protocompile.SearchResult{Source: bytes.NewReader(content)}, nil
}

type fileNotFoundErr struct{ path string }

func (e *fileNotFoundErr) Error() string { return "file not found: " + e.path }

// findFile returns the response file whose name matches path or t.Fatal-s.
func findFile(t *testing.T, files []*pluginpb.CodeGeneratorResponse_File, path string) *pluginpb.CodeGeneratorResponse_File {
	t.Helper()
	for _, f := range files {
		if f.GetName() == path {
			return f
		}
	}
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.GetName()
	}
	t.Fatalf("file %q not in response (have %v)", path, names)
	return nil
}
