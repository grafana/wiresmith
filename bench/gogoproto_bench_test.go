package bench

import (
	"testing"

	gogologs "grafana-protoc/gen/gogopb/logs/v1"
	gogometrics "grafana-protoc/gen/gogopb/metrics/v1"
	gogoprofiles "grafana-protoc/gen/gogopb/profiles/v1development"
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

func BenchmarkMarshalLogs_GogoProto(b *testing.B) {
	var data gogologs.LogsData
	if err := data.Unmarshal(logsBytes50); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

func BenchmarkMarshalGauge_GogoProto(b *testing.B) {
	var data gogometrics.MetricsData
	if err := data.Unmarshal(gaugeBytes50); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

func BenchmarkMarshalSum_GogoProto(b *testing.B) {
	var data gogometrics.MetricsData
	if err := data.Unmarshal(sumBytes50); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

func BenchmarkMarshalExpHistogram_GogoProto(b *testing.B) {
	var data gogometrics.MetricsData
	if err := data.Unmarshal(expHistBytes50); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

func BenchmarkMarshalSummary_GogoProto(b *testing.B) {
	var data gogometrics.MetricsData
	if err := data.Unmarshal(summaryBytes50); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

func BenchmarkMarshalProfiles_GogoProto(b *testing.B) {
	var data gogoprofiles.ProfilesData
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

func BenchmarkUnmarshalLogs_GogoProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out gogologs.LogsData
		_ = out.Unmarshal(logsBytes50)
	}
}

func BenchmarkUnmarshalGauge_GogoProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out gogometrics.MetricsData
		_ = out.Unmarshal(gaugeBytes50)
	}
}

func BenchmarkUnmarshalSum_GogoProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out gogometrics.MetricsData
		_ = out.Unmarshal(sumBytes50)
	}
}

func BenchmarkUnmarshalExpHistogram_GogoProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out gogometrics.MetricsData
		_ = out.Unmarshal(expHistBytes50)
	}
}

func BenchmarkUnmarshalSummary_GogoProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out gogometrics.MetricsData
		_ = out.Unmarshal(summaryBytes50)
	}
}

func BenchmarkUnmarshalProfiles_GogoProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out gogoprofiles.ProfilesData
		_ = out.Unmarshal(profilesBytes50)
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
