# Mimir Compatibility: Remaining Changes

Changes from the `mimir-compat` branch not yet ported to `main`.

Already implemented:
- Inline unmarshal optimization (iNdEx/dAtA pattern) — #31
- `Marshal` returns non-nil slice for empty messages — #32
- `Equal(that interface{}) bool` per-message equality — #32
- Proto source comments preserved in generated code — #32
- `MarshalToSizedBuffer` returns `(int, error)` — #32
- `MarshalTo(dAtA []byte) (int, error)` method — #32

## Gogo-Specific Features (require GogoCompat flag)

These are NOT generally applicable — they only matter for gogo/protobuf drop-in replacement.

### Struct tags
- `protobuf:"varint,1,opt,name=foo,json=fooBar,proto3"` tags on struct fields
- `protobuf_oneof:"field_name"` tags on oneof interface fields

### Gogo method set
- `Reset()`, `ProtoMessage()`, `String()`, `GoString()`
- `XXX_*` methods (deprecated but required for gogo compat)
- Getter methods (`Get*()` for each field)
- Registration: `proto.RegisterType()` / `proto.RegisterEnum()` in `init()`

### Pointer semantics
- `(gogoproto.nullable)` equivalent shipped as `(wiresmith.options.pointer) = true` — singular `*T` and repeated `[]*T` for message fields. See [docs/extensions.md](docs/extensions.md). User protos must `import "wiresmith/options.proto"` (embedded by the compiler, no vendoring).
- `(gogoproto.customtype)` — replace Go type entirely
- `(gogoproto.casttype)` — cast to different type name

### Well-known type helpers
- `google.protobuf.Duration` → `time.Duration` with `StdDurationMarshal/Unmarshal`
- `google.protobuf.Timestamp` → `time.Time` with `StdTimeMarshal/Unmarshal`

### gRPC service stubs
- Client/server interfaces, handlers, service descriptors

### Enum prefixing
- `(gogoproto.goproto_enum_prefix)` — prefix enum values with parent message name

### Multi-path proto resolution
- `ProtoPaths []string` and `ProtoFiles []string` on Generator
- `multiPathResolver` for cross-directory proto imports

### Output conventions
- `.pb.go` suffix instead of `.go`
- `go_package` option for output directory
