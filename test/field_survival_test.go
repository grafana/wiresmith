package test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	commonv1 "wiresmith/gen/otlp/common/v1"
	logsv1 "wiresmith/gen/otlp/logs/v1"
	metricsv1 "wiresmith/gen/otlp/metrics/v1"
	profilesv1 "wiresmith/gen/otlp/profiles/v1development"
	resourcev1 "wiresmith/gen/otlp/resource/v1"
	tracev1 "wiresmith/gen/otlp/trace/v1"

	otlpcommon "go.opentelemetry.io/proto/otlp/common/v1"
	otlplogs "go.opentelemetry.io/proto/otlp/logs/v1"
	otlpmetrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	otlpresource "go.opentelemetry.io/proto/otlp/resource/v1"
	otlptrace "go.opentelemetry.io/proto/otlp/trace/v1"
)

// withPresence returns a copy of v with all fieldsPresent bitmaps populated
// by doing a marshal→unmarshal roundtrip. This lets reflect.DeepEqual compare
// Go-constructed structs against wire-decoded structs.
func withPresence[T any, PT interface {
	*T
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
}](t *testing.T, v T) T {
	t.Helper()
	b, err := PT(&v).Marshal()
	require.NoError(t, err)
	require.NotEmpty(t, b, "withPresence: marshal produced empty bytes")

	var out T
	require.NoError(t, PT(&out).Unmarshal(b))

	// Verify the roundtrip is stable: re-marshal must produce identical bytes.
	b2, err := PT(&out).Marshal()
	require.NoError(t, err)
	require.Equal(t, b, b2, "withPresence: re-marshal produced different bytes")

	return out
}

// makeNestedAnyValue builds a deeply nested AnyValue to exercise
// kvlist, array, and scalar variants together.
func makeNestedAnyValue() commonv1.AnyValue {
	return commonv1.AnyValue{
		Value: &commonv1.AnyValue_KvlistValue{KvlistValue: commonv1.KeyValueList{
			Values: []commonv1.KeyValue{
				{Key: "inner.string", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "nested-val"}}},
				{Key: "inner.array", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_ArrayValue{ArrayValue: commonv1.ArrayValue{
					Values: []commonv1.AnyValue{
						{Value: &commonv1.AnyValue_IntValue{IntValue: 42}},
						{Value: &commonv1.AnyValue_BoolValue{BoolValue: true}},
						{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: 2.718}},
						{Value: &commonv1.AnyValue_BytesValue{BytesValue: []byte{0xca, 0xfe}}},
					},
				}}}},
				{Key: "inner.int", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: -99}}},
			},
		}},
	}
}

// makeFullResource returns a Resource with all fields populated.
func makeFullResource() resourcev1.Resource {
	return resourcev1.Resource{
		Attributes: []commonv1.KeyValue{
			{Key: "service.name", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "survival-svc"}}},
			{Key: "host.arch", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "amd64"}}},
		},
		DroppedAttributesCount: 7,
		EntityRefs: []commonv1.EntityRef{
			{
				SchemaUrl:       "https://example.com/entity-schema",
				Type:            "service",
				IdKeys:          []string{"service.name", "service.namespace"},
				DescriptionKeys: []string{"service.version"},
			},
		},
	}
}

// makeFullScope returns an InstrumentationScope with all fields populated.
func makeFullScope() commonv1.InstrumentationScope {
	return commonv1.InstrumentationScope{
		Name:    "survival-lib",
		Version: "3.7.1",
		Attributes: []commonv1.KeyValue{
			{Key: "scope.attr", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: true}}},
		},
		DroppedAttributesCount: 2,
	}
}

