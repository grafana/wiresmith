package bench

import (
	"testing"

	metricsv1 "grafana-protoc/gen/otlp/metrics/v1"
	tracev1 "grafana-protoc/gen/otlp/trace/v1"
)

// --- Marshal ---

func BenchmarkMarshalTraces_Ours(b *testing.B) {
	var data tracev1.TracesData
	if err := data.Unmarshal(tracesBytes100); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

func BenchmarkMarshalHistogram_Ours(b *testing.B) {
	var data metricsv1.MetricsData
	if err := data.Unmarshal(histogramBytes50); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

func BenchmarkMarshalSingleSpan_Ours(b *testing.B) {
	var span tracev1.Span
	if err := span.Unmarshal(singleSpanBytes); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = span.Marshal()
	}
}

// --- Unmarshal ---

func BenchmarkUnmarshalTraces_Ours(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out tracev1.TracesData
		_ = out.Unmarshal(tracesBytes100)
	}
}

func BenchmarkUnmarshalHistogram_Ours(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out metricsv1.MetricsData
		_ = out.Unmarshal(histogramBytes50)
	}
}

func BenchmarkUnmarshalSingleSpan_Ours(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out tracev1.Span
		_ = out.Unmarshal(singleSpanBytes)
	}
}

// --- Size ---

func BenchmarkSizeTraces_Ours(b *testing.B) {
	var data tracev1.TracesData
	if err := data.Unmarshal(tracesBytes100); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = data.Size()
	}
}
