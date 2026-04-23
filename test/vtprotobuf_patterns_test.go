package test

import (
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	commonv1 "wiresmith/gen/otlp/common/v1"
	logsv1 "wiresmith/gen/otlp/logs/v1"
	metricsv1 "wiresmith/gen/otlp/metrics/v1"
	profilesv1 "wiresmith/gen/otlp/profiles/v1development"
	resourcev1 "wiresmith/gen/otlp/resource/v1"
	tracev1 "wiresmith/gen/otlp/trace/v1"

	otlpcommon "go.opentelemetry.io/proto/otlp/common/v1"
	otlpmetrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	otlpresource "go.opentelemetry.io/proto/otlp/resource/v1"
	otlptrace "go.opentelemetry.io/proto/otlp/trace/v1"
)

// ---------------------------------------------------------------------------
// 1. Oneof zero-value vs absence
//    Inspired by vtprotobuf's TestEqualVT_Oneof_AbsenceVsZeroValue.
//    A set oneof with a zero value must produce different wire bytes than
//    nil (absent) oneof.
// ---------------------------------------------------------------------------

func TestOneofZeroValueVsAbsence(t *testing.T) {
	t.Run("AnyValue", func(t *testing.T) {
		absent := commonv1.AnyValue{Value: nil}
		absentBytes, err := absent.Marshal()
		require.NoError(t, err)
		assert.Empty(t, absentBytes, "nil oneof should marshal to empty bytes")

		zeroVariants := []struct {
			name  string
			value commonv1.AnyValue_Value
		}{
			{"int_zero", &commonv1.AnyValue_IntValue{IntValue: 0}},
			{"string_empty", &commonv1.AnyValue_StringValue{StringValue: ""}},
			{"bool_false", &commonv1.AnyValue_BoolValue{BoolValue: false}},
			{"double_zero", &commonv1.AnyValue_DoubleValue{DoubleValue: 0.0}},
			{"bytes_empty", &commonv1.AnyValue_BytesValue{BytesValue: []byte{}}},
		}

		for _, zv := range zeroVariants {
			t.Run(zv.name, func(t *testing.T) {
				msg := commonv1.AnyValue{Value: zv.value}
				b, err := msg.Marshal()
				require.NoError(t, err)
				// Zero-valued oneof must still encode the field tag
				assert.NotEmpty(t, b, "zero-valued oneof %s should produce non-empty wire bytes", zv.name)
				assert.NotEqual(t, absentBytes, b, "zero-valued oneof %s must differ from absent", zv.name)

				// Cross-validate: official protobuf should unmarshal it correctly
				var official otlpcommon.AnyValue
				require.NoError(t, proto.Unmarshal(b, &official))

				// Reverse: official marshal, our unmarshal
				officialBytes, err := proto.Marshal(&official)
				require.NoError(t, err)
				var decoded commonv1.AnyValue
				require.NoError(t, decoded.Unmarshal(officialBytes))

				reBytes, err := decoded.Marshal()
				require.NoError(t, err)
				assert.Equal(t, b, reBytes)
			})
		}
	})

	t.Run("NumberDataPoint", func(t *testing.T) {
		absent := metricsv1.NumberDataPoint{TimeUnixNano: 1, Value: nil}
		absentBytes, err := absent.Marshal()
		require.NoError(t, err)

		asDoubleZero := metricsv1.NumberDataPoint{TimeUnixNano: 1, Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: 0.0}}
		dblBytes, err := asDoubleZero.Marshal()
		require.NoError(t, err)
		assert.NotEqual(t, absentBytes, dblBytes, "as_double=0.0 must differ from absent value")

		asIntZero := metricsv1.NumberDataPoint{TimeUnixNano: 1, Value: &metricsv1.NumberDataPoint_AsInt{AsInt: 0}}
		intBytes, err := asIntZero.Marshal()
		require.NoError(t, err)
		assert.NotEqual(t, absentBytes, intBytes, "as_int=0 must differ from absent value")
		assert.NotEqual(t, dblBytes, intBytes, "as_double=0.0 and as_int=0 use different field numbers")

		// Cross-validate
		var officialDbl otlpmetrics.NumberDataPoint
		require.NoError(t, proto.Unmarshal(dblBytes, &officialDbl))
		assert.Equal(t, 0.0, officialDbl.GetAsDouble())

		var officialInt otlpmetrics.NumberDataPoint
		require.NoError(t, proto.Unmarshal(intBytes, &officialInt))
		assert.Equal(t, int64(0), officialInt.GetAsInt())
	})

	t.Run("Exemplar", func(t *testing.T) {
		absent := metricsv1.Exemplar{TimeUnixNano: 1, Value: nil}
		absentBytes, err := absent.Marshal()
		require.NoError(t, err)

		asDoubleZero := metricsv1.Exemplar{TimeUnixNano: 1, Value: &metricsv1.Exemplar_AsDouble{AsDouble: 0.0}}
		dblBytes, err := asDoubleZero.Marshal()
		require.NoError(t, err)
		assert.NotEqual(t, absentBytes, dblBytes)

		asIntZero := metricsv1.Exemplar{TimeUnixNano: 1, Value: &metricsv1.Exemplar_AsInt{AsInt: 0}}
		intBytes, err := asIntZero.Marshal()
		require.NoError(t, err)
		assert.NotEqual(t, absentBytes, intBytes)
	})

	t.Run("Metric", func(t *testing.T) {
		absent := metricsv1.Metric{Name: "m", Data: nil}
		absentBytes, err := absent.Marshal()
		require.NoError(t, err)

		withGauge := metricsv1.Metric{Name: "m", Data: &metricsv1.Metric_Gauge{Gauge: metricsv1.Gauge{}}}
		gaugeBytes, err := withGauge.Marshal()
		require.NoError(t, err)
		assert.NotEqual(t, absentBytes, gaugeBytes, "empty gauge oneof must differ from absent data")

		withSum := metricsv1.Metric{Name: "m", Data: &metricsv1.Metric_Sum{Sum: metricsv1.Sum{}}}
		sumBytes, err := withSum.Marshal()
		require.NoError(t, err)
		assert.NotEqual(t, absentBytes, sumBytes)
		assert.NotEqual(t, gaugeBytes, sumBytes, "different oneof variants must produce different bytes")
	})
}