// TestFieldSurvival_TracesData verifies that every field in a maximally-populated
// TracesData survives a marshal->unmarshal round-trip using reflect.DeepEqual.
func TestFieldSurvival_TracesData(t *testing.T) {
	original := withPresence(t, tracev1.TracesData{
		ResourceSpans: []tracev1.ResourceSpans{
			{
				Resource: makeFullResource(),
				ScopeSpans: []tracev1.ScopeSpans{
					{
						Scope: makeFullScope(),
						Spans: []tracev1.Span{
							{
								TraceId:           []byte{0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0xF6, 0x07, 0x18, 0x29, 0x3A, 0x4B, 0x5C, 0x6D, 0x7E, 0x8F, 0x90},
								SpanId:            []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88},
								TraceState:        "vendor1=val1,vendor2=val2",
								ParentSpanId:      []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x01, 0x02},
								Flags:             0x00000301,
								Name:              "survival-span",
								Kind:              tracev1.SPAN_KIND_CLIENT,
								StartTimeUnixNano: 1700000000000000000,
								EndTimeUnixNano:   1700000001000000000,
								Attributes: []commonv1.KeyValue{
									{Key: "http.method", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "POST"}}},
									{Key: "http.status_code", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 201}}},
									{Key: "nested.val", Value: makeNestedAnyValue()},
								},
								DroppedAttributesCount: 3,
								Events: []tracev1.Span_Event{
									{
										TimeUnixNano: 1700000000500000000,
										Name:         "event-alpha",
										Attributes: []commonv1.KeyValue{
											{Key: "event.detail", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "detail-val"}}},
											{Key: "event.count", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 17}}},
										},
										DroppedAttributesCount: 1,
									},
									{
										TimeUnixNano: 1700000000600000000,
										Name:         "event-beta",
									},
								},
								DroppedEventsCount: 5,
								Links: []tracev1.Span_Link{
									{
										TraceId:    []byte{0xF0, 0xE1, 0xD2, 0xC3, 0xB4, 0xA5, 0x96, 0x87, 0x78, 0x69, 0x5A, 0x4B, 0x3C, 0x2D, 0x1E, 0x0F},
										SpanId:     []byte{0x99, 0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22},
										TraceState: "link-state=abc",
										Attributes: []commonv1.KeyValue{
											{Key: "link.attr", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: 1.618}}},
										},
										DroppedAttributesCount: 4,
										Flags:                  0x00000200,
									},
								},
								DroppedLinksCount: 9,
								Status: tracev1.Status{
									Code:    tracev1.STATUS_CODE_ERROR,
									Message: "something went wrong",
								},
							},
						},
						SchemaUrl: "https://example.com/scope-schema",
					},
				},
				SchemaUrl: "https://example.com/resource-schema",
			},
		},
	},
	)

	// Step 1: self round-trip
	oursBytes, err := original.Marshal()
	require.NoError(t, err)
	require.NotEmpty(t, oursBytes)

	var decoded tracev1.TracesData
	require.NoError(t, decoded.Unmarshal(oursBytes))

	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("TracesData self-roundtrip: decoded does not match original\n  original: %+v\n  decoded:  %+v", original, decoded)
	}

	// Step 2: determinism -- re-marshal must produce identical bytes
	reBytes, err := decoded.Marshal()
	require.NoError(t, err)
	assert.Equal(t, oursBytes, reBytes, "TracesData re-marshal produced different bytes (non-deterministic)")

	// Step 3: cross-library round-trip (wiresmith -> official -> wiresmith)
	var official otlptrace.TracesData
	require.NoError(t, proto.Unmarshal(oursBytes, &official))
	officialBytes, err := proto.Marshal(&official)
	require.NoError(t, err)

	var crossDecoded tracev1.TracesData
	require.NoError(t, crossDecoded.Unmarshal(officialBytes))

	if !reflect.DeepEqual(original, crossDecoded) {
		t.Fatalf("TracesData cross-library roundtrip: decoded does not match original\n  original: %+v\n  decoded:  %+v", original, crossDecoded)
	}
}

