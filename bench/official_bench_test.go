package bench

import (
	"testing"

	"google.golang.org/protobuf/proto"

	otlplogs "go.opentelemetry.io/proto/otlp/logs/v1"
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

func BenchmarkMarshalLogs_Official(b *testing.B) {
	var data otlplogs.LogsData
	if err := proto.Unmarshal(logsBytes50, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proto.Marshal(&data)
	}
}

func BenchmarkMarshalGauge_Official(b *testing.B) {
	var data otlpmetrics.MetricsData
	if err := proto.Unmarshal(gaugeBytes50, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proto.Marshal(&data)
	}
}

func BenchmarkMarshalSum_Official(b *testing.B) {
	var data otlpmetrics.MetricsData
	if err := proto.Unmarshal(sumBytes50, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proto.Marshal(&data)
	}
}

func BenchmarkMarshalExpHistogram_Official(b *testing.B) {
	var data otlpmetrics.MetricsData
	if err := proto.Unmarshal(expHistBytes50, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proto.Marshal(&data)
	}
}

func BenchmarkMarshalSummary_Official(b *testing.B) {
	var data otlpmetrics.MetricsData
	if err := proto.Unmarshal(summaryBytes50, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proto.Marshal(&data)
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

func BenchmarkUnmarshalLogs_Official(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out otlplogs.LogsData
		_ = proto.Unmarshal(logsBytes50, &out)
	}
}

func BenchmarkUnmarshalGauge_Official(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out otlpmetrics.MetricsData
		_ = proto.Unmarshal(gaugeBytes50, &out)
	}
}

func BenchmarkUnmarshalSum_Official(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out otlpmetrics.MetricsData
		_ = proto.Unmarshal(sumBytes50, &out)
	}
}

func BenchmarkUnmarshalExpHistogram_Official(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out otlpmetrics.MetricsData
		_ = proto.Unmarshal(expHistBytes50, &out)
	}
}

func BenchmarkUnmarshalSummary_Official(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out otlpmetrics.MetricsData
		_ = proto.Unmarshal(summaryBytes50, &out)
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
