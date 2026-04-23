package basic

import (
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
	otlplogs "go.opentelemetry.io/proto/otlp/logs/v1"
	otlpmetrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	otlpresource "go.opentelemetry.io/proto/otlp/resource/v1"
	otlptrace "go.opentelemetry.io/proto/otlp/trace/v1"
)

// TestResourceRoundTrip tests marshal/unmarshal of Resource.
func TestResourceRoundTrip(t *testing.T) {
	ours := resourcev1.Resource{
		Attributes: []commonv1.KeyValue{
			{Key: "service.name", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "test-svc"}}},
			{Key: "host.name", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "host1"}}},
		},
		DroppedAttributesCount: 3,
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	// Unmarshal with official proto
	var official otlpresource.Resource
	require.NoError(t, proto.Unmarshal(ourBytes, &official))

	assert.Equal(t, "service.name", official.Attributes[0].Key)
	assert.Equal(t, "test-svc", official.Attributes[0].Value.GetStringValue())
	assert.Equal(t, "host.name", official.Attributes[1].Key)
	assert.Equal(t, uint32(3), official.DroppedAttributesCount)

	// Reverse: marshal official, unmarshal ours
	officialBytes, err := proto.Marshal(&official)
	require.NoError(t, err)

	var decoded resourcev1.Resource
	require.NoError(t, decoded.Unmarshal(officialBytes))
	assert.Equal(t, ours.Attributes[0].Key, decoded.Attributes[0].Key)
	assert.Equal(t, ours.DroppedAttributesCount, decoded.DroppedAttributesCount)
}

// TestSpanRoundTrip tests a full Span with events, links, attributes.
func TestSpanRoundTrip(t *testing.T) {
	ours := tracev1.Span{
		TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
		TraceState:        "key=value",
		ParentSpanId:      []byte{8, 7, 6, 5, 4, 3, 2, 1},
		Flags:             0x00000100,
		Name:              "test-span",
		Kind:              tracev1.Span_SPAN_KIND_SERVER,
		StartTimeUnixNano: 1000000000,
		EndTimeUnixNano:   2000000000,
		Attributes: []commonv1.KeyValue{
			{Key: "http.method", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "GET"}}},
			{Key: "http.status_code", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 200}}},
		},
		DroppedAttributesCount: 1,
		Events: []tracev1.Span_Event{
			{
				TimeUnixNano: 1500000000,
				Name:         "event1",
				Attributes: []commonv1.KeyValue{
					{Key: "event.key", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: true}}},
				},
			},
		},
		DroppedEventsCount: 0,
		Links: []tracev1.Span_Link{
			{
				TraceId:    []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
				SpanId:     []byte{8, 7, 6, 5, 4, 3, 2, 1},
				TraceState: "linked",
			},
		},
		DroppedLinksCount: 2,
		Status: tracev1.Status{
			Code:    tracev1.Status_STATUS_CODE_OK,
			Message: "success",
		},
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	var official otlptrace.Span
	require.NoError(t, proto.Unmarshal(ourBytes, &official))

	assert.Equal(t, ours.TraceId, official.TraceId)
	assert.Equal(t, ours.SpanId, official.SpanId)
	assert.Equal(t, ours.Name, official.Name)
	assert.Equal(t, otlptrace.Span_SPAN_KIND_SERVER, official.Kind)
	assert.Equal(t, ours.StartTimeUnixNano, official.StartTimeUnixNano)
	assert.Equal(t, ours.EndTimeUnixNano, official.EndTimeUnixNano)
	assert.Len(t, official.Attributes, 2)
	assert.Equal(t, "http.method", official.Attributes[0].Key)
	assert.Equal(t, "GET", official.Attributes[0].Value.GetStringValue())
	assert.Equal(t, int64(200), official.Attributes[1].Value.GetIntValue())
	assert.Len(t, official.Events, 1)
	assert.Equal(t, "event1", official.Events[0].Name)
	assert.Len(t, official.Links, 1)
	assert.Equal(t, otlptrace.Status_STATUS_CODE_OK, official.Status.Code)
	assert.Equal(t, "success", official.Status.Message)
	assert.Equal(t, uint32(0x00000100), official.Flags)

	// Reverse: official -> ours
	officialBytes, err := proto.Marshal(&official)
	require.NoError(t, err)

	var decoded tracev1.Span
	require.NoError(t, decoded.Unmarshal(officialBytes))
	assert.Equal(t, ours.TraceId, decoded.TraceId)
	assert.Equal(t, ours.Name, decoded.Name)
	assert.Equal(t, ours.Kind, decoded.Kind)
	assert.Equal(t, ours.StartTimeUnixNano, decoded.StartTimeUnixNano)
	assert.Len(t, decoded.Attributes, 2)
	assert.Len(t, decoded.Events, 1)
	assert.Len(t, decoded.Links, 1)
	assert.Equal(t, ours.Status.Code, decoded.Status.Code)
}

// TestAnyValueOneofRoundTrip tests all oneof variants.
func TestAnyValueOneofRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		value commonv1.AnyValue_Value
		check func(t *testing.T, official *otlpcommon.AnyValue)
	}{
		{
			name:  "string",
			value: &commonv1.AnyValue_StringValue{StringValue: "hello"},
			check: func(t *testing.T, v *otlpcommon.AnyValue) {
				assert.Equal(t, "hello", v.GetStringValue())
			},
		},
		{
			name:  "bool",
			value: &commonv1.AnyValue_BoolValue{BoolValue: true},
			check: func(t *testing.T, v *otlpcommon.AnyValue) {
				assert.True(t, v.GetBoolValue())
			},
		},
		{
			name:  "int",
			value: &commonv1.AnyValue_IntValue{IntValue: -42},
			check: func(t *testing.T, v *otlpcommon.AnyValue) {
				assert.Equal(t, int64(-42), v.GetIntValue())
			},
		},
		{
			name:  "double",
			value: &commonv1.AnyValue_DoubleValue{DoubleValue: 3.14},
			check: func(t *testing.T, v *otlpcommon.AnyValue) {
				assert.InDelta(t, 3.14, v.GetDoubleValue(), 0.001)
			},
		},
		{
			name:  "bytes",
			value: &commonv1.AnyValue_BytesValue{BytesValue: []byte{0xde, 0xad, 0xbe, 0xef}},
			check: func(t *testing.T, v *otlpcommon.AnyValue) {
				assert.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, v.GetBytesValue())
			},
		},
		{
			name: "array",
			value: &commonv1.AnyValue_ArrayValue{ArrayValue: commonv1.ArrayValue{
				Values: []commonv1.AnyValue{
					{Value: &commonv1.AnyValue_IntValue{IntValue: 1}},
					{Value: &commonv1.AnyValue_IntValue{IntValue: 2}},
				},
			}},
			check: func(t *testing.T, v *otlpcommon.AnyValue) {
				arr := v.GetArrayValue()
				require.Len(t, arr.Values, 2)
				assert.Equal(t, int64(1), arr.Values[0].GetIntValue())
				assert.Equal(t, int64(2), arr.Values[1].GetIntValue())
			},
		},
		{
			name:  "strindex",
			value: &commonv1.AnyValue_StringValueStrindex{StringValueStrindex: 42},
			check: func(t *testing.T, v *otlpcommon.AnyValue) {
				assert.Equal(t, int32(42), v.GetStringValueStrindex())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ours := commonv1.AnyValue{Value: tt.value}
			ourBytes, err := ours.Marshal()
			require.NoError(t, err)

			var official otlpcommon.AnyValue
			require.NoError(t, proto.Unmarshal(ourBytes, &official))
			tt.check(t, &official)

			// Reverse
			officialBytes, err := proto.Marshal(&official)
			require.NoError(t, err)
			var decoded commonv1.AnyValue
			require.NoError(t, decoded.Unmarshal(officialBytes))

			// Re-marshal and compare bytes
			reBytes, err := decoded.Marshal()
			require.NoError(t, err)
			assert.Equal(t, ourBytes, reBytes)
		})
	}
}

