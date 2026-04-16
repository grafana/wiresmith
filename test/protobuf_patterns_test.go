package test

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	commonv1 "wiresmith/gen/otlp/common/v1"
	logsv1 "wiresmith/gen/otlp/logs/v1"
	metricsv1 "wiresmith/gen/otlp/metrics/v1"
	resourcev1 "wiresmith/gen/otlp/resource/v1"
	tracev1 "wiresmith/gen/otlp/trace/v1"

	otlpcommon "go.opentelemetry.io/proto/otlp/common/v1"
	otlpresource "go.opentelemetry.io/proto/otlp/resource/v1"
	otlptrace "go.opentelemetry.io/proto/otlp/trace/v1"
)

// overify marshals, unmarshals, re-marshals, and asserts byte-level determinism.
func overify(t *testing.T, m message) {
	t.Helper()
	b1, err := m.Marshal()
	require.NoError(t, err)

	m2 := newMessage(m)
	require.NoError(t, m2.Unmarshal(b1))

	b2, err := m2.Marshal()
	require.NoError(t, err)

	assert.Equal(t, b1, b2, "marshal→unmarshal→marshal produced different bytes")
}

// newMessage returns a zero-value instance of the same concrete type as m.
func newMessage(m message) message {
	switch m.(type) {
	case *commonv1.AnyValue:
		return new(commonv1.AnyValue)
	case *commonv1.ArrayValue:
		return new(commonv1.ArrayValue)
	case *commonv1.InstrumentationScope:
		return new(commonv1.InstrumentationScope)
	case *commonv1.KeyValue:
		return new(commonv1.KeyValue)
	case *commonv1.KeyValueList:
		return new(commonv1.KeyValueList)
	case *commonv1.EntityRef:
		return new(commonv1.EntityRef)
	case *resourcev1.Resource:
		return new(resourcev1.Resource)
	case *tracev1.TracesData:
		return new(tracev1.TracesData)
	case *tracev1.ResourceSpans:
		return new(tracev1.ResourceSpans)
	case *tracev1.ScopeSpans:
		return new(tracev1.ScopeSpans)
	case *tracev1.Span:
		return new(tracev1.Span)
	case *tracev1.Span_Event:
		return new(tracev1.Span_Event)
	case *tracev1.Span_Link:
		return new(tracev1.Span_Link)
	case *tracev1.Status:
		return new(tracev1.Status)
	case *logsv1.LogsData:
		return new(logsv1.LogsData)
	case *logsv1.ResourceLogs:
		return new(logsv1.ResourceLogs)
	case *logsv1.ScopeLogs:
		return new(logsv1.ScopeLogs)
	case *logsv1.LogRecord:
		return new(logsv1.LogRecord)
	case *metricsv1.MetricsData:
		return new(metricsv1.MetricsData)
	case *metricsv1.ResourceMetrics:
		return new(metricsv1.ResourceMetrics)
	case *metricsv1.ScopeMetrics:
		return new(metricsv1.ScopeMetrics)
	case *metricsv1.Metric:
		return new(metricsv1.Metric)
	case *metricsv1.Gauge:
		return new(metricsv1.Gauge)
	case *metricsv1.Sum:
		return new(metricsv1.Sum)
	case *metricsv1.Histogram:
		return new(metricsv1.Histogram)
	case *metricsv1.HistogramDataPoint:
		return new(metricsv1.HistogramDataPoint)
	case *metricsv1.ExponentialHistogram:
		return new(metricsv1.ExponentialHistogram)
	case *metricsv1.ExponentialHistogramDataPoint:
		return new(metricsv1.ExponentialHistogramDataPoint)
	case *metricsv1.ExponentialHistogramDataPoint_Buckets:
		return new(metricsv1.ExponentialHistogramDataPoint_Buckets)
	case *metricsv1.Summary:
		return new(metricsv1.Summary)
	case *metricsv1.SummaryDataPoint:
		return new(metricsv1.SummaryDataPoint)
	case *metricsv1.SummaryDataPoint_ValueAtQuantile:
		return new(metricsv1.SummaryDataPoint_ValueAtQuantile)
	case *metricsv1.NumberDataPoint:
		return new(metricsv1.NumberDataPoint)
	case *metricsv1.Exemplar:
		return new(metricsv1.Exemplar)
	default:
		panic("unknown message type in newMessage")
	}
}

// ---------------------------------------------------------------------------
// 1. Double-roundtrip determinism
// ---------------------------------------------------------------------------

