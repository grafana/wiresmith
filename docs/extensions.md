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
option go_package = "github.com/grafana/wiresmith/gen/basic/pointer/v1";

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

## `(wiresmith.options.customtype) = "import/path.TypeName"`

The customtype option replaces the generated Go field type with a user-supplied type that owns its wire encoding. The on-wire layout is unchanged — the field still occupies the same number and is encoded length-delimited; only the Go-side representation differs.

```proto
message LabelSet {
  bytes  pairs     = 1 [(wiresmith.options.customtype) = "github.com/myorg/pkg.LabelPairs"];
  string tenant_id = 2 [(wiresmith.options.customtype) = "github.com/myorg/pkg.TenantID"];
}
```

```go
type LabelSet struct {
    Pairs    pkg.LabelPairs `protobuf:"..."`
    TenantId pkg.TenantID   `protobuf:"..."`
}
```

The import is registered automatically — users don't need to vendor anything beyond their own type. The value accepts two shapes:

- `"import/path.TypeName"` — fully qualified; the generator pulls in the import with an explicit alias derived from the path's base name (`bar` for `github.com/foo/bar`) and qualifies the type via that alias (`bar.TypeName`). The path base must be a valid Go identifier; it does *not* need to match the imported package's `package` declaration, so module major-version layouts (`.../foo/v2`) and packages whose directory name differs from their declared name are both supported. If two customtype paths share the same base name (or the base collides with another import in the same file), the second alias gets a numeric suffix (`bar1`, `bar2`, …) so the generated file always compiles.
- `"TypeName"` — same-package shorthand for types defined in the same Go package as the generated file (no import is emitted).

### The CustomMarshaler interface

The user-supplied type must implement the wiresmith reverse-write shape:

```go
type CustomMarshaler interface {
    SizeWiresmith() int
    MarshalWiresmith(buf []byte) (int, error)
    UnmarshalWiresmith(buf []byte) error
    EqualWiresmith(other any) bool
    CompareWiresmith(other any) int    // -1/0/+1 like bytes.Compare
}
```

