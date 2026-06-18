package generator

import (
	"fmt"

	"github.com/grafana/wiresmith/compiler/types"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// fieldsForPreScan returns fields whose element count can be determined by
// counting wire-format field-number occurrences. This includes repeated
// message/string/bytes fields (packed scalars are excluded because one wire
// occurrence contains many elements) and map fields (each wire occurrence
// is one map entry).
func fieldsForPreScan(md protoreflect.MessageDescriptor) []protoreflect.FieldDescriptor {
	var fields []protoreflect.FieldDescriptor
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if fd.IsMap() {
			fields = append(fields, fd)
			continue
		}
		if !fd.IsList() {
			continue
		}
		switch fd.Kind() {
		case protoreflect.MessageKind, protoreflect.StringKind, protoreflect.BytesKind:
			fields = append(fields, fd)
		}
	}
	return fields
}

// preScanMinBytes is the minimum message size for the pre-scan to run.
const preScanMinBytes = 256

// emitPreScan emits a lightweight tag-scanning loop that counts occurrences of
// repeated message/string/bytes fields, then pre-allocates their slices with
// exact capacity. Uses inline varint decoding for performance. It returns true
// when a pre-scan block was emitted (the message has at least one pre-scannable
// field) and false otherwise; callers use this to decide whether a
// pre-scan-skipping entry point (UnmarshalNoPrescan) is worth emitting.
//
// The emitted guard is `if l >= preScanMinBytes && depth >= 0`. The `depth >= 0`
// term is what UnmarshalNoPrescan exploits: the top-level entry points pass
// depth 0 (Unmarshal) or -1 (UnmarshalNoPrescan), so a negative starting depth
// disables the pre-scan for the top-level message only. Nested unmarshals always
// advance depth by 1 (same-package `unmarshal(b, depth+1)`, cross-package
// `UnmarshalWithDepth(b, depth+1)` which never re-clamps a non-negative value),
// so -1 becomes 0 at the first hop and every nested pre-scan still runs. See
// emitUnmarshal for the entry-point wiring.
func (fg *FileGenerator) emitPreScan(md protoreflect.MessageDescriptor) bool {
	fields := fieldsForPreScan(md)
	if len(fields) == 0 {
		return false
	}

	// `depth >= 0` is the pre-scan opt-out switch (see doc comment): -1 means
	// "top-level UnmarshalNopScan, skip this scan"; the +1 every nested call
	// applies lifts -1 to 0 so nested scans are unaffected.
	fmt.Fprintf(fg.body, "\tif l >= %d && depth >= 0 {\n", preScanMinBytes)
	fmt.Fprintf(fg.body, "\t\tvar preIdx int\n")
	for _, fd := range fields {
		fmt.Fprintf(fg.body, "\t\tvar field%dcount int\n", fd.Number())
	}
	fmt.Fprintf(fg.body, "\t\tfor preIdx < l {\n")

	// Inline tag decode
	fmt.Fprintf(fg.body, "\t\t\tvar preWire uint64\n")
	fmt.Fprintf(fg.body, "\t\t\tfor shift := uint(0); ; shift += 7 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif preIdx >= l {\n\t\t\t\t\tbreak\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tb := dAtA[preIdx]\n")
	fmt.Fprintf(fg.body, "\t\t\t\tpreIdx++\n")
	fmt.Fprintf(fg.body, "\t\t\t\tpreWire |= uint64(b&0x7F) << shift\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif b < 0x80 {\n\t\t\t\t\tbreak\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tpreNum := int32(preWire >> 3)\n")
	fmt.Fprintf(fg.body, "\t\t\tpreTyp := int(preWire & 0x7)\n")

	fmt.Fprintf(fg.body, "\t\t\tswitch preNum {\n")
	for _, fd := range fields {
		fmt.Fprintf(fg.body, "\t\t\tcase %d:\n", fd.Number())
		fmt.Fprintf(fg.body, "\t\t\t\tfield%dcount++\n", fd.Number())
	}
	fmt.Fprintf(fg.body, "\t\t\t}\n")

	// Skip field value
	fmt.Fprintf(fg.body, "\t\t\tswitch preTyp {\n")
	fmt.Fprintf(fg.body, "\t\t\tcase 0:\n") // varint
	fmt.Fprintf(fg.body, "\t\t\t\tfor preIdx < l {\n\t\t\t\t\tpreIdx++\n\t\t\t\t\tif dAtA[preIdx-1] < 0x80 {\n\t\t\t\t\t\tbreak\n\t\t\t\t\t}\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tcase 1:\n") // fixed64
	fmt.Fprintf(fg.body, "\t\t\t\tpreIdx += 8\n")
	fmt.Fprintf(fg.body, "\t\t\tcase 2:\n") // bytes
	fmt.Fprintf(fg.body, "\t\t\t\tvar preLen uint64\n")
	fmt.Fprintf(fg.body, "\t\t\t\tfor shift := uint(0); ; shift += 7 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tif preIdx >= l {\n\t\t\t\t\t\tbreak\n\t\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tb := dAtA[preIdx]\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tpreIdx++\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tpreLen |= uint64(b&0x7F) << shift\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tif b < 0x80 {\n\t\t\t\t\t\tbreak\n\t\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tpreIdx += int(preLen)\n")
	fmt.Fprintf(fg.body, "\t\t\tcase 5:\n") // fixed32
	fmt.Fprintf(fg.body, "\t\t\t\tpreIdx += 4\n")
	// Wire types 3/4 (proto2 groups) and 6/7 (reserved) are not produced
	// by compliant proto3 encoders for this schema, and crucially the
	// pre-scan does not know how to skip them: wire type 3 requires
	// matching a corresponding end-group tag and 4/6/7 have no defined
	// payload framing. The pre-scan is only an allocation hint (the main
	// loop is the source of truth), so on an unknown wire type we abort
	// by forcing preIdx out of bounds — the post-switch bounds check
	// below then breaks the outer loop in the same iteration. This
	// prevents SEC-2-style amplification where a single unknown-wire-type
	// tag would otherwise leave preIdx un-advanced and let payload bytes
	// be re-interpreted as more tags.
	fmt.Fprintf(fg.body, "\t\t\tdefault:\n")
	fmt.Fprintf(fg.body, "\t\t\t\tpreIdx = -1\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tif preIdx < 0 || preIdx > l {\n\t\t\t\tbreak\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t}\n")

	// Cap pre-allocated capacity by an attacker-resistant bound. Every
	// pre-scan-tracked element is length-delimited, so each occurrence
	// consumes at least 2 bytes on the wire: tag varint (≥1 byte) plus
	// length varint (≥1 byte, encoding 0 in the smallest case). No
	// compliant payload of length l can therefore produce more than l/2
	// occurrences. Without this cap, a payload of N zero-length entries of
	// a struct-typed repeated field allocates capacity N * sizeof(struct),
	// so a tiny payload can force a multi-MB allocation (SEC-1). `l/2` is
	// a universal safe upper bound that costs nothing on legitimate
	// payloads (where count is already ≤ l/2) and bounds the worst case
	// at len(payload)/2 elements.
	fmt.Fprintf(fg.body, "\t\tpreCapMax := l / 2\n")
	for _, fd := range fields {
		goName := fg.goFieldName(fd)
		// goFieldType respects (wiresmith.options.pointer) so a repeated
		// pointer-message field pre-allocates as `[]*Msg` rather than `[]Msg`.
		goType := fg.goFieldType(fd)
		// Inline `if c > preCapMax { c = preCapMax }` is observably tighter
		// than `min(c, preCapMax)` (the latter shows +1.5pp regression on
		// UnmarshalTraces vs the inline branch); the Go compiler does not
		// always reduce `min` to a cmov on this hot path.
		fmt.Fprintf(fg.body, "\t\tif c := field%dcount; c > 0 {\n", fd.Number())
		fmt.Fprintf(fg.body, "\t\t\tif c > preCapMax {\n\t\t\t\tc = preCapMax\n\t\t\t}\n")
		if fd.IsMap() {
			// Merge semantics: an existing map keeps its entries (the main
			// decode loop overwrites same-key entries, last-write-wins);
			// only a nil map gets the count-sized allocation.
			fmt.Fprintf(fg.body, "\t\t\tif m.%s == nil {\n", goName)
			fmt.Fprintf(fg.body, "\t\t\t\tm.%s = make(%s, c)\n", goName, goType)
			fmt.Fprintf(fg.body, "\t\t\t}\n")
		} else {
			// Option A: reserve the count-sized capacity ONLY for a fresh
			// (empty) slice. Unmarshal appends to repeated fields (gogo merge
			// parity) regardless of whether the pre-scan ran, so when the
			// slice is empty this exact-fit make() is byte-identical to a
			// fresh decode and still preserves the pooled-message pattern
			// (Mimir resets a sync.Pool message to len 0 and unmarshals into
			// it; len==0 with cap>=c reuses the backing array, no make()).
			//
			// When the slice is already populated (merge-by-unmarshal: repeated
			// Unmarshal into a NON-reset message), we deliberately do NOT
			// reserve. An exact-fit grow to len+c reallocates and copies the
			// entire backing array on EVERY call (cap < len+c always holds when
			// len grows by ~c each call), which is O(n²) total. Skipping the
			// reserve lets the main decode loop's append grow the slice with
			// amortized doubling — O(n) total, matching gogo, which has no
			// pre-scan. The preCapMax clamp above keeps the SEC-1 cap bound.
			fmt.Fprintf(fg.body, "\t\t\tif len(m.%s) == 0 && cap(m.%s) < c {\n", goName, goName)
			fmt.Fprintf(fg.body, "\t\t\t\tm.%s = make(%s, 0, c)\n", goName, goType)
			fmt.Fprintf(fg.body, "\t\t\t}\n")
		}
		fmt.Fprintf(fg.body, "\t\t}\n")
	}

	fmt.Fprintf(fg.body, "\t}\n")
	return true
}

