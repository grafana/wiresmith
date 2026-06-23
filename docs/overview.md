# Overview

wiresmith is a custom protobuf compiler that generates high-performance Go code from OpenTelemetry `.proto` files, built on `google.golang.org/protobuf/encoding/protowire` and reverse-write marshaling.

## Why

The official Go protobuf runtime uses reflection-based marshaling, which adds overhead that matters at scale. Existing alternatives like [vtprotobuf](https://github.com/planetscale/vtprotobuf) and [gogoproto](https://github.com/gogo/protobuf) generate faster code but still use pointer-based message fields, trading heap allocations for indirection on every access.

wiresmith takes a different approach: **value-type structs with generated marshal/unmarshal code and zero reflection**. The result is fewer allocations, better cache locality, and faster serialization across the board.

## Advantages

This section is the feature tour; see [design.md](design.md) for the engineering rationale behind each decision and the deliberate limitations it implies.

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

Unmarshal merges into a non-empty message (repeated fields append, map entries last-write-wins -- gogo parity), and the prealloc reuses a caller-provided backing array when it already has room (pooled messages keep their buffers across decodes). Call `Reset()` first for replace semantics. For callers decoding into a reused/pooled message -- where the prealloc never fires and the scan is pure overhead -- `UnmarshalNoPrescan` skips the top-level scan; see [generated-api.md](generated-api.md#pre-scan-and-unmarshalnoprescan).

### Packed scalar exact-capacity allocation

For fixed-size packed fields (`uint64`, `float64`), we compute `len(data)/8` and allocate once.

## Supported proto3 features

Messages, nested messages, enums (top-level and nested), oneof, optional, repeated (packed + non-packed), maps, reserved fields, cross-file imports, fully-qualified type references.

Scalar types: `string`, `bool`, `int32`, `int64`, `uint32`, `uint64`, `sint32`, `sint64`, `float`, `double`, `bytes`, `fixed32`, `fixed64`, `sfixed32`, `sfixed64`.

Map keys: all scalar types except `float`, `double`, and `bytes`. Map values: all scalars, enums, and messages.

**gRPC services.** A `.proto` that declares `service` blocks gets `<name>_grpc.pb.go` stubs -- unary and streaming RPCs -- produced by a vendored copy of `protoc-gen-go-grpc` that reuses wiresmith's generated message types. No separate `protoc-gen-go-grpc` invocation is needed. See [buf.md](buf.md).

**Well-known types.** `google.protobuf.Timestamp` and `Duration` are supported via the `stdtime` / `stdduration` field options (value-type substitution to `time.Time` / `time.Duration` -- see [extensions.md](extensions.md)). `google.protobuf.Any` is supported directly: a field referencing it resolves to wiresmith's shipped [`types/known/anypb`](../types/known/anypb) package, which carries wiresmith's wire methods -- see [generated-api.md](generated-api.md#googleprotobufany).

Not supported (not needed for OTel protos): extensions, other well-known types (`Empty`, `Struct`, `FieldMask`, wrappers), and proto2.
