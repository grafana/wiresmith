package generator

import (
	"strings"
	"testing"
)

// expectInvalidStdDuration asserts the error contains both the
// `(wiresmith.options.stdduration) placement` header and the per-field
// reason, mirroring expectInvalidStdtime.
func expectInvalidStdDuration(t *testing.T, err error, reasonSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "invalid (wiresmith.options.stdduration) placement") {
		t.Errorf("missing header in error: %s", msg)
	}
	if !strings.Contains(msg, reasonSubstr) {
		t.Errorf("missing reason %q in error: %s", reasonSubstr, msg)
	}
}

func TestStdDurationOption_RejectsInt32(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  int32 x = 1 [(wiresmith.options.stdduration) = true];
}
`)
	expectInvalidStdDuration(t, err, "only applies to google.protobuf.Duration fields")
}

func TestStdDurationOption_RejectsNonDurationMessage(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message Inner {}
message M {
  Inner x = 1 [(wiresmith.options.stdduration) = true];
}
`)
	expectInvalidStdDuration(t, err, "got test.v1.Inner")
}

func TestStdDurationOption_RejectsTimestamp(t *testing.T) {
	// stdtime, not stdduration, is the right option for Timestamp — make
	// sure a misuse on a Timestamp field is rejected with a clear message
	// rather than silently producing broken code.
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
import "google/protobuf/timestamp.proto";
message M {
  google.protobuf.Timestamp x = 1 [(wiresmith.options.stdduration) = true];
}
`)
	expectInvalidStdDuration(t, err, "got google.protobuf.Timestamp")
}

func TestStdDurationOption_RejectsRepeated(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
import "google/protobuf/duration.proto";
message M {
  repeated google.protobuf.Duration x = 1 [(wiresmith.options.stdduration) = true];
}
`)
	expectInvalidStdDuration(t, err, "not supported on repeated fields")
}

func TestStdDurationOption_RejectsOneofVariant(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
import "google/protobuf/duration.proto";
message M {
  oneof choice {
    google.protobuf.Duration x = 1 [(wiresmith.options.stdduration) = true];
  }
}
`)
	expectInvalidStdDuration(t, err, "not supported on oneof variants")
}

func TestStdDurationOption_RejectsOptional(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
import "google/protobuf/duration.proto";
message M {
  optional google.protobuf.Duration x = 1 [(wiresmith.options.stdduration) = true];
}
`)
	expectInvalidStdDuration(t, err, "not supported on proto3 `optional` fields")
}

func TestStdDurationOption_RejectsMap(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
import "google/protobuf/duration.proto";
message M {
  map<string, google.protobuf.Duration> x = 1 [(wiresmith.options.stdduration) = true];
}
`)
	expectInvalidStdDuration(t, err, "not supported on map fields")
}

// TestStdDurationOption_RejectsPointerCombo locks down that the two
// shape-changing options can't be combined on the same field — they would
// produce conflicting Go types (`*time.Duration` vs `time.Duration`) and
// silently picking one over the other would erase a user-meaningful intent.
// Mirrors TestStdtimeOption_RejectsPointerCombo.
func TestStdDurationOption_RejectsPointerCombo(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
import "google/protobuf/duration.proto";
message M {
  google.protobuf.Duration x = 1 [
    (wiresmith.options.stdduration) = true,
    (wiresmith.options.pointer) = true
  ];
}
`)
	expectInvalidStdDuration(t, err, "cannot combine with (wiresmith.options.pointer)")
}
