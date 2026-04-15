package bench

import (
	"testing"

	"google.golang.org/protobuf/proto"

	commonv1 "grafana-protoc/gen/otlp/common/v1"
	metricsv1 "grafana-protoc/gen/otlp/metrics/v1"
	resourcev1 "grafana-protoc/gen/otlp/resource/v1"
	tracev1 "grafana-protoc/gen/otlp/trace/v1"

	otlpcommon "go.opentelemetry.io/proto/otlp/common/v1"
	otlpmetrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	otlpresource "go.opentelemetry.io/proto/otlp/resource/v1"
	otlptrace "go.opentelemetry.io/proto/otlp/trace/v1"
)

// --- Build test data ---

func buildOurTracesData(nSpans int) tracev1.TracesData {
	spans := make([]tracev1.Span, nSpans)
	for i := range spans {
		spans[i] = tracev1.Span{
			TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name:              "test-span",
			Kind:              tracev1.SPAN_KIND_SERVER,
			StartTimeUnixNano: 1000000000,
			EndTimeUnixNano:   2000000000,
			Attributes: []commonv1.KeyValue{
				{Key: "http.method", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "GET"}}},
				{Key: "http.url", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "https://example.com/api/v1/resource"}}},
				{Key: "http.status_code", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 200}}},
			},
			Events: []tracev1.Span_Event{
				{TimeUnixNano: 1500000000, Name: "event1"},
			},
			Status: tracev1.Status{Code: tracev1.STATUS_CODE_OK},
		}
	}
	return tracev1.TracesData{
		ResourceSpans: []tracev1.ResourceSpans{
			{
				Resource: resourcev1.Resource{
					Attributes: []commonv1.KeyValue{
						{Key: "service.name", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "bench-service"}}},
					},
				},
				ScopeSpans: []tracev1.ScopeSpans{
					{
						Scope: commonv1.InstrumentationScope{Name: "bench-lib", Version: "1.0"},
						Spans: spans,
					},
				},
			},
		},
	}
}

func buildOfficialTracesData(nSpans int) *otlptrace.TracesData {
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

func buildOurHistogramMetrics(nPoints int) metricsv1.MetricsData {
	sum := 1234.5
	min := 0.1
	max := 999.9
	points := make([]metricsv1.HistogramDataPoint, nPoints)
	for i := range points {
		points[i] = metricsv1.HistogramDataPoint{
			StartTimeUnixNano: 1000000000,
			TimeUnixNano:      2000000000,
			Count:             100,
			Sum:               &sum,
			BucketCounts:      []uint64{5, 10, 25, 30, 20, 10},
			ExplicitBounds:    []float64{1, 5, 10, 25, 50},
			Min:               &min,
			Max:               &max,
			Attributes: []commonv1.KeyValue{
				{Key: "method", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "GET"}}},
			},
		}
	}
	return metricsv1.MetricsData{
		ResourceMetrics: []metricsv1.ResourceMetrics{
			{
				ScopeMetrics: []metricsv1.ScopeMetrics{
					{
						Metrics: []metricsv1.Metric{
							{
								Name: "http.request.duration",
								Data: &metricsv1.Metric_Histogram{Histogram: metricsv1.Histogram{
									AggregationTemporality: metricsv1.AGGREGATION_TEMPORALITY_CUMULATIVE,
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

func buildOfficialHistogramMetrics(nPoints int) *otlpmetrics.MetricsData {
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

// --- Marshal benchmarks ---

func BenchmarkMarshalTraces_Ours(b *testing.B) {
	data := buildOurTracesData(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.MarshalProto()
	}
}

func BenchmarkMarshalTraces_Official(b *testing.B) {
	data := buildOfficialTracesData(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proto.Marshal(data)
	}
}

func BenchmarkMarshalHistogram_Ours(b *testing.B) {
	data := buildOurHistogramMetrics(50)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.MarshalProto()
	}
}

func BenchmarkMarshalHistogram_Official(b *testing.B) {
	data := buildOfficialHistogramMetrics(50)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proto.Marshal(data)
	}
}

// --- Unmarshal benchmarks ---

func BenchmarkUnmarshalTraces_Ours(b *testing.B) {
	data := buildOurTracesData(100)
	buf, _ := data.MarshalProto()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out tracev1.TracesData
		_ = out.UnmarshalProto(buf)
	}
}

func BenchmarkUnmarshalTraces_Official(b *testing.B) {
	data := buildOfficialTracesData(100)
	buf, _ := proto.Marshal(data)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out otlptrace.TracesData
		_ = proto.Unmarshal(buf, &out)
	}
}

func BenchmarkUnmarshalHistogram_Ours(b *testing.B) {
	data := buildOurHistogramMetrics(50)
	buf, _ := data.MarshalProto()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out metricsv1.MetricsData
		_ = out.UnmarshalProto(buf)
	}
}

func BenchmarkUnmarshalHistogram_Official(b *testing.B) {
	data := buildOfficialHistogramMetrics(50)
	buf, _ := proto.Marshal(data)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out otlpmetrics.MetricsData
		_ = proto.Unmarshal(buf, &out)
	}
}

// --- Size benchmarks ---

func BenchmarkSizeTraces_Ours(b *testing.B) {
	data := buildOurTracesData(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = data.SizeProto()
	}
}

func BenchmarkSizeTraces_Official(b *testing.B) {
	data := buildOfficialTracesData(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = proto.Size(data)
	}
}

// --- Single span benchmarks ---

func BenchmarkMarshalSingleSpan_Ours(b *testing.B) {
	span := buildOurTracesData(1).ResourceSpans[0].ScopeSpans[0].Spans[0]
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = span.MarshalProto()
	}
}

func BenchmarkMarshalSingleSpan_Official(b *testing.B) {
	span := buildOfficialTracesData(1).ResourceSpans[0].ScopeSpans[0].Spans[0]
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proto.Marshal(span)
	}
}

func BenchmarkUnmarshalSingleSpan_Ours(b *testing.B) {
	span := buildOurTracesData(1).ResourceSpans[0].ScopeSpans[0].Spans[0]
	buf, _ := span.MarshalProto()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out tracev1.Span
		_ = out.UnmarshalProto(buf)
	}
}

func BenchmarkUnmarshalSingleSpan_Official(b *testing.B) {
	span := buildOfficialTracesData(1).ResourceSpans[0].ScopeSpans[0].Spans[0]
	buf, _ := proto.Marshal(span)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out otlptrace.Span
		_ = proto.Unmarshal(buf, &out)
	}
}
