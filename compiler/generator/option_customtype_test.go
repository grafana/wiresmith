package generator

import (
	"strings"
	"testing"
)

// expectInvalidCustomtype asserts the error contains the header plus the
// expected per-field reason, mirroring expectInvalidPointer.
func expectInvalidCustomtype(t *testing.T, err error, reasonSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "invalid (wiresmith.options.customtype) placement") {
		t.Errorf("missing header in error: %s", msg)
	}
	if !strings.Contains(msg, reasonSubstr) {
		t.Errorf("missing reason %q in error: %s", reasonSubstr, msg)
	}
}

func TestCustomtypeOption_RejectsInt32(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  int32 x = 1 [(wiresmith.options.customtype) = "example.com/foo.Bar"];
}
`)
	expectInvalidCustomtype(t, err, "only applies to singular bytes or string fields")
}

func TestCustomtypeOption_RejectsMessage(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message Inner {}
message M {
  Inner x = 1 [(wiresmith.options.customtype) = "example.com/foo.Bar"];
}
`)
	expectInvalidCustomtype(t, err, "only applies to singular bytes or string fields")
}

func TestCustomtypeOption_RejectsRepeated(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  repeated bytes x = 1 [(wiresmith.options.customtype) = "example.com/foo.Bar"];
}
`)
	expectInvalidCustomtype(t, err, "not supported on repeated fields")
}

func TestCustomtypeOption_RejectsOneofVariant(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  oneof choice {
    bytes x = 1 [(wiresmith.options.customtype) = "example.com/foo.Bar"];
  }
}
`)
	expectInvalidCustomtype(t, err, "not supported on oneof variants")
}

func TestCustomtypeOption_RejectsOptional(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  optional bytes x = 1 [(wiresmith.options.customtype) = "example.com/foo.Bar"];
}
`)
	expectInvalidCustomtype(t, err, "not supported on proto3 `optional` fields")
}

func TestCustomtypeOption_RejectsMalformedValue(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message M {
  bytes x = 1 [(wiresmith.options.customtype) = "github.com/foo/bar."];
}
`)
	expectInvalidCustomtype(t, err, "type name is empty")
}

// TestParseCustomtypeValue exercises the split rules directly. Covers the
// edge cases that are easy to get wrong: dots in import-path host names,
// same-package types with no path, and trailing/leading dots.
func TestParseCustomtypeValue(t *testing.T) {
	cases := []struct {
		in         string
		wantPath   string
		wantType   string
		wantErr    bool
		errSubstr  string
		errMessage string
	}{
		{in: "github.com/foo/bar.LabelAdapter", wantPath: "github.com/foo/bar", wantType: "LabelAdapter"},
		{in: "example.com/pkg.Type", wantPath: "example.com/pkg", wantType: "Type"},
		{in: "MyType", wantPath: "", wantType: "MyType"},
		{in: "github.com/foo/bar.", wantErr: true, errSubstr: "type name is empty"},
		{in: ".LabelAdapter", wantErr: true, errSubstr: "package segment is empty"},
		{in: "", wantErr: true, errSubstr: "must not be empty"},
		{in: "github.com/foo/bar.1Bad", wantErr: true, errSubstr: "must start with a letter"},
		// Path base names that aren't valid Go identifiers must be rejected
		// up front — leaving them to surface as a generated-file compile
		// error would lose the source of the typo.
		{in: "github.com/foo/bar-baz.Type", wantErr: true, errSubstr: "package alias derived from import path"},
		{in: "github.com/foo/123x.Type", wantErr: true, errSubstr: "package alias derived from import path"},
		// A slashed value with no ".TypeName" suffix mustn't silently fall
		// back to same-package "bar" — that would convert an import-path
		// typo into a "local type does not exist" compile error far from
		// the source. Reject explicitly.
		{in: "github.com/foo/bar", wantErr: true, errSubstr: "missing a \".TypeName\" suffix"},
		// Whitespace anywhere in the value (in the path prefix in
		// particular — validateGoIdentifier doesn't see it) would survive
		// into the emitted import statement and fail at `go build`. Reject
		// at parse time so the error points at the source.
		{in: "github.com/ foo/bar.Type", wantErr: true, errSubstr: "must not contain whitespace"},
		{in: "github.com/foo/bar.Type\n", wantErr: true, errSubstr: "must not contain whitespace"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			gotPath, gotType, err := parseCustomtypeValue(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (path=%q, type=%q)", gotPath, gotType)
				}
				if !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("expected error containing %q, got: %v", tc.errSubstr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotPath != tc.wantPath || gotType != tc.wantType {
				t.Errorf("got (%q, %q), want (%q, %q)", gotPath, gotType, tc.wantPath, tc.wantType)
			}
		})
	}
}