func TestMarshalDeterminism(t *testing.T) {
	t.Run("Resource", func(t *testing.T) {
		overify(t, &resourcev1.Resource{
			Attributes: []commonv1.KeyValue{
				{Key: "k1", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v1"}}},
				{Key: "k2", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 42}}},
			},
			DroppedAttributesCount: 5,
		})
	})

	t.Run("Span", func(t *testing.T) {
		overify(t, &tracev1.Span{
			TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name:              "test-span",
			Kind:              tracev1.SPAN_KIND_SERVER,
			StartTimeUnixNano: 1000000000,
			EndTimeUnixNano:   2000000000,
			Events: []tracev1.Span_Event{
				{TimeUnixNano: 1500000000, Name: "ev1"},
			},
			Links: []tracev1.Span_Link{
				{TraceId: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}, SpanId: []byte{8, 7, 6, 5, 4, 3, 2, 1}},
			},
			Status: tracev1.Status{Message: "ok", Code: tracev1.STATUS_CODE_OK},
		})
	})

	t.Run("LogRecord", func(t *testing.T) {
		overify(t, &logsv1.LogRecord{
			TimeUnixNano:         1000,
			ObservedTimeUnixNano: 2000,
			SeverityNumber:       logsv1.SEVERITY_NUMBER_ERROR,
			SeverityText:         "ERROR",
			Body:                 commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "something broke"}},
			TraceId:              []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:               []byte{1, 2, 3, 4, 5, 6, 7, 8},
		})
	})

	t.Run("TracesDataNested", func(t *testing.T) {
		overify(t, &tracev1.TracesData{
			ResourceSpans: []tracev1.ResourceSpans{
				{
					Resource: resourcev1.Resource{
						Attributes: []commonv1.KeyValue{
							{Key: "svc", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "a"}}},
						},
					},
					ScopeSpans: []tracev1.ScopeSpans{
						{
							Scope: commonv1.InstrumentationScope{Name: "lib", Version: "1.0"},
							Spans: []tracev1.Span{
								{TraceId: make([]byte, 16), SpanId: make([]byte, 8), Name: "s1"},
								{TraceId: make([]byte, 16), SpanId: make([]byte, 8), Name: "s2"},
							},
						},
					},
					SchemaUrl: "https://example.com/schema",
				},
			},
		})
	})

	t.Run("MetricsDataNested", func(t *testing.T) {
		sum42 := 42.0
		overify(t, &metricsv1.MetricsData{
			ResourceMetrics: []metricsv1.ResourceMetrics{
				{
					ScopeMetrics: []metricsv1.ScopeMetrics{
						{
							Metrics: []metricsv1.Metric{
								{
									Name: "requests",
									Data: &metricsv1.Metric_Histogram{Histogram: metricsv1.Histogram{
										DataPoints: []metricsv1.HistogramDataPoint{
											{Count: 10, Sum: &sum42, BucketCounts: []uint64{1, 2, 3, 4}, ExplicitBounds: []float64{10, 50, 100}},
										},
									}},
								},
							},
						},
					},
				},
			},
		})
	})

	t.Run("LogsDataNested", func(t *testing.T) {
		overify(t, &logsv1.LogsData{
			ResourceLogs: []logsv1.ResourceLogs{
				{
					ScopeLogs: []logsv1.ScopeLogs{
						{
							LogRecords: []logsv1.LogRecord{
								{SeverityNumber: logsv1.SEVERITY_NUMBER_INFO, Body: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "hello"}}},
								{SeverityNumber: logsv1.SEVERITY_NUMBER_WARN, Body: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 999}}},
							},
						},
					},
				},
			},
		})
	})

	t.Run("AllOneofVariants", func(t *testing.T) {
		variants := []commonv1.AnyValue{
			{Value: &commonv1.AnyValue_StringValue{StringValue: "hello"}},
			{Value: &commonv1.AnyValue_BoolValue{BoolValue: true}},
			{Value: &commonv1.AnyValue_IntValue{IntValue: -999}},
			{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: 3.14}},
			{Value: &commonv1.AnyValue_BytesValue{BytesValue: []byte{0xDE, 0xAD}}},
			{Value: &commonv1.AnyValue_ArrayValue{ArrayValue: commonv1.ArrayValue{
				Values: []commonv1.AnyValue{{Value: &commonv1.AnyValue_IntValue{IntValue: 1}}},
			}}},
			{Value: &commonv1.AnyValue_KvlistValue{KvlistValue: commonv1.KeyValueList{
				Values: []commonv1.KeyValue{{Key: "k", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: false}}}},
			}}},
		}
		for _, v := range variants {
			overify(t, &v)
		}
	})

	t.Run("MetricOneofs", func(t *testing.T) {
		metrics := []metricsv1.Metric{
			{Name: "g", Data: &metricsv1.Metric_Gauge{Gauge: metricsv1.Gauge{DataPoints: []metricsv1.NumberDataPoint{{TimeUnixNano: 1, Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: 1.5}}}}}},
			{Name: "s", Data: &metricsv1.Metric_Sum{Sum: metricsv1.Sum{DataPoints: []metricsv1.NumberDataPoint{{TimeUnixNano: 2, Value: &metricsv1.NumberDataPoint_AsInt{AsInt: 100}}}}}},
			{Name: "h", Data: &metricsv1.Metric_Histogram{Histogram: metricsv1.Histogram{DataPoints: []metricsv1.HistogramDataPoint{{Count: 5}}}}},
			{Name: "e", Data: &metricsv1.Metric_ExponentialHistogram{ExponentialHistogram: metricsv1.ExponentialHistogram{DataPoints: []metricsv1.ExponentialHistogramDataPoint{{Count: 3}}}}},
			{Name: "u", Data: &metricsv1.Metric_Summary{Summary: metricsv1.Summary{DataPoints: []metricsv1.SummaryDataPoint{{Count: 7}}}}},
		}
		for _, m := range metrics {
			overify(t, &m)
		}
	})
}