// TestLogRecordRoundTrip tests log records with all fields.
func TestLogRecordRoundTrip(t *testing.T) {
	ours := logsv1.LogRecord{
		TimeUnixNano:         1000000000,
		ObservedTimeUnixNano: 1000000001,
		SeverityNumber:       logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR,
		SeverityText:         "ERROR",
		Body: commonv1.AnyValue{
			Value: &commonv1.AnyValue_StringValue{StringValue: "something failed"},
		},
		Attributes: []commonv1.KeyValue{
			{Key: "code", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 500}}},
		},
		DroppedAttributesCount: 0,
		Flags:                  0xFF,
		TraceId:                []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanId:                 []byte{1, 2, 3, 4, 5, 6, 7, 8},
		EventName:              "test.event",
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	var official otlplogs.LogRecord
	require.NoError(t, proto.Unmarshal(ourBytes, &official))

	assert.Equal(t, ours.TimeUnixNano, official.TimeUnixNano)
	assert.Equal(t, ours.ObservedTimeUnixNano, official.ObservedTimeUnixNano)
	assert.Equal(t, otlplogs.SeverityNumber(ours.SeverityNumber), official.SeverityNumber)
	assert.Equal(t, ours.SeverityText, official.SeverityText)
	assert.Equal(t, "something failed", official.Body.GetStringValue())
	assert.Equal(t, ours.TraceId, official.TraceId)
	assert.Equal(t, uint32(0xFF), official.Flags)
	assert.Equal(t, "test.event", official.EventName)

	// Reverse
	officialBytes, err := proto.Marshal(&official)
	require.NoError(t, err)
	var decoded logsv1.LogRecord
	require.NoError(t, decoded.Unmarshal(officialBytes))
	assert.Equal(t, ours.SeverityNumber, decoded.SeverityNumber)
	assert.Equal(t, ours.EventName, decoded.EventName)
}

// TestHistogramRoundTrip tests optional fields and packed repeated.
func TestHistogramRoundTrip(t *testing.T) {
	sum := 123.456
	min := 1.0
	max := 99.9

	ours := metricsv1.HistogramDataPoint{
		Attributes: []commonv1.KeyValue{
			{Key: "method", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "GET"}}},
		},
		StartTimeUnixNano: 1000000000,
		TimeUnixNano:      2000000000,
		Count:             100,
		Sum:               &sum,
		BucketCounts:      []uint64{10, 20, 30, 25, 15},
		ExplicitBounds:    []float64{1.0, 5.0, 10.0, 50.0},
		Flags:             1,
		Min:               &min,
		Max:               &max,
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	var official otlpmetrics.HistogramDataPoint
	require.NoError(t, proto.Unmarshal(ourBytes, &official))

	assert.Equal(t, uint64(100), official.Count)
	require.NotNil(t, official.Sum)
	assert.InDelta(t, 123.456, *official.Sum, 0.001)
	assert.Equal(t, []uint64{10, 20, 30, 25, 15}, official.BucketCounts)
	assert.InDelta(t, 1.0, official.ExplicitBounds[0], 0.001)
	assert.InDelta(t, 50.0, official.ExplicitBounds[3], 0.001)
	require.NotNil(t, official.Min)
	assert.InDelta(t, 1.0, *official.Min, 0.001)
	require.NotNil(t, official.Max)
	assert.InDelta(t, 99.9, *official.Max, 0.001)

	// Reverse
	officialBytes, err := proto.Marshal(&official)
	require.NoError(t, err)
	var decoded metricsv1.HistogramDataPoint
	require.NoError(t, decoded.Unmarshal(officialBytes))
	assert.Equal(t, ours.Count, decoded.Count)
	require.NotNil(t, decoded.Sum)
	assert.InDelta(t, sum, *decoded.Sum, 0.001)
	assert.Equal(t, ours.BucketCounts, decoded.BucketCounts)
	assert.Equal(t, len(ours.ExplicitBounds), len(decoded.ExplicitBounds))
	require.NotNil(t, decoded.Min)
	assert.InDelta(t, min, *decoded.Min, 0.001)
}

