package basic

import (
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ks "github.com/grafana/wiresmith/gen/test/kitchensink/v1"
)

// helper to run the standard marshal/unmarshal/re-marshal cycle.
func roundTrip[T interface {
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
	Size() int
	Equal(interface{}) bool
}](t *testing.T, src T) []byte {
	t.Helper()

	b, err := src.Marshal()
	require.NoError(t, err)
	assert.Equal(t, src.Size(), len(b), "Size() must match marshaled length")

	dst := reflect.New(reflect.TypeOf(src).Elem()).Interface().(T)
	require.NoError(t, dst.Unmarshal(b))
	assert.EqualExportedValues(t, src, dst, "unmarshal must reproduce original")
	assert.True(t, src.Equal(dst), "Equal() must agree with assert.Equal")

	b2, err := dst.Marshal()
	require.NoError(t, err)
	assert.Equal(t, b, b2, "re-marshal must be deterministic")

	return b
}

// --- AllScalars ---

func TestAllScalars_NonZero(t *testing.T) {
	msg := &ks.AllScalars{
		FieldDouble:   3.14,
		FieldFloat:    2.71,
		FieldInt32:    -42,
		FieldInt64:    -100000,
		FieldUint32:   42,
		FieldUint64:   100000,
		FieldSint32:   -12345,
		FieldSint64:   -9876543210,
		FieldFixed32:  0xDEAD,
		FieldFixed64:  0xDEADBEEFCAFE,
		FieldSfixed32: -99999,
		FieldSfixed64: -1234567890123,
		FieldBool:     true,
		FieldString:   "hello wiresmith",
		FieldBytes:    []byte{0xca, 0xfe, 0xba, 0xbe},
	}
	roundTrip(t, msg)
}

func TestAllScalars_Zero(t *testing.T) {
	msg := &ks.AllScalars{}
	b := roundTrip(t, msg)
	assert.Empty(t, b, "all-zero message must marshal to empty bytes")
}

// --- AllOptionalScalars ---

func TestAllOptionalScalars_AllSet(t *testing.T) {
	d := 1.23
	f := float32(4.56)
	i32 := int32(-7)
	i64 := int64(-8)
	u32 := uint32(9)
	u64 := uint64(10)
	si32 := int32(-11)
	si64 := int64(-12)
	fx32 := uint32(13)
	fx64 := uint64(14)
	sf32 := int32(-15)
	sf64 := int64(-16)
	b := true
	s := "optional"

	msg := &ks.AllOptionalScalars{
		FieldDouble:   &d,
		FieldFloat:    &f,
		FieldInt32:    &i32,
		FieldInt64:    &i64,
		FieldUint32:   &u32,
		FieldUint64:   &u64,
		FieldSint32:   &si32,
		FieldSint64:   &si64,
		FieldFixed32:  &fx32,
		FieldFixed64:  &fx64,
		FieldSfixed32: &sf32,
		FieldSfixed64: &sf64,
		FieldBool:     &b,
		FieldString:   &s,
		// Bytes use non-empty value because empty []byte{} round-trips to nil
		// (proto3 does not distinguish empty vs absent for bytes).
		FieldBytes: []byte{0x01, 0x02},
	}
	roundTrip(t, msg)
}