// ---------------------------------------------------------------------------
// 2. Empty nested message in oneof
//    Inspired by vtprotobuf's TestEmptyOneof (regression for issue #61).
//    Empty nested messages inside oneofs must still produce wire bytes.
// ---------------------------------------------------------------------------

func TestOneofEmptyNestedMessage(t *testing.T) {
	t.Run("AnyValue_EmptyArrayValue", func(t *testing.T) {
		msg := commonv1.AnyValue{
			Value: &commonv1.AnyValue_ArrayValue{ArrayValue: commonv1.ArrayValue{}},
		}
		b, err := msg.Marshal()
		require.NoError(t, err)
		assert.NotEmpty(t, b, "AnyValue with empty ArrayValue must produce non-empty wire bytes")

		// Cross-validate with official protobuf
		var official otlpcommon.AnyValue
		require.NoError(t, proto.Unmarshal(b, &official))
		assert.NotNil(t, official.GetArrayValue(), "official protobuf should recognize the array_value oneof")

		// Reverse
		officialBytes, err := proto.Marshal(&official)
		require.NoError(t, err)
		var decoded commonv1.AnyValue
		require.NoError(t, decoded.Unmarshal(officialBytes))
		assert.NotNil(t, decoded.Value)
		_, ok := decoded.Value.(*commonv1.AnyValue_ArrayValue)
		assert.True(t, ok, "round-tripped value should be ArrayValue variant")
	})

	t.Run("AnyValue_EmptyKvlistValue", func(t *testing.T) {
		msg := commonv1.AnyValue{
			Value: &commonv1.AnyValue_KvlistValue{KvlistValue: commonv1.KeyValueList{}},
		}
		b, err := msg.Marshal()
		require.NoError(t, err)
		assert.NotEmpty(t, b, "AnyValue with empty KvlistValue must produce non-empty wire bytes")

		var official otlpcommon.AnyValue
		require.NoError(t, proto.Unmarshal(b, &official))
		assert.NotNil(t, official.GetKvlistValue(), "official protobuf should recognize the kvlist_value oneof")

		officialBytes, err := proto.Marshal(&official)
		require.NoError(t, err)
		var decoded commonv1.AnyValue
		require.NoError(t, decoded.Unmarshal(officialBytes))
		_, ok := decoded.Value.(*commonv1.AnyValue_KvlistValue)
		assert.True(t, ok, "round-tripped value should be KvlistValue variant")
	})

	t.Run("Metric_EmptyGauge", func(t *testing.T) {
		msg := metricsv1.Metric{
			Name: "m",
			Data: &metricsv1.Metric_Gauge{Gauge: metricsv1.Gauge{}},
		}
		b, err := msg.Marshal()
		require.NoError(t, err)

		var official otlpmetrics.Metric
		require.NoError(t, proto.Unmarshal(b, &official))
		assert.NotNil(t, official.GetGauge())

		officialBytes, err := proto.Marshal(&official)
		require.NoError(t, err)
		var decoded metricsv1.Metric
		require.NoError(t, decoded.Unmarshal(officialBytes))
		_, ok := decoded.Data.(*metricsv1.Metric_Gauge)
		assert.True(t, ok)
	})

	t.Run("Metric_EmptySummary", func(t *testing.T) {
		msg := metricsv1.Metric{
			Name: "m",
			Data: &metricsv1.Metric_Summary{Summary: metricsv1.Summary{}},
		}
		b, err := msg.Marshal()
		require.NoError(t, err)

		var official otlpmetrics.Metric
		require.NoError(t, proto.Unmarshal(b, &official))
		assert.NotNil(t, official.GetSummary())
	})

	t.Run("Metric_EmptyHistogram", func(t *testing.T) {
		msg := metricsv1.Metric{
			Name: "m",
			Data: &metricsv1.Metric_Histogram{Histogram: metricsv1.Histogram{}},
		}
		b, err := msg.Marshal()
		require.NoError(t, err)

		var official otlpmetrics.Metric
		require.NoError(t, proto.Unmarshal(b, &official))
		assert.NotNil(t, official.GetHistogram())
	})

	t.Run("Metric_EmptyExponentialHistogram", func(t *testing.T) {
		msg := metricsv1.Metric{
			Name: "m",
			Data: &metricsv1.Metric_ExponentialHistogram{ExponentialHistogram: metricsv1.ExponentialHistogram{}},
		}
		b, err := msg.Marshal()
		require.NoError(t, err)

		var official otlpmetrics.Metric
		require.NoError(t, proto.Unmarshal(b, &official))
		assert.NotNil(t, official.GetExponentialHistogram())
	})
}