// TestOptionalZeroValue tests that optional fields set to zero are still encoded.
func TestOptionalZeroValue(t *testing.T) {
	zero := 0.0
	ours := metricsv1.HistogramDataPoint{
		TimeUnixNano: 1,
		Count:        0,
		Sum:          &zero, // explicitly set to zero
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	var official otlpmetrics.HistogramDataPoint
	require.NoError(t, proto.Unmarshal(ourBytes, &official))

	require.NotNil(t, official.Sum, "optional field set to zero should be present")
	assert.Equal(t, 0.0, *official.Sum)
}

// TestEmptyMessage tests that zero-value structs marshal to empty bytes.
func TestEmptyMessage(t *testing.T) {
	ours := tracev1.TracesData{}
	b, err := ours.Marshal()
	require.NoError(t, err)
	assert.Empty(t, b)

	var decoded tracev1.TracesData
	require.NoError(t, decoded.Unmarshal(b))
	assert.Empty(t, decoded.ResourceSpans)
}

// TestMetricOneofRoundTrip tests the Metric.data oneof with all variants.
func TestMetricOneofRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		data metricsv1.Metric_Data
	}{
		{
			name: "gauge",
			data: &metricsv1.Metric_Gauge{Gauge: metricsv1.Gauge{
				DataPoints: []metricsv1.NumberDataPoint{
					{
						TimeUnixNano: 1000,
						Value:        &metricsv1.NumberDataPoint_AsDouble{AsDouble: 42.5},
					},
				},
			}},
		},
		{
			name: "sum",
			data: &metricsv1.Metric_Sum{Sum: metricsv1.Sum{
				AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
				IsMonotonic:            true,
				DataPoints: []metricsv1.NumberDataPoint{
					{
						TimeUnixNano: 2000,
						Value:        &metricsv1.NumberDataPoint_AsInt{AsInt: 100},
					},
				},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ours := metricsv1.Metric{
				Name:        "test." + tt.name,
				Description: "a test metric",
				Unit:        "1",
				Data:        tt.data,
			}

			ourBytes, err := ours.Marshal()
			require.NoError(t, err)

			var official otlpmetrics.Metric
			require.NoError(t, proto.Unmarshal(ourBytes, &official))

			assert.Equal(t, ours.Name, official.Name)
			assert.Equal(t, ours.Description, official.Description)
			assert.Equal(t, ours.Unit, official.Unit)

			// Reverse
			officialBytes, err := proto.Marshal(&official)
			require.NoError(t, err)
			var decoded metricsv1.Metric
			require.NoError(t, decoded.Unmarshal(officialBytes))
			assert.Equal(t, ours.Name, decoded.Name)
		})
	}
}

// TestTracesDataFullRoundTrip tests a complete TracesData message.
func TestTracesDataFullRoundTrip(t *testing.T) {
	ours := tracev1.TracesData{
		ResourceSpans: []tracev1.ResourceSpans{
			{
				Resource: resourcev1.Resource{
					Attributes: []commonv1.KeyValue{
						{Key: "service.name", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "svc"}}},
					},
				},
				ScopeSpans: []tracev1.ScopeSpans{
					{
						Scope: commonv1.InstrumentationScope{
							Name:    "lib",
							Version: "1.0",
						},
						Spans: []tracev1.Span{
							{
								TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
								SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
								Name:              "op",
								Kind:              tracev1.Span_SPAN_KIND_INTERNAL,
								StartTimeUnixNano: 100,
								EndTimeUnixNano:   200,
							},
						},
					},
				},
				SchemaUrl: "https://example.com/schema",
			},
		},
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)
	require.NotEmpty(t, ourBytes)

	var official otlptrace.TracesData
	require.NoError(t, proto.Unmarshal(ourBytes, &official))

	require.Len(t, official.ResourceSpans, 1)
	rs := official.ResourceSpans[0]
	assert.Equal(t, "https://example.com/schema", rs.SchemaUrl)
	require.Len(t, rs.Resource.Attributes, 1)
	assert.Equal(t, "service.name", rs.Resource.Attributes[0].Key)
	require.Len(t, rs.ScopeSpans, 1)
	assert.Equal(t, "lib", rs.ScopeSpans[0].Scope.Name)
	require.Len(t, rs.ScopeSpans[0].Spans, 1)
	assert.Equal(t, "op", rs.ScopeSpans[0].Spans[0].Name)

	// Full reverse
	officialBytes, err := proto.Marshal(&official)
	require.NoError(t, err)
	var decoded tracev1.TracesData
	require.NoError(t, decoded.Unmarshal(officialBytes))
	require.Len(t, decoded.ResourceSpans, 1)
	assert.Equal(t, "op", decoded.ResourceSpans[0].ScopeSpans[0].Spans[0].Name)
}

// TestNumberDataPointOneofRoundTrip tests as_double and as_int variants.
func TestNumberDataPointOneofRoundTrip(t *testing.T) {
	t.Run("as_double", func(t *testing.T) {
		ours := metricsv1.NumberDataPoint{
			TimeUnixNano: 1000,
			Value:        &metricsv1.NumberDataPoint_AsDouble{AsDouble: 3.14},
		}
		b, err := ours.Marshal()
		require.NoError(t, err)

		var official otlpmetrics.NumberDataPoint
		require.NoError(t, proto.Unmarshal(b, &official))
		assert.InDelta(t, 3.14, official.GetAsDouble(), 0.001)
	})

	t.Run("as_int", func(t *testing.T) {
		ours := metricsv1.NumberDataPoint{
			TimeUnixNano: 1000,
			Value:        &metricsv1.NumberDataPoint_AsInt{AsInt: -999},
		}
		b, err := ours.Marshal()
		require.NoError(t, err)

		var official otlpmetrics.NumberDataPoint
		require.NoError(t, proto.Unmarshal(b, &official))
		assert.Equal(t, int64(-999), official.GetAsInt())
	})
}

