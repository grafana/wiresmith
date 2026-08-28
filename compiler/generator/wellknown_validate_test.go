package generator

import (
	"strings"
	"testing"
)

// expectUnsupportedWKT asserts the error carries both the well-known-type
// header and the per-field reason. Mirrors expectInvalidStdtime /
// expectInvalidPointer.
func expectUnsupportedWKT(t *testing.T, err error, reasonSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unsupported well-known type field") {
		t.Errorf("missing header in error: %s", msg)
	}
	if !strings.Contains(msg, reasonSubstr) {
		t.Errorf("missing reason %q in error: %s", reasonSubstr, msg)
	}
}

// TestValidateWellKnownFields_RejectsUnannotatedTimestamp locks down the
// wiresmith-2xxx-class bug this validator exists to catch: without this
// check, a plain `google.protobuf.Timestamp` field (no stdtime option)
// resolves to the official timestamppb.Timestamp Go type while wiresmith
// still emits calls like .Size()/.MarshalToSizedBuffer() on it — those
// don't exist on the official type, so the failure previously only
// surfaced at `go build`, far from the actual mistake.
func TestValidateWellKnownFields_RejectsUnannotatedTimestamp(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "google/protobuf/timestamp.proto";
message M {
  google.protobuf.Timestamp x = 1;
}
`)
	expectUnsupportedWKT(t, err, "without (wiresmith.options.stdtime)")
}

// TestValidateWellKnownFields_RejectsUnannotatedOptionalTimestamp is the
// exact shape reported against wiresmith: a client_model-style
// `optional google.protobuf.Timestamp created_timestamp` with no stdtime
// annotation. Same failure mode as the unannotated singular case above.
func TestValidateWellKnownFields_RejectsUnannotatedOptionalTimestamp(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "google/protobuf/timestamp.proto";
message M {
  optional google.protobuf.Timestamp created_timestamp = 1;
}
`)
	expectUnsupportedWKT(t, err, "without (wiresmith.options.stdtime)")
}

func TestValidateWellKnownFields_RejectsUnannotatedDuration(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "google/protobuf/duration.proto";
message M {
  google.protobuf.Duration x = 1;
}
`)
	expectUnsupportedWKT(t, err, "without (wiresmith.options.stdduration)")
}

func TestValidateWellKnownFields_RejectsEmpty(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "google/protobuf/empty.proto";
message M {
  google.protobuf.Empty x = 1;
}
`)
	expectUnsupportedWKT(t, err, "google.protobuf.Empty")
}

func TestValidateWellKnownFields_RejectsStructValue(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "google/protobuf/struct.proto";
message M {
  google.protobuf.Struct x = 1;
}
`)
	expectUnsupportedWKT(t, err, "google.protobuf.Struct")
}

func TestValidateWellKnownFields_RejectsWrapperType(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "google/protobuf/wrappers.proto";
message M {
  google.protobuf.StringValue x = 1;
}
`)
	expectUnsupportedWKT(t, err, "google.protobuf.StringValue")
}

// TestValidateWellKnownFields_RejectsUnannotatedMapDuration exercises the
// map-value special case: the outer field's own Kind()/Message() describe
// the synthetic MapEntry wrapper, so the validator must look at
// MapValue() instead to catch a map whose value type is an unsupported WKT.
func TestValidateWellKnownFields_RejectsUnannotatedMapDuration(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "google/protobuf/duration.proto";
message M {
  map<string, google.protobuf.Duration> x = 1;
}
`)
	expectUnsupportedWKT(t, err, "without (wiresmith.options.stdduration)")
}

// TestValidateWellKnownFields_AcceptsAnnotatedTimestamp confirms the
// validator doesn't regress the supported path: a properly
// stdtime-annotated field must still generate cleanly.
func TestValidateWellKnownFields_AcceptsAnnotatedTimestamp(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
import "google/protobuf/timestamp.proto";
message M {
  google.protobuf.Timestamp x = 1 [(wiresmith.options.stdtime) = true];
}
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestValidateWellKnownFields_AcceptsAny confirms google.protobuf.Any
// (which has a full wiresmith-shipped replacement, see wellknown.go) is
// never flagged as an unsupported well-known type.
func TestValidateWellKnownFields_AcceptsAny(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "google/protobuf/any.proto";
message M {
  google.protobuf.Any x = 1;
}
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestValidateWellKnownFields_AcceptsPlainUserMessage is the noise control:
// a normal user-defined message field must never be flagged.
func TestValidateWellKnownFields_AcceptsPlainUserMessage(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
message Inner { int32 x = 1; }
message M {
  Inner x = 1;
  map<string, Inner> m = 2;
}
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