// ---------------------------------------------------------------------------
// 2. Boundary value testing
// ---------------------------------------------------------------------------

func TestBoundaryValues(t *testing.T) {
	t.Run("Int64MaxMin", func(t *testing.T) {
		for _, val := range []int64{math.MaxInt64, math.MinInt64, 0, 1, -1} {
			av := commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: val}}
			b, err := av.Marshal()
			require.NoError(t, err)
			var decoded commonv1.AnyValue
			require.NoError(t, decoded.Unmarshal(b))
			assert.Equal(t, val, decoded.Value.(*commonv1.AnyValue_IntValue).IntValue)

			// Cross-validate with official protobuf
			var official otlpcommon.AnyValue
			require.NoError(t, proto.Unmarshal(b, &official))
			assert.Equal(t, val, official.GetIntValue())
		}
	})

	t.Run("Uint64Max", func(t *testing.T) {
		for _, val := range []uint64{math.MaxUint64, 0, 1} {
			span := tracev1.Span{
				TraceId:           make([]byte, 16),
				SpanId:            make([]byte, 8),
				StartTimeUnixNano: val,
				EndTimeUnixNano:   val,
			}
			b, err := span.Marshal()
			require.NoError(t, err)
			var decoded tracev1.Span
			require.NoError(t, decoded.Unmarshal(b))
			assert.Equal(t, val, decoded.StartTimeUnixNano)
			assert.Equal(t, val, decoded.EndTimeUnixNano)

			var official otlptrace.Span
			require.NoError(t, proto.Unmarshal(b, &official))
			assert.Equal(t, val, official.StartTimeUnixNano)
		}
	})

	t.Run("Float64Extremes", func(t *testing.T) {
		for _, val := range []float64{math.MaxFloat64, math.SmallestNonzeroFloat64, math.Inf(1), math.Inf(-1), math.Copysign(0, -1), 0.0} {
			av := commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: val}}
			b, err := av.Marshal()
			require.NoError(t, err)
			var decoded commonv1.AnyValue
			require.NoError(t, decoded.Unmarshal(b))
			assert.Equal(t, val, decoded.Value.(*commonv1.AnyValue_DoubleValue).DoubleValue)

			var official otlpcommon.AnyValue
			require.NoError(t, proto.Unmarshal(b, &official))
			assert.Equal(t, val, official.GetDoubleValue())
		}
	})

	t.Run("Float64NaN", func(t *testing.T) {
		av := commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: math.NaN()}}
		b, err := av.Marshal()
		require.NoError(t, err)
		var decoded commonv1.AnyValue
		require.NoError(t, decoded.Unmarshal(b))
		assert.True(t, math.IsNaN(decoded.Value.(*commonv1.AnyValue_DoubleValue).DoubleValue))
	})

	t.Run("Float64InHistogram", func(t *testing.T) {
		for _, val := range []float64{math.MaxFloat64, math.SmallestNonzeroFloat64, math.Inf(1), math.Inf(-1)} {
			v := val
			dp := metricsv1.HistogramDataPoint{
				Count: 1,
				Sum:   &v,
				Min:   &v,
				Max:   &v,
			}
			b, err := dp.Marshal()
			require.NoError(t, err)
			var decoded metricsv1.HistogramDataPoint
			require.NoError(t, decoded.Unmarshal(b))
			assert.Equal(t, val, *decoded.Sum)
			assert.Equal(t, val, *decoded.Min)
			assert.Equal(t, val, *decoded.Max)
		}
	})

	t.Run("Uint32Max", func(t *testing.T) {
		for _, val := range []uint32{math.MaxUint32, 0, 1} {
			r := resourcev1.Resource{DroppedAttributesCount: val}
			b, err := r.Marshal()
			require.NoError(t, err)
			var decoded resourcev1.Resource
			require.NoError(t, decoded.Unmarshal(b))
			assert.Equal(t, val, decoded.DroppedAttributesCount)

			if val != 0 {
				var official otlpresource.Resource
				require.NoError(t, proto.Unmarshal(b, &official))
				assert.Equal(t, val, official.DroppedAttributesCount)
			}
		}
	})

	t.Run("Int32Extremes", func(t *testing.T) {
		for _, val := range []int32{math.MaxInt32, math.MinInt32, 0, 1, -1} {
			dp := metricsv1.ExponentialHistogramDataPoint{
				Scale: val,
				Positive: metricsv1.ExponentialHistogramDataPoint_Buckets{
					Offset: val,
				},
			}
			b, err := dp.Marshal()
			require.NoError(t, err)
			var decoded metricsv1.ExponentialHistogramDataPoint
			require.NoError(t, decoded.Unmarshal(b))
			assert.Equal(t, val, decoded.Scale)
			assert.Equal(t, val, decoded.Positive.Offset)
		}
	})

	t.Run("QuantileEdges", func(t *testing.T) {
		for _, q := range []float64{0.0, 0.5, 0.99, 1.0, math.SmallestNonzeroFloat64} {
			vaq := metricsv1.SummaryDataPoint_ValueAtQuantile{Quantile: q, Value: 100.0}
			b, err := vaq.Marshal()
			require.NoError(t, err)
			var decoded metricsv1.SummaryDataPoint_ValueAtQuantile
			require.NoError(t, decoded.Unmarshal(b))
			assert.Equal(t, q, decoded.Quantile)
		}
	})
}