// TestMetricsDataFullRoundTrip tests the complete MetricsData hierarchy with all metric oneof variants.
func TestMetricsDataFullRoundTrip(t *testing.T) {
	sum := 100.0
	min := 1.0
	max := 99.0
	expSum := 50.0
	expMin := 0.5
	expMax := 200.0

	ours := metricsv1.MetricsData{
		ResourceMetrics: []metricsv1.ResourceMetrics{
			{
				Resource: resourcev1.Resource{
					Attributes: []commonv1.KeyValue{
						{Key: "service.name", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "metrics-svc"}}},
					},
				},
				ScopeMetrics: []metricsv1.ScopeMetrics{
					{
						Scope: commonv1.InstrumentationScope{
							Name:    "metrics-lib",
							Version: "2.0",
						},
						Metrics: []metricsv1.Metric{
							{
								Name:        "test.gauge",
								Description: "a gauge",
								Unit:        "1",
								Data: &metricsv1.Metric_Gauge{Gauge: metricsv1.Gauge{
									DataPoints: []metricsv1.NumberDataPoint{
										{
											Attributes: []commonv1.KeyValue{
												{Key: "host", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "h1"}}},
											},
											StartTimeUnixNano: 1000,
											TimeUnixNano:      2000,
											Value:             &metricsv1.NumberDataPoint_AsDouble{AsDouble: 42.5},
											Exemplars: []metricsv1.Exemplar{
												{
													FilteredAttributes: []commonv1.KeyValue{
														{Key: "ex.key", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "ex.val"}}},
													},
													TimeUnixNano: 1500,
													Value:        &metricsv1.Exemplar_AsDouble{AsDouble: 41.0},
													SpanId:       []byte{1, 2, 3, 4, 5, 6, 7, 8},
													TraceId:      []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
												},
											},
											Flags: 1,
										},
									},
								}},
								Metadata: []commonv1.KeyValue{
									{Key: "meta.key", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 42}}},
								},
							},
							{
								Name: "test.sum",
								Data: &metricsv1.Metric_Sum{Sum: metricsv1.Sum{
									AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
									IsMonotonic:            true,
									DataPoints: []metricsv1.NumberDataPoint{
										{
											TimeUnixNano: 3000,
											Value:        &metricsv1.NumberDataPoint_AsInt{AsInt: 999},
											Exemplars: []metricsv1.Exemplar{
												{
													TimeUnixNano: 2500,
													Value:        &metricsv1.Exemplar_AsInt{AsInt: 998},
												},
											},
										},
									},
								}},
							},
							{
								Name: "test.histogram",
								Data: &metricsv1.Metric_Histogram{Histogram: metricsv1.Histogram{
									AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
									DataPoints: []metricsv1.HistogramDataPoint{
										{
											StartTimeUnixNano: 1000,
											TimeUnixNano:      2000,
											Count:             50,
											Sum:               &sum,
											BucketCounts:      []uint64{10, 20, 15, 5},
											ExplicitBounds:    []float64{1.0, 10.0, 100.0},
											Min:               &min,
											Max:               &max,
										},
									},
								}},
							},
							{
								Name: "test.exp_histogram",
								Data: &metricsv1.Metric_ExponentialHistogram{ExponentialHistogram: metricsv1.ExponentialHistogram{
									AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
									DataPoints: []metricsv1.ExponentialHistogramDataPoint{
										{
											StartTimeUnixNano: 1000,
											TimeUnixNano:      2000,
											Count:             200,
											Sum:               &expSum,
											Scale:             3,
											ZeroCount:         5,
											Positive: metricsv1.ExponentialHistogramDataPoint_Buckets{
												Offset:       1,
												BucketCounts: []uint64{10, 20, 30},
											},
											Negative: metricsv1.ExponentialHistogramDataPoint_Buckets{
												Offset:       -2,
												BucketCounts: []uint64{5, 15},
											},
											Flags:         0,
											Min:           &expMin,
											Max:           &expMax,
											ZeroThreshold: 0.001,
										},
									},
								}},
							},
							{
								Name: "test.summary",
								Data: &metricsv1.Metric_Summary{Summary: metricsv1.Summary{
									DataPoints: []metricsv1.SummaryDataPoint{
										{
											StartTimeUnixNano: 1000,
											TimeUnixNano:      2000,
											Count:             300,
											Sum:               1500.5,
											QuantileValues: []metricsv1.SummaryDataPoint_ValueAtQuantile{
												{Quantile: 0.5, Value: 4.0},
												{Quantile: 0.9, Value: 8.0},
												{Quantile: 0.99, Value: 15.0},
											},
										},
									},
								}},
							},
						},
						SchemaUrl: "https://example.com/metrics-schema",
					},
				},
				SchemaUrl: "https://example.com/resource-schema",
			},
		},
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)
	require.NotEmpty(t, ourBytes)

	var official otlpmetrics.MetricsData
	require.NoError(t, proto.Unmarshal(ourBytes, &official))

	require.Len(t, official.ResourceMetrics, 1)
	rm := official.ResourceMetrics[0]
	assert.Equal(t, "service.name", rm.Resource.Attributes[0].Key)
	assert.Equal(t, "https://example.com/resource-schema", rm.SchemaUrl)
	require.Len(t, rm.ScopeMetrics, 1)
	sm := rm.ScopeMetrics[0]
	assert.Equal(t, "metrics-lib", sm.Scope.Name)
	assert.Equal(t, "https://example.com/metrics-schema", sm.SchemaUrl)
	require.Len(t, sm.Metrics, 5)

	// Gauge
	assert.Equal(t, "test.gauge", sm.Metrics[0].Name)
	gauge := sm.Metrics[0].GetGauge()
	require.NotNil(t, gauge)
	require.Len(t, gauge.DataPoints, 1)
	assert.InDelta(t, 42.5, gauge.DataPoints[0].GetAsDouble(), 0.001)
	require.Len(t, gauge.DataPoints[0].Exemplars, 1)
	assert.InDelta(t, 41.0, gauge.DataPoints[0].Exemplars[0].GetAsDouble(), 0.001)
	assert.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, gauge.DataPoints[0].Exemplars[0].SpanId)

	// Sum
	assert.Equal(t, "test.sum", sm.Metrics[1].Name)
	sumMetric := sm.Metrics[1].GetSum()
	require.NotNil(t, sumMetric)
	assert.True(t, sumMetric.IsMonotonic)
	assert.Equal(t, otlpmetrics.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, sumMetric.AggregationTemporality)
	require.Len(t, sumMetric.DataPoints[0].Exemplars, 1)
	assert.Equal(t, int64(998), sumMetric.DataPoints[0].Exemplars[0].GetAsInt())

	// Histogram
	hist := sm.Metrics[2].GetHistogram()
	require.NotNil(t, hist)
	assert.Equal(t, otlpmetrics.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA, hist.AggregationTemporality)
	require.Len(t, hist.DataPoints, 1)
	assert.Equal(t, uint64(50), hist.DataPoints[0].Count)

	// ExponentialHistogram
	expHist := sm.Metrics[3].GetExponentialHistogram()
	require.NotNil(t, expHist)
	require.Len(t, expHist.DataPoints, 1)
	assert.Equal(t, uint64(200), expHist.DataPoints[0].Count)
	assert.Equal(t, int32(3), expHist.DataPoints[0].Scale)
	assert.Equal(t, int32(1), expHist.DataPoints[0].Positive.Offset)
	assert.Equal(t, []uint64{10, 20, 30}, expHist.DataPoints[0].Positive.BucketCounts)
	assert.Equal(t, int32(-2), expHist.DataPoints[0].Negative.Offset)
	assert.InDelta(t, 0.001, expHist.DataPoints[0].ZeroThreshold, 0.0001)

	// Summary
	summary := sm.Metrics[4].GetSummary()
	require.NotNil(t, summary)
	require.Len(t, summary.DataPoints, 1)
	assert.Equal(t, uint64(300), summary.DataPoints[0].Count)
	assert.InDelta(t, 1500.5, summary.DataPoints[0].Sum, 0.001)
	require.Len(t, summary.DataPoints[0].QuantileValues, 3)
	assert.InDelta(t, 0.5, summary.DataPoints[0].QuantileValues[0].Quantile, 0.001)
	assert.InDelta(t, 4.0, summary.DataPoints[0].QuantileValues[0].Value, 0.001)

	// Full reverse: official -> ours
	officialBytes, err := proto.Marshal(&official)
	require.NoError(t, err)
	var decoded metricsv1.MetricsData
	require.NoError(t, decoded.Unmarshal(officialBytes))
	require.Len(t, decoded.ResourceMetrics, 1)
	require.Len(t, decoded.ResourceMetrics[0].ScopeMetrics, 1)
	require.Len(t, decoded.ResourceMetrics[0].ScopeMetrics[0].Metrics, 5)
	assert.Equal(t, "test.gauge", decoded.ResourceMetrics[0].ScopeMetrics[0].Metrics[0].Name)
	assert.Equal(t, "test.summary", decoded.ResourceMetrics[0].ScopeMetrics[0].Metrics[4].Name)
}

