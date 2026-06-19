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

- **Reset / ProtoMessage / String.** `Reset()` zeroes the struct (`*m = Type{}`). `ProtoMessage()` is a no-op marker method matching the standard `proto.Message` shape. `String()` is a hand-rolled emitter (the gogoproto `stringer` model) that renders **proto text** — the same shape `protoc-gen-go` / gogo users see, so migrators get a familiar dump: `field_name: value field_name: value nested: {inner: 1} rep: 1 rep: 2`. Fields walk in ascending field number; names are the proto (snake_case) names; enums render by their value name (via the enum's `String()`); strings/bytes are quoted; nested messages recurse in `{ }` via the child's `String()`; repeated fields emit one entry per element; maps emit one sorted `key: <k> value: <v>` entry per key; oneofs print only the set variant; and fields are omitted using the same presence/zero predicate the marshaler uses (so the dump reflects what `Marshal` emits). Every pointer is dereferenced, so output is content- AND byte-deterministic — no leaked heap addresses, no `XXX_fieldsPresent` bitmap, and no prototext "detrand" whitespace jitter (a stability improvement over `protoc-gen-go`). It deliberately does **not** use `protoimpl.X.MessageStringOf` / `prototext`: those walk fields through the `protoreflect` API, which wiresmith's `ProtoReflect` bridge panics on — value-typed message fields are incompatible with the official field converters (see "No field-level reflection" below and `protohelpers/message.go`). The previous `fmt.Sprintf("%v", *m)` printed a `%p` hex address for every pointer field at depth ≥ 1 and leaked the bitmap; the hand-rolled form fixes both. `String()` is emitted into the cold companion `<name>_util.pb.go` (same icache rationale as `_compare`), so it adds no weight to the hot main `.pb.go`.

- **Clone.** `Clone() *T` is a generated, non-reflection deep copy (wiresmith-oz2l). A `nil` receiver returns `nil`; otherwise a fresh message is allocated and every field is deep-copied per-field (mirroring the `Equal`/`Compare`/`String` walkers): bytes/slices/maps get fresh backing storage (`slices.Clone` / fresh `make`), pointer and nested-message fields recurse through their own `Clone()`, oneofs reconstruct the concrete variant with a cloned payload, and the `XXX_fieldsPresent` bitmap (a value array) is copied wholesale — so `clone.Equal(orig)` holds. It exists because reflection-based `proto.Clone` is unsupported by design (the `ProtoReflect` bridge panics on value-typed message fields) and a wire round-trip is both slow and lossy (`[]T{}`→`nil`). customtype/casttype fields are value-copied (the contract is documented in [extensions.md](extensions.md)); a casttype over `bytes` is deep-cloned. `Clone()` is emitted into the cold companion `<name>_util.pb.go`, so it adds no weight to the hot main `.pb.go`.

- **Enum name maps.** Each enum gets `TypeName_name` (int32→string), `TypeName_value` (string→int32), and a `String()` method. Constant names follow `protoc-gen-go`'s prefixing rules: enum name for top-level enums (`Color_COLOR_RED`), parent message chain for nested enums (`Span_SPAN_KIND_SERVER`). Map string values use bare proto names.

- **Type registration with the official registry.** The generated `init()` embeds raw file descriptor bytes and registers them with `protoregistry.GlobalFiles`, then registers messages via `protoimpl.MessageInfo` and enums via `protoimpl.EnumInfo`. This makes wiresmith messages usable from `proto.Marshal`, `proto.Unmarshal`, `proto.Size`, `proto.Equal`, `proto.MessageName`, and descriptor lookups — see the reflection-support discussion in [generated-api.md](generated-api.md) for the limits.

## DB-6 reuse-safety review (wiresmith-u4qg)

The pre-scan prealloc reuses a caller-provided slice's backing array instead of
always allocating: `if len(m.X) == 0 && cap(m.X) < c { m.X = make([]T, 0, c) }`
(see `emitPreScan` in `compiler/generator/emit_unmarshal.go`). When the slice is
already at length zero with enough capacity, no `make` runs and the existing
array is reused. This section is the adversarial review of that decision. The
verdict is **no leak**; the safety rests on two structural invariants, both
pinned by regression tests in `test/basic/prescan_reuse_security_test.go`.

**Invariant 1 — nothing reads `[len:cap]`.** Every generated consumer of a
repeated/map field iterates `[:len]` (via `range` or `len(...)` bounds) and
never reslices to `cap`. `cap(...)` appears in wiresmith-owned generated code
*only* in the pre-scan reuse guard itself; `Marshal`, `Size`, `Equal`,
`Compare`, `String`, and the `Get<Field>()` getters never touch it. Field-level
reflection panics by design (see Limitations), and `proto.Marshal/Size/Equal`
route through the len-bounded fast-path methods, so the reflect path cannot
observe the tail either. Consequence: stale elements left in `[len:cap]` by a
previous decode are physically present but unobservable through any generated
method.

**Invariant 2 — the main decode loop zeroes each reused slot before writing.**
Repeated message/pointer fields append a fresh composite literal
(`append(m.X, T{})` / `append(m.X, &T{})`) and only then decode into the new
slot, so a slot that previously held data from decode N−1 is overwritten with a
zero value before any decode-N field lands in it. A decode-N element with a
default (unserialized) field therefore reads as zero, not as the stale value.
Repeated scalar/string/bytes append freshly-built values
(`string(dAtA[...])`, `append([]byte(nil), ...)`), never reusing the old slot's
contents.

### Scenario verdicts

1. **Cross-tenant / cross-request exposure — SAFE.** Decode tenant-A (8
   elements), truncate to `[:0]` keeping capacity (the pooled pattern), decode
   shorter tenant-B (3 elements): the backing array is reused and tenant-A's
   secret survives in `[3:8]`, but `Marshal`, `Size`, `Equal`, `Compare`,
   `String`, and `GetExtras()` all observe only tenant-B `[:len]`
   (`TestPreScanReuse_NoCrossTenantLeakViaAnyMethod`). The append-zeroing
   invariant also blocks an in-band leak when tenant-B's elements have default
   values (`TestPreScanReuse_AppendZeroesReusedSlot`).

   **Note on `Reset()`:** `Reset()` does `*m = T{}`, which **nils** the slice —
   it does **not** retain capacity. The capacity-retaining reuse is triggered
   only by an explicit `m.X = m.X[:0]` (or a pool's `ResetVT`-style truncation),
   never by the generated `Reset()`. The bead's "after Reset truncates to
   `[:0]`" phrasing refers to the pooling caller's own truncation, not the
   generated `Reset`.

2. **Error-path partial state — SAFE.** A decode that errors partway (truncated
   final element) leaves the message holding only zeroed/partial tenant-B data
   in `[:len]`; no stale tenant-A bytes are reachable, and `Marshal` of the
   partial message does not serialize them
   (`TestPreScanReuse_ErrorPathNoStaleData`). This is a direct corollary of the
   two invariants: the partially-decoded element was zeroed by `append` before
   the error, and the failed tail beyond `len` is unobservable.

   **Caller contract for pooled/shared destinations.** Wiresmith never aliases
   the wire buffer (unlike vtprotobuf's `UnmarshalVTUnsafe`): bytes and strings
   are always copied, so a returned message never depends on the input buffer's
   lifetime. The remaining caller obligation is the standard gogo/vtprotobuf
   one: **on an `Unmarshal` error, treat the destination as garbage** — do not
   forward, log, or return a message to a shared pool after a failed decode
   without zeroing it (`Reset()` or fresh allocation). For replace (non-merge)
   semantics, `Reset()` (or a fresh message) before `Unmarshal`; reusing a
   non-reset message appends to repeated fields and merges maps (gogo parity,
   see the merge-semantics decision above). The gRPC proto codec allocates a
   fresh message per decode, so it is unaffected; Mimir's `WriteRequest` pool
   must `Reset()`/zero on `Put` (or before reuse) — the partial-state element
   lives within `[:len]` and is overwritten or discarded by that reset, and even
   absent a reset no tenant-A bytes are observable per invariant 1.

3. **`preCapMax` `l/2` clamp interaction — SAFE, no retention regression.** When
   `c` is clamped below the true element count, the reuse guard compares against
   the clamped `c`; if the pooled array's `cap` exceeds it the array is reused,
   and the main loop's `append` grows a fresh array once it overflows `cap`.
   On that growth Go reassigns `m.X` to the new array and the old array becomes
   unreferenced — no generated code holds a second reference, so at most one
   backing array is live at a time. This matches the pre-DB-6 behavior (which
   always allocated fresh) and the clamp only gates the *initial* `make`, so it
   introduces no path that keeps both old and new arrays alive.

4. **Unbounded pool growth — caller's problem, documented.** A pooled message
   retains the largest backing array it has ever held (capacity is never shrunk
   by reuse or by `m.X = m.X[:0]`). A single large decode permanently inflates
   the pooled message's footprint until the caller drops it. This is inherent to
   any retain-capacity pooling scheme (vtprotobuf's `ResetVT` has the identical
   property) and is the pool owner's responsibility: cap the message size
   admitted to the pool, or periodically drop oversized entries.

5. **Prior art.** vtprotobuf's pool mode uses `ResetVT` to keep slice/map
   backing memory across `UnmarshalVT` calls and ships a dedicated
   `pool_with_slice_reuse` test proto — the same retain-capacity-on-reset design
   as wiresmith, with the same documented merge-on-non-reset caveat. gogoproto's
   `Unmarshal` likewise appends to repeated fields (merge semantics) with no
   pre-scan. No CVE or security advisory was found for either project tied to
   pool reset / slice reuse; the behaviors are documented contract caveats, not
   tracked vulnerabilities. The one genuinely unsafe variant in the prior art is
   vtprotobuf's `UnmarshalVTUnsafe`, which aliases the wire buffer into byte and
   string fields (corruptible if the buffer is reused while the message is
   live). Wiresmith deliberately does **not** offer that mode — it always copies
   — so it carries strictly less aliasing risk than the prior art.

## Limitations

The decisions above produce some deliberate non-features.

- **Unknown fields are discarded** on unmarshal and never preserved. Preserving them would cost a per-struct byte slice; wiresmith assumes the schema is known to both sides.
- **No deterministic marshaling.** Generated `MarshalToSizedBuffer` ranges over Go maps in iteration order. The fast-path `Methods` table does not advertise `SupportMarshalDeterministic`, so `proto.MarshalOptions{Deterministic: true}` falls through to the reflection slow-path and panics rather than silently emitting unstable bytes.
- **Field-level reflection panics.** `proto.Marshal/Unmarshal/Size/Equal/MessageName` work via the fast-path methods table, but `Range/Get/Set/Mutable` on the returned `protoreflect.Message` panic — value-type message fields are incompatible with `protoimpl`'s field converters.
- **`protojson`, `prototext`, `proto.Clone`, `proto.Merge` are unsupported** because they are built on top of field-level reflection.
- **No proto2 or extensions** (other than the wiresmith options) — out of scope for the OpenTelemetry-style workloads wiresmith targets. (gRPC *services* **are** supported: a `.proto` declaring `service` blocks emits `<name>_grpc.pb.go` stubs via a vendored `protoc-gen-go-grpc` — see [buf.md](buf.md).)
- **Well-known types are supported only where consumers need them:** `Timestamp` / `Duration` via the `stdtime` / `stdduration` options, and `google.protobuf.Any` via a shipped replacement package (`types/known/anypb`) that references resolve to automatically (see `compiler/generator/wellknown.go`). The replacement omits proto-registry registration — the official runtime already registers `google.protobuf.Any` — and hand-writes a `ProtoReflect` that delegates to the official descriptor. Other WKTs (Empty, Struct, FieldMask, wrappers) are out of scope.

For approaches that were investigated and rejected, see the "Rejected approaches" section of [`AGENTS.md`](../AGENTS.md) — it captures the `//go:fix inline` experiment and why it does not work given the current Go inliner.
