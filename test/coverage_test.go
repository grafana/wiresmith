package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"

	commonv1 "wiresmith/gen/otlp/common/v1"
	logsv1 "wiresmith/gen/otlp/logs/v1"
	metricsv1 "wiresmith/gen/otlp/metrics/v1"
	profilesv1 "wiresmith/gen/otlp/profiles/v1development"
	resourcev1 "wiresmith/gen/otlp/resource/v1"
	tracev1 "wiresmith/gen/otlp/trace/v1"
)

// Compile-time interface assertions for oneof marker methods.
var (
	_ commonv1.AnyValue_Value         = (*commonv1.AnyValue_StringValue)(nil)
	_ commonv1.AnyValue_Value         = (*commonv1.AnyValue_BoolValue)(nil)
	_ commonv1.AnyValue_Value         = (*commonv1.AnyValue_IntValue)(nil)
	_ commonv1.AnyValue_Value         = (*commonv1.AnyValue_DoubleValue)(nil)
	_ commonv1.AnyValue_Value         = (*commonv1.AnyValue_ArrayValue)(nil)
	_ commonv1.AnyValue_Value         = (*commonv1.AnyValue_KvlistValue)(nil)
	_ commonv1.AnyValue_Value         = (*commonv1.AnyValue_BytesValue)(nil)
	_ commonv1.AnyValue_Value         = (*commonv1.AnyValue_StringValueStrindex)(nil)
	_ metricsv1.Metric_Data           = (*metricsv1.Metric_Gauge)(nil)
	_ metricsv1.Metric_Data           = (*metricsv1.Metric_Sum)(nil)
	_ metricsv1.Metric_Data           = (*metricsv1.Metric_Histogram)(nil)
	_ metricsv1.Metric_Data           = (*metricsv1.Metric_ExponentialHistogram)(nil)
	_ metricsv1.Metric_Data           = (*metricsv1.Metric_Summary)(nil)
	_ metricsv1.NumberDataPoint_Value = (*metricsv1.NumberDataPoint_AsDouble)(nil)
	_ metricsv1.NumberDataPoint_Value = (*metricsv1.NumberDataPoint_AsInt)(nil)
	_ metricsv1.Exemplar_Value        = (*metricsv1.Exemplar_AsDouble)(nil)
	_ metricsv1.Exemplar_Value        = (*metricsv1.Exemplar_AsInt)(nil)
)

