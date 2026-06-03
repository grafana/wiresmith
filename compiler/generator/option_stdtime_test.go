package generator

import (
	"strings"
	"testing"
)

// expectInvalidStdtime asserts the error contains both the
// `(wiresmith.options.stdtime) placement` header and the per-field reason,
// mirroring expectInvalidPointer / expectInvalidCustomtype.
func expectInvalidStdtime(t *testing.T, err error, reasonSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "invalid (wiresmith.options.stdtime) placement") {
		t.Errorf("missing header in error: %s", msg)
	}
	if !strings.Contains(msg, reasonSubstr) {
		t.Errorf("missing reason %q in error: %s", reasonSubstr, msg)
	}
}

func TestStdtimeOption_RejectsInt32(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  int32 x = 1 [(wiresmith.options.stdtime) = true];
}
`)
	expectInvalidStdtime(t, err, "only applies to google.protobuf.Timestamp fields")
}

func TestStdtimeOption_RejectsNonTimestampMessage(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message Inner {}
message M {
  Inner x = 1 [(wiresmith.options.stdtime) = true];
}
`)
	expectInvalidStdtime(t, err, "got test.v1.Inner")
}

func TestStdtimeOption_RejectsRepeated(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
import "google/protobuf/timestamp.proto";
message M {
  repeated google.protobuf.Timestamp x = 1 [(wiresmith.options.stdtime) = true];
}
`)
	expectInvalidStdtime(t, err, "not supported on repeated fields")
}

func TestStdtimeOption_RejectsOneofVariant(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
import "google/protobuf/timestamp.proto";
message M {
  oneof choice {
    google.protobuf.Timestamp x = 1 [(wiresmith.options.stdtime) = true];
  }
}
`)
	expectInvalidStdtime(t, err, "not supported on oneof variants")
}

func TestStdtimeOption_RejectsOptional(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
import "google/protobuf/timestamp.proto";
message M {
  optional google.protobuf.Timestamp x = 1 [(wiresmith.options.stdtime) = true];
}
`)
	expectInvalidStdtime(t, err, "not supported on proto3 `optional` fields")
}

func TestStdtimeOption_RejectsMap(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
import "google/protobuf/timestamp.proto";
message M {
  map<string, google.protobuf.Timestamp> x = 1 [(wiresmith.options.stdtime) = true];
}
`)
	expectInvalidStdtime(t, err, "not supported on map fields")
}

// TestStdtimeOption_RejectsPointerCombo locks down that the two
// shape-changing options can't be combined on the same field — they would
// produce conflicting Go types (`*time.Time` vs `time.Time`) and silently
// picking one over the other would erase a user-meaningful intent. The
// rejection emits a clear "pick one" message instead.
func TestStdtimeOption_RejectsPointerCombo(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
import "google/protobuf/timestamp.proto";
message M {
  google.protobuf.Timestamp x = 1 [
    (wiresmith.options.stdtime) = true,
    (wiresmith.options.pointer) = true
  ];
}
`)
	expectInvalidStdtime(t, err, "cannot combine with (wiresmith.options.pointer)")
}
