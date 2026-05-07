package fuzz

import (
	"bytes"
	"encoding/binary"
	"math"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"

	commonv1 "wiresmith/gen/otlp/common/v1"
	logsv1 "wiresmith/gen/otlp/logs/v1"
	metricsv1 "wiresmith/gen/otlp/metrics/v1"
	resourcev1 "wiresmith/gen/otlp/resource/v1"
	tracev1 "wiresmith/gen/otlp/trace/v1"
	"wiresmith/test/testutil"

	otlpcommon "go.opentelemetry.io/proto/otlp/common/v1"
	otlplogs "go.opentelemetry.io/proto/otlp/logs/v1"
	otlpmetrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	otlpresource "go.opentelemetry.io/proto/otlp/resource/v1"
	otlptrace "go.opentelemetry.io/proto/otlp/trace/v1"
)

// FuzzUnmarshal feeds random bytes into every generated Unmarshal
// method. The test passes as long as no method panics — errors are expected.
func FuzzUnmarshal(f *testing.F) {
	// Seed corpus with interesting byte patterns.
	f.Add([]byte{})                                                     // empty
	f.Add([]byte{0x08, 0x01})                                           // valid varint field
	f.Add([]byte{0x0a, 0x00})                                           // valid empty bytes field
	f.Add([]byte{0x80})                                                 // truncated tag
	f.Add([]byte{0x08, 0x80})                                           // truncated varint
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})                   // overflow varint
	f.Add([]byte{0x0a, 0xff, 0xff, 0xff, 0xff, 0x0f})                   // huge length prefix
	f.Add([]byte{0x0d, 0x01, 0x02, 0x03, 0x04})                         // fixed32 field
	f.Add([]byte{0x09, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}) // fixed64 field

	ctors := testutil.AllPanicSafeConstructors()

	f.Fuzz(func(t *testing.T, data []byte) {
		for name, newMsg := range ctors {
			msg := newMsg()
			// We only care that it doesn't panic; errors are fine.
			_ = msg.Unmarshal(data)
			_ = name
		}
	})
}

// FuzzRoundTrip verifies that Unmarshal→Marshal→Unmarshal produces identical
// structs. Seeds the corpus with marshaled bytes from deeply-nested valid
// messages so the fuzzer starts from realistic data and mutates it.
func FuzzRoundTrip(f *testing.F) {
	for _, seed := range marshaledSeeds() {
		f.Add(seed)
	}

	ctors := testutil.AllMessageConstructors()

	f.Fuzz(func(t *testing.T, data []byte) {
		for name, newMsg := range ctors {
			msg1 := newMsg()
			if err := msg1.Unmarshal(data); err != nil {
				continue
			}

			bytes1, err := msg1.Marshal()
			if err != nil {
				t.Fatalf("%s: Marshal after Unmarshal failed: %v", name, err)
			}

			msg2 := newMsg()
			if err := msg2.Unmarshal(bytes1); err != nil {
				t.Fatalf("%s: second Unmarshal failed: %v", name, err)
			}

			bytes2, err := msg2.Marshal()
			if err != nil {
				t.Fatalf("%s: re-Marshal failed: %v", name, err)
			}

			// Compare marshaled bytes rather than structs to avoid
			// nil-vs-empty-slice false positives (semantically equal in proto3).
			if !bytes.Equal(bytes1, bytes2) {
				t.Fatalf("%s: round-trip bytes mismatch: %d vs %d bytes", name, len(bytes1), len(bytes2))
			}
		}
	})
}

// FuzzMarshalSize verifies that Size() matches the actual Marshal() output
// length. A mismatch indicates a bug in size computation for some field
// combination — especially relevant for nested messages with length prefixes.
// Map iteration order does not affect the marshal length, so map-bearing
// types are safe to include here.
func FuzzMarshalSize(f *testing.F) {
	for _, seed := range marshaledSeeds() {
		f.Add(seed)
	}

	ctors := testutil.AllPanicSafeConstructors()

	f.Fuzz(func(t *testing.T, data []byte) {
		for name, newMsg := range ctors {
			msg := newMsg()
			if err := msg.Unmarshal(data); err != nil {
				continue
			}

			size := msg.Size()
			bytes, err := msg.Marshal()
			if err != nil {
				t.Fatalf("%s: Marshal failed: %v", name, err)
			}

			if size != len(bytes) {
				t.Fatalf("%s: Size()=%d but Marshal() produced %d bytes", name, size, len(bytes))
			}
		}
	})
}