// ---------------------------------------------------------------------------
// 3. Raw wire format tests
// ---------------------------------------------------------------------------

func TestRawWireFormat(t *testing.T) {
	t.Run("ResourceFromBytes", func(t *testing.T) {
		// Build Resource{dropped_attributes_count: 7} by hand.
		// Field 2, varint type => tag = (2 << 3) | 0 = 0x10
		var wire []byte
		wire = protowire.AppendTag(wire, 2, protowire.VarintType)
		wire = protowire.AppendVarint(wire, 7)

		var r resourcev1.Resource
		require.NoError(t, r.Unmarshal(wire))
		assert.Equal(t, uint32(7), r.DroppedAttributesCount)
		assert.Empty(t, r.Attributes)
	})

	t.Run("ResourceWithAttribute", func(t *testing.T) {
		// Build KeyValue{key: "k", value: AnyValue{string_value: "v"}} by hand.
		// KeyValue: field 1 (key) = bytes, field 2 (value) = bytes (sub-message)
		// AnyValue: field 1 (string_value) = bytes
		var kv []byte
		kv = protowire.AppendTag(kv, 1, protowire.BytesType)
		kv = protowire.AppendString(kv, "k")
		// AnyValue sub-message
		var av []byte
		av = protowire.AppendTag(av, 1, protowire.BytesType)
		av = protowire.AppendString(av, "v")
		kv = protowire.AppendTag(kv, 2, protowire.BytesType)
		kv = protowire.AppendBytes(kv, av)

		// Resource: field 1 (attributes) = bytes (repeated sub-message)
		var wire []byte
		wire = protowire.AppendTag(wire, 1, protowire.BytesType)
		wire = protowire.AppendBytes(wire, kv)
		wire = protowire.AppendTag(wire, 2, protowire.VarintType)
		wire = protowire.AppendVarint(wire, 3)

		var r resourcev1.Resource
		require.NoError(t, r.Unmarshal(wire))
		require.Len(t, r.Attributes, 1)
		assert.Equal(t, "k", r.Attributes[0].Key)
		assert.Equal(t, "v", r.Attributes[0].Value.Value.(*commonv1.AnyValue_StringValue).StringValue)
		assert.Equal(t, uint32(3), r.DroppedAttributesCount)
	})

	t.Run("SpanFixed64Fields", func(t *testing.T) {
		// Span: field 7 (start_time_unix_nano) = fixed64, field 8 (end_time_unix_nano) = fixed64
		var wire []byte
		wire = protowire.AppendTag(wire, 7, protowire.Fixed64Type)
		wire = protowire.AppendFixed64(wire, 1000000000)
		wire = protowire.AppendTag(wire, 8, protowire.Fixed64Type)
		wire = protowire.AppendFixed64(wire, 2000000000)

		var s tracev1.Span
		require.NoError(t, s.Unmarshal(wire))
		assert.Equal(t, uint64(1000000000), s.StartTimeUnixNano)
		assert.Equal(t, uint64(2000000000), s.EndTimeUnixNano)
	})

	t.Run("SpanFixed32Flags", func(t *testing.T) {
		// Span: field 16 (flags) = fixed32
		var wire []byte
		wire = protowire.AppendTag(wire, 16, protowire.Fixed32Type)
		wire = protowire.AppendFixed32(wire, 0x00000300)

		var s tracev1.Span
		require.NoError(t, s.Unmarshal(wire))
		assert.Equal(t, uint32(0x00000300), s.Flags)
	})

	t.Run("NumberDataPointOneofAsDouble", func(t *testing.T) {
		// NumberDataPoint: field 4 oneof as_double = fixed64 (double)
		var wire []byte
		wire = protowire.AppendTag(wire, 4, protowire.Fixed64Type)
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], math.Float64bits(3.14))
		wire = append(wire, buf[:]...)

		var dp metricsv1.NumberDataPoint
		require.NoError(t, dp.Unmarshal(wire))
		require.IsType(t, &metricsv1.NumberDataPoint_AsDouble{}, dp.Value)
		assert.InDelta(t, 3.14, dp.Value.(*metricsv1.NumberDataPoint_AsDouble).AsDouble, 1e-10)
	})

	t.Run("NumberDataPointOneofAsInt", func(t *testing.T) {
		// NumberDataPoint: field 6 oneof as_int = sfixed64
		var wire []byte
		wire = protowire.AppendTag(wire, 6, protowire.Fixed64Type)
		var buf [8]byte
		neg42 := int64(-42)
		binary.LittleEndian.PutUint64(buf[:], uint64(neg42))
		wire = append(wire, buf[:]...)

		var dp metricsv1.NumberDataPoint
		require.NoError(t, dp.Unmarshal(wire))
		require.IsType(t, &metricsv1.NumberDataPoint_AsInt{}, dp.Value)
		assert.Equal(t, int64(-42), dp.Value.(*metricsv1.NumberDataPoint_AsInt).AsInt)
	})

	t.Run("PackedRepeatedFixed64", func(t *testing.T) {
		// HistogramDataPoint: field 6 (bucket_counts) = packed repeated fixed64
		var packed []byte
		packed = protowire.AppendFixed64(packed, 10)
		packed = protowire.AppendFixed64(packed, 20)
		packed = protowire.AppendFixed64(packed, 30)

		var wire []byte
		wire = protowire.AppendTag(wire, 6, protowire.BytesType)
		wire = protowire.AppendBytes(wire, packed)

		var dp metricsv1.HistogramDataPoint
		require.NoError(t, dp.Unmarshal(wire))
		assert.Equal(t, []uint64{10, 20, 30}, dp.BucketCounts)
	})

	t.Run("PackedRepeatedFloat64", func(t *testing.T) {
		// HistogramDataPoint: field 7 (explicit_bounds) = packed repeated double
		var packed []byte
		for _, v := range []float64{10.0, 50.0, 100.0} {
			var buf [8]byte
			binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
			packed = append(packed, buf[:]...)
		}

		var wire []byte
		wire = protowire.AppendTag(wire, 7, protowire.BytesType)
		wire = protowire.AppendBytes(wire, packed)

		var dp metricsv1.HistogramDataPoint
		require.NoError(t, dp.Unmarshal(wire))
		assert.Equal(t, []float64{10.0, 50.0, 100.0}, dp.ExplicitBounds)
	})

	t.Run("ReverseResourceToBytes", func(t *testing.T) {
		// Marshal a known struct and verify the exact wire output.
		r := resourcev1.Resource{DroppedAttributesCount: 7}
		b, err := r.Marshal()
		require.NoError(t, err)

		// Expected: tag(2, varint) + varint(7)
		var expected []byte
		expected = protowire.AppendTag(expected, 2, protowire.VarintType)
		expected = protowire.AppendVarint(expected, 7)
		assert.Equal(t, expected, b)
	})

	t.Run("ReverseStatusToBytes", func(t *testing.T) {
		// Status: field 2 (message) = bytes, field 3 (code) = varint
		s := tracev1.Status{Message: "err", Code: tracev1.STATUS_CODE_ERROR}
		b, err := s.Marshal()
		require.NoError(t, err)

		var expected []byte
		expected = protowire.AppendTag(expected, 2, protowire.BytesType)
		expected = protowire.AppendString(expected, "err")
		expected = protowire.AppendTag(expected, 3, protowire.VarintType)
		expected = protowire.AppendVarint(expected, 2) // STATUS_CODE_ERROR = 2

		assert.Equal(t, expected, b)
	})

	t.Run("CrossValidateWithOfficial", func(t *testing.T) {
		// Build wire bytes with official protobuf, unmarshal with wiresmith.
		official := &otlptrace.Span{
			TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name:              "official-span",
			Kind:              otlptrace.Span_SPAN_KIND_CLIENT,
			StartTimeUnixNano: 5000,
			EndTimeUnixNano:   6000,
		}
		officialBytes, err := proto.Marshal(official)
		require.NoError(t, err)

		var ours tracev1.Span
		require.NoError(t, ours.Unmarshal(officialBytes))
		assert.Equal(t, "official-span", ours.Name)
		assert.Equal(t, tracev1.SPAN_KIND_CLIENT, ours.Kind)
		assert.Equal(t, uint64(5000), ours.StartTimeUnixNano)

		// And reverse: wiresmith → official
		oursBytes, err := ours.Marshal()
		require.NoError(t, err)
		var decoded otlptrace.Span
		require.NoError(t, proto.Unmarshal(oursBytes, &decoded))
		assert.Equal(t, "official-span", decoded.Name)
	})
}

