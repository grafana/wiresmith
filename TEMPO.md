# Tempo Integration

Summary of wiresmith2 changes needed to integrate with Grafana Tempo.

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

## Compiler Changes

### Recursive directory scanning

`buildImportMapping` was changed from flat directory listing to `filepath.Walk`. Nested layouts like `common/v1/common.proto` use the relative path as the import key, while flat layouts fall back to the package-derived path.

### Configurable package paths

- `goPackageDir`: strips a configurable prefix (e.g. `tempopb.common.v1` with `--strip_prefix=tempopb` produces `common/v1`)
- `goDeclaredPkgName`: with `--strip_prefix`, uses the last path component (`v1`) instead of the concatenated form (`commonv1`)
- `goImportPath`: uses `--import_base` instead of `module/gen/`
- `helpersImportPath`: uses `--helpers_import` if set

### Getter methods (`emit_getters.go`)

Generated `GetXxx()` for every field. Required because Tempo code uses nil-safe getter chains like `rs.GetResource().GetAttributes()`.

- Scalar/enum fields: return value, zero if nil receiver
- Message fields (value types): return `*T` (pointer to the value field), nil if nil receiver
- Repeated/map fields: return slice/map, nil if nil receiver
- Oneof interface: return the interface, nil if nil receiver
- Oneof variant getters: type-assert the interface and extract the value

### Proto-compat methods (`emit_compat.go`)

Generated `Reset()`, `String()`, `ProtoMessage()` for every message. `ProtoMessage()` is needed as a marker for proto.Message interface satisfaction. `Reset()` is used by some Tempo code paths.

### Enum name maps and String() (`emit_enum.go`)

- Constants use the parent message prefix: `Status_STATUS_CODE_ERROR`
- Name maps use bare enum value names (no prefix): `"STATUS_CODE_ERROR"`
- This matches gogoproto behavior where `String()` returns `"STATUS_CODE_ERROR"`, not `"Status_STATUS_CODE_ERROR"`

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