// FuzzCrossLibrary compares our Unmarshal against the official protobuf library
// for top-level message types. Catches cases where we parse fields differently.
func FuzzCrossLibrary(f *testing.F) {
	for _, seed := range marshaledSeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// TracesData
		crossCheck(t, data, "TracesData",
			func() testutil.Message { return new(tracev1.TracesData) },
			func(b []byte) (int, error) {
				var m otlptrace.TracesData
				if err := proto.Unmarshal(b, &m); err != nil {
					return 0, err
				}
				return countTraceSpans(&m), nil
			},
			func(msg testutil.Message) int {
				m := msg.(*tracev1.TracesData)
				n := 0
				for i := range m.ResourceSpans {
					for j := range m.ResourceSpans[i].ScopeSpans {
						n += len(m.ResourceSpans[i].ScopeSpans[j].Spans)
					}
				}
				return n
			},
		)

		// LogsData
		crossCheck(t, data, "LogsData",
			func() testutil.Message { return new(logsv1.LogsData) },
			func(b []byte) (int, error) {
				var m otlplogs.LogsData
				if err := proto.Unmarshal(b, &m); err != nil {
					return 0, err
				}
				return countLogRecords(&m), nil
			},
			func(msg testutil.Message) int {
				m := msg.(*logsv1.LogsData)
				n := 0
				for i := range m.ResourceLogs {
					for j := range m.ResourceLogs[i].ScopeLogs {
						n += len(m.ResourceLogs[i].ScopeLogs[j].LogRecords)
					}
				}
				return n
			},
		)

		// MetricsData
		crossCheck(t, data, "MetricsData",
			func() testutil.Message { return new(metricsv1.MetricsData) },
			func(b []byte) (int, error) {
				var m otlpmetrics.MetricsData
				if err := proto.Unmarshal(b, &m); err != nil {
					return 0, err
				}
				return countMetrics(&m), nil
			},
			func(msg testutil.Message) int {
				m := msg.(*metricsv1.MetricsData)
				n := 0
				for i := range m.ResourceMetrics {
					for j := range m.ResourceMetrics[i].ScopeMetrics {
						n += len(m.ResourceMetrics[i].ScopeMetrics[j].Metrics)
					}
				}
				return n
			},
		)

		// Resource (leaf type — compare attribute count)
		crossCheck(t, data, "Resource",
			func() testutil.Message { return new(resourcev1.Resource) },
			func(b []byte) (int, error) {
				var m otlpresource.Resource
				if err := proto.Unmarshal(b, &m); err != nil {
					return 0, err
				}
				return len(m.Attributes), nil
			},
			func(msg testutil.Message) int {
				return len(msg.(*resourcev1.Resource).Attributes)
			},
		)

		// AnyValue (compare which oneof variant is set)
		crossCheck(t, data, "AnyValue",
			func() testutil.Message { return new(commonv1.AnyValue) },
			func(b []byte) (int, error) {
				var m otlpcommon.AnyValue
				if err := proto.Unmarshal(b, &m); err != nil {
					return 0, err
				}
				return anyValueTag(&m), nil
			},
			func(msg testutil.Message) int {
				return ourAnyValueTag(msg.(*commonv1.AnyValue))
			},
		)
	})
}

// crossCheck unmarshals data with both our library and the official one, then
// compares a structural metric (e.g. element count). If the official library
// rejects the input, we skip — we are lenient by design. But if both accept,
// the structural metric must match.
func crossCheck(
	t *testing.T,
	data []byte,
	name string,
	newOurs func() testutil.Message,
	officialMetric func([]byte) (int, error),
	ourMetric func(testutil.Message) int,
) {
	t.Helper()

	ours := newOurs()
	if err := ours.Unmarshal(data); err != nil {
		return
	}

	officialCount, err := officialMetric(data)
	if err != nil {
		// Official rejects it — that's fine, we're more lenient.
		return
	}

	ourCount := ourMetric(ours)
	if officialCount != ourCount {
		t.Fatalf("%s: structural mismatch: official=%d ours=%d", name, officialCount, ourCount)
	}
}

func countTraceSpans(m *otlptrace.TracesData) int {
	n := 0
	for _, rs := range m.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			n += len(ss.Spans)
		}
	}
	return n
}

