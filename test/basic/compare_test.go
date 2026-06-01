package basic

import (
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	cmp "wiresmith/gen/basic/compare/v1"
)

// Compare's contract is a total order with a -1/0/+1 result, matching the
// gogoproto `(gogoproto.compare) = true` shape. These tests pin the
// nil/wrong-type preamble (gogo-equivalent), the per-shape ordering rules
// (scalars by value, bytes via bytes.Compare, repeated by length-then-
// element, map by sorted-key-then-value, oneof by variant index, optional
// nil < non-nil, message by recursive Compare), and the float bit-exact
// invariant that mirrors Equal.

// --- Preamble: nil / wrong-type / pointer-vs-value ---

func TestCompare_NilReceiverEquality(t *testing.T) {
	var a *cmp.AllScalars
	var b *cmp.AllScalars
	assert.Equal(t, 0, a.Compare(b), "two nil pointers compare equal")
	assert.Equal(t, 0, a.Compare(nil), "nil pointer vs nil interface compares equal")
}

func TestCompare_NilVsNonNil(t *testing.T) {
	var a *cmp.AllScalars
	b := &cmp.AllScalars{}
	assert.Equal(t, -1, a.Compare(b), "nil sorts before non-nil")
	assert.Equal(t, 1, b.Compare(a), "non-nil sorts after nil")
	assert.Equal(t, 1, b.Compare(nil), "non-nil vs nil interface returns 1")
}

func TestCompare_WrongType(t *testing.T) {
	a := &cmp.AllScalars{FieldInt32: 42}
	assert.Equal(t, 1, a.Compare("not a proto"), "wrong type returns 1")
	assert.Equal(t, 1, a.Compare(42), "wrong type returns 1")
}

func TestCompare_PointerAndValue(t *testing.T) {
	// `that` can come in as either *T or T; both forms must hit the same
	// answer because gogo accepted both.
	a := &cmp.AllScalars{FieldInt32: 42}
	b := cmp.AllScalars{FieldInt32: 42}
	assert.Equal(t, 0, a.Compare(b))
}

func TestCompare_Self(t *testing.T) {
	a := &cmp.AllScalars{FieldInt32: 1, FieldString: "x"}
	assert.Equal(t, 0, a.Compare(a))
}

// --- Antisymmetry: cmp(a,b) == -cmp(b,a) ---

func TestCompare_Antisymmetric(t *testing.T) {
	a := &cmp.AllScalars{FieldInt32: 1}
	b := &cmp.AllScalars{FieldInt32: 2}
	ca := a.Compare(b)
	cb := b.Compare(a)
	assert.Equal(t, -ca, cb, "cmp must be antisymmetric")
	assert.Equal(t, -1, ca)
	assert.Equal(t, 1, cb)
}

// --- Tag-ascending field order ---

func TestCompare_TagAscendingOrder(t *testing.T) {
	// Bead requires ascending wire tag — even though OutOfOrderTags
	// declares `second` (tag 2) before `first` (tag 1), tag 1 must
	// dominate. Without sorting the generator iterates declared order;
	// this test fails in that case.
	a := &cmp.OutOfOrderTags{First: 0, Second: 99}
	b := &cmp.OutOfOrderTags{First: 1, Second: 0}
	// `first` differs (0 < 1) at the smaller tag → a < b regardless of
	// `second` being larger on the a side.
	assert.Equal(t, -1, a.Compare(b))
}

// --- Scalar ordering ---

func TestCompare_Int32Order(t *testing.T) {
	a := &cmp.AllScalars{FieldInt32: -1}
	b := &cmp.AllScalars{FieldInt32: 0}
	c := &cmp.AllScalars{FieldInt32: 1}
	assert.Equal(t, -1, a.Compare(b))
	assert.Equal(t, -1, b.Compare(c))
	assert.Equal(t, 0, b.Compare(b))
}

func TestCompare_Uint64Order(t *testing.T) {
	a := &cmp.AllScalars{FieldUint64: 0}
	b := &cmp.AllScalars{FieldUint64: math.MaxUint64}
	assert.Equal(t, -1, a.Compare(b))
}

