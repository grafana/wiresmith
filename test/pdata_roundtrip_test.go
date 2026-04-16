package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"

	commonv1 "github.com/grafana/wiresmith/gen/otlp/common/v1"
	logsv1 "github.com/grafana/wiresmith/gen/otlp/logs/v1"
	metricsv1 "github.com/grafana/wiresmith/gen/otlp/metrics/v1"
	tracev1 "github.com/grafana/wiresmith/gen/otlp/trace/v1"
)

// TestPdataTracesRoundTrip builds traces via the official pdata API, serializes
// to protobuf, round-trips through our generated code, and verifies equality.
func TestPdataTracesRoundTrip(t *testing.T) {
	traces := ptrace.NewTraces()

	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "pdata-test-svc")
	rs.Resource().Attributes().PutInt("process.pid", 1234)
	rs.Resource().SetDroppedAttributesCount(1)

	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("go.opentelemetry.io/test")
	ss.Scope().SetVersion("0.1.0")
	ss.Scope().Attributes().PutStr("scope.attr", "val")

	span := ss.Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
	span.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	span.SetParentSpanID(pcommon.SpanID([8]byte{8, 7, 6, 5, 4, 3, 2, 1}))
	span.TraceState().FromRaw("vendor=opaque")
	span.SetName("pdata-span")
	span.SetKind(ptrace.SpanKindClient)
	span.SetStartTimestamp(pcommon.Timestamp(1700000000000000000))
	span.SetEndTimestamp(pcommon.Timestamp(1700000001000000000))
	span.SetFlags(0x01)
	span.Attributes().PutStr("http.method", "POST")
	span.Attributes().PutInt("http.status_code", 201)
	span.Attributes().PutDouble("http.duration_ms", 42.5)
	span.Attributes().PutBool("http.ok", true)
	span.SetDroppedAttributesCount(2)

	ev := span.Events().AppendEmpty()
	ev.SetName("exception")
	ev.SetTimestamp(pcommon.Timestamp(1700000000500000000))
	ev.Attributes().PutStr("exception.type", "RuntimeError")
	ev.Attributes().PutStr("exception.message", "something broke")
	ev.SetDroppedAttributesCount(1)
	span.SetDroppedEventsCount(3)

	link := span.Links().AppendEmpty()
	link.SetTraceID(pcommon.TraceID([16]byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}))
	link.SetSpanID(pcommon.SpanID([8]byte{8, 7, 6, 5, 4, 3, 2, 1}))
	link.TraceState().FromRaw("linked=true")
	link.Attributes().PutStr("link.reason", "retry")
	link.SetDroppedAttributesCount(0)
	link.SetFlags(0x01)
	span.SetDroppedLinksCount(1)

	span.Status().SetCode(ptrace.StatusCodeError)
	span.Status().SetMessage("deadline exceeded")

	// Second span — minimal
	span2 := ss.Spans().AppendEmpty()
	span2.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
	span2.SetSpanID(pcommon.SpanID([8]byte{2, 2, 2, 2, 2, 2, 2, 2}))
	span2.SetName("child-span")
	span2.SetKind(ptrace.SpanKindInternal)
	span2.SetStartTimestamp(pcommon.Timestamp(1700000000100000000))
	span2.SetEndTimestamp(pcommon.Timestamp(1700000000900000000))
	span2.Status().SetCode(ptrace.StatusCodeOk)

	// Marshal with pdata
	marshaler := &ptrace.ProtoMarshaler{}
	pdataBytes, err := marshaler.MarshalTraces(traces)
	require.NoError(t, err)

	// Unmarshal with our generated code
	var ours tracev1.TracesData
	require.NoError(t, ours.Unmarshal(pdataBytes))

	// Verify structure
	require.Len(t, ours.ResourceSpans, 1)
	require.Len(t, ours.ResourceSpans[0].ScopeSpans, 1)
	require.Len(t, ours.ResourceSpans[0].ScopeSpans[0].Spans, 2)

	s := ours.ResourceSpans[0].ScopeSpans[0].Spans[0]
	assert.Equal(t, "pdata-span", s.Name)
	assert.Equal(t, tracev1.SPAN_KIND_CLIENT, s.Kind)
	assert.Equal(t, uint64(1700000000000000000), s.StartTimeUnixNano)
	assert.Equal(t, uint64(1700000001000000000), s.EndTimeUnixNano)
	assert.Len(t, s.Attributes, 4)
	assert.Len(t, s.Events, 1)
	assert.Equal(t, "exception", s.Events[0].Name)
	assert.Len(t, s.Links, 1)
	assert.Equal(t, tracev1.STATUS_CODE_ERROR, s.Status.Code)
	assert.Equal(t, "deadline exceeded", s.Status.Message)

	// Re-marshal with our code
	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	// Unmarshal back with pdata
	unmarshaler := &ptrace.ProtoUnmarshaler{}
	roundTripped, err := unmarshaler.UnmarshalTraces(ourBytes)
	require.NoError(t, err)

	// Compare original and round-tripped via a second pdata marshal
	roundTrippedBytes, err := marshaler.MarshalTraces(roundTripped)
	require.NoError(t, err)
	assert.Equal(t, pdataBytes, roundTrippedBytes)
}

