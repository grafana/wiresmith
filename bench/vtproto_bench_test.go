package bench

import (
	"testing"

	"google.golang.org/protobuf/proto"

	vtmetrics "grafana-protoc/gen/vtpb/metrics/v1"
	vttrace "grafana-protoc/gen/vtpb/trace/v1"
)

// --- Marshal ---

func BenchmarkMarshalTraces_VTProto(b *testing.B) {
	var data vttrace.TracesData
	if err := proto.Unmarshal(tracesBytes100, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.MarshalVT()
	}
}

func BenchmarkMarshalHistogram_VTProto(b *testing.B) {
	var data vtmetrics.MetricsData
	if err := proto.Unmarshal(histogramBytes50, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.MarshalVT()
	}
}

func BenchmarkMarshalSingleSpan_VTProto(b *testing.B) {
	var span vttrace.Span
	if err := proto.Unmarshal(singleSpanBytes, &span); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = span.MarshalVT()
	}
}

// --- Unmarshal ---

func BenchmarkUnmarshalTraces_VTProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out vttrace.TracesData
		_ = out.UnmarshalVT(tracesBytes100)
	}
}

func BenchmarkUnmarshalHistogram_VTProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out vtmetrics.MetricsData
		_ = out.UnmarshalVT(histogramBytes50)
	}
}

func BenchmarkUnmarshalSingleSpan_VTProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out vttrace.Span
		_ = out.UnmarshalVT(singleSpanBytes)
	}
}

// --- Size ---

func BenchmarkSizeTraces_VTProto(b *testing.B) {
	var data vttrace.TracesData
	if err := proto.Unmarshal(tracesBytes100, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = data.SizeVT()
	}
}
