# Benchmark Architecture

Comparative benchmarks for protobuf serialization across four implementations:
- **Ours** (`gen/otlp/`): Value-type structs, reverse-write marshaling, no reflection
- **Official** (`go.opentelemetry.io/proto/otlp`): Standard `google.golang.org/protobuf`
- **VTProto** (`gen/vtpb/`): `planetscale/vtprotobuf` optimized codegen on top of standard proto
- **GogoProto** (`gen/gogopb/`): `github.com/gogo/protobuf` legacy fast protobuf (unmaintained)

## Shared Input Strategy

All benchmarks use identical wire-format bytes generated once via official proto in `inputs_test.go`. Each library unmarshals from these canonical bytes during setup (for marshal/size benchmarks) or uses them directly (for unmarshal benchmarks). This guarantees apples-to-apples comparison with a single source of truth for test data. Exception: ProfilesData uses vtproto types for canonical byte generation because the official proto package doesn't include profiles.

## File Layout

- `inputs_test.go` — Canonical wire bytes generation (single source of truth, uses official proto + vtproto for profiles)
- `ours_bench_test.go` — wiresmith benchmarks
- `official_bench_test.go` — google.golang.org/protobuf benchmarks
- `vtproto_bench_test.go` — vtprotobuf benchmarks
- `gogoproto_bench_test.go` — gogoproto benchmarks
- `map_bench_test.go` — Map field benchmarks (all four implementations)
- `../proto/bench/maps.proto` — Proto definition for map benchmarks

## Running

```bash
GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn go test ./bench/ -bench=. -benchmem -count=5
```

The env var suppresses panics from multiple proto libraries registering the same `.proto` file descriptors.

## Benchmark Variants

Each library is benchmarked on:
- `MarshalTraces` / `UnmarshalTraces` — 100 spans with attributes, events, status
- `MarshalHistogram` / `UnmarshalHistogram` — 50 histogram data points with bounds and buckets
- `MarshalSingleSpan` / `UnmarshalSingleSpan` — single span message
- `MarshalLogs` / `UnmarshalLogs` — 50 log records with severity, body, attributes, trace context
- `MarshalGauge` / `UnmarshalGauge` — 50 gauge number data points (double values)
- `MarshalSum` / `UnmarshalSum` — 50 monotonic cumulative sum data points (int values)
- `MarshalExpHistogram` / `UnmarshalExpHistogram` — 50 exponential histogram data points with positive/negative buckets
- `MarshalSummary` / `UnmarshalSummary` — 50 summary data points with quantile values
- `MarshalProfiles` / `UnmarshalProfiles` — profile with 50 samples, dictionary with functions/locations/mappings/stacks (Ours/VTProto/GogoProto only, no official proto)
- `SizeTraces` — size computation for 100 spans
- `MarshalMap` / `UnmarshalMap` / `SizeMap` — 100-entry maps (string, int64, message values)

## Generated Code

All generated code lives under `gen/`:
- `gen/otlp/` — Our generated code (`go run ./cmd/wiresmith/`)
- `gen/vtpb/` — vtproto (`protoc` + `protoc-gen-go` + `protoc-gen-go-vtproto`)
- `gen/gogopb/` — gogoproto (`protoc` + `protoc-gen-gogofast`)
- `gen/bench/` — Map benchmark code (all four implementations from `proto/bench/maps.proto`)

Regenerate all at once:
```bash
make generate
```

Requires: `protoc`, `protoc-gen-go`, `protoc-gen-go-vtproto`, `protoc-gen-gogofast`

## Map benchmark results (Apple M4 Pro)

100-entry maps with string, int64, and message values:

| Operation | Ours | VTProto | GogoProto | Official |
|-----------|-----:|--------:|----------:|---------:|
| Marshal ns/op | 5,800 | 8,560 | 8,550 | 33,400 |
| Marshal B/op | 9,472 | 9,472 | 9,472 | 22,272 |
| Marshal allocs | 1 | 1 | 1 | 1,001 |
| Unmarshal ns/op | 14,840 | 16,870 | 16,730 | 44,500 |
| Unmarshal B/op | 25,704 | 39,368 | 37,768 | 51,304 |
| Unmarshal allocs | 512 | 633 | 633 | 1,518 |
| Size ns/op | 1,700 | 1,600 | — | — |

Marshal is ~32% faster than vtproto/gogoproto due to reverse-write. Unmarshal is ~12% faster with 35% less memory due to pre-scan map pre-allocation and value-type message values (avoids per-entry heap allocation at the cost of larger map entries during growth — compensated by the pre-allocation eliminating rehashing).

## Gogoproto and `optional`

Proto3 `optional` fields (`optional double sum`, `min`, `max`) are stripped before gogoproto generation because `protoc-gen-gogofast` predates proto3 optional support. Those fields become plain `double` (zero-value semantics instead of nullable `*float64`). This is acceptable for benchmarking serialization speed and is handled automatically by the generation script.
