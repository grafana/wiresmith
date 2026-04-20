# Mimir Compatibility: Remaining Changes

Changes from the `mimir-compat` branch not yet ported to `main`.

Already implemented:
- Inline unmarshal optimization (iNdEx/dAtA pattern) — #31
- `Marshal` returns non-nil slice for empty messages — #32
- `Equal(that interface{}) bool` per-message equality — #32
- Proto source comments preserved in generated code — #32

## Method Signature Changes (generally applicable)

### `MarshalToSizedBuffer` returns `(int, error)` instead of `int`
Current: `func (m *T) MarshalToSizedBuffer(dAtA []byte) int`
Target:  `func (m *T) MarshalToSizedBuffer(dAtA []byte) (int, error)`
- All internal callers (nested message marshal) must propagate the error.
- `MessageType.EmitMarshal` and `EmitValueMarshal` in `compiler/types/message.go` already emit `(int, error)` return handling — the emitter itself needs updating in `emit_marshal.go`.

### New `MarshalTo` method
```go
func (m *T) MarshalTo(dAtA []byte) (int, error) {
    size := m.Size()
    return m.MarshalToSizedBuffer(dAtA[:size])
}
```

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
- `(gogoproto.nullable)` support — message fields as `*T` instead of `T`
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