// TestPdataMetricsRoundTrip builds metrics (gauge, sum, histogram,
// exponential histogram) via pdata, round-trips through our generated code.
func TestPdataMetricsRoundTrip(t *testing.T) {
	metrics := pmetric.NewMetrics()

	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "metrics-test-svc")

	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("test-meter")
	sm.Scope().SetVersion("0.2.0")

	// Gauge
	gauge := sm.Metrics().AppendEmpty()
	gauge.SetName("cpu_usage")
	gauge.SetDescription("CPU usage percentage")
	gauge.SetUnit("%")
	gdp := gauge.SetEmptyGauge().DataPoints().AppendEmpty()
	gdp.SetTimestamp(pcommon.Timestamp(1700000000000000000))
	gdp.SetDoubleValue(72.5)
	gdp.Attributes().PutStr("cpu", "0")

	// Sum (monotonic, cumulative)
	sum := sm.Metrics().AppendEmpty()
	sum.SetName("requests_total")
	sum.SetDescription("Total requests")
	sum.SetUnit("1")
	s := sum.SetEmptySum()
	s.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	s.SetIsMonotonic(true)
	sdp := s.DataPoints().AppendEmpty()
	sdp.SetStartTimestamp(pcommon.Timestamp(1699999000000000000))
	sdp.SetTimestamp(pcommon.Timestamp(1700000000000000000))
	sdp.SetIntValue(42567)
	sdp.Attributes().PutStr("method", "GET")
	sdp.Attributes().PutStr("path", "/api/v1/data")

	// Histogram
	hist := sm.Metrics().AppendEmpty()
	hist.SetName("request_duration")
	hist.SetDescription("Request duration in seconds")
	hist.SetUnit("s")
	h := hist.SetEmptyHistogram()
	h.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	hdp := h.DataPoints().AppendEmpty()
	hdp.SetStartTimestamp(pcommon.Timestamp(1699999000000000000))
	hdp.SetTimestamp(pcommon.Timestamp(1700000000000000000))
	hdp.SetCount(500)
	hdp.SetSum(123.456)
	hdp.SetMin(0.001)
	hdp.SetMax(9.99)
	hdp.ExplicitBounds().FromRaw([]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0})
	hdp.BucketCounts().FromRaw([]uint64{10, 20, 50, 80, 100, 80, 60, 50, 30, 15, 5, 0})
	hdp.Attributes().PutStr("endpoint", "/health")

	// Exponential histogram
	expHist := sm.Metrics().AppendEmpty()
	expHist.SetName("response_size")
	expHist.SetDescription("Response body size")
	expHist.SetUnit("By")
	eh := expHist.SetEmptyExponentialHistogram()
	eh.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	ehdp := eh.DataPoints().AppendEmpty()
	ehdp.SetStartTimestamp(pcommon.Timestamp(1699999000000000000))
	ehdp.SetTimestamp(pcommon.Timestamp(1700000000000000000))
	ehdp.SetCount(200)
	ehdp.SetSum(51200.0)
	ehdp.SetScale(4)
	ehdp.SetZeroCount(3)
	ehdp.SetZeroThreshold(0.001)
	ehdp.SetMin(1.0)
	ehdp.SetMax(8192.0)
	ehdp.Positive().SetOffset(2)
	ehdp.Positive().BucketCounts().FromRaw([]uint64{5, 10, 25, 40, 30, 20, 10})
	ehdp.Negative().SetOffset(1)
	ehdp.Negative().BucketCounts().FromRaw([]uint64{2, 3, 5})
	ehdp.Attributes().PutStr("service", "api")

	// Marshal with pdata
	marshaler := &pmetric.ProtoMarshaler{}
	pdataBytes, err := marshaler.MarshalMetrics(metrics)
	require.NoError(t, err)

	// Unmarshal with our generated code
	var ours metricsv1.MetricsData
	require.NoError(t, ours.Unmarshal(pdataBytes))

	// Verify structure
	require.Len(t, ours.ResourceMetrics, 1)
	require.Len(t, ours.ResourceMetrics[0].ScopeMetrics, 1)
	scopeMetrics := ours.ResourceMetrics[0].ScopeMetrics[0]
	require.Len(t, scopeMetrics.Metrics, 4)

	// Gauge
	assert.Equal(t, "cpu_usage", scopeMetrics.Metrics[0].Name)
	require.NotNil(t, scopeMetrics.Metrics[0].Data)
	gaugeData := scopeMetrics.Metrics[0].Data.(*metricsv1.Metric_Gauge)
	require.Len(t, gaugeData.Gauge.DataPoints, 1)
	assert.InDelta(t, 72.5, gaugeData.Gauge.DataPoints[0].Value.(*metricsv1.NumberDataPoint_AsDouble).AsDouble, 0.001)

	// Sum
	assert.Equal(t, "requests_total", scopeMetrics.Metrics[1].Name)
	sumData := scopeMetrics.Metrics[1].Data.(*metricsv1.Metric_Sum)
	assert.True(t, sumData.Sum.IsMonotonic)
	assert.Equal(t, metricsv1.AGGREGATION_TEMPORALITY_CUMULATIVE, sumData.Sum.AggregationTemporality)
	require.Len(t, sumData.Sum.DataPoints, 1)
	assert.Equal(t, int64(42567), sumData.Sum.DataPoints[0].Value.(*metricsv1.NumberDataPoint_AsInt).AsInt)

	// Histogram
	assert.Equal(t, "request_duration", scopeMetrics.Metrics[2].Name)
	histData := scopeMetrics.Metrics[2].Data.(*metricsv1.Metric_Histogram)
	require.Len(t, histData.Histogram.DataPoints, 1)
	assert.Equal(t, uint64(500), histData.Histogram.DataPoints[0].Count)
	assert.InDelta(t, 123.456, *histData.Histogram.DataPoints[0].Sum, 0.001)

	// Exponential histogram
	assert.Equal(t, "response_size", scopeMetrics.Metrics[3].Name)
	expHistData := scopeMetrics.Metrics[3].Data.(*metricsv1.Metric_ExponentialHistogram)
	require.Len(t, expHistData.ExponentialHistogram.DataPoints, 1)
	assert.Equal(t, uint64(200), expHistData.ExponentialHistogram.DataPoints[0].Count)
	assert.Equal(t, int32(4), expHistData.ExponentialHistogram.DataPoints[0].Scale)

	// Re-marshal with our code
	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	// Unmarshal back with pdata
	unmarshaler := &pmetric.ProtoUnmarshaler{}
	roundTripped, err := unmarshaler.UnmarshalMetrics(ourBytes)
	require.NoError(t, err)

	// Compare via re-serialization
	roundTrippedBytes, err := marshaler.MarshalMetrics(roundTripped)
	require.NoError(t, err)
	assert.Equal(t, pdataBytes, roundTrippedBytes)
}

