package generator

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Clone() deep-copy methods are emitted into the consolidated cold companion
// `_util.pb.go` (alongside String() and the reflection glue), never the hot
// main `.pb.go`. Clone is cold — callers that need a deep copy reach for it
// off the hot path (e.g. a combiner deep-copying a response) — so it follows
// the same icache/iTLB split as the other companions.
//
// Motivation (wiresmith-oz2l): consumers that deep-copy proto values otherwise
// have no non-reflection option. reflection-based proto.Clone is unsupported by
// design (wiresmith's ProtoReflect bridge panics on value-typed message fields)
// and a wire round-trip (Marshal+Unmarshal) is both slow and lossy
// ([]T{}→nil). A generated, per-field Clone() removes both costs.
//
// Shape: `func (m *T) Clone() *T`. A nil receiver returns nil. Otherwise a
// fresh message is allocated and every field is deep-copied — the per-field
// rules live in compiler/types/clone.go (EmitClone per FieldType); oneofs are
// reconstructed here by rebuilding the concrete variant with a cloned payload;
// the XXX_fieldsPresent bitmap is a value array, copied wholesale so presence
// round-trips. clone.Equal(orig) holds for every generated message.
func (fg *FileGenerator) emitAllCloneMethods(fd protoreflect.FileDescriptor) {
	ce := &cloneEmitter{fg: fg}
	forEachMessage(fd, func(md protoreflect.MessageDescriptor) {
		fg.emitClone(ce, md)
	})
}

func (fg *FileGenerator) emitClone(ce *cloneEmitter, md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	out := fg.utilBody

	fmt.Fprintf(out, "func (m *%s) Clone() *%s {\n", name, name)
	fmt.Fprintf(out, "\tif m == nil {\n\t\treturn nil\n\t}\n")
	fmt.Fprintf(out, "\tout := &%s{}\n", name)

	// The presence bitmap is a fixed-size [N]uint64 array (a value type), so a
	// plain assignment copies it wholesale and presence round-trips. Emitted
	// before the field copies so it reads as "snapshot the message, then deep
	// copy the reference-bearing fields".
	if fg.presenceBitmapWords(md) > 0 {
		fmt.Fprintf(out, "\tout.XXX_fieldsPresent = m.XXX_fieldsPresent\n")
	}

	seenOneofs := map[string]bool{}
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)

		if isRealOneof(fd) {
			oo := fd.ContainingOneof()
			ooName := string(oo.Name())
			if !seenOneofs[ooName] {
				seenOneofs[ooName] = true
				fg.emitOneofClone(ce, md, oo)
			}
			continue
		}

		goName := fg.goFieldName(fd)
		ft := fg.fieldType(fd)
		ft.EmitClone(ce, "\t", "out."+goName, "m."+goName)
	}

	fmt.Fprintf(out, "\treturn out\n")
	fmt.Fprintf(out, "}\n\n")
}

// emitOneofClone reconstructs the set oneof variant with a deep-copied payload.
// A nil oneof interface (unset) is left as the zero value `out` already has.
func (fg *FileGenerator) emitOneofClone(ce *cloneEmitter, md protoreflect.MessageDescriptor, oo protoreflect.OneofDescriptor) {
	goName := snakeToPascal(string(oo.Name()))
	out := fg.utilBody

	fmt.Fprintf(out, "\tswitch v := m.%s.(type) {\n", goName)
	for i := 0; i < oo.Fields().Len(); i++ {
		fd := oo.Fields().Get(i)
		variantType := oneofVariantName(md, fd)
		fieldName := fg.goFieldName(fd)
		fmt.Fprintf(out, "\tcase *%s:\n", variantType)
		fmt.Fprintf(out, "\t\tout.%s = &%s{%s: %s}\n",
			goName, variantType, fieldName, fg.oneofCloneExpr(ce, fd, "v."+fieldName))
	}
	fmt.Fprintf(out, "\t}\n")
}

// oneofCloneExpr returns the deep-copy expression for a oneof variant payload.
// Oneof variants are never option-substituted (stdtime/customtype/pointer all
// reject oneof), so a message payload is always a wiresmith value message and
// bytes is a plain []byte.
func (fg *FileGenerator) oneofCloneExpr(ce *cloneEmitter, fd protoreflect.FieldDescriptor, access string) string {
	switch fd.Kind() {
	case protoreflect.MessageKind:
		// Value message payload: deep-clone and store the dereferenced value.
		return "*" + access + ".Clone()"
	case protoreflect.BytesKind:
		ce.AddImport("slices", "")
		return "slices.Clone(" + access + ")"
	default:
		// Scalars, string, enum: value copy.
		return access
	}
}

// cloneEmitter routes types.EmitClone callbacks into the consolidated
// `_util.pb.go` buffer/imports (the same destination as String() and the
// reflection glue). Clone never emits wire tags, so ReverseTag is unreachable
// and panics if anything ever reaches it.
type cloneEmitter struct{ fg *FileGenerator }

func (ce *cloneEmitter) Writef(format string, args ...any) {
	fmt.Fprintf(ce.fg.utilBody, format, args...)
}

func (ce *cloneEmitter) AddImport(path, alias string) {
	ce.fg.utilImports.addImport(path, alias)
}

func (ce *cloneEmitter) ReverseTag(indent string, num protowire.Number, wt protowire.Type) {
	panic("cloneEmitter.ReverseTag: Clone must not emit wire tags")
}
