# grafana-protoc

Custom protobuf compiler that generates high-performance Go code from OpenTelemetry `.proto` files. Built on `google.golang.org/protobuf/encoding/protowire` and reverse-write marshaling.

## Why

The official Go protobuf runtime uses reflection-based marshaling, which adds overhead that matters at scale. Existing alternatives like [vtprotobuf](https://github.com/planetscale/vtprotobuf) and [gogoproto](https://github.com/gogo/protobuf) generate faster code but still use pointer-based message fields, trading heap allocations for indirection on every access.

grafana-protoc takes a different approach: **value-type structs with generated marshal/unmarshal code and zero reflection**. The result is fewer allocations, better cache locality, and faster serialization across the board.

## Benchmarks

Measured on Apple M4 Pro, 10 iterations per library. Full trace payload (100 spans with attributes, events, links).

### Throughput (sec/op, lower is better)

| Benchmark | Ours | Official | VTProto | GogoProto |
|-----------|------|----------|---------|-----------|
| MarshalTraces | **6.4 us** | 46.2 us (+618%) | 7.7 us (+20%) | 7.7 us (+19%) |
| UnmarshalTraces | **33.4 us** | 70.1 us (+110%) | 38.9 us (+16%) | 36.4 us (+9%) |
| SizeTraces | **1.4 us** | 17.0 us (+1076%) | 2.2 us (+52%) | 2.0 us (+40%) |
| **Geometric mean** | **1.96 us** | 8.11 us (+314%) | 2.33 us (+19%) | 2.26 us (+15%) |

### Memory (B/op, lower is better)

| Benchmark | Ours | Official | VTProto | GogoProto |
|-----------|------|----------|---------|-----------|
| UnmarshalTraces | **78.5 KiB** | 112.2 KiB (+43%) | 112.1 KiB (+43%) | 102.7 KiB (+31%) |
| UnmarshalSingleSpan | **528 B** | 1120 B (+112%) | 832 B (+58%) | 736 B (+39%) |

### Allocations (allocs/op, lower is better)

| Benchmark | Ours | Official | VTProto | GogoProto |
|-----------|------|----------|---------|-----------|
| UnmarshalTraces | **1,609** | 2,523 (+57%) | 2,522 (+57%) | 2,522 (+57%) |
| UnmarshalSingleSpan | **16** | 25 (+56%) | 24 (+50%) | 24 (+50%) |

## Advantages

### Value-type struct fields

Message fields are value types (`Resource Resource`, not `*Resource`). This is the single biggest differentiator:

- **Fewer allocations** -- no `new(Span)` per element; the struct lives inline in the slice backing array.
- **Better cache locality** -- iterating `[]Span` reads contiguous memory instead of chasing pointers through `[]*Span`.
- Trade-off: slice growth copies larger elements. We mitigate this with pre-scan pre-allocation (see below).

### Reverse-write marshaling

`MarshalToSizedBuffer` writes from the end of the buffer backwards. Nested messages need their size as a varint prefix -- forward-write must compute size first, then write. Reverse-write writes the child, then prepends the length in one pass instead of two.

Same technique vtprotobuf uses, but combined with value types we avoid a pointer dereference per field.

### Pre-computed tag bytes

Tags are emitted as byte literals (`dAtA[i] = 0x0a`) at codegen time, not computed at runtime.

### No reflection

All marshal/unmarshal/size code is directly generated. The official protobuf library uses `protoreflect` interfaces at runtime, which is why it's 3-7x slower.

### Pre-scan pre-allocation

During unmarshal, a lightweight pre-scan counts repeated elements before allocating slices at exact capacity. This eliminates growth waste entirely:

- VTProto/GogoProto use pointer slices, so each wasted slot costs 8 bytes.
- Our value-type slices would waste `sizeof(struct)` per slot (e.g. 256 bytes for `Span`).
- The pre-scan makes this a non-issue: exact capacity, zero growth waste.

Net result: value-type cache locality benefits without the memory penalty -- 30-40% less memory than VTProto on unmarshal.

### Packed scalar exact-capacity allocation

For fixed-size packed fields (`uint64`, `float64`), we compute `len(data)/8` and allocate once.

## Supported proto3 features

Messages, nested messages, enums (top-level and nested), oneof, optional, repeated (packed + non-packed), reserved fields, cross-file imports, fully-qualified type references.

Scalar types: `string`, `bool`, `int32`, `int64`, `uint32`, `uint64`, `sint32`, `double`, `bytes`, `fixed32`, `fixed64`, `sfixed64`.

Not supported (not needed for OTel protos): maps, services/RPCs, extensions, well-known types, proto2.

## Usage

```
make generate    # regenerate Go code from .proto files
make build       # build all packages
make test        # run round-trip correctness tests
make bench       # run comparative benchmarks
```

See `Makefile` for all targets.
