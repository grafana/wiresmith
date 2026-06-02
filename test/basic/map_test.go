package basic

import (
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"

	ks "wiresmith/gen/test/kitchensink/v1"
)

// mapRoundTrip verifies marshal/unmarshal consistency for messages containing
// maps. Unlike roundTrip, it does not compare byte-level output between two
// marshals because Go map iteration order is non-deterministic.
func mapRoundTrip[T interface {
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
	Size() int
	Equal(interface{}) bool
}](t *testing.T, src T) {
	t.Helper()

	b, err := src.Marshal()
	require.NoError(t, err)
	assert.Equal(t, src.Size(), len(b), "Size() must match marshaled length")

	dst := newZero(src)
	require.NoError(t, dst.Unmarshal(b))
	assert.EqualExportedValues(t, src, dst, "unmarshal must reproduce original")
	assert.True(t, src.Equal(dst), "Equal() must agree with assert.Equal")
}

// newZero returns a new zero-value instance of the same concrete type.
func newZero[T any](v T) T {
	return reflect.New(reflect.TypeOf(v).Elem()).Interface().(T)
}

func TestAllMaps_FullyPopulated(t *testing.T) {
	msg := &ks.AllMaps{
		MapInt32Int32:       map[int32]int32{-1: 1, 0: 0, math.MaxInt32: math.MinInt32},
		MapInt64Int64:       map[int64]int64{math.MinInt64: math.MaxInt64, 0: 0},
		MapUint32Uint32:     map[uint32]uint32{0: 0, 1: 1, math.MaxUint32: 42},
		MapUint64Uint64:     map[uint64]uint64{0: 0, math.MaxUint64: 1},
		MapSint32Sint32:     map[int32]int32{math.MinInt32: math.MaxInt32, -1: 1},
		MapSint64Sint64:     map[int64]int64{math.MinInt64: math.MaxInt64, -1: 1},
		MapFixed32Fixed32:   map[uint32]uint32{0: 0, 0xDEADBEEF: 0xCAFEBABE},
		MapFixed64Fixed64:   map[uint64]uint64{0: 0, 0xDEADBEEFCAFEBABE: 1},
		MapSfixed32Sfixed32: map[int32]int32{math.MinInt32: math.MaxInt32, -1: 1},
		MapSfixed64Sfixed64: map[int64]int64{math.MinInt64: math.MaxInt64, -1: 1},
		MapBoolBool:         map[bool]bool{true: false, false: true},
		MapStringString:     map[string]string{"hello": "world", "": "empty_key", "empty_val": ""},
		MapStringBytes:      map[string][]byte{"data": {0xDE, 0xAD}},
		MapInt32Float:       map[int32]float32{1: 3.14, -1: float32(math.Inf(-1))},
		MapInt32Double:      map[int32]float64{1: math.MaxFloat64, -1: math.SmallestNonzeroFloat64},
		MapStringMessage:    map[string]ks.Inner{"a": {Data: "inner-a", SignedVal: -100}, "b": {Raw: []byte{0xFF}}},
		MapStringEnum:       map[string]ks.Color{"red": ks.Color_COLOR_RED, "blue": ks.Color_COLOR_BLUE, "zero": ks.Color_COLOR_UNSPECIFIED},
	}
	mapRoundTrip(t, msg)
}

func TestAllMaps_Empty(t *testing.T) {
	msg := &ks.AllMaps{}
	b, err := msg.Marshal()
	require.NoError(t, err)
	assert.Empty(t, b, "nil maps must marshal to zero bytes")
	assert.Equal(t, 0, msg.Size())
}

func TestAllMaps_SingleEntry(t *testing.T) {
	msg := &ks.AllMaps{
		MapStringString: map[string]string{"only": "one"},
	}
	mapRoundTrip(t, msg)
}

func TestMap_MessageValue(t *testing.T) {
	msg := &ks.AllMaps{
		MapStringMessage: map[string]ks.Inner{
			"deep": {
				Data:      "nested-data",
				Raw:       []byte{0x01, 0x02, 0x03},
				SignedVal: math.MinInt64,
				FixedVal:  math.MaxInt32,
			},
		},
	}
	mapRoundTrip(t, msg)
}

func TestMap_EmptyMessageValue(t *testing.T) {
	msg := &ks.AllMaps{
		MapStringMessage: map[string]ks.Inner{
			"empty": {},
		},
	}
	mapRoundTrip(t, msg)
}

func TestMap_LargeMap(t *testing.T) {
	m := make(map[int32]int32, 1000)
	for i := int32(0); i < 1000; i++ {
		m[i] = i * i
	}
	msg := &ks.AllMaps{MapInt32Int32: m}
	mapRoundTrip(t, msg)
}