// TestFieldSurvival_MetricsData verifies all 5 metric types and every field
// survives a marshal->unmarshal round-trip.
func TestFieldSurvival_MetricsData(t *testing.T) {
	histSum := 456.789
	histMin := 0.5
	histMax := 123.4
	expHistSum := 789.012
	expHistMin := 0.001
	expHistMax := 500.5
	// Histogram zero-value optional: pointer to 0.0 differs from nil
	zeroVal := 0.0

	original := withPresence(t, metricsv1.MetricsData{
		ResourceMetrics: []metricsv1.ResourceMetrics{
			{
				Resource: makeFullResource(),
				ScopeMetrics: []metricsv1.ScopeMetrics{
					{
						Scope: makeFullScope(),
						Metrics: []metricsv1.Metric{
							// Gauge with double value
							{
								Name:        "survival.gauge",
								Description: "a gauge for survival testing",
								Unit:        "ms",
								Data: &metricsv1.Metric_Gauge{Gauge: metricsv1.Gauge{
									DataPoints: []metricsv1.NumberDataPoint{
										{
											Attributes: []commonv1.KeyValue{
												{Key: "gauge.label", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "g-val"}}},
											},
											StartTimeUnixNano: 1700000000000000000,
											TimeUnixNano:      1700000001000000000,
											Value:             &metricsv1.NumberDataPoint_AsDouble{AsDouble: 77.7},
											Exemplars: []metricsv1.Exemplar{
												{
													FilteredAttributes: []commonv1.KeyValue{
														{Key: "ex.filter", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "fv"}}},
													},
													TimeUnixNano: 1700000000500000000,
													Value:        &metricsv1.Exemplar_AsDouble{AsDouble: 76.6},
													SpanId:       []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
													TraceId:      []byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80, 0x90, 0xA0, 0xB0, 0xC0, 0xD0, 0xE0, 0xF0, 0x01},
												},
											},
											Flags: 1,
										},
									},
								}},
								Metadata: []commonv1.KeyValue{
									{Key: "meta.gauge", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 101}}},
								},
							},
							// Sum with int value
							{
								Name:        "survival.sum",
								Description: "a monotonic cumulative sum",
								Unit:        "By",
								Data: &metricsv1.Metric_Sum{Sum: metricsv1.Sum{
									AggregationTemporality: metricsv1.AGGREGATION_TEMPORALITY_CUMULATIVE,
									IsMonotonic:            true,
									DataPoints: []metricsv1.NumberDataPoint{
										{
											Attributes: []commonv1.KeyValue{
												{Key: "sum.label", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 42}}},
											},
											StartTimeUnixNano: 1700000000000000000,
											TimeUnixNano:      1700000002000000000,
											Value:             &metricsv1.NumberDataPoint_AsInt{AsInt: 9999},
											Exemplars: []metricsv1.Exemplar{
												{
													TimeUnixNano: 1700000001500000000,
													Value:        &metricsv1.Exemplar_AsInt{AsInt: 9998},
													TraceId:      []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0x00},
												},
											},
											Flags: 0,
										},
									},
								}},
								Metadata: []commonv1.KeyValue{
									{Key: "meta.sum", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "sum-meta"}}},
								},
							},
							// Histogram
							{
								Name:        "survival.histogram",
								Description: "a histogram with all fields",
								Unit:        "us",
								Data: &metricsv1.Metric_Histogram{Histogram: metricsv1.Histogram{
									AggregationTemporality: metricsv1.AGGREGATION_TEMPORALITY_DELTA,
									DataPoints: []metricsv1.HistogramDataPoint{
										{
											Attributes: []commonv1.KeyValue{
												{Key: "hist.label", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "hv"}}},
											},
											StartTimeUnixNano: 1700000000000000000,
											TimeUnixNano:      1700000003000000000,
											Count:             250,
											Sum:               &histSum,
											BucketCounts:      []uint64{10, 50, 80, 60, 30, 20},
											ExplicitBounds:    []float64{1.0, 5.0, 10.0, 50.0, 100.0},
											Exemplars: []metricsv1.Exemplar{
												{
													TimeUnixNano: 1700000002500000000,
													Value:        &metricsv1.Exemplar_AsDouble{AsDouble: 7.77},
												},
											},
											Flags: 1,
											Min:   &histMin,
											Max:   &histMax,
										},
										// A second data point with zero-value optional (pointer to 0.0)
										{
											TimeUnixNano: 1700000004000000000,
											Count:        0,
											Sum:          &zeroVal,
											Min:          &zeroVal,
											Max:          &zeroVal,
										},
									},
								}},
							},
							// ExponentialHistogram
							{
								Name:        "survival.exp_histogram",
								Description: "exponential histogram with all fields",
								Unit:        "ns",
								Data: &metricsv1.Metric_ExponentialHistogram{ExponentialHistogram: metricsv1.ExponentialHistogram{
									AggregationTemporality: metricsv1.AGGREGATION_TEMPORALITY_CUMULATIVE,
									DataPoints: []metricsv1.ExponentialHistogramDataPoint{
										{
											Attributes: []commonv1.KeyValue{
												{Key: "exp.region", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "us-west"}}},
											},
											StartTimeUnixNano: 1700000000000000000,
											TimeUnixNano:      1700000005000000000,
											Count:             5000,
											Sum:               &expHistSum,
											Scale:             7,
											ZeroCount:         42,
											Positive: metricsv1.ExponentialHistogramDataPoint_Buckets{
												Offset:       3,
												BucketCounts: []uint64{100, 200, 300, 400, 500},
											},
											Negative: metricsv1.ExponentialHistogramDataPoint_Buckets{
												Offset:       -5,
												BucketCounts: []uint64{50, 100, 150},
											},
											Flags: 1,
											Exemplars: []metricsv1.Exemplar{
												{
													TimeUnixNano: 1700000004500000000,
													Value:        &metricsv1.Exemplar_AsDouble{AsDouble: 33.33},
													SpanId:       []byte{0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0xF6, 0x07, 0x18},
													TraceId:      []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10},
												},
											},
											Min:           &expHistMin,
											Max:           &expHistMax,
											ZeroThreshold: 1e-8,
										},
									},
								}},
							},
							// Summary
							{
								Name:        "survival.summary",
								Description: "summary with quantiles",
								Unit:        "s",
								Data: &metricsv1.Metric_Summary{Summary: metricsv1.Summary{
									DataPoints: []metricsv1.SummaryDataPoint{
										{
											Attributes: []commonv1.KeyValue{
												{Key: "summary.ep", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "/api/v1"}}},
											},
											StartTimeUnixNano: 1700000000000000000,
											TimeUnixNano:      1700000006000000000,
											Count:             10000,
											Sum:               55555.55,
											QuantileValues: []metricsv1.SummaryDataPoint_ValueAtQuantile{
												{Quantile: 0.0, Value: 0.001},
												{Quantile: 0.5, Value: 5.5},
												{Quantile: 0.9, Value: 9.9},
												{Quantile: 0.95, Value: 14.5},
												{Quantile: 0.99, Value: 29.9},
												{Quantile: 1.0, Value: 120.0},
											},
											Flags: 1,
										},
									},
								}},
							},
						},
						SchemaUrl: "https://example.com/scope-metrics-schema",
					},
				},
				SchemaUrl: "https://example.com/resource-metrics-schema",
			},
		},
	},
	)

	// Step 1: self round-trip
	oursBytes, err := original.Marshal()
	require.NoError(t, err)
	require.NotEmpty(t, oursBytes)

	var decoded metricsv1.MetricsData
	require.NoError(t, decoded.Unmarshal(oursBytes))

	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("MetricsData self-roundtrip: decoded does not match original\n  original: %+v\n  decoded:  %+v", original, decoded)
	}

	// Step 2: determinism
	reBytes, err := decoded.Marshal()
	require.NoError(t, err)
	assert.Equal(t, oursBytes, reBytes, "MetricsData re-marshal produced different bytes (non-deterministic)")

	// Step 3: cross-library round-trip (wiresmith -> official -> wiresmith)
	var official otlpmetrics.MetricsData
	require.NoError(t, proto.Unmarshal(oursBytes, &official))
	officialBytes, err := proto.Marshal(&official)
	require.NoError(t, err)

	var crossDecoded metricsv1.MetricsData
	require.NoError(t, crossDecoded.Unmarshal(officialBytes))

	if !reflect.DeepEqual(original, crossDecoded) {
		t.Fatalf("MetricsData cross-library roundtrip: decoded does not match original\n  original: %+v\n  decoded:  %+v", original, crossDecoded)
	}
}

