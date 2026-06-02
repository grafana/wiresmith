package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"

	"wiresmith/compiler/types"
)

// Equal() methods are emitted into the companion `_equal.pb.go` file, not
// into the main `.pb.go`. Same icache/iTLB rationale as the reflect/registration
// split documented in emit_registration.go: Equal is cold, the marshal hot
// path is hot, and co-locating them in one compilation unit shifts the hot
// code onto different cache lines for no benefit. Wiresmith-6ci measured on
// Apple M4 Pro, count=20, benchtime=1s, serial (baseline = Equal still in
// main file):
//
//	UnmarshalSingleSpan_Ours -6.23% (p=0.000)
//	MarshalProfiles_Ours     -5.45% (p=0.000)
//	UnmarshalLogs_Ours       -5.13% (p=0.000)
//	MarshalSingleSpan_Ours   -4.40% (p=0.000)
//	UnmarshalHistogram_Ours  -4.38% (p=0.000)
//	MarshalMap_Ours          -3.76% (p=0.000)
//	UnmarshalTraces_Ours     -3.73% (p=0.000)
//	(geomean across hot Marshal/Unmarshal benches: -1.75%, alloc unchanged)
//
// Smaller regressions in the noise: MarshalSummary +0.92%, UnmarshalProfiles
// +0.73%. Size_Ours benches (which Equal can't touch) also moved by -1.5% to
// -3.3% — that's the noise floor from generic code-layout shift after
// removing Equal from the main .pb.go; the largest hot-path gains exceed it
// comfortably. Keep this split unless someone re-measures.
func (fg *FileGenerator) emitAllEqualMethods(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, fg.emitEqual)
}

func (fg *FileGenerator) emitEqual(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	out := fg.equalBody
	em := equalEmitter{fg: fg}

	fmt.Fprintf(out, "func (this *%s) Equal(that interface{}) bool {\n", name)
	fmt.Fprintf(out, "\tif that == nil {\n\t\treturn this == nil\n\t}\n\n")

	fmt.Fprintf(out, "\tthat1, ok := that.(*%s)\n", name)
	fmt.Fprintf(out, "\tif !ok {\n")
	fmt.Fprintf(out, "\t\tthat2, ok := that.(%s)\n", name)
	fmt.Fprintf(out, "\t\tif ok {\n\t\t\tthat1 = &that2\n\t\t} else {\n\t\t\treturn false\n\t\t}\n")
	fmt.Fprintf(out, "\t}\n")
	fmt.Fprintf(out, "\tif that1 == nil {\n\t\treturn this == nil\n\t} else if this == nil {\n\t\treturn false\n\t}\n")

	seenOneofs := map[string]bool{}
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)

		if isRealOneof(fd) {
			oo := fd.ContainingOneof()
			ooName := string(oo.Name())
			if !seenOneofs[ooName] {
				seenOneofs[ooName] = true
				fg.emitOneofEqual(md, oo)
			}
			continue
		}

		goName := snakeToPascal(string(fd.Name()))
		ft := fg.fieldType(fd)
		ft.EmitEqual(em, "\t", "this."+goName, "that1."+goName)
	}

	fmt.Fprintf(out, "\treturn true\n")
	fmt.Fprintf(out, "}\n\n")
}

func (fg *FileGenerator) emitOneofEqual(md protoreflect.MessageDescriptor, oo protoreflect.OneofDescriptor) {
	goName := snakeToPascal(string(oo.Name()))
	out := fg.equalBody
	em := equalEmitter{fg: fg}

	fmt.Fprintf(out, "\tif (this.%s == nil) != (that1.%s == nil) {\n", goName, goName)
	fmt.Fprintf(out, "\t\treturn false\n\t}\n")
	fmt.Fprintf(out, "\tif this.%s != nil {\n", goName)
	fmt.Fprintf(out, "\t\tswitch v := this.%s.(type) {\n", goName)

	for i := 0; i < oo.Fields().Len(); i++ {
		fd := oo.Fields().Get(i)
		variantType := oneofVariantName(md, fd)
		fieldName := snakeToPascal(string(fd.Name()))

		fmt.Fprintf(out, "\t\tcase *%s:\n", variantType)
		fmt.Fprintf(out, "\t\t\tv2, ok := that1.%s.(*%s)\n", goName, variantType)
		fmt.Fprintf(out, "\t\t\tif !ok {\n\t\t\t\treturn false\n\t\t\t}\n")

		// Per-variant comparison: scalars/string/enum → `!=`, bytes →
		// bytes.Equal (and lazy import), message → `.Equal()`.
		types.Get(fd.Kind()).EmitEqual(em, "\t\t\t", "v."+fieldName, "v2."+fieldName)
	}

	fmt.Fprintf(out, "\t\tdefault:\n\t\t\treturn false\n")
	fmt.Fprintf(out, "\t\t}\n")
	fmt.Fprintf(out, "\t}\n")
}
