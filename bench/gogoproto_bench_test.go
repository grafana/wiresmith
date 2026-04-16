package bench

import (
	"testing"

	gogometrics "grafana-protoc/gen/gogopb/metrics/v1"
	gogotrace "grafana-protoc/gen/gogopb/trace/v1"
)

// --- Marshal ---

func BenchmarkMarshalTraces_GogoProto(b *testing.B) {
	var data gogotrace.TracesData
	if err := data.Unmarshal(tracesBytes100); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

func BenchmarkMarshalHistogram_GogoProto(b *testing.B) {
	var data gogometrics.MetricsData
	if err := data.Unmarshal(histogramBytes50); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

func BenchmarkMarshalSingleSpan_GogoProto(b *testing.B) {
	var span gogotrace.Span
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

func BenchmarkUnmarshalTraces_GogoProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out gogotrace.TracesData
		_ = out.Unmarshal(tracesBytes100)
	}
}

func BenchmarkUnmarshalHistogram_GogoProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out gogometrics.MetricsData
		_ = out.Unmarshal(histogramBytes50)
	}
}

func BenchmarkUnmarshalSingleSpan_GogoProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out gogotrace.Span
		_ = out.Unmarshal(singleSpanBytes)
	}
}

// --- Size ---

func BenchmarkSizeTraces_GogoProto(b *testing.B) {
	var data gogotrace.TracesData
	if err := data.Unmarshal(tracesBytes100); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = data.Size()
	}
}