// TestFieldSurvival_LogsData verifies all LogRecord fields survive round-trip.
func TestFieldSurvival_LogsData(t *testing.T) {
	original := withPresence(t, logsv1.LogsData{
		ResourceLogs: []logsv1.ResourceLogs{
			{
				Resource: makeFullResource(),
				ScopeLogs: []logsv1.ScopeLogs{
					{
						Scope: makeFullScope(),
						LogRecords: []logsv1.LogRecord{
							{
								TimeUnixNano:         1700000000000000000,
								ObservedTimeUnixNano: 1700000000000100000,
								SeverityNumber:       logsv1.SEVERITY_NUMBER_ERROR,
								SeverityText:         "ERROR",
								Body:                 makeNestedAnyValue(),
								Attributes: []commonv1.KeyValue{
									{Key: "log.source", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "app.main"}}},
									{Key: "log.line", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 142}}},
								},
								DroppedAttributesCount: 6,
								Flags:                  0x00000101,
								TraceId:                []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF},
								SpanId:                 []byte{0xFE, 0xDC, 0xBA, 0x98, 0x76, 0x54, 0x32, 0x10},
								EventName:              "exception.thrown",
							},
						},
						SchemaUrl: "https://example.com/scope-logs-schema",
					},
				},
				SchemaUrl: "https://example.com/resource-logs-schema",
			},
		},
	},
	)

	// Step 1: self round-trip
	oursBytes, err := original.Marshal()
	require.NoError(t, err)
	require.NotEmpty(t, oursBytes)

	var decoded logsv1.LogsData
	require.NoError(t, decoded.Unmarshal(oursBytes))

	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("LogsData self-roundtrip: decoded does not match original\n  original: %+v\n  decoded:  %+v", original, decoded)
	}

	// Step 2: determinism
	reBytes, err := decoded.Marshal()
	require.NoError(t, err)
	assert.Equal(t, oursBytes, reBytes, "LogsData re-marshal produced different bytes (non-deterministic)")

	// Step 3: cross-library round-trip (wiresmith -> official -> wiresmith)
	var official otlplogs.LogsData
	require.NoError(t, proto.Unmarshal(oursBytes, &official))
	officialBytes, err := proto.Marshal(&official)
	require.NoError(t, err)

	var crossDecoded logsv1.LogsData
	require.NoError(t, crossDecoded.Unmarshal(officialBytes))

	if !reflect.DeepEqual(original, crossDecoded) {
		t.Fatalf("LogsData cross-library roundtrip: decoded does not match original\n  original: %+v\n  decoded:  %+v", original, crossDecoded)
	}
}

