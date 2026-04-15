# grafana-protoc

Custom protobuf compiler that generates high-performance Go code from OpenTelemetry .proto files using `google.golang.org/protobuf/encoding/protowire` and reverse-write marshaling.

## Project structure

- `proto/` - Source OpenTelemetry .proto files (common, resource, metrics, trace, logs, profiles)
- `compiler/generator/` - Code generator: reads proto descriptors via `bufbuild/protocompile`, emits Go structs + marshal/unmarshal/size methods
- `cmd/grafana-protoc/` - CLI entry point
- `gen/otlp/` - Generated Go packages (one per proto file)
- `gen/protohelpers/` - Shared reverse-write encoding helpers (based on vtprotobuf's protohelpers, Apache 2.0)
- `test/` - Round-trip correctness tests against official `google.golang.org/protobuf`
- `bench/` - Comparative benchmarks (ours vs official protobuf vs vtproto)
- `bench/vtpb/` - vtproto-generated code for benchmark comparison

## Commands

### Regenerate code from protos
```
go run ./cmd/grafana-protoc/ --proto_path=proto --out=gen --module=grafana-protoc
```

### Run tests
```
go test ./test/ -v
```

### Run fuzz tests
```
go test ./test/ -fuzz FuzzUnmarshalProto -fuzztime 30s
```
Feeds random bytes into all generated `UnmarshalProto` methods to verify they return errors rather than panic on malformed input.

### Run benchmarks
```
GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn go test ./bench/ -bench=. -benchmem -count=5
```
The env var is needed because both official OTel proto packages and our vtpb copies register the same proto files.

### Regenerate vtproto comparison code
Requires `protoc`, `protoc-gen-go`, `protoc-gen-go-vtproto` installed:
```
PROTO_ROOT=$(mktemp -d)
mkdir -p $PROTO_ROOT/opentelemetry/proto/{common/v1,resource/v1,metrics/v1,trace/v1,logs/v1,profiles/v1development}
cp proto/common.proto $PROTO_ROOT/opentelemetry/proto/common/v1/
cp proto/resource.proto $PROTO_ROOT/opentelemetry/proto/resource/v1/
cp proto/metrics.proto $PROTO_ROOT/opentelemetry/proto/metrics/v1/
cp proto/trace.proto $PROTO_ROOT/opentelemetry/proto/trace/v1/
cp proto/logs.proto $PROTO_ROOT/opentelemetry/proto/logs/v1/
cp proto/profiles.proto $PROTO_ROOT/opentelemetry/proto/profiles/v1development/

protoc -I "$PROTO_ROOT" \
  --go_out=. --go_opt=module=grafana-protoc \
  --go_opt=Mopentelemetry/proto/common/v1/common.proto=grafana-protoc/bench/vtpb/common/v1 \
  --go_opt=Mopentelemetry/proto/resource/v1/resource.proto=grafana-protoc/bench/vtpb/resource/v1 \
  --go_opt=Mopentelemetry/proto/metrics/v1/metrics.proto=grafana-protoc/bench/vtpb/metrics/v1 \
  --go_opt=Mopentelemetry/proto/trace/v1/trace.proto=grafana-protoc/bench/vtpb/trace/v1 \
  --go_opt=Mopentelemetry/proto/logs/v1/logs.proto=grafana-protoc/bench/vtpb/logs/v1 \
  --go_opt=Mopentelemetry/proto/profiles/v1development/profiles.proto=grafana-protoc/bench/vtpb/profiles/v1development \
  --go-vtproto_out=. --go-vtproto_opt=module=grafana-protoc \
  --go-vtproto_opt=features=marshal+unmarshal+size \
  --go-vtproto_opt=Mopentelemetry/proto/common/v1/common.proto=grafana-protoc/bench/vtpb/common/v1 \
  --go-vtproto_opt=Mopentelemetry/proto/resource/v1/resource.proto=grafana-protoc/bench/vtpb/resource/v1 \
  --go-vtproto_opt=Mopentelemetry/proto/metrics/v1/metrics.proto=grafana-protoc/bench/vtpb/metrics/v1 \
  --go-vtproto_opt=Mopentelemetry/proto/trace/v1/trace.proto=grafana-protoc/bench/vtpb/trace/v1 \
  --go-vtproto_opt=Mopentelemetry/proto/logs/v1/logs.proto=grafana-protoc/bench/vtpb/logs/v1 \
  --go-vtproto_opt=Mopentelemetry/proto/profiles/v1development/profiles.proto=grafana-protoc/bench/vtpb/profiles/v1development \
  opentelemetry/proto/common/v1/common.proto \
  opentelemetry/proto/resource/v1/resource.proto \
  opentelemetry/proto/metrics/v1/metrics.proto \
  opentelemetry/proto/trace/v1/trace.proto \
  opentelemetry/proto/logs/v1/logs.proto \
  opentelemetry/proto/profiles/v1development/profiles.proto
```

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
