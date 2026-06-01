# Custom proto extensions

wiresmith exposes a single custom `.proto` file with field-level options that influence the generated Go shape without changing the wire format. The on-wire layout always matches stock proto3 — switching the option on or off does not break binary compatibility with peers using other libraries.

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