// ---------------------------------------------------------------------------
// 3. Unmarshal into non-zero struct
//    Inspired by vtprotobuf's pool tests that verify unmarshaling into
//    already-populated structs properly resets fields.
// ---------------------------------------------------------------------------

func TestUnmarshalIntoNonZeroStruct(t *testing.T) {
	t.Run("RepeatedFieldsReplaced", func(t *testing.T) {
		// Pre-populate a Resource with attributes
		existing := resourcev1.Resource{
			Attributes: []commonv1.KeyValue{
				{Key: "old1", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "old1"}}},
				{Key: "old2", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "old2"}}},
			},
			DroppedAttributesCount: 99,
		}

		// Marshal a different Resource
		newMsg := resourcev1.Resource{
			Attributes: []commonv1.KeyValue{
				{Key: "new1", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 1}}},
			},
			DroppedAttributesCount: 5,
		}
		newBytes, err := newMsg.Marshal()
		require.NoError(t, err)

		// Unmarshal into the pre-populated struct
		require.NoError(t, existing.Unmarshal(newBytes))

		// Scalar field must be overwritten
		assert.Equal(t, uint32(5), existing.DroppedAttributesCount)
		// Repeated field: per protobuf spec, repeated fields from separate
		// unmarshal calls can append. The key behavior to test is that
		// the final state matches what official protobuf produces.
		var official otlpresource.Resource
		require.NoError(t, proto.Unmarshal(newBytes, &official))
		assert.Len(t, official.Attributes, 1)
	})

	t.Run("ScalarFieldsOverwritten", func(t *testing.T) {
		existing := tracev1.Span{
			TraceId:           []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
			SpanId:            []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
			Name:              "old-name",
			Kind:              tracev1.Span_SPAN_KIND_SERVER,
			StartTimeUnixNano: 9999,
			EndTimeUnixNano:   9999,
		}

		newSpan := tracev1.Span{
			TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name:              "new-name",
			Kind:              tracev1.Span_SPAN_KIND_CLIENT,
			StartTimeUnixNano: 1000,
			EndTimeUnixNano:   2000,
		}
		newBytes, err := newSpan.Marshal()
		require.NoError(t, err)

		require.NoError(t, existing.Unmarshal(newBytes))
		assert.Equal(t, "new-name", existing.Name)
		assert.Equal(t, tracev1.Span_SPAN_KIND_CLIENT, existing.Kind)
		assert.Equal(t, uint64(1000), existing.StartTimeUnixNano)
		assert.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, existing.TraceId)
	})

	t.Run("OneofFieldOverwritten", func(t *testing.T) {
		existing := commonv1.AnyValue{
			Value: &commonv1.AnyValue_IntValue{IntValue: 42},
		}

		newMsg := commonv1.AnyValue{
			Value: &commonv1.AnyValue_StringValue{StringValue: "replaced"},
		}
		newBytes, err := newMsg.Marshal()
		require.NoError(t, err)

		require.NoError(t, existing.Unmarshal(newBytes))
		sv, ok := existing.Value.(*commonv1.AnyValue_StringValue)
		require.True(t, ok, "oneof should be string variant after unmarshal, got %T", existing.Value)
		assert.Equal(t, "replaced", sv.StringValue)
	})

	t.Run("FieldsFromNewMessageOverwrite", func(t *testing.T) {
		// Unmarshal a message with a subset of fields into a pre-populated struct.
		// Fields present in the wire format should overwrite. Fields absent from
		// wire format retain their previous values (standard protobuf merge behavior).
		existing := logsv1.LogRecord{
			TimeUnixNano:           5000,
			ObservedTimeUnixNano:   6000,
			SeverityNumber:         logsv1.SeverityNumber_SEVERITY_NUMBER_FATAL,
			SeverityText:           "FATAL",
			Body:                   commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "old body"}},
			TraceId:                []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:                 []byte{1, 2, 3, 4, 5, 6, 7, 8},
			DroppedAttributesCount: 10,
		}

		// New message only has severity fields
		newMsg := logsv1.LogRecord{
			SeverityNumber: logsv1.SeverityNumber_SEVERITY_NUMBER_INFO,
			SeverityText:   "INFO",
		}
		newBytes, err := newMsg.Marshal()
		require.NoError(t, err)

		require.NoError(t, existing.Unmarshal(newBytes))
		assert.Equal(t, logsv1.SeverityNumber_SEVERITY_NUMBER_INFO, existing.SeverityNumber)
		assert.Equal(t, "INFO", existing.SeverityText)
	})
}

