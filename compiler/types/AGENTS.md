# compiler/types/

Per-kind type dispatch for the code generator. Replaces switch-on-`protoreflect.Kind` statements in the emit files with interface-based dispatch.

## Architecture

- **`Type` interface** — per-kind scalar types (bool, int32, string, message, etc.) implementing size/marshal/unmarshal code emission
- **Composite types** (`OptionalField`, `RepeatedField`, `MapField`, `OneofField`) — wrap scalar types to handle context-specific logic (nil checks, loops, packed encoding, map iteration)
- **`FieldType` interface** — unified interface that callers use: `EmitSize`, `EmitMarshal`, `EmitUnmarshal`
- **`Emitter` interface** — decouples types from `FileGenerator` to avoid circular imports
- **`ForField(fd)`** — constructs the right `FieldType` (singular, optional, repeated, or map) from a field descriptor

## Shared bases

- `varintBase` — Int32, Uint32, Int64, Uint64 (parameterized by cast expression)
- `fixed32Base` — Fixed32, Sfixed32, Float (parameterized by put/get expressions)
- `fixed64Base` — Fixed64, Sfixed64, Double

Standalone: Bool, Sint32, Sint64, Enum, String, Bytes, Message.

## Adding a new type

1. Create `<type>.go` with a struct implementing the `Type` interface
2. Register it via `init()` calling `register(protoreflect.XxxKind, &YourType{})`
3. Verify: `make generate-ours && git diff gen/` must show no changes
