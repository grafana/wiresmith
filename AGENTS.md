# wiresmith

Custom protobuf compiler that generates high-performance Go code from OpenTelemetry .proto files using `google.golang.org/protobuf/encoding/protowire` and reverse-write marshaling.

## Project structure

- `proto/` - All .proto source files, organized by purpose:
  - `otlp/` - OpenTelemetry protos (common, resource, metrics, trace, logs, profiles)
  - `test/` - Test protos (kitchen_sink)
  - `bench/` - Benchmark protos (maps)
  - `conformance/` - Conformance protos (protocol envelope + test messages)
- `compiler/generator/` - Code generator: reads proto descriptors via `bufbuild/protocompile`, emits Go structs + marshal/unmarshal/size methods
- `compiler/types/` - Per-kind type dispatch for code emission, see [compiler/types/AGENTS.md](compiler/types/AGENTS.md)
- `cmd/wiresmith/` - CLI entry point
- `gen/otlp/` - Generated Go packages (one per proto file)
- `gen/vtpb/` - vtproto-generated code for benchmark comparison
- `gen/gogopb/` - gogoproto-generated code for benchmark comparison
- `gen/protohelpers/` - Shared reverse-write encoding helpers (based on vtprotobuf's protohelpers, Apache 2.0)
- `test/` - All tests, organized by purpose:
  - `testutil/` - Shared test helpers (message interface, constructors)
  - `basic/` - Code-path exercise tests (roundtrip, equal, has_field, kitchen_sink, map, etc.)
  - `fuzz/` - Fuzz tests (unmarshal, roundtrip, cross-library, structured, differential)
  - `differential/` - Cross-library comparison tests (official protobuf, pdata)
  - `peer/` - Tests sourced from vtproto/gogoproto patterns
  - `conformance/` - Google protobuf conformance tests (Docker-based), see [test/conformance/AGENTS.md](test/conformance/AGENTS.md)
- `bench/` - Comparative benchmarks (ours vs official protobuf vs vtproto vs gogoproto)

## Commands

All commands are available via `make`:

| Target | Description |
|--------|-------------|
| `make build` | Build all packages |
| `make test` | Run correctness tests |
| `make fuzz` | Fuzz `Unmarshal` methods (30s) — feeds random bytes to verify errors, not panics |
| `make generate` | Regenerate all code (ours + vtproto + gogoproto). Requires `protoc`, `protoc-gen-go`, `protoc-gen-go-vtproto`, `protoc-gen-gogofast` |
| `make generate-ours` | Regenerate all wiresmith + conformance code. Requires `protoc`, `protoc-gen-go` |
| `make coverage` | Run tests with coverage report |
| `make bench` | Run comparative benchmarks (5 iterations) |
| `make bench-compare` | Run per-library benchmarks and compare with `benchstat`. Accepts `COUNT=-count=N` |
| `make conformance` | Run Google protobuf conformance tests in Docker |
| `make clean` | Remove all generated code under `gen/` |

## Design decisions

- **Value-type struct fields**: Message fields are value types (`Resource Resource`, not `*Resource`). Only `optional` proto3 fields use pointers (`*float64`).
- **Reverse-write marshaling**: `MarshalToSizedBuffer` writes from the end of the buffer backwards, eliminating double size computation for nested messages. Based on the same technique vtprotobuf uses.
- **Pre-computed tag bytes**: Tag bytes are computed at codegen time and emitted as byte literals (`dAtA[i] = 0x0a`).
- **Packed repeated scalars**: Repeated numeric fields use packed encoding (proto3 default). Unmarshal handles both packed and unpacked for compatibility.
- **No reflection**: All marshal/unmarshal/size code is directly generated with no runtime reflection.
- **bufbuild/protocompile for parsing**: Reuses the buf compiler for proto parsing and type resolution instead of a custom parser.
- **Field presence bitmap**: Singular non-optional, non-oneof fields get a `fieldsPresent` bitmap (`[N]uint64`) that tracks which fields were seen during `Unmarshal`. Generated `Has<Field>()` methods let callers distinguish "field absent" from "field set to zero value". The bitmap is not serialized. Repeated, map, optional, and oneof fields are excluded — they already have presence semantics via nil slice/pointer/interface. **Note on optional bytes**: optional bytes fields use `[]byte` (same Go type as regular bytes), with `nil` = absent and non-nil = present. This matches gogoproto/official protobuf behavior. The distinction is via nil-check, not the bitmap — `[]byte{}` is "present but empty", `nil` is "absent".
- **Getter methods**: `Get<Field>()` methods are generated for all fields (nil-safe like gogoproto). For value-type message fields, the getter returns `*MessageType` and uses the presence bitmap to return `nil` when the field was absent from the wire. Optional fields dereference the pointer, oneof fields type-assert, repeated/map fields return the slice/map.
- **Reset/ProtoMessage/String**: `Reset()` zeroes the struct (`*m = Type{}`). `ProtoMessage()` is a no-op marker method, matching the standard `proto.Message` interface shape. `String()` uses `fmt.Sprintf("%v", *m)`.
- **Enum name maps**: Each enum gets `TypeName_name` (int32→string) and `TypeName_value` (string→int32) maps, plus a `String()` method. Enum constants are prefixed matching `protoc-gen-go`: enum name for top-level enums (`Color_COLOR_RED`), parent message chain for nested enums (`Span_SPAN_KIND_SERVER`). Map string values use bare proto names.
- **Type registration**: Generated `init()` registers all messages and enums via `protohelpers.RegisterType`/`RegisterEnum`. No gogo/protobuf dependency — uses a lightweight self-hosted registry in `gen/protohelpers/`.
- **Unknown fields discarded**: Unknown fields are intentionally skipped during unmarshal and not preserved. This is a deliberate performance trade-off: wiresmith is designed for working with messages of known schema, so unknown field preservation would add per-struct overhead with no benefit for the primary use case.

## Supported proto3 features

Messages, nested messages, enums (top-level and nested), oneof, optional, repeated (packed + non-packed), maps, reserved fields, cross-file imports, fully-qualified type references. Scalar types: string, bool, int32, int64, uint32, uint64, sint32, sint64, float, double, bytes, fixed32, fixed64, sfixed32, sfixed64. Map keys: all scalar types except float/double/bytes. Map values: all scalars, enums, messages.

Not supported (not needed for OTel protos): services/RPCs, extensions, well-known types, proto2.

## Conformance test status

695 passing, 5 expected failures (3 message merge with recursive messages, 2 unknown field preservation). Unknown fields are intentionally discarded for performance. Run with `make conformance`.

**Updating the failure list:** The conformance runner errors when a failure list entry matches a now-passing test ("is in the failure list, but test succeeded"). After fixing conformance-related bugs, run conformance and remove entries that the runner flags:

```
docker run --rm --entrypoint conformance_test_runner wiresmith-conformance /usr/local/bin/testee
```

Compare the `unexpected failures` output against `test/conformance/failure_list.txt` and remove entries that no longer appear. The expected failure count in the runner output should equal the number of entries in the file.

## Common review caveats

Recurring themes from PR reviews. Keep these in mind when modifying the generator or generated code.

### Nil-safety on all generated receiver methods
Every generated method with a pointer receiver (`String()`, `Has<Field>()`, `Get<Field>()`, `Equal()`, `Reset()`) must handle `nil` receivers gracefully — return zero value or `"<nil>"`, never panic. Callers assume uniform nil-safety across the generated API.

### Generated code must compile for edge-case protos
- **`allow_alias` enums**: Multiple enum names can map to the same numeric value. Emitting a map literal with duplicate keys fails compilation — deduplicate by number or use assignment statements.
- **Empty .proto files**: Files with no messages or enums must not emit an empty `init()` or unused imports — both cause compilation errors.
- **`[packed = false]`**: Repeated scalar fields with explicit `[packed = false]` must be marshaled/sized as individual tag+value pairs, not as packed length-delimited blobs.

### Wire format safety in unmarshal
- **Reject field number 0**: Inline tag decode must validate `fieldNum >= 1` before dispatching. Zero is not a valid protobuf field number.
- **Varint byte limits**: Enforce 10-byte maximum for varint decoding. The 10th byte must have `b & 0x7F <= 1` at `shift == 63` to prevent silent overflow.
- **Packed field bounds**: Packed element reads must check against `postIndex` (packed payload boundary), not `l` (message length). Malformed packed data must not consume bytes from subsequent fields.
- **32-bit length safety**: Length-delimited varint decode into `int` truncates on 32-bit architectures when `shift >= 32`. Decode into `uint64` first, then bounds-check before converting.

### Map field correctness
- **Merge semantics**: Track value-field presence with a boolean, not `len(mapValueBytes) > 0`. An empty-but-present message value (length 0) must trigger merge, not overwrite. Absent value field must preserve the existing entry.
- **Bytes aliasing**: Bytes map values must allocate a fresh slice per entry (`append([]byte(nil), ...)` or `slices.Clone`). Reusing a backing array via `append(varName[:0], ...)` corrupts previously stored entries.

### Oneof equality requires value comparison
Comparing oneof fields with `!=` checks interface pointer identity, not semantic equality. Two independently allocated oneofs with identical payloads compare as unequal. Use a type-switch that compares variant type and payload.

### Keep documentation in sync with code
CLAUDE.md, AGENTS.md, MIMIR.md, and TEMPO.md track conformance counts, feature status, and CLI flags. Update them when features land or conformance results change. Reviewers consistently flag stale docs.

### Generator test coverage
The generator smoke test (`TestGenerateMatchesCheckedIn`) only checks files that were produced — it does not fail when a file stops being generated. Significant new generator features (Equal, presence bitmap, registration) should have dedicated tests, not just regeneration checks.

## Known issues

- `go test ./...` panics in `bench/` with `proto: file "maps.proto" is already registered` due to conflicting proto registrations between `gen/bench/official`, `gen/bench/vtpb`, and `gen/bench/gogopb`. Use `go test ./test/... ./compiler/...` to run tests without the bench package, or `GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn go test ./bench/` to run benchmarks.
