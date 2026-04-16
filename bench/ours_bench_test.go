package bench

import (
	"testing"

	logsv1 "wiresmith/gen/otlp/logs/v1"
	metricsv1 "wiresmith/gen/otlp/metrics/v1"
	profilesv1 "wiresmith/gen/otlp/profiles/v1development"
	tracev1 "wiresmith/gen/otlp/trace/v1"
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

func BenchmarkMarshalLogs_Ours(b *testing.B) {
	var data logsv1.LogsData
	if err := data.Unmarshal(logsBytes50); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

func BenchmarkMarshalGauge_Ours(b *testing.B) {
	var data metricsv1.MetricsData
	if err := data.Unmarshal(gaugeBytes50); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

func BenchmarkMarshalSum_Ours(b *testing.B) {
	var data metricsv1.MetricsData
	if err := data.Unmarshal(sumBytes50); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

func BenchmarkMarshalExpHistogram_Ours(b *testing.B) {
	var data metricsv1.MetricsData
	if err := data.Unmarshal(expHistBytes50); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

func BenchmarkMarshalSummary_Ours(b *testing.B) {
	var data metricsv1.MetricsData
	if err := data.Unmarshal(summaryBytes50); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

func BenchmarkMarshalProfiles_Ours(b *testing.B) {
	var data profilesv1.ProfilesData
	if err := data.Unmarshal(profilesBytes50); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
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

func BenchmarkUnmarshalLogs_Ours(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out logsv1.LogsData
		_ = out.Unmarshal(logsBytes50)
	}
}

func BenchmarkUnmarshalGauge_Ours(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out metricsv1.MetricsData
		_ = out.Unmarshal(gaugeBytes50)
	}
}

func BenchmarkUnmarshalSum_Ours(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out metricsv1.MetricsData
		_ = out.Unmarshal(sumBytes50)
	}
}

func BenchmarkUnmarshalExpHistogram_Ours(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out metricsv1.MetricsData
		_ = out.Unmarshal(expHistBytes50)
	}
}

func BenchmarkUnmarshalSummary_Ours(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out metricsv1.MetricsData
		_ = out.Unmarshal(summaryBytes50)
	}
}

func BenchmarkUnmarshalProfiles_Ours(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out profilesv1.ProfilesData
		_ = out.Unmarshal(profilesBytes50)
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
