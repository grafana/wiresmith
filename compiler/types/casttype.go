package types

import (
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// CastType wraps a scalar Type with a user-supplied Go alias name. The wire
// encoding is the underlying scalar's verbatim — only the Go-side
// declaration and the unmarshal cast change. EmitSize and EmitMarshal
// delegate straight to Inner because Go's type system already converts a
// defined integer type to its underlying when passed through `uint64(...)`
// or used in arithmetic / comparison expressions; the wrapper only needs to
// intervene at the unmarshal assignment and (for bytes) at Equal/Compare.
//
// v1 scope: integer scalars (int32/int64/uint32/uint64, sint32/sint64,
// fixed32/sfixed32/fixed64/sfixed64), bool, string, bytes. Float/double are
// rejected at validation time because math.Float*bits — used by the float
// emit path for bit-exact -0.0/NaN preservation — does not accept a
// defined-type argument; supporting it would require touching every
// emit site rather than wrapping at the FieldType boundary.
type CastType struct {
	Inner   Type
	GoAlias string // e.g. "github.com/myapp/pkg.UserID" → printed via the registered import alias
}

// RequiredImports forwards Inner's imports unchanged — the cast itself
// pulls in nothing beyond what the scalar already needs (the alias's own
// import is registered by the option layer, not by this wrapper).
func (c *CastType) RequiredImports() []string { return c.Inner.RequiredImports() }

// EmitSize delegates to Inner. The wire-size accumulator reads access via
// the underlying scalar's API (e.g. `uint64(m.X)` for integers, `len(m.X)`
// for length-delimited), and Go handles the defined-type-to-underlying
// conversion implicitly at the call site.
func (c *CastType) EmitSize(e Emitter, access string, tagSize int) {
	c.Inner.EmitSize(e, access, tagSize)
}

// EmitMarshal delegates to Inner for the same reason as EmitSize. The
// reverse-write path encodes from access through whatever the scalar's put
// expression is (uint64 cast, copy for slices, etc.) — all of which Go
// allows on defined types over the right underlying.
func (c *CastType) EmitMarshal(e Emitter, access string, num protowire.Number) {
	c.Inner.EmitMarshal(e, access, num)
}

// EmitUnmarshal consumes via Inner's wire-format primitive, then wraps the
// per-kind CastExpr with the user alias. Combining the two casts looks
// like `MyAlias(int64(v))` or `MyAlias(string(dAtA[iNdEx:postIndex]))` —
// Go folds the redundant inner cast away at compile time, so the runtime
// is identical to the un-aliased path.
//
// For length-delimited kinds (string, bytes) the source is the payload
// slice instead of the varint `v`, and we re-emit the `iNdEx = postIndex`
// advance that the underlying type's EmitUnmarshal would have produced.
func (c *CastType) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	c.Inner.EmitConsume(e)
	lengthDelimited := c.Inner.WireType() == "protowire.BytesType"
	source := "v"
	if lengthDelimited {
		source = "dAtA[iNdEx:postIndex]"
	}
	innerCast := c.Inner.CastExpr(source, ctx)
	e.Writef("\t\t\t%s = %s(%s)\n", access, c.GoAlias, innerCast)
	if lengthDelimited {
		e.Writef("\t\t\tiNdEx = postIndex\n")
	}
}

// EmitEqual delegates to Inner for everything except `bytes`, where the
// stdlib bytes.Equal requires its arguments to be `[]byte` exactly (Go
// does not auto-convert defined slice types as call arguments). The
// explicit `[]byte(MyAlias)` cast bridges the two.
func (c *CastType) EmitEqual(e Emitter, indent, lhs, rhs string) {
	if _, isBytes := c.Inner.(*BytesType); isBytes {
		e.AddImport("bytes", "")
		e.Writef("%sif !bytes.Equal([]byte(%s), []byte(%s)) {\n%s\treturn false\n%s}\n", indent, lhs, rhs, indent, indent)
		return
	}
	c.Inner.EmitEqual(e, indent, lhs, rhs)
}

// EmitCompare mirrors EmitEqual: bytes.Compare needs explicit `[]byte`
// arguments; everything else delegates because Go's ordered comparisons
// (`<`, `>`) work on defined types over an ordered underlying.
func (c *CastType) EmitCompare(e Emitter, indent, lhs, rhs string) {
	if _, isBytes := c.Inner.(*BytesType); isBytes {
		e.AddImport("bytes", "")
		e.Writef("%sif c := bytes.Compare([]byte(%s), []byte(%s)); c != 0 {\n%s\treturn c\n%s}\n", indent, lhs, rhs, indent, indent)
		return
	}
	c.Inner.EmitCompare(e, indent, lhs, rhs)
}

// CasttypeAllowed reports whether kind is in the v1 scope for casttype.
// Centralised here so the option's validation list and any future emitter
// asserting the FieldType wrapper stay in agreement.
func CasttypeAllowed(kind protoreflect.Kind) bool {
	switch kind {
	case protoreflect.Int32Kind, protoreflect.Int64Kind,
		protoreflect.Uint32Kind, protoreflect.Uint64Kind,
		protoreflect.Sint32Kind, protoreflect.Sint64Kind,
		protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind,
		protoreflect.BoolKind,
		protoreflect.StringKind, protoreflect.BytesKind:
		return true
	}
	return false
}