func TestMap_EachKeyType(t *testing.T) {
	// Ensure each key type works in isolation.
	t.Run("int32", func(t *testing.T) {
		mapRoundTrip(t, &ks.AllMaps{MapInt32Int32: map[int32]int32{42: 99}})
	})
	t.Run("int64", func(t *testing.T) {
		mapRoundTrip(t, &ks.AllMaps{MapInt64Int64: map[int64]int64{42: 99}})
	})
	t.Run("uint32", func(t *testing.T) {
		mapRoundTrip(t, &ks.AllMaps{MapUint32Uint32: map[uint32]uint32{42: 99}})
	})
	t.Run("uint64", func(t *testing.T) {
		mapRoundTrip(t, &ks.AllMaps{MapUint64Uint64: map[uint64]uint64{42: 99}})
	})
	t.Run("sint32", func(t *testing.T) {
		mapRoundTrip(t, &ks.AllMaps{MapSint32Sint32: map[int32]int32{-42: 99}})
	})
	t.Run("sint64", func(t *testing.T) {
		mapRoundTrip(t, &ks.AllMaps{MapSint64Sint64: map[int64]int64{-42: 99}})
	})
	t.Run("fixed32", func(t *testing.T) {
		mapRoundTrip(t, &ks.AllMaps{MapFixed32Fixed32: map[uint32]uint32{42: 99}})
	})
	t.Run("fixed64", func(t *testing.T) {
		mapRoundTrip(t, &ks.AllMaps{MapFixed64Fixed64: map[uint64]uint64{42: 99}})
	})
	t.Run("sfixed32", func(t *testing.T) {
		mapRoundTrip(t, &ks.AllMaps{MapSfixed32Sfixed32: map[int32]int32{-42: 99}})
	})
	t.Run("sfixed64", func(t *testing.T) {
		mapRoundTrip(t, &ks.AllMaps{MapSfixed64Sfixed64: map[int64]int64{-42: 99}})
	})
	t.Run("bool", func(t *testing.T) {
		mapRoundTrip(t, &ks.AllMaps{MapBoolBool: map[bool]bool{true: false}})
	})
	t.Run("string", func(t *testing.T) {
		mapRoundTrip(t, &ks.AllMaps{MapStringString: map[string]string{"k": "v"}})
	})
}

func TestMap_RawWire_DuplicateKey(t *testing.T) {
	// Two map entries with the same key — last one wins per proto spec.
	entry1 := buildMapEntryVarintVarint(42, 100)
	entry2 := buildMapEntryVarintVarint(42, 200)

	// AllMaps field 1 = map<int32, int32>
	var wire []byte
	wire = protowire.AppendTag(wire, 1, protowire.BytesType)
	wire = protowire.AppendBytes(wire, entry1)
	wire = protowire.AppendTag(wire, 1, protowire.BytesType)
	wire = protowire.AppendBytes(wire, entry2)

	var msg ks.AllMaps
	require.NoError(t, msg.Unmarshal(wire))
	assert.Equal(t, int32(200), msg.MapInt32Int32[42], "last entry should win for duplicate keys")
}

func TestMap_RawWire_MissingKeyAndValue(t *testing.T) {
	// Empty entry — both key and value should default to zero.
	var wire []byte
	wire = protowire.AppendTag(wire, 1, protowire.BytesType)
	wire = protowire.AppendBytes(wire, []byte{})

	var msg ks.AllMaps
	require.NoError(t, msg.Unmarshal(wire))
	require.Contains(t, msg.MapInt32Int32, int32(0))
	assert.Equal(t, int32(0), msg.MapInt32Int32[0])
}

func TestMap_RawWire_MissingValue(t *testing.T) {
	// Entry with only key — value defaults to zero.
	var entry []byte
	entry = protowire.AppendTag(entry, 1, protowire.VarintType)
	entry = protowire.AppendVarint(entry, 7)

	var wire []byte
	wire = protowire.AppendTag(wire, 1, protowire.BytesType)
	wire = protowire.AppendBytes(wire, entry)

	var msg ks.AllMaps
	require.NoError(t, msg.Unmarshal(wire))
	assert.Equal(t, int32(0), msg.MapInt32Int32[7])
}

func TestMap_RawWire_MissingKey(t *testing.T) {
	// Entry with only value — key defaults to zero.
	var entry []byte
	entry = protowire.AppendTag(entry, 2, protowire.VarintType)
	entry = protowire.AppendVarint(entry, 99)

	var wire []byte
	wire = protowire.AppendTag(wire, 1, protowire.BytesType)
	wire = protowire.AppendBytes(wire, entry)

	var msg ks.AllMaps
	require.NoError(t, msg.Unmarshal(wire))
	assert.Equal(t, int32(99), msg.MapInt32Int32[0])
}