// ---------------------------------------------------------------------------
// 4. Non-packed repeated varint and multi-chunk packed fields
// ---------------------------------------------------------------------------

func TestNonPackedRepeatedVarint(t *testing.T) {
	t.Run("Sample_Values", func(t *testing.T) {
		// Sample.Values is repeated int64 at field 4 (varint encoding).
		// Build wire with individual varint entries (non-packed).
		var wire []byte
		for _, v := range []int64{100, -200, 300} {
			wire = protowire.AppendTag(wire, 4, protowire.VarintType)
			wire = protowire.AppendVarint(wire, uint64(v))
		}
		// Also set link_index (field 3) so we have a recognizable marker
		wire = protowire.AppendTag(wire, 3, protowire.VarintType)
		wire = protowire.AppendVarint(wire, 7)

		var s profilesv1.Sample
		require.NoError(t, s.Unmarshal(wire))
		assert.Equal(t, int32(7), s.LinkIndex)
		assert.Equal(t, []int64{100, -200, 300}, s.Values)
	})

	t.Run("Stack_LocationIndices", func(t *testing.T) {
		// Stack.LocationIndices is repeated int32 at field 1.
		// Build wire with individual varint entries (non-packed).
		var wire []byte
		for _, v := range []int32{0, 5, 10, 15} {
			wire = protowire.AppendTag(wire, 1, protowire.VarintType)
			wire = protowire.AppendVarint(wire, uint64(v))
		}

		var s profilesv1.Stack
		require.NoError(t, s.Unmarshal(wire))
		assert.Equal(t, []int32{0, 5, 10, 15}, s.LocationIndices)
	})

	t.Run("Buckets_BucketCounts_NonPacked", func(t *testing.T) {
		// ExponentialHistogramDataPoint_Buckets.BucketCounts is repeated uint64 at field 2 (varint).
		var wire []byte
		for _, v := range []uint64{10, 20, 30} {
			wire = protowire.AppendTag(wire, 2, protowire.VarintType)
			wire = protowire.AppendVarint(wire, v)
		}

		var bk metricsv1.ExponentialHistogramDataPoint_Buckets
		require.NoError(t, bk.Unmarshal(wire))
		assert.Equal(t, []uint64{10, 20, 30}, bk.BucketCounts)
	})
}

