# Tempo Integration

Summary of wiresmith changes needed to integrate with Grafana Tempo.

## CLI Flags Added

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

## Already Implemented

- Getter methods (`GetXxx()`) for all fields — #34
- `Reset()`, `ProtoMessage()`, `String()` on all messages — #34, #35
- Enum name maps (`_name`/`_value`) and `String()` method — #35
- `.pb.go` output file suffix — #35
- Type registration via `protohelpers.RegisterType`/`RegisterEnum` in `init()` — #35

## Compiler Changes (remaining)

### Recursive directory scanning

`buildImportMapping` was changed from flat directory listing to `filepath.Walk`. Nested layouts like `common/v1/common.proto` use the relative path as the import key, while flat layouts fall back to the package-derived path.

### Configurable package paths

- `goPackageDir`: strips a configurable prefix (e.g. `tempopb.common.v1` with `--strip_prefix=tempopb` produces `common/v1`)
- `goDeclaredPkgName`: with `--strip_prefix`, uses the last path component (`v1`) instead of the concatenated form (`commonv1`)
- `goImportPath`: uses `--import_base` instead of `module/gen/`
- `helpersImportPath`: uses `--helpers_import` if set

### Struct tags (`emit_struct.go`)

Generated `protobuf` and `json` struct tags on every field. Required for:

- `jsonpb.Marshaler` (gogoproto JSON) which reads `protobuf` tags to discover field names and types
- `encoding/json` unmarshal which reads `json` tags for field mapping
- OTLP JSON spec compliance: int64/uint64 fields get `json:"...,string"` tag because the proto JSON spec encodes 64-bit integers as strings

Oneof fields get `protobuf_oneof:"name"` tag.

## Tempo-Side Changes Required

### Value types instead of pointers

wiresmith2 generates value-type message fields (`Resource Resource` not `Resource *Resource`). This requires mechanical changes throughout the codebase:

- Remove `&` from struct literals: `&v1.AnyValue{...}` becomes `v1.AnyValue{...}`
- Change pointer slices to value slices: `[]*v1.KeyValue{...}` becomes `[]v1.KeyValue{...}`
- Replace nil checks on message fields: `if resource != nil` becomes a zero-value check or is removed
- Use index-based range loops where mutations must persist: `for i := range slice` with `&slice[i]` instead of `for _, v := range slice`
- Function parameters taking `[]*KeyValue` change to `[]KeyValue`

### AnyValue JSON handling

A custom `UnmarshalJSON`/`MarshalJSON` must be added to `AnyValue` (in a separate file in the common/v1 package) because:

- `encoding/json` cannot handle oneof fields natively
- `jsonpb.Unmarshaler` requires proto type registration which wiresmith2 types lack
- The custom marshaler encodes int64 as strings per the OTLP JSON spec

### jsonpb replacement

Calls to `jsonpb.Marshal(writer, &anyValue)` and `jsonpb.Unmarshal(reader, &anyValue)` in parquet schema files must be replaced with `json.Marshal`/`json.Unmarshal`, since wiresmith2 types don't support gogoproto's proto reflection.

`MarshalToJSONV1` (which marshals `*Trace`) can keep using `jsonpb.Marshaler` because `Trace` is still gogoproto-generated. `UnmarshalFromJSONV1` must use `encoding/json` because jsonpb can't unmarshal into wiresmith2-typed nested fields.

### proto.Equal replacement

`proto.Equal` panics on wiresmith2 types (no proto reflection). Replace with `reflect.DeepEqual` or `assert.Equal` in tests.

### spansetID determinism

The `spansetID` function in `pkg/traceql/combine.go` used `anyValue.String()` for identity. gogoproto's `String()` produced `string_value:"foo"` while wiresmith2's produces `{Value:0x...}`. Fixed by using a custom `attributeValueString` that type-switches on the oneof variant.
