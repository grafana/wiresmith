package generator

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
	"github.com/bufbuild/protocompile/reporter"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// compileProtoFixture compiles src as an in-memory .proto file named
// "test.proto" and returns the resulting FileDescriptor. Tests use this to
// drive the emit_* phases against a synthetic descriptor without touching the
// disk-bound full Generator pipeline.
//
// The embedded wiresmith/options.proto is always available to import — tests
// that exercise the pointer-option path can do so without disk I/O.
func compileProtoFixture(t *testing.T, src string) protoreflect.FileDescriptor {
	t.Helper()
	results := compileAllFixture(t, src)
	for _, fd := range results {
		if fd.Path() == "test.proto" {
			return fd
		}
	}
	t.Fatalf("compileProtoFixture: test.proto missing from compile results")
	return nil
}

// compileAllFixture is the multi-file form: returns every linked file from the
// compile. Useful when a test needs the embedded options descriptor alongside
// the user proto (to resolve the pointer extension).
func compileAllFixture(t *testing.T, src string) linker.Files {
	t.Helper()
	files := map[string][]byte{
		"test.proto":        []byte(src),
		embeddedOptionsPath: embeddedOptionsProto,
	}
	compiler := protocompile.Compiler{
		Resolver:       protocompile.WithStandardImports(&memResolver{files: files}),
		SourceInfoMode: protocompile.SourceInfoStandard,
		Reporter: reporter.NewReporter(
			func(err reporter.ErrorWithPos) error { return err },
			nil,
		),
	}
	results, err := compiler.Compile(context.Background(), "test.proto", embeddedOptionsPath)
	if err != nil {
		t.Fatalf("compiling proto: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("compile returned no files")
	}
	return results
}

// newFixtureGenerator builds a FileGenerator wired to fd with a destination
// map that covers exactly fd's own package. Cross-package emit cases are out
// of scope for these unit tests — the integration tests in
// option_pointer_test.go and generator_test.go cover the multi-file path.
//
// The pointer-option extension descriptor is resolved best-effort: tests that
// use a fixture without wiresmith/options.proto loaded get a nil pointerExt
// (and hasPointerOption returns false), which is the documented behavior.
func newFixtureGenerator(t *testing.T, fd protoreflect.FileDescriptor) *FileGenerator {
	t.Helper()
	return newFixtureGeneratorWith(t, fd, nil)
}

// newFixtureGeneratorWith is the variant that accepts the linked-files set
// (returned by compileAllFixture) so the pointer extension can be resolved.
// Tests that exercise (wiresmith.options.pointer) MUST use this form.
func newFixtureGeneratorWith(t *testing.T, fd protoreflect.FileDescriptor, allFiles linker.Files) *FileGenerator {
	t.Helper()
	selfDest := goDest{
		importPath: "github.com/grafana/wiresmith/gen/test/v1",
		relDir:     "test/v1",
		pkgName:    "testv1",
	}
	destinations := map[string]goDest{
		fd.Path(): selfDest,
	}
	var pointerExt, jsontagExt, customnameExt, customtypeExt, stdtimeExt protoreflect.FieldDescriptor
	for _, f := range allFiles {
		if f.Path() != embeddedOptionsPath {
			continue
		}
		exts := f.Extensions()
		for i := 0; i < exts.Len(); i++ {
			x := exts.Get(i)
			switch string(x.FullName()) {
			case pointerExtensionName:
				pointerExt = x
			case jsontagExtensionName:
				jsontagExt = x
			case customnameExtensionName:
				customnameExt = x
			case customtypeExtensionName:
				customtypeExt = x
			case stdtimeExtensionName:
				stdtimeExt = x
			}
		}
	}
	return &FileGenerator{
		fd:             fd,
		module:         "wiresmith",
		imports:        newImportTracker("wiresmith", selfDest, destinations),
		body:           &bytes.Buffer{},
		reflectImports: newImportTracker("wiresmith", selfDest, destinations),
		reflectBody:    &bytes.Buffer{},
		fileVarName:    sanitizeFileVarName(fd.Path()),
		pointerExt:     pointerExt,
		jsontagExt:     jsontagExt,
		customnameExt:  customnameExt,
		customtypeExt:  customtypeExt,
		stdtimeExt:     stdtimeExt,
	}
}

// messageByName looks up a top-level message by its declared short name.
// Tests that build multi-message fixtures use this to address a specific
// subject.
func messageByName(t *testing.T, fd protoreflect.FileDescriptor, name string) protoreflect.MessageDescriptor {
	t.Helper()
	md := fd.Messages().ByName(protoreflect.Name(name))
	if md == nil {
		t.Fatalf("message %q not found in fixture (have %d)", name, fd.Messages().Len())
	}
	return md
}

// assertContains fails the test if want is missing from the emitted body.
// The substring form matches how the generator-test reviews usually phrase
// expectations: pin a recognisable line of generated code, not a whole block.
func assertContains(t *testing.T, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Errorf("expected body to contain %q\n--- body ---\n%s\n--- end ---", want, body)
	}
}

// assertNotContains is the negative counterpart — useful for verifying that
// an empty-input emitter stays silent (e.g. emitRegistration on a proto with
// no messages and no enums must not emit an init()).
func assertNotContains(t *testing.T, body, unwanted string) {
	t.Helper()
	if strings.Contains(body, unwanted) {
		t.Errorf("expected body NOT to contain %q\n--- body ---\n%s\n--- end ---", unwanted, body)
	}
}
