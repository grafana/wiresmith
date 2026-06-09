package generator

import (
	"strings"
	"testing"
)

// expectInvalidCasttype asserts the error contains both the
// `(wiresmith.options.casttype) placement` header and the per-field reason,
// mirroring expectInvalidCustomtype.
func expectInvalidCasttype(t *testing.T, err error, reasonSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "invalid (wiresmith.options.casttype) placement") {
		t.Errorf("missing header in error: %s", msg)
	}
	if !strings.Contains(msg, reasonSubstr) {
		t.Errorf("missing reason %q in error: %s", reasonSubstr, msg)
	}
}

func TestCasttypeOption_RejectsMessage(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message Inner {}
message M {
  Inner x = 1 [(wiresmith.options.casttype) = "MyAlias"];
}
`)
	expectInvalidCasttype(t, err, "not supported on message fields")
}

func TestCasttypeOption_RejectsEnum(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
enum E { ZERO = 0; ONE = 1; }
message M {
  E x = 1 [(wiresmith.options.casttype) = "MyAlias"];
}
`)
	expectInvalidCasttype(t, err, "not supported on enum fields")
}

func TestCasttypeOption_RejectsFloat(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  float x = 1 [(wiresmith.options.casttype) = "MyAlias"];
}
`)
	expectInvalidCasttype(t, err, "not supported on float fields")
}

func TestCasttypeOption_RejectsDouble(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  double x = 1 [(wiresmith.options.casttype) = "MyAlias"];
}
`)
	expectInvalidCasttype(t, err, "not supported on double fields")
}

func TestCasttypeOption_RejectsRepeated(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  repeated int64 x = 1 [(wiresmith.options.casttype) = "MyAlias"];
}
`)
	expectInvalidCasttype(t, err, "not supported on repeated fields")
}

func TestCasttypeOption_RejectsOneofVariant(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  oneof choice {
    int64 x = 1 [(wiresmith.options.casttype) = "MyAlias"];
  }
}
`)
	expectInvalidCasttype(t, err, "not supported on oneof variants")
}

func TestCasttypeOption_RejectsOptional(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  optional int64 x = 1 [(wiresmith.options.casttype) = "MyAlias"];
}
`)
	expectInvalidCasttype(t, err, "not supported on proto3 `optional` fields")
}

func TestCasttypeOption_RejectsMap(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  map<string, int64> x = 1 [(wiresmith.options.casttype) = "MyAlias"];
}
`)
	expectInvalidCasttype(t, err, "not supported on map fields")
}

// TestCasttypeOption_RejectsMalformedValue mirrors the customtype path —
// since parseCustomtypeValue is shared between the two options, the same
// malformed-input messages surface here.
func TestCasttypeOption_RejectsMalformedValue(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  int64 x = 1 [(wiresmith.options.casttype) = "github.com/foo/bar"];
}
`)
	expectInvalidCasttype(t, err, "import path is missing a \".TypeName\" suffix")
}