func TestAllOptionalScalars_SetToZero(t *testing.T) {
	// Optional fields set to zero must still be encoded (unlike regular fields).
	d := float64(0)
	f := float32(0)
	i32 := int32(0)
	i64 := int64(0)
	u32 := uint32(0)
	u64 := uint64(0)
	si32 := int32(0)
	si64 := int64(0)
	fx32 := uint32(0)
	fx64 := uint64(0)
	sf32 := int32(0)
	sf64 := int64(0)
	b := false
	s := ""

	msg := &ks.AllOptionalScalars{
		FieldDouble:   &d,
		FieldFloat:    &f,
		FieldInt32:    &i32,
		FieldInt64:    &i64,
		FieldUint32:   &u32,
		FieldUint64:   &u64,
		FieldSint32:   &si32,
		FieldSint64:   &si64,
		FieldFixed32:  &fx32,
		FieldFixed64:  &fx64,
		FieldSfixed32: &sf32,
		FieldSfixed64: &sf64,
		FieldBool:     &b,
		FieldString:   &s,
		FieldBytes:    []byte{},
	}

	bytes, err := msg.Marshal()
	require.NoError(t, err)
	// Zero-valued optional scalars must produce non-empty bytes (they are present).
	assert.NotEmpty(t, bytes, "optional fields set to zero must be encoded")
	assert.Equal(t, msg.Size(), len(bytes))

	var decoded ks.AllOptionalScalars
	require.NoError(t, decoded.Unmarshal(bytes))
	require.NotNil(t, decoded.FieldDouble, "optional double set to 0 must survive")
	require.NotNil(t, decoded.FieldFloat, "optional float set to 0 must survive")
	require.NotNil(t, decoded.FieldInt32, "optional int32 set to 0 must survive")
	require.NotNil(t, decoded.FieldSint32, "optional sint32 set to 0 must survive")
	require.NotNil(t, decoded.FieldSfixed32, "optional sfixed32 set to 0 must survive")
	require.NotNil(t, decoded.FieldBool, "optional bool set to false must survive")
	require.NotNil(t, decoded.FieldString, "optional string set to empty must survive")
}

func TestAllOptionalScalars_AllNil(t *testing.T) {
	msg := &ks.AllOptionalScalars{}
	b := roundTrip(t, msg)
	assert.Empty(t, b, "all-nil optional message must marshal to empty bytes")
}

// --- AllRepeatedScalars ---

func TestAllRepeatedScalars_MultipleElements(t *testing.T) {
	msg := &ks.AllRepeatedScalars{
		FieldDouble:   []float64{1.1, 2.2, 3.3},
		FieldFloat:    []float32{1.1, 2.2},
		FieldInt32:    []int32{-1, 0, 1, math.MaxInt32, math.MinInt32},
		FieldInt64:    []int64{-1, 0, 1, math.MaxInt64, math.MinInt64},
		FieldUint32:   []uint32{0, 1, math.MaxUint32},
		FieldUint64:   []uint64{0, 1, math.MaxUint64},
		FieldSint32:   []int32{math.MinInt32, -1, 0, 1, math.MaxInt32},
		FieldSint64:   []int64{math.MinInt64, -1, 0, 1, math.MaxInt64},
		FieldFixed32:  []uint32{0, 1, 0xFFFFFFFF},
		FieldFixed64:  []uint64{0, 1, 0xFFFFFFFFFFFFFFFF},
		FieldSfixed32: []int32{math.MinInt32, -1, 0, 1, math.MaxInt32},
		FieldSfixed64: []int64{math.MinInt64, -1, 0, 1, math.MaxInt64},
		FieldBool:     []bool{true, false, true},
		FieldString:   []string{"alpha", "beta", "gamma"},
		// Avoid empty []byte{} in the slice: proto3 round-trips it as nil.
		FieldBytes: [][]byte{{0x01}, {0x02, 0x03}, {0x00}},
	}
	roundTrip(t, msg)
}

func TestAllRepeatedScalars_Empty(t *testing.T) {
	msg := &ks.AllRepeatedScalars{}
	b := roundTrip(t, msg)
	assert.Empty(t, b, "empty repeated fields must marshal to empty bytes")
}

// --- OneofVariants ---

