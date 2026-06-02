package generator

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/reflect/protoreflect"

	"wiresmith/compiler/types"
)

// emitAllCompareMethods emits `Compare(that interface{}) int` on every
// message in fd. Compare returns -1/0/+1 like bytes.Compare with the
// gogoproto-compatible nil/wrong-type preamble.
//
// All writes are routed through a compareEmitter so the methods land in
// `<name>_compare.pb.go` rather than the main file. The reason is icache
// pressure: emitting Compare next to the hot Marshal/Unmarshal in a single
// .pb.go was measured to add ~9% geomean to OTel hot benchmarks even
// though Compare itself is never called there. The split is the same
// trick the reflect companion uses; see generator.go's emitCompareFileBanner
// for the in-artifact rationale.
func (fg *FileGenerator) emitAllCompareMethods(fd protoreflect.FileDescriptor) {
	ce := &compareEmitter{fg: fg}
	forEachMessage(fd, func(md protoreflect.MessageDescriptor) {
		fg.emitCompare(ce, md)
	})
}

// emitCompare emits the Compare method for one message into ce. The
// nil-handling preamble matches gogo's pattern verbatim so callers that
// depend on `nil.Compare(non-nil) == -1` / `non-nil.Compare(nil) == 1` /
// `nil.Compare(nil) == 0` / `m.Compare("wrong type") == 1` keep their
// existing expectations. Fields walk in ascending wire tag (not
// declaration order) so the per-field ordering is stable against proto
// file reorderings that don't change the wire format.
func (fg *FileGenerator) emitCompare(ce *compareEmitter, md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	out := fg.compareBody

	fmt.Fprintf(out, "func (this *%s) Compare(that interface{}) int {\n", name)
	fmt.Fprintf(out, "\tif that == nil {\n\t\tif this == nil {\n\t\t\treturn 0\n\t\t}\n\t\treturn 1\n\t}\n\n")

	fmt.Fprintf(out, "\tthat1, ok := that.(*%s)\n", name)
	fmt.Fprintf(out, "\tif !ok {\n")
	fmt.Fprintf(out, "\t\tthat2, ok := that.(%s)\n", name)
	fmt.Fprintf(out, "\t\tif ok {\n\t\t\tthat1 = &that2\n\t\t} else {\n\t\t\treturn 1\n\t\t}\n")
	fmt.Fprintf(out, "\t}\n")
	fmt.Fprintf(out, "\tif that1 == nil {\n\t\tif this == nil {\n\t\t\treturn 0\n\t\t}\n\t\treturn 1\n\t} else if this == nil {\n\t\treturn -1\n\t}\n")

	seenOneofs := map[string]bool{}
	for _, fd := range sortedByTag(md) {
		if isRealOneof(fd) {
			oo := fd.ContainingOneof()
			ooName := string(oo.Name())
			if !seenOneofs[ooName] {
				seenOneofs[ooName] = true
				fg.emitOneofCompare(ce, md, oo)
			}
			continue
		}

		goName := fg.goFieldName(fd)
		ft := fg.fieldType(fd)
		ft.EmitCompare(ce, "\t", "this."+goName, "that1."+goName)
	}

	fmt.Fprintf(out, "\treturn 0\n")
	fmt.Fprintf(out, "}\n\n")
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
func (fg *FileGenerator) emitOneofCompare(ce *compareEmitter, md protoreflect.MessageDescriptor, oo protoreflect.OneofDescriptor) {
	goName := snakeToPascal(string(oo.Name()))
	lhs := "this." + goName
	rhs := "that1." + goName
	out := fg.compareBody

	fmt.Fprintf(out, "\t{\n")
	fmt.Fprintf(out, "\t\tthisIdx := -1\n")
	fmt.Fprintf(out, "\t\tswitch %s.(type) {\n", lhs)
	for i := 0; i < oo.Fields().Len(); i++ {
		fd := oo.Fields().Get(i)
		variantType := oneofVariantName(md, fd)
		fmt.Fprintf(out, "\t\tcase *%s:\n\t\t\tthisIdx = %d\n", variantType, i)
	}
	fmt.Fprintf(out, "\t\t}\n")

	fmt.Fprintf(out, "\t\tthatIdx := -1\n")
	fmt.Fprintf(out, "\t\tswitch %s.(type) {\n", rhs)
	for i := 0; i < oo.Fields().Len(); i++ {
		fd := oo.Fields().Get(i)
		variantType := oneofVariantName(md, fd)
		fmt.Fprintf(out, "\t\tcase *%s:\n\t\t\tthatIdx = %d\n", variantType, i)
	}
	fmt.Fprintf(out, "\t\t}\n")

	fmt.Fprintf(out, "\t\tif thisIdx != thatIdx {\n")
	fmt.Fprintf(out, "\t\t\tif thisIdx < thatIdx {\n\t\t\t\treturn -1\n\t\t\t}\n")
	fmt.Fprintf(out, "\t\t\treturn 1\n\t\t}\n")

	fmt.Fprintf(out, "\t\tif thisIdx != -1 {\n")
	fmt.Fprintf(out, "\t\t\tswitch v := %s.(type) {\n", lhs)
	for i := 0; i < oo.Fields().Len(); i++ {
		fd := oo.Fields().Get(i)
		variantType := oneofVariantName(md, fd)
		fieldName := fg.goFieldName(fd)

		fmt.Fprintf(out, "\t\t\tcase *%s:\n", variantType)
		fmt.Fprintf(out, "\t\t\t\tv2 := %s.(*%s)\n", rhs, variantType)
		fmt.Fprintf(out, "\t\t\t\t_ = v2\n")
		inner := types.Get(fd.Kind())
		inner.EmitCompare(ce, "\t\t\t\t", "v."+fieldName, "v2."+fieldName)
	}
	fmt.Fprintf(out, "\t\t\t}\n")
	fmt.Fprintf(out, "\t\t}\n")
	fmt.Fprintf(out, "\t}\n")
}
