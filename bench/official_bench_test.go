package bench

import (
	"testing"

	"google.golang.org/protobuf/proto"

	otlpmetrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	otlptrace "go.opentelemetry.io/proto/otlp/trace/v1"
)

// --- Marshal ---

func BenchmarkMarshalTraces_Official(b *testing.B) {
	var data otlptrace.TracesData
	if err := proto.Unmarshal(tracesBytes100, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proto.Marshal(&data)
	}
}

func BenchmarkMarshalHistogram_Official(b *testing.B) {
	var data otlpmetrics.MetricsData
	if err := proto.Unmarshal(histogramBytes50, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proto.Marshal(&data)
	}
}

func BenchmarkMarshalSingleSpan_Official(b *testing.B) {
	var span otlptrace.Span
	if err := proto.Unmarshal(singleSpanBytes, &span); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proto.Marshal(&span)
	}
}

// --- Unmarshal ---

func BenchmarkUnmarshalTraces_Official(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out otlptrace.TracesData
		_ = proto.Unmarshal(tracesBytes100, &out)
	}
}

func BenchmarkUnmarshalHistogram_Official(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out otlpmetrics.MetricsData
		_ = proto.Unmarshal(histogramBytes50, &out)
	}
}

func BenchmarkUnmarshalSingleSpan_Official(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out otlptrace.Span
		_ = proto.Unmarshal(singleSpanBytes, &out)
	}
}

// --- Size ---

func BenchmarkSizeTraces_Official(b *testing.B) {
	var data otlptrace.TracesData
	if err := proto.Unmarshal(tracesBytes100, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = proto.Size(&data)
	}
}
