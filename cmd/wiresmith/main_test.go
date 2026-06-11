package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// sep is the OS path-list separator the flag splits on (`:` on Unix,
// `;` on Windows). Hoisted into a package-level helper so every test
// uses the same form the production code does and the suite stays
// portable across both platforms.
var sep = string(os.PathListSeparator)

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

// TestProtoPathsFlag_ListSplit pins the convenience form: a single
// flag value may carry list-separated entries (`:`-joined on Unix,
// `;`-joined on Windows via os.PathListSeparator). Entries are
// appended in order so the resulting slice is indistinguishable from
// three separate --proto_path flags. Using os.PathListSeparator rather
// than a hard-coded ':' keeps Windows drive-letter paths
// (e.g. C:\proto) from being mangled into "C" + "\proto".
func TestProtoPathsFlag_ListSplit(t *testing.T) {
	p := &protoPathsFlag{}
	value := strings.Join([]string{"a", "b", "c"}, sep)
	if err := p.Set(value); err != nil {
		t.Fatalf("Set(%q): %v", value, err)
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(p.dirs, want) {
		t.Errorf("dirs = %v, want %v", p.dirs, want)
	}
}

// TestProtoPathsFlag_MixedRepeatAndSplit pins that the two forms can be
// freely combined; both end up in one ordered slice. This is the path
// a Makefile that builds --proto_path arguments from several sources
// (some single, some list-joined) will take.
func TestProtoPathsFlag_MixedRepeatAndSplit(t *testing.T) {
	p := &protoPathsFlag{}
	values := []string{"first", "second" + sep + "third", "fourth"}
	for _, v := range values {
		if err := p.Set(v); err != nil {
			t.Fatalf("Set(%q): %v", v, err)
		}
	}
	want := []string{"first", "second", "third", "fourth"}
	if !reflect.DeepEqual(p.dirs, want) {
		t.Errorf("dirs = %v, want %v", p.dirs, want)
	}
}

// TestProtoPathsFlag_PreservesDriveLetter pins the platform-portable
// split: a value that on Unix happens to contain ':' inside one path
// (rare in practice but possible) or on Windows contains a drive
// letter must stay intact when it is the only separator-shaped
// character in the value. Without os.PathListSeparator the Windows
// invocation `--proto_path=C:\proto` would split into `C` and `\proto`
// and silently misroute the walk.
func TestProtoPathsFlag_PreservesDriveLetter(t *testing.T) {
	p := &protoPathsFlag{}
	// Pick a value that contains the *other* platform's separator so
	// the split must leave it alone. On Unix that's ';'; on Windows
	// that's ':'. The value is deliberately separator-free in the
	// production split semantics on whichever platform runs the test.
	otherSep := ":"
	if sep == ":" {
		otherSep = ";"
	}
	value := "root" + otherSep + "sub"
	if err := p.Set(value); err != nil {
		t.Fatalf("Set(%q): %v", value, err)
	}
	want := []string{value}
	if !reflect.DeepEqual(p.dirs, want) {
		t.Errorf("dirs = %v, want %v (separator on this platform is %q)", p.dirs, want, sep)
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

// TestProtoPathsFlag_RejectsEmptyEntry pins that the list-split form
// rejects an embedded empty entry. `a::b` (or `a;;b` on Windows) is
// almost always a typo (extra separator, an undefined env var
// expanding to ""), and silently dropping the empty entry would mask
// the typo.
func TestProtoPathsFlag_RejectsEmptyEntry(t *testing.T) {
	p := &protoPathsFlag{}
	value := "a" + sep + sep + "b"
	if err := p.Set(value); err == nil {
		t.Errorf("Set(%q) must error on the empty middle entry; got nil", value)
	}
}

// TestProtoPathsFlag_SeedReplacedByUser pins the default-seeding contract:
// main() seeds the flag with the historical "proto" root so `-h` reports a
// meaningful default, and the first user-supplied --proto_path/-I must *replace*
// that seed rather than append to it (otherwise every explicit invocation would
// silently also walk "proto").
func TestProtoPathsFlag_SeedReplacedByUser(t *testing.T) {
	p := &protoPathsFlag{dirs: []string{"proto"}}
	if got := p.String(); got != "proto" {
		t.Errorf("seeded String() = %q, want %q (must surface as the -h default)", got, "proto")
	}
	if err := p.Set("custom"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	want := []string{"custom"}
	if !reflect.DeepEqual(p.dirs, want) {
		t.Errorf("dirs = %v, want %v (seed must be replaced, not appended)", p.dirs, want)
	}
}

// TestProtoPathsFlag_String_RoundTrips pins that String() echoes the
// accumulated entries in a form the user can paste back as a single
// flag value (using the OS list separator). This is what
// `flag.PrintDefaults()` and any usage dump display.
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
	if got, want := p.String(), "a"+sep+"b"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
