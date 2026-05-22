# Comparison with vtproto, gogoproto, and the official runtime

wiresmith competes most directly with [vtprotobuf](https://github.com/planetscale/vtprotobuf) and [gogoproto](https://github.com/gogo/protobuf) — both generate Go-specific marshal/unmarshal code on top of (or alongside) the official `google.golang.org/protobuf` runtime. The four implementations differ on field shape, allocation strategy, and reflection support; the table below summarizes the differences that matter at adoption time.

## Feature matrix

| Feature                                | wiresmith            | vtproto              | gogoproto            | Official             |
|----------------------------------------|----------------------|----------------------|----------------------|----------------------|
| Singular message field shape           | Value (`T`)          | Pointer (`*T`)       | Pointer (`*T`)       | Pointer (`*T`)       |
| Repeated message field shape           | Slice of value (`[]T`) | Slice of pointer (`[]*T`) | Slice of pointer (`[]*T`) | Slice of pointer (`[]*T`) |
| Reverse-write marshaling               | Yes                  | Yes                  | No                   | No                   |
| Pre-computed tag byte literals         | Yes                  | Yes                  | Yes                  | No (runtime)         |
| Runtime reflection on hot path         | No                   | No                   | No                   | Yes                  |
| Pre-scan pre-allocation of slices      | Yes                  | No                   | No                   | No                   |
| Exact-capacity packed scalar alloc     | Yes                  | Partial              | Partial              | No                   |
| Field presence bitmap (`Has<F>()`)     | Yes                  | No                   | Via `nullable=false` | Via proto2 / `optional` |
| Custom pointer-shape opt-in            | `(wiresmith.options.pointer)` | n/a         | `gogoproto.nullable` | n/a                  |
| Unknown field preservation             | No (discarded)       | Yes                  | Yes                  | Yes                  |
| Deterministic marshaling               | No                   | Yes                  | Yes                  | Yes                  |
| Field-level `protoreflect` Range/Get/Set | Panics             | Yes                  | Yes (via official)   | Yes                  |
| `protojson` / `prototext` / `proto.Clone` / `proto.Merge` | No (panics) | Yes (via official)   | Yes (via official)   | Yes                  |
| Maintained                             | Yes                  | Yes                  | No (deprecated)      | Yes                  |

See [design.md](design.md) for the rationale behind each row in the wiresmith column, and [generated-api.md](generated-api.md) for what the panicking reflection paths look like.

## Benchmarks

Measured on Apple M4 Pro, 10 iterations per library, on a full trace payload (100 spans with attributes, events, links). Bytes are generated once by the official runtime and consumed by all four libraries from the same wire image — see [`bench/AGENTS.md`](../bench/AGENTS.md) for the full methodology. Reproduce with `make bench-compare` (or run `bench/` directly; both wrap `go test ./bench/ -bench=...`).

### Throughput (us/op, lower is better)

| Benchmark         | Ours        | Official            | VTProto             | GogoProto           |
|-------------------|-------------|---------------------|---------------------|---------------------|
| MarshalTraces     | **6.4**     | 46.2 (+618%)        | 7.7 (+20%)          | 7.7 (+19%)          |
| UnmarshalTraces   | **33.4**    | 70.1 (+110%)        | 38.9 (+16%)         | 36.4 (+9%)          |
| SizeTraces        | **1.4**     | 17.0 (+1076%)       | 2.2 (+52%)          | 2.0 (+40%)          |
| **Geomean, full `bench/` suite** | **1.96** | 8.11 (+314%)        | 2.33 (+19%)         | 2.26 (+15%)         |

The geomean row aggregates every Marshal/Unmarshal/Size variant in [`bench/`](../bench/AGENTS.md#benchmark-variants) — not just the three trace rows above — so its value can sit below the slowest visible row when the broader suite includes smaller payloads such as `MarshalSingleSpan`.

### Memory (B/op, lower is better)

| Benchmark            | Ours        | Official            | VTProto             | GogoProto           |
|----------------------|-------------|---------------------|---------------------|---------------------|
| UnmarshalTraces      | **78.5 KiB**| 112.2 KiB (+43%)    | 112.1 KiB (+43%)    | 102.7 KiB (+31%)    |
| UnmarshalSingleSpan  | **528 B**   | 1120 B (+112%)      | 832 B (+58%)        | 736 B (+39%)        |

### Allocations (allocs/op, lower is better)

| Benchmark            | Ours        | Official            | VTProto             | GogoProto           |
|----------------------|-------------|---------------------|---------------------|---------------------|
| UnmarshalTraces      | **1,609**   | 2,523 (+57%)        | 2,522 (+57%)        | 2,522 (+57%)        |
| UnmarshalSingleSpan  | **16**      | 25 (+56%)           | 24 (+50%)           | 24 (+50%)           |

### Map fields (100-entry maps with string, int64, and message values)

| Operation           | Ours   | VTProto | GogoProto | Official |
|---------------------|-------:|--------:|----------:|---------:|
| Marshal ns/op       | 5,480  | 8,360   | 8,440     | 31,670   |
| Marshal B/op        | 9,472  | 9,472   | 9,472     | 22,272   |
| Marshal allocs      | 1      | 1       | 1         | 1,001    |
| Unmarshal ns/op     | 10,820 | 16,860  | 16,850    | 43,050   |
| Unmarshal B/op      | 25,704 | 39,368  | 37,768    | 51,304   |
| Unmarshal allocs    | 512    | 633     | 633       | 1,518    |
| Size ns/op          | 1,684  | 1,634   | —         | —        |

Marshal is ~34% faster than vtproto/gogoproto on maps due to reverse-write; unmarshal is ~36% faster with ~35% less memory thanks to inline map-entry decoding, pre-scan map pre-allocation, and value-type message values.