func countLogRecords(m *otlplogs.LogsData) int {
	n := 0
	for _, rl := range m.ResourceLogs {
		for _, sl := range rl.ScopeLogs {
			n += len(sl.LogRecords)
		}
	}
	return n
}

func countMetrics(m *otlpmetrics.MetricsData) int {
	n := 0
	for _, rm := range m.ResourceMetrics {
		for _, sm := range rm.ScopeMetrics {
			n += len(sm.Metrics)
		}
	}
	return n
}

func anyValueTag(m *otlpcommon.AnyValue) int {
	switch m.Value.(type) {
	case *otlpcommon.AnyValue_StringValue:
		return 1
	case *otlpcommon.AnyValue_BoolValue:
		return 2
	case *otlpcommon.AnyValue_IntValue:
		return 3
	case *otlpcommon.AnyValue_DoubleValue:
		return 4
	case *otlpcommon.AnyValue_ArrayValue:
		return 5
	case *otlpcommon.AnyValue_KvlistValue:
		return 6
	case *otlpcommon.AnyValue_BytesValue:
		return 7
	default:
		return 0
	}
}

func ourAnyValueTag(m *commonv1.AnyValue) int {
	switch m.Value.(type) {
	case *commonv1.AnyValue_StringValue:
		return 1
	case *commonv1.AnyValue_BoolValue:
		return 2
	case *commonv1.AnyValue_IntValue:
		return 3
	case *commonv1.AnyValue_DoubleValue:
		return 4
	case *commonv1.AnyValue_ArrayValue:
		return 5
	case *commonv1.AnyValue_KvlistValue:
		return 6
	case *commonv1.AnyValue_BytesValue:
		return 7
	default:
		return 0
	}
}

// FuzzStructuredTrace builds valid TracesData from fuzz parameters, ensuring
// deeply nested structures are always exercised. The fuzzer mutates the
// parameters (not bytes), so every input produces a structurally valid message.
func FuzzStructuredTrace(f *testing.F) {
	f.Add(int64(0), uint64(100), uint64(200), uint8(1), uint8(1), uint8(1), true)
	f.Add(int64(42), uint64(math.MaxUint64), uint64(0), uint8(3), uint8(2), uint8(5), false)
	f.Add(int64(-1), uint64(1000000000), uint64(2000000000), uint8(0), uint8(0), uint8(0), true)

	f.Fuzz(func(t *testing.T, seed int64, startTime uint64, endTime uint64, nSpans uint8, nEvents uint8, nLinks uint8, withAttrs bool) {
		// Cap to avoid excessive allocations
		spanCount := int(nSpans%8) + 1
		eventCount := int(nEvents % 8)
		linkCount := int(nLinks % 8)

		spans := make([]tracev1.Span, spanCount)
		for i := range spans {
			spans[i] = tracev1.Span{
				TraceId:           traceID(seed, i),
				SpanId:            spanID(seed, i),
				ParentSpanId:      spanID(seed, i+100),
				Name:              "span",
				Kind:              tracev1.Span_SpanKind(i % 6),
				StartTimeUnixNano: startTime,
				EndTimeUnixNano:   endTime,
				Status:            tracev1.Status{Code: tracev1.Status_StatusCode(i % 3)},
			}

			if withAttrs {
				spans[i].Attributes = []commonv1.KeyValue{
					strAttr("key", "val"),
					intAttr("num", seed),
					doubleAttr("dbl", math.Float64frombits(uint64(seed))),
					boolAttr("flag", seed > 0),
					bytesAttr("bin", spanID(seed, i)),
					nestedAttr("nested"),
				}
				spans[i].DroppedAttributesCount = uint32(i)
			}

			events := make([]tracev1.Span_Event, eventCount)
			for j := range events {
				events[j] = tracev1.Span_Event{
					TimeUnixNano: startTime + uint64(j),
					Name:         "event",
				}
				if withAttrs {
					events[j].Attributes = []commonv1.KeyValue{strAttr("e", "v")}
				}
			}
			spans[i].Events = events
			spans[i].DroppedEventsCount = uint32(eventCount)

			links := make([]tracev1.Span_Link, linkCount)
			for j := range links {
				links[j] = tracev1.Span_Link{
					TraceId: traceID(seed, j+200),
					SpanId:  spanID(seed, j+200),
				}
				if withAttrs {
					links[j].Attributes = []commonv1.KeyValue{strAttr("l", "v")}
				}
			}
			spans[i].Links = links
			spans[i].DroppedLinksCount = uint32(linkCount)
		}

		msg := tracev1.TracesData{
			ResourceSpans: []tracev1.ResourceSpans{
				{
					Resource: resourcev1.Resource{
						Attributes: []commonv1.KeyValue{strAttr("service.name", "fuzz-svc")},
					},
					ScopeSpans: []tracev1.ScopeSpans{
						{
							Scope: commonv1.InstrumentationScope{Name: "fuzz-lib", Version: "1.0"},
							Spans: spans,
						},
					},
				},
			},
		}

		assertRoundTrip(t, &msg)
	})
}

