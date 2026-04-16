package bench

import (
	"testing"

	"google.golang.org/protobuf/proto"

	vtlogs "wiresmith/gen/vtpb/logs/v1"
	vtmetrics "wiresmith/gen/vtpb/metrics/v1"
	vtprofiles "wiresmith/gen/vtpb/profiles/v1development"
	vttrace "wiresmith/gen/vtpb/trace/v1"
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

func BenchmarkMarshalLogs_VTProto(b *testing.B) {
	var data vtlogs.LogsData
	if err := proto.Unmarshal(logsBytes50, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.MarshalVT()
	}
}

func BenchmarkMarshalGauge_VTProto(b *testing.B) {
	var data vtmetrics.MetricsData
	if err := proto.Unmarshal(gaugeBytes50, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.MarshalVT()
	}
}

func BenchmarkMarshalSum_VTProto(b *testing.B) {
	var data vtmetrics.MetricsData
	if err := proto.Unmarshal(sumBytes50, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.MarshalVT()
	}
}

func BenchmarkMarshalExpHistogram_VTProto(b *testing.B) {
	var data vtmetrics.MetricsData
	if err := proto.Unmarshal(expHistBytes50, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.MarshalVT()
	}
}

func BenchmarkMarshalSummary_VTProto(b *testing.B) {
	var data vtmetrics.MetricsData
	if err := proto.Unmarshal(summaryBytes50, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.MarshalVT()
	}
}

func BenchmarkMarshalProfiles_VTProto(b *testing.B) {
	var data vtprofiles.ProfilesData
	if err := proto.Unmarshal(profilesBytes50, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.MarshalVT()
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

func BenchmarkUnmarshalLogs_VTProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out vtlogs.LogsData
		_ = out.UnmarshalVT(logsBytes50)
	}
}

func BenchmarkUnmarshalGauge_VTProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out vtmetrics.MetricsData
		_ = out.UnmarshalVT(gaugeBytes50)
	}
}

func BenchmarkUnmarshalSum_VTProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out vtmetrics.MetricsData
		_ = out.UnmarshalVT(sumBytes50)
	}
}

func BenchmarkUnmarshalExpHistogram_VTProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out vtmetrics.MetricsData
		_ = out.UnmarshalVT(expHistBytes50)
	}
}

func BenchmarkUnmarshalSummary_VTProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out vtmetrics.MetricsData
		_ = out.UnmarshalVT(summaryBytes50)
	}
}

func BenchmarkUnmarshalProfiles_VTProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out vtprofiles.ProfilesData
		_ = out.UnmarshalVT(profilesBytes50)
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