func TestOneofVariants_EachType(t *testing.T) {
	tests := []struct {
		name    string
		variant ks.OneofVariants_Value
	}{
		{"double", &ks.OneofVariants_DoubleValue{DoubleValue: 99.99}},
		{"float", &ks.OneofVariants_FloatValue{FloatValue: 42.5}},
		{"int32", &ks.OneofVariants_Int32Value{Int32Value: -123}},
		{"int64", &ks.OneofVariants_Int64Value{Int64Value: -456789}},
		{"uint32", &ks.OneofVariants_Uint32Value{Uint32Value: 123}},
		{"uint64", &ks.OneofVariants_Uint64Value{Uint64Value: 456789}},
		{"sint32", &ks.OneofVariants_Sint32Value{Sint32Value: -321}},
		{"sint64", &ks.OneofVariants_Sint64Value{Sint64Value: -654321}},
		{"fixed32", &ks.OneofVariants_Fixed32Value{Fixed32Value: 0xABCD}},
		{"fixed64", &ks.OneofVariants_Fixed64Value{Fixed64Value: 0xABCDEF01}},
		{"sfixed32", &ks.OneofVariants_Sfixed32Value{Sfixed32Value: -7777}},
		{"sfixed64", &ks.OneofVariants_Sfixed64Value{Sfixed64Value: -88888888}},
		{"bool", &ks.OneofVariants_BoolValue{BoolValue: true}},
		{"string", &ks.OneofVariants_StringValue{StringValue: "oneof_str"}},
		{"bytes", &ks.OneofVariants_BytesValue{BytesValue: []byte{0xDE, 0xAD}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &ks.OneofVariants{Value: tt.variant}
			roundTrip(t, msg)
		})
	}
}

func TestOneofVariants_LastSetWins(t *testing.T) {
	// Marshal a message with sint32, then unmarshal: only sint32 should be present.
	msg := &ks.OneofVariants{Value: &ks.OneofVariants_Sint32Value{Sint32Value: -999}}
	b, err := msg.Marshal()
	require.NoError(t, err)

	var decoded ks.OneofVariants
	require.NoError(t, decoded.Unmarshal(b))
	_, ok := decoded.Value.(*ks.OneofVariants_Sint32Value)
	assert.True(t, ok, "expected sint32 variant")
}

// --- Nested messages ---

func TestNested_FullyPopulated(t *testing.T) {
	msg := &ks.Outer{
		Middle: ks.Middle{
			Inner: ks.Inner{
				Data:      "deep",
				Raw:       []byte{0xFF},
				SignedVal: -9999,
				FixedVal:  -12345,
			},
			Inners: []ks.Inner{
				{Data: "inner1", SignedVal: math.MinInt64, FixedVal: math.MinInt32},
				{Data: "inner2", SignedVal: math.MaxInt64, FixedVal: math.MaxInt32},
			},
			Value: 42,
		},
		Middles: []ks.Middle{
			{Value: 100},
			{
				Inner:  ks.Inner{Data: "nested"},
				Inners: []ks.Inner{{Raw: []byte{0x01, 0x02}}},
				Value:  200,
			},
		},
		Name: "outer-name",
	}
	roundTrip(t, msg)
}

// --- HighFieldNumbers ---

func TestHighFieldNumbers_AllSet(t *testing.T) {
	msg := &ks.HighFieldNumbers{
		Field1:     "field-1",
		Field16:    "field-16",
		Field128:   "field-128",
		Field2048:  "field-2048",
		Field16384: "field-16384",
	}
	roundTrip(t, msg)
}

func TestHighFieldNumbers_LargeValues(t *testing.T) {
	// High field numbers combined with large string values
	bigStr := make([]byte, 1024)
	for i := range bigStr {
		bigStr[i] = byte('A' + i%26)
	}
	msg := &ks.HighFieldNumbers{
		Field1:     string(bigStr),
		Field16384: string(bigStr),
	}
	roundTrip(t, msg)
}

// --- WithEnum ---

func TestWithEnum_SingularAndRepeated(t *testing.T) {
	msg := &ks.WithEnum{
		Color:  ks.Color_COLOR_BLUE,
		Colors: []ks.Color{ks.Color_COLOR_RED, ks.Color_COLOR_GREEN, ks.Color_COLOR_BLUE, ks.Color_COLOR_UNSPECIFIED},
	}
	roundTrip(t, msg)
}