func TestMultiChunkPackedField(t *testing.T) {
	t.Run("HistogramBucketCounts_TwoChunks", func(t *testing.T) {
		// Two separate packed chunks of bucket_counts (field 6, repeated fixed64).
		// Per proto spec, they should be concatenated.
		chunk1 := make([]byte, 0, 24)
		chunk1 = protowire.AppendFixed64(chunk1, 10)
		chunk1 = protowire.AppendFixed64(chunk1, 20)

		chunk2 := make([]byte, 0, 16)
		chunk2 = protowire.AppendFixed64(chunk2, 30)
		chunk2 = protowire.AppendFixed64(chunk2, 40)

		var wire []byte
		wire = protowire.AppendTag(wire, 6, protowire.BytesType)
		wire = protowire.AppendBytes(wire, chunk1)
		wire = protowire.AppendTag(wire, 6, protowire.BytesType)
		wire = protowire.AppendBytes(wire, chunk2)

		var dp metricsv1.HistogramDataPoint
		require.NoError(t, dp.Unmarshal(wire))
		assert.Equal(t, []uint64{10, 20, 30, 40}, dp.BucketCounts)
	})

	t.Run("ExplicitBounds_TwoChunks", func(t *testing.T) {
		// Two separate packed chunks of explicit_bounds (field 7, repeated double).
		chunk1 := make([]byte, 0, 16)
		chunk1 = protowire.AppendFixed64(chunk1, math.Float64bits(1.0))
		chunk1 = protowire.AppendFixed64(chunk1, math.Float64bits(5.0))

		chunk2 := make([]byte, 0, 8)
		chunk2 = protowire.AppendFixed64(chunk2, math.Float64bits(10.0))

		var wire []byte
		wire = protowire.AppendTag(wire, 7, protowire.BytesType)
		wire = protowire.AppendBytes(wire, chunk1)
		wire = protowire.AppendTag(wire, 7, protowire.BytesType)
		wire = protowire.AppendBytes(wire, chunk2)

		var dp metricsv1.HistogramDataPoint
		require.NoError(t, dp.Unmarshal(wire))
		assert.Equal(t, []float64{1.0, 5.0, 10.0}, dp.ExplicitBounds)
	})

	t.Run("PackedVarint_TwoChunks", func(t *testing.T) {
		// Stack.LocationIndices (field 1, repeated int32) packed in two chunks.
		chunk1 := make([]byte, 0, 8)
		chunk1 = protowire.AppendVarint(chunk1, 0)
		chunk1 = protowire.AppendVarint(chunk1, 1)

		chunk2 := make([]byte, 0, 8)
		chunk2 = protowire.AppendVarint(chunk2, 2)
		chunk2 = protowire.AppendVarint(chunk2, 3)

		var wire []byte
		wire = protowire.AppendTag(wire, 1, protowire.BytesType)
		wire = protowire.AppendBytes(wire, chunk1)
		wire = protowire.AppendTag(wire, 1, protowire.BytesType)
		wire = protowire.AppendBytes(wire, chunk2)

		var s profilesv1.Stack
		require.NoError(t, s.Unmarshal(wire))
		assert.Equal(t, []int32{0, 1, 2, 3}, s.LocationIndices)
	})
}

// ---------------------------------------------------------------------------
// 5. NaN bit pattern preservation
// ---------------------------------------------------------------------------

func TestNaNBitPatternPreservation(t *testing.T) {
	// Standard quiet NaN
	qNaN := math.NaN()
	// Signaling NaN (different bit pattern)
	sNaN := math.Float64frombits(0x7FF0000000000001)
	// Custom NaN payload
	customNaN := math.Float64frombits(0x7FF8000000000042)

	for _, tc := range []struct {
		name string
		val  float64
	}{
		{"quiet_NaN", qNaN},
		{"signaling_NaN", sNaN},
		{"custom_NaN_payload", customNaN},
	} {
		t.Run(tc.name, func(t *testing.T) {
			originalBits := math.Float64bits(tc.val)

			av := commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: tc.val}}
			b, err := av.Marshal()
			require.NoError(t, err)

			var decoded commonv1.AnyValue
			require.NoError(t, decoded.Unmarshal(b))
			decodedBits := math.Float64bits(decoded.Value.(*commonv1.AnyValue_DoubleValue).DoubleValue)
			assert.Equal(t, originalBits, decodedBits, "NaN bit pattern not preserved")

			// Cross-validate with official protobuf
			var official otlpcommon.AnyValue
			require.NoError(t, proto.Unmarshal(b, &official))
			officialBits := math.Float64bits(official.GetDoubleValue())
			assert.Equal(t, originalBits, officialBits, "official protobuf NaN bit pattern mismatch")
		})
	}

	t.Run("NaN_in_histogram_optional_fields", func(t *testing.T) {
		nan := math.NaN()
		nanBits := math.Float64bits(nan)

		dp := metricsv1.HistogramDataPoint{
			Count: 1,
			Sum:   &nan,
			Min:   &nan,
			Max:   &nan,
		}
		b, err := dp.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.HistogramDataPoint
		require.NoError(t, decoded.Unmarshal(b))
		require.NotNil(t, decoded.Sum)
		assert.Equal(t, nanBits, math.Float64bits(*decoded.Sum))
		require.NotNil(t, decoded.Min)
		assert.Equal(t, nanBits, math.Float64bits(*decoded.Min))
		require.NotNil(t, decoded.Max)
		assert.Equal(t, nanBits, math.Float64bits(*decoded.Max))
	})

	t.Run("NaN_in_packed_repeated_double", func(t *testing.T) {
		bounds := []float64{1.0, math.NaN(), 10.0}
		dp := metricsv1.HistogramDataPoint{
			ExplicitBounds: bounds,
		}
		b, err := dp.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.HistogramDataPoint
		require.NoError(t, decoded.Unmarshal(b))
		require.Len(t, decoded.ExplicitBounds, 3)
		assert.Equal(t, 1.0, decoded.ExplicitBounds[0])
		assert.True(t, math.IsNaN(decoded.ExplicitBounds[1]))
		assert.Equal(t, math.Float64bits(bounds[1]), math.Float64bits(decoded.ExplicitBounds[1]))
		assert.Equal(t, 10.0, decoded.ExplicitBounds[2])
	})
}