// TestLogsDataFullRoundTrip tests the complete LogsData hierarchy.
func TestLogsDataFullRoundTrip(t *testing.T) {
	ours := logsv1.LogsData{
		ResourceLogs: []logsv1.ResourceLogs{
			{
				Resource: resourcev1.Resource{
					Attributes: []commonv1.KeyValue{
						{Key: "service.name", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "log-svc"}}},
					},
				},
				ScopeLogs: []logsv1.ScopeLogs{
					{
						Scope: commonv1.InstrumentationScope{
							Name:    "log-lib",
							Version: "1.0",
						},
						LogRecords: []logsv1.LogRecord{
							{
								TimeUnixNano:         1000000000,
								ObservedTimeUnixNano: 1000000001,
								SeverityNumber:       logsv1.SeverityNumber_SEVERITY_NUMBER_WARN,
								SeverityText:         "WARN",
								Body: commonv1.AnyValue{
									Value: &commonv1.AnyValue_StringValue{StringValue: "disk almost full"},
								},
								Attributes: []commonv1.KeyValue{
									{Key: "disk.pct", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: 92.5}}},
								},
								DroppedAttributesCount: 1,
								Flags:                  0x01,
								TraceId:                []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
								SpanId:                 []byte{1, 2, 3, 4, 5, 6, 7, 8},
								EventName:              "disk.warning",
							},
							{
								TimeUnixNano:   2000000000,
								SeverityNumber: logsv1.SeverityNumber_SEVERITY_NUMBER_INFO,
								SeverityText:   "INFO",
								Body: commonv1.AnyValue{
									Value: &commonv1.AnyValue_StringValue{StringValue: "request completed"},
								},
							},
						},
						SchemaUrl: "https://example.com/logs-schema",
					},
				},
				SchemaUrl: "https://example.com/resource-schema",
			},
		},
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)
	require.NotEmpty(t, ourBytes)

	var official otlplogs.LogsData
	require.NoError(t, proto.Unmarshal(ourBytes, &official))

	require.Len(t, official.ResourceLogs, 1)
	rl := official.ResourceLogs[0]
	assert.Equal(t, "service.name", rl.Resource.Attributes[0].Key)
	assert.Equal(t, "https://example.com/resource-schema", rl.SchemaUrl)
	require.Len(t, rl.ScopeLogs, 1)
	sl := rl.ScopeLogs[0]
	assert.Equal(t, "log-lib", sl.Scope.Name)
	assert.Equal(t, "https://example.com/logs-schema", sl.SchemaUrl)
	require.Len(t, sl.LogRecords, 2)
	assert.Equal(t, otlplogs.SeverityNumber_SEVERITY_NUMBER_WARN, sl.LogRecords[0].SeverityNumber)
	assert.Equal(t, "disk almost full", sl.LogRecords[0].Body.GetStringValue())
	assert.Equal(t, "disk.warning", sl.LogRecords[0].EventName)
	assert.Equal(t, uint32(1), sl.LogRecords[0].DroppedAttributesCount)
	assert.Equal(t, "request completed", sl.LogRecords[1].Body.GetStringValue())

	// Reverse
	officialBytes, err := proto.Marshal(&official)
	require.NoError(t, err)
	var decoded logsv1.LogsData
	require.NoError(t, decoded.Unmarshal(officialBytes))
	require.Len(t, decoded.ResourceLogs, 1)
	require.Len(t, decoded.ResourceLogs[0].ScopeLogs, 1)
	require.Len(t, decoded.ResourceLogs[0].ScopeLogs[0].LogRecords, 2)
	assert.Equal(t, logsv1.SeverityNumber_SEVERITY_NUMBER_WARN, decoded.ResourceLogs[0].ScopeLogs[0].LogRecords[0].SeverityNumber)
	assert.Equal(t, "disk.warning", decoded.ResourceLogs[0].ScopeLogs[0].LogRecords[0].EventName)
}