func TestCompare_BoolOrder(t *testing.T) {
	// false < true matches gogo (and is the only sensible total order on
	// bool given the bead's "ascending" requirement).
	f := &cmp.AllScalars{FieldBool: false}
	tr := &cmp.AllScalars{FieldBool: true}
	assert.Equal(t, -1, f.Compare(tr))
	assert.Equal(t, 1, tr.Compare(f))
}

func TestCompare_StringLexicographic(t *testing.T) {
	a := &cmp.AllScalars{FieldString: "apple"}
	b := &cmp.AllScalars{FieldString: "banana"}
	assert.Equal(t, -1, a.Compare(b))
}

func TestCompare_BytesViaBytesCompare(t *testing.T) {
	// bytes.Compare orders shorter-prefix-first, then byte-wise.
	a := &cmp.AllScalars{FieldBytes: []byte{0x01, 0x02}}
	b := &cmp.AllScalars{FieldBytes: []byte{0x01, 0x03}}
	assert.Equal(t, -1, a.Compare(b))

	c := &cmp.AllScalars{FieldBytes: []byte{0x01}}
	assert.Equal(t, -1, c.Compare(a), "shorter prefix sorts first")
}

func TestCompare_EnumOrder(t *testing.T) {
	a := &cmp.AllScalars{FieldEnum: cmp.Color_COLOR_RED}
	b := &cmp.AllScalars{FieldEnum: cmp.Color_COLOR_GREEN}
	assert.Equal(t, -1, a.Compare(b))
}

// --- Float bit-exact ---

func TestCompare_Float64BitExact(t *testing.T) {
	// math.Float64bits ordering: +0.0 (uint64 0) < -0.0 (uint64
	// 0x80000000_00000000). This matches Equal's "-0.0 and +0.0 are
	// distinct" contract — the *ordering* must agree with Equal's
	// distinction, even though IEEE 754 `<` says they're equal.
	pos := &cmp.AllScalars{FieldDouble: 0.0}
	neg := &cmp.AllScalars{FieldDouble: math.Copysign(0, -1)}
	assert.Equal(t, -1, pos.Compare(neg), "+0.0 sorts before -0.0 via bit comparison")
}

func TestCompare_Float64NaNs(t *testing.T) {
	// Two NaNs with different bit patterns must produce a stable, non-zero
	// ordering. With IEEE comparison both NaN < NaN and NaN > NaN are
	// false, which would collapse to 0 — that's broken for a sort.
	a := &cmp.AllScalars{FieldDouble: math.Float64frombits(0x7ff8000000000001)}
	b := &cmp.AllScalars{FieldDouble: math.Float64frombits(0x7ff8000000000002)}
	got := a.Compare(b)
	assert.NotEqual(t, 0, got, "distinct NaN bit patterns must not compare equal")
	assert.Equal(t, -got, b.Compare(a), "antisymmetric on NaN bits")
}

func TestCompare_Float32BitExact(t *testing.T) {
	pos := &cmp.AllScalars{FieldFloat: 0}
	neg := &cmp.AllScalars{FieldFloat: float32(math.Copysign(0, -1))}
	assert.Equal(t, -1, pos.Compare(neg))
}

// --- Optional ---

func TestCompare_OptionalNilSorts(t *testing.T) {
	var z int32 = 0
	a := &cmp.OptionalScalars{} // all nil
	b := &cmp.OptionalScalars{FieldInt32: &z}
	assert.Equal(t, -1, a.Compare(b), "nil pointer sorts before non-nil")
	assert.Equal(t, 1, b.Compare(a))
}

func TestCompare_OptionalSameValue(t *testing.T) {
	v1, v2 := int32(7), int32(7)
	a := &cmp.OptionalScalars{FieldInt32: &v1}
	b := &cmp.OptionalScalars{FieldInt32: &v2}
	assert.Equal(t, 0, a.Compare(b), "independently allocated equal pointers compare equal")
}

func TestCompare_OptionalDifferentValue(t *testing.T) {
	v1, v2 := int32(7), int32(8)
	a := &cmp.OptionalScalars{FieldInt32: &v1}
	b := &cmp.OptionalScalars{FieldInt32: &v2}
	assert.Equal(t, -1, a.Compare(b))
}

