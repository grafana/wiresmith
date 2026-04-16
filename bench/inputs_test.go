package bench

import (
	"google.golang.org/protobuf/proto"

	otlpcommon "go.opentelemetry.io/proto/otlp/common/v1"
	otlplogs "go.opentelemetry.io/proto/otlp/logs/v1"
	otlpmetrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	otlpresource "go.opentelemetry.io/proto/otlp/resource/v1"
	otlptrace "go.opentelemetry.io/proto/otlp/trace/v1"

	vtcommon "grafana-protoc/gen/vtpb/common/v1"
	vtprofiles "grafana-protoc/gen/vtpb/profiles/v1development"
	vtresource "grafana-protoc/gen/vtpb/resource/v1"
)

// Canonical wire-format bytes generated once via official proto.
// All benchmarks use these as their sole input source.
// Profiles use vtproto types because official proto lacks a profiles package.
var (
	tracesBytes100   []byte
	histogramBytes50 []byte
	singleSpanBytes  []byte
	logsBytes50      []byte
	gaugeBytes50     []byte
	sumBytes50       []byte
	expHistBytes50   []byte
	summaryBytes50   []byte
	profilesBytes50  []byte
)

func init() {
	tracesBytes100 = mustMarshal(buildCanonicalTracesData(100))
	histogramBytes50 = mustMarshal(buildCanonicalHistogramMetrics(50))
	logsBytes50 = mustMarshal(buildCanonicalLogsData(50))
	gaugeBytes50 = mustMarshal(buildCanonicalGaugeMetrics(50))
	sumBytes50 = mustMarshal(buildCanonicalSumMetrics(50))
	expHistBytes50 = mustMarshal(buildCanonicalExpHistogramMetrics(50))
	summaryBytes50 = mustMarshal(buildCanonicalSummaryMetrics(50))
	profilesBytes50 = mustMarshal(buildCanonicalProfilesData(50))

	td := buildCanonicalTracesData(1)
	singleSpanBytes = mustMarshal(td.ResourceSpans[0].ScopeSpans[0].Spans[0])
}

func mustMarshal(m proto.Message) []byte {
	b, err := proto.Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}

func buildCanonicalTracesData(nSpans int) *otlptrace.TracesData {
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

func buildCanonicalHistogramMetrics(nPoints int) *otlpmetrics.MetricsData {
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

func buildCanonicalLogsData(nRecords int) *otlplogs.LogsData {
	records := make([]*otlplogs.LogRecord, nRecords)
	for i := range records {
		records[i] = &otlplogs.LogRecord{
			TimeUnixNano:         uint64(1000000000 + i*1000000),
			ObservedTimeUnixNano: uint64(1000000000 + i*1000000 + 500),
			SeverityNumber:       otlplogs.SeverityNumber_SEVERITY_NUMBER_INFO,
			SeverityText:         "INFO",
			Body:                 &otlpcommon.AnyValue{Value: &otlpcommon.AnyValue_StringValue{StringValue: "request completed successfully"}},
			Attributes: []*otlpcommon.KeyValue{
				{Key: "http.method", Value: &otlpcommon.AnyValue{Value: &otlpcommon.AnyValue_StringValue{StringValue: "GET"}}},
				{Key: "http.status_code", Value: &otlpcommon.AnyValue{Value: &otlpcommon.AnyValue_IntValue{IntValue: 200}}},
				{Key: "http.path", Value: &otlpcommon.AnyValue{Value: &otlpcommon.AnyValue_StringValue{StringValue: "/api/v1/users"}}},
			},
			TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
		}
	}
	return &otlplogs.LogsData{
		ResourceLogs: []*otlplogs.ResourceLogs{
			{
				Resource: &otlpresource.Resource{
					Attributes: []*otlpcommon.KeyValue{
						{Key: "service.name", Value: &otlpcommon.AnyValue{Value: &otlpcommon.AnyValue_StringValue{StringValue: "bench-service"}}},
					},
				},
				ScopeLogs: []*otlplogs.ScopeLogs{
					{
						Scope:      &otlpcommon.InstrumentationScope{Name: "bench-lib", Version: "1.0"},
						LogRecords: records,
					},
				},
			},
		},
	}
}

func buildCanonicalGaugeMetrics(nPoints int) *otlpmetrics.MetricsData {
	points := make([]*otlpmetrics.NumberDataPoint, nPoints)
	for i := range points {
		points[i] = &otlpmetrics.NumberDataPoint{
			StartTimeUnixNano: 1000000000,
			TimeUnixNano:      uint64(2000000000 + i*1000000),
			Value:             &otlpmetrics.NumberDataPoint_AsDouble{AsDouble: 42.5 + float64(i)},
			Attributes: []*otlpcommon.KeyValue{
				{Key: "host.name", Value: &otlpcommon.AnyValue{Value: &otlpcommon.AnyValue_StringValue{StringValue: "worker-01"}}},
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
								Name: "system.cpu.utilization",
								Data: &otlpmetrics.Metric_Gauge{Gauge: &otlpmetrics.Gauge{
									DataPoints: points,
								}},
							},
						},
					},
				},
			},
		},
	}
}