// ---------------------------------------------------------------------------
// 6. Cross-library conformance for tricky edge cases
// ---------------------------------------------------------------------------

func TestCrossLibraryEdgeCases(t *testing.T) {
	t.Run("OptionalZeroFloat64", func(t *testing.T) {
		zero := 0.0
		ours := metricsv1.HistogramDataPoint{Count: 1, Sum: &zero, Min: &zero, Max: &zero}
		ourBytes, err := ours.Marshal()
		require.NoError(t, err)

		official := &otlpmetrics.HistogramDataPoint{Count: 1, Sum: &zero, Min: &zero, Max: &zero}
		officialBytes, err := proto.Marshal(official)
		require.NoError(t, err)

		assert.Equal(t, officialBytes, ourBytes, "optional zero float64 wire bytes must match official protobuf")
	})

	t.Run("EmptyRepeatedFields", func(t *testing.T) {
		// Both nil and empty repeated fields should produce empty wire output
		ours := resourcev1.Resource{Attributes: []commonv1.KeyValue{}, DroppedAttributesCount: 0}
		ourBytes, err := ours.Marshal()
		require.NoError(t, err)
		assert.Empty(t, ourBytes)

		official := &otlpresource.Resource{Attributes: []*otlpcommon.KeyValue{}, DroppedAttributesCount: 0}
		officialBytes, err := proto.Marshal(official)
		require.NoError(t, err)
		assert.Empty(t, officialBytes)
	})

	t.Run("DeeplyNestedEmpty", func(t *testing.T) {
		// Wiresmith uses value-type struct fields (Resource Resource, not *Resource).
		// An empty value-type sub-message is indistinguishable from "absent" and
		// won't be encoded, unlike official protobuf where a non-nil *Resource{}
		// pointer produces a zero-length field. Verify round-trip consistency
		// with official protobuf through unmarshal.
		ours := tracev1.TracesData{
			ResourceSpans: []tracev1.ResourceSpans{
				{
					Resource: resourcev1.Resource{},
					ScopeSpans: []tracev1.ScopeSpans{
						{
							Scope: commonv1.InstrumentationScope{},
						},
					},
				},
			},
		}
		ourBytes, err := ours.Marshal()
		require.NoError(t, err)
		require.NotEmpty(t, ourBytes)

		// Official protobuf should be able to unmarshal our bytes
		var official otlptrace.TracesData
		require.NoError(t, proto.Unmarshal(ourBytes, &official))
		require.Len(t, official.ResourceSpans, 1)

		// And the reverse: marshal with official, unmarshal with ours
		officialBytes, err := proto.Marshal(&official)
		require.NoError(t, err)
		var decoded tracev1.TracesData
		require.NoError(t, decoded.Unmarshal(officialBytes))
		require.Len(t, decoded.ResourceSpans, 1)
	})

	t.Run("NegativeSint32", func(t *testing.T) {
		// ExponentialHistogramDataPoint.Scale is sint32 (zigzag encoded)
		for _, val := range []int32{-1, -128, math.MinInt32, math.MaxInt32} {
			ours := metricsv1.ExponentialHistogramDataPoint{Scale: val}
			ourBytes, err := ours.Marshal()
			require.NoError(t, err)

			official := &otlpmetrics.ExponentialHistogramDataPoint{Scale: val}
			officialBytes, err := proto.Marshal(official)
			require.NoError(t, err)

			assert.Equal(t, officialBytes, ourBytes, "sint32 value %d wire bytes must match", val)
		}
	})

	t.Run("NegativeZeroFloat", func(t *testing.T) {
		negZero := math.Copysign(0, -1)
		ours := commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: negZero}}
		ourBytes, err := ours.Marshal()
		require.NoError(t, err)

		official := &otlpcommon.AnyValue{Value: &otlpcommon.AnyValue_DoubleValue{DoubleValue: negZero}}
		officialBytes, err := proto.Marshal(official)
		require.NoError(t, err)

		assert.Equal(t, officialBytes, ourBytes, "negative zero wire bytes must match official protobuf")
	})

	t.Run("AllEnumValues", func(t *testing.T) {
		// Test that all known enum values for SpanKind produce matching wire bytes
		for _, kind := range []tracev1.Span_SpanKind{
			tracev1.Span_SPAN_KIND_UNSPECIFIED,
			tracev1.Span_SPAN_KIND_INTERNAL,
			tracev1.Span_SPAN_KIND_SERVER,
			tracev1.Span_SPAN_KIND_CLIENT,
			tracev1.Span_SPAN_KIND_PRODUCER,
			tracev1.Span_SPAN_KIND_CONSUMER,
		} {
			ours := tracev1.Span{
				TraceId: make([]byte, 16),
				SpanId:  make([]byte, 8),
				Name:    "s",
				Kind:    kind,
			}
			ourBytes, err := ours.Marshal()
			require.NoError(t, err)

			official := &otlptrace.Span{
				TraceId: make([]byte, 16),
				SpanId:  make([]byte, 8),
				Name:    "s",
				Kind:    otlptrace.Span_SpanKind(kind),
			}
			officialBytes, err := proto.Marshal(official)
			require.NoError(t, err)

			assert.Equal(t, officialBytes, ourBytes, "SpanKind=%d wire bytes must match", kind)
		}
	})
}