func TestWithEnum_Zero(t *testing.T) {
	msg := &ks.WithEnum{}
	b := roundTrip(t, msg)
	assert.Empty(t, b)
}

// --- Empty ---

func TestEmpty_RoundTrip(t *testing.T) {
	msg := &ks.Empty{}
	b := roundTrip(t, msg)
	assert.Empty(t, b, "empty message must marshal to zero bytes")
}

// --- OnlyRepeated ---

func TestOnlyRepeated_WithElements(t *testing.T) {
	msg := &ks.OnlyRepeated{
		Names:  []string{"alice", "bob", "charlie"},
		Values: []int64{-1, 0, 1, math.MaxInt64},
		Items: []ks.Inner{
			{Data: "item1", SignedVal: -100, FixedVal: 200},
			{Data: "item2", Raw: []byte{0xAB}},
		},
	}
	roundTrip(t, msg)
}

func TestOnlyRepeated_Empty(t *testing.T) {
	msg := &ks.OnlyRepeated{}
	b := roundTrip(t, msg)
	assert.Empty(t, b)
}

// --- Container ---

func TestContainer_Combined(t *testing.T) {
	msg := &ks.Container{
		Variant: ks.OneofVariants{
			Value: &ks.OneofVariants_Sfixed32Value{Sfixed32Value: -55555},
		},
		Scalars: ks.AllScalars{
			FieldDouble:   1.0,
			FieldFloat:    2.0,
			FieldSint32:   -10,
			FieldSfixed32: -20,
			FieldString:   "container",
		},
		Variants: []ks.OneofVariants{
			{Value: &ks.OneofVariants_BoolValue{BoolValue: true}},
			{Value: &ks.OneofVariants_Sint64Value{Sint64Value: -777}},
			{Value: &ks.OneofVariants_BytesValue{BytesValue: []byte{0x01}}},
		},
	}
	roundTrip(t, msg)
}

// --- Boundary value tests ---

func TestSint32_BoundaryValues(t *testing.T) {
	tests := []struct {
		name string
		val  int32
	}{
		{"MinInt32", math.MinInt32},
		{"MaxInt32", math.MaxInt32},
		{"Neg1", -1},
		{"Zero", 0},
		{"Pos1", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &ks.AllScalars{FieldSint32: tt.val}
			roundTrip(t, msg)
		})
	}
}

func TestSint64_BoundaryValues(t *testing.T) {
	tests := []struct {
		name string
		val  int64
	}{
		{"MinInt64", math.MinInt64},
		{"MaxInt64", math.MaxInt64},
		{"Neg1", -1},
		{"Zero", 0},
		{"Pos1", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &ks.AllScalars{FieldSint64: tt.val}
			roundTrip(t, msg)
		})
	}
}

func TestSfixed32_BoundaryValues(t *testing.T) {
	tests := []struct {
		name string
		val  int32
	}{
		{"MinInt32", math.MinInt32},
		{"MaxInt32", math.MaxInt32},
		{"Neg1", -1},
		{"Zero", 0},
		{"Pos1", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &ks.AllScalars{FieldSfixed32: tt.val}
			roundTrip(t, msg)
		})
	}
}

func TestFloat_BoundaryValues(t *testing.T) {
	tests := []struct {
		name string
		val  float32
	}{
		{"MaxFloat32", math.MaxFloat32},
		{"SmallestNonzero", math.SmallestNonzeroFloat32},
		{"PosInf", float32(math.Inf(1))},
		{"NegInf", float32(math.Inf(-1))},
		{"NegZero", float32(math.Copysign(0, -1))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &ks.AllScalars{FieldFloat: tt.val}
			roundTrip(t, msg)
		})
	}
}