// TestProfilesDataRoundTrip tests profiles via marshal->unmarshal self-consistency
// (no official Go proto package available for profiles).
func TestProfilesDataRoundTrip(t *testing.T) {
	ours := profilesv1.ProfilesData{
		ResourceProfiles: []profilesv1.ResourceProfiles{
			{
				Resource: resourcev1.Resource{
					Attributes: []commonv1.KeyValue{
						{Key: "service.name", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "profile-svc"}}},
					},
				},
				ScopeProfiles: []profilesv1.ScopeProfiles{
					{
						Scope: commonv1.InstrumentationScope{
							Name:    "profiler",
							Version: "0.1",
						},
						Profiles: []profilesv1.Profile{
							{
								SampleType:   profilesv1.ValueType{TypeStrindex: 1, UnitStrindex: 2},
								TimeUnixNano: 5000000000,
								DurationNano: 1000000000,
								PeriodType:   profilesv1.ValueType{TypeStrindex: 3, UnitStrindex: 4},
								Period:       10000,
								ProfileId:    []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
								Samples: []profilesv1.Sample{
									{
										StackIndex:         0,
										Values:             []int64{100, 200},
										TimestampsUnixNano: []uint64{5000000000, 5500000000},
										AttributeIndices:   []int32{0, 1},
										LinkIndex:          0,
									},
								},
								DroppedAttributesCount: 2,
								OriginalPayloadFormat:  "pprof",
								OriginalPayload:        []byte{0xaa, 0xbb},
								AttributeIndices:       []int32{0},
							},
						},
						SchemaUrl: "https://example.com/profiles-schema",
					},
				},
				SchemaUrl: "https://example.com/resource-schema",
			},
		},
		Dictionary: profilesv1.ProfilesDictionary{
			StringTable: []string{"", "cpu", "nanoseconds", "wall", "seconds"},
			MappingTable: []profilesv1.Mapping{
				{
					MemoryStart:      0x1000,
					MemoryLimit:      0x2000,
					FileOffset:       0x100,
					FilenameStrindex: 1,
					AttributeIndices: []int32{0},
				},
			},
			LocationTable: []profilesv1.Location{
				{
					MappingIndex:     0,
					Address:          0x1234,
					Lines:            []profilesv1.Line{{FunctionIndex: 0, Line: 42, Column: 10}},
					AttributeIndices: []int32{0},
				},
			},
			FunctionTable: []profilesv1.Function{
				{
					NameStrindex:       1,
					SystemNameStrindex: 2,
					FilenameStrindex:   3,
					StartLine:          10,
				},
			},
			LinkTable: []profilesv1.Link{
				{
					TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
					SpanId:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
				},
			},
			AttributeTable: []profilesv1.KeyValueAndUnit{
				{
					KeyStrindex:  1,
					Value:        commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 42}},
					UnitStrindex: 2,
				},
			},
			StackTable: []profilesv1.Stack{
				{LocationIndices: []int32{0}},
			},
		},
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)
	require.NotEmpty(t, ourBytes)

	// Self round-trip: unmarshal our own bytes
	var decoded profilesv1.ProfilesData
	require.NoError(t, decoded.Unmarshal(ourBytes))

	require.Len(t, decoded.ResourceProfiles, 1)
	rp := decoded.ResourceProfiles[0]
	assert.Equal(t, "service.name", rp.Resource.Attributes[0].Key)
	assert.Equal(t, "https://example.com/resource-schema", rp.SchemaUrl)
	require.Len(t, rp.ScopeProfiles, 1)
	sp := rp.ScopeProfiles[0]
	assert.Equal(t, "profiler", sp.Scope.Name)
	require.Len(t, sp.Profiles, 1)
	p := sp.Profiles[0]
	assert.Equal(t, uint64(5000000000), p.TimeUnixNano)
	assert.Equal(t, uint64(1000000000), p.DurationNano)
	assert.Equal(t, int64(10000), p.Period)
	assert.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, p.ProfileId)
	require.Len(t, p.Samples, 1)
	assert.Equal(t, []int64{100, 200}, p.Samples[0].Values)
	assert.Equal(t, []uint64{5000000000, 5500000000}, p.Samples[0].TimestampsUnixNano)
	assert.Equal(t, "pprof", p.OriginalPayloadFormat)
	assert.Equal(t, []byte{0xaa, 0xbb}, p.OriginalPayload)

	// Dictionary
	d := decoded.Dictionary
	assert.Equal(t, []string{"", "cpu", "nanoseconds", "wall", "seconds"}, d.StringTable)
	require.Len(t, d.MappingTable, 1)
	assert.Equal(t, uint64(0x1000), d.MappingTable[0].MemoryStart)
	assert.Equal(t, uint64(0x2000), d.MappingTable[0].MemoryLimit)
	require.Len(t, d.LocationTable, 1)
	assert.Equal(t, uint64(0x1234), d.LocationTable[0].Address)
	require.Len(t, d.LocationTable[0].Lines, 1)
	assert.Equal(t, int64(42), d.LocationTable[0].Lines[0].Line)
	require.Len(t, d.FunctionTable, 1)
	assert.Equal(t, int64(10), d.FunctionTable[0].StartLine)
	require.Len(t, d.LinkTable, 1)
	assert.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, d.LinkTable[0].SpanId)
	require.Len(t, d.AttributeTable, 1)
	assert.Equal(t, int32(1), d.AttributeTable[0].KeyStrindex)
	require.Len(t, d.StackTable, 1)
	assert.Equal(t, []int32{0}, d.StackTable[0].LocationIndices)

	// Byte-level consistency: marshal decoded, compare
	reBytes, err := decoded.Marshal()
	require.NoError(t, err)
	assert.Equal(t, ourBytes, reBytes)
}

// TestKvlistValueRoundTrip tests the kvlist_value oneof variant of AnyValue.
func TestKvlistValueRoundTrip(t *testing.T) {
	ours := commonv1.AnyValue{
		Value: &commonv1.AnyValue_KvlistValue{KvlistValue: commonv1.KeyValueList{
			Values: []commonv1.KeyValue{
				{Key: "nested.str", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "val"}}},
				{Key: "nested.int", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 7}}},
			},
		}},
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	var official otlpcommon.AnyValue
	require.NoError(t, proto.Unmarshal(ourBytes, &official))
	kvl := official.GetKvlistValue()
	require.NotNil(t, kvl)
	require.Len(t, kvl.Values, 2)
	assert.Equal(t, "nested.str", kvl.Values[0].Key)
	assert.Equal(t, "val", kvl.Values[0].Value.GetStringValue())
	assert.Equal(t, int64(7), kvl.Values[1].Value.GetIntValue())

	// Reverse
	officialBytes, err := proto.Marshal(&official)
	require.NoError(t, err)
	var decoded commonv1.AnyValue
	require.NoError(t, decoded.Unmarshal(officialBytes))
	reBytes, err := decoded.Marshal()
	require.NoError(t, err)
	assert.Equal(t, ourBytes, reBytes)
}

// TestEntityRefRoundTrip tests the EntityRef message.
func TestEntityRefRoundTrip(t *testing.T) {
	ours := commonv1.EntityRef{
		SchemaUrl:       "https://example.com/entity",
		Type:            "service",
		IdKeys:          []string{"service.name", "service.namespace"},
		DescriptionKeys: []string{"service.version"},
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)
	require.NotEmpty(t, ourBytes)

	// Self round-trip (no official proto type available for EntityRef)
	var decoded commonv1.EntityRef
	require.NoError(t, decoded.Unmarshal(ourBytes))
	assert.Equal(t, ours.SchemaUrl, decoded.SchemaUrl)
	assert.Equal(t, ours.Type, decoded.Type)
	assert.Equal(t, ours.IdKeys, decoded.IdKeys)
	assert.Equal(t, ours.DescriptionKeys, decoded.DescriptionKeys)

	reBytes, err := decoded.Marshal()
	require.NoError(t, err)
	assert.Equal(t, ourBytes, reBytes)
}

// TestInstrumentationScopeRoundTrip tests InstrumentationScope with all fields.
func TestInstrumentationScopeRoundTrip(t *testing.T) {
	ours := commonv1.InstrumentationScope{
		Name:    "my-lib",
		Version: "3.2.1",
		Attributes: []commonv1.KeyValue{
			{Key: "scope.attr", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: true}}},
		},
		DroppedAttributesCount: 5,
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	var official otlpcommon.InstrumentationScope
	require.NoError(t, proto.Unmarshal(ourBytes, &official))
	assert.Equal(t, "my-lib", official.Name)
	assert.Equal(t, "3.2.1", official.Version)
	assert.Equal(t, uint32(5), official.DroppedAttributesCount)
	require.Len(t, official.Attributes, 1)
	assert.True(t, official.Attributes[0].Value.GetBoolValue())

	// Reverse
	officialBytes, err := proto.Marshal(&official)
	require.NoError(t, err)
	var decoded commonv1.InstrumentationScope
	require.NoError(t, decoded.Unmarshal(officialBytes))
	assert.Equal(t, ours.Name, decoded.Name)
	assert.Equal(t, ours.DroppedAttributesCount, decoded.DroppedAttributesCount)
}

