# Custom proto extensions

wiresmith exposes a single custom `.proto` file with field-, message-, and file-level options that influence the generated Go shape or surface area without changing the wire format. The on-wire layout always matches stock proto3 — switching any option on or off does not break binary compatibility with peers using other libraries.

## `wiresmith/options.proto`

The file lives in the repo at [`compiler/generator/embed/wiresmith/options.proto`](../compiler/generator/embed/wiresmith/options.proto) and is embedded into the compiler binary. The CLI serves it from the canonical import path `wiresmith/options.proto`, so user proto trees can import it without vendoring:

```proto
syntax = "proto3";

package myapp.v1;

import "wiresmith/options.proto";

message Foo {
  // ...
}
```

The lookup is keyed on the import path, so a user file that happens to declare the same proto package (`wiresmith.options`) cannot shadow the embedded definition. See [`compiler/generator/option_pointer.go`](../compiler/generator/option_pointer.go) for the resolution and validation pass.

## `(wiresmith.options.pointer) = true`

The pointer option opts a field into protoc-gen-go / gogoproto-style pointer codegen.

| Field shape on the proto         | Default Go type | With `(wiresmith.options.pointer) = true` |
|----------------------------------|-----------------|-------------------------------------------|
| Singular `message Foo`           | `Foo`           | `*Foo`                                    |
| Repeated `message Foo`           | `[]Foo`         | `[]*Foo`                                  |

For the pointer-shaped form, `nil` means absent: no tag is emitted on marshal, and a `nil` element in a repeated slice is silently skipped.

### Where it applies

Allowed: singular or repeated message fields.

Rejected (and reported as a single combined compile-time error from `validatePointerOptions`):

- Scalars, enums, `bytes`, and `string` — the option only makes sense for message fields. Error: `(wiresmith.options.pointer) only applies to message fields, got <kind>`.
- Map fields. Error: `(wiresmith.options.pointer) is not supported on map fields`.
- Oneof variants — they are already interface-boxed. Error: `(wiresmith.options.pointer) is not supported on oneof variants — variants are already interface-boxed`.
- Fields declared with the proto3 `optional` keyword — `optional` already produces a pointer. Error: `(wiresmith.options.pointer) cannot combine with 'optional' — 'optional' already produces a pointer`.

The validation source of truth is `pointerOptionRejection` in `compiler/generator/option_pointer.go`.

### Worked example

[`proto/basic/pointer.proto`](../proto/basic/pointer.proto) exercises all three positions side by side and serves as the integration test fixture:

```proto
syntax = "proto3";

package basic.pointer.v1;
option go_package = "wiresmith/gen/basic/pointer/v1";

import "wiresmith/options.proto";

message Leaf {
  int64  id   = 1;
  string name = 2;
}

message PointerHolder {
  string name = 1;

  // Generated Go: head *Leaf
  Leaf head = 2 [(wiresmith.options.pointer) = true];

  // Generated Go: items []*Leaf
  repeated Leaf items = 3 [(wiresmith.options.pointer) = true];

  // Control: default value-type shape, generated Go: tail Leaf
  Leaf tail = 4;
}
```

The option is local to a field; mixing pointer and value shapes within the same message is supported and exercised by this fixture.

## `(wiresmith.options.compare)` / `(wiresmith.options.compare_all)`

The compare options opt messages into a generated `Compare(other interface{}) int` method, the wiresmith equivalent of gogoproto's `compare` / `compare_all`. Returns -1/0/+1 like `bytes.Compare`, with the standard gogo nil/wrong-type preamble (`nil.Compare(nil) == 0`, `nil.Compare(non-nil) == -1`, `m.Compare("wrong type") == 1`).

```proto
// Per-message opt-in.
message Job {
  option (wiresmith.options.compare) = true;
  string id = 1;
  int64  priority = 2;
}

// Or, for entire files, the file-level switch.
option (wiresmith.options.compare_all) = true;
```

### Ordering rules

Fields walk in ascending wire-tag order so the relation is stable against declaration-order edits that don't change the tags.

| Shape                       | Comparison                                                                                        |
|-----------------------------|---------------------------------------------------------------------------------------------------|
| `int*` / `uint*` / `sint*`  | Go `<` on the numeric value.                                                                      |
| `bool`                      | `false < true`.                                                                                   |
| `float` / `double`          | Bit-exact via `math.Float{32,64}bits` — matches the bit-exact Equal contract (NaN/-0.0 distinct). |
| `fixed*` / `sfixed*`        | Natural unsigned/signed `<`.                                                                      |
| `enum`                      | Underlying integer value.                                                                         |
| `string`                    | Go's lexicographic `<`.                                                                           |
| `bytes`                     | `bytes.Compare`.                                                                                  |
| `repeated T`                | Shorter slice sorts first; otherwise element-wise.                                                |
| `map<K, V>`                 | Shorter map sorts first; otherwise sorted-key order, key-then-value at each position.             |
| Nested `message`            | Recursive nil-safe `Compare` on the value.                                                        |
| `(wiresmith.options.pointer)` message | `nil` sorts before any non-nil, then recursive `Compare`.                               |
| `optional` scalar           | `nil` sorts before any non-nil, then dereferenced compare.                                        |
| `optional bytes` / `*Msg`   | Same nil-pair ordering, then `bytes.Compare` or recursive `Compare`.                              |
| `oneof`                     | Unset sorts before any set; otherwise variant declaration index, then payload at the same variant. |

### Closure over message-field references

When a Compare-enabled message references another message through a singular, repeated, map-value, or oneof-variant field, the inner message also receives a Compare method automatically — the generator computes the transitive closure once at codegen time. This means you only need to flip the option on the "root" of a message subtree; nothing inside cascades a compile error.

### Why opt-in (rather than always-emit)

Always emitting Compare on every message added ~9% to OTel hot-path microbenchmarks (Marshal/Unmarshal/Size) via icache pressure on the linked binary, even though Compare itself was never called on those paths. Opt-in keeps the cost zero for callers who don't need ordering. See [`compiler/generator/option_compare.go`](../compiler/generator/option_compare.go) for the resolution + closure pass.

### Worked example

[`proto/basic/compare.proto`](../proto/basic/compare.proto) exercises every supported shape; the matching tests are in [`test/basic/compare_test.go`](../test/basic/compare_test.go).
