package bench

import (
	"math"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"

	metricsv1 "wiresmith/gen/opentelemetry/proto/metrics/v1"
)

// Focused micro-benchmarks for packed-scalar unmarshal hot loops. These
// isolate the path changed by wiresmith-pgw so we can see whether the
// inline-decoder rewrite moves the needle on the actual code we touched,
// independent of any layout flux in adjacent macro-benchmarks.

var (
	packedFixed64_64elem  = makePackedFixed64HistogramWire(64)
	packedFixed64_256elem = makePackedFixed64HistogramWire(256)
)

// makePackedFixed64HistogramWire constructs a wire payload with a
// HistogramDataPoint carrying `n` packed fixed64 bucket_counts and `n`
// packed double explicit_bounds (also fixed64 on the wire). No other fields,
// so the entire unmarshal is dominated by the two packed-fixed64 hot loops.
func makePackedFixed64HistogramWire(n int) []byte {
	counts := make([]byte, 0, n*8)
	for i := 0; i < n; i++ {
		counts = protowire.AppendFixed64(counts, uint64(i+1))
	}
	bounds := make([]byte, 0, n*8)
	for i := 0; i < n; i++ {
		bounds = protowire.AppendFixed64(bounds, math.Float64bits(float64(i)+0.5))
	}
	var wire []byte
	wire = protowire.AppendTag(wire, 6, protowire.BytesType) // bucket_counts
	wire = protowire.AppendBytes(wire, counts)
	wire = protowire.AppendTag(wire, 7, protowire.BytesType) // explicit_bounds
	wire = protowire.AppendBytes(wire, bounds)
	return wire
}

func BenchmarkUnmarshalPackedFixed64_64_Ours(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var dp metricsv1.HistogramDataPoint
		_ = dp.Unmarshal(packedFixed64_64elem)
	}
}

func BenchmarkUnmarshalPackedFixed64_256_Ours(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var dp metricsv1.HistogramDataPoint
		_ = dp.Unmarshal(packedFixed64_256elem)
	}
}

// Packed-varint focused micro: ExpHistogram Buckets.bucket_counts (field 2,
// repeated uint64 -> varint). Mirrors the fixed64 micro but for the varint
// inner loop (different code path in compiler/types/repeated.go).
func makePackedVarintBucketsWire(n int) []byte {
	counts := make([]byte, 0, n*4)
	for i := 0; i < n; i++ {
		counts = protowire.AppendVarint(counts, uint64(i*97+1))
	}
	var wire []byte
	wire = protowire.AppendTag(wire, 2, protowire.BytesType) // bucket_counts
	wire = protowire.AppendBytes(wire, counts)
	return wire
}

var (
	packedVarint_64elem  = makePackedVarintBucketsWire(64)
	packedVarint_256elem = makePackedVarintBucketsWire(256)
)

func BenchmarkUnmarshalPackedVarint_64_Ours(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var dp metricsv1.ExponentialHistogramDataPoint_Buckets
		_ = dp.Unmarshal(packedVarint_64elem)
	}
}

func BenchmarkUnmarshalPackedVarint_256_Ours(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var dp metricsv1.ExponentialHistogramDataPoint_Buckets
		_ = dp.Unmarshal(packedVarint_256elem)
	}
}