// TestUnknownFieldsSkippedAllTypes tests that unknown fields are gracefully skipped
// across multiple wire types and message types. This exercises the skipField helper.
func TestUnknownFieldsSkippedAllTypes(t *testing.T) {
	// Build unknown fields covering all wire types
	buildUnknownFields := func() []byte {
		var extra []byte
		extra = protowire.AppendTag(extra, 200, protowire.VarintType)
		extra = protowire.AppendVarint(extra, 99999)
		extra = protowire.AppendTag(extra, 201, protowire.Fixed32Type)
		extra = protowire.AppendFixed32(extra, 0xCAFEBABE)
		extra = protowire.AppendTag(extra, 202, protowire.Fixed64Type)
		extra = protowire.AppendFixed64(extra, 0xDEADBEEFCAFEBABE)
		extra = protowire.AppendTag(extra, 203, protowire.BytesType)
		extra = protowire.AppendString(extra, "unknown payload")
		return extra
	}

	t.Run("Span", func(t *testing.T) {
		span := tracev1.Span{
			TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name:    "test",
		}
		b, err := span.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded tracev1.Span
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "test", decoded.Name)
		assert.Equal(t, span.TraceId, decoded.TraceId)
	})

	t.Run("SpanEvent", func(t *testing.T) {
		ev := tracev1.Span_Event{TimeUnixNano: 1000, Name: "ev"}
		b, err := ev.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded tracev1.Span_Event
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "ev", decoded.Name)
	})

	t.Run("SpanLink", func(t *testing.T) {
		link := tracev1.Span_Link{
			TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
		}
		b, err := link.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded tracev1.Span_Link
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, link.TraceId, decoded.TraceId)
	})

	t.Run("LogRecord", func(t *testing.T) {
		lr := logsv1.LogRecord{
			TimeUnixNano:   5000,
			SeverityNumber: logsv1.SEVERITY_NUMBER_ERROR,
			SeverityText:   "ERROR",
		}
		b, err := lr.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded logsv1.LogRecord
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "ERROR", decoded.SeverityText)
	})

	t.Run("AnyValue", func(t *testing.T) {
		av := commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 42}}
		b, err := av.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded commonv1.AnyValue
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, int64(42), decoded.Value.(*commonv1.AnyValue_IntValue).IntValue)
	})

	t.Run("KeyValue", func(t *testing.T) {
		kv := commonv1.KeyValue{Key: "k", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v"}}}
		b, err := kv.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded commonv1.KeyValue
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "k", decoded.Key)
	})

	t.Run("InstrumentationScope", func(t *testing.T) {
		scope := commonv1.InstrumentationScope{Name: "lib", Version: "1.0"}
		b, err := scope.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded commonv1.InstrumentationScope
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "lib", decoded.Name)
	})

	t.Run("NumberDataPoint", func(t *testing.T) {
		dp := metricsv1.NumberDataPoint{
			TimeUnixNano: 1000,
			Value:        &metricsv1.NumberDataPoint_AsDouble{AsDouble: 3.14},
		}
		b, err := dp.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded metricsv1.NumberDataPoint
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, uint64(1000), decoded.TimeUnixNano)
	})

	t.Run("HistogramDataPoint", func(t *testing.T) {
		dp := metricsv1.HistogramDataPoint{
			TimeUnixNano: 2000,
			Count:        10,
		}
		b, err := dp.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded metricsv1.HistogramDataPoint
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, uint64(10), decoded.Count)
	})

	t.Run("ExponentialHistogramDataPoint", func(t *testing.T) {
		dp := metricsv1.ExponentialHistogramDataPoint{
			TimeUnixNano: 3000,
			Count:        20,
			Scale:        5,
		}
		b, err := dp.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded metricsv1.ExponentialHistogramDataPoint
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, uint64(20), decoded.Count)
	})

	t.Run("SummaryDataPoint", func(t *testing.T) {
		dp := metricsv1.SummaryDataPoint{
			TimeUnixNano: 4000,
			Count:        30,
			Sum:          100.0,
		}
		b, err := dp.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded metricsv1.SummaryDataPoint
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, uint64(30), decoded.Count)
	})

	t.Run("Exemplar", func(t *testing.T) {
		ex := metricsv1.Exemplar{
			TimeUnixNano: 5000,
			Value:        &metricsv1.Exemplar_AsInt{AsInt: 7},
		}
		b, err := ex.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded metricsv1.Exemplar
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, int64(7), decoded.Value.(*metricsv1.Exemplar_AsInt).AsInt)
	})

	t.Run("Profile", func(t *testing.T) {
		p := profilesv1.Profile{
			TimeUnixNano: 6000,
			DurationNano: 1000,
			ProfileId:    []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		}
		b, err := p.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded profilesv1.Profile
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, uint64(6000), decoded.TimeUnixNano)
	})

	t.Run("Sample", func(t *testing.T) {
		s := profilesv1.Sample{StackIndex: 5, Values: []int64{10, 20}}
		b, err := s.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded profilesv1.Sample
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, int32(5), decoded.StackIndex)
	})

	t.Run("Mapping", func(t *testing.T) {
		m := profilesv1.Mapping{MemoryStart: 0x1000, MemoryLimit: 0x2000}
		b, err := m.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded profilesv1.Mapping
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, uint64(0x1000), decoded.MemoryStart)
	})

	t.Run("Location", func(t *testing.T) {
		loc := profilesv1.Location{
			MappingIndex: 1,
			Address:      0x4000,
			Lines:        []profilesv1.Line{{FunctionIndex: 0, Line: 10}},
		}
		b, err := loc.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded profilesv1.Location
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, uint64(0x4000), decoded.Address)
	})

	t.Run("Function", func(t *testing.T) {
		fn := profilesv1.Function{NameStrindex: 1, StartLine: 42}
		b, err := fn.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded profilesv1.Function
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, int64(42), decoded.StartLine)
	})

	t.Run("Line", func(t *testing.T) {
		line := profilesv1.Line{FunctionIndex: 3, Line: 99, Column: 10}
		b, err := line.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded profilesv1.Line
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, int64(99), decoded.Line)
	})

	t.Run("ValueType", func(t *testing.T) {
		vt := profilesv1.ValueType{TypeStrindex: 1, UnitStrindex: 2}
		b, err := vt.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded profilesv1.ValueType
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, int32(1), decoded.TypeStrindex)
	})

	t.Run("MetricsData", func(t *testing.T) {
		md := metricsv1.MetricsData{}
		b, err := md.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded metricsv1.MetricsData
		require.NoError(t, decoded.Unmarshal(b))
	})

	t.Run("TracesData", func(t *testing.T) {
		td := tracev1.TracesData{}
		b, err := td.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded tracev1.TracesData
		require.NoError(t, decoded.Unmarshal(b))
	})

	t.Run("LogsData", func(t *testing.T) {
		ld := logsv1.LogsData{}
		b, err := ld.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded logsv1.LogsData
		require.NoError(t, decoded.Unmarshal(b))
	})

	t.Run("ResourceSpans", func(t *testing.T) {
		rs := tracev1.ResourceSpans{SchemaUrl: "https://example.com"}
		b, err := rs.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded tracev1.ResourceSpans
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "https://example.com", decoded.SchemaUrl)
	})

	t.Run("ScopeSpans", func(t *testing.T) {
		ss := tracev1.ScopeSpans{SchemaUrl: "https://example.com/scope"}
		b, err := ss.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded tracev1.ScopeSpans
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "https://example.com/scope", decoded.SchemaUrl)
	})

	t.Run("ResourceLogs", func(t *testing.T) {
		rl := logsv1.ResourceLogs{SchemaUrl: "https://example.com/logs"}
		b, err := rl.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded logsv1.ResourceLogs
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "https://example.com/logs", decoded.SchemaUrl)
	})

	t.Run("ScopeLogs", func(t *testing.T) {
		sl := logsv1.ScopeLogs{SchemaUrl: "https://example.com/slogs"}
		b, err := sl.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded logsv1.ScopeLogs
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "https://example.com/slogs", decoded.SchemaUrl)
	})

	t.Run("ResourceMetrics", func(t *testing.T) {
		rm := metricsv1.ResourceMetrics{SchemaUrl: "https://example.com/metrics"}
		b, err := rm.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded metricsv1.ResourceMetrics
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "https://example.com/metrics", decoded.SchemaUrl)
	})

	t.Run("ScopeMetrics", func(t *testing.T) {
		sm := metricsv1.ScopeMetrics{SchemaUrl: "https://example.com/smetrics"}
		b, err := sm.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded metricsv1.ScopeMetrics
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "https://example.com/smetrics", decoded.SchemaUrl)
	})

	t.Run("ResourceProfiles", func(t *testing.T) {
		rp := profilesv1.ResourceProfiles{SchemaUrl: "https://example.com/profiles"}
		b, err := rp.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded profilesv1.ResourceProfiles
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "https://example.com/profiles", decoded.SchemaUrl)
	})

	t.Run("ScopeProfiles", func(t *testing.T) {
		sp := profilesv1.ScopeProfiles{SchemaUrl: "https://example.com/sprofiles"}
		b, err := sp.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded profilesv1.ScopeProfiles
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "https://example.com/sprofiles", decoded.SchemaUrl)
	})

	t.Run("Metric", func(t *testing.T) {
		m := metricsv1.Metric{
			Name: "test.metric",
			Data: &metricsv1.Metric_Gauge{Gauge: metricsv1.Gauge{}},
		}
		b, err := m.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded metricsv1.Metric
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "test.metric", decoded.Name)
	})

	t.Run("ExponentialHistogramDataPoint_Buckets", func(t *testing.T) {
		bk := metricsv1.ExponentialHistogramDataPoint_Buckets{
			Offset:       -2,
			BucketCounts: []uint64{10, 20, 30},
		}
		b, err := bk.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded metricsv1.ExponentialHistogramDataPoint_Buckets
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, int32(-2), decoded.Offset)
	})

	t.Run("SummaryDataPoint_ValueAtQuantile", func(t *testing.T) {
		vq := metricsv1.SummaryDataPoint_ValueAtQuantile{Quantile: 0.99, Value: 42.0}
		b, err := vq.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded metricsv1.SummaryDataPoint_ValueAtQuantile
		require.NoError(t, decoded.Unmarshal(b))
		assert.InDelta(t, 0.99, decoded.Quantile, 0.001)
	})

	t.Run("Status", func(t *testing.T) {
		st := tracev1.Status{Message: "ok", Code: tracev1.STATUS_CODE_OK}
		b, err := st.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded tracev1.Status
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "ok", decoded.Message)
	})

	t.Run("ProfilesDictionary", func(t *testing.T) {
		pd := profilesv1.ProfilesDictionary{
			StringTable: []string{"", "cpu"},
		}
		b, err := pd.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded profilesv1.ProfilesDictionary
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, []string{"", "cpu"}, decoded.StringTable)
	})

	t.Run("KeyValueAndUnit", func(t *testing.T) {
		kvu := profilesv1.KeyValueAndUnit{
			KeyStrindex:  1,
			UnitStrindex: 2,
			Value:        commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 10}},
		}
		b, err := kvu.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded profilesv1.KeyValueAndUnit
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, int32(1), decoded.KeyStrindex)
	})

	t.Run("Stack", func(t *testing.T) {
		s := profilesv1.Stack{LocationIndices: []int32{0, 1, 2}}
		b, err := s.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded profilesv1.Stack
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, []int32{0, 1, 2}, decoded.LocationIndices)
	})

	t.Run("Link", func(t *testing.T) {
		l := profilesv1.Link{
			TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
		}
		b, err := l.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded profilesv1.Link
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, l.TraceId, decoded.TraceId)
	})

	t.Run("EntityRef", func(t *testing.T) {
		er := commonv1.EntityRef{Type: "svc", SchemaUrl: "https://example.com"}
		b, err := er.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded commonv1.EntityRef
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "svc", decoded.Type)
	})

	t.Run("ArrayValue", func(t *testing.T) {
		av := commonv1.ArrayValue{
			Values: []commonv1.AnyValue{
				{Value: &commonv1.AnyValue_IntValue{IntValue: 1}},
			},
		}
		b, err := av.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded commonv1.ArrayValue
		require.NoError(t, decoded.Unmarshal(b))
		require.Len(t, decoded.Values, 1)
	})

	t.Run("KeyValueList", func(t *testing.T) {
		kvl := commonv1.KeyValueList{
			Values: []commonv1.KeyValue{{Key: "k"}},
		}
		b, err := kvl.Marshal()
		require.NoError(t, err)
		b = append(b, buildUnknownFields()...)

		var decoded commonv1.KeyValueList
		require.NoError(t, decoded.Unmarshal(b))
		require.Len(t, decoded.Values, 1)
	})
}