// FuzzStructuredMetrics builds valid MetricsData from fuzz parameters,
// covering all 5 metric types with nested data points and exemplars.
func FuzzStructuredMetrics(f *testing.F) {
	f.Add(uint64(1000), uint64(2000), uint8(2), int64(42), true)
	f.Add(uint64(0), uint64(math.MaxUint64), uint8(0), int64(-1), false)
	f.Add(uint64(1<<62), uint64(1<<63), uint8(5), int64(0), true)

	f.Fuzz(func(t *testing.T, startTime uint64, endTime uint64, nPoints uint8, intVal int64, withOptional bool) {
		pointCount := int(nPoints%4) + 1
		dblVal := math.Float64frombits(uint64(intVal))

		numberPoints := make([]metricsv1.NumberDataPoint, pointCount)
		for i := range numberPoints {
			np := metricsv1.NumberDataPoint{
				StartTimeUnixNano: startTime,
				TimeUnixNano:      endTime,
				Attributes:        []commonv1.KeyValue{strAttr("pt", "v")},
				Exemplars: []metricsv1.Exemplar{
					{TimeUnixNano: endTime, Value: &metricsv1.Exemplar_AsDouble{AsDouble: dblVal}},
				},
			}
			if i%2 == 0 {
				np.Value = &metricsv1.NumberDataPoint_AsDouble{AsDouble: dblVal}
			} else {
				np.Value = &metricsv1.NumberDataPoint_AsInt{AsInt: intVal}
			}
			numberPoints[i] = np
		}

		var histSum, histMin, histMax *float64
		if withOptional {
			s, mn, mx := dblVal, dblVal*0.1, dblVal*10
			histSum, histMin, histMax = &s, &mn, &mx
		}

		metrics := []metricsv1.Metric{
			{
				Name: "fuzz.gauge",
				Data: &metricsv1.Metric_Gauge{Gauge: metricsv1.Gauge{DataPoints: numberPoints}},
			},
			{
				Name: "fuzz.sum",
				Data: &metricsv1.Metric_Sum{Sum: metricsv1.Sum{
					AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
					IsMonotonic:            true,
					DataPoints:             numberPoints,
				}},
			},
			{
				Name: "fuzz.histogram",
				Data: &metricsv1.Metric_Histogram{Histogram: metricsv1.Histogram{
					AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
					DataPoints: []metricsv1.HistogramDataPoint{
						{
							StartTimeUnixNano: startTime,
							TimeUnixNano:      endTime,
							Count:             uint64(pointCount) * 10,
							Sum:               histSum,
							BucketCounts:      []uint64{1, 2, 3, 4},
							ExplicitBounds:    []float64{1.0, 10.0, 100.0},
							Min:               histMin,
							Max:               histMax,
						},
					},
				}},
			},
			{
				Name: "fuzz.exp_histogram",
				Data: &metricsv1.Metric_ExponentialHistogram{ExponentialHistogram: metricsv1.ExponentialHistogram{
					AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
					DataPoints: []metricsv1.ExponentialHistogramDataPoint{
						{
							StartTimeUnixNano: startTime,
							TimeUnixNano:      endTime,
							Count:             uint64(pointCount) * 20,
							Sum:               histSum,
							Scale:             int32(intVal % 20),
							ZeroCount:         5,
							Positive: metricsv1.ExponentialHistogramDataPoint_Buckets{
								Offset:       int32(intVal % 100),
								BucketCounts: []uint64{10, 20, 30},
							},
							Negative: metricsv1.ExponentialHistogramDataPoint_Buckets{
								Offset:       int32(-intVal % 100),
								BucketCounts: []uint64{5, 15},
							},
							Min:           histMin,
							Max:           histMax,
							ZeroThreshold: 0.001,
						},
					},
				}},
			},
			{
				Name: "fuzz.summary",
				Data: &metricsv1.Metric_Summary{Summary: metricsv1.Summary{
					DataPoints: []metricsv1.SummaryDataPoint{
						{
							StartTimeUnixNano: startTime,
							TimeUnixNano:      endTime,
							Count:             uint64(pointCount) * 30,
							Sum:               dblVal,
							QuantileValues: []metricsv1.SummaryDataPoint_ValueAtQuantile{
								{Quantile: 0.5, Value: dblVal * 0.5},
								{Quantile: 0.9, Value: dblVal * 0.9},
								{Quantile: 0.99, Value: dblVal * 0.99},
							},
						},
					},
				}},
			},
		}

		msg := metricsv1.MetricsData{
			ResourceMetrics: []metricsv1.ResourceMetrics{
				{
					Resource: resourcev1.Resource{
						Attributes: []commonv1.KeyValue{strAttr("service.name", "fuzz-metrics")},
					},
					ScopeMetrics: []metricsv1.ScopeMetrics{
						{
							Scope:   commonv1.InstrumentationScope{Name: "fuzz-lib"},
							Metrics: metrics,
						},
					},
				},
			},
		}

		assertRoundTrip(t, &msg)
	})
}

