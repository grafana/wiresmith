package bench

import (
	"testing"

	vtcommon "grafana-protoc/bench/vtpb/common/v1"
	vtmetrics "grafana-protoc/bench/vtpb/metrics/v1"
	vtresource "grafana-protoc/bench/vtpb/resource/v1"
	vttrace "grafana-protoc/bench/vtpb/trace/v1"
)

func buildVTTracesData(nSpans int) *vttrace.TracesData {
	spans := make([]*vttrace.Span, nSpans)
	for i := range spans {
		spans[i] = &vttrace.Span{
			TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name:              "test-span",
			Kind:              vttrace.Span_SPAN_KIND_SERVER,
			StartTimeUnixNano: 1000000000,
			EndTimeUnixNano:   2000000000,
			Attributes: []*vtcommon.KeyValue{
				{Key: "http.method", Value: &vtcommon.AnyValue{Value: &vtcommon.AnyValue_StringValue{StringValue: "GET"}}},
				{Key: "http.url", Value: &vtcommon.AnyValue{Value: &vtcommon.AnyValue_StringValue{StringValue: "https://example.com/api/v1/resource"}}},
				{Key: "http.status_code", Value: &vtcommon.AnyValue{Value: &vtcommon.AnyValue_IntValue{IntValue: 200}}},
			},
			Events: []*vttrace.Span_Event{
				{TimeUnixNano: 1500000000, Name: "event1"},
			},
			Status: &vttrace.Status{Code: vttrace.Status_STATUS_CODE_OK},
		}
	}
	return &vttrace.TracesData{
		ResourceSpans: []*vttrace.ResourceSpans{
			{
				Resource: &vtresource.Resource{
					Attributes: []*vtcommon.KeyValue{
						{Key: "service.name", Value: &vtcommon.AnyValue{Value: &vtcommon.AnyValue_StringValue{StringValue: "bench-service"}}},
					},
				},
				ScopeSpans: []*vttrace.ScopeSpans{
					{
						Scope: &vtcommon.InstrumentationScope{Name: "bench-lib", Version: "1.0"},
						Spans: spans,
					},
				},
			},
		},
	}
}

func buildVTHistogramMetrics(nPoints int) *vtmetrics.MetricsData {
	sum := 1234.5
	min := 0.1
	max := 999.9
	points := make([]*vtmetrics.HistogramDataPoint, nPoints)
	for i := range points {
		points[i] = &vtmetrics.HistogramDataPoint{
			StartTimeUnixNano: 1000000000,
			TimeUnixNano:      2000000000,
			Count:             100,
			Sum:               &sum,
			BucketCounts:      []uint64{5, 10, 25, 30, 20, 10},
			ExplicitBounds:    []float64{1, 5, 10, 25, 50},
			Min:               &min,
			Max:               &max,
			Attributes: []*vtcommon.KeyValue{
				{Key: "method", Value: &vtcommon.AnyValue{Value: &vtcommon.AnyValue_StringValue{StringValue: "GET"}}},
			},
		}
	}
	return &vtmetrics.MetricsData{
		ResourceMetrics: []*vtmetrics.ResourceMetrics{
			{
				ScopeMetrics: []*vtmetrics.ScopeMetrics{
					{
						Metrics: []*vtmetrics.Metric{
							{
								Name: "http.request.duration",
								Data: &vtmetrics.Metric_Histogram{Histogram: &vtmetrics.Histogram{
									AggregationTemporality: vtmetrics.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
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

// --- VTProto Marshal benchmarks ---

func BenchmarkMarshalTraces_VTProto(b *testing.B) {
	data := buildVTTracesData(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.MarshalVT()
	}
}

func BenchmarkMarshalHistogram_VTProto(b *testing.B) {
	data := buildVTHistogramMetrics(50)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.MarshalVT()
	}
}

// --- VTProto Unmarshal benchmarks ---

func BenchmarkUnmarshalTraces_VTProto(b *testing.B) {
	data := buildVTTracesData(100)
	buf, _ := data.MarshalVT()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out vttrace.TracesData
		_ = out.UnmarshalVT(buf)
	}
}

func BenchmarkUnmarshalHistogram_VTProto(b *testing.B) {
	data := buildVTHistogramMetrics(50)
	buf, _ := data.MarshalVT()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out vtmetrics.MetricsData
		_ = out.UnmarshalVT(buf)
	}
}

// --- VTProto Size benchmarks ---

func BenchmarkSizeTraces_VTProto(b *testing.B) {
	data := buildVTTracesData(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = data.SizeVT()
	}
}

// --- VTProto Single span benchmarks ---

func BenchmarkMarshalSingleSpan_VTProto(b *testing.B) {
	span := buildVTTracesData(1).ResourceSpans[0].ScopeSpans[0].Spans[0]
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = span.MarshalVT()
	}
}

func BenchmarkUnmarshalSingleSpan_VTProto(b *testing.B) {
	span := buildVTTracesData(1).ResourceSpans[0].ScopeSpans[0].Spans[0]
	buf, _ := span.MarshalVT()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out vttrace.Span
		_ = out.UnmarshalVT(buf)
	}
}
