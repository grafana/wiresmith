package generator

import (
	"strings"
	"testing"
)

// TestEmitRegistration_EmptyFile pins the early-return: a proto with no
// messages and no enums emits nothing into reflectBody. Without this guard,
// the file writer in generateFile would emit a _reflect.pb.go containing
// just the package clause — an empty file that consumers cannot import or
// link against.
func TestEmitRegistration_EmptyFile(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
`))
	fg.emitRegistration(fg.fd)
	if got := fg.reflectBody.String(); got != "" {
		t.Errorf("expected reflectBody empty for empty proto, got:\n%s", got)
	}
}

// TestEmitRegistration_OnlyEnums covers the messageless path: a file with
// enums-but-no-messages must still emit the init() and enumTypes array, but
// must NOT emit a msgTypes array — declaring a 0-length array would compile
// but every other emit (ProtoReflect dispatch, descriptor lookup) reads it
// as "no messages registered", which is correct, but the array itself adds
// dead bytes to the binary.
func TestEmitRegistration_OnlyEnums(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
enum Color {
  COLOR_UNKNOWN = 0;
  COLOR_RED = 1;
}
`))
	fg.emitRegistration(fg.fd)
	body := fg.reflectBody.String()
	assertContains(t, body, "func init() {")
	assertContains(t, body, "_enumTypes = make([]protoimpl.EnumInfo, 1)")
	assertNotContains(t, body, "_msgTypes = make([]protoimpl.MessageInfo")
}

// TestEmitRegistration_AllowAliasEnumDedup pins the requirement that the
// generated `_name` map (int32→string) for an enum with `option allow_alias`
// does NOT contain duplicate keys. Multiple names with the same number are
// legal under allow_alias, but emitting them as duplicate map keys would
// fail Go compilation. emit_enum.go owns this dedup; the registration path
// reuses the same emitter via emitAllEnums.
func TestEmitRegistration_AllowAliasEnumDedup(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
enum Status {
  option allow_alias = true;
  STATUS_UNKNOWN = 0;
  STATUS_OK = 1;
  STATUS_GOOD = 1;
}
`))
	fg.emitAllEnums(fg.fd)
	body := fg.body.String()
	// Name map must have one entry per unique number — STATUS_OK and
	// STATUS_GOOD both share number 1, only the first survives.
	nameMap := extractBlock(t, body, "var Status_name = map[int32]string{", "}")
	if got := strings.Count(nameMap, "1:"); got != 1 {
		t.Errorf("Status_name has %d entries for number 1, want 1 (alias dedup)\n%s", got, nameMap)
	}
	// Value map intentionally keeps both: string keys are unique, no
	// compile error, and reverse lookups (`Status_value["STATUS_GOOD"]`)
	// must work.
	valueMap := extractBlock(t, body, "var Status_value = map[string]int32{", "}")
	assertContains(t, valueMap, `"STATUS_OK": 1`)
	assertContains(t, valueMap, `"STATUS_GOOD": 1`)
}

// extractBlock returns body's text from the first occurrence of start through
// the next occurrence of end. Caller uses this to limit substring/count checks
// to one map literal instead of searching the whole emitted file.
func extractBlock(t *testing.T, body, start, end string) string {
	t.Helper()
	s := strings.Index(body, start)
	if s < 0 {
		t.Fatalf("block start %q not found in body", start)
	}
	rest := body[s:]
	e := strings.Index(rest, end)
	if e < 0 {
		t.Fatalf("block end %q not found after %q", end, start)
	}
	return rest[:e+len(end)]
}