// FuzzStructuredLogs builds valid LogsData from fuzz parameters with nested
// AnyValue bodies (kvlist, array) to exercise deep nesting in unmarshal.
func FuzzStructuredLogs(f *testing.F) {
	f.Add(uint64(1000), uint8(2), int32(9), int64(42), true)
	f.Add(uint64(0), uint8(0), int32(0), int64(-1), false)
	f.Add(uint64(math.MaxUint64), uint8(7), int32(24), int64(math.MaxInt64), true)

	f.Fuzz(func(t *testing.T, ts uint64, nRecords uint8, severity int32, intVal int64, deepBody bool) {
		recordCount := int(nRecords%8) + 1
		sevNum := logsv1.SeverityNumber(severity % 25)

		records := make([]logsv1.LogRecord, recordCount)
		for i := range records {
			rec := logsv1.LogRecord{
				TimeUnixNano:         ts + uint64(i),
				ObservedTimeUnixNano: ts + uint64(i) + 1,
				SeverityNumber:       sevNum,
				SeverityText:         "FUZZ",
				Flags:                uint32(i),
				TraceId:              traceID(intVal, i),
				SpanId:               spanID(intVal, i),
				Attributes:           []commonv1.KeyValue{strAttr("idx", "v"), intAttr("n", intVal)},
			}

			if deepBody {
				// Nested kvlist body exercises deep unmarshal paths
				rec.Body = commonv1.AnyValue{
					Value: &commonv1.AnyValue_KvlistValue{KvlistValue: commonv1.KeyValueList{
						Values: []commonv1.KeyValue{
							strAttr("inner", "val"),
							{Key: "array", Value: commonv1.AnyValue{
								Value: &commonv1.AnyValue_ArrayValue{ArrayValue: commonv1.ArrayValue{
									Values: []commonv1.AnyValue{
										{Value: &commonv1.AnyValue_IntValue{IntValue: intVal}},
										{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: math.Float64frombits(uint64(intVal))}},
										{Value: &commonv1.AnyValue_BoolValue{BoolValue: intVal > 0}},
										{Value: &commonv1.AnyValue_BytesValue{BytesValue: spanID(intVal, i)}},
									},
								}},
							}},
						},
					}},
				}
			} else {
				rec.Body = commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "simple"}}
			}

			records[i] = rec
		}

		msg := logsv1.LogsData{
			ResourceLogs: []logsv1.ResourceLogs{
				{
					Resource: resourcev1.Resource{
						Attributes: []commonv1.KeyValue{strAttr("service.name", "fuzz-logs")},
					},
					ScopeLogs: []logsv1.ScopeLogs{
						{
							Scope:      commonv1.InstrumentationScope{Name: "fuzz-lib"},
							LogRecords: records,
						},
					},
				},
			},
		}

		assertRoundTrip(t, &msg)
	})
}

