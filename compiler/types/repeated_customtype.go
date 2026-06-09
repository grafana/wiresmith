package types

import (
	"google.golang.org/protobuf/encoding/protowire"
)

// RepeatedCustomType is the FieldType for a repeated bytes, string, or
// message field annotated with (wiresmith.options.customtype). Each
// per-element envelope is length-delimited regardless of source kind
// (wire type 2: tag + varint length + payload), so the emit shape is
// kind-agnostic — the user-supplied Go type owns the payload bytes via
// Size/Marshal/Unmarshal/Equal/CompareWiresmith, the same contract the
// singular CustomType uses, scaled out into a per-element loop.
//
// GoType is the Go expression used to spell a fresh zero value during
// unmarshal (e.g. "LabelAdapter" for a same-package type or
// "customtypes.LabelAdapter" for an imported one). The customtype option
// resolves the alias before constructing this type so the emit sites stay
// import-agnostic.
type RepeatedCustomType struct {
	GoType string
}

// RequiredImports declares "fmt" because EmitMarshal asserts the user's
// MarshalWiresmith returned exactly SizeWiresmith() bytes via fmt.Errorf.
// Matches CustomType.RequiredImports for the same reason — every wiresmith
// file that uses customtype is also a fmt user, but declaring it here
// keeps the import sound in any future file where the only fmt user is a
// repeated customtype.
func (r *RepeatedCustomType) RequiredImports() []string { return []string{"fmt"} }

// EmitSize emits a per-element loop accumulating tag + varint length +
// payload size for every entry in the slice. Unlike the singular
// CustomType, we emit each element unconditionally — proto3 repeated
// semantics preserve every slice element on the wire (a zero-payload
// element appears as `tag + 0`), and skipping `SizeWiresmith()==0`
// elements would silently drop them on re-marshal.
func (r *RepeatedCustomType) EmitSize(e Emitter, access string, tagSize int) {
	e.Writef("\tfor i := range %s {\n", access)
	e.Writef("\t\ts := %s[i].SizeWiresmith()\n", access)
	e.Writef("\t\tn += %d + protowire.SizeVarint(uint64(s)) + s\n", tagSize)
	e.Writef("\t}\n")
}

// EmitMarshal walks the slice in reverse (the reverse-write convention
// every other repeated emitter uses) and writes payload, varint length,
// and field tag for each element. Every element is emitted, including
// those whose SizeWiresmith reports 0 — same rationale as EmitSize.
func (r *RepeatedCustomType) EmitMarshal(e Emitter, access string, num protowire.Number) {
	e.Writef("\tfor iNdEx := len(%s) - 1; iNdEx >= 0; iNdEx-- {\n", access)
	e.Writef("\t\ts := %s[iNdEx].SizeWiresmith()\n", access)
	e.Writef("\t\ti -= s\n")
	e.Writef("\t\tif s > 0 {\n")
	e.Writef("\t\t\tn, err := %s[iNdEx].MarshalWiresmith(dAtA[i : i+s])\n", access)
	e.Writef("\t\t\tif err != nil {\n")
	e.Writef("\t\t\t\treturn 0, err\n")
	e.Writef("\t\t\t}\n")
	e.Writef("\t\t\tif n != s {\n")
	e.Writef("\t\t\t\treturn 0, fmt.Errorf(\"%s[iNdEx].MarshalWiresmith returned %%d bytes, expected %%d\", n, s)\n", access)
	e.Writef("\t\t\t}\n")
	e.Writef("\t\t}\n")
	e.Writef("\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(s))\n")
	e.ReverseTag("\t\t", num, protowire.BytesType)
	e.Writef("\t}\n")
}

// EmitUnmarshal decodes one element into a fresh local of the user type,
// then appends. Going through a local (rather than appending an empty
// composite literal first and indexing back) keeps the emitter syntax-
// agnostic across user type shapes: named primitive aliases (`type Tag
// string`) can't be spelled as `Tag{}`, but `var elem Tag` works for any
// kind. The local is addressable, so a pointer-receiver
// UnmarshalWiresmith binds without an explicit `&`.
//
// Allocating per-element (rather than mutating a reused scratch) matches
// the proto3 "each occurrence is a fresh value" decode contract.
func (r *RepeatedCustomType) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	emitConsumeBytesLen(e)
	e.Writef("\t\t\tvar elem %s\n", r.GoType)
	e.Writef("\t\t\tif err := elem.UnmarshalWiresmith(dAtA[iNdEx:postIndex]); err != nil {\n")
	e.Writef("\t\t\t\treturn err\n")
	e.Writef("\t\t\t}\n")
	e.Writef("\t\t\t%s = append(%s, elem)\n", access, access)
	e.Writef("\t\t\tiNdEx = postIndex\n")
}

// EmitEqual emits a len-check + index loop and defers per-element
// comparison to the user type's EqualWiresmith. Matches RepeatedField's
// emit shape so a customtype repeated field reads the same way as any
// other repeated kind from the call-site's perspective.
func (r *RepeatedCustomType) EmitEqual(e Emitter, indent, lhs, rhs string) {
	e.Writef("%sif len(%s) != len(%s) {\n%s\treturn false\n%s}\n", indent, lhs, rhs, indent, indent)
	e.Writef("%sfor i := range %s {\n", indent, lhs)
	e.Writef("%s\tif !%s[i].EqualWiresmith(%s[i]) {\n%s\t\treturn false\n%s\t}\n", indent, lhs, rhs, indent, indent)
	e.Writef("%s}\n", indent)
}

// EmitCompare emits a length-ordered comparison and an element-wise loop
// that delegates to the user type's CompareWiresmith. Mirrors
// RepeatedField.EmitCompare: shorter slice sorts first, then the first
// differing element decides ordering.
func (r *RepeatedCustomType) EmitCompare(e Emitter, indent, lhs, rhs string) {
	emitLenOrderingGuard(e, indent, lhs, rhs)
	e.Writef("%sfor i := range %s {\n", indent, lhs)
	e.Writef("%s\tif c := %s[i].CompareWiresmith(%s[i]); c != 0 {\n", indent, lhs, rhs)
	e.Writef("%s\t\treturn c\n", indent)
	e.Writef("%s\t}\n", indent)
	e.Writef("%s}\n", indent)
}