// TestPdataLogsRoundTrip builds logs via pdata, round-trips through our generated code.
func TestPdataLogsRoundTrip(t *testing.T) {
	logs := plog.NewLogs()

	rl := logs.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "logs-test-svc")
	rl.Resource().Attributes().PutStr("deployment.environment", "staging")

	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName("app-logger")
	sl.Scope().SetVersion("1.0.0")

	// INFO log
	lr1 := sl.LogRecords().AppendEmpty()
	lr1.SetTimestamp(pcommon.Timestamp(1700000000000000000))
	lr1.SetObservedTimestamp(pcommon.Timestamp(1700000000001000000))
	lr1.SetSeverityNumber(plog.SeverityNumberInfo)
	lr1.SetSeverityText("INFO")
	lr1.Body().SetStr("User login successful")
	lr1.Attributes().PutStr("user.id", "u-12345")
	lr1.Attributes().PutStr("session.id", "s-67890")
	lr1.SetFlags(plog.DefaultLogRecordFlags)
	lr1.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
	lr1.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))

	// ERROR log with structured body
	lr2 := sl.LogRecords().AppendEmpty()
	lr2.SetTimestamp(pcommon.Timestamp(1700000001000000000))
	lr2.SetObservedTimestamp(pcommon.Timestamp(1700000001001000000))
	lr2.SetSeverityNumber(plog.SeverityNumberError)
	lr2.SetSeverityText("ERROR")
	lr2.Body().SetStr("Database connection failed: timeout after 30s")
	lr2.Attributes().PutStr("db.system", "postgresql")
	lr2.Attributes().PutStr("db.connection_string", "host=db.example.com port=5432")
	lr2.Attributes().PutInt("retry.count", 3)
	lr2.SetDroppedAttributesCount(1)

	// WARN log — minimal
	lr3 := sl.LogRecords().AppendEmpty()
	lr3.SetTimestamp(pcommon.Timestamp(1700000002000000000))
	lr3.SetSeverityNumber(plog.SeverityNumberWarn)
	lr3.SetSeverityText("WARN")
	lr3.Body().SetStr("Rate limit approaching threshold")

	// Marshal with pdata
	marshaler := &plog.ProtoMarshaler{}
	pdataBytes, err := marshaler.MarshalLogs(logs)
	require.NoError(t, err)

	// Unmarshal with our generated code
	var ours logsv1.LogsData
	require.NoError(t, ours.Unmarshal(pdataBytes))

	// Verify structure
	require.Len(t, ours.ResourceLogs, 1)
	require.Len(t, ours.ResourceLogs[0].ScopeLogs, 1)
	scopeLogs := ours.ResourceLogs[0].ScopeLogs[0]
	require.Len(t, scopeLogs.LogRecords, 3)

	// INFO log
	bodyStr := scopeLogs.LogRecords[0].Body.Value.(*commonv1.AnyValue_StringValue)
	assert.Equal(t, "User login successful", bodyStr.StringValue)
	assert.Equal(t, logsv1.SEVERITY_NUMBER_INFO, scopeLogs.LogRecords[0].SeverityNumber)
	assert.Equal(t, "INFO", scopeLogs.LogRecords[0].SeverityText)
	assert.Equal(t, uint64(1700000000000000000), scopeLogs.LogRecords[0].TimeUnixNano)
	assert.Equal(t, uint64(1700000000001000000), scopeLogs.LogRecords[0].ObservedTimeUnixNano)
	assert.Len(t, scopeLogs.LogRecords[0].Attributes, 2)

	// ERROR log
	assert.Equal(t, logsv1.SEVERITY_NUMBER_ERROR, scopeLogs.LogRecords[1].SeverityNumber)
	assert.Equal(t, uint32(1), scopeLogs.LogRecords[1].DroppedAttributesCount)

	// Re-marshal with our code
	ourBytes, err := ours.Marshal()
	require.NoError(t, err)

	// Unmarshal back with pdata
	unmarshaler := &plog.ProtoUnmarshaler{}
	roundTripped, err := unmarshaler.UnmarshalLogs(ourBytes)
	require.NoError(t, err)

	// Compare via re-serialization
	roundTrippedBytes, err := marshaler.MarshalLogs(roundTripped)
	require.NoError(t, err)
	assert.Equal(t, pdataBytes, roundTrippedBytes)
}