- `SizeWiresmith()` returns the encoded payload length (the wrapper supplies the tag + varint length around it). The generator skips the field entirely when this returns `0`, matching proto3 omit-default semantics on plain bytes/string.
- `MarshalWiresmith(buf)` writes forward into `buf`, which is sized to exactly `SizeWiresmith()` bytes by the generator before the call. Reverse-write happens at the proto-envelope level; the customtype only sees a contiguous slice.
- `UnmarshalWiresmith(buf)` is invoked with the exact payload bytes the decoder sliced out of the wire input. Pointer-receiver implementations are required for fields backed by struct types; value-aliased primitive types (e.g. `type T string`) can use the same shape via `*T`.
- `EqualWiresmith(other any)` is consulted from the generated `Equal()` method. Implementations type-assert and return `false` on mismatch.
- `CompareWiresmith(other any) int` is consulted from the generated `Compare()` method (added by #100). Returns -1/0/+1 like `bytes.Compare` / `strings.Compare`. Implementations type-assert and return a sentinel (commonly `-1`) on type mismatch so the wrapper's Compare stays total.

The interface deliberately uses wiresmith-specific method names so a caller can't accidentally satisfy it with gogoproto's `Marshal()` / `Unmarshal()` shape — those have incompatible signatures.

### Where it applies

Allowed: singular or repeated `bytes`, `string`, and message fields. Each per-element envelope is length-delimited regardless of source kind, so the user type's `SizeWiresmith` / `MarshalWiresmith` / `UnmarshalWiresmith` contract is identical across the three — only the bytes the type owns differ (raw bytes for `bytes`, UTF-8 for `string`, an encoded submessage for `message`).

Rejected (combined compile-time error from `customtypeOption.Validate`):

- Non-bytes/non-string scalars, enums.
- Map fields, oneof variants, proto3 `optional` fields.
- Malformed values: empty string, leading-digit type name, embedded whitespace, etc.
- Combined with any peer wiresmith `FieldOption` not in the customtype whitelist (currently `customname`). Combining customtype with `pointer` or `stdtime` would produce two conflicting Go-shape overrides, so it's rejected at validation time.

The compatibility whitelist lives in `customtypeCompatiblePeers` in `compiler/generator/option_customtype.go`; adding a new peer option means deciding explicitly whether it can ride alongside customtype. `jsontag` is not validated through the same `FieldOption` registry (it changes only the struct tag, not the Go type or wire bytes), so it's implicitly compatible with customtype without needing a whitelist entry.

### Worked examples

- [`proto/basic/customtype.proto`](../proto/basic/customtype.proto) annotates singular and repeated bytes/string fields. The singular cases use the `LabelPairs` / `TenantID` types in [`test/customtypes/customtypes.go`](../test/customtypes/customtypes.go); the repeated cases use `UUID` (fixed-size opaque bytes) and `Tag` (string-backed) — the canonical "I want a typed slice element" patterns.
- [`proto/basic/customtype_message.proto`](../proto/basic/customtype_message.proto) annotates singular and repeated message fields. The `LabelAdapter` type in [`test/customtypes/label_adapter.go`](../test/customtypes/label_adapter.go) writes the inner `Label` submessage's wire payload directly via `google.golang.org/protobuf/encoding/protowire`, demonstrating that the customtype contract is identical across kinds — the user type owns the payload bytes; wiresmith owns the envelope.

## `(wiresmith.options.customname) = "Identifier"`

The customname option overrides the Go identifier the generator picks for the field. By default the proto field `block_id` becomes `BlockId` (initialism mangled); with customname it can be `BlockID` (or any other valid exported Go identifier).

```proto
message BlockMeta {
  string block_id = 1 [(wiresmith.options.customname) = "BlockID"];
}
```

```go
type BlockMeta struct {
    BlockID string `protobuf:"..." json:"block_id,omitempty"`
}

func (m *BlockMeta) GetBlockID() string { ... }
func (m *BlockMeta) HasBlockID() bool   { ... }
```

The rename reaches every consumer the generator emits: struct field declaration, `Get<Name>()` and `Has<Name>()` accessors, marshal/unmarshal access expressions, `Equal()` comparisons, and the field inside a oneof variant's wrapper struct. The wire format is unchanged.

### Scope of the rename

- **Renamed:** the struct field, every `Get*` / `Has*` accessor, and (for oneof variants) the wrapper-struct field.
- **NOT renamed:** the oneof variant wrapper *type* (e.g. `Foo_BlockId` stays anchored to the proto field name even when the variant carries `(wiresmith.options.customname) = "BlockID"`). The wrapper type is the schema-visible Go identity; the user-facing API is the accessor methods, which do follow the override.

### Validation

Values must be valid **exported** Go identifiers — the first character must be an uppercase letter and the rest must be letters, digits, or underscores. A lowercase-first or punctuation-bearing value is rejected at codegen time with a clear diagnostic.

The validation source of truth is `customnameOptionRejection` in `compiler/generator/option_customname.go`.

### Worked example

[`proto/basic/customname.proto`](../proto/basic/customname.proto) exercises the option across scalar, message, repeated, and oneof-variant fields, with an unannotated control showing that default `snake_to_PascalCase` conversion still applies elsewhere.

## `(wiresmith.options.casttype) = "import/path.TypeName"`

The casttype option renames the field's Go type without changing the wire encoding. The marshal/unmarshal hot path keeps the underlying scalar's logic and bridges via Go type conversions at the boundaries — unlike customtype, the user does not implement any wiresmith interface.

```proto
message Request {
  int64  user_id    = 1 [(wiresmith.options.casttype) = "github.com/myapp/ids.UserID"];
  string tenant_tag = 2 [(wiresmith.options.casttype) = "github.com/myapp/ids.TenantTag"];
  bytes  payload    = 3 [(wiresmith.options.casttype) = "github.com/myapp/ids.Payload"];
}
```

```go
type Request struct {
    UserID    ids.UserID    `protobuf:"varint,1,opt,name=user_id,proto3"`
    TenantTag ids.TenantTag `protobuf:"bytes,2,opt,name=tenant_tag,proto3"`
    Payload   ids.Payload   `protobuf:"bytes,3,opt,name=payload,proto3"`
}
```

The aliased Go type must be a defined type or alias over the proto field's natural Go shape — e.g. `type UserID int64`, `type TenantTag string`, `type Payload []byte`. The generator inserts the necessary casts at every emit site:

- **Size / Marshal** rely on Go's automatic conversion of defined integer / slice types when used in arithmetic (`uint64(m.UserID)`) or builtin calls (`len(m.TenantTag)`).
- **Unmarshal** wraps the underlying scalar's CastExpr with the user alias: `m.UserID = ids.UserID(int64(v))`, `m.TenantTag = ids.TenantTag(string(dAtA[iNdEx:postIndex]))`. The Go compiler folds the redundant inner cast at compile time.
- **Equal / Compare** delegate to the scalar's `==` / `<` for kinds Go can compare directly; `bytes` casttype falls back to `bytes.Equal([]byte(a), []byte(b))` / `bytes.Compare([]byte(a), []byte(b))` because the stdlib helpers do not accept defined slice types.

### Value format

Same shape as customtype: `"import/path.TypeName"` for an external type (the import is registered automatically with an explicit alias derived from the path base) or `"TypeName"` for a same-package type. Whitespace inside the value and trailing `/path` with no `.TypeName` suffix are rejected at codegen time.

### Where it applies (v1)

Allowed: singular fields of kind int32, int64, uint32, uint64, sint32, sint64, fixed32, sfixed32, fixed64, sfixed64, bool, string, bytes.

Rejected (combined compile-time error from `casttypeOption.Validate`):

- Float and double — bit-cast handling (`math.Float*bits`) does not accept a defined-type argument; supporting it cleanly requires changing every emit site rather than wrapping at the FieldType boundary. Filed as a follow-up if a real use case surfaces.
- Enum — defer until casttype semantics on enum-valued fields are decided (do we want `type Severity int32` or do we want a stronger guarantee that casttype values match declared enum constants?).
- Message — use `(wiresmith.options.customtype)` for message-valued user types; casttype's "same wire, different name" contract does not generalise to submessages.
- Map, oneof, and proto3 `optional` fields. Filed as follow-up beads if needed.

### Difference from customtype

`(wiresmith.options.customtype)` and `(wiresmith.options.casttype)` are deliberately separate options because they serve different needs:

- **customtype**: the user type **owns the wire encoding** (must implement `SizeWiresmith`/`MarshalWiresmith`/`UnmarshalWiresmith`/`EqualWiresmith`/`CompareWiresmith`). Use when the Go representation differs structurally from the proto kind (e.g. a pooled `PreallocBytes` wrapper, a `LabelAdapter` with custom serialization).
- **casttype**: the user type **shares the underlying** (no methods required). Use for type-safe identifier aliases like `type UserID int64`, `type AccountID int64` where the wire encoding is unchanged but the Go type system catches accidental cross-domain assignment.

### Worked example

[`proto/basic/casttype.proto`](../proto/basic/casttype.proto) annotates an int64, a string, and a bytes field next to plain controls. [`test/casttypes/casttypes.go`](../test/casttypes/casttypes.go) declares the trivial alias types, and [`test/basic/casttype_test.go`](../test/basic/casttype_test.go) pins the field-type swap, round-trip, wire-compatibility with the unannotated control, Equal/Compare, and nil-safe getter invariants documented above.

## `(wiresmith.options.stdtime) = true`

The stdtime option swaps a `google.protobuf.Timestamp` field for a stdlib `time.Time` value. Wire format is unchanged — the value is still encoded as the standard Timestamp sub-message (int64 `seconds` field 1, int32 `nanos` field 2) so peers using any other proto library see the same bytes.

```proto
syntax = "proto3";
package myapp.v1;

import "wiresmith/options.proto";
import "google/protobuf/timestamp.proto";

message Snapshot {
  string name    = 1;
  uint64 version = 2;

  // Generated Go: Created time.Time
  google.protobuf.Timestamp created = 3 [(wiresmith.options.stdtime) = true];
}
```

```go
type Snapshot struct {
    Name    string
    Version uint64
    Created time.Time `protobuf:"bytes,3,opt,name=created,proto3" json:"created,omitempty"`
}
```

### Zero-value presence (gogoproto-compatible)

Go's zero `time.Time{}` (January 1, year 1 UTC — the only value for which `IsZero()` returns true) is treated as "field not set". On marshal, no tag is emitted. On unmarshal, an absent tag leaves the field as the Go zero.

This matches `gogoproto.stdtime`'s contract and means **the Unix epoch is distinct from "unset"**: `time.Unix(0, 0).UTC()` is not the Go zero, so it marshals as an explicit empty Timestamp payload (`0x1a 0x00` — tag, length 0) and decodes back to the same instant. If you need to distinguish the literal year-1 zero from "no value", store presence alongside the time.

Presence is carried entirely by `time.Time.IsZero()` — wiresmith does **not** emit a `Has<Name>()` accessor for stdtime fields (unlike most other singular non-oneof fields where presence rides on a bitmap). Callers check `m.GetCreated().IsZero()` instead of `m.HasCreated()`.

### UTC normalization

Decoded times are always in UTC. UTC is the canonical Timestamp timezone in the proto spec; encoded `seconds + nanos` carry no zone information, so the decoder picks UTC to keep the round-trip independent of the writer's local zone. Code that needs a local zone after decode should call `.In(loc)` itself.

### Where it applies (v1)

Allowed: singular `google.protobuf.Timestamp` fields.

Rejected (combined compile-time error from `validateStdtimeOptions`):

- Non-Timestamp message fields, scalars, enums, and `bytes`/`string`. Error: `(wiresmith.options.stdtime) only applies to google.protobuf.Timestamp fields, got <kind-or-name>`.
- Map, oneof, repeated, and proto3 `optional` fields. Filed as follow-up beads if a real Mimir / Tempo use case surfaces.
- Combination with `(wiresmith.options.pointer)`. The two options produce conflicting Go shapes (`*time.Time` vs `time.Time`); the generator refuses to pick one for you.

### Protoreflect compatibility caveat

The generated `*_reflect.pb.go` still describes the field as `google.protobuf.Timestamp` (via the file descriptor's `rawDesc`) and references `google.golang.org/protobuf/types/known/timestamppb` for the goTypes table. The Go struct field is `time.Time`, so any code that introspects this field via `protoreflect.Message.Get`/`Set` or routes through google.golang.org/protobuf's reflection machinery will not see a normal `*Timestamp` slot — the field is opaque from protoreflect's perspective. Same trade-off `gogoproto.stdtime` makes; for Mimir/Tempo's marshal-focused usage this is fine, and for protoreflect-driven code the field should stay un-annotated.

### Worked example

[`proto/basic/stdtime.proto`](../proto/basic/stdtime.proto) annotates a Timestamp field next to stock scalar controls, and [`test/basic/stdtime_test.go`](../test/basic/stdtime_test.go) pins the round-trip / zero-presence / UTC-normalization / cross-library wire-format invariants documented above.

## `(wiresmith.options.stdduration) = true`

The stdduration option swaps a `google.protobuf.Duration` field for a stdlib `time.Duration` value. Wire format is unchanged — the value is still encoded as the standard Duration sub-message (int64 `seconds` field 1, int32 `nanos` field 2). Mirrors `gogoproto.stdduration` and pairs with `stdtime` on the Timestamp side.

```proto
syntax = "proto3";
package myapp.v1;

import "wiresmith/options.proto";
import "google/protobuf/duration.proto";

message Query {
  string name    = 1;
  uint32 retries = 2;

  // Generated Go: Lookback time.Duration
  google.protobuf.Duration lookback = 3 [(wiresmith.options.stdduration) = true];
}
```

```go
type Query struct {
    Name     string
    Retries  uint32
    Lookback time.Duration `protobuf:"bytes,3,opt,name=lookback,proto3" json:"lookback,omitempty"`
}
```

### Zero-value presence

`time.Duration(0)` is treated as "field not set" — proto3 default-suppression applied to the whole Duration envelope. On marshal, no tag is emitted; on unmarshal, an absent tag leaves the field at zero.

Unlike stdtime, `time.Duration` has only one zero value, so there is **no "explicit zero vs unset" distinction**. A wire payload encoding `seconds=0 nanos=0` (which can arise from a peer that marshals an empty Duration submessage) decodes to `time.Duration(0)`, the same value a never-set field produces. If you need to distinguish "explicit zero" from "unset", store presence alongside the duration.

Presence is carried entirely by the value (`d != 0`) — wiresmith does **not** emit a `Has<Name>()` accessor for stdduration fields, same shape as stdtime.

### Overflow saturation

`time.Duration` is int64 nanoseconds and tops out at ~292 years; proto Duration permits up to ~10000 years. A payload whose `seconds * 1e9 + nanos` does not fit decodes to `math.MaxInt64` (or `math.MinInt64` on negative overflow) rather than wrapping silently — matches `(*durationpb.Duration).AsDuration()` in `google.golang.org/protobuf`. See `protohelpers.DecodeStdDuration`.

### Where it applies (v1)

Allowed: singular `google.protobuf.Duration` fields.

Rejected (combined compile-time error from `stddurationOption.Validate`):

- Non-Duration message fields, scalars, enums, and `bytes`/`string`. Error: `(wiresmith.options.stdduration) only applies to google.protobuf.Duration fields, got <kind-or-name>`.
- Map, oneof, repeated, and proto3 `optional` fields. Filed as follow-up beads if a real Mimir / Tempo use case surfaces.
- Combination with `(wiresmith.options.pointer)`. The two options produce conflicting Go shapes (`*time.Duration` vs `time.Duration`); the generator refuses to pick one for you.

### Protoreflect compatibility caveat

Same caveat as stdtime: the `*_reflect.pb.go` describes the field as `google.protobuf.Duration` (referencing `google.golang.org/protobuf/types/known/durationpb`) while the Go struct field is `time.Duration`. Code routing through `protoreflect.Message.Get`/`Set` will not see a normal `*Duration` slot — the field is opaque from protoreflect's perspective. Matches the `gogoproto.stdduration` trade-off.

### Worked example

[`proto/basic/stdtime.proto`](../proto/basic/stdtime.proto) (shared with stdtime) declares a `StdDurationHolder` message with an annotated `lookback` field, and [`test/basic/stdduration_test.go`](../test/basic/stdduration_test.go) pins the round-trip / zero-presence / negative / truncation-boundary / cross-library wire-format invariants documented above.