// TestChildTypeMarshalRoundTrip calls Marshal() directly on child types
// that are normally only marshaled via parent's MarshalToSizedBuffer.
func TestChildTypeMarshalRoundTrip(t *testing.T) {
	t.Run("ResourceSpans", func(t *testing.T) {
		rs := tracev1.ResourceSpans{
			Resource: resourcev1.Resource{
				Attributes: []commonv1.KeyValue{
					{Key: "k", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v"}}},
				},
			},
			SchemaUrl: "https://example.com",
		}
		b, err := rs.Marshal()
		require.NoError(t, err)
		require.NotEmpty(t, b)

		var decoded tracev1.ResourceSpans
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "https://example.com", decoded.SchemaUrl)
		assert.Equal(t, "k", decoded.Resource.Attributes[0].Key)
	})

	t.Run("ScopeSpans", func(t *testing.T) {
		ss := tracev1.ScopeSpans{
			Scope: commonv1.InstrumentationScope{Name: "lib"},
			Spans: []tracev1.Span{
				{Name: "span1", TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, SpanId: []byte{1, 2, 3, 4, 5, 6, 7, 8}},
			},
			SchemaUrl: "https://example.com/scope",
		}
		b, err := ss.Marshal()
		require.NoError(t, err)

		var decoded tracev1.ScopeSpans
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "lib", decoded.Scope.Name)
		require.Len(t, decoded.Spans, 1)
	})

	t.Run("Span_Event", func(t *testing.T) {
		ev := tracev1.Span_Event{
			TimeUnixNano:           1000,
			Name:                   "exception",
			DroppedAttributesCount: 2,
			Attributes: []commonv1.KeyValue{
				{Key: "exception.type", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "NullPointerException"}}},
			},
		}
		b, err := ev.Marshal()
		require.NoError(t, err)

		var decoded tracev1.Span_Event
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "exception", decoded.Name)
		assert.Equal(t, uint32(2), decoded.DroppedAttributesCount)
	})

	t.Run("Span_Link", func(t *testing.T) {
		link := tracev1.Span_Link{
			TraceId:    []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:     []byte{1, 2, 3, 4, 5, 6, 7, 8},
			TraceState: "rojo=00f067aa0ba902b7",
			Flags:      1,
		}
		b, err := link.Marshal()
		require.NoError(t, err)

		var decoded tracev1.Span_Link
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "rojo=00f067aa0ba902b7", decoded.TraceState)
		assert.Equal(t, uint32(1), decoded.Flags)
	})

	t.Run("Status", func(t *testing.T) {
		st := tracev1.Status{Message: "cancelled", Code: tracev1.STATUS_CODE_ERROR}
		b, err := st.Marshal()
		require.NoError(t, err)

		var decoded tracev1.Status
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "cancelled", decoded.Message)
		assert.Equal(t, tracev1.STATUS_CODE_ERROR, decoded.Code)
	})

	t.Run("ResourceLogs", func(t *testing.T) {
		rl := logsv1.ResourceLogs{
			Resource:  resourcev1.Resource{DroppedAttributesCount: 5},
			SchemaUrl: "https://example.com/logs",
		}
		b, err := rl.Marshal()
		require.NoError(t, err)

		var decoded logsv1.ResourceLogs
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, uint32(5), decoded.Resource.DroppedAttributesCount)
	})

	t.Run("ScopeLogs", func(t *testing.T) {
		sl := logsv1.ScopeLogs{
			Scope: commonv1.InstrumentationScope{Name: "logger"},
			LogRecords: []logsv1.LogRecord{
				{SeverityText: "WARN", SeverityNumber: logsv1.SEVERITY_NUMBER_WARN},
			},
		}
		b, err := sl.Marshal()
		require.NoError(t, err)

		var decoded logsv1.ScopeLogs
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "logger", decoded.Scope.Name)
		require.Len(t, decoded.LogRecords, 1)
	})

	t.Run("ResourceMetrics", func(t *testing.T) {
		rm := metricsv1.ResourceMetrics{
			Resource:  resourcev1.Resource{DroppedAttributesCount: 3},
			SchemaUrl: "https://example.com/metrics",
		}
		b, err := rm.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.ResourceMetrics
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, uint32(3), decoded.Resource.DroppedAttributesCount)
	})

	t.Run("ScopeMetrics", func(t *testing.T) {
		sm := metricsv1.ScopeMetrics{
			Scope: commonv1.InstrumentationScope{Name: "meter"},
			Metrics: []metricsv1.Metric{
				{Name: "m1", Data: &metricsv1.Metric_Gauge{Gauge: metricsv1.Gauge{}}},
			},
		}
		b, err := sm.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.ScopeMetrics
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "meter", decoded.Scope.Name)
		require.Len(t, decoded.Metrics, 1)
	})

	t.Run("ExponentialHistogramDataPoint_Buckets", func(t *testing.T) {
		bk := metricsv1.ExponentialHistogramDataPoint_Buckets{
			Offset:       -3,
			BucketCounts: []uint64{10, 20, 30},
		}
		b, err := bk.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.ExponentialHistogramDataPoint_Buckets
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, int32(-3), decoded.Offset)
		assert.Equal(t, []uint64{10, 20, 30}, decoded.BucketCounts)
	})

	t.Run("SummaryDataPoint_ValueAtQuantile", func(t *testing.T) {
		vq := metricsv1.SummaryDataPoint_ValueAtQuantile{Quantile: 0.95, Value: 123.0}
		b, err := vq.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.SummaryDataPoint_ValueAtQuantile
		require.NoError(t, decoded.Unmarshal(b))
		assert.InDelta(t, 0.95, decoded.Quantile, 0.001)
		assert.InDelta(t, 123.0, decoded.Value, 0.001)
	})

	t.Run("Profile", func(t *testing.T) {
		p := profilesv1.Profile{
			TimeUnixNano: 1000,
			DurationNano: 500,
			Period:       10000,
			ProfileId:    []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			Samples:      []profilesv1.Sample{{StackIndex: 0, Values: []int64{100}}},
		}
		b, err := p.Marshal()
		require.NoError(t, err)

		var decoded profilesv1.Profile
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, uint64(1000), decoded.TimeUnixNano)
		assert.Equal(t, int64(10000), decoded.Period)
	})

	t.Run("ProfilesDictionary", func(t *testing.T) {
		pd := profilesv1.ProfilesDictionary{
			StringTable:    []string{"", "fn1"},
			MappingTable:   []profilesv1.Mapping{{MemoryStart: 0x1000}},
			LocationTable:  []profilesv1.Location{{Address: 0x2000}},
			FunctionTable:  []profilesv1.Function{{NameStrindex: 1}},
			LinkTable:      []profilesv1.Link{{TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}}},
			AttributeTable: []profilesv1.KeyValueAndUnit{{KeyStrindex: 1}},
			StackTable:     []profilesv1.Stack{{LocationIndices: []int32{0}}},
		}
		b, err := pd.Marshal()
		require.NoError(t, err)

		var decoded profilesv1.ProfilesDictionary
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, []string{"", "fn1"}, decoded.StringTable)
		require.Len(t, decoded.MappingTable, 1)
		require.Len(t, decoded.FunctionTable, 1)
	})

	t.Run("Mapping", func(t *testing.T) {
		m := profilesv1.Mapping{
			MemoryStart:      0x1000,
			MemoryLimit:      0x2000,
			FileOffset:       0x100,
			FilenameStrindex: 1,
			AttributeIndices: []int32{0, 1},
		}
		b, err := m.Marshal()
		require.NoError(t, err)

		var decoded profilesv1.Mapping
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, uint64(0x1000), decoded.MemoryStart)
		assert.Equal(t, []int32{0, 1}, decoded.AttributeIndices)
	})

	t.Run("Location", func(t *testing.T) {
		loc := profilesv1.Location{
			MappingIndex:     0,
			Address:          0x4000,
			Lines:            []profilesv1.Line{{FunctionIndex: 0, Line: 42}},
			AttributeIndices: []int32{0},
		}
		b, err := loc.Marshal()
		require.NoError(t, err)

		var decoded profilesv1.Location
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, uint64(0x4000), decoded.Address)
		require.Len(t, decoded.Lines, 1)
	})

	t.Run("Function", func(t *testing.T) {
		fn := profilesv1.Function{
			NameStrindex:       1,
			SystemNameStrindex: 2,
			FilenameStrindex:   3,
			StartLine:          100,
		}
		b, err := fn.Marshal()
		require.NoError(t, err)

		var decoded profilesv1.Function
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, int64(100), decoded.StartLine)
	})

	t.Run("Link", func(t *testing.T) {
		l := profilesv1.Link{
			TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
		}
		b, err := l.Marshal()
		require.NoError(t, err)

		var decoded profilesv1.Link
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, l.TraceId, decoded.TraceId)
		assert.Equal(t, l.SpanId, decoded.SpanId)
	})

	t.Run("Stack", func(t *testing.T) {
		s := profilesv1.Stack{LocationIndices: []int32{0, 1, 2}}
		b, err := s.Marshal()
		require.NoError(t, err)

		var decoded profilesv1.Stack
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, []int32{0, 1, 2}, decoded.LocationIndices)
	})

	t.Run("KeyValueAndUnit", func(t *testing.T) {
		kvu := profilesv1.KeyValueAndUnit{
			KeyStrindex:  1,
			Value:        commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 42}},
			UnitStrindex: 2,
		}
		b, err := kvu.Marshal()
		require.NoError(t, err)

		var decoded profilesv1.KeyValueAndUnit
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, int32(1), decoded.KeyStrindex)
		assert.Equal(t, int32(2), decoded.UnitStrindex)
	})

	t.Run("ArrayValue", func(t *testing.T) {
		av := commonv1.ArrayValue{
			Values: []commonv1.AnyValue{
				{Value: &commonv1.AnyValue_IntValue{IntValue: 1}},
				{Value: &commonv1.AnyValue_StringValue{StringValue: "two"}},
			},
		}
		b, err := av.Marshal()
		require.NoError(t, err)

		var decoded commonv1.ArrayValue
		require.NoError(t, decoded.Unmarshal(b))
		require.Len(t, decoded.Values, 2)
	})

	t.Run("KeyValueList", func(t *testing.T) {
		kvl := commonv1.KeyValueList{
			Values: []commonv1.KeyValue{
				{Key: "a", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 1}}},
			},
		}
		b, err := kvl.Marshal()
		require.NoError(t, err)

		var decoded commonv1.KeyValueList
		require.NoError(t, decoded.Unmarshal(b))
		require.Len(t, decoded.Values, 1)
		assert.Equal(t, "a", decoded.Values[0].Key)
	})

	t.Run("Gauge", func(t *testing.T) {
		g := metricsv1.Gauge{
			DataPoints: []metricsv1.NumberDataPoint{
				{TimeUnixNano: 1000, Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: 3.14}},
			},
		}
		b, err := g.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.Gauge
		require.NoError(t, decoded.Unmarshal(b))
		require.Len(t, decoded.DataPoints, 1)
	})

	t.Run("Sum", func(t *testing.T) {
		s := metricsv1.Sum{
			AggregationTemporality: metricsv1.AGGREGATION_TEMPORALITY_CUMULATIVE,
			IsMonotonic:            true,
			DataPoints: []metricsv1.NumberDataPoint{
				{TimeUnixNano: 1000, Value: &metricsv1.NumberDataPoint_AsInt{AsInt: 100}},
			},
		}
		b, err := s.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.Sum
		require.NoError(t, decoded.Unmarshal(b))
		assert.True(t, decoded.IsMonotonic)
		assert.Equal(t, metricsv1.AGGREGATION_TEMPORALITY_CUMULATIVE, decoded.AggregationTemporality)
	})

	t.Run("Histogram", func(t *testing.T) {
		h := metricsv1.Histogram{
			AggregationTemporality: metricsv1.AGGREGATION_TEMPORALITY_DELTA,
			DataPoints: []metricsv1.HistogramDataPoint{
				{TimeUnixNano: 1000, Count: 5},
			},
		}
		b, err := h.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.Histogram
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, metricsv1.AGGREGATION_TEMPORALITY_DELTA, decoded.AggregationTemporality)
	})

	t.Run("ExponentialHistogram", func(t *testing.T) {
		eh := metricsv1.ExponentialHistogram{
			AggregationTemporality: metricsv1.AGGREGATION_TEMPORALITY_CUMULATIVE,
			DataPoints: []metricsv1.ExponentialHistogramDataPoint{
				{TimeUnixNano: 1000, Count: 10, Scale: 3},
			},
		}
		b, err := eh.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.ExponentialHistogram
		require.NoError(t, decoded.Unmarshal(b))
		require.Len(t, decoded.DataPoints, 1)
	})

	t.Run("Summary", func(t *testing.T) {
		s := metricsv1.Summary{
			DataPoints: []metricsv1.SummaryDataPoint{
				{TimeUnixNano: 1000, Count: 50, Sum: 250.0},
			},
		}
		b, err := s.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.Summary
		require.NoError(t, decoded.Unmarshal(b))
		require.Len(t, decoded.DataPoints, 1)
	})
}

