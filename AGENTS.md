# wiresmith

Custom protobuf compiler that generates high-performance Go code from OpenTelemetry .proto files using `google.golang.org/protobuf/encoding/protowire` and reverse-write marshaling.

## Project structure

- `proto/` - All .proto source files, organized by purpose:
  - `otlp/` - OpenTelemetry protos (common, resource, metrics, trace, logs, profiles)
  - `test/` - Test protos (kitchen_sink)
  - `basic/` - Basic type coverage protos (maps, numeric, enum, oneof, nesting, recursive)
  - `conformance/` - Conformance protos (protocol envelope + test messages)
- `compiler/generator/` - Code generator: reads proto descriptors via `bufbuild/protocompile`, emits Go structs + marshal/unmarshal/size methods
- `compiler/types/` - Per-kind type dispatch for code emission, see [compiler/types/AGENTS.md](compiler/types/AGENTS.md)
- `cmd/wiresmith/` - CLI entry point
- `gen/otlp/` - Generated Go packages (one per proto file)
- `gen/basic/` - Generated Go packages for basic type coverage protos
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

- **Value-type struct fields**: Message fields are value types (`Resource Resource`, not `*Resource`). `optional` proto3 fields use pointers (`*float64`, `*MessageType`). This enables recursive message definitions via `optional` self-references.
- **Reverse-write marshaling**: `MarshalToSizedBuffer` writes from the end of the buffer backwards, eliminating double size computation for nested messages. Based on the same technique vtprotobuf uses.
- **Pre-computed tag bytes**: Tag bytes are computed at codegen time and emitted as byte literals (`dAtA[i] = 0x0a`).
- **Packed repeated scalars**: Repeated numeric fields use packed encoding (proto3 default). Unmarshal handles both packed and unpacked for compatibility.
- **No reflection**: All marshal/unmarshal/size code is directly generated with no runtime reflection.
- **bufbuild/protocompile for parsing**: Reuses the buf compiler for proto parsing and type resolution instead of a custom parser.
- **Field presence bitmap**: Singular non-optional, non-oneof fields get a `fieldsPresent` bitmap (`[N]uint64`) that tracks which fields were seen during `Unmarshal`. Generated `Has<Field>()` methods let callers distinguish "field absent" from "field set to zero value". The bitmap is not serialized. Repeated, map, optional, and oneof fields are excluded — they already have presence semantics via nil slice/pointer/interface. **Note on optional bytes**: optional bytes fields use `[]byte` (same Go type as regular bytes), with `nil` = absent and non-nil = present. This matches gogoproto/official protobuf behavior. The distinction is via nil-check, not the bitmap — `[]byte{}` is "present but empty", `nil` is "absent".
- **Getter methods**: `Get<Field>()` methods are generated for all fields (nil-safe like gogoproto). For value-type message fields, the getter returns `*MessageType` and uses the presence bitmap to return `nil` when the field was absent from the wire. Optional fields dereference the pointer, oneof fields type-assert, repeated/map fields return the slice/map.
- **Reset/ProtoMessage/String**: `Reset()` zeroes the struct (`*m = Type{}`). `ProtoMessage()` is a no-op marker method, matching the standard `proto.Message` interface shape. `String()` uses `fmt.Sprintf("%v", *m)`.
- **Enum name maps**: Each enum gets `TypeName_name` (int32→string) and `TypeName_value` (string→int32) maps, plus a `String()` method. Enum constants are prefixed matching `protoc-gen-go`: enum name for top-level enums (`Color_COLOR_RED`), parent message chain for nested enums (`Span_SPAN_KIND_SERVER`). Map string values use bare proto names.
- **Type registration**: Generated `init()` embeds raw file descriptor bytes and registers them with `protoregistry.GlobalFiles`, then registers message types via `protoimpl.MessageInfo` and enum types via `protoimpl.EnumInfo` with `protoregistry.GlobalTypes`. Messages implement `proto.Message` via `ProtoReflect()`, enums implement `protoreflect.Enum`. Same registration pattern as protoc-gen-go. **Reflection support is partial**: `proto.Marshal`, `proto.Unmarshal`, `proto.Size`, `proto.Equal`, `proto.MessageName`, and descriptor lookups work via the `ProtoMethods` fast-path. Field-level reflection (`Range`/`Get`/`Set`/`Mutable` on the returned `protoreflect.Message`) panics — wiresmith uses value-type message fields incompatible with `protoimpl`'s field converters. Helpers built on top of field reflection (`protojson`, `prototext`, `proto.Clone`, `proto.Merge`) are unsupported.
- **Unknown fields discarded**: Unknown fields are intentionally skipped during unmarshal and not preserved. This is a deliberate performance trade-off: wiresmith is designed for working with messages of known schema, so unknown field preservation would add per-struct overhead with no benefit for the primary use case.