// ---------------------------------------------------------------------------
// 4. Size consistency
// ---------------------------------------------------------------------------

func TestSizeConsistency(t *testing.T) {
	t.Run("PopulatedMessages", func(t *testing.T) {
		sum42 := 42.0
		messages := map[string]message{
			"Resource": &resourcev1.Resource{
				Attributes:             []commonv1.KeyValue{{Key: "k", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v"}}}},
				DroppedAttributesCount: 5,
			},
			"Span": &tracev1.Span{
				TraceId: make([]byte, 16), SpanId: make([]byte, 8), Name: "s",
				Kind: tracev1.SPAN_KIND_SERVER, StartTimeUnixNano: 1, EndTimeUnixNano: 2,
			},
			"LogRecord": &logsv1.LogRecord{
				TimeUnixNano: 1000, SeverityNumber: logsv1.SEVERITY_NUMBER_ERROR,
				Body: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "msg"}},
			},
			"HistogramDataPoint": &metricsv1.HistogramDataPoint{
				Count: 10, Sum: &sum42, BucketCounts: []uint64{1, 2, 3}, ExplicitBounds: []float64{10, 50},
			},
			"ExponentialHistogramDataPoint": &metricsv1.ExponentialHistogramDataPoint{
				Count: 5, Scale: -3,
				Positive: metricsv1.ExponentialHistogramDataPoint_Buckets{Offset: 1, BucketCounts: []uint64{1, 2}},
				Negative: metricsv1.ExponentialHistogramDataPoint_Buckets{Offset: -1, BucketCounts: []uint64{3}},
			},
			"SummaryDataPoint": &metricsv1.SummaryDataPoint{
				Count: 100, Sum: 999.9,
				QuantileValues: []metricsv1.SummaryDataPoint_ValueAtQuantile{{Quantile: 0.5, Value: 50}},
			},
			"NumberDataPointDouble": &metricsv1.NumberDataPoint{
				TimeUnixNano: 1, Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: math.MaxFloat64},
			},
			"NumberDataPointInt": &metricsv1.NumberDataPoint{
				TimeUnixNano: 1, Value: &metricsv1.NumberDataPoint_AsInt{AsInt: math.MinInt64},
			},
			"Exemplar": &metricsv1.Exemplar{
				TimeUnixNano: 1000, Value: &metricsv1.Exemplar_AsDouble{AsDouble: 1.5},
				FilteredAttributes: []commonv1.KeyValue{{Key: "f", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: true}}}},
			},
			"AnyValueBytes": &commonv1.AnyValue{Value: &commonv1.AnyValue_BytesValue{BytesValue: []byte{0xFF, 0x00, 0xAB}}},
			"AnyValueKvlist": &commonv1.AnyValue{Value: &commonv1.AnyValue_KvlistValue{KvlistValue: commonv1.KeyValueList{
				Values: []commonv1.KeyValue{{Key: "nested", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 1}}}},
			}}},
			"InstrumentationScope": &commonv1.InstrumentationScope{
				Name: "lib", Version: "2.0",
				Attributes:             []commonv1.KeyValue{{Key: "a", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: true}}}},
				DroppedAttributesCount: 1,
			},
		}

		for name, m := range messages {
			t.Run(name, func(t *testing.T) {
				b, err := m.Marshal()
				require.NoError(t, err)
				assert.Equal(t, m.Size(), len(b), "Size() != len(Marshal())")
			})
		}
	})

	t.Run("EmptyMessages", func(t *testing.T) {
		for name, ctor := range allMessageConstructors() {
			t.Run(name, func(t *testing.T) {
				m := ctor()
				b, err := m.Marshal()
				require.NoError(t, err)
				assert.Equal(t, m.Size(), len(b), "Size() != len(Marshal()) for empty message")
			})
		}
	})

	t.Run("OptionalNilVsZero", func(t *testing.T) {
		zero := 0.0
		// nil optional: Sum field absent
		dpNil := metricsv1.HistogramDataPoint{Count: 1, Sum: nil}
		bNil, err := dpNil.Marshal()
		require.NoError(t, err)
		assert.Equal(t, dpNil.Size(), len(bNil))

		// zero-value optional: Sum field present with value 0.0
		dpZero := metricsv1.HistogramDataPoint{Count: 1, Sum: &zero}
		bZero, err := dpZero.Marshal()
		require.NoError(t, err)
		assert.Equal(t, dpZero.Size(), len(bZero))

		// They must differ in wire output (zero is explicitly encoded)
		assert.NotEqual(t, bNil, bZero, "nil and zero-value optional should produce different wire bytes")
	})

	t.Run("EmptyVsNilSlice", func(t *testing.T) {
		// Both nil and empty slices should produce the same wire output (no fields)
		rNil := resourcev1.Resource{Attributes: nil}
		rEmpty := resourcev1.Resource{Attributes: []commonv1.KeyValue{}}

		bNil, err := rNil.Marshal()
		require.NoError(t, err)
		bEmpty, err := rEmpty.Marshal()
		require.NoError(t, err)
		assert.Equal(t, bNil, bEmpty)
		assert.Equal(t, rNil.Size(), rEmpty.Size())
	})
}