// assertRoundTrip marshals a message and verifies Unmarshal→Marshal→Unmarshal
// produces identical structs, and that Size() matches Marshal() length.
func assertRoundTrip(t *testing.T, msg testutil.Message) {
	t.Helper()

	size := msg.Size()
	bytes1, err := msg.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if size != len(bytes1) {
		t.Fatalf("Size()=%d but Marshal() produced %d bytes", size, len(bytes1))
	}

	// Create a new zero-value instance of the same concrete type
	msg2 := reflect.New(reflect.TypeOf(msg).Elem()).Interface().(testutil.Message)
	if err := msg2.Unmarshal(bytes1); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	bytes2, err := msg2.Marshal()
	if err != nil {
		t.Fatalf("re-Marshal failed: %v", err)
	}

	// Compare marshaled bytes for determinism. This is stronger than
	// DeepEqual on structs and avoids NaN != NaN false positives.
	if !bytes.Equal(bytes1, bytes2) {
		t.Fatalf("marshal output not deterministic: first=%d bytes, second=%d bytes", len(bytes1), len(bytes2))
	}
}

// marshaledSeeds returns marshaled bytes from deeply-nested valid messages,
// giving byte-level fuzz targets realistic starting points.
func marshaledSeeds() [][]byte {
	sum := 100.0
	min := 1.0
	max := 99.0

	traces := tracev1.TracesData{
		ResourceSpans: []tracev1.ResourceSpans{{
			Resource: resourcev1.Resource{
				Attributes: []commonv1.KeyValue{
					strAttr("service.name", "seed-svc"),
					intAttr("pid", 1234),
				},
			},
			ScopeSpans: []tracev1.ScopeSpans{{
				Scope: commonv1.InstrumentationScope{Name: "lib", Version: "1.0"},
				Spans: []tracev1.Span{
					{
						TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
						SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
						ParentSpanId:      []byte{8, 7, 6, 5, 4, 3, 2, 1},
						Name:              "op",
						Kind:              tracev1.Span_SPAN_KIND_SERVER,
						StartTimeUnixNano: 100,
						EndTimeUnixNano:   200,
						Attributes:        []commonv1.KeyValue{strAttr("k", "v"), nestedAttr("n")},
						Events: []tracev1.Span_Event{
							{TimeUnixNano: 150, Name: "evt", Attributes: []commonv1.KeyValue{strAttr("e", "v")}},
						},
						Links: []tracev1.Span_Link{
							{TraceId: []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}, SpanId: []byte{8, 7, 6, 5, 4, 3, 2, 1}},
						},
						Status: tracev1.Status{Code: tracev1.Status_STATUS_CODE_OK, Message: "ok"},
					},
				},
			}},
		}},
	}

	metrics := metricsv1.MetricsData{
		ResourceMetrics: []metricsv1.ResourceMetrics{{
			Resource: resourcev1.Resource{Attributes: []commonv1.KeyValue{strAttr("service.name", "seed-metrics")}},
			ScopeMetrics: []metricsv1.ScopeMetrics{{
				Scope: commonv1.InstrumentationScope{Name: "m-lib"},
				Metrics: []metricsv1.Metric{
					{Name: "gauge", Data: &metricsv1.Metric_Gauge{Gauge: metricsv1.Gauge{
						DataPoints: []metricsv1.NumberDataPoint{{
							TimeUnixNano: 1000,
							Value:        &metricsv1.NumberDataPoint_AsDouble{AsDouble: 42.5},
							Exemplars:    []metricsv1.Exemplar{{TimeUnixNano: 999, Value: &metricsv1.Exemplar_AsDouble{AsDouble: 41.0}}},
						}},
					}}},
					{Name: "hist", Data: &metricsv1.Metric_Histogram{Histogram: metricsv1.Histogram{
						AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
						DataPoints: []metricsv1.HistogramDataPoint{{
							Count: 50, Sum: &sum, BucketCounts: []uint64{10, 20, 15, 5},
							ExplicitBounds: []float64{1, 10, 100}, Min: &min, Max: &max,
						}},
					}}},
					{Name: "exp", Data: &metricsv1.Metric_ExponentialHistogram{ExponentialHistogram: metricsv1.ExponentialHistogram{
						DataPoints: []metricsv1.ExponentialHistogramDataPoint{{
							Count: 200, Scale: 3, ZeroCount: 5,
							Positive: metricsv1.ExponentialHistogramDataPoint_Buckets{Offset: 1, BucketCounts: []uint64{10, 20, 30}},
							Negative: metricsv1.ExponentialHistogramDataPoint_Buckets{Offset: -2, BucketCounts: []uint64{5, 15}},
						}},
					}}},
					{Name: "summary", Data: &metricsv1.Metric_Summary{Summary: metricsv1.Summary{
						DataPoints: []metricsv1.SummaryDataPoint{{
							Count: 300, Sum: 1500.5,
							QuantileValues: []metricsv1.SummaryDataPoint_ValueAtQuantile{{Quantile: 0.5, Value: 4.0}, {Quantile: 0.99, Value: 15.0}},
						}},
					}}},
				},
			}},
		}},
	}

	logs := logsv1.LogsData{
		ResourceLogs: []logsv1.ResourceLogs{{
			Resource: resourcev1.Resource{Attributes: []commonv1.KeyValue{strAttr("service.name", "seed-logs")}},
			ScopeLogs: []logsv1.ScopeLogs{{
				Scope: commonv1.InstrumentationScope{Name: "l-lib"},
				LogRecords: []logsv1.LogRecord{
					{
						TimeUnixNano: 1000, SeverityNumber: logsv1.SeverityNumber_SEVERITY_NUMBER_WARN, SeverityText: "WARN",
						Body: commonv1.AnyValue{Value: &commonv1.AnyValue_KvlistValue{KvlistValue: commonv1.KeyValueList{
							Values: []commonv1.KeyValue{
								strAttr("msg", "nested body"),
								{Key: "arr", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_ArrayValue{ArrayValue: commonv1.ArrayValue{
									Values: []commonv1.AnyValue{
										{Value: &commonv1.AnyValue_IntValue{IntValue: 1}},
										{Value: &commonv1.AnyValue_StringValue{StringValue: "two"}},
									},
								}}}},
							},
						}}},
						Attributes: []commonv1.KeyValue{strAttr("k", "v")},
						TraceId:    []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
						SpanId:     []byte{1, 2, 3, 4, 5, 6, 7, 8},
					},
				},
			}},
		}},
	}

	seeds := make([][]byte, 0, 3)
	for _, msg := range []testutil.Message{&traces, &metrics, &logs} {
		b, err := msg.Marshal()
		if err != nil {
			panic(err)
		}
		seeds = append(seeds, b)
	}
	return seeds
}

