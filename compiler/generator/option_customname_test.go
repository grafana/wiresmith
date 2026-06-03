package generator

import (
	"strings"
	"testing"
)

func expectInvalidCustomname(t *testing.T, err error, reasonSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "invalid (wiresmith.options.customname) value") {
		t.Errorf("missing header in error: %s", msg)
	}
	if !strings.Contains(msg, reasonSubstr) {
		t.Errorf("missing reason %q in error: %s", reasonSubstr, msg)
	}
}

func TestCustomnameOption_RejectsLowercaseFirst(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  string x = 1 [(wiresmith.options.customname) = "lowercase"];
}
`)
	expectInvalidCustomname(t, err, "must start with an uppercase letter")
}

func TestCustomnameOption_RejectsEmpty(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  string x = 1 [(wiresmith.options.customname) = ""];
}
`)
	expectInvalidCustomname(t, err, "must not be empty")
}

func TestCustomnameOption_RejectsInvalidCharacter(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  string x = 1 [(wiresmith.options.customname) = "Bad-Name"];
}
`)
	expectInvalidCustomname(t, err, "invalid character")
}

func TestCustomnameOption_AcceptsValidIdentifier(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  string x = 1 [(wiresmith.options.customname) = "BlockID"];
}
`)
	if err != nil {
		t.Fatalf("valid customname must be accepted, got: %v", err)
	}
}

// TestCustomnameOption_RejectsNonASCIIFirstByteBug pins the UTF-8 first-rune
// decode: a single byte of a multi-byte uppercase letter (Σ = 0xCE 0xA3) is
// a continuation byte that unicode.IsUpper would not accept. The validator
// must decode the first *rune* via range iteration, not cast value[0].
// Without that fix, "Σigma" would be misclassified as "doesn't start with
// uppercase" even though it does.
func TestCustomnameOption_AcceptsNonASCIIUppercaseStart(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  string x = 1 [(wiresmith.options.customname) = "Σigma"];
}
`)
	if err != nil {
		t.Fatalf("valid Unicode-uppercase identifier must be accepted, got: %v", err)
	}
}

// TestCustomnameOption_RejectsReservedMethodName pins that a customname
// matching an always-emitted method name (e.g. Reset) is rejected. A struct
// field of that name would shadow the method and produce an "ambiguous
// selector" compile error far from the offending .proto.
func TestCustomnameOption_RejectsReservedMethodName(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  string x = 1 [(wiresmith.options.customname) = "Reset"];
}
`)
	expectInvalidCustomname(t, err, "collides with an always-generated method")
}

// TestCustomnameOption_RejectsDuplicateGoName checks the in-message
// collision: two fields resolving to the same Go identifier (one via
// customname, the other via default snake_to_PascalCase) would emit a
// struct with two fields of the same name and fail compilation.
func TestCustomnameOption_RejectsDuplicateGoName(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  string foo = 1;
  string bar = 2 [(wiresmith.options.customname) = "Foo"];
}
`)
	expectInvalidCustomname(t, err, "resolved Go name \"Foo\" collides")
}

// TestCustomnameOption_RejectsTwoCustomnameCollision is the
// customname/customname variant of the duplicate check.
func TestCustomnameOption_RejectsTwoCustomnameCollision(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  string a = 1 [(wiresmith.options.customname) = "Same"];
  string b = 2 [(wiresmith.options.customname) = "Same"];
}
`)
	expectInvalidCustomname(t, err, "resolved Go name \"Same\" collides")
}

// TestCustomnameOption_AcceptsAllFieldKinds is the positive counterpart —
// the option works on every kind, unlike pointer/customtype which restrict
// placement.
func TestCustomnameOption_AcceptsAllFieldKinds(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message Inner {}
message M {
  string scalar = 1 [(wiresmith.options.customname) = "Scalar"];
  Inner msg = 2 [(wiresmith.options.customname) = "Msg"];
  repeated int32 nums = 3 [(wiresmith.options.customname) = "Nums"];
  map<string,int64> mapping = 4 [(wiresmith.options.customname) = "Mapping"];
  oneof choice {
    string variant = 5 [(wiresmith.options.customname) = "Variant"];
  }
  optional bool opt = 6 [(wiresmith.options.customname) = "Opt"];
}
`)
	if err != nil {
		t.Fatalf("customname must be accepted on every field kind, got: %v", err)
	}
}