// ---------------------------------------------------------------------------
// 5. Mutation independence
// ---------------------------------------------------------------------------

func TestMutationIndependence(t *testing.T) {
	t.Run("ByteFieldsNotAliased", func(t *testing.T) {
		original := tracev1.Span{
			TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name:              "original",
			StartTimeUnixNano: 1000,
		}

		b1, err := original.Marshal()
		require.NoError(t, err)
		b1Copy := make([]byte, len(b1))
		copy(b1Copy, b1)

		// Unmarshal into a new struct
		var decoded tracev1.Span
		require.NoError(t, decoded.Unmarshal(b1))

		// Mutate the decoded struct's byte fields
		decoded.TraceId[0] = 0xFF
		decoded.SpanId[0] = 0xFF
		decoded.Name = "mutated"
		decoded.StartTimeUnixNano = 9999

		// Re-marshal the original — bytes must be unchanged
		b1Again, err := original.Marshal()
		require.NoError(t, err)
		assert.Equal(t, b1Copy, b1Again, "mutating decoded struct affected original marshal output")
	})

	t.Run("BytesValueNotAliased", func(t *testing.T) {
		original := commonv1.AnyValue{Value: &commonv1.AnyValue_BytesValue{BytesValue: []byte{0xDE, 0xAD, 0xBE, 0xEF}}}
		b1, err := original.Marshal()
		require.NoError(t, err)
		b1Copy := make([]byte, len(b1))
		copy(b1Copy, b1)

		var decoded commonv1.AnyValue
		require.NoError(t, decoded.Unmarshal(b1))

		// Mutate decoded bytes
		decoded.Value.(*commonv1.AnyValue_BytesValue).BytesValue[0] = 0x00

		b1Again, err := original.Marshal()
		require.NoError(t, err)
		assert.Equal(t, b1Copy, b1Again)
	})

	t.Run("SliceFieldsNotAliased", func(t *testing.T) {
		original := resourcev1.Resource{
			Attributes: []commonv1.KeyValue{
				{Key: "k1", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v1"}}},
				{Key: "k2", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 42}}},
			},
			DroppedAttributesCount: 1,
		}

		b1, err := original.Marshal()
		require.NoError(t, err)
		b1Copy := make([]byte, len(b1))
		copy(b1Copy, b1)

		var decoded resourcev1.Resource
		require.NoError(t, decoded.Unmarshal(b1))

		// Mutate decoded slices
		decoded.Attributes[0].Key = "mutated"
		decoded.DroppedAttributesCount = 999

		b1Again, err := original.Marshal()
		require.NoError(t, err)
		assert.Equal(t, b1Copy, b1Again)
	})

	t.Run("PackedFieldsNotAliased", func(t *testing.T) {
		original := metricsv1.HistogramDataPoint{
			BucketCounts:   []uint64{1, 2, 3, 4, 5},
			ExplicitBounds: []float64{10, 20, 30, 40},
			Count:          15,
		}
		b1, err := original.Marshal()
		require.NoError(t, err)
		b1Copy := make([]byte, len(b1))
		copy(b1Copy, b1)

		var decoded metricsv1.HistogramDataPoint
		require.NoError(t, decoded.Unmarshal(b1))

		// Mutate decoded packed fields
		decoded.BucketCounts[0] = 9999
		decoded.ExplicitBounds[0] = 9999.0

		b1Again, err := original.Marshal()
		require.NoError(t, err)
		assert.Equal(t, b1Copy, b1Again)
	})

	t.Run("MarshalOutputIsolated", func(t *testing.T) {
		// Verify that modifying Marshal() output doesn't affect a second Marshal() call.
		span := tracev1.Span{
			TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name:    "test",
		}
		b1, err := span.Marshal()
		require.NoError(t, err)
		b1Copy := make([]byte, len(b1))
		copy(b1Copy, b1)

		// Corrupt the first marshal output
		for i := range b1 {
			b1[i] = 0xFF
		}

		b2, err := span.Marshal()
		require.NoError(t, err)
		assert.Equal(t, b1Copy, b2, "corrupting Marshal() output affected subsequent Marshal()")
	})
}

