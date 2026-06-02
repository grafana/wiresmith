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

## `(wiresmith.options.jsontag) = "..."`

The jsontag option overrides the `json:"..."` value of the generated Go struct tag. The wire format is unaffected; the option only changes the JSON-side contract a caller sees through `encoding/json`. Use it when the proto field name diverges from an existing JSON schema you have to honor (e.g. `block_id` on the wire vs. `blockID` in an HTTP API).

```proto
message BlockMeta {
  string block_id      = 1 [(wiresmith.options.jsontag) = "blockID"];
  uint64 total_objects = 2 [(wiresmith.options.jsontag) = "totalObjects,omitempty"];
}
```

```go
type BlockMeta struct {
    BlockId      string `protobuf:"bytes,1,opt,..." json:"blockID"`
    TotalObjects uint64 `protobuf:"varint,2,opt,..." json:"totalObjects,omitempty"`
}
```

The supplied value is used **verbatim** — `,omitempty` is not appended for you, matching `gogoproto.jsontag` semantics. To opt a field out of JSON serialization, use `"-"` (which produces `json:"-"`, the `encoding/json` opt-out idiom); an empty string would emit `json:""`, which `encoding/json` treats as "no tag given" and is *not* an opt-out.

### Where it applies

Allowed on every field kind: scalar, message, repeated, map, and oneof variant.

Rejected (reported as a single combined compile-time error from `validateJsontagOptions`):

- Values containing a backtick. Error: `(wiresmith.options.jsontag) must not contain backticks (would terminate the struct tag)`. Quotes inside the value are safe because tag emission escapes them through `%q`.

The validation source of truth is `validateJsontagOptions` in `compiler/generator/option_jsontag.go`.

### Worked example

[`proto/basic/jsontag.proto`](../proto/basic/jsontag.proto) exercises the option across scalar, message, repeated, map, and the `"-"` opt-out, with an unannotated field as the control showing the default `json:"<proto_name>,omitempty"` shape is unaffected.
