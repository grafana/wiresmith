package test

import (
	"bytes"
	"math"
	"math/rand"
	"reflect"
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

// sizedBufferMarshaler is implemented by all generated types.
type sizedBufferMarshaler interface {
	message
	MarshalToSizedBuffer([]byte) (int, error)
}

// ---------------------------------------------------------------------------
// 1. MarshalToSizedBuffer verification
//    Inspired by gogoproto's Test{Type}MarshalTo pattern.
// ---------------------------------------------------------------------------

func TestMarshalToSizedBuffer(t *testing.T) {
	t.Run("EmptyMessages", func(t *testing.T) {
		for name, ctor := range allMessageConstructors() {
			t.Run(name, func(t *testing.T) {
				m := ctor().(sizedBufferMarshaler)
				size := m.Size()
				buf := make([]byte, size)
				n, err := m.MarshalToSizedBuffer(buf)
				require.NoError(t, err)
				assert.Equal(t, size, n, "MarshalToSizedBuffer returned %d, Size() is %d", n, size)

				marshaled, err := m.Marshal()
				require.NoError(t, err)
				// Marshal() returns nil for empty messages; MarshalToSizedBuffer with 0-len buf returns []byte{}
				assert.True(t, bytes.Equal(marshaled, buf[:n]), "MarshalToSizedBuffer output differs from Marshal()")
			})
		}
	})

	t.Run("PopulatedMessages", func(t *testing.T) {
		sum42 := 42.0
		populated := map[string]sizedBufferMarshaler{
			"Resource": &resourcev1.Resource{
				Attributes:             []commonv1.KeyValue{{Key: "k", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v"}}}},
				DroppedAttributesCount: 5,
			},
			"Span": &tracev1.Span{
				TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				SpanId:  []byte{1, 2, 3, 4, 5, 6, 7, 8}, Name: "test",
				Kind: tracev1.Span_SpanKind_SPAN_KIND_SERVER, StartTimeUnixNano: 1000, EndTimeUnixNano: 2000,
				Events: []tracev1.Span_Event{{TimeUnixNano: 1500, Name: "ev"}},
				Links:  []tracev1.Span_Link{{TraceId: make([]byte, 16), SpanId: make([]byte, 8)}},
				Status: tracev1.Status{Code: tracev1.Status_StatusCode_STATUS_CODE_OK, Message: "ok"},
			},
			"LogRecord": &logsv1.LogRecord{
				TimeUnixNano: 1000, SeverityNumber: logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR, SeverityText: "ERROR",
				Body: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "msg"}},
			},
			"HistogramDataPoint": &metricsv1.HistogramDataPoint{
				Count: 10, Sum: &sum42, BucketCounts: []uint64{1, 2, 3, 4}, ExplicitBounds: []float64{10, 50, 100},
			},
			"ExponentialHistogramDataPoint": &metricsv1.ExponentialHistogramDataPoint{
				Count: 5, Scale: -3,
				Positive: metricsv1.ExponentialHistogramDataPoint_Buckets{Offset: 1, BucketCounts: []uint64{1, 2}},
				Negative: metricsv1.ExponentialHistogramDataPoint_Buckets{Offset: -1, BucketCounts: []uint64{3}},
			},
			"SummaryDataPoint": &metricsv1.SummaryDataPoint{
				Count: 100, Sum: 999.9,
				QuantileValues: []metricsv1.SummaryDataPoint_ValueAtQuantile{{Quantile: 0.5, Value: 50}},
			},
			"NumberDataPoint_Double": &metricsv1.NumberDataPoint{
				TimeUnixNano: 1, Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: 3.14},
			},
			"NumberDataPoint_Int": &metricsv1.NumberDataPoint{
				TimeUnixNano: 1, Value: &metricsv1.NumberDataPoint_AsInt{AsInt: -42},
			},
			"Exemplar": &metricsv1.Exemplar{
				TimeUnixNano: 1000, Value: &metricsv1.Exemplar_AsDouble{AsDouble: 1.5},
				FilteredAttributes: []commonv1.KeyValue{{Key: "f", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: true}}}},
			},
			"AnyValue_Kvlist": &commonv1.AnyValue{Value: &commonv1.AnyValue_KvlistValue{KvlistValue: commonv1.KeyValueList{
				Values: []commonv1.KeyValue{{Key: "n", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 1}}}},
			}}},
			"AnyValue_Array": &commonv1.AnyValue{Value: &commonv1.AnyValue_ArrayValue{ArrayValue: commonv1.ArrayValue{
				Values: []commonv1.AnyValue{{Value: &commonv1.AnyValue_IntValue{IntValue: 1}}, {Value: &commonv1.AnyValue_StringValue{StringValue: "two"}}},
			}}},
			"Metric_Gauge": &metricsv1.Metric{
				Name: "g", Data: &metricsv1.Metric_Gauge{Gauge: metricsv1.Gauge{
					DataPoints: []metricsv1.NumberDataPoint{{TimeUnixNano: 1, Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: 1.5}}},
				}},
			},
			"Metric_Sum": &metricsv1.Metric{
				Name: "s", Data: &metricsv1.Metric_Sum{Sum: metricsv1.Sum{
					IsMonotonic: true, AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
					DataPoints: []metricsv1.NumberDataPoint{{TimeUnixNano: 1, Value: &metricsv1.NumberDataPoint_AsInt{AsInt: 100}}},
				}},
			},
			"Profile": &profilesv1.Profile{
				TimeUnixNano: 1000, DurationNano: 500,
				ProfileId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				Samples:   []profilesv1.Sample{{StackIndex: 0, Values: []int64{100, 200}}},
			},
			"ProfilesDictionary": &profilesv1.ProfilesDictionary{
				StringTable:   []string{"", "fn1", "fn2"},
				FunctionTable: []profilesv1.Function{{NameStrindex: 1, StartLine: 10}},
				StackTable:    []profilesv1.Stack{{LocationIndices: []int32{0, 1}}},
			},
			"TracesData": &tracev1.TracesData{
				ResourceSpans: []tracev1.ResourceSpans{{
					Resource: resourcev1.Resource{Attributes: []commonv1.KeyValue{{Key: "svc", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "a"}}}}},
					ScopeSpans: []tracev1.ScopeSpans{{
						Scope: commonv1.InstrumentationScope{Name: "lib"},
						Spans: []tracev1.Span{{TraceId: make([]byte, 16), SpanId: make([]byte, 8), Name: "s1"}},
					}},
				}},
			},
		}

		for name, m := range populated {
			t.Run(name, func(t *testing.T) {
				size := m.Size()
				require.Greater(t, size, 0, "populated message should have non-zero size")

				buf := make([]byte, size)
				n, err := m.MarshalToSizedBuffer(buf)
				require.NoError(t, err)
				assert.Equal(t, size, n, "MarshalToSizedBuffer wrote %d bytes, Size() is %d", n, size)

				marshaled, err := m.Marshal()
				require.NoError(t, err)
				assert.Equal(t, marshaled, buf[:n], "MarshalToSizedBuffer output differs from Marshal()")
			})
		}
	})

	t.Run("OversizedBuffer", func(t *testing.T) {
		// MarshalToSizedBuffer should work with buffers larger than Size().
		// The data is written at the end of the buffer (reverse write).
		r := &resourcev1.Resource{
			Attributes:             []commonv1.KeyValue{{Key: "k", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v"}}}},
			DroppedAttributesCount: 5,
		}
		size := r.Size()
		oversized := make([]byte, size+100)
		n, err := r.MarshalToSizedBuffer(oversized)
		require.NoError(t, err)
		assert.Equal(t, size, n)

		marshaled, err := r.Marshal()
		require.NoError(t, err)

		// Reverse-write puts data at end of buffer
		assert.Equal(t, marshaled, oversized[len(oversized)-n:])
	})
}

// ---------------------------------------------------------------------------
// 2. "Little fuzz" byte mutation
//    Inspired by gogoproto's pattern: marshal valid message, randomly
//    mutate bytes, verify Unmarshal never panics.
// ---------------------------------------------------------------------------

func TestLittleFuzz(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	sum := 42.0
	messages := map[string]message{
		"Resource": &resourcev1.Resource{
			Attributes:             []commonv1.KeyValue{{Key: "k", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v"}}}},
			DroppedAttributesCount: 5,
		},
		"Span": &tracev1.Span{
			TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SpanId:  []byte{1, 2, 3, 4, 5, 6, 7, 8}, Name: "test",
			Kind: tracev1.Span_SpanKind_SPAN_KIND_SERVER, StartTimeUnixNano: 1000, EndTimeUnixNano: 2000,
			Events: []tracev1.Span_Event{{TimeUnixNano: 1500, Name: "ev"}},
			Status: tracev1.Status{Code: tracev1.Status_StatusCode_STATUS_CODE_OK},
		},
		"LogRecord": &logsv1.LogRecord{
			TimeUnixNano: 1000, SeverityNumber: logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR,
			Body: commonv1.AnyValue{Value: &commonv1.AnyValue_KvlistValue{KvlistValue: commonv1.KeyValueList{
				Values: []commonv1.KeyValue{{Key: "msg", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "err"}}}},
			}}},
		},
		"HistogramDataPoint": &metricsv1.HistogramDataPoint{
			Count: 10, Sum: &sum, BucketCounts: []uint64{1, 2, 3}, ExplicitBounds: []float64{10, 50},
		},
		"ExponentialHistogramDataPoint": &metricsv1.ExponentialHistogramDataPoint{
			Count: 5, Scale: 3,
			Positive: metricsv1.ExponentialHistogramDataPoint_Buckets{Offset: 1, BucketCounts: []uint64{10, 20, 30}},
		},
		"Metric_Gauge": &metricsv1.Metric{
			Name: "g", Data: &metricsv1.Metric_Gauge{Gauge: metricsv1.Gauge{
				DataPoints: []metricsv1.NumberDataPoint{{TimeUnixNano: 1, Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: 1.5}}},
			}},
		},
		"Profile": &profilesv1.Profile{
			TimeUnixNano: 1000, DurationNano: 500,
			ProfileId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			Samples:   []profilesv1.Sample{{StackIndex: 0, Values: []int64{100}}},
		},
		"TracesData": &tracev1.TracesData{
			ResourceSpans: []tracev1.ResourceSpans{{
				Resource: resourcev1.Resource{Attributes: []commonv1.KeyValue{{Key: "svc", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "a"}}}}},
				ScopeSpans: []tracev1.ScopeSpans{{
					Scope: commonv1.InstrumentationScope{Name: "lib"},
					Spans: []tracev1.Span{{TraceId: make([]byte, 16), SpanId: make([]byte, 8), Name: "s1"}},
				}},
			}},
		},
	}

	for name, msg := range messages {
		t.Run(name, func(t *testing.T) {
			b, err := msg.Marshal()
			require.NoError(t, err)
			require.NotEmpty(t, b)

			littlefuzz := make([]byte, len(b))
			copy(littlefuzz, b)

			for range 100 {
				littlefuzz[rng.Intn(len(littlefuzz))] = byte(rng.Intn(256))
				// Occasionally grow
				if rng.Intn(5) == 0 {
					littlefuzz = append(littlefuzz, byte(rng.Intn(256)))
				}
				m2 := reflect.New(reflect.TypeOf(msg).Elem()).Interface().(message)
				// Must not panic — errors are expected and fine
				_ = m2.Unmarshal(littlefuzz)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3. Random populated message roundtrip
//    Inspired by gogoproto's NewPopulated{Type} + Test{Type}Proto pattern.
// ---------------------------------------------------------------------------

func TestRandomPopulatedRoundTrip(t *testing.T) {
	const iterations = 50
	rng := rand.New(rand.NewSource(616)) // Same seed as gogoproto benchmarks

	for name, ctor := range allMessageConstructors() {
		t.Run(name, func(t *testing.T) {
			for i := range iterations {
				m := ctor()
				populateRandom(m, rng)

				size := m.Size()
				b, err := m.Marshal()
				require.NoError(t, err)
				require.Equal(t, size, len(b), "iter %d: Size()=%d but Marshal() produced %d bytes", i, size, len(b))

				m2 := ctor()
				require.NoError(t, m2.Unmarshal(b))

				b2, err := m2.Marshal()
				require.NoError(t, err)
				require.True(t, bytes.Equal(b, b2), "iter %d: roundtrip bytes mismatch: %d vs %d bytes", i, len(b), len(b2))
			}
		})
	}
}

// populateRandom fills exported fields of a message struct with random values.
func populateRandom(m message, rng *rand.Rand) {
	v := reflect.ValueOf(m).Elem()
	populateRandomValue(v, rng, 0)
}

func populateRandomValue(v reflect.Value, rng *rand.Rand, depth int) {
	if depth > 3 {
		return // Avoid unbounded recursion
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := range v.NumField() {
			f := v.Field(i)
			if !f.CanSet() {
				continue
			}
			populateRandomField(f, v.Type().Field(i), rng, depth)
		}
	}
}

func populateRandomField(f reflect.Value, sf reflect.StructField, rng *rand.Rand, depth int) {
	switch f.Kind() {
	case reflect.String:
		n := rng.Intn(20)
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(rng.Intn(26) + 'a')
		}
		f.SetString(string(b))

	case reflect.Bool:
		f.SetBool(rng.Intn(2) == 0)

	case reflect.Int32:
		f.SetInt(int64(rng.Int31()))
		if rng.Intn(2) == 0 {
			f.SetInt(-f.Int())
		}

	case reflect.Int64:
		f.SetInt(rng.Int63())
		if rng.Intn(2) == 0 {
			f.SetInt(-f.Int())
		}

	case reflect.Uint32:
		f.SetUint(uint64(rng.Uint32()))

	case reflect.Uint64:
		f.SetUint(rng.Uint64())

	case reflect.Float64:
		f.SetFloat(rng.NormFloat64() * 1000)

	case reflect.Slice:
		elemType := f.Type().Elem()
		switch elemType.Kind() {
		case reflect.Uint8: // []byte
			n := rng.Intn(20) + 1
			b := make([]byte, n)
			for i := range b {
				b[i] = byte(rng.Intn(256))
			}
			f.SetBytes(b)
		case reflect.Uint64:
			n := rng.Intn(5) + 1
			s := reflect.MakeSlice(f.Type(), n, n)
			for i := range n {
				s.Index(i).SetUint(rng.Uint64())
			}
			f.Set(s)
		case reflect.Int64:
			n := rng.Intn(5) + 1
			s := reflect.MakeSlice(f.Type(), n, n)
			for i := range n {
				s.Index(i).SetInt(rng.Int63())
			}
			f.Set(s)
		case reflect.Int32:
			n := rng.Intn(5) + 1
			s := reflect.MakeSlice(f.Type(), n, n)
			for i := range n {
				s.Index(i).SetInt(int64(rng.Int31()))
			}
			f.Set(s)
		case reflect.Float64:
			n := rng.Intn(5) + 1
			s := reflect.MakeSlice(f.Type(), n, n)
			for i := range n {
				s.Index(i).SetFloat(rng.NormFloat64() * 100)
			}
			f.Set(s)
		case reflect.String:
			n := rng.Intn(3) + 1
			s := reflect.MakeSlice(f.Type(), n, n)
			for i := range n {
				b := make([]byte, rng.Intn(10)+1)
				for j := range b {
					b[j] = byte(rng.Intn(26) + 'a')
				}
				s.Index(i).SetString(string(b))
			}
			f.Set(s)
		case reflect.Struct:
			if depth < 3 {
				n := rng.Intn(3) + 1
				s := reflect.MakeSlice(f.Type(), n, n)
				for i := range n {
					populateRandomValue(s.Index(i), rng, depth+1)
				}
				f.Set(s)
			}
		}

	case reflect.Pointer:
		// Optional fields (like *float64 for histogram Sum/Min/Max)
		switch f.Type().Elem().Kind() {
		case reflect.Float64:
			v := rng.NormFloat64() * 1000
			p := reflect.New(f.Type().Elem())
			p.Elem().SetFloat(v)
			f.Set(p)
		}

	case reflect.Interface:
		// Oneof fields — set a random variant based on known types
		populateRandomOneof(f, sf, rng, depth)

	case reflect.Struct:
		populateRandomValue(f, rng, depth+1)
	}
}

func populateRandomOneof(f reflect.Value, sf reflect.StructField, rng *rand.Rand, depth int) {
	if depth > 2 {
		return
	}
	typeName := sf.Name
	switch typeName {
	case "Value":
		// Could be AnyValue_Value, NumberDataPoint_Value, or Exemplar_Value
		parentType := f.Type()
		if parentType == reflect.TypeFor[commonv1.AnyValue_Value]() {
			// AnyValue oneof
			switch rng.Intn(7) {
			case 0:
				f.Set(reflect.ValueOf(&commonv1.AnyValue_StringValue{StringValue: randString(rng)}))
			case 1:
				f.Set(reflect.ValueOf(&commonv1.AnyValue_BoolValue{BoolValue: rng.Intn(2) == 0}))
			case 2:
				f.Set(reflect.ValueOf(&commonv1.AnyValue_IntValue{IntValue: rng.Int63()}))
			case 3:
				f.Set(reflect.ValueOf(&commonv1.AnyValue_DoubleValue{DoubleValue: rng.NormFloat64()}))
			case 4:
				f.Set(reflect.ValueOf(&commonv1.AnyValue_BytesValue{BytesValue: []byte{byte(rng.Intn(256)), byte(rng.Intn(256))}}))
			case 5:
				f.Set(reflect.ValueOf(&commonv1.AnyValue_ArrayValue{ArrayValue: commonv1.ArrayValue{
					Values: []commonv1.AnyValue{{Value: &commonv1.AnyValue_IntValue{IntValue: rng.Int63()}}},
				}}))
			case 6:
				f.Set(reflect.ValueOf(&commonv1.AnyValue_KvlistValue{KvlistValue: commonv1.KeyValueList{
					Values: []commonv1.KeyValue{{Key: "k", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: rng.Int63()}}}},
				}}))
			}
		} else if parentType == reflect.TypeFor[metricsv1.NumberDataPoint_Value]() {
			if rng.Intn(2) == 0 {
				f.Set(reflect.ValueOf(&metricsv1.NumberDataPoint_AsDouble{AsDouble: rng.NormFloat64()}))
			} else {
				f.Set(reflect.ValueOf(&metricsv1.NumberDataPoint_AsInt{AsInt: rng.Int63()}))
			}
		} else if parentType == reflect.TypeFor[metricsv1.Exemplar_Value]() {
			if rng.Intn(2) == 0 {
				f.Set(reflect.ValueOf(&metricsv1.Exemplar_AsDouble{AsDouble: rng.NormFloat64()}))
			} else {
				f.Set(reflect.ValueOf(&metricsv1.Exemplar_AsInt{AsInt: rng.Int63()}))
			}
		}

	case "Data":
		// Metric.Data oneof
		switch rng.Intn(5) {
		case 0:
			f.Set(reflect.ValueOf(&metricsv1.Metric_Gauge{Gauge: metricsv1.Gauge{}}))
		case 1:
			f.Set(reflect.ValueOf(&metricsv1.Metric_Sum{Sum: metricsv1.Sum{}}))
		case 2:
			f.Set(reflect.ValueOf(&metricsv1.Metric_Histogram{Histogram: metricsv1.Histogram{}}))
		case 3:
			f.Set(reflect.ValueOf(&metricsv1.Metric_ExponentialHistogram{ExponentialHistogram: metricsv1.ExponentialHistogram{}}))
		case 4:
			f.Set(reflect.ValueOf(&metricsv1.Metric_Summary{Summary: metricsv1.Summary{}}))
		}
	}
}

func randString(rng *rand.Rand) string {
	n := rng.Intn(20) + 1
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rng.Intn(26) + 'a')
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// 4. Varint encoding size boundary values
//    Inspired by gogoproto's TestVarintSize.
// ---------------------------------------------------------------------------

func TestVarintBoundaryValues(t *testing.T) {
	// Varint encoding boundaries: values that cross byte thresholds
	boundaries := []struct {
		name string
		val  uint32
	}{
		{"0", 0},
		{"1", 1},
		{"127 (1-byte max)", 127},
		{"128 (2-byte min)", 128},
		{"16383 (2-byte max)", 16383},
		{"16384 (3-byte min)", 16384},
		{"2097151 (3-byte max)", 2097151},
		{"2097152 (4-byte min)", 2097152},
		{"268435455 (4-byte max)", 268435455},
		{"268435456 (5-byte min)", 268435456},
		{"max uint32", math.MaxUint32},
	}

	t.Run("DroppedAttributesCount", func(t *testing.T) {
		for _, tc := range boundaries {
			t.Run(tc.name, func(t *testing.T) {
				r := resourcev1.Resource{DroppedAttributesCount: tc.val}
				b, err := r.Marshal()
				require.NoError(t, err)
				assert.Equal(t, r.Size(), len(b))

				var decoded resourcev1.Resource
				require.NoError(t, decoded.Unmarshal(b))
				assert.Equal(t, tc.val, decoded.DroppedAttributesCount)
			})
		}
	})

	t.Run("DroppedEventsCount", func(t *testing.T) {
		for _, tc := range boundaries {
			t.Run(tc.name, func(t *testing.T) {
				s := tracev1.Span{DroppedEventsCount: tc.val}
				b, err := s.Marshal()
				require.NoError(t, err)
				assert.Equal(t, s.Size(), len(b))

				var decoded tracev1.Span
				require.NoError(t, decoded.Unmarshal(b))
				assert.Equal(t, tc.val, decoded.DroppedEventsCount)
			})
		}
	})

	t.Run("EnumBoundaries", func(t *testing.T) {
		// Enum values that cross varint boundaries
		for _, sev := range []logsv1.SeverityNumber{0, 1, 9, 15, 24} {
			lr := logsv1.LogRecord{SeverityNumber: sev}
			b, err := lr.Marshal()
			require.NoError(t, err)
			assert.Equal(t, lr.Size(), len(b))

			var decoded logsv1.LogRecord
			require.NoError(t, decoded.Unmarshal(b))
			assert.Equal(t, sev, decoded.SeverityNumber)
		}
	})

	t.Run("Uint64VarintBoundaries", func(t *testing.T) {
		// uint64 varint boundaries for Count field
		vals := []uint64{0, 1, 127, 128, 16383, 16384, math.MaxUint32, math.MaxUint64}
		for _, v := range vals {
			dp := metricsv1.HistogramDataPoint{Count: v}
			b, err := dp.Marshal()
			require.NoError(t, err)
			assert.Equal(t, dp.Size(), len(b), "Count=%d", v)

			var decoded metricsv1.HistogramDataPoint
			require.NoError(t, decoded.Unmarshal(b))
			assert.Equal(t, v, decoded.Count)
		}
	})

	t.Run("Sint32VarintBoundaries", func(t *testing.T) {
		// sint32 uses zigzag encoding, test near boundaries
		vals := []int32{0, 1, -1, 63, -64, 64, -65, 8191, -8192, 8192, -8193, math.MaxInt32, math.MinInt32}
		for _, v := range vals {
			dp := metricsv1.ExponentialHistogramDataPoint{Scale: v}
			b, err := dp.Marshal()
			require.NoError(t, err)
			assert.Equal(t, dp.Size(), len(b), "Scale=%d", v)

			var decoded metricsv1.ExponentialHistogramDataPoint
			require.NoError(t, decoded.Unmarshal(b))
			assert.Equal(t, v, decoded.Scale)
		}
	})
}

// ---------------------------------------------------------------------------
// 5. Large packed repeated field handling
//    Inspired by gogoproto's TestIssue436.
// ---------------------------------------------------------------------------

func TestLargePackedRepeatedFields(t *testing.T) {
	t.Run("LargeBucketCounts", func(t *testing.T) {
		n := 10000
		counts := make([]uint64, n)
		for i := range counts {
			counts[i] = uint64(i)
		}
		dp := metricsv1.HistogramDataPoint{
			Count:        uint64(n),
			BucketCounts: counts,
		}

		size := dp.Size()
		b, err := dp.Marshal()
		require.NoError(t, err)
		require.Equal(t, size, len(b))

		var decoded metricsv1.HistogramDataPoint
		require.NoError(t, decoded.Unmarshal(b))
		require.Len(t, decoded.BucketCounts, n)
		assert.Equal(t, counts, decoded.BucketCounts)
	})

	t.Run("LargeExplicitBounds", func(t *testing.T) {
		n := 10000
		bounds := make([]float64, n)
		for i := range bounds {
			bounds[i] = float64(i) * 0.1
		}
		dp := metricsv1.HistogramDataPoint{
			ExplicitBounds: bounds,
		}

		size := dp.Size()
		b, err := dp.Marshal()
		require.NoError(t, err)
		require.Equal(t, size, len(b))

		var decoded metricsv1.HistogramDataPoint
		require.NoError(t, decoded.Unmarshal(b))
		require.Len(t, decoded.ExplicitBounds, n)
		assert.Equal(t, bounds, decoded.ExplicitBounds)
	})

	t.Run("LargePackedVarint", func(t *testing.T) {
		n := 10000
		indices := make([]int32, n)
		for i := range indices {
			indices[i] = int32(i)
		}
		s := profilesv1.Stack{LocationIndices: indices}

		size := s.Size()
		b, err := s.Marshal()
		require.NoError(t, err)
		require.Equal(t, size, len(b))

		var decoded profilesv1.Stack
		require.NoError(t, decoded.Unmarshal(b))
		require.Len(t, decoded.LocationIndices, n)
		assert.Equal(t, indices, decoded.LocationIndices)
	})

	t.Run("LargeExponentialBucketCounts", func(t *testing.T) {
		n := 10000
		counts := make([]uint64, n)
		for i := range counts {
			counts[i] = uint64(i * 100)
		}
		bk := metricsv1.ExponentialHistogramDataPoint_Buckets{
			Offset:       5,
			BucketCounts: counts,
		}

		size := bk.Size()
		b, err := bk.Marshal()
		require.NoError(t, err)
		require.Equal(t, size, len(b))

		var decoded metricsv1.ExponentialHistogramDataPoint_Buckets
		require.NoError(t, decoded.Unmarshal(b))
		require.Len(t, decoded.BucketCounts, n)
	})

	t.Run("LargeRepeatedMessages", func(t *testing.T) {
		n := 5000
		attrs := make([]commonv1.KeyValue, n)
		for i := range attrs {
			attrs[i] = commonv1.KeyValue{
				Key:   "k",
				Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: int64(i)}},
			}
		}
		r := resourcev1.Resource{Attributes: attrs}

		size := r.Size()
		b, err := r.Marshal()
		require.NoError(t, err)
		require.Equal(t, size, len(b))

		var decoded resourcev1.Resource
		require.NoError(t, decoded.Unmarshal(b))
		require.Len(t, decoded.Attributes, n)
	})

	t.Run("PackedFieldCapNotOverAllocated", func(t *testing.T) {
		n := 1000
		counts := make([]uint64, n)
		for i := range counts {
			counts[i] = uint64(i)
		}
		dp := metricsv1.HistogramDataPoint{BucketCounts: counts}

		b, err := dp.Marshal()
		require.NoError(t, err)

		var decoded metricsv1.HistogramDataPoint
		require.NoError(t, decoded.Unmarshal(b))
		require.Len(t, decoded.BucketCounts, n)
		// Cap should not be more than 2x the length
		assert.LessOrEqual(t, cap(decoded.BucketCounts), n*2,
			"packed field cap (%d) is more than 2x length (%d)", cap(decoded.BucketCounts), n)
	})
}