// TestFieldSurvival_ProfilesData verifies all Profile dictionary tables and
// nested fields survive a self-roundtrip (no official Go proto for profiles).
func TestFieldSurvival_ProfilesData(t *testing.T) {
	original := withPresence(t, profilesv1.ProfilesData{
		ResourceProfiles: []profilesv1.ResourceProfiles{
			{
				Resource: makeFullResource(),
				ScopeProfiles: []profilesv1.ScopeProfiles{
					{
						Scope: makeFullScope(),
						Profiles: []profilesv1.Profile{
							{
								SampleType: profilesv1.ValueType{TypeStrindex: 1, UnitStrindex: 2},
								Samples: []profilesv1.Sample{
									{
										StackIndex:         0,
										AttributeIndices:   []int32{0, 1},
										LinkIndex:          0,
										Values:             []int64{100, 200, 300},
										TimestampsUnixNano: []uint64{1700000000000000000, 1700000000500000000, 1700000001000000000},
									},
									{
										StackIndex:         1,
										AttributeIndices:   []int32{2},
										LinkIndex:          1,
										Values:             []int64{-50},
										TimestampsUnixNano: []uint64{1700000002000000000},
									},
								},
								TimeUnixNano:           1700000000000000000,
								DurationNano:           2000000000,
								PeriodType:             profilesv1.ValueType{TypeStrindex: 3, UnitStrindex: 4},
								Period:                 10000,
								ProfileId:              []byte{0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0xF6, 0x07, 0x18, 0x29, 0x3A, 0x4B, 0x5C, 0x6D, 0x7E, 0x8F, 0x90},
								DroppedAttributesCount: 3,
								OriginalPayloadFormat:  "pprof",
								OriginalPayload:        []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02},
								AttributeIndices:       []int32{0, 1, 2},
							},
						},
						SchemaUrl: "https://example.com/scope-profiles-schema",
					},
				},
				SchemaUrl: "https://example.com/resource-profiles-schema",
			},
		},
		Dictionary: profilesv1.ProfilesDictionary{
			StringTable: []string{"", "cpu", "nanoseconds", "wall", "seconds", "main", "runtime.goexit"},
			MappingTable: []profilesv1.Mapping{
				{
					MemoryStart:      0x400000,
					MemoryLimit:      0x500000,
					FileOffset:       0x1000,
					FilenameStrindex: 5,
					AttributeIndices: []int32{0},
				},
				{
					MemoryStart:      0x7f0000,
					MemoryLimit:      0x7f1000,
					FileOffset:       0,
					FilenameStrindex: 6,
					AttributeIndices: []int32{1, 2},
				},
			},
			LocationTable: []profilesv1.Location{
				{
					MappingIndex: 0,
					Address:      0x401234,
					Lines: []profilesv1.Line{
						{FunctionIndex: 0, Line: 42, Column: 10},
						{FunctionIndex: 1, Line: 100, Column: 5},
					},
					AttributeIndices: []int32{0},
				},
				{
					MappingIndex:     1,
					Address:          0x7f0500,
					Lines:            []profilesv1.Line{{FunctionIndex: 2, Line: 7, Column: 1}},
					AttributeIndices: []int32{1},
				},
			},
			FunctionTable: []profilesv1.Function{
				{NameStrindex: 1, SystemNameStrindex: 2, FilenameStrindex: 5, StartLine: 10},
				{NameStrindex: 3, SystemNameStrindex: 4, FilenameStrindex: 6, StartLine: 95},
				{NameStrindex: 5, SystemNameStrindex: 6, FilenameStrindex: 1, StartLine: 1},
			},
			LinkTable: []profilesv1.Link{
				{
					TraceId: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10},
					SpanId:  []byte{0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0xF6, 0x07, 0x18},
				},
				{
					TraceId: []byte{0xF0, 0xE0, 0xD0, 0xC0, 0xB0, 0xA0, 0x90, 0x80, 0x70, 0x60, 0x50, 0x40, 0x30, 0x20, 0x10, 0x00},
					SpanId:  []byte{0xFF, 0xEE, 0xDD, 0xCC, 0xBB, 0xAA, 0x99, 0x88},
				},
			},
			AttributeTable: []profilesv1.KeyValueAndUnit{
				{KeyStrindex: 1, Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 42}}, UnitStrindex: 2},
				{KeyStrindex: 3, Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "attr-val"}}, UnitStrindex: 4},
				{KeyStrindex: 5, Value: commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: true}}, UnitStrindex: 0},
			},
			StackTable: []profilesv1.Stack{
				{LocationIndices: []int32{0}},
				{LocationIndices: []int32{0, 1}},
			},
		},
	},
	)

	// Step 1: self round-trip
	oursBytes, err := original.Marshal()
	require.NoError(t, err)
	require.NotEmpty(t, oursBytes)

	var decoded profilesv1.ProfilesData
	require.NoError(t, decoded.Unmarshal(oursBytes))

	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("ProfilesData self-roundtrip: decoded does not match original\n  original: %+v\n  decoded:  %+v", original, decoded)
	}

	// Step 2: determinism
	reBytes, err := decoded.Marshal()
	require.NoError(t, err)
	assert.Equal(t, oursBytes, reBytes, "ProfilesData re-marshal produced different bytes (non-deterministic)")
}