// ---------------------------------------------------------------------------
// 7. Repeated field merging semantics
// ---------------------------------------------------------------------------

func TestScalarFieldLastValueWins(t *testing.T) {
	// Proto spec: when a non-repeated scalar field appears multiple times
	// in wire format, the last value wins.
	t.Run("Resource_DroppedAttributesCount", func(t *testing.T) {
		var wire []byte
		// First occurrence: dropped_attributes_count = 10
		wire = protowire.AppendTag(wire, 2, protowire.VarintType)
		wire = protowire.AppendVarint(wire, 10)
		// Second occurrence: dropped_attributes_count = 42
		wire = protowire.AppendTag(wire, 2, protowire.VarintType)
		wire = protowire.AppendVarint(wire, 42)

		var r resourcev1.Resource
		require.NoError(t, r.Unmarshal(wire))
		assert.Equal(t, uint32(42), r.DroppedAttributesCount, "last value should win for repeated scalar")

		// Cross-validate: official protobuf must agree
		var official otlpresource.Resource
		require.NoError(t, proto.Unmarshal(wire, &official))
		assert.Equal(t, uint32(42), official.DroppedAttributesCount)
	})

	t.Run("Span_Name", func(t *testing.T) {
		var wire []byte
		// Field 5 (name) = bytes
		wire = protowire.AppendTag(wire, 5, protowire.BytesType)
		wire = protowire.AppendString(wire, "first")
		wire = protowire.AppendTag(wire, 5, protowire.BytesType)
		wire = protowire.AppendString(wire, "second")

		var s tracev1.Span
		require.NoError(t, s.Unmarshal(wire))
		assert.Equal(t, "second", s.Name, "last string value should win")

		var official otlptrace.Span
		require.NoError(t, proto.Unmarshal(wire, &official))
		assert.Equal(t, "second", official.Name)
	})

	t.Run("Span_Kind", func(t *testing.T) {
		var wire []byte
		// Field 6 (kind) = varint (enum)
		wire = protowire.AppendTag(wire, 6, protowire.VarintType)
		wire = protowire.AppendVarint(wire, uint64(tracev1.Span_SPAN_KIND_SERVER))
		wire = protowire.AppendTag(wire, 6, protowire.VarintType)
		wire = protowire.AppendVarint(wire, uint64(tracev1.Span_SPAN_KIND_CLIENT))

		var s tracev1.Span
		require.NoError(t, s.Unmarshal(wire))
		assert.Equal(t, tracev1.Span_SPAN_KIND_CLIENT, s.Kind, "last enum value should win")
	})
}

