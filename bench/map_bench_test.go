package bench

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	oursmaps "wiresmith/gen/basic/maps/v1"
	gogomaps "wiresmith/gen/bench/gogopb"
	officialmaps "wiresmith/gen/bench/official"
	vtmaps "wiresmith/gen/bench/vtpb"
)

var mapBytes []byte

func init() {
	msg := buildCanonicalMapBench()
	var err error
	mapBytes, err = proto.Marshal(msg)
	if err != nil {
		panic(fmt.Sprintf("marshal canonical map bench: %v", err))
	}
}

func buildCanonicalMapBench() *officialmaps.MapBench {
	n := 100
	sm := make(map[string]string, n)
	im := make(map[int64]int64, n)
	mm := make(map[string]*officialmaps.Inner, n)
	for i := range n {
		k := fmt.Sprintf("key-%04d", i)
		sm[k] = fmt.Sprintf("value-%04d-padding-for-realistic-size", i)
		im[int64(i)] = int64(i * i)
		mm[k] = &officialmaps.Inner{
			Name:  fmt.Sprintf("inner-%04d", i),
			Value: int64(i * 100),
			Data:  []byte{byte(i), byte(i >> 8), byte(i >> 16)},
		}
	}
	return &officialmaps.MapBench{
		StringMap:  sm,
		IntMap:     im,
		MessageMap: mm,
	}
}

// --- Marshal ---

func BenchmarkMarshalMap_Ours(b *testing.B) {
	var data oursmaps.MapBench
	if err := data.Unmarshal(mapBytes); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

func BenchmarkMarshalMap_Official(b *testing.B) {
	var data officialmaps.MapBench
	if err := proto.Unmarshal(mapBytes, &data); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = proto.Marshal(&data)
	}
}

func BenchmarkMarshalMap_VTProto(b *testing.B) {
	var data vtmaps.MapBench
	if err := data.UnmarshalVT(mapBytes); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.MarshalVT()
	}
}

func BenchmarkMarshalMap_GogoProto(b *testing.B) {
	var data gogomaps.MapBench
	if err := data.Unmarshal(mapBytes); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = data.Marshal()
	}
}

// --- Unmarshal ---

func BenchmarkUnmarshalMap_Ours(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out oursmaps.MapBench
		_ = out.Unmarshal(mapBytes)
	}
}

func BenchmarkUnmarshalMap_Official(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out officialmaps.MapBench
		_ = proto.Unmarshal(mapBytes, &out)
	}
}

func BenchmarkUnmarshalMap_VTProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out vtmaps.MapBench
		_ = out.UnmarshalVT(mapBytes)
	}
}

func BenchmarkUnmarshalMap_GogoProto(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var out gogomaps.MapBench
		_ = out.Unmarshal(mapBytes)
	}
}

// --- Size ---

func BenchmarkSizeMap_Ours(b *testing.B) {
	var data oursmaps.MapBench
	if err := data.Unmarshal(mapBytes); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = data.Size()
	}
}

func BenchmarkSizeMap_VTProto(b *testing.B) {
	var data vtmaps.MapBench
	if err := data.UnmarshalVT(mapBytes); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = data.SizeVT()
	}
}