// TestFieldSurvival_CrossLibrary_Traces verifies the full cross-library chain:
// wiresmith marshal -> official unmarshal -> official marshal -> wiresmith unmarshal
// and compares the final result to the original using reflect.DeepEqual.
func TestFieldSurvival_CrossLibrary_Traces(t *testing.T) {
	original := withPresence(t, buildFullTracesData())

	// wiresmith -> bytes
	oursBytes, err := original.Marshal()
	require.NoError(t, err)

	// bytes -> official
	var step1 otlptrace.TracesData
	require.NoError(t, proto.Unmarshal(oursBytes, &step1))

	// official -> bytes
	officialBytes, err := proto.Marshal(&step1)
	require.NoError(t, err)

	// bytes -> official (2nd official round)
	var step2 otlptrace.TracesData
	require.NoError(t, proto.Unmarshal(officialBytes, &step2))

	// official -> bytes (2nd)
	officialBytes2, err := proto.Marshal(&step2)
	require.NoError(t, err)

	// bytes -> wiresmith
	var final tracev1.TracesData
	require.NoError(t, final.Unmarshal(officialBytes2))

	if !reflect.DeepEqual(original, final) {
		t.Fatalf("Traces cross-library chain: final does not match original\n  original: %+v\n  final:    %+v", original, final)
	}
}