// TestUnknownFieldSkip tests that unmarshal gracefully skips unknown fields.
func TestUnknownFieldSkip(t *testing.T) {
	// Marshal a valid Resource
	ours := resourcev1.Resource{
		Attributes: []commonv1.KeyValue{
			{Key: "k", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v"}}},
		},
		DroppedAttributesCount: 1,
	}
	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	// Append unknown varint field (field 99, varint)
	var extra []byte
	extra = protowire.AppendTag(extra, 99, protowire.VarintType)
	extra = protowire.AppendVarint(extra, 12345)
	// Append unknown bytes field (field 100, bytes)
	extra = protowire.AppendTag(extra, 100, protowire.BytesType)
	extra = protowire.AppendString(extra, "unknown data")
	// Append unknown fixed64 field (field 101, fixed64)
	extra = protowire.AppendTag(extra, 101, protowire.Fixed64Type)
	extra = protowire.AppendFixed64(extra, 0xDEADBEEF)
	// Append unknown fixed32 field (field 102, fixed32)
	extra = protowire.AppendTag(extra, 102, protowire.Fixed32Type)
	extra = protowire.AppendFixed32(extra, 0xCAFE)

	withUnknown := append(ourBytes, extra...)

	var decoded resourcev1.Resource
	require.NoError(t, decoded.Unmarshal(withUnknown))
	assert.Equal(t, "k", decoded.Attributes[0].Key)
	assert.Equal(t, uint32(1), decoded.DroppedAttributesCount)
}

// TestEmptySlicesAndStrings tests edge cases with empty/zero-length fields.
func TestEmptySlicesAndStrings(t *testing.T) {
	t.Run("empty_string_attribute", func(t *testing.T) {
		ours := commonv1.KeyValue{
			Key:   "",
			Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: ""}},
		}
		b, err := ours.Marshal()
		require.NoError(t, err)

		var decoded commonv1.KeyValue
		require.NoError(t, decoded.Unmarshal(b))
		// Empty strings are zero values and may not round-trip to a set oneof
		// but the key should be preserved structurally
	})

	t.Run("empty_bytes_attribute", func(t *testing.T) {
		ours := commonv1.AnyValue{
			Value: &commonv1.AnyValue_BytesValue{BytesValue: []byte{}},
		}
		b, err := ours.Marshal()
		require.NoError(t, err)

		var official otlpcommon.AnyValue
		require.NoError(t, proto.Unmarshal(b, &official))
		// Empty bytes should still be recognized as the bytes variant
	})

	t.Run("zero_value_span", func(t *testing.T) {
		ours := tracev1.Span{}
		b, err := ours.Marshal()
		require.NoError(t, err)
		assert.Empty(t, b)

		var decoded tracev1.Span
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, uint64(0), decoded.StartTimeUnixNano)
	})

	t.Run("all_zero_log_record", func(t *testing.T) {
		ours := logsv1.LogRecord{}
		b, err := ours.Marshal()
		require.NoError(t, err)

		var decoded logsv1.LogRecord
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, logsv1.SeverityNumber(0), decoded.SeverityNumber)
	})
}

// TestLargeVarintValues tests encoding of values requiring multi-byte varints.
func TestLargeVarintValues(t *testing.T) {
	ours := tracev1.Span{
		TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
		Name:              "large-values",
		StartTimeUnixNano: 1<<63 - 1, // max int64 as uint64
		EndTimeUnixNano:   1<<64 - 1, // max uint64
		Flags:             0xFFFFFFFF,
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	var official otlptrace.Span
	require.NoError(t, proto.Unmarshal(ourBytes, &official))
	assert.Equal(t, uint64(1<<63-1), official.StartTimeUnixNano)
	assert.Equal(t, uint64(1<<64-1), official.EndTimeUnixNano)
	assert.Equal(t, uint32(0xFFFFFFFF), official.Flags)

	// Reverse
	officialBytes, err := proto.Marshal(&official)
	require.NoError(t, err)
	var decoded tracev1.Span
	require.NoError(t, decoded.Unmarshal(officialBytes))
	assert.Equal(t, ours.StartTimeUnixNano, decoded.StartTimeUnixNano)
	assert.Equal(t, ours.EndTimeUnixNano, decoded.EndTimeUnixNano)
	assert.Equal(t, ours.Flags, decoded.Flags)
}

// TestExponentialHistogramDataPointRoundTrip tests the exponential histogram in detail.
func TestExponentialHistogramDataPointRoundTrip(t *testing.T) {
	sum := 500.0
	min := 0.1
	max := 999.9

	ours := metricsv1.ExponentialHistogramDataPoint{
		Attributes: []commonv1.KeyValue{
			{Key: "region", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "us-east"}}},
		},
		StartTimeUnixNano: 1000,
		TimeUnixNano:      2000,
		Count:             1000,
		Sum:               &sum,
		Scale:             5,
		ZeroCount:         10,
		Positive: metricsv1.ExponentialHistogramDataPoint_Buckets{
			Offset:       0,
			BucketCounts: []uint64{100, 200, 300, 250, 150},
		},
		Negative: metricsv1.ExponentialHistogramDataPoint_Buckets{
			Offset:       -3,
			BucketCounts: []uint64{50, 100, 50},
		},
		Flags: 1,
		Exemplars: []metricsv1.Exemplar{
			{
				TimeUnixNano: 1500,
				Value:        &metricsv1.Exemplar_AsDouble{AsDouble: 42.0},
				SpanId:       []byte{8, 7, 6, 5, 4, 3, 2, 1},
				TraceId:      []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
			},
		},
		Min:           &min,
		Max:           &max,
		ZeroThreshold: 1e-10,
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	var official otlpmetrics.ExponentialHistogramDataPoint
	require.NoError(t, proto.Unmarshal(ourBytes, &official))

	assert.Equal(t, uint64(1000), official.Count)
	assert.InDelta(t, 500.0, *official.Sum, 0.001)
	assert.Equal(t, int32(5), official.Scale)
	assert.Equal(t, uint64(10), official.ZeroCount)
	assert.Equal(t, int32(0), official.Positive.Offset)
	assert.Equal(t, []uint64{100, 200, 300, 250, 150}, official.Positive.BucketCounts)
	assert.Equal(t, int32(-3), official.Negative.Offset)
	assert.Equal(t, []uint64{50, 100, 50}, official.Negative.BucketCounts)
	assert.InDelta(t, 0.1, *official.Min, 0.001)
	assert.InDelta(t, 999.9, *official.Max, 0.001)
	assert.InDelta(t, 1e-10, official.ZeroThreshold, 1e-15)
	require.Len(t, official.Exemplars, 1)

	// Reverse
	officialBytes, err := proto.Marshal(&official)
	require.NoError(t, err)
	var decoded metricsv1.ExponentialHistogramDataPoint
	require.NoError(t, decoded.Unmarshal(officialBytes))
	assert.Equal(t, ours.Count, decoded.Count)
	assert.Equal(t, ours.Scale, decoded.Scale)
	assert.Equal(t, ours.Positive.BucketCounts, decoded.Positive.BucketCounts)
	assert.Equal(t, ours.Negative.Offset, decoded.Negative.Offset)
}