// TestUnmarshalMalformedInput tests that Unmarshal returns errors (not panics)
// for various kinds of malformed protobuf input.
func TestUnmarshalMalformedInput(t *testing.T) {
	// Truncated tag: high bit set indicates more bytes needed
	truncatedTag := []byte{0x80}

	// Valid tag (field 1, varint) + truncated varint
	truncatedVarint := []byte{0x08, 0x80}

	// Valid tag (field 1, fixed64) + only 4 bytes instead of 8
	truncatedFixed64 := func() []byte {
		var b []byte
		b = protowire.AppendTag(b, 1, protowire.Fixed64Type)
		b = append(b, 0x01, 0x02, 0x03, 0x04)
		return b
	}()

	// Valid tag (field 1, bytes) + invalid length (huge varint)
	invalidBytesLen := func() []byte {
		var b []byte
		b = protowire.AppendTag(b, 1, protowire.BytesType)
		// Encode a length of 999999 but only provide 2 bytes of data
		b = protowire.AppendVarint(b, 999999)
		b = append(b, 0x01, 0x02)
		return b
	}()

	// Valid nested message field with corrupted inner bytes
	corruptedNested := func() []byte {
		var b []byte
		// Field 1 as bytes (simulating a nested message)
		b = protowire.AppendTag(b, 1, protowire.BytesType)
		// Inner message: truncated tag
		inner := []byte{0x80}
		b = protowire.AppendBytes(b, inner)
		return b
	}()

	// Test each malformed input against representative types
	testCases := []struct {
		name  string
		input []byte
	}{
		{"truncated_tag", truncatedTag},
		{"truncated_varint", truncatedVarint},
		{"truncated_fixed64", truncatedFixed64},
		{"invalid_bytes_len", invalidBytesLen},
	}

	for _, tc := range testCases {
		t.Run("Span/"+tc.name, func(t *testing.T) {
			var m tracev1.Span
			assert.Error(t, m.Unmarshal(tc.input))
		})
		t.Run("AnyValue/"+tc.name, func(t *testing.T) {
			var m commonv1.AnyValue
			assert.Error(t, m.Unmarshal(tc.input))
		})
		t.Run("Resource/"+tc.name, func(t *testing.T) {
			var m resourcev1.Resource
			assert.Error(t, m.Unmarshal(tc.input))
		})
		t.Run("LogRecord/"+tc.name, func(t *testing.T) {
			var m logsv1.LogRecord
			assert.Error(t, m.Unmarshal(tc.input))
		})
		t.Run("NumberDataPoint/"+tc.name, func(t *testing.T) {
			var m metricsv1.NumberDataPoint
			assert.Error(t, m.Unmarshal(tc.input))
		})
		t.Run("HistogramDataPoint/"+tc.name, func(t *testing.T) {
			var m metricsv1.HistogramDataPoint
			assert.Error(t, m.Unmarshal(tc.input))
		})
		t.Run("ExponentialHistogramDataPoint/"+tc.name, func(t *testing.T) {
			var m metricsv1.ExponentialHistogramDataPoint
			assert.Error(t, m.Unmarshal(tc.input))
		})
		t.Run("SummaryDataPoint/"+tc.name, func(t *testing.T) {
			var m metricsv1.SummaryDataPoint
			assert.Error(t, m.Unmarshal(tc.input))
		})
		t.Run("Profile/"+tc.name, func(t *testing.T) {
			var m profilesv1.Profile
			assert.Error(t, m.Unmarshal(tc.input))
		})
		t.Run("KeyValue/"+tc.name, func(t *testing.T) {
			var m commonv1.KeyValue
			assert.Error(t, m.Unmarshal(tc.input))
		})
		t.Run("Metric/"+tc.name, func(t *testing.T) {
			var m metricsv1.Metric
			assert.Error(t, m.Unmarshal(tc.input))
		})
	}

	// Corrupted nested message — only applies to types with nested message fields
	t.Run("Resource/corrupted_nested", func(t *testing.T) {
		var m resourcev1.Resource
		assert.Error(t, m.Unmarshal(corruptedNested))
	})
	t.Run("ScopeSpans/corrupted_nested", func(t *testing.T) {
		var m tracev1.ScopeSpans
		assert.Error(t, m.Unmarshal(corruptedNested))
	})
	t.Run("ScopeLogs/corrupted_nested", func(t *testing.T) {
		var m logsv1.ScopeLogs
		assert.Error(t, m.Unmarshal(corruptedNested))
	})
	t.Run("ScopeMetrics/corrupted_nested", func(t *testing.T) {
		var m metricsv1.ScopeMetrics
		assert.Error(t, m.Unmarshal(corruptedNested))
	})

	// Empty input should succeed (zero-value message)
	t.Run("empty_input_succeeds", func(t *testing.T) {
		var span tracev1.Span
		assert.NoError(t, span.Unmarshal(nil))
		assert.NoError(t, span.Unmarshal([]byte{}))

		var lr logsv1.LogRecord
		assert.NoError(t, lr.Unmarshal(nil))

		var dp metricsv1.NumberDataPoint
		assert.NoError(t, dp.Unmarshal(nil))
	})
}

