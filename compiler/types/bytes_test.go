package types

import (
	"strings"
	"testing"
)

func TestBytesType_Classification(t *testing.T) {
	b := BytesType{}
	if got := b.WireType(); got != "protowire.BytesType" {
		t.Errorf("WireType() = %q, want protowire.BytesType", got)
	}
	if b.IsPackable() {
		t.Error("IsPackable() = true, want false (bytes are length-delimited, never packed)")
	}
	if b.IsFixed32() || b.IsFixed64() {
		t.Error("bytes must not classify as fixed-width")
	}
	if got := b.FixedSize(); got != 0 {
		t.Errorf("FixedSize() = %d, want 0 (variable-length)", got)
	}
	if b.SizeByIndex() {
		t.Error("SizeByIndex() = true, want false")
	}
	if got := b.OptionalAccess("x"); got != "x" {
		t.Errorf("OptionalAccess(%q) = %q, want %q (bytes []byte is already nullable)", "x", got, "x")
	}
	if got := b.ZeroLiteral(); got != "nil" {
		t.Errorf("ZeroLiteral() = %q, want %q", got, "nil")
	}
}

func TestBytesType_VarintSizeExpr_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for non-packable VarintSizeExpr, got none")
		}
	}()
	BytesType{}.VarintSizeExpr("x")
}

func TestBytesType_EmitSize(t *testing.T) {
	e := &captureEmitter{}
	BytesType{}.EmitSize(e, "m.Data", 1)
	want := "\tif len(m.Data) > 0 {\n\t\tn += 1 + protowire.SizeVarint(uint64(len(m.Data))) + len(m.Data)\n\t}\n"
	if got := e.buf.String(); got != want {
		t.Errorf("EmitSize:\n got: %q\nwant: %q", got, want)
	}
}

func TestBytesType_EmitMarshal(t *testing.T) {
	e := &captureEmitter{}
	BytesType{}.EmitMarshal(e, "m.Data", 3)
	got := e.buf.String()
	// Reverse-write order: copy payload, then write length varint, then tag.
	for _, sub := range []string{
		"if len(m.Data) > 0 {",
		"i -= len(m.Data)",
		"copy(dAtA[i:], m.Data)",
		"i = protohelpers.EncodeVarint(dAtA, i, uint64(len(m.Data)))",
	} {
		if !strings.Contains(got, sub) {
			t.Errorf("EmitMarshal missing %q in:\n%s", sub, got)
		}
	}
}

// EmitUnmarshal uses append(varName[:0], ...) for singular fields — the
// receiver's existing backing array is reused. The bytes-aliasing guard
// (a fresh []byte(nil)) is required only for map values; the singular
// path is safe because each unmarshal writes into a distinct field.
func TestBytesType_EmitUnmarshal_SingularReusesBacking(t *testing.T) {
	e := &captureEmitter{}
	BytesType{}.EmitUnmarshal(e, "m.Data", FieldContext{})
	got := e.buf.String()
	want := "\t\t\tm.Data = append(m.Data[:0], dAtA[iNdEx:postIndex]...)\n"
	if !strings.Contains(got, want) {
		t.Errorf("EmitUnmarshal singular path:\n got: %q\nwant substring: %q", got, want)
	}
}

// EmitMapEntryUnmarshal MUST allocate a fresh []byte per map entry. Reusing a
// backing array via `append(varName[:0], ...)` would corrupt previously stored
// entries because the map value shares storage with the next decoded payload.
// See CLAUDE.md → "Map field correctness → Bytes aliasing".
func TestBytesType_EmitMapEntryUnmarshal_FreshSlicePerEntry(t *testing.T) {
	e := &captureEmitter{}
	BytesType{}.EmitMapEntryUnmarshal(e, "mapvalue", "\t\t\t\t\t", FieldContext{})
	got := e.buf.String()
	want := "mapvalue = append([]byte(nil), dAtA[iNdEx:postIndex]...)"
	if !strings.Contains(got, want) {
		t.Errorf("EmitMapEntryUnmarshal: missing fresh-slice allocation %q in:\n%s", want, got)
	}
	// Ensure the unsafe `append(mapvalue[:0], ...)` form is NOT emitted.
	bad := "mapvalue = append(mapvalue[:0],"
	if strings.Contains(got, bad) {
		t.Errorf("EmitMapEntryUnmarshal: emits bytes-aliasing pattern %q which would corrupt prior map entries:\n%s", bad, got)
	}
}

// CastExpr must produce the same fresh-slice form so packed/repeated paths
// that go through it (rather than EmitUnmarshal) still get isolation.
func TestBytesType_CastExpr_AllocatesFreshSlice(t *testing.T) {
	got := BytesType{}.CastExpr("dAtA[iNdEx:postIndex]", FieldContext{})
	want := "append([]byte(nil), dAtA[iNdEx:postIndex]...)"
	if got != want {
		t.Errorf("CastExpr = %q, want %q", got, want)
	}
}