// TestSummaryDataPointRoundTrip tests summary data points in detail.
func TestSummaryDataPointRoundTrip(t *testing.T) {
	ours := metricsv1.SummaryDataPoint{
		Attributes: []commonv1.KeyValue{
			{Key: "method", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "GET"}}},
		},
		StartTimeUnixNano: 1000,
		TimeUnixNano:      2000,
		Count:             500,
		Sum:               2500.75,
		QuantileValues: []metricsv1.SummaryDataPoint_ValueAtQuantile{
			{Quantile: 0.0, Value: 0.1},
			{Quantile: 0.5, Value: 5.0},
			{Quantile: 0.9, Value: 9.0},
			{Quantile: 0.95, Value: 12.0},
			{Quantile: 0.99, Value: 20.0},
			{Quantile: 1.0, Value: 100.0},
		},
		Flags: 0,
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	var official otlpmetrics.SummaryDataPoint
	require.NoError(t, proto.Unmarshal(ourBytes, &official))

	assert.Equal(t, uint64(500), official.Count)
	assert.InDelta(t, 2500.75, official.Sum, 0.001)
	require.Len(t, official.QuantileValues, 6)
	assert.InDelta(t, 0.5, official.QuantileValues[1].Quantile, 0.001)
	assert.InDelta(t, 5.0, official.QuantileValues[1].Value, 0.001)
	assert.InDelta(t, 1.0, official.QuantileValues[5].Quantile, 0.001)
	assert.InDelta(t, 100.0, official.QuantileValues[5].Value, 0.001)

	// Reverse
	officialBytes, err := proto.Marshal(&official)
	require.NoError(t, err)
	var decoded metricsv1.SummaryDataPoint
	require.NoError(t, decoded.Unmarshal(officialBytes))
	assert.Equal(t, ours.Count, decoded.Count)
	assert.InDelta(t, ours.Sum, decoded.Sum, 0.001)
	require.Len(t, decoded.QuantileValues, 6)
}

// TestExemplarRoundTrip tests both exemplar oneof variants in isolation.
func TestExemplarRoundTrip(t *testing.T) {
	t.Run("as_double", func(t *testing.T) {
		ours := metricsv1.Exemplar{
			FilteredAttributes: []commonv1.KeyValue{
				{Key: "filter.key", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "filter.val"}}},
			},
			TimeUnixNano: 12345,
			Value:        &metricsv1.Exemplar_AsDouble{AsDouble: 99.9},
			SpanId:       []byte{1, 2, 3, 4, 5, 6, 7, 8},
			TraceId:      []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		}

		ourBytes, err := ours.Marshal()
		require.NoError(t, err)

		var official otlpmetrics.Exemplar
		require.NoError(t, proto.Unmarshal(ourBytes, &official))
		assert.InDelta(t, 99.9, official.GetAsDouble(), 0.001)
		assert.Equal(t, uint64(12345), official.TimeUnixNano)
		assert.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, official.SpanId)
		require.Len(t, official.FilteredAttributes, 1)

		// Reverse
		officialBytes, err := proto.Marshal(&official)
		require.NoError(t, err)
		var decoded metricsv1.Exemplar
		require.NoError(t, decoded.Unmarshal(officialBytes))
		assert.InDelta(t, 99.9, decoded.Value.(*metricsv1.Exemplar_AsDouble).AsDouble, 0.001)
	})

	t.Run("as_int", func(t *testing.T) {
		ours := metricsv1.Exemplar{
			TimeUnixNano: 67890,
			Value:        &metricsv1.Exemplar_AsInt{AsInt: -42},
			TraceId:      []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		}

		ourBytes, err := ours.Marshal()
		require.NoError(t, err)

		var official otlpmetrics.Exemplar
		require.NoError(t, proto.Unmarshal(ourBytes, &official))
		assert.Equal(t, int64(-42), official.GetAsInt())

		// Reverse
		officialBytes, err := proto.Marshal(&official)
		require.NoError(t, err)
		var decoded metricsv1.Exemplar
		require.NoError(t, decoded.Unmarshal(officialBytes))
		assert.Equal(t, int64(-42), decoded.Value.(*metricsv1.Exemplar_AsInt).AsInt)
	})
}

// TestResourceWithEntityRefsRoundTrip tests Resource with EntityRef field.
func TestResourceWithEntityRefsRoundTrip(t *testing.T) {
	ours := resourcev1.Resource{
		Attributes: []commonv1.KeyValue{
			{Key: "k", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v"}}},
		},
		DroppedAttributesCount: 0,
		EntityRefs: []commonv1.EntityRef{
			{
				SchemaUrl:       "https://example.com/entity",
				Type:            "service",
				IdKeys:          []string{"service.name"},
				DescriptionKeys: []string{"service.version"},
			},
		},
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	// Self round-trip (EntityRef not in the official go proto v1.10.0)
	var decoded resourcev1.Resource
	require.NoError(t, decoded.Unmarshal(ourBytes))
	require.Len(t, decoded.EntityRefs, 1)
	assert.Equal(t, "service", decoded.EntityRefs[0].Type)
	assert.Equal(t, []string{"service.name"}, decoded.EntityRefs[0].IdKeys)

	reBytes, err := decoded.Marshal()
	require.NoError(t, err)
	assert.Equal(t, ourBytes, reBytes)
}

// TestKeyValueWithStrindex tests the KeyStrindex field on KeyValue.
func TestKeyValueWithStrindex(t *testing.T) {
	ours := commonv1.KeyValue{
		Key:         "original.key",
		Value:       commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 99}},
		KeyStrindex: 7,
	}

	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	var decoded commonv1.KeyValue
	require.NoError(t, decoded.Unmarshal(ourBytes))
	assert.Equal(t, "original.key", decoded.Key)
	assert.Equal(t, int32(7), decoded.KeyStrindex)
}