## Supported proto3 features

Messages, nested messages, enums (top-level and nested), oneof, optional, repeated (packed + non-packed), maps, reserved fields, cross-file imports, fully-qualified type references. Scalar types: string, bool, int32, int64, uint32, uint64, sint32, sint64, float, double, bytes, fixed32, fixed64, sfixed32, sfixed64. Map keys: all scalar types except float/double/bytes. Map values: all scalars, enums, messages.

Not supported (not needed for OTel protos): services/RPCs, extensions, well-known types, proto2.

## Conformance test status

696 passing, 5 expected failures (3 message merge with recursive messages, 2 unknown field preservation). Unknown fields are intentionally discarded for performance. Run with `make conformance`.

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

- `go test ./...` panics with `proto: file ... is already registered` due to conflicting proto registrations between wiresmith types and official protobuf types (in `test/...`) and between `gen/bench/official`, `gen/bench/vtpb`, and `gen/bench/gogopb` (in `bench/`). Use `GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn go test ./...` to run all tests, or `go test ./compiler/...` for the conflict-free subset.

## Rejected approaches

Approaches that were investigated and deliberately not adopted. Documented to save future contributors from re-discovering the same dead ends.

### `//go:fix inline` for hot-path helpers

**Idea:** Have the generator emit calls to small helpers (`EncodeVarint`, `ConsumeVarint`, `ConsumeFixed32`, etc.) marked `//go:fix inline`, then run `go fix -inline ./...` as a codegen post-process step. Generator stays simple (single helper call per emit site); committed `.pb.go` ends up with the helper bodies inlined; we keep today's manual-inline performance.

**Why it doesn't work:** Go's `inline` analyzer (Go 1.26, both `go fix -inline` and the standalone `golang.org/x/tools/go/analysis/passes/inline/cmd/inline` tool) only applies a fix when the call can be **reduced to an expression**. Multi-statement bodies — loops, early-return error paths, slice mutations — would require *literalization* (wrapping the body in `func(){...}()`), which the analyzer "discards unconditionally, on grounds of style" (per `go tool fix help inline`). The diagnostic still fires (`Call of X should be inlined [inline_call]`), but no source rewrite happens.

For wiresmith specifically:

- `protohelpers.EncodeVarint` has a `for` loop and slice writes → not inlined despite the directive. Confirmed: `go fix -inline ./gen/...` left every call site untouched.
- `protohelpers.SizeOfVarint` is single-expression and *is* inlinable, but the generator already emits its body inline (`(bits.Len64(x|1)+6)/7`) so there are no call sites to fix in `gen/otlp/`, `gen/test/`, or `gen/protobuf_test_messages/`.
- The proposed unmarshal-side helpers (`ConsumeVarint`, `ConsumeBytesLen`, `ConsumeFixed32`, `ConsumeFixed64`, `ConsumeTag`) all have loops and/or `return 0, 0, err` early-return paths → not inlinable. Extracting them would convert today's inlined unmarshal hot path into uninlined function calls, regressing the 25-28% speedup from commit `594501d` ("perf: inline varint/fixed decoding").

**When this could become viable:** if the Go inliner gains the ability to apply non-literalized rewrites for multi-statement bodies at statement-level call sites (i.e. expand the body inline as a labeled block instead of refusing). Tracking issue: <https://github.com/golang/go/issues/32816> (the original `//go:fix inline` proposal). Until then, wiresmith's "emit the loop directly" approach in `compiler/types/type.go:114-196` and `compiler/generator/emit_unmarshal.go` is the only way to keep these paths inlined.
