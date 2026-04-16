# wiresmith

Custom protobuf compiler that generates high-performance Go code from OpenTelemetry .proto files using `google.golang.org/protobuf/encoding/protowire` and reverse-write marshaling.

## Project structure

- `proto/` - Source OpenTelemetry .proto files (common, resource, metrics, trace, logs, profiles)
- `compiler/generator/` - Code generator: reads proto descriptors via `bufbuild/protocompile`, emits Go structs + marshal/unmarshal/size methods
- `cmd/wiresmith/` - CLI entry point
- `gen/otlp/` - Generated Go packages (one per proto file)
- `gen/vtpb/` - vtproto-generated code for benchmark comparison
- `gen/gogopb/` - gogoproto-generated code for benchmark comparison
- `gen/protohelpers/` - Shared reverse-write encoding helpers (based on vtprotobuf's protohelpers, Apache 2.0)
- `test/` - Round-trip correctness tests against official `google.golang.org/protobuf`
- `bench/` - Comparative benchmarks (ours vs official protobuf vs vtproto vs gogoproto)
## Commands

All commands are available via `make`:

| Target | Description |
|--------|-------------|
| `make build` | Build all packages |
| `make test` | Run correctness tests |
| `make fuzz` | Fuzz all 7 targets (10s each) — panic safety, round-trip, size, cross-library, structured trace/metrics/logs |
| `make generate` | Regenerate all code (ours + vtproto + gogoproto). Requires `protoc`, `protoc-gen-go`, `protoc-gen-go-vtproto`, `protoc-gen-gogofast` |
| `make generate-ours` | Regenerate only our code from protos |
| `make bench` | Run comparative benchmarks (5 iterations) |
| `make bench-compare` | Run per-library benchmarks and compare with `benchstat`. Accepts `COUNT=-count=N` |
| `make clean` | Remove all generated code under `gen/` |

## Design decisions

- **Value-type struct fields**: Message fields are value types (`Resource Resource`, not `*Resource`). Only `optional` proto3 fields use pointers (`*float64`).
- **Reverse-write marshaling**: `MarshalToSizedBuffer` writes from the end of the buffer backwards, eliminating double size computation for nested messages. Based on the same technique vtprotobuf uses.
- **Pre-computed tag bytes**: Tag bytes are computed at codegen time and emitted as byte literals (`dAtA[i] = 0x0a`).
- **Packed repeated scalars**: Repeated numeric fields use packed encoding (proto3 default). Unmarshal handles both packed and unpacked for compatibility.
- **No reflection**: All marshal/unmarshal/size code is directly generated with no runtime reflection.
- **bufbuild/protocompile for parsing**: Reuses the buf compiler for proto parsing and type resolution instead of a custom parser.

## Supported proto3 features

Messages, nested messages, enums (top-level and nested), oneof, optional, repeated (packed + non-packed), reserved fields, cross-file imports, fully-qualified type references. Scalar types: string, bool, int32, int64, uint32, uint64, sint32, double, bytes, fixed32, fixed64, sfixed64.

Not supported (not needed for OTel protos): maps, services/RPCs, extensions, well-known types, proto2.
