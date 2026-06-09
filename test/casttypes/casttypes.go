// Package casttypes provides the user-defined Go alias types referenced by
// proto/basic/casttype.proto via (wiresmith.options.casttype).
//
// Unlike customtype, casttype targets a *defined type over the proto field's
// natural Go shape* (int64, string, []byte, …). The Go compiler bridges to
// the underlying via casts; no marshaler interface is required.
//
// The types here are intentionally trivial — they exist to verify the
// generator emits the right casts at the marshal/unmarshal/equal/compare
// boundaries, not to test any user-side logic.
package casttypes

// UserID is a defined int64 alias — the canonical casttype use case (e.g.
// `gogoproto.casttype = "UserID"` on `int64 user_id`).
type UserID int64

// TenantTag is a defined string alias for tag-style identifiers.
type TenantTag string

// Payload is a defined []byte alias. The bytes case is the trickiest of the
// three: stdlib helpers like bytes.Equal and bytes.Compare don't auto-
// convert defined slice types, so the generator emits an explicit
// []byte(...) cast at Equal/Compare sites.
type Payload []byte
