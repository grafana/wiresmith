# Tempo Integration

Summary of wiresmith changes needed to integrate with Grafana Tempo.

## CLI Flags

```
--strip_prefix   Proto package prefix to strip for Go paths (e.g. "tempopb")
--import_base    Go import path base (replaces module/gen/ prefix)
--helpers_import Import path for protohelpers package
```

Tempo invocation:

```bash
wiresmith \
  --proto_path=pkg/.patched-proto \
  --out=pkg/tempopb \
  --module=github.com/grafana/tempo \
  --strip_prefix=tempopb \
  --import_base=github.com/grafana/tempo/pkg/tempopb \
  --helpers_import=github.com/grafana/tempo/pkg/tempopb/protohelpers
```

## Compiler Features (implemented)

- Getter methods (`GetXxx()`) for all fields — #34
- `Reset()`, `ProtoMessage()`, `String()` on all messages — #34, #35
- Enum name maps (`_name`/`_value`) and `String()` method — #35
- `.pb.go` output file suffix — #35
- Type registration via `protohelpers.RegisterType`/`RegisterEnum` in `init()` — #35
- Recursive directory scanning via `filepath.Walk` — `feat/cli-flags-recursive-go-package`
- Configurable package paths (`--strip_prefix`, `--import_base`, `--helpers_import`) — `feat/cli-flags-recursive-go-package`
- `protobuf` and `json` struct tags on every field — `feat/cli-flags-recursive-go-package`
- JSON struct tags use camelCase from proto `json_name` — `tempo-compat`
- 64-bit fields get `json:",string"` tag per OTLP JSON spec — `tempo-compat`
- Enum constants prefixed with parent message name (`Span_SPAN_KIND_CLIENT`) — `tempo-compat`
- Enum `MarshalJSON`/`UnmarshalJSON` accepting both string names and integers — `tempo-compat`
- Message getters return `&m.Field` unconditionally (no presence bitmap gate) — `tempo-compat`
- `XXX_fieldsPresent` (prefixed so gogoproto's reflection skips it) — `tempo-compat`
- `XXX_Unmarshal`, `XXX_Marshal`, `XXX_Merge`, `XXX_Size`, `XXX_DiscardUnknown`, `XXX_MessageName` — `tempo-compat`

## Tempo-Side Changes (done)

### Value types instead of pointers

wiresmith generates value-type message fields (`Resource Resource` not `Resource *Resource`). Mechanical changes throughout the codebase:

- `&v1.AnyValue{...}` → `v1.AnyValue{...}`
- `[]*v1.KeyValue{...}` → `[]v1.KeyValue{...}`
- Nil checks on message fields removed or replaced with zero-value checks
- Index-based range loops where mutations must persist: `for i := range slice` with `&slice[i]`
- Function parameters `[]*KeyValue` → `[]KeyValue`

### AnyValue JSON handling

Custom `UnmarshalJSON`/`MarshalJSON` on `AnyValue` in `pkg/tempopb/common/v1/json.go`. Required because `encoding/json` cannot handle oneof fields natively and the custom marshaler encodes int64 as strings per the OTLP JSON spec.

### jsonpb → encoding/json

`jsonpb.Marshaler`/`jsonpb.Unmarshaler` replaced with `json.Marshal`/`json.NewDecoder().Decode()` in:

- `modules/frontend/combiner/common.go` — JSON marshal and unmarshal paths
- `modules/frontend/combiner/*_test.go` — test helpers
- `modules/frontend/*_test.go` — handler test assertions
- `modules/generator/processor/servicegraphs/servicegraphs_test.go` — test data loading
- `pkg/tempopb/trace_utils.go` — `UnmarshalFromJSONV1` uses `json.Unmarshal`
- `cmd/tempo-vulture/main_test.go` — fixture generation and loading

### spansetID determinism

`attributeValueString` in `pkg/traceql/combine.go` type-switches on the oneof variant instead of relying on `AnyValue.String()`.

### proto.Equal / proto.Clone

Now work via `XXX_` compat methods on wiresmith types. No workarounds needed.

### Enum String() format

wiresmith's `String()` returns the proto value name without parent prefix: `"SPAN_KIND_SERVER"` not `"Span_SPAN_KIND_SERVER"`. Test expectations updated where enum strings appear in log output or serialized form.

## Remaining Work

### vparquet schema tests

`tempodb/encoding/vparquet{3,4,5}/schema_test.go` — `TestTraceToParquet/span_scope_and_resource_attributes` fails. The parquet schema files use `jsonpb.Marshaler` to marshal `AnyValue` to JSON for storage. Since `jsonpb.Marshaler` uses proto reflection and wiresmith types lack it, the marshaled JSON format may differ. These schema files still import `github.com/golang/protobuf/jsonpb` and need to switch to `encoding/json`.

### WAL tests

`tempodb/wal/TestFindByTraceID` fails in vParquet3. Likely related to the vparquet schema issue above.

### Remaining jsonpb in production code

The following files still use `jsonpb` and may need migration:

- `cmd/tempo-query/tempo/plugin.go` — Jaeger query plugin, unmarshals search responses
- `cmd/tempo-cli/cmd-query-*.go`, `cmd/tempo-cli/shared.go` — CLI trace/search output
- `tempodb/encoding/vparquet{3,4,5}/schema.go` — AnyValue JSON in parquet columns
- `modules/frontend/metrics_query_handler.go` — query instant JSON response
- `modules/querier/http.go` — querier HTTP JSON responses
- `pkg/httpclient/client.go` — HTTP client JSON unmarshaling
- `pkg/tempopb/trace_utils.go` — `MarshalToJSONV1` still uses `jsonpb.Marshaler`

Most of these operate on gogoproto types (`SearchResponse`, `QueryRangeResponse`, `Trace`) where `jsonpb` works. Migration is only required for paths that touch wiresmith-generated types (common/v1, resource/v1, trace/v1).

### Remaining proto.Equal in tests

`proto.Equal` now works via `XXX_` methods. However, `proto.Equal` uses gogoproto's reflection-based comparison, which traverses struct fields including `XXX_fieldsPresent`. Two traces with identical data but different presence bitmaps (one constructed in Go, one from Unmarshal) will compare equal because gogoproto skips `XXX_`-prefixed fields. No action needed.