// TestFieldSurvival_CrossLibrary_Metrics verifies the full cross-library chain for metrics.
func TestFieldSurvival_CrossLibrary_Metrics(t *testing.T) {
	sum := 456.789
	min := 0.5
	max := 123.4
	original := withPresence(t, metricsv1.MetricsData{
		ResourceMetrics: []metricsv1.ResourceMetrics{
			{
				Resource: resourcev1.Resource{
					Attributes: []commonv1.KeyValue{
						{Key: "svc", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "cross-metrics"}}},
					},
					DroppedAttributesCount: 1,
				},
				ScopeMetrics: []metricsv1.ScopeMetrics{
					{
						Scope: commonv1.InstrumentationScope{Name: "cross-lib", Version: "1.0"},
						Metrics: []metricsv1.Metric{
							{
								Name: "cross.gauge",
								Data: &metricsv1.Metric_Gauge{Gauge: metricsv1.Gauge{
									DataPoints: []metricsv1.NumberDataPoint{
										{TimeUnixNano: 1000, Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: 11.11}},
									},
								}},
							},
							{
								Name: "cross.histogram",
								Data: &metricsv1.Metric_Histogram{Histogram: metricsv1.Histogram{
									AggregationTemporality: metricsv1.AGGREGATION_TEMPORALITY_DELTA,
									DataPoints: []metricsv1.HistogramDataPoint{
										{
											TimeUnixNano:   2000,
											Count:          100,
											Sum:            &sum,
											BucketCounts:   []uint64{10, 20, 30, 40},
											ExplicitBounds: []float64{1.0, 10.0, 100.0},
											Min:            &min,
											Max:            &max,
										},
									},
								}},
							},
						},
						SchemaUrl: "https://schema.example.com",
					},
				},
				SchemaUrl: "https://res-schema.example.com",
			},
		},
	},
	)

	oursBytes, err := original.Marshal()
	require.NoError(t, err)

	var step1 otlpmetrics.MetricsData
	require.NoError(t, proto.Unmarshal(oursBytes, &step1))
	officialBytes, err := proto.Marshal(&step1)
	require.NoError(t, err)

	var final metricsv1.MetricsData
	require.NoError(t, final.Unmarshal(officialBytes))

	if !reflect.DeepEqual(original, final) {
		t.Fatalf("Metrics cross-library chain: final does not match original\n  original: %+v\n  final:    %+v", original, final)
	}
}

// TestFieldSurvival_CrossLibrary_Logs verifies the full cross-library chain for logs.
func TestFieldSurvival_CrossLibrary_Logs(t *testing.T) {
	original := withPresence(t, logsv1.LogsData{
		ResourceLogs: []logsv1.ResourceLogs{
			{
				Resource: resourcev1.Resource{
					Attributes: []commonv1.KeyValue{
						{Key: "svc", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "cross-logs"}}},
					},
					DroppedAttributesCount: 2,
				},
				ScopeLogs: []logsv1.ScopeLogs{
					{
						Scope: commonv1.InstrumentationScope{Name: "cross-log-lib", Version: "2.0"},
						LogRecords: []logsv1.LogRecord{
							{
								TimeUnixNano:         3000000000,
								ObservedTimeUnixNano: 3000000001,
								SeverityNumber:       logsv1.SEVERITY_NUMBER_WARN,
								SeverityText:         "WARN",
								Body:                 commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "cross-body"}},
								Attributes: []commonv1.KeyValue{
									{Key: "cl.attr", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 77}}},
								},
								DroppedAttributesCount: 3,
								Flags:                  0xFF,
								TraceId:                []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10},
								SpanId:                 []byte{0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0xF6, 0x07, 0x18},
								EventName:              "cross.event",
							},
						},
						SchemaUrl: "https://cross-log-schema.example.com",
					},
				},
				SchemaUrl: "https://cross-res-schema.example.com",
			},
		},
	},
	)

	oursBytes, err := original.Marshal()
	require.NoError(t, err)

	var step1 otlplogs.LogsData
	require.NoError(t, proto.Unmarshal(oursBytes, &step1))
	officialBytes, err := proto.Marshal(&step1)
	require.NoError(t, err)

	var final logsv1.LogsData
	require.NoError(t, final.Unmarshal(officialBytes))

	if !reflect.DeepEqual(original, final) {
		t.Fatalf("Logs cross-library chain: final does not match original\n  original: %+v\n  final:    %+v", original, final)
	}
}

