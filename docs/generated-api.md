# Generated Go API

This page describes the shape of the code wiresmith emits, from a consumer's point of view. For the reasoning behind each choice, see [design.md](design.md).

## Per-message methods

Every generated method below is nil-safe — `String()`, `Has<Field>()`, `Get<Field>()`, `Equal()`, `Reset()`, `Marshal()`, `MarshalTo()`, `MarshalToSizedBuffer()`, and `Size()` all handle a nil receiver gracefully (returning a zero value, `"<nil>"`, or `(nil, nil)` as appropriate) instead of panicking. The two exceptions are `Unmarshal()` and `UnmarshalWithDepth()`: they write into the receiver's struct fields, so calling them on a nil pointer panics. Always allocate the destination before unmarshalling (`var m T; m.Unmarshal(...)` or `m := &T{}; m.Unmarshal(...)`).

| Method                                              | Purpose                                                                                   |
|-----------------------------------------------------|-------------------------------------------------------------------------------------------|
| `Marshal() ([]byte, error)`                         | Allocate a buffer of the right size and serialize into it.                                |
| `MarshalTo(dAtA []byte) (int, error)`               | Serialize into a caller-provided buffer assumed to be `>= Size()`.                        |
| `MarshalToSizedBuffer(dAtA []byte) (int, error)`    | Reverse-write into a caller-provided buffer of exactly `Size()` bytes. Hot-path entry point. |
| `Unmarshal(dAtA []byte) error`                      | Parse wire bytes into the receiver. Populates the field-presence bitmap.                  |
| `UnmarshalWithDepth(dAtA []byte, depth int) error`  | Same as `Unmarshal`, but starts depth tracking at the given value. Used by cross-package callers so the recursion-depth guard remains monotonic across package boundaries. Top-level callers should use `Unmarshal`. |
| `Size() int`                                        | Computed serialized length (independent of `Marshal`, kept consistent by codegen).        |
| `Reset()`                                           | Zero the struct (`*m = Type{}`).                                                          |
| `ProtoMessage()`                                    | Marker method satisfying `proto.Message`.                                                 |
| `String() string`                                   | Debug representation via `fmt.Sprintf("%v", *m)`.                                         |
| `ProtoReflect() protoreflect.Message`               | Returns a fast-path `protoreflect.Message` that supports `proto.Marshal/Unmarshal/Size/Equal`. See caveats below. |
| `Equal(other *T) bool`                              | Semantic equality. Compares oneof variants by type + value, not by interface identity. Float and double fields are compared by `math.Float{32,64}bits` (bit-exact) so identical NaN payloads are equal and `-0.0`/`+0.0` are not — matching `proto.Equal` and the marshal path's bit-exact preservation. |
| `Get<Field>() <FieldType>`                          | One per field. See "Field shape" below for the return type per shape.                     |
| `Has<Field>() bool`                                 | Only for singular non-optional, non-oneof fields. Reads the presence bitmap.              |

## Field shape

| Proto declaration                        | Generated Go field type                  | Presence signal                       |
|------------------------------------------|------------------------------------------|---------------------------------------|
| Scalar (`int32`, `string`, …)            | Native Go type                           | `Has<Field>()` (bitmap)               |
| `bytes`                                  | `[]byte`                                 | `Has<Field>()` (bitmap)               |
| Singular `message Foo`                   | `Foo` (value)                            | `Has<Field>()` (bitmap); `Get<Field>()` returns `*Foo` and uses the bitmap to return `nil` when absent |
| `optional` scalar                        | `*T`                                     | `nil` vs non-nil pointer              |
| `optional bytes`                         | `[]byte` (same as plain `bytes`)         | `nil` = absent, non-nil = present (including `[]byte{}` = "present but empty") |
| `optional` message                       | `*MessageType`                           | `nil` vs non-nil pointer              |
| Repeated scalar / message                | `[]T` / `[]MessageType`                  | `nil` vs non-`nil` slice              |
| Map field                                | `map[K]V`                                | `nil` vs non-`nil` map                |
| Oneof                                    | Interface field + per-variant wrapper structs | `nil` vs non-`nil` interface     |
| Field with `(wiresmith.options.pointer)` | `*Foo` / `[]*Foo` — see [extensions.md](extensions.md) | `nil` vs non-`nil`           |

## Reflection: what works and what does not

The generator advertises a `ProtoMethods` fast path against `google.golang.org/protobuf/runtime/protoimpl`, so the common high-level operations work transparently:

- `proto.Marshal`, `proto.Unmarshal`, `proto.Size` — go through the fast path.
- `proto.Equal` — delegates to the generated `Equal` method.
- `proto.MessageName`, `protoregistry.GlobalTypes.FindMessageByName`, and descriptor lookups against `protoregistry.GlobalFiles` — work because `init()` registers the embedded file descriptors.

What does **not** work — these will panic if called on a wiresmith-generated message:

- Field-level `protoreflect.Message` operations: `Range`, `Get`, `Set`, `Mutable`. wiresmith's value-type message fields are incompatible with `protoimpl`'s field converters.
- Anything built on top of field-level reflection: `protojson.Marshal/Unmarshal`, `prototext.Marshal/Unmarshal`, `proto.Clone`, `proto.Merge`.
- `proto.MarshalOptions{Deterministic: true}` — the fast-path methods table does not advertise determinism, so the call falls through to the reflection slow-path and panics rather than emitting non-deterministic bytes silently. There is no way to ask wiresmith for deterministic output today; sort map keys yourself before marshaling if you need it.

## Enums

For each enum, wiresmith emits:

- A typed int32 with one constant per value. Constants are prefixed to match `protoc-gen-go`: enum name for top-level enums (`Color_COLOR_RED`), parent message chain for nested enums (`Span_SPAN_KIND_SERVER`).
- `<EnumType>_name map[int32]string` — `int32 → bare proto name`. Deduplicated when `allow_alias` is set so the literal compiles.
- `<EnumType>_value map[string]int32` — bare proto name → int32.
- `func (e EnumType) String() string` — looks up `_name`, falls back to the integer.
- An `EnumDescriptor()`/`Descriptor()` pair plus the methods needed to satisfy `protoreflect.Enum`.

The enums are registered with `protoregistry.GlobalTypes` via `protoimpl.EnumInfo` so descriptor-based lookups by full name work the same as for protoc-gen-go-generated enums.
