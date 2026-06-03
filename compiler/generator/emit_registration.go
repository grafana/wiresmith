package generator

import (
	"bytes"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// emitRegistration emits, into the companion `_reflect.pb.go` file:
//
//  1. the embedded `file_*_rawDesc` byte blob (a marshaled FileDescriptorProto
//     with SourceCodeInfo and the wiresmith.options dependency stripped);
//  2. `file_*_goTypes` — `[]any` holding `(*MsgType)(nil)` for every declared
//     message (including `nil` for map-entry messages), `(EnumType)(0)` for
//     every declared enum, and dependency entries for externally-referenced
//     types — in TypeBuilder's flattened ordering;
//  3. `file_*_depIdxs` — `[]int32` encoding the cross-message field-type
//     dependency graph, with 5 trailing boundary markers in reverse sub-list
//     order;
//  4. `file_*_msgTypes` / `file_*_enumTypes` — pre-sized `protoimpl.MessageInfo`
//     / `protoimpl.EnumInfo` slices that Build() populates;
//  5. `file_*_init()` — once-guarded function that assigns OneofWrappers, then
//     calls `protoimpl.TypeBuilder{...}.Build()` which decodes the raw
//     descriptor, sets `GoReflectType`+`Desc` on every MessageInfo/EnumInfo,
//     and registers with `protoregistry.GlobalFiles`+`GlobalTypes`;
//  6. a top-level `init()` calling `file_*_init()` for eager startup
//     registration (matching protoc-gen-go behavior).
//
// =======================================================================
// WHY WE EMIT THE TypeBuilder/DescBuilder SHAPE
// =======================================================================
//
// An earlier iteration of this generator used a different runtime path:
//
//	fdp := new(descriptorpb.FileDescriptorProto)
//	proto.Unmarshal([]byte(rawDesc), fdp)
//	fd, _ := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
//	protoregistry.GlobalFiles.RegisterFile(fd)
//	// + manually set msgTypes[i].Desc, msgTypes[i].GoReflectType, register
//
// That path reaches `descriptorpb.FileDescriptorProto`, which transitively
// pulls in the entire `google.golang.org/protobuf/types/descriptorpb` package
// — 687 new symbols, ~64KB of code — plus `protodesc.NewFile`'s converter
// layer. Linking that into a wiresmith-using binary added ~377KB to __TEXT
// and produced a measured +7–14% regression on all Marshal/Unmarshal
// benchmarks:
//
//	UnmarshalProfiles_Ours +12.6% (p=0.000, n=10)
//	UnmarshalTraces_Ours   +12.2%
//	UnmarshalHistogram_Ours+11.7%
//	UnmarshalLogs_Ours     +11.2%
//	(geomean across 22 _Ours benchmarks: +7%)
//
// Diagnosis: code-layout effect, not a code-level slowdown. Hot-path
// Marshal/Unmarshal opcodes were byte-identical between branches; their
// addresses shifted ~131KB further into the binary because of all the
// newly-linked descriptorpb code, landing on different L1i / iTLB / BTB
// resources during steady-state execution. SIGPROF-based profiling masked
// the regression entirely (pipeline-flushing per tick equalizes the
// microarchitectural state), which is why pprof showed no new hot spots
// even though wall-clock measurement reproduced the slowdown with p=0.000.
//
// The fix: emit the same shape protoc-gen-go uses, via
// `protoimpl.TypeBuilder{File: protoimpl.DescBuilder{...}}.Build()`. That
// path parses the raw FileDescriptorProto bytes directly using `protowire`
// + the hard-coded field-number constants in
// `google.golang.org/protobuf/internal/genid`, and constructs the
// `protoreflect.FileDescriptor` via `internal/filedesc`. It never
// instantiates a `descriptorpb.FileDescriptorProto`, so descriptorpb's 64KB
// of code is unreachable from the link graph.
//
// Verified: `bench-main.test` (links protoc-gen-go output) has 0 descriptorpb
// symbols. The previous descriptorpb-based wiresmith emission caused 687
// descriptorpb symbols to be linked. After this change, the count is 0.
//
// =======================================================================
// FLATTENED ORDERING (CRITICAL)
// =======================================================================
//
// TypeBuilder consumes messages and enums in the order
// `filedesc.Builder.unmarshalCounts` adds them to `fbOut.Messages` /
// `fbOut.Enums` while walking the raw FileDescriptorProto bytes. That order
// is **layered per parent**, not classic depth-first pre-order:
//
//	walkMessages(parent):
//	    yield ALL of parent.Messages (declaration order)
//	    for each child in parent.Messages:
//	        walkMessages(child)
//
// Reference: google.golang.org/protobuf@v1.36.11
// cmd/protoc-gen-go/internal_gengo/init.go:55-84.
//
// `flattenedMessages` and `flattenedEnums` (in generator.go) implement this
// order. Map-entry messages are INCLUDED in `flattenedMessages` — they
// occupy a `file_*_msgTypes` slot (Build allocates one per nested message
// in the raw descriptor regardless of map-entry status) and a `nil` slot in
// `goTypes`. The ProtoReflect emit loop skips them (no Go type → no method)
// but still bumps `nextMsgIndex` so positions stay aligned.
//
// If you ever change the iteration order in `emitAllProtoReflectMethods` or
// `emitAllEnumReflectMethods`, the indices Build() assigns will no longer
// match the indices ProtoReflect()/Descriptor() use to find their slot. Run
// the conformance suite after any such change.
func (fg *FileGenerator) emitRegistration(fd protoreflect.FileDescriptor) {
	prefix := fg.fileVarName
	enums := flattenedEnums(fd)
	msgs := flattenedMessages(fd)

	// Empty proto file (no enums and no messages): emit nothing. The
	// caller already skips writing the _reflect.pb.go file if reflectBody
	// stayed empty after all emitters ran.
	if len(enums) == 0 && len(msgs) == 0 {
		return
	}

	// 1. rawDesc const.
	rawDesc := serializeFileDescriptor(fd)
	fg.reflectBody.WriteString("const ")
	fg.reflectBody.WriteString(prefix)
	fg.reflectBody.WriteString("_rawDesc = \"\" +\n")
	encodeRawDescriptor(fg.reflectBody, rawDesc)
	fg.reflectBody.WriteString("\n\n")

	// 2. File-descriptor cache var.
	fmt.Fprintf(fg.reflectBody, "var %s_fd protoreflect.FileDescriptor\n\n", prefix)

	// 3. Pre-allocated MessageInfo / EnumInfo slices. Build() populates
	//    GoReflectType and Desc on each non-map-entry slot; map-entry slots
	//    stay zero-valued and are skipped by Build's registration loop.
	if len(msgs) > 0 {
		fmt.Fprintf(fg.reflectBody, "var %s_msgTypes = make([]protoimpl.MessageInfo, %d)\n",
			prefix, len(msgs))
	}
	if len(enums) > 0 {
		fmt.Fprintf(fg.reflectBody, "var %s_enumTypes = make([]protoimpl.EnumInfo, %d)\n",
			prefix, len(enums))
	}
	fg.reflectBody.WriteString("\n")

	// 4. goTypes + depIdxs — the codegen-time work that lets Build() do
	//    its job without descriptorpb at runtime.
	fg.emitGoTypesAndDepIdxs(enums, msgs)

	// 5. init function (eager top-level + lazy guarded helper).
	fmt.Fprintf(fg.reflectBody, "func init() { %s_init() }\n\n", prefix)
	fmt.Fprintf(fg.reflectBody, "func %s_init() {\n", prefix)
	fmt.Fprintf(fg.reflectBody, "\tif %s_fd != nil {\n\t\treturn\n\t}\n", prefix)

	// 5a. OneofWrappers — Build() does NOT populate this; we must set it
	//     before the Build() call (matching protoc-gen-go's init shape).
	//     Walk flattenedMessages and skip map-entry positions while still
	//     advancing the index so OneofWrappers land in the right slot.
	msgIdx := 0
	for _, md := range msgs {
		if md.IsMapEntry() {
			msgIdx++
			continue
		}
		var wrappers []string
		for i := 0; i < md.Oneofs().Len(); i++ {
			oo := md.Oneofs().Get(i)
			if oo.IsSynthetic() {
				continue
			}
			for j := 0; j < oo.Fields().Len(); j++ {
				wrappers = append(wrappers,
					fmt.Sprintf("(*%s)(nil)", oneofVariantName(md, oo.Fields().Get(j))))
			}
		}
		if len(wrappers) > 0 {
			fmt.Fprintf(fg.reflectBody, "\t%s_msgTypes[%d].OneofWrappers = []any{%s}\n",
				prefix, msgIdx, strings.Join(wrappers, ", "))
		}
		msgIdx++
	}

	// 5b. The Build() call. `type x struct{}` lets us recover the Go package
	//     path via reflect.TypeOf — same idiom protoc-gen-go uses. unsafe.Slice
	//     hands rawDesc bytes to Build without re-copying.
	fmt.Fprintf(fg.reflectBody, "\ttype x struct{}\n")
	fmt.Fprintf(fg.reflectBody, "\tout := protoimpl.TypeBuilder{\n")
	fmt.Fprintf(fg.reflectBody, "\t\tFile: protoimpl.DescBuilder{\n")
	fmt.Fprintf(fg.reflectBody, "\t\t\tGoPackagePath: reflect.TypeOf(x{}).PkgPath(),\n")
	fmt.Fprintf(fg.reflectBody, "\t\t\tRawDescriptor: unsafe.Slice(unsafe.StringData(%s_rawDesc), len(%s_rawDesc)),\n",
		prefix, prefix)
	fmt.Fprintf(fg.reflectBody, "\t\t\tNumEnums:      %d,\n", len(enums))
	fmt.Fprintf(fg.reflectBody, "\t\t\tNumMessages:   %d,\n", len(msgs))
	fmt.Fprintf(fg.reflectBody, "\t\t\tNumExtensions: 0,\n")
	fmt.Fprintf(fg.reflectBody, "\t\t\tNumServices:   0,\n")
	fmt.Fprintf(fg.reflectBody, "\t\t},\n")
	fmt.Fprintf(fg.reflectBody, "\t\tGoTypes:           %s_goTypes,\n", prefix)
	fmt.Fprintf(fg.reflectBody, "\t\tDependencyIndexes: %s_depIdxs,\n", prefix)
	if len(enums) > 0 {
		fmt.Fprintf(fg.reflectBody, "\t\tEnumInfos:         %s_enumTypes,\n", prefix)
	}
	if len(msgs) > 0 {
		fmt.Fprintf(fg.reflectBody, "\t\tMessageInfos:      %s_msgTypes,\n", prefix)
	}
	fmt.Fprintf(fg.reflectBody, "\t}.Build()\n")
	fmt.Fprintf(fg.reflectBody, "\t%s_fd = out.File\n", prefix)
	// Drop the codegen-only goTypes/depIdxs slices once Build has consumed
	// them — matches protoc-gen-go and shrinks the steady-state retained heap.
	fmt.Fprintf(fg.reflectBody, "\t%s_goTypes = nil\n", prefix)
	fmt.Fprintf(fg.reflectBody, "\t%s_depIdxs = nil\n", prefix)
	fmt.Fprintf(fg.reflectBody, "}\n")

	// Imports for reflect file. DROPPED vs the old descriptorpb path:
	// google.golang.org/protobuf/proto, .../reflect/protodesc,
	// .../reflect/protoregistry, .../types/descriptorpb. ADDED: "unsafe".
	fg.reflectImports.addImport("reflect", "")
	fg.reflectImports.addImport("unsafe", "")
	fg.reflectImports.addImport("google.golang.org/protobuf/reflect/protoreflect", "")
	fg.reflectImports.addImport("google.golang.org/protobuf/runtime/protoimpl", "")
	fg.reflectImports.addImport(protohelpersImport, "")
}

// emitGoTypesAndDepIdxs emits the two codegen-time slices that
// TypeBuilder.Build needs to wire up the file descriptor:
//
//   - `<prefix>_goTypes []any` — one entry per declared enum (as `(E)(0)`),
//     then one per declared message (as `(*M)(nil)` for non-map-entry,
//     `nil` for map entries), then deduplicated dependency types referenced
//     by fields of those messages.
//
//   - `<prefix>_depIdxs []int32` — `listFieldDeps` entries (one per
//     message/enum-kind field in flattened message+field order, holding the
//     `goTypes` index of the referenced type), followed by 5 trailing
//     boundary markers in REVERSE sub-list order matching `filedesc.Builder`'s
//     `listFieldDeps`/`listExtTargets`/`listExtDeps`/`listMethInDeps`/
//     `listMethOutDeps` enum (build.go:61-68). wiresmith doesn't support
//     extensions or services, so the four ext/method sub-lists are empty —
//     their start positions all equal `len(listFieldDeps)`.
//
// External package references are resolved via `fg.reflectImports`. The
// dependency entries reuse the same import alias the rest of the file uses
// (via `goMessageType`/`goEnumType`), and `reflectImports.addProtoImport`
// inside those helpers registers the alias for the reflect file's import
// block.
func (fg *FileGenerator) emitGoTypesAndDepIdxs(
	enums []protoreflect.EnumDescriptor,
	msgs []protoreflect.MessageDescriptor,
) {
	prefix := fg.fileVarName

	// goTypeIndex maps a FullName to its position in the goTypes slice.
	// Used for: (a) dedup'ing dependency entries (a referenced external
	// type appears once in goTypes no matter how many fields reference
	// it), (b) recursive/self-referencing messages (LinkedList.next → its
	// own slot), (c) cross-message references inside the same file.
	goTypeIndex := make(map[protoreflect.FullName]int, len(enums)+len(msgs))
	var goTypeLines []string

	appendGoType := func(name protoreflect.FullName, lit string) int {
		if idx, ok := goTypeIndex[name]; ok {
			return idx
		}
		idx := len(goTypeLines)
		goTypeLines = append(goTypeLines, lit)
		goTypeIndex[name] = idx
		return idx
	}

	// 1. Declared enums (indices 0..len(enums)-1 in goTypes).
	for i, ed := range enums {
		appendGoType(ed.FullName(), fmt.Sprintf("(%s)(0), // %d: %s",
			fg.reflectImports.goEnumType(ed), i, ed.FullName()))
	}

	// 2. Declared messages (indices len(enums)..len(enums)+len(msgs)-1).
	//    Map entries get a `nil` slot — Build() skips them during
	//    GoReflectType assignment but still allocates a MessageInfo entry.
	for i, md := range msgs {
		idx := len(enums) + i
		var lit string
		if md.IsMapEntry() {
			lit = fmt.Sprintf("nil, // %d: %s", idx, md.FullName())
		} else {
			lit = fmt.Sprintf("(*%s)(nil), // %d: %s",
				fg.reflectImports.goMessageType(md), idx, md.FullName())
		}
		appendGoType(md.FullName(), lit)
	}

	// 3. Field dependencies. Walk messages in flattened order, fields in
	//    declaration order. For each Message/Enum-kind field, append its
	//    referenced type's goTypes index to listFieldDeps; if the type is
	//    external (not already in goTypeIndex), append a dependency entry
	//    to goTypes first.
	var depLines []string
	depComment := func(fdesc protoreflect.FieldDescriptor, refName protoreflect.FullName) string {
		// Mirrors protoc-gen-go's annotation style: `<source>:type_name -> <target>`.
		return fmt.Sprintf("%s:type_name -> %s", fdesc.FullName(), refName)
	}
	for _, md := range msgs {
		for i := 0; i < md.Fields().Len(); i++ {
			fdesc := md.Fields().Get(i)
			switch fdesc.Kind() {
			case protoreflect.EnumKind:
				ed := fdesc.Enum()
				if _, declared := goTypeIndex[ed.FullName()]; !declared {
					// External enum dependency.
					appendGoType(ed.FullName(), fmt.Sprintf("(%s)(0), // %d: %s",
						fg.reflectImports.goEnumType(ed),
						len(goTypeLines), ed.FullName()))
				}
				idx := goTypeIndex[ed.FullName()]
				depLines = append(depLines, fmt.Sprintf("%d, // %d: %s",
					idx, len(depLines), depComment(fdesc, ed.FullName())))
			case protoreflect.MessageKind:
				child := fdesc.Message()
				if _, declared := goTypeIndex[child.FullName()]; !declared {
					// External message dependency (cross-package or
					// cross-file reference). Map entries can't appear here —
					// they're always declared in their parent's file.
					appendGoType(child.FullName(), fmt.Sprintf("(*%s)(nil), // %d: %s",
						fg.reflectImports.goMessageType(child),
						len(goTypeLines), child.FullName()))
				}
				idx := goTypeIndex[child.FullName()]
				depLines = append(depLines, fmt.Sprintf("%d, // %d: %s",
					idx, len(depLines), depComment(fdesc, child.FullName())))
			case protoreflect.GroupKind:
				// proto2 group syntax — wiresmith only targets proto3.
				panic(fmt.Sprintf("wiresmith: group fields are unsupported (%s)", fdesc.FullName()))
			}
		}
	}

	// 4. Emit goTypes.
	fmt.Fprintf(fg.reflectBody, "var %s_goTypes = []any{\n", prefix)
	for _, ln := range goTypeLines {
		fmt.Fprintf(fg.reflectBody, "\t%s\n", ln)
	}
	fmt.Fprintf(fg.reflectBody, "}\n\n")

	// 5. Emit depIdxs with the 5 trailing boundary markers in REVERSE
	//    sub-list order. The constants in filedesc/build.go:61-68 are
	//      listFieldDeps   = 0
	//      listExtTargets  = 1
	//      listExtDeps     = 2
	//      listMethInDeps  = 3
	//      listMethOutDeps = 4
	//    and Build reads marker[N-1-listFoo] to find sub-list listFoo's
	//    start. So the trailing slice, written in source order, is:
	//      [methOut_start, methIn_start, extDeps_start, extTargets_start, fieldDeps_start]
	//    which for wiresmith (no extensions, no services) collapses to
	//    `[N, N, N, N, 0]` where N = len(listFieldDeps).
	fmt.Fprintf(fg.reflectBody, "var %s_depIdxs = []int32{\n", prefix)
	for _, ln := range depLines {
		fmt.Fprintf(fg.reflectBody, "\t%s\n", ln)
	}
	n := int32(len(depLines))
	fmt.Fprintf(fg.reflectBody, "\t%d, // [%d:%d] is the sub-list for method output_type\n", n, n, n)
	fmt.Fprintf(fg.reflectBody, "\t%d, // [%d:%d] is the sub-list for method input_type\n", n, n, n)
	fmt.Fprintf(fg.reflectBody, "\t%d, // [%d:%d] is the sub-list for extension type_name\n", n, n, n)
	fmt.Fprintf(fg.reflectBody, "\t%d, // [%d:%d] is the sub-list for extension extendee\n", n, n, n)
	fmt.Fprintf(fg.reflectBody, "\t0, // [0:%d] is the sub-list for field type_name\n", n)
	fmt.Fprintf(fg.reflectBody, "}\n\n")
}

// serializeFileDescriptor converts a protoreflect.FileDescriptor to raw proto
// bytes for embedding in a generated .pb.go file. The wiresmith options proto
// is dropped from the dependency list: it is a codegen-only annotation schema
// with no runtime semantics, so requiring callers to register it would force a
// wiresmith_options.pb.go into every Go binary just to satisfy DescBuilder's
// dependency lookups.
//
// Note: this function runs at CODEGEN time inside the wiresmith binary, where
// pulling in descriptorpb/protodesc is fine. The OUTPUT — the generated
// `_reflect.pb.go` file — never imports descriptorpb.
func serializeFileDescriptor(fd protoreflect.FileDescriptor) []byte {
	fdp := protodesc.ToFileDescriptorProto(fd)
	fdp.SourceCodeInfo = nil
	fdp.Dependency = filterOutDep(fdp.Dependency, embeddedOptionsPath)
	// wiresmith does not emit gRPC service stubs and registers the file with
	// NumServices: 0. Leaving Service entries in the embedded raw descriptor
	// would make protoimpl.checkDecls panic with "mismatching cardinality" at
	// init time. Stripping them lets protos that mix wiresmith-generated
	// messages with services (consumed by external generators like
	// protoc-gen-go-grpc) compile cleanly while wiresmith stays scope-pure.
	//
	// Tradeoff: descriptor-based tooling that walks the *protoreflect*
	// FileDescriptor returned by ProtoReflect().Descriptor().ParentFile()
	// will see zero services for wiresmith-generated files. In particular,
	// the grpc-go reflection service's FileContainingSymbol /
	// FileByFilename flows resolve services by walking protoregistry-
	// registered file descriptors — those calls will not surface the
	// stripped services for a wiresmith-owned file. This is consistent
	// with wiresmith's design (services are handled by protoc-gen-go-grpc,
	// not us), and grpc reflection's standard service-listing path
	// (`grpc.Server.GetServiceInfo`, used by `reflection.Register`) still
	// works because it reads the live `grpc.ServiceDesc` registered by
	// protoc-gen-go-grpc rather than the protoreflect descriptor. Emitting
	// the correct NumServices / method input-output dep indices to keep
	// services in the raw descriptor would require wiresmith to model
	// service descriptors at codegen time and is deliberately out of
	// scope; revisit if a concrete consumer needs the descriptor view.
	fdp.Service = nil
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(fdp)
	if err != nil {
		panic(fmt.Sprintf("marshaling file descriptor for %s: %v", fd.Path(), err))
	}
	return b
}

func filterOutDep(deps []string, drop string) []string {
	out := deps[:0]
	for _, d := range deps {
		if d != drop {
			out = append(out, d)
		}
	}
	return out
}

// encodeRawDescriptor writes bytes as a Go string literal with \x hex escapes.
func encodeRawDescriptor(buf *bytes.Buffer, b []byte) {
	const lineLen = 40
	for i := 0; i < len(b); i += lineLen {
		end := i + lineLen
		if end > len(b) {
			end = len(b)
		}
		buf.WriteString("\t\"")
		for _, c := range b[i:end] {
			fmt.Fprintf(buf, "\\x%02x", c)
		}
		buf.WriteString("\"")
		if end < len(b) {
			buf.WriteString(" +\n")
		} else {
			buf.WriteString("\n")
		}
	}
}

// sanitizeFileVarName converts a proto file path to a valid Go identifier prefix.
func sanitizeFileVarName(path string) string {
	var b strings.Builder
	b.WriteString("file_")
	for _, c := range path {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
