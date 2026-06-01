package generator

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/reflect/protoreflect"

	"wiresmith/compiler/types"
)

// emitAllCompareMethods emits `Compare(that interface{}) int` on every
// message in fd that the Generator's compareSet has flagged. The set is
// the closure over direct opt-ins via `(wiresmith.options.compare)` /
// `(wiresmith.options.compare_all)` and the messages they reach through
// message-typed fields. Compare returns -1/0/+1 like bytes.Compare and
// is the gogo-equivalent of `(gogoproto.compare) = true`.
//
// Opt-in is required because always-emit added ~9% to OTel hot-path
// benchmarks via icache pressure on the linked binary even though
// Compare itself is never called on those paths. Users who don't need
// Compare pay nothing.
func (fg *FileGenerator) emitAllCompareMethods(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, func(md protoreflect.MessageDescriptor) {
		if !fg.shouldEmitCompare(md) {
			return
		}
		fg.emitCompare(md)
	})
}

// emitCompare emits the Compare method for one message. The nil-handling
// preamble matches gogo's pattern verbatim so callers that depend on
// `nil.Compare(non-nil) == -1` / `non-nil.Compare(nil) == 1` /
// `nil.Compare(nil) == 0` / `m.Compare("wrong type") == 1` keep their
// existing expectations. Fields walk in ascending wire tag (not
// declaration order) so the per-field ordering is stable against proto
// file reorderings that don't change the wire format.
func (fg *FileGenerator) emitCompare(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)

	fmt.Fprintf(fg.body, "func (this *%s) Compare(that interface{}) int {\n", name)
	fmt.Fprintf(fg.body, "\tif that == nil {\n\t\tif this == nil {\n\t\t\treturn 0\n\t\t}\n\t\treturn 1\n\t}\n\n")

	fmt.Fprintf(fg.body, "\tthat1, ok := that.(*%s)\n", name)
	fmt.Fprintf(fg.body, "\tif !ok {\n")
	fmt.Fprintf(fg.body, "\t\tthat2, ok := that.(%s)\n", name)
	fmt.Fprintf(fg.body, "\t\tif ok {\n\t\t\tthat1 = &that2\n\t\t} else {\n\t\t\treturn 1\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\tif that1 == nil {\n\t\tif this == nil {\n\t\t\treturn 0\n\t\t}\n\t\treturn 1\n\t} else if this == nil {\n\t\treturn -1\n\t}\n")

	seenOneofs := map[string]bool{}
	for _, fd := range sortedByTag(md) {
		if isRealOneof(fd) {
			oo := fd.ContainingOneof()
			ooName := string(oo.Name())
			if !seenOneofs[ooName] {
				seenOneofs[ooName] = true
				fg.emitOneofCompare(md, oo)
			}
			continue
		}

		goName := snakeToPascal(string(fd.Name()))
		ft := fg.fieldType(fd)
		ft.EmitCompare(fg, "\t", "this."+goName, "that1."+goName)
	}

	fmt.Fprintf(fg.body, "\treturn 0\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

// sortedByTag returns the message's fields in ascending wire-tag order.
// Declaration order would be a fine alternative for almost every real
// .proto (fields are usually declared in tag order), but sorting by tag
// gives Compare's behavior a stable contract independent of source-file
// reordering — a renumber-without-reorder edit changes the ordering;
// a reorder-without-renumber edit does not.
func sortedByTag(md protoreflect.MessageDescriptor) []protoreflect.FieldDescriptor {
	n := md.Fields().Len()
	out := make([]protoreflect.FieldDescriptor, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, md.Fields().Get(i))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number() < out[j].Number() })
	return out
}

// emitOneofCompare compares a oneof field by variant index first, then
// payload when the variants match. "Variant index" is the variant's
// position in the oneof's declaration order; the unset case is encoded as
// -1 and sorts before every set case.
//
// The double type-switch shape — one to extract this's index, one to
// extract that1's index — is verbose but produces a clean total order
// without resorting to a synthetic interface method. The Go compiler
// reduces the two switches to direct type-tag comparisons, so cost stays
// flat. The payload type-switch at the end runs only when both sides hit
// the same variant; the `_ = v2` is there because some variant payload
// EmitCompare implementations don't reference both sides (defensive — all
// current ones do, but a future scalar-less variant would otherwise
// produce "declared and not used").
func (fg *FileGenerator) emitOneofCompare(md protoreflect.MessageDescriptor, oo protoreflect.OneofDescriptor) {
	goName := snakeToPascal(string(oo.Name()))
	lhs := "this." + goName
	rhs := "that1." + goName

	fmt.Fprintf(fg.body, "\t{\n")
	fmt.Fprintf(fg.body, "\t\tthisIdx := -1\n")
	fmt.Fprintf(fg.body, "\t\tswitch %s.(type) {\n", lhs)
	for i := 0; i < oo.Fields().Len(); i++ {
		fd := oo.Fields().Get(i)
		variantType := oneofVariantName(md, fd)
		fmt.Fprintf(fg.body, "\t\tcase *%s:\n\t\t\tthisIdx = %d\n", variantType, i)
	}
	fmt.Fprintf(fg.body, "\t\t}\n")

	fmt.Fprintf(fg.body, "\t\tthatIdx := -1\n")
	fmt.Fprintf(fg.body, "\t\tswitch %s.(type) {\n", rhs)
	for i := 0; i < oo.Fields().Len(); i++ {
		fd := oo.Fields().Get(i)
		variantType := oneofVariantName(md, fd)
		fmt.Fprintf(fg.body, "\t\tcase *%s:\n\t\t\tthatIdx = %d\n", variantType, i)
	}
	fmt.Fprintf(fg.body, "\t\t}\n")

	fmt.Fprintf(fg.body, "\t\tif thisIdx != thatIdx {\n")
	fmt.Fprintf(fg.body, "\t\t\tif thisIdx < thatIdx {\n\t\t\t\treturn -1\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\treturn 1\n\t\t}\n")

	fmt.Fprintf(fg.body, "\t\tif thisIdx != -1 {\n")
	fmt.Fprintf(fg.body, "\t\t\tswitch v := %s.(type) {\n", lhs)
	for i := 0; i < oo.Fields().Len(); i++ {
		fd := oo.Fields().Get(i)
		variantType := oneofVariantName(md, fd)
		fieldName := snakeToPascal(string(fd.Name()))

		fmt.Fprintf(fg.body, "\t\t\tcase *%s:\n", variantType)
		fmt.Fprintf(fg.body, "\t\t\t\tv2 := %s.(*%s)\n", rhs, variantType)
		fmt.Fprintf(fg.body, "\t\t\t\t_ = v2\n")
		types.Get(fd.Kind()).EmitCompare(fg, "\t\t\t\t", "v."+fieldName, "v2."+fieldName)
	}
	fmt.Fprintf(fg.body, "\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t}\n")
	fmt.Fprintf(fg.body, "\t}\n")
}