func buildCanonicalSumMetrics(nPoints int) *otlpmetrics.MetricsData {
	points := make([]*otlpmetrics.NumberDataPoint, nPoints)
	for i := range points {
		points[i] = &otlpmetrics.NumberDataPoint{
			StartTimeUnixNano: 1000000000,
			TimeUnixNano:      uint64(2000000000 + i*1000000),
			Value:             &otlpmetrics.NumberDataPoint_AsInt{AsInt: int64(1000 + i*10)},
			Attributes: []*otlpcommon.KeyValue{
				{Key: "http.method", Value: &otlpcommon.AnyValue{Value: &otlpcommon.AnyValue_StringValue{StringValue: "POST"}}},
				{Key: "http.status_code", Value: &otlpcommon.AnyValue{Value: &otlpcommon.AnyValue_IntValue{IntValue: 201}}},
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
								Name: "http.server.request_count",
								Data: &otlpmetrics.Metric_Sum{Sum: &otlpmetrics.Sum{
									AggregationTemporality: otlpmetrics.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
									IsMonotonic:            true,
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

func buildCanonicalExpHistogramMetrics(nPoints int) *otlpmetrics.MetricsData {
	sum := 5678.9
	min := 0.01
	max := 2000.0
	points := make([]*otlpmetrics.ExponentialHistogramDataPoint, nPoints)
	for i := range points {
		points[i] = &otlpmetrics.ExponentialHistogramDataPoint{
			StartTimeUnixNano: 1000000000,
			TimeUnixNano:      2000000000,
			Count:             200,
			Sum:               &sum,
			Scale:             5,
			ZeroCount:         3,
			Min:               &min,
			Max:               &max,
			Positive: &otlpmetrics.ExponentialHistogramDataPoint_Buckets{
				Offset:       0,
				BucketCounts: []uint64{1, 3, 5, 10, 20, 30, 25, 15, 8, 4, 2},
			},
			Negative: &otlpmetrics.ExponentialHistogramDataPoint_Buckets{
				Offset:       -2,
				BucketCounts: []uint64{2, 4, 8, 15, 25, 20, 10, 5, 3, 1},
			},
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
								Data: &otlpmetrics.Metric_ExponentialHistogram{ExponentialHistogram: &otlpmetrics.ExponentialHistogram{
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

func buildCanonicalSummaryMetrics(nPoints int) *otlpmetrics.MetricsData {
	points := make([]*otlpmetrics.SummaryDataPoint, nPoints)
	for i := range points {
		points[i] = &otlpmetrics.SummaryDataPoint{
			StartTimeUnixNano: 1000000000,
			TimeUnixNano:      2000000000,
			Count:             500,
			Sum:               12345.6,
			QuantileValues: []*otlpmetrics.SummaryDataPoint_ValueAtQuantile{
				{Quantile: 0.5, Value: 25.0},
				{Quantile: 0.9, Value: 80.0},
				{Quantile: 0.95, Value: 95.0},
				{Quantile: 0.99, Value: 120.0},
			},
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
								Name: "http.request.duration.summary",
								Data: &otlpmetrics.Metric_Summary{Summary: &otlpmetrics.Summary{
									DataPoints: points,
								}},
							},
						},
					},
				},
			},
		},
	}
}

// buildCanonicalProfilesData uses vtproto types because official proto
// doesn't include a profiles package.
func buildCanonicalProfilesData(nSamples int) *vtprofiles.ProfilesData {
	functions := []*vtprofiles.Function{
		{}, // zero value at index 0
		{NameStrindex: 1, SystemNameStrindex: 2, FilenameStrindex: 3, StartLine: 42},
		{NameStrindex: 4, SystemNameStrindex: 5, FilenameStrindex: 6, StartLine: 100},
		{NameStrindex: 7, SystemNameStrindex: 8, FilenameStrindex: 9, StartLine: 55},
	}

	locations := []*vtprofiles.Location{
		{}, // zero value at index 0
		{MappingIndex: 1, Address: 0x7f0001000, Lines: []*vtprofiles.Line{{FunctionIndex: 1, Line: 42}}},
		{MappingIndex: 1, Address: 0x7f0002000, Lines: []*vtprofiles.Line{{FunctionIndex: 2, Line: 100}}},
		{MappingIndex: 1, Address: 0x7f0003000, Lines: []*vtprofiles.Line{{FunctionIndex: 3, Line: 55}}},
	}

	mappings := []*vtprofiles.Mapping{
		{}, // zero value at index 0
		{MemoryStart: 0x7f0000000, MemoryLimit: 0x7f0100000, FileOffset: 0, FilenameStrindex: 10},
	}

	stacks := []*vtprofiles.Stack{
		{}, // zero value at index 0
		{LocationIndices: []int32{1, 2, 3}},
		{LocationIndices: []int32{2, 3}},
		{LocationIndices: []int32{1, 3}},
	}

	samples := make([]*vtprofiles.Sample, nSamples)
	for i := range samples {
		samples[i] = &vtprofiles.Sample{
			StackIndex: int32(i%3 + 1),
			Values:     []int64{int64(100 + i*10)},
		}
	}

	profiles := []*vtprofiles.Profile{
		{
			SampleType:   &vtprofiles.ValueType{TypeStrindex: 11, UnitStrindex: 12},
			Samples:      samples,
			TimeUnixNano: 1000000000,
			DurationNano: 5000000000,
			PeriodType:   &vtprofiles.ValueType{TypeStrindex: 13, UnitStrindex: 14},
			Period:       10000000,
			ProfileId:    []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		},
	}

	return &vtprofiles.ProfilesData{
		ResourceProfiles: []*vtprofiles.ResourceProfiles{
			{
				Resource: &vtresource.Resource{
					Attributes: []*vtcommon.KeyValue{
						{Key: "service.name", Value: &vtcommon.AnyValue{Value: &vtcommon.AnyValue_StringValue{StringValue: "bench-service"}}},
					},
				},
				ScopeProfiles: []*vtprofiles.ScopeProfiles{
					{
						Scope:    &vtcommon.InstrumentationScope{Name: "bench-profiler", Version: "1.0"},
						Profiles: profiles,
					},
				},
			},
		},
		Dictionary: &vtprofiles.ProfilesDictionary{
			MappingTable:  mappings,
			LocationTable: locations,
			FunctionTable: functions,
			LinkTable:     []*vtprofiles.Link{{}},
			StringTable: []string{
				"",                   // 0: required empty
				"main",               // 1: function name
				"main",               // 2: system name
				"main.go",            // 3: filename
				"handleRequest",      // 4: function name
				"handleRequest",      // 5: system name
				"handler.go",         // 6: filename
				"processData",        // 7: function name
				"processData",        // 8: system name
				"process.go",         // 9: filename
				"/usr/local/bin/app", // 10: mapping filename
				"cpu",                // 11: sample type
				"nanoseconds",        // 12: sample unit
				"cpu",                // 13: period type
				"nanoseconds",        // 14: period unit
			},
			AttributeTable: []*vtprofiles.KeyValueAndUnit{{}},
			StackTable:     stacks,
		},
	}
}