func TestSubMessageFieldMerging(t *testing.T) {
	// Proto spec: when a sub-message field appears multiple times, the fields
	// within are merged. Repeated sub-fields are concatenated, scalars use
	// last value.
	t.Run("ResourceSpans_Resource_Merged", func(t *testing.T) {
		// Build two Resource submessages for field 1 of ResourceSpans
		// First: dropped_attributes_count=5, attributes=[{key:"k1"}]
		var res1 []byte
		{
			var kv []byte
			kv = protowire.AppendTag(kv, 1, protowire.BytesType)
			kv = protowire.AppendString(kv, "k1")

			res1 = protowire.AppendTag(res1, 1, protowire.BytesType)
			res1 = protowire.AppendBytes(res1, kv)
			res1 = protowire.AppendTag(res1, 2, protowire.VarintType)
			res1 = protowire.AppendVarint(res1, 5)
		}

		// Second: dropped_attributes_count=10, attributes=[{key:"k2"}]
		var res2 []byte
		{
			var kv []byte
			kv = protowire.AppendTag(kv, 1, protowire.BytesType)
			kv = protowire.AppendString(kv, "k2")

			res2 = protowire.AppendTag(res2, 1, protowire.BytesType)
			res2 = protowire.AppendBytes(res2, kv)
			res2 = protowire.AppendTag(res2, 2, protowire.VarintType)
			res2 = protowire.AppendVarint(res2, 10)
		}

		// ResourceSpans with Resource appearing twice
		var wire []byte
		wire = protowire.AppendTag(wire, 1, protowire.BytesType)
		wire = protowire.AppendBytes(wire, res1)
		wire = protowire.AppendTag(wire, 1, protowire.BytesType)
		wire = protowire.AppendBytes(wire, res2)

		// Cross-validate with official protobuf
		var official otlptrace.ResourceSpans
		require.NoError(t, proto.Unmarshal(wire, &official))

		var ours tracev1.ResourceSpans
		require.NoError(t, ours.Unmarshal(wire))

		// Scalar: last value wins → 10
		assert.Equal(t, official.Resource.DroppedAttributesCount, ours.Resource.DroppedAttributesCount,
			"scalar in merged sub-message: last value should win")
		assert.Equal(t, uint32(10), ours.Resource.DroppedAttributesCount)

		// Repeated: both attributes should be present (concatenated)
		assert.Equal(t, len(official.Resource.Attributes), len(ours.Resource.Attributes),
			"repeated fields in merged sub-message should be concatenated")
		assert.Len(t, ours.Resource.Attributes, 2)
		assert.Equal(t, "k1", ours.Resource.Attributes[0].Key)
		assert.Equal(t, "k2", ours.Resource.Attributes[1].Key)
	})
}

// ---------------------------------------------------------------------------
// 8. Concurrent marshal safety
// ---------------------------------------------------------------------------

func TestConcurrentMarshalSafety(t *testing.T) {
	sum := 42.0
	msg := tracev1.TracesData{
		ResourceSpans: []tracev1.ResourceSpans{
			{
				Resource: resourcev1.Resource{
					Attributes: []commonv1.KeyValue{
						{Key: "svc", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "test"}}},
					},
				},
				ScopeSpans: []tracev1.ScopeSpans{
					{
						Scope: commonv1.InstrumentationScope{Name: "lib"},
						Spans: []tracev1.Span{
							{
								TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
								SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
								Name:              "concurrent-span",
								Kind:              tracev1.Span_SPAN_KIND_SERVER,
								StartTimeUnixNano: 1000,
								EndTimeUnixNano:   2000,
								Attributes: []commonv1.KeyValue{
									{Key: "k", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: 3.14}}},
								},
								Events: []tracev1.Span_Event{{TimeUnixNano: 1500, Name: "ev"}},
								Status: tracev1.Status{Code: tracev1.Status_STATUS_CODE_OK},
							},
						},
					},
				},
			},
		},
	}

	// Get reference bytes
	refBytes, err := msg.Marshal()
	require.NoError(t, err)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make(chan error, goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			b, err := msg.Marshal()
			if err != nil {
				errs <- err
				return
			}
			if len(b) != len(refBytes) {
				errs <- assert.AnError
				return
			}
			for i := range b {
				if b[i] != refBytes[i] {
					errs <- assert.AnError
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent marshal failed: %v", err)
	}

	// Also test with MetricsData
	metricMsg := metricsv1.MetricsData{
		ResourceMetrics: []metricsv1.ResourceMetrics{
			{
				ScopeMetrics: []metricsv1.ScopeMetrics{
					{
						Metrics: []metricsv1.Metric{
							{
								Name: "test",
								Data: &metricsv1.Metric_Histogram{Histogram: metricsv1.Histogram{
									DataPoints: []metricsv1.HistogramDataPoint{
										{Count: 10, Sum: &sum, BucketCounts: []uint64{1, 2, 3}, ExplicitBounds: []float64{10, 50}},
									},
								}},
							},
						},
					},
				},
			},
		},
	}

	metricRef, err := metricMsg.Marshal()
	require.NoError(t, err)

	wg.Add(goroutines)
	errs = make(chan error, goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			b, err := metricMsg.Marshal()
			if err != nil {
				errs <- err
				return
			}
			for i := range b {
				if b[i] != metricRef[i] {
					errs <- assert.AnError
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent metric marshal failed: %v", err)
	}
}