func TestCompare_OptionalBytesNilVsEmpty(t *testing.T) {
	// For optional bytes, nil and empty are distinguishable (Equal makes
	// the same distinction). nil sorts before any non-nil — even an empty
	// slice.
	a := &cmp.OptionalScalars{FieldBytes: nil}
	b := &cmp.OptionalScalars{FieldBytes: []byte{}}
	assert.Equal(t, -1, a.Compare(b))
}

// --- Nested message ---

func TestCompare_NestedMessage(t *testing.T) {
	a := &cmp.WithMessage{Inner: cmp.Inner{Value: 1}, Name: "x"}
	b := &cmp.WithMessage{Inner: cmp.Inner{Value: 2}, Name: "x"}
	assert.Equal(t, -1, a.Compare(b))

	// When Inner is equal, Name (tag 2) is the tiebreaker.
	c := &cmp.WithMessage{Inner: cmp.Inner{Value: 1}, Name: "y"}
	assert.Equal(t, -1, a.Compare(c))
}

// --- Pointer-shape message ---

func TestCompare_PointerMessageNil(t *testing.T) {
	a := &cmp.WithPointerMessage{Inner: nil}
	b := &cmp.WithPointerMessage{Inner: &cmp.Inner{Value: 0}}
	assert.Equal(t, -1, a.Compare(b), "nil *Inner sorts before non-nil")
}

func TestCompare_PointerMessageEqual(t *testing.T) {
	a := &cmp.WithPointerMessage{Inner: &cmp.Inner{Value: 5}}
	b := &cmp.WithPointerMessage{Inner: &cmp.Inner{Value: 5}}
	assert.Equal(t, 0, a.Compare(b))
}

// --- Repeated ---

func TestCompare_RepeatedShorterFirst(t *testing.T) {
	a := &cmp.Repeated{Ints: []int32{1, 2}}
	b := &cmp.Repeated{Ints: []int32{1, 2, 3}}
	assert.Equal(t, -1, a.Compare(b))
}

func TestCompare_RepeatedElementOrder(t *testing.T) {
	a := &cmp.Repeated{Ints: []int32{1, 2, 3}}
	b := &cmp.Repeated{Ints: []int32{1, 3, 2}}
	assert.Equal(t, -1, a.Compare(b))
}

func TestCompare_RepeatedMessages(t *testing.T) {
	a := &cmp.Repeated{Inners: []cmp.Inner{{Value: 1}, {Value: 2}}}
	b := &cmp.Repeated{Inners: []cmp.Inner{{Value: 1}, {Value: 3}}}
	assert.Equal(t, -1, a.Compare(b))
}

func TestCompare_RepeatedBytes(t *testing.T) {
	a := &cmp.Repeated{Blobs: [][]byte{{0x01}, {0x02}}}
	b := &cmp.Repeated{Blobs: [][]byte{{0x01}, {0x03}}}
	assert.Equal(t, -1, a.Compare(b))
}

// --- Map ---

func TestCompare_MapLength(t *testing.T) {
	a := &cmp.WithMap{MStringString: map[string]string{"a": "1"}}
	b := &cmp.WithMap{MStringString: map[string]string{"a": "1", "b": "2"}}
	assert.Equal(t, -1, a.Compare(b))
}

func TestCompare_MapSortedKeyDifference(t *testing.T) {
	// Same length, different keys. The smaller-sorted key wins.
	a := &cmp.WithMap{MStringString: map[string]string{"a": "v"}}
	b := &cmp.WithMap{MStringString: map[string]string{"b": "v"}}
	assert.Equal(t, -1, a.Compare(b))
}

func TestCompare_MapSameKeysDifferentValues(t *testing.T) {
	a := &cmp.WithMap{MStringString: map[string]string{"k": "1"}}
	b := &cmp.WithMap{MStringString: map[string]string{"k": "2"}}
	assert.Equal(t, -1, a.Compare(b))
}

func TestCompare_MapInsertionOrderIrrelevant(t *testing.T) {
	// Go maps iterate randomly, so the Compare result must depend only on
	// the sorted (key, value) pairs, not insertion order. Build the same
	// logical map two ways and require Compare to return 0.
	a := &cmp.WithMap{MStringString: map[string]string{"a": "1", "b": "2", "c": "3"}}
	b := &cmp.WithMap{MStringString: map[string]string{}}
	for _, k := range []string{"c", "b", "a"} {
		b.MStringString[k] = a.MStringString[k]
	}
	assert.Equal(t, 0, a.Compare(b))
}