// TestMetricOneofVariantsMarshal exercises all Metric oneof variants
// through direct Marshal calls, ensuring each variant's marshal path works.
func TestMetricOneofVariantsMarshal(t *testing.T) {
	t.Run("Gauge", func(t *testing.T) {
		m := metricsv1.Metric{
			Name:        "gauge.metric",
			Description: "a gauge",
			Unit:        "1",
			Data: &metricsv1.Metric_Gauge{Gauge: metricsv1.Gauge{
				DataPoints: []metricsv1.NumberDataPoint{
					{TimeUnixNano: 1000, Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: 3.14}},
				},
			}},
		}
		b, err := m.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.Metric
		require.NoError(t, decoded.Unmarshal(b))
		assert.Equal(t, "gauge.metric", decoded.Name)
		g, ok := decoded.Data.(*metricsv1.Metric_Gauge)
		require.True(t, ok)
		require.Len(t, g.Gauge.DataPoints, 1)
	})

	t.Run("Sum", func(t *testing.T) {
		m := metricsv1.Metric{
			Name: "sum.metric",
			Data: &metricsv1.Metric_Sum{Sum: metricsv1.Sum{
				AggregationTemporality: metricsv1.AGGREGATION_TEMPORALITY_CUMULATIVE,
				IsMonotonic:            true,
				DataPoints: []metricsv1.NumberDataPoint{
					{TimeUnixNano: 1000, Value: &metricsv1.NumberDataPoint_AsInt{AsInt: 42}},
				},
			}},
		}
		b, err := m.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.Metric
		require.NoError(t, decoded.Unmarshal(b))
		s, ok := decoded.Data.(*metricsv1.Metric_Sum)
		require.True(t, ok)
		assert.True(t, s.Sum.IsMonotonic)
	})

	t.Run("Histogram", func(t *testing.T) {
		sum := 100.0
		min := 1.0
		max := 50.0
		m := metricsv1.Metric{
			Name: "histogram.metric",
			Data: &metricsv1.Metric_Histogram{Histogram: metricsv1.Histogram{
				AggregationTemporality: metricsv1.AGGREGATION_TEMPORALITY_DELTA,
				DataPoints: []metricsv1.HistogramDataPoint{
					{
						TimeUnixNano:   1000,
						Count:          10,
						Sum:            &sum,
						BucketCounts:   []uint64{2, 3, 5},
						ExplicitBounds: []float64{10.0, 25.0},
						Min:            &min,
						Max:            &max,
					},
				},
			}},
		}
		b, err := m.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.Metric
		require.NoError(t, decoded.Unmarshal(b))
		h, ok := decoded.Data.(*metricsv1.Metric_Histogram)
		require.True(t, ok)
		require.Len(t, h.Histogram.DataPoints, 1)
		assert.Equal(t, uint64(10), h.Histogram.DataPoints[0].Count)
	})

	t.Run("ExponentialHistogram", func(t *testing.T) {
		m := metricsv1.Metric{
			Name: "exp_histogram.metric",
			Data: &metricsv1.Metric_ExponentialHistogram{ExponentialHistogram: metricsv1.ExponentialHistogram{
				AggregationTemporality: metricsv1.AGGREGATION_TEMPORALITY_CUMULATIVE,
				DataPoints: []metricsv1.ExponentialHistogramDataPoint{
					{
						TimeUnixNano: 1000,
						Count:        100,
						Scale:        5,
						Positive: metricsv1.ExponentialHistogramDataPoint_Buckets{
							Offset:       0,
							BucketCounts: []uint64{10, 20, 30, 40},
						},
					},
				},
			}},
		}
		b, err := m.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.Metric
		require.NoError(t, decoded.Unmarshal(b))
		eh, ok := decoded.Data.(*metricsv1.Metric_ExponentialHistogram)
		require.True(t, ok)
		require.Len(t, eh.ExponentialHistogram.DataPoints, 1)
	})

	t.Run("Summary", func(t *testing.T) {
		m := metricsv1.Metric{
			Name: "summary.metric",
			Data: &metricsv1.Metric_Summary{Summary: metricsv1.Summary{
				DataPoints: []metricsv1.SummaryDataPoint{
					{
						TimeUnixNano: 1000,
						Count:        100,
						Sum:          500.0,
						QuantileValues: []metricsv1.SummaryDataPoint_ValueAtQuantile{
							{Quantile: 0.5, Value: 5.0},
							{Quantile: 0.99, Value: 20.0},
						},
					},
				},
			}},
		}
		b, err := m.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.Metric
		require.NoError(t, decoded.Unmarshal(b))
		s, ok := decoded.Data.(*metricsv1.Metric_Summary)
		require.True(t, ok)
		require.Len(t, s.Summary.DataPoints, 1)
		assert.Equal(t, uint64(100), s.Summary.DataPoints[0].Count)
	})
}
