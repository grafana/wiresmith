package generator

import (
	"fmt"
	"strings"
	"testing"
)

// TestEmitHas_NoPresenceFields covers the 0-field corner: a message whose
// fields are all repeated/map/optional/oneof carries no presence bitmap and
// emits no Has methods. Anything else (an empty word array, a stray Has stub)
// would force consumers to deal with state they cannot meaningfully set.
func TestEmitHas_NoPresenceFields(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
message M {
  repeated int32 xs = 1;
  optional int32 maybe = 2;
  map<string, int32> kv = 3;
  oneof choice {
    string s = 4;
    int32 n = 5;
  }
}
`))
	md := messageByName(t, fg.fd, "M")
	if got := fg.presenceBitmapWords(md); got != 0 {
		t.Errorf("presenceBitmapWords = %d, want 0", got)
	}
	fg.emitHasMethods(md)
	if body := fg.body.String(); body != "" {
		t.Errorf("expected no Has methods, got:\n%s", body)
	}
}

// TestEmitHas_SinglePresenceField covers the 1-field path: one tracked field
// → one Has method, bit index 0 in word 0, one bitmap word.
func TestEmitHas_SinglePresenceField(t *testing.T) {
	fg := newFixtureGenerator(t, compileProtoFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
message Inner {}
message M {
  Inner x = 1;
}
`))
	md := messageByName(t, fg.fd, "M")
	if got := fg.presenceBitmapWords(md); got != 1 {
		t.Errorf("presenceBitmapWords = %d, want 1", got)
	}
	fg.emitHasMethods(md)
	body := fg.body.String()
	assertContains(t, body, "func (m *M) HasX() bool {")
	assertContains(t, body, "if m == nil {")
	assertContains(t, body, "return m.fieldsPresent[0]&(1<<0) != 0")
}

// TestEmitHas_BitmapWordCount_BoundaryAt65 pins the (n+63)/64 rounding:
// 64 fields fit in one word, 65 fields require two. This is the single
// arithmetic invariant that, if regressed, would corrupt every presence
// query on messages with 65+ tracked fields.
func TestEmitHas_BitmapWordCount_BoundaryAt65(t *testing.T) {
	for _, tc := range []struct {
		name      string
		nFields   int
		wantWords int
	}{
		{"64 fields → 1 word", 64, 1},
		{"65 fields → 2 words", 65, 2},
		{"128 fields → 2 words", 128, 2},
		{"129 fields → 3 words", 129, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b strings.Builder
			b.WriteString("syntax = \"proto3\";\npackage test.v1;\noption go_package = \"wiresmith/gen/test/v1\";\nmessage Inner {}\nmessage M {\n")
			for i := 1; i <= tc.nFields; i++ {
				fmt.Fprintf(&b, "  Inner f%d = %d;\n", i, i)
			}
			b.WriteString("}\n")
			fg := newFixtureGenerator(t, compileProtoFixture(t, b.String()))
			md := messageByName(t, fg.fd, "M")
			if got := fg.presenceBitmapWords(md); got != tc.wantWords {
				t.Errorf("presenceBitmapWords = %d, want %d", got, tc.wantWords)
			}
			// Bit assignment for the last field must land in word (n-1)/64,
			// bit (n-1)%64 — cross-checking the helper alongside the word count
			// catches the off-by-one that pure word-count assertions miss.
			fg.emitHasMethods(md)
			wantLast := fmt.Sprintf("return m.fieldsPresent[%d]&(1<<%d) != 0", (tc.nFields-1)/64, (tc.nFields-1)%64)
			assertContains(t, fg.body.String(), wantLast)
		})
	}
}

// TestEmitHas_PointerOptionExcluded confirms that a singular message field
// flagged with `(wiresmith.options.pointer) = true` is skipped from the
// bitmap: the field carries its own nil-check presence, and tracking the
// same field twice would desynchronise on the first set.
func TestEmitHas_PointerOptionExcluded(t *testing.T) {
	files := compileAllFixture(t, `
syntax = "proto3";
package test.v1;
option go_package = "wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message Inner {}
message M {
  Inner ptr = 1 [(wiresmith.options.pointer) = true];
  Inner val = 2;
}
`)
	var fd = files[0]
	for _, f := range files {
		if f.Path() == "test.proto" {
			fd = f
			break
		}
	}
	fg := newFixtureGeneratorWith(t, fd, files)
	md := messageByName(t, fd, "M")
	// Only `val` survives presence tracking; `ptr` is excluded.
	if got := fg.presenceBitmapWords(md); got != 1 {
		t.Errorf("presenceBitmapWords = %d, want 1 (val only — ptr is excluded)", got)
	}
	fg.emitHasMethods(md)
	body := fg.body.String()
	assertContains(t, body, "func (m *M) HasVal() bool {")
	assertNotContains(t, body, "func (m *M) HasPtr() bool {")
}
