# Design decisions and tradeoffs

The choices below shape every line of generated code. Most exist for performance and would change if wiresmith targeted a different use case (general-purpose protobuf runtime, broad proto2/proto3 coverage, schema evolution). They are documented so users can decide whether wiresmith fits their workload before adopting it.

## Decisions

- **Value-type struct fields.** Message fields are value types (`Resource Resource`, not `*Resource`). Proto3 `optional` fields use pointers (`*float64`, `*MessageType`), which is also what enables recursive message definitions via `optional` self-references. The fewer-allocations win comes at the cost of a larger element size in slices — mitigated by the pre-scan pre-allocation below.

- **Reverse-write marshaling.** `MarshalToSizedBuffer` writes from the end of the buffer backwards, eliminating the double size computation that forward-write needs for nested messages. Same technique vtprotobuf uses; combined with value types, wiresmith also avoids a pointer dereference per field.

- **Pre-computed tag bytes.** Tag bytes are computed at codegen time and emitted as byte literals (`dAtA[i] = 0x0a`) rather than computed at marshal time.

- **Packed repeated scalars.** Repeated numeric fields are emitted with packed encoding (the proto3 default). Unmarshal handles both packed and unpacked layouts for compatibility with peers that emit either.

- **No reflection.** All marshal/unmarshal/size code is directly generated; no runtime reflection on the hot path. The official runtime's 3–7× slowdown comes from its `protoreflect` interface dispatch.

- **Pre-scan pre-allocation.** A lightweight pre-scan during unmarshal counts repeated elements before allocating slices at exact capacity. With value-type fields, exact capacity matters more than for pointer-based libraries — a wasted slot in `[]Span` costs `sizeof(Span)` rather than 8 bytes. The pre-scan only pays on a fresh destination; for callers decoding into reused/pooled messages (where the prealloc guard never fires) wiresmith emits `UnmarshalNoPrescan`, which skips the top-level scan while preserving nested pre-scans. See [generated-api.md](generated-api.md#pre-scan-and-unmarshalnoprescan).

- **Merge semantics: Unmarshal appends to repeated fields (gogo parity).** Decoding into a non-empty message appends decoded elements after the existing ones and merges map entries (last-write-wins per key), regardless of payload size — the pre-scan prealloc grows the slice to `len+count` preserving existing elements, and reuses the existing backing array when it already has room (the pooled-message pattern: a message reset to length zero from a `sync.Pool` keeps its buffers across decodes). Callers that want replace semantics call `Reset()` first, exactly as with gogoproto. Holding references to a previous decode's slices while re-unmarshaling into the same pooled message observes them being overwritten once the pool reuses the arrays — that is the pooling contract, not a wiresmith-specific hazard.

- **bufbuild/protocompile for parsing.** Proto parsing and type resolution reuse the buf compiler; wiresmith does not maintain its own parser.

- **Field presence bitmap.** Singular non-optional, non-oneof fields get a `XXX_fieldsPresent` bitmap (`[N]uint64`) populated during `Unmarshal`. Generated `Has<Field>()` methods read it so callers can distinguish "field absent" from "field set to zero value". The bitmap is not serialized. Repeated, map, optional, and oneof fields are excluded — they already carry presence via `nil` slice / pointer / interface. **Optional bytes** are a special case: `[]byte(nil)` means absent, non-nil means present (`[]byte{}` is "present but empty"). This matches gogoproto and the official runtime; the distinction is via nil-check, not the bitmap. The `XXX_` prefix is deliberate: gogoproto's reflection-based `proto.Equal`/`Clone`/`Merge` skip `XXX_`-prefixed fields, so mixed gogo/wiresmith trees interoperate during staged migrations. Generic deep-equality (`reflect.DeepEqual`, `cmp.Diff`, testify) **does** see the exported field — ignore it with a `cmp.FilterPath` on the field name (see `ignoreBitmaps` in `test/basic/kitchen_sink_test.go`), use the generated `Equal()`, or opt the message out of the bitmap entirely with `(wiresmith.options.no_presence)`.

- **Getter methods.** `Get<Field>()` methods are generated for every field and are nil-safe (return zero value on a nil receiver). For value-type message fields the getter returns `*MessageType` and consults the presence bitmap, returning `nil` when the field was absent from the wire. Optional getters dereference; oneof getters type-assert; repeated/map getters return the slice/map.

- **Reset / ProtoMessage / String.** `Reset()` zeroes the struct (`*m = Type{}`). `ProtoMessage()` is a no-op marker method matching the standard `proto.Message` shape. `String()` uses `fmt.Sprintf("%v", *m)`.

- **Enum name maps.** Each enum gets `TypeName_name` (int32→string), `TypeName_value` (string→int32), and a `String()` method. Constant names follow `protoc-gen-go`'s prefixing rules: enum name for top-level enums (`Color_COLOR_RED`), parent message chain for nested enums (`Span_SPAN_KIND_SERVER`). Map string values use bare proto names.

- **Type registration with the official registry.** The generated `init()` embeds raw file descriptor bytes and registers them with `protoregistry.GlobalFiles`, then registers messages via `protoimpl.MessageInfo` and enums via `protoimpl.EnumInfo`. This makes wiresmith messages usable from `proto.Marshal`, `proto.Unmarshal`, `proto.Size`, `proto.Equal`, `proto.MessageName`, and descriptor lookups — see the reflection-support discussion in [generated-api.md](generated-api.md) for the limits.

## Limitations

The decisions above produce some deliberate non-features.

- **Unknown fields are discarded** on unmarshal and never preserved. Preserving them would cost a per-struct byte slice; wiresmith assumes the schema is known to both sides.
- **No deterministic marshaling.** Generated `MarshalToSizedBuffer` ranges over Go maps in iteration order. The fast-path `Methods` table does not advertise `SupportMarshalDeterministic`, so `proto.MarshalOptions{Deterministic: true}` falls through to the reflection slow-path and panics rather than silently emitting unstable bytes.
- **Field-level reflection panics.** `proto.Marshal/Unmarshal/Size/Equal/MessageName` work via the fast-path methods table, but `Range/Get/Set/Mutable` on the returned `protoreflect.Message` panic — value-type message fields are incompatible with `protoimpl`'s field converters.
- **`protojson`, `prototext`, `proto.Clone`, `proto.Merge` are unsupported** because they are built on top of field-level reflection.
- **No proto2, services/RPCs, extensions (other than the wiresmith options), well-known types.** Out of scope for the OpenTelemetry-style workloads wiresmith targets.

For approaches that were investigated and rejected, see the "Rejected approaches" section of [`AGENTS.md`](../AGENTS.md) — it captures the `//go:fix inline` experiment and why it does not work given the current Go inliner.
