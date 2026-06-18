package types

// EmitClone implementations for every FieldType, collected in one file so the
// deep-copy contract for the whole type system reads top-to-bottom. The
// generator drives these from compiler/generator/emit_clone.go, which emits the
// enclosing `func (m *T) Clone() *T` (nil-receiver returns nil; allocates a
// fresh message and deep-copies every field) into the cold `_util.pb.go`.
//
// The contract per kind:
//   - scalars / string / enum: value copy (`dst = src`). Strings are immutable,
//     so sharing the backing array is safe.
//   - bytes: fresh backing array via slices.Clone (never alias); slices.Clone
//     preserves the nil-vs-empty distinction (nil stays nil, []byte{} stays a
//     non-nil empty slice), which the optional-bytes round-trip relies on.
//   - singular value message: `dst = *src.Clone()` — recurse through the
//     nested message's own deep Clone(), then store the value.
//   - pointer / optional message (*Msg): `dst = src.Clone()` (Clone is nil-safe).
//   - optional scalar (*T): reallocate a fresh pointee, preserving nil.
//   - repeated / map: allocate fresh backing storage and deep-copy each element
//     (slices.Clone for the slice header, then per-element recursion for
//     reference-bearing element kinds).
//   - customtype / casttype: value copy. wiresmith cannot see a user type's
//     internal invariants, so it copies the Go value verbatim — matching the
//     EmitEqual/EmitCompare delegation contract. A customtype/casttype whose Go
//     representation holds a reference (e.g. a `type T []byte` casttype, or a
//     customtype wrapping a slice/pointer) is value-copied and will alias; such
//     types must be value-copy-safe or treated as immutable. (casttype over the
//     bytes kind is special-cased below to clone the backing array, since that
//     is the one casttype shape with an obvious reference.)

// emitValueCopy is the shared `dst = src` form used by every value-copy-safe
// leaf kind.
func emitValueCopy(e Emitter, indent, dst, src string) {
	e.Writef("%s%s = %s\n", indent, dst, src)
}

// cloneNeedsDeepElement reports whether a repeated/composite element kind needs
// per-element deep copying after the slice header is cloned. Value-copy-safe
// kinds (scalars, string, enum) are fully handled by slices.Clone; bytes and
// message elements alias their backing storage and must be recursed into.
func cloneNeedsDeepElement(t Type) bool {
	switch t.(type) {
	case *MessageType, *BytesType:
		return true
	default:
		return false
	}
}

func (varintBase) EmitClone(e Emitter, indent, dst, src string) { emitValueCopy(e, indent, dst, src) }
func (BoolType) EmitClone(e Emitter, indent, dst, src string)   { emitValueCopy(e, indent, dst, src) }
func (Sint32Type) EmitClone(e Emitter, indent, dst, src string) { emitValueCopy(e, indent, dst, src) }
func (Sint64Type) EmitClone(e Emitter, indent, dst, src string) { emitValueCopy(e, indent, dst, src) }
func (f fixed32Base) EmitClone(e Emitter, indent, dst, src string) {
	emitValueCopy(e, indent, dst, src)
}
func (f fixed64Base) EmitClone(e Emitter, indent, dst, src string) {
	emitValueCopy(e, indent, dst, src)
}
func (StringType) EmitClone(e Emitter, indent, dst, src string) { emitValueCopy(e, indent, dst, src) }

// stdtime/stdduration are time.Time / time.Duration value types — a plain copy
// is the correct, full deep copy (time.Time's internal *Location is immutable
// and shared by design, exactly as the stdlib copies it).
func (StdtimeType) EmitClone(e Emitter, indent, dst, src string) { emitValueCopy(e, indent, dst, src) }
func (StdDurationType) EmitClone(e Emitter, indent, dst, src string) {
	emitValueCopy(e, indent, dst, src)
}

// BytesType clones the backing array (never alias) via slices.Clone, which
// preserves nil-vs-empty so the field round-trips through Equal.
func (BytesType) EmitClone(e Emitter, indent, dst, src string) {
	e.AddImport("slices", "")
	e.Writef("%s%s = slices.Clone(%s)\n", indent, dst, src)
}

// MessageType (singular value message field) recurses through the nested
// message's nil-safe Clone() and stores the dereferenced value. src is always
// addressable at the call sites (struct field, slice index, or addressable map
// loop variable), so the pointer-receiver Clone binds without an explicit `&`.
func (MessageType) EmitClone(e Emitter, indent, dst, src string) {
	e.Writef("%s%s = *%s.Clone()\n", indent, dst, src)
}