func TestFloat_NaN(t *testing.T) {
	// NaN != NaN, so we test marshal/unmarshal manually using math.IsNaN.
	msg := &ks.AllScalars{FieldFloat: float32(math.NaN())}
	b, err := msg.Marshal()
	require.NoError(t, err)
	assert.Equal(t, msg.Size(), len(b))

	var decoded ks.AllScalars
	require.NoError(t, decoded.Unmarshal(b))
	assert.True(t, math.IsNaN(float64(decoded.FieldFloat)), "expected NaN")
}

func TestSfixed64_BoundaryValues(t *testing.T) {
	tests := []struct {
		name string
		val  int64
	}{
		{"MinInt64", math.MinInt64},
		{"MaxInt64", math.MaxInt64},
		{"Neg1", -1},
		{"Zero", 0},
		{"Pos1", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &ks.AllScalars{FieldSfixed64: tt.val}
			roundTrip(t, msg)
		})
	}
}

// --- Packed repeated boundary values ---

func TestRepeatedSint32_Packed(t *testing.T) {
	msg := &ks.AllRepeatedScalars{
		FieldSint32: []int32{math.MinInt32, -1, 0, 1, math.MaxInt32},
	}
	roundTrip(t, msg)
}

func TestRepeatedSint64_Packed(t *testing.T) {
	msg := &ks.AllRepeatedScalars{
		FieldSint64: []int64{math.MinInt64, -1, 0, 1, math.MaxInt64},
	}
	roundTrip(t, msg)
}

func TestRepeatedSfixed32_Packed(t *testing.T) {
	msg := &ks.AllRepeatedScalars{
		FieldSfixed32: []int32{math.MinInt32, -1, 0, 1, math.MaxInt32},
	}
	roundTrip(t, msg)
}

func TestRepeatedSfixed64_Packed(t *testing.T) {
	msg := &ks.AllRepeatedScalars{
		FieldSfixed64: []int64{math.MinInt64, -1, 0, 1, math.MaxInt64},
	}
	roundTrip(t, msg)
}

func TestRepeatedFloat_Packed(t *testing.T) {
	msg := &ks.AllRepeatedScalars{
		FieldFloat: []float32{
			math.MaxFloat32,
			math.SmallestNonzeroFloat32,
			float32(math.Inf(1)),
			float32(math.Inf(-1)),
			0,
			-1.5,
		},
	}
	roundTrip(t, msg)
}

// --- Optional sint/sfixed boundary ---

func TestOptionalSint32_BoundaryValues(t *testing.T) {
	vals := []int32{math.MinInt32, -1, 0, 1, math.MaxInt32}
	for _, v := range vals {
		v := v
		msg := &ks.AllOptionalScalars{FieldSint32: &v}
		roundTrip(t, msg)
	}
}

func TestOptionalSfixed32_BoundaryValues(t *testing.T) {
	vals := []int32{math.MinInt32, -1, 0, 1, math.MaxInt32}
	for _, v := range vals {
		v := v
		msg := &ks.AllOptionalScalars{FieldSfixed32: &v}
		roundTrip(t, msg)
	}
}

func TestOptionalFloat_BoundaryValues(t *testing.T) {
	vals := []float32{math.MaxFloat32, math.SmallestNonzeroFloat32, float32(math.Inf(1))}
	for _, v := range vals {
		v := v
		msg := &ks.AllOptionalScalars{FieldFloat: &v}
		roundTrip(t, msg)
	}
}

// --- Oneof sint/sfixed boundary ---

func TestOneofSint32_Boundary(t *testing.T) {
	vals := []int32{math.MinInt32, -1, 0, 1, math.MaxInt32}
	for _, v := range vals {
		msg := &ks.OneofVariants{Value: &ks.OneofVariants_Sint32Value{Sint32Value: v}}
		roundTrip(t, msg)
	}
}

func TestOneofSfixed32_Boundary(t *testing.T) {
	vals := []int32{math.MinInt32, -1, 0, 1, math.MaxInt32}
	for _, v := range vals {
		msg := &ks.OneofVariants{Value: &ks.OneofVariants_Sfixed32Value{Sfixed32Value: v}}
		roundTrip(t, msg)
	}
}