func TestMap_RawWire_UnknownFieldInEntry(t *testing.T) {
	var entry []byte
	entry = protowire.AppendTag(entry, 1, protowire.VarintType)
	entry = protowire.AppendVarint(entry, 5)
	entry = protowire.AppendTag(entry, 2, protowire.VarintType)
	entry = protowire.AppendVarint(entry, 10)
	entry = protowire.AppendTag(entry, 3, protowire.VarintType) // unknown field
	entry = protowire.AppendVarint(entry, 999)

	var wire []byte
	wire = protowire.AppendTag(wire, 1, protowire.BytesType)
	wire = protowire.AppendBytes(wire, entry)

	var msg ks.AllMaps
	require.NoError(t, msg.Unmarshal(wire))
	assert.Equal(t, int32(10), msg.MapInt32Int32[5])
}

// buildMapEntryVarintVarint builds a map entry with varint key and value.
func buildMapEntryVarintVarint(key, val uint64) []byte {
	var entry []byte
	entry = protowire.AppendTag(entry, 1, protowire.VarintType)
	entry = protowire.AppendVarint(entry, key)
	entry = protowire.AppendTag(entry, 2, protowire.VarintType)
	entry = protowire.AppendVarint(entry, val)
	return entry
}

// TestMap_RawWire_DuplicateKey_MessageValue covers the message-valued
// counterpart of TestMap_RawWire_DuplicateKey. proto3's "last-write-wins"
// semantics for duplicate keys mean each entry REPLACES the previous one
// — fields that appeared in the first wire entry but not the second must
// NOT survive in the final map. wiresmith-05d backs out a prior merge
// branch that would have surfaced fields from BOTH entries here.
//
// First entry: Inner{Data: "first", SignedVal: 111}
// Second entry (same key): Inner{Raw: []byte{0xFF}, FixedVal: 222}
// Expected after REPLACE: Inner{Raw: []byte{0xFF}, FixedVal: 222}
//
//	(no Data, no SignedVal — they belonged to the discarded first entry)
//
// MERGE behaviour, by contrast, would have ended with all four fields
// populated. Conformance test
// `Required.Proto3.ProtobufInput.ValidDataMap.STRING.MESSAGE.MergeValue`
// exercises the same invariant against the upstream test corpus once the
// failure-list entry it lives behind is pruned.
func TestMap_RawWire_DuplicateKey_MessageValue(t *testing.T) {
	first := buildInnerWire("first", 111, nil, 0)
	second := buildInnerWire("", 0, []byte{0xFF}, 222)

	entry1 := buildMapEntryStringMessage("k", first)
	entry2 := buildMapEntryStringMessage("k", second)

	// AllMaps field 16 = map<string, Inner>
	var wire []byte
	wire = protowire.AppendTag(wire, 16, protowire.BytesType)
	wire = protowire.AppendBytes(wire, entry1)
	wire = protowire.AppendTag(wire, 16, protowire.BytesType)
	wire = protowire.AppendBytes(wire, entry2)

	var msg ks.AllMaps
	require.NoError(t, msg.Unmarshal(wire))

	got, ok := msg.MapStringMessage["k"]
	require.True(t, ok, "expected key 'k' in MapStringMessage")
	assert.Equal(t, "", got.Data, "Data must come ONLY from the second entry (REPLACE)")
	assert.Equal(t, int64(0), got.SignedVal, "SignedVal must come ONLY from the second entry (REPLACE)")
	assert.Equal(t, []byte{0xFF}, got.Raw, "Raw must come from the second entry")
	assert.Equal(t, int32(222), got.FixedVal, "FixedVal must come from the second entry")
}

// buildInnerWire builds a wire-format ks.Inner. Fields default to their
// zero value; pass an empty / zero arg to skip that field on the wire.
//
//	Data      string = field 1 (bytes)
//	Raw       []byte = field 2 (bytes)
//	SignedVal int64  = field 3 (zigzag64)
//	FixedVal  int32  = field 4 (fixed32)
func buildInnerWire(data string, signedVal int64, raw []byte, fixedVal int32) []byte {
	var b []byte
	if data != "" {
		b = protowire.AppendTag(b, 1, protowire.BytesType)
		b = protowire.AppendString(b, data)
	}
	if raw != nil {
		b = protowire.AppendTag(b, 2, protowire.BytesType)
		b = protowire.AppendBytes(b, raw)
	}
	if signedVal != 0 {
		b = protowire.AppendTag(b, 3, protowire.VarintType)
		b = protowire.AppendVarint(b, protowire.EncodeZigZag(signedVal))
	}
	if fixedVal != 0 {
		b = protowire.AppendTag(b, 4, protowire.Fixed32Type)
		b = protowire.AppendFixed32(b, uint32(fixedVal))
	}
	return b
}

// buildMapEntryStringMessage builds a map entry with string key (field 1)
// and an embedded message value (field 2). The message bytes are
// length-prefixed.
func buildMapEntryStringMessage(key string, msgBytes []byte) []byte {
	var entry []byte
	entry = protowire.AppendTag(entry, 1, protowire.BytesType)
	entry = protowire.AppendString(entry, key)
	entry = protowire.AppendTag(entry, 2, protowire.BytesType)
	entry = protowire.AppendBytes(entry, msgBytes)
	return entry
}