// CustomType copies the user value verbatim — see the file header contract.
func (c *CustomType) EmitClone(e Emitter, indent, dst, src string) {
	emitValueCopy(e, indent, dst, src)
}

// CastType copies by value, except for the bytes underlying kind: a casttype
// over bytes (`type T []byte`) holds a slice, so a value copy would alias the
// backing array. slices.Clone is generic over `~[]E`, so it preserves the
// defined alias type and needs no conversion (nor the alias's import).
func (c *CastType) EmitClone(e Emitter, indent, dst, src string) {
	if _, isBytes := c.Inner.(*BytesType); isBytes {
		e.AddImport("slices", "")
		e.Writef("%s%s = slices.Clone(%s)\n", indent, dst, src)
		return
	}
	emitValueCopy(e, indent, dst, src)
}

// OptionalField mirrors EmitEqual's three-shape dispatch.
func (o *OptionalField) EmitClone(e Emitter, indent, dst, src string) {
	if _, isBytes := o.Inner.(*BytesType); isBytes {
		// []byte is already nullable; slices.Clone preserves nil-vs-empty.
		e.AddImport("slices", "")
		e.Writef("%s%s = slices.Clone(%s)\n", indent, dst, src)
		return
	}
	if _, isMsg := o.Inner.(*MessageType); isMsg {
		// *Msg: nil-safe Clone preserves nil.
		e.Writef("%s%s = %s.Clone()\n", indent, dst, src)
		return
	}
	// Scalar optional (*T): reallocate a fresh pointee, preserving nil.
	e.Writef("%sif %s != nil {\n", indent, src)
	e.Writef("%s\ttmp := *%s\n", indent, src)
	e.Writef("%s\t%s = &tmp\n", indent, dst)
	e.Writef("%s}\n", indent)
}

// PointerField (singular *Msg) delegates to the nil-safe Clone.
func (p *PointerField) EmitClone(e Emitter, indent, dst, src string) {
	e.Writef("%s%s = %s.Clone()\n", indent, dst, src)
}

// RepeatedField clones the slice header (slices.Clone, which preserves nil and
// allocates a fresh backing array) and then deep-copies each element in place
// for reference-bearing element kinds (bytes, value message). Value-copy-safe
// element kinds (scalars, string, enum) are fully handled by slices.Clone.
func (r *RepeatedField) EmitClone(e Emitter, indent, dst, src string) {
	e.AddImport("slices", "")
	e.Writef("%s%s = slices.Clone(%s)\n", indent, dst, src)
	if cloneNeedsDeepElement(r.Inner) {
		e.Writef("%sfor i := range %s {\n", indent, dst)
		r.Inner.EmitClone(e, indent+"\t", dst+"[i]", src+"[i]")
		e.Writef("%s}\n", indent)
	}
}

// RepeatedPointer ([]*Msg) clones the slice header, then replaces each pointer
// with a deep clone of its pointee (Clone is nil-safe, so nil entries survive).
func (r *RepeatedPointer) EmitClone(e Emitter, indent, dst, src string) {
	e.AddImport("slices", "")
	e.Writef("%s%s = slices.Clone(%s)\n", indent, dst, src)
	e.Writef("%sfor i := range %s {\n", indent, dst)
	e.Writef("%s\t%s[i] = %s[i].Clone()\n", indent, dst, dst)
	e.Writef("%s}\n", indent)
}

// RepeatedCustomType ([]CustomT) clones the slice header; elements are copied by
// value (the customtype contract — see the file header).
func (r *RepeatedCustomType) EmitClone(e Emitter, indent, dst, src string) {
	e.AddImport("slices", "")
	e.Writef("%s%s = slices.Clone(%s)\n", indent, dst, src)
}

// MapField allocates a fresh map and deep-copies each value (keys are scalar,
// copied by value). The nil guard preserves a nil map (vs an empty one).
func (m *MapField) EmitClone(e Emitter, indent, dst, src string) {
	e.Writef("%sif %s != nil {\n", indent, src)
	e.Writef("%s\t%s = make(%s, len(%s))\n", indent, dst, m.MapType, src)
	e.Writef("%s\tfor k, v := range %s {\n", indent, src)
	m.Val.EmitClone(e, indent+"\t\t", dst+"[k]", "v")
	e.Writef("%s\t}\n", indent)
	e.Writef("%s}\n", indent)
}