func (fg *FileGenerator) emitUnmarshal(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	fg.imports.addImport("fmt", "")
	fg.imports.addImport("io", "")
	fg.imports.addImport(protohelpersImport, "")

	// Public wrapper that starts depth tracking at zero. UnmarshalWithDepth
	// is the cross-package entry point: callers across the package boundary
	// invoke it with the parent's depth+1 so the recursion-depth counter
	// remains monotonic — otherwise a graph bouncing between packages
	// would silently reset depth at each hop and recurse up to
	// maxUnmarshalDepth*pkgCount levels (SEC-5).
	//
	// Negative starting depth is clamped to 0: the caller-supplied depth
	// feeds directly into the `depth > maxUnmarshalDepth` guard, so a
	// negative value would silently widen the budget. Clamping keeps the
	// guard's monotonicity property even if a buggy caller passes -N.
	fmt.Fprintf(fg.body, "func (m *%s) Unmarshal(b []byte) error {\n", name)
	fmt.Fprintf(fg.body, "\treturn m.unmarshal(b, 0)\n")
	fmt.Fprintf(fg.body, "}\n\n")
	fmt.Fprintf(fg.body, "func (m *%s) UnmarshalWithDepth(b []byte, depth int) error {\n", name)
	fmt.Fprintf(fg.body, "\tif depth < 0 {\n")
	fmt.Fprintf(fg.body, "\t\tdepth = 0\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\treturn m.unmarshal(b, depth)\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// Pre-render the body so we know whether a pre-scan block was emitted
	// before we write the entry points. UnmarshalNoPrescan is only emitted for
	// messages whose unmarshal actually contains a pre-scan (a message without
	// a pre-scan has nothing to skip, so the extra method would be dead weight
	// and bloat every generated file). The pre-scan guard itself carries the
	// `&& depth >= 0` term that lets a -1 starting depth disable the top-level
	// scan; see emitPreScan.
	fmt.Fprintf(fg.body, "func (m *%s) unmarshal(dAtA []byte, depth int) error {\n", name)
	fmt.Fprintf(fg.body, "\tif depth > protohelpers.MaxUnmarshalDepth {\n")
	fmt.Fprintf(fg.body, "\t\treturn fmt.Errorf(\"exceeded max recursion depth\")\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\tl := len(dAtA)\n")
	fmt.Fprintf(fg.body, "\tiNdEx := 0\n")

	hasPreScan := fg.emitPreScan(md)

	// Main parse loop with inline tag decoding.
	fmt.Fprintf(fg.body, "\tfor iNdEx < l {\n")

	types.EmitConsumeTagAt(fg, "\t\t", "wire")
	fmt.Fprintf(fg.body, "\t\tfieldNum := int32(wire >> 3)\n")
	fmt.Fprintf(fg.body, "\t\twireType := int(wire & 0x7)\n")

	fmt.Fprintf(fg.body, "\t\tswitch fieldNum {\n")

	pm := fg.presenceMap(md)
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		fg.emitFieldUnmarshal(md, fd)
		if bitIndex, ok := pm[fd.Number()]; ok {
			fmt.Fprintf(fg.body, "\t\t\t%s\n", presenceSet(bitIndex))
		}
	}

	fmt.Fprintf(fg.body, "\t\tdefault:\n")
	fmt.Fprintf(fg.body, "\t\t\tn, err := protohelpers.SkipValue(dAtA[iNdEx:], wireType, fieldNum)\n")
	fmt.Fprintf(fg.body, "\t\t\tif err != nil {\n\t\t\t\treturn err\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tiNdEx += n\n")
	fmt.Fprintf(fg.body, "\t\t}\n") // end switch
	fmt.Fprintf(fg.body, "\t}\n")   // end for
	fmt.Fprintf(fg.body, "\tif iNdEx > l {\n\t\treturn io.ErrUnexpectedEOF\n\t}\n")
	fmt.Fprintf(fg.body, "\treturn nil\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// UnmarshalNoPrescan decodes identically to Unmarshal but skips the
	// TOP-LEVEL repeated-field counting pre-scan. It is emitted only for
	// messages that have a pre-scan (hasPreScan); a message with no pre-scan
	// would produce a method byte-identical to Unmarshal, so we omit it.
	//
	// Use it when unmarshalling into a REUSED or pooled message (one whose
	// repeated slices already have len>0, or were reset to [:0] with capacity
	// retained): the pre-scan's exact-fit prealloc never fires in that case
	// (its guard is `len==0 && cap<count`), so the scan is pure overhead.
	// For a FRESH message the pre-scan's prealloc pays off — keep using
	// Unmarshal there.
	//
	// The mechanism is a -1 starting depth: the pre-scan guard is
	// `l >= N && depth >= 0`, so depth -1 disables the top-level scan, while
	// every nested unmarshal advances depth by 1 (lifting -1 to 0), preserving
	// nested pre-scans. The unmarshal() depth guard (`depth > MaxUnmarshalDepth`)
	// tolerates the negative value.
	if hasPreScan {
		fmt.Fprintf(fg.body, "func (m *%s) UnmarshalNoPrescan(dAtA []byte) error {\n", name)
		fmt.Fprintf(fg.body, "\treturn m.unmarshal(dAtA, -1)\n")
		fmt.Fprintf(fg.body, "}\n\n")
	}
}

func (fg *FileGenerator) emitFieldUnmarshal(md protoreflect.MessageDescriptor, fd protoreflect.FieldDescriptor) {
	goName := fg.goFieldName(fd)
	kind := fd.Kind()
	access := "m." + goName

	fmt.Fprintf(fg.body, "\t\tcase %d: // %s\n", fd.Number(), fd.Name())

	if fd.IsMap() {
		fg.emitWireTypeCheck(protoreflect.MessageKind)
		mf := &types.MapField{
			Key:       types.Get(fd.MapKey().Kind()),
			Val:       types.Get(fd.MapValue().Kind()),
			MapType:   fg.imports.goType(fd),
			KeyGoType: fg.imports.goSingularType(fd.MapKey()),
			ValGoType: fg.imports.goSingularType(fd.MapValue()),
			KeyCtx:    fg.fieldContext(fd.MapKey()),
			ValCtx:    fg.fieldContext(fd.MapValue()),
		}
		types.AddTypeImports(fg, mf)
		mf.EmitUnmarshal(fg, access, types.FieldContext{})
		return
	}

	t := types.Get(kind)

	// Packed repeated fields handle wire type dispatch internally.
	if fd.IsList() && t.IsPackable() {
		ctx := fg.fieldContext(fd)
		ctx.SliceType = fg.imports.goType(fd)
		rf := &types.RepeatedField{Inner: t, IsPacked: fd.IsPacked()}
		types.AddTypeImports(fg, rf)
		rf.EmitUnmarshal(fg, access, ctx)
		return
	}

	fg.emitWireTypeCheck(kind)

	if fd.IsList() {
		ctx := fg.fieldContext(fd)
		// fieldType dispatches between RepeatedField and RepeatedPointer based
		// on `(wiresmith.options.pointer)`; the FieldType interface keeps the
		// call site uniform.
		ft := fg.fieldType(fd)
		types.AddTypeImports(fg, ft)
		ft.EmitUnmarshal(fg, access, ctx)
		return
	}

	ctx := fg.fieldContext(fd)

	if isRealOneof(fd) {
		oo := fd.ContainingOneof()
		of := &types.OneofField{
			Inner:       t,
			OneofName:   snakeToPascal(string(oo.Name())),
			VariantName: oneofVariantName(md, fd),
			FieldName:   fg.goFieldName(fd),
		}
		of.EmitUnmarshal(fg, "m."+of.OneofName, ctx)
		return
	}

	if fd.HasOptionalKeyword() {
		// Optional message has the same `*Msg` shape as the pointer-option
		// case, so reuse PointerField. Other optional kinds need the *T
		// allocation in OptionalField.
		if fd.Kind() == protoreflect.MessageKind {
			pf := &types.PointerField{Inner: t}
			types.AddTypeImports(fg, pf)
			pf.EmitUnmarshal(fg, access, ctx)
			return
		}
		of := &types.OptionalField{Inner: t}
		types.AddTypeImports(fg, of)
		of.EmitUnmarshal(fg, access, ctx)
		return
	}

	// Singular `(wiresmith.options.pointer) = true` on a message field is
	// dispatched through fg.fieldType — the same single entry point used by
	// emit_marshal, emit_size, and the repeated branch above. Routing it here
	// instead of inlining a second PointerField construction keeps the option
	// visible in exactly one place.
	if fg.hasPointerOption(fd) && fd.Kind() == protoreflect.MessageKind {
		pf := fg.fieldType(fd)
		types.AddTypeImports(fg, pf)
		pf.EmitUnmarshal(fg, access, ctx)
		return
	}

	// Singular bytes/string with `(wiresmith.options.customtype)` routes
	// through the same FieldType the size/marshal/equal phases use. The
	// other emit phases already go through fg.fieldType; the singular
	// scalar path here historically bypassed it to skip a polymorphic call,
	// so customtype needs a dedicated branch.
	if ft, ok := fg.customtypeFieldType(fd); ok {
		ft.EmitUnmarshal(fg, access, ctx)
		return
	}

	// Singular Timestamp with `(wiresmith.options.stdtime) = true` swaps the
	// message-kind path for an inline Timestamp envelope decode into a
	// `time.Time`. Same single-FieldType dispatch as customtype above.
	if ft, ok := fg.stdtimeFieldType(fd); ok {
		types.AddTypeImports(fg, ft)
		ft.EmitUnmarshal(fg, access, ctx)
		return
	}

	// Singular Duration with `(wiresmith.options.stdduration) = true` swaps
	// the message-kind path for an inline Duration envelope decode into a
	// `time.Duration`. Same single-FieldType dispatch as stdtime above.
	if ft, ok := fg.stdDurationFieldType(fd); ok {
		types.AddTypeImports(fg, ft)
		ft.EmitUnmarshal(fg, access, ctx)
		return
	}

	// Singular scalar with `(wiresmith.options.casttype)` wraps the
	// underlying scalar's emit with the user-supplied Go alias. CastType
	// uses Inner.EmitConsume + a re-cast assignment, so the dispatch shape
	// matches customtype/stdtime — the underlying t.EmitUnmarshal call at
	// the bottom of this function would emit the un-aliased cast.
	if ft, ok := fg.casttypeFieldType(fd); ok {
		types.AddTypeImports(fg, ft)
		ft.EmitUnmarshal(fg, access, ctx)
		return
	}

	types.AddTypeImports(fg, t)
	t.EmitUnmarshal(fg, access, ctx)
}

// emitWireTypeCheck emits a check that the wire type matches the expected type
// for a given proto kind, skipping the field if it doesn't match.
func (fg *FileGenerator) emitWireTypeCheck(kind protoreflect.Kind) {
	wtInt := types.WireTypeInt(kind)
	fmt.Fprintf(fg.body, "\t\t\tif wireType != %d {\n", wtInt)
	fmt.Fprintf(fg.body, "\t\t\t\tn, err := protohelpers.SkipValue(dAtA[iNdEx:], wireType, fieldNum)\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif err != nil {\n\t\t\t\t\treturn err\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tiNdEx += n\n")
	fmt.Fprintf(fg.body, "\t\t\t\tcontinue\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
}