// ---------------------------------------------------------------------------
// 6. Known-bad wire inputs
// ---------------------------------------------------------------------------

func TestMalformedWireErrors(t *testing.T) {
	t.Run("TruncatedVarint", func(t *testing.T) {
		// A varint where the last byte has the continuation bit set (no terminator)
		wire := []byte{0x08, 0x80} // tag(1, varint) + incomplete varint (high bit set, no follow-up)
		for name, ctor := range allMessageConstructors() {
			t.Run(name, func(t *testing.T) {
				m := ctor()
				// Must not panic
				_ = m.Unmarshal(wire)
			})
		}
	})

	t.Run("VarintOverflow", func(t *testing.T) {
		// >10 byte varint: all bytes have continuation bit set
		wire := []byte{0x08} // tag(1, varint)
		for range 11 {
			wire = append(wire, 0x80)
		}
		wire = append(wire, 0x01) // terminator

		for name, ctor := range allMessageConstructors() {
			t.Run(name, func(t *testing.T) {
				m := ctor()
				_ = m.Unmarshal(wire)
			})
		}
	})

	t.Run("NegativeLengthPrefix", func(t *testing.T) {
		// Bytes field with a huge varint length that wraps negative in int
		var wire []byte
		wire = protowire.AppendTag(wire, 1, protowire.BytesType)
		wire = protowire.AppendVarint(wire, uint64(math.MaxInt64)+1) // wraps negative as int64

		for name, ctor := range allMessageConstructors() {
			t.Run(name, func(t *testing.T) {
				m := ctor()
				err := m.Unmarshal(wire)
				assert.Error(t, err, "should reject negative/huge length prefix")
			})
		}
	})

	t.Run("TruncatedLengthDelimited", func(t *testing.T) {
		// Length says 100 bytes, only 5 present
		var wire []byte
		wire = protowire.AppendTag(wire, 1, protowire.BytesType)
		wire = protowire.AppendVarint(wire, 100)
		wire = append(wire, []byte("short")...)

		for name, ctor := range allMessageConstructors() {
			t.Run(name, func(t *testing.T) {
				m := ctor()
				err := m.Unmarshal(wire)
				assert.Error(t, err, "should reject truncated length-delimited field")
			})
		}
	})

	t.Run("InvalidWireType", func(t *testing.T) {
		// Wire type 6 is invalid
		wire := []byte{0x0E} // tag = (1 << 3) | 6 = field 1, wire type 6
		for name, ctor := range allMessageConstructors() {
			t.Run(name, func(t *testing.T) {
				m := ctor()
				err := m.Unmarshal(wire)
				assert.Error(t, err, "should reject invalid wire type")
			})
		}
	})

	t.Run("TruncatedFixed32", func(t *testing.T) {
		var wire []byte
		wire = protowire.AppendTag(wire, 1, protowire.Fixed32Type)
		wire = append(wire, 0x01, 0x02) // only 2 bytes, need 4

		for name, ctor := range allMessageConstructors() {
			t.Run(name, func(t *testing.T) {
				m := ctor()
				_ = m.Unmarshal(wire)
			})
		}
	})

	t.Run("TruncatedFixed64", func(t *testing.T) {
		var wire []byte
		wire = protowire.AppendTag(wire, 1, protowire.Fixed64Type)
		wire = append(wire, 0x01, 0x02, 0x03) // only 3 bytes, need 8

		for name, ctor := range allMessageConstructors() {
			t.Run(name, func(t *testing.T) {
				m := ctor()
				_ = m.Unmarshal(wire)
			})
		}
	})

	t.Run("CorruptedNestedMessage", func(t *testing.T) {
		// Build a valid Resource with a corrupted KeyValue inside.
		// The outer length is correct, but the inner data is garbage.
		var innerGarbage []byte
		innerGarbage = protowire.AppendTag(innerGarbage, 1, protowire.BytesType)
		innerGarbage = protowire.AppendVarint(innerGarbage, 50) // claims 50 bytes
		innerGarbage = append(innerGarbage, []byte("nope")...)  // only 4 bytes

		var wire []byte
		wire = protowire.AppendTag(wire, 1, protowire.BytesType)
		wire = protowire.AppendBytes(wire, innerGarbage)

		var r resourcev1.Resource
		err := r.Unmarshal(wire)
		assert.Error(t, err, "should detect corruption inside nested message")
	})

	t.Run("EmptyInputIsValid", func(t *testing.T) {
		// Empty input should always succeed (zero-value message)
		for name, ctor := range allMessageConstructors() {
			t.Run(name, func(t *testing.T) {
				m := ctor()
				err := m.Unmarshal([]byte{})
				assert.NoError(t, err, "empty input should produce zero-value message")
			})
		}
	})

	t.Run("NilInputIsValid", func(t *testing.T) {
		for name, ctor := range allMessageConstructors() {
			t.Run(name, func(t *testing.T) {
				m := ctor()
				err := m.Unmarshal(nil)
				assert.NoError(t, err, "nil input should produce zero-value message")
			})
		}
	})

	t.Run("TagOnly", func(t *testing.T) {
		// Just a tag with no payload
		wire := []byte{0x08} // tag(1, varint) with no varint payload
		for name, ctor := range allMessageConstructors() {
			t.Run(name, func(t *testing.T) {
				m := ctor()
				_ = m.Unmarshal(wire)
			})
		}
	})

	t.Run("RepeatedFieldsAppend", func(t *testing.T) {
		// Two separate instances of the same repeated bytes field should append.
		// Resource field 1 (attributes) appears twice.
		var kv1, kv2 []byte
		kv1 = protowire.AppendTag(kv1, 1, protowire.BytesType)
		kv1 = protowire.AppendString(kv1, "k1")
		kv2 = protowire.AppendTag(kv2, 1, protowire.BytesType)
		kv2 = protowire.AppendString(kv2, "k2")

		var wire []byte
		wire = protowire.AppendTag(wire, 1, protowire.BytesType)
		wire = protowire.AppendBytes(wire, kv1)
		wire = protowire.AppendTag(wire, 1, protowire.BytesType)
		wire = protowire.AppendBytes(wire, kv2)

		var r resourcev1.Resource
		require.NoError(t, r.Unmarshal(wire))
		require.Len(t, r.Attributes, 2)
		assert.Equal(t, "k1", r.Attributes[0].Key)
		assert.Equal(t, "k2", r.Attributes[1].Key)
	})

	t.Run("NonPackedRepeatedFixed64", func(t *testing.T) {
		// bucket_counts is repeated fixed64 — non-packed uses individual fixed64 fields.
		var wire []byte
		for _, v := range []uint64{10, 20, 30} {
			wire = protowire.AppendTag(wire, 6, protowire.Fixed64Type)
			wire = protowire.AppendFixed64(wire, v)
		}

		var dp metricsv1.HistogramDataPoint
		require.NoError(t, dp.Unmarshal(wire))
		assert.Equal(t, []uint64{10, 20, 30}, dp.BucketCounts)
	})
}