// --- Helpers for building fuzz messages ---

func traceID(seed int64, i int) []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b[0:8], uint64(seed))
	binary.LittleEndian.PutUint64(b[8:16], uint64(i))
	return b
}

func spanID(seed int64, i int) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b[0:8], uint64(seed)+uint64(i))
	return b
}

func strAttr(k, v string) commonv1.KeyValue {
	return commonv1.KeyValue{Key: k, Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: v}}}
}

func intAttr(k string, v int64) commonv1.KeyValue {
	return commonv1.KeyValue{Key: k, Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: v}}}
}

func doubleAttr(k string, v float64) commonv1.KeyValue {
	return commonv1.KeyValue{Key: k, Value: commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: v}}}
}

func boolAttr(k string, v bool) commonv1.KeyValue {
	return commonv1.KeyValue{Key: k, Value: commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: v}}}
}

func bytesAttr(k string, v []byte) commonv1.KeyValue {
	return commonv1.KeyValue{Key: k, Value: commonv1.AnyValue{Value: &commonv1.AnyValue_BytesValue{BytesValue: v}}}
}

// nestedAttr creates a kvlist attribute containing an array — 3 levels deep.
func nestedAttr(k string) commonv1.KeyValue {
	return commonv1.KeyValue{
		Key: k,
		Value: commonv1.AnyValue{Value: &commonv1.AnyValue_KvlistValue{KvlistValue: commonv1.KeyValueList{
			Values: []commonv1.KeyValue{
				strAttr("inner", "val"),
				{Key: "arr", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_ArrayValue{ArrayValue: commonv1.ArrayValue{
					Values: []commonv1.AnyValue{
						{Value: &commonv1.AnyValue_IntValue{IntValue: 1}},
						{Value: &commonv1.AnyValue_StringValue{StringValue: "two"}},
					},
				}}}},
			},
		}}},
	}
}
