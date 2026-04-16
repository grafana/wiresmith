package bench

import (
	"google.golang.org/protobuf/proto"

	otlpcommon "go.opentelemetry.io/proto/otlp/common/v1"
	otlpmetrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	otlpresource "go.opentelemetry.io/proto/otlp/resource/v1"
	otlptrace "go.opentelemetry.io/proto/otlp/trace/v1"
)

// Canonical wire-format bytes generated once via official proto.
// All benchmarks use these as their sole input source.
var (
	tracesBytes100   []byte
	histogramBytes50 []byte
	singleSpanBytes  []byte
)

func init() {
	tracesBytes100 = mustMarshal(buildCanonicalTracesData(100))
	histogramBytes50 = mustMarshal(buildCanonicalHistogramMetrics(50))

	td := buildCanonicalTracesData(1)
	singleSpanBytes = mustMarshal(td.ResourceSpans[0].ScopeSpans[0].Spans[0])
}

func mustMarshal(m proto.Message) []byte {
	b, err := proto.Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}

func buildCanonicalTracesData(nSpans int) *otlptrace.TracesData {
	spans := make([]*otlptrace.Span, nSpans)
	for i := range spans {
		spans[i] = &otlptrace.Span{
			TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name:              "test-span",
			Kind:              otlptrace.Span_SPAN_KIND_SERVER,
			StartTimeUnixNano: 1000000000,
			EndTimeUnixNano:   2000000000,
			Attributes: []*otlpcommon.KeyValue{
				{Key: "http.method", Value: &otlpcommon.AnyValue{Value: &otlpcommon.AnyValue_StringValue{StringValue: "GET"}}},
				{Key: "http.url", Value: &otlpcommon.AnyValue{Value: &otlpcommon.AnyValue_StringValue{StringValue: "https://example.com/api/v1/resource"}}},
				{Key: "http.status_code", Value: &otlpcommon.AnyValue{Value: &otlpcommon.AnyValue_IntValue{IntValue: 200}}},
			},
			Events: []*otlptrace.Span_Event{
				{TimeUnixNano: 1500000000, Name: "event1"},
			},
			Status: &otlptrace.Status{Code: otlptrace.Status_STATUS_CODE_OK},
		}
	}
	return &otlptrace.TracesData{
		ResourceSpans: []*otlptrace.ResourceSpans{
			{
				Resource: &otlpresource.Resource{
					Attributes: []*otlpcommon.KeyValue{
						{Key: "service.name", Value: &otlpcommon.AnyValue{Value: &otlpcommon.AnyValue_StringValue{StringValue: "bench-service"}}},
					},
				},
				ScopeSpans: []*otlptrace.ScopeSpans{
					{
						Scope: &otlpcommon.InstrumentationScope{Name: "bench-lib", Version: "1.0"},
						Spans: spans,
					},
				},
			},
		},
	}
}

func buildCanonicalHistogramMetrics(nPoints int) *otlpmetrics.MetricsData {
	sum := 1234.5
	min := 0.1
	max := 999.9
	points := make([]*otlpmetrics.HistogramDataPoint, nPoints)
	for i := range points {
		points[i] = &otlpmetrics.HistogramDataPoint{
			StartTimeUnixNano: 1000000000,
			TimeUnixNano:      2000000000,
			Count:             100,
			Sum:               &sum,
			BucketCounts:      []uint64{5, 10, 25, 30, 20, 10},
			ExplicitBounds:    []float64{1, 5, 10, 25, 50},
			Min:               &min,
			Max:               &max,
			Attributes: []*otlpcommon.KeyValue{
				{Key: "method", Value: &otlpcommon.AnyValue{Value: &otlpcommon.AnyValue_StringValue{StringValue: "GET"}}},
			},
		}
	}
	return &otlpmetrics.MetricsData{
		ResourceMetrics: []*otlpmetrics.ResourceMetrics{
			{
				ScopeMetrics: []*otlpmetrics.ScopeMetrics{
					{
						Metrics: []*otlpmetrics.Metric{
							{
								Name: "http.request.duration",
								Data: &otlpmetrics.Metric_Histogram{Histogram: &otlpmetrics.Histogram{
									AggregationTemporality: otlpmetrics.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
									DataPoints:             points,
								}},
							},
						},
					},
				},
			},
		},
	}
}