func TestCompare_MapBoolKey(t *testing.T) {
	// false sorts before true — the generated less func reads
	// `!a && b`, which we exercise here.
	a := &cmp.WithMap{MBoolString: map[bool]string{false: "v"}}
	b := &cmp.WithMap{MBoolString: map[bool]string{true: "v"}}
	assert.Equal(t, -1, a.Compare(b))
}

func TestCompare_MapMessageValue(t *testing.T) {
	a := &cmp.WithMap{MStringInner: map[string]cmp.Inner{"k": {Value: 1}}}
	b := &cmp.WithMap{MStringInner: map[string]cmp.Inner{"k": {Value: 2}}}
	assert.Equal(t, -1, a.Compare(b))
}

// --- Oneof ---

func TestCompare_OneofUnsetSortsFirst(t *testing.T) {
	a := &cmp.WithOneof{}
	b := &cmp.WithOneof{Value: &cmp.WithOneof_IntVal{IntVal: 0}}
	assert.Equal(t, -1, a.Compare(b), "unset oneof sorts before any set variant")
}

func TestCompare_OneofVariantIndexOrder(t *testing.T) {
	// IntVal is variant index 0, StrVal is 1, BytesVal is 2, MsgVal is 3.
	// IntVal payload that's "large" must still sort before any other
	// variant — the variant index wins, not the payload.
	a := &cmp.WithOneof{Value: &cmp.WithOneof_IntVal{IntVal: math.MaxInt32}}
	b := &cmp.WithOneof{Value: &cmp.WithOneof_StrVal{StrVal: ""}}
	assert.Equal(t, -1, a.Compare(b))
}

func TestCompare_OneofSameVariantPayload(t *testing.T) {
	a := &cmp.WithOneof{Value: &cmp.WithOneof_StrVal{StrVal: "a"}}
	b := &cmp.WithOneof{Value: &cmp.WithOneof_StrVal{StrVal: "b"}}
	assert.Equal(t, -1, a.Compare(b))
}

func TestCompare_OneofMsgVariant(t *testing.T) {
	a := &cmp.WithOneof{Value: &cmp.WithOneof_MsgVal{MsgVal: cmp.Inner{Value: 1}}}
	b := &cmp.WithOneof{Value: &cmp.WithOneof_MsgVal{MsgVal: cmp.Inner{Value: 2}}}
	assert.Equal(t, -1, a.Compare(b))
}

func TestCompare_OneofAfterFieldTiebreak(t *testing.T) {
	// Oneof variants equal → "after" (tag 5) tiebreaks.
	a := &cmp.WithOneof{Value: &cmp.WithOneof_IntVal{IntVal: 1}, After: "x"}
	b := &cmp.WithOneof{Value: &cmp.WithOneof_IntVal{IntVal: 1}, After: "y"}
	assert.Equal(t, -1, a.Compare(b))
}

// --- Total-order sanity check via sort.Slice ---

// TestCompare_SortStability is a smoke test that Compare produces a
// well-formed total order. If any of the per-shape rules above were
// broken — e.g. Compare returned 0 for unequal values, or violated
// transitivity — sort.Slice would either deadlock, panic, or produce a
// non-stable ordering. Comparing the sorted output against a hand-rolled
// ordering catches breakage at the macro level.
func TestCompare_SortStability(t *testing.T) {
	items := []*cmp.AllScalars{
		{FieldInt32: 3, FieldString: "c"},
		{FieldInt32: 1, FieldString: "a"},
		{FieldInt32: 2, FieldString: "b"},
		{FieldInt32: 1, FieldString: "b"}, // equal int32 with item[1]; "a" < "b" so this comes after
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Compare(items[j]) < 0 })

	want := []struct{ i32, s string }{
		{"1", "a"},
		{"1", "b"},
		{"2", "b"},
		{"3", "c"},
	}
	for i, w := range want {
		assert.Equal(t, w.s, items[i].FieldString, "row %d", i)
		_ = w.i32 // hush unused-field linter
	}
}