// TestFieldSurvival_OptionalZeroPointers verifies that optional fields pointing
// to zero (e.g., *float64 = 0.0) survive round-trip and are not confused with nil.
func TestFieldSurvival_OptionalZeroPointers(t *testing.T) {
	zero := 0.0
	original := withPresence(t, metricsv1.HistogramDataPoint{
		TimeUnixNano: 1,
		Count:        0,
		Sum:          &zero,
		Min:          &zero,
		Max:          &zero,
	})

	oursBytes, err := original.Marshal()
	require.NoError(t, err)

	var decoded metricsv1.HistogramDataPoint
	require.NoError(t, decoded.Unmarshal(oursBytes))

	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("HistogramDataPoint optional zero pointers: decoded does not match original\n  original: %+v\n  decoded:  %+v", original, decoded)
	}

	// Cross-library verification
	var official otlpmetrics.HistogramDataPoint
	require.NoError(t, proto.Unmarshal(oursBytes, &official))
	require.NotNil(t, official.Sum, "optional Sum=0.0 must survive as non-nil")
	require.NotNil(t, official.Min, "optional Min=0.0 must survive as non-nil")
	require.NotNil(t, official.Max, "optional Max=0.0 must survive as non-nil")
	assert.Equal(t, 0.0, *official.Sum)
	assert.Equal(t, 0.0, *official.Min)
	assert.Equal(t, 0.0, *official.Max)
}

// buildFullTracesData returns a maximally-populated TracesData for reuse across tests.
func buildFullTracesData() tracev1.TracesData {
	return tracev1.TracesData{
		ResourceSpans: []tracev1.ResourceSpans{
			{
				Resource: makeFullResource(),
				ScopeSpans: []tracev1.ScopeSpans{
					{
						Scope: makeFullScope(),
						Spans: []tracev1.Span{
							{
								TraceId:           []byte{0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0xF6, 0x07, 0x18, 0x29, 0x3A, 0x4B, 0x5C, 0x6D, 0x7E, 0x8F, 0x90},
								SpanId:            []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88},
								TraceState:        "vendor1=val1,vendor2=val2",
								ParentSpanId:      []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x01, 0x02},
								Flags:             0x00000301,
								Name:              "cross-lib-span",
								Kind:              tracev1.SPAN_KIND_PRODUCER,
								StartTimeUnixNano: 1700000000000000000,
								EndTimeUnixNano:   1700000001000000000,
								Attributes: []commonv1.KeyValue{
									{Key: "rpc.system", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "grpc"}}},
								},
								DroppedAttributesCount: 2,
								Events: []tracev1.Span_Event{
									{
										TimeUnixNano: 1700000000500000000,
										Name:         "message",
										Attributes: []commonv1.KeyValue{
											{Key: "message.type", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "SENT"}}},
										},
										DroppedAttributesCount: 1,
									},
								},
								DroppedEventsCount: 3,
								Links: []tracev1.Span_Link{
									{
										TraceId:                []byte{0xF0, 0xE1, 0xD2, 0xC3, 0xB4, 0xA5, 0x96, 0x87, 0x78, 0x69, 0x5A, 0x4B, 0x3C, 0x2D, 0x1E, 0x0F},
										SpanId:                 []byte{0x99, 0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22},
										TraceState:             "link-ts=xyz",
										DroppedAttributesCount: 7,
										Flags:                  0x00000100,
									},
								},
								DroppedLinksCount: 4,
								Status: tracev1.Status{
									Code:    tracev1.STATUS_CODE_OK,
									Message: "all good",
								},
							},
						},
						SchemaUrl: "https://example.com/scope-schema",
					},
				},
				SchemaUrl: "https://example.com/resource-schema",
			},
		},
	}
}

// Ensure unused imports are used (cross-library tests reference these)
var _ = (*otlpcommon.AnyValue)(nil)
var _ = (*otlpresource.Resource)(nil)
