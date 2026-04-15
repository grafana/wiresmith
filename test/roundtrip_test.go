package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	commonv1 "grafana-protoc/gen/otlp/common/v1"
	logsv1 "grafana-protoc/gen/otlp/logs/v1"
	metricsv1 "grafana-protoc/gen/otlp/metrics/v1"
	resourcev1 "grafana-protoc/gen/otlp/resource/v1"
	tracev1 "grafana-protoc/gen/otlp/trace/v1"

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

	ourBytes, err := ours.MarshalProto()
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
	require.NoError(t, decoded.UnmarshalProto(officialBytes))
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
		Kind:              tracev1.SPAN_KIND_SERVER,
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
			Code:    tracev1.STATUS_CODE_OK,
			Message: "success",
		},
	}

	ourBytes, err := ours.MarshalProto()
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
	require.NoError(t, decoded.UnmarshalProto(officialBytes))
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
			ourBytes, err := ours.MarshalProto()
			require.NoError(t, err)

			var official otlpcommon.AnyValue
			require.NoError(t, proto.Unmarshal(ourBytes, &official))
			tt.check(t, &official)

			// Reverse
			officialBytes, err := proto.Marshal(&official)
			require.NoError(t, err)
			var decoded commonv1.AnyValue
			require.NoError(t, decoded.UnmarshalProto(officialBytes))

			// Re-marshal and compare bytes
			reBytes, err := decoded.MarshalProto()
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
		SeverityNumber:       logsv1.SEVERITY_NUMBER_ERROR,
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

	ourBytes, err := ours.MarshalProto()
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
	require.NoError(t, decoded.UnmarshalProto(officialBytes))
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

	ourBytes, err := ours.MarshalProto()
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
	require.NoError(t, decoded.UnmarshalProto(officialBytes))
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

	ourBytes, err := ours.MarshalProto()
	require.NoError(t, err)

	var official otlpmetrics.HistogramDataPoint
	require.NoError(t, proto.Unmarshal(ourBytes, &official))

	require.NotNil(t, official.Sum, "optional field set to zero should be present")
	assert.Equal(t, 0.0, *official.Sum)
}

// TestEmptyMessage tests that zero-value structs marshal to empty bytes.
func TestEmptyMessage(t *testing.T) {
	ours := tracev1.TracesData{}
	b, err := ours.MarshalProto()
	require.NoError(t, err)
	assert.Empty(t, b)

	var decoded tracev1.TracesData
	require.NoError(t, decoded.UnmarshalProto(b))
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
				AggregationTemporality: metricsv1.AGGREGATION_TEMPORALITY_CUMULATIVE,
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

			ourBytes, err := ours.MarshalProto()
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
			require.NoError(t, decoded.UnmarshalProto(officialBytes))
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
								Kind:              tracev1.SPAN_KIND_INTERNAL,
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

	ourBytes, err := ours.MarshalProto()
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
	require.NoError(t, decoded.UnmarshalProto(officialBytes))
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
		b, err := ours.MarshalProto()
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
		b, err := ours.MarshalProto()
		require.NoError(t, err)

		var official otlpmetrics.NumberDataPoint
		require.NoError(t, proto.Unmarshal(b, &official))
		assert.Equal(t, int64(-999), official.GetAsInt())
	})
}
