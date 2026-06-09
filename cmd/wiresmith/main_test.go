package main

import (
	"reflect"
	"testing"
)

// TestProtoPathsFlag_Repeatable verifies the most common --proto_path
// usage path: one flag occurrence per root, preserving call order. The
// order matters for error messages (multi-root collision lists both
// candidates) but is irrelevant for import resolution (collisions are
// errors, not first-wins).
func TestProtoPathsFlag_Repeatable(t *testing.T) {
	p := &protoPathsFlag{}
	for _, v := range []string{"a", "b/sub", "c"} {
		if err := p.Set(v); err != nil {
			t.Fatalf("Set(%q): %v", v, err)
		}
	}
	want := []string{"a", "b/sub", "c"}
	if !reflect.DeepEqual(p.dirs, want) {
		t.Errorf("dirs = %v, want %v", p.dirs, want)
	}
}

// TestProtoPathsFlag_ColonSplit pins the convenience form: a single
// flag value may carry ':'-separated entries, matching protoc's
// '-I=a:b:c' shorthand. Entries are appended in order so the
// resulting slice is indistinguishable from three separate --proto_path
// flags.
func TestProtoPathsFlag_ColonSplit(t *testing.T) {
	p := &protoPathsFlag{}
	if err := p.Set("a:b:c"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(p.dirs, want) {
		t.Errorf("dirs = %v, want %v", p.dirs, want)
	}
}

// TestProtoPathsFlag_MixedRepeatAndSplit pins that the two forms can be
// freely combined; both end up in one ordered slice. This is the path
// a Makefile that builds --proto_path arguments from several sources
// (some single, some colon-joined) will take.
func TestProtoPathsFlag_MixedRepeatAndSplit(t *testing.T) {
	p := &protoPathsFlag{}
	for _, v := range []string{"first", "second:third", "fourth"} {
		if err := p.Set(v); err != nil {
			t.Fatalf("Set(%q): %v", v, err)
		}
	}
	want := []string{"first", "second", "third", "fourth"}
	if !reflect.DeepEqual(p.dirs, want) {
		t.Errorf("dirs = %v, want %v", p.dirs, want)
	}
}

// TestProtoPathsFlag_RejectsEmpty pins that the flag refuses an empty
// value rather than silently registering an empty string as a "root".
// An empty proto_path would later surface as a confusing
// "directory does not exist" on path "" — fail loudly here instead.
func TestProtoPathsFlag_RejectsEmpty(t *testing.T) {
	p := &protoPathsFlag{}
	if err := p.Set(""); err == nil {
		t.Error("Set(\"\") must error; got nil")
	}
}

// TestProtoPathsFlag_RejectsEmptyEntry pins that the colon-split form
// rejects an embedded empty entry. `a::b` is almost always a typo
// (extra colon, an undefined env var expanding to ""), and silently
// dropping the empty entry would mask the typo.
func TestProtoPathsFlag_RejectsEmptyEntry(t *testing.T) {
	p := &protoPathsFlag{}
	if err := p.Set("a::b"); err == nil {
		t.Error("Set(\"a::b\") must error on the empty middle entry; got nil")
	}
}

// TestProtoPathsFlag_String_RoundTrips pins that String() echoes the
// accumulated entries in a form the user can paste back as a single
// colon-joined flag value. This is what `flag.PrintDefaults()` and any
// usage dump display.
func TestProtoPathsFlag_String_RoundTrips(t *testing.T) {
	p := &protoPathsFlag{}
	if got := p.String(); got != "" {
		t.Errorf("empty flag String() = %q, want %q", got, "")
	}
	for _, v := range []string{"a", "b"} {
		if err := p.Set(v); err != nil {
			t.Fatalf("Set(%q): %v", v, err)
		}
	}
	if got, want := p.String(), "a:b"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