// ---------------------------------------------------------------------------
// 6. Fields in non-canonical order
//    Inspired by gogoproto's fuzz tests discovering field-order bugs.
// ---------------------------------------------------------------------------

func TestNonCanonicalFieldOrder(t *testing.T) {
	t.Run("Resource_ReverseOrder", func(t *testing.T) {
		// Build Resource wire bytes with field 2 before field 1
		var kv []byte
		kv = protowire.AppendTag(kv, 1, protowire.BytesType)
		kv = protowire.AppendString(kv, "k1")

		var wire []byte
		// Field 2 (dropped_attributes_count) first
		wire = protowire.AppendTag(wire, 2, protowire.VarintType)
		wire = protowire.AppendVarint(wire, 42)
		// Field 1 (attributes) second
		wire = protowire.AppendTag(wire, 1, protowire.BytesType)
		wire = protowire.AppendBytes(wire, kv)

		var r resourcev1.Resource
		require.NoError(t, r.Unmarshal(wire))
		assert.Equal(t, uint32(42), r.DroppedAttributesCount)
		require.Len(t, r.Attributes, 1)
		assert.Equal(t, "k1", r.Attributes[0].Key)
	})

	t.Run("Span_ReverseOrder", func(t *testing.T) {
		// Build Span with fields in reverse order
		var wire []byte
		// Status (field 15) first
		var status []byte
		status = protowire.AppendTag(status, 2, protowire.BytesType)
		status = protowire.AppendString(status, "ok")
		status = protowire.AppendTag(status, 3, protowire.VarintType)
		status = protowire.AppendVarint(status, 1)
		wire = protowire.AppendTag(wire, 15, protowire.BytesType)
		wire = protowire.AppendBytes(wire, status)

		// Kind (field 6) second
		wire = protowire.AppendTag(wire, 6, protowire.VarintType)
		wire = protowire.AppendVarint(wire, uint64(tracev1.Span_SpanKind_SPAN_KIND_SERVER))

		// Name (field 5) third
		wire = protowire.AppendTag(wire, 5, protowire.BytesType)
		wire = protowire.AppendString(wire, "reverse")

		// TraceId (field 1) last
		wire = protowire.AppendTag(wire, 1, protowire.BytesType)
		wire = protowire.AppendBytes(wire, make([]byte, 16))

		var s tracev1.Span
		require.NoError(t, s.Unmarshal(wire))
		assert.Equal(t, "reverse", s.Name)
		assert.Equal(t, tracev1.Span_SpanKind_SPAN_KIND_SERVER, s.Kind)
		assert.Equal(t, tracev1.Status_StatusCode_STATUS_CODE_OK, s.Status.Code)
	})

	t.Run("HistogramDataPoint_InterleavedKnownAndUnknown", func(t *testing.T) {
		// Known field, unknown field, known field, unknown field
		var wire []byte

		// Count (field 4) = fixed64
		wire = protowire.AppendTag(wire, 4, protowire.Fixed64Type)
		wire = protowire.AppendFixed64(wire, 100)

		// Unknown field 200 = varint
		wire = protowire.AppendTag(wire, 200, protowire.VarintType)
		wire = protowire.AppendVarint(wire, 999)

		// Packed bucket_counts (field 6)
		var packed []byte
		packed = protowire.AppendFixed64(packed, 10)
		packed = protowire.AppendFixed64(packed, 20)
		wire = protowire.AppendTag(wire, 6, protowire.BytesType)
		wire = protowire.AppendBytes(wire, packed)

		// Unknown field 201 = fixed64
		wire = protowire.AppendTag(wire, 201, protowire.Fixed64Type)
		wire = protowire.AppendFixed64(wire, 0xDEADBEEF)

		// Sum (field 5) = double (fixed64)
		sum := 42.5
		wire = protowire.AppendTag(wire, 5, protowire.Fixed64Type)
		wire = protowire.AppendFixed64(wire, math.Float64bits(sum))

		var dp metricsv1.HistogramDataPoint
		require.NoError(t, dp.Unmarshal(wire))
		assert.Equal(t, uint64(100), dp.Count)
		assert.Equal(t, []uint64{10, 20}, dp.BucketCounts)
		require.NotNil(t, dp.Sum)
		assert.Equal(t, 42.5, *dp.Sum)
	})

	t.Run("LogRecord_FieldsScattered", func(t *testing.T) {
		// Scatter fields across the wire in non-sequential order
		var wire []byte

		// SeverityText (field 3) first
		wire = protowire.AppendTag(wire, 3, protowire.BytesType)
		wire = protowire.AppendString(wire, "WARN")

		// TimeUnixNano (field 1) second
		wire = protowire.AppendTag(wire, 1, protowire.Fixed64Type)
		wire = protowire.AppendFixed64(wire, 5000)

		// SeverityNumber (field 2) third
		wire = protowire.AppendTag(wire, 2, protowire.VarintType)
		wire = protowire.AppendVarint(wire, uint64(logsv1.SeverityNumber_SEVERITY_NUMBER_WARN))

		// Flags (field 8) fourth
		wire = protowire.AppendTag(wire, 8, protowire.Fixed32Type)
		wire = protowire.AppendFixed32(wire, 0x01)

		var lr logsv1.LogRecord
		require.NoError(t, lr.Unmarshal(wire))
		assert.Equal(t, "WARN", lr.SeverityText)
		assert.Equal(t, uint64(5000), lr.TimeUnixNano)
		assert.Equal(t, logsv1.SeverityNumber_SEVERITY_NUMBER_WARN, lr.SeverityNumber)
		assert.Equal(t, uint32(1), lr.Flags)
	})

	t.Run("Metric_OneofAfterOtherFields", func(t *testing.T) {
		// Build a Metric where the oneof data field comes before the name
		var gauge []byte
		// Empty Gauge message (just the wrapper, no data points)

		var wire []byte
		// Data (Gauge = field 5) first
		wire = protowire.AppendTag(wire, 5, protowire.BytesType)
		wire = protowire.AppendBytes(wire, gauge) // empty gauge

		// Name (field 1) second
		wire = protowire.AppendTag(wire, 1, protowire.BytesType)
		wire = protowire.AppendString(wire, "my.metric")

		// Unit (field 3) third
		wire = protowire.AppendTag(wire, 3, protowire.BytesType)
		wire = protowire.AppendString(wire, "ms")

		var m metricsv1.Metric
		require.NoError(t, m.Unmarshal(wire))
		assert.Equal(t, "my.metric", m.Name)
		assert.Equal(t, "ms", m.Unit)
		_, ok := m.Data.(*metricsv1.Metric_Gauge)
		assert.True(t, ok, "expected Gauge oneof variant")
	})
}

// ---------------------------------------------------------------------------
// 7. MarshalToSizedBuffer with undersized buffer
// ---------------------------------------------------------------------------

func TestMarshalToSizedBufferUndersized(t *testing.T) {
	r := &resourcev1.Resource{
		Attributes: []commonv1.KeyValue{
			{Key: "k1", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v1"}}},
			{Key: "k2", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 42}}},
		},
		DroppedAttributesCount: 5,
	}
	size := r.Size()
	require.Greater(t, size, 10)

	// Undersized buffer should panic (reverse-write goes out of bounds)
	assert.Panics(t, func() {
		tiny := make([]byte, 1)
		r.MarshalToSizedBuffer(tiny) //nolint:errcheck // we expect a panic, not an error
	}, "MarshalToSizedBuffer with undersized buffer should panic")
}
