package generator

import (
	"strings"
	"testing"
)

// TestJsontagOption_RejectsBacktick pins the only validation rule for
// (wiresmith.options.jsontag): a backtick in the value would terminate the
// raw-string struct tag emitted by fieldTag/mapFieldTag and produce code that
// does not compile. The check must catch it before emit time.
func TestJsontagOption_RejectsBacktick(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  string x = 1 [(wiresmith.options.jsontag) = "bad`+"`"+`tick"];
}
`)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "invalid (wiresmith.options.jsontag) value") {
		t.Errorf("missing header in error: %s", msg)
	}
	if !strings.Contains(msg, "must not contain backticks") {
		t.Errorf("missing reason in error: %s", msg)
	}
}

// TestJsontagOption_AcceptsEmpty pins that the empty-string override is a
// valid explicit value (matches gogoproto.jsontag = "" semantics — opts the
// field out of JSON serialization). A regression here would cause the bench's
// own integration fixture to fail to generate.
func TestJsontagOption_AcceptsEmpty(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  string x = 1 [(wiresmith.options.jsontag) = ""];
}
`)
	if err != nil {
		t.Fatalf("empty jsontag must be accepted, got: %v", err)
	}
}
