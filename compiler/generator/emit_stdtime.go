package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// fileHasStdtime reports whether any field in fd is annotated with
// `(wiresmith.options.stdtime) = true`. Used by emitStdtimeHelpers to decide
// whether the per-file Timestamp helpers need to be emitted; empty
// stdtime usage must not pull `time` / `io` / `fmt` imports into a file
// that has none of its own.
func (fg *FileGenerator) fileHasStdtime(fd protoreflect.FileDescriptor) bool {
	if fg.stdtimeExt == nil {
		return false
	}
	found := false
	walkFields(fd, func(field protoreflect.FieldDescriptor) {
		if found {
			return
		}
		if hasStdtimeOption(fg.stdtimeExt, field) {
			found = true
		}
	})
	return found
}

// emitStdtimeHelpers emits the per-file Timestamp helper trio
// (`sizeStdTime`, `encodeStdTime`, `decodeStdTime`) referenced by every
// stdtime call site. They live at package scope and share the same wire
// shape across every stdtime-annotated field in the file — emitting them
// once per file keeps the call sites short and reuses one tag-decode loop
// for all Timestamps in the package.
//
// The decoder uses the local `skipValue` helper for unknown / wire-type-
// mismatched inner fields, so it must be emitted in the same file as
// skipValue. Routing through emitAllUnmarshalMethods (which is where
// skipValue is emitted) guarantees that ordering without an explicit cross-
// pass dependency.
//
// Caller-side imports (`time` on the struct field, protohelpers /
// protowire on the call sites) are already registered by the call-site
// emitters; this helper-side registration handles `io` and `fmt`, plus
// the helpers' own `time`, so the implementation compiles even when the
// helpers are the only consumers of those packages in the file.
func (fg *FileGenerator) emitStdtimeHelpers(fd protoreflect.FileDescriptor) {
	if !fg.fileHasStdtime(fd) {
		return
	}
	fg.imports.addImport("time", "")
	fg.imports.addImport("io", "")
	fg.imports.addImport("fmt", "")
	fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")
	fg.imports.addImport(protohelpersImport, "")

	// sizeStdTime returns the inner Timestamp payload size (excluding the
	// outer field tag and length prefix). Returns 0 for the Go zero value
	// so the caller's outer `!IsZero()` gate and this helper's "skip-when-
	// default" inner rule agree on what "empty" means.
	fmt.Fprintf(fg.body, "// sizeStdTime returns the wire size of t encoded as a Timestamp\n")
	fmt.Fprintf(fg.body, "// payload (the bytes inside the outer length-delimited header). Returns 0\n")
	fmt.Fprintf(fg.body, "// for the Go zero time, which is treated as \"not set\" by the\n")
	fmt.Fprintf(fg.body, "// (wiresmith.options.stdtime) contract.\n")
	fmt.Fprintf(fg.body, "func sizeStdTime(t time.Time) int {\n")
	fmt.Fprintf(fg.body, "\tif t.IsZero() {\n\t\treturn 0\n\t}\n")
	fmt.Fprintf(fg.body, "\tseconds := t.Unix()\n")
	fmt.Fprintf(fg.body, "\tnanos := int32(t.Nanosecond())\n")
	fmt.Fprintf(fg.body, "\tn := 0\n")
	fmt.Fprintf(fg.body, "\tif seconds != 0 {\n\t\tn += 1 + protohelpers.SizeOfVarint(uint64(seconds))\n\t}\n")
	fmt.Fprintf(fg.body, "\tif nanos != 0 {\n\t\tn += 1 + protohelpers.SizeOfVarint(uint64(nanos))\n\t}\n")
	fmt.Fprintf(fg.body, "\treturn n\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// encodeStdTime writes the inner Timestamp payload (no outer header) in
	// reverse-write order into dAtA[:offset]. nanos (field 2) is written
	// first so the resulting bytes appear in ascending tag order — the
	// canonical proto3 emit order — once the caller flips the slice around
	// via the reverse-write convention.
	//
	// The function is unguarded against `t.IsZero()`; callers gate the
	// envelope before invoking encodeStdTime. The skip-when-default rule on
	// the inner fields keeps the encoded bytes consistent with the gogoproto
	// wire format for explicit zero (Timestamp{seconds=0, nanos=0} encodes
	// as an empty length-delimited payload, not a payload containing two
	// zero subfields).
	fmt.Fprintf(fg.body, "// encodeStdTime writes the Timestamp payload for t in reverse-write order\n")
	fmt.Fprintf(fg.body, "// into dAtA, ending at offset. Caller must reserve sizeStdTime(t) bytes\n")
	fmt.Fprintf(fg.body, "// before offset and must check t.IsZero() before invoking — encodeStdTime\n")
	fmt.Fprintf(fg.body, "// does not gate the envelope itself.\n")
	fmt.Fprintf(fg.body, "func encodeStdTime(dAtA []byte, offset int, t time.Time) int {\n")
	fmt.Fprintf(fg.body, "\tseconds := t.Unix()\n")
	fmt.Fprintf(fg.body, "\tnanos := int32(t.Nanosecond())\n")
	fmt.Fprintf(fg.body, "\tif nanos != 0 {\n")
	fmt.Fprintf(fg.body, "\t\toffset = protohelpers.EncodeVarint(dAtA, offset, uint64(nanos))\n")
	fmt.Fprintf(fg.body, "\t\toffset--\n")
	// Inner field 2 (nanos), wire type 0 → tag byte 0x10.
	fmt.Fprintf(fg.body, "\t\tdAtA[offset] = 0x10\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\tif seconds != 0 {\n")
	fmt.Fprintf(fg.body, "\t\toffset = protohelpers.EncodeVarint(dAtA, offset, uint64(seconds))\n")
	fmt.Fprintf(fg.body, "\t\toffset--\n")
	// Inner field 1 (seconds), wire type 0 → tag byte 0x08.
	fmt.Fprintf(fg.body, "\t\tdAtA[offset] = 0x08\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\treturn offset\n")
	fmt.Fprintf(fg.body, "}\n\n")

	// decodeStdTime parses an inner Timestamp payload and returns the
	// corresponding UTC `time.Time`. UTC is the canonical Timestamp zone in
	// the proto spec; even malformed-but-valid wire data that came from a
	// non-UTC source decodes consistently because seconds/nanos carry no
	// zone information themselves.
	//
	// Unknown inner fields and wire-type mismatches on the two known fields
	// route through skipValue — a Timestamp encoded by a future schema with
	// extra inner fields stays decodable, in keeping with the rest of the
	// generated unmarshal's tolerance to forward-compatible additions.
	fmt.Fprintf(fg.body, "// decodeStdTime parses a Timestamp payload (the bytes inside the outer\n")
	fmt.Fprintf(fg.body, "// length-delimited header) and returns the corresponding UTC time.Time.\n")
	fmt.Fprintf(fg.body, "// Used by (wiresmith.options.stdtime) field unmarshalers.\n")
	fmt.Fprintf(fg.body, "func decodeStdTime(dAtA []byte) (time.Time, error) {\n")
	fmt.Fprintf(fg.body, "\tvar seconds int64\n")
	fmt.Fprintf(fg.body, "\tvar nanos int32\n")
	fmt.Fprintf(fg.body, "\tiNdEx := 0\n")
	fmt.Fprintf(fg.body, "\tl := len(dAtA)\n")
	fmt.Fprintf(fg.body, "\tfor iNdEx < l {\n")
	// Inline tag decode — mirrors EmitConsumeTagAt but without import side
	// effects since fg.imports already has them at this point.
	fmt.Fprintf(fg.body, "\t\tvar wire uint64\n")
	fmt.Fprintf(fg.body, "\t\tif iNdEx < l && dAtA[iNdEx] < 0x80 {\n")
	fmt.Fprintf(fg.body, "\t\t\twire = uint64(dAtA[iNdEx])\n")
	fmt.Fprintf(fg.body, "\t\t\tiNdEx++\n")
	fmt.Fprintf(fg.body, "\t\t} else {\n")
	fmt.Fprintf(fg.body, "\t\t\tfor shift := uint(0); ; shift += 7 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif shift >= 35 {\n\t\t\t\t\treturn time.Time{}, fmt.Errorf(\"proto: integer overflow\")\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif iNdEx >= l {\n\t\t\t\t\treturn time.Time{}, io.ErrUnexpectedEOF\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tb := dAtA[iNdEx]\n")
	fmt.Fprintf(fg.body, "\t\t\t\tiNdEx++\n")
	fmt.Fprintf(fg.body, "\t\t\t\twire |= uint64(b&0x7F) << shift\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif b < 0x80 {\n\t\t\t\t\tbreak\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t}\n")
	fmt.Fprintf(fg.body, "\t\tfieldNum := int32(wire >> 3)\n")
	fmt.Fprintf(fg.body, "\t\twireType := int(wire & 0x7)\n")
	fmt.Fprintf(fg.body, "\t\tif fieldNum < 1 {\n\t\t\treturn time.Time{}, fmt.Errorf(\"proto: invalid field number\")\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t\tswitch fieldNum {\n")
	// case 1: seconds (varint, int64)
	fmt.Fprintf(fg.body, "\t\tcase 1:\n")
	fmt.Fprintf(fg.body, "\t\t\tif wireType != 0 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\tn, err := skipValue(dAtA[iNdEx:], wireType, fieldNum)\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif err != nil {\n\t\t\t\t\treturn time.Time{}, err\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tiNdEx += n\n")
	fmt.Fprintf(fg.body, "\t\t\t\tcontinue\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tvar v uint64\n")
	fmt.Fprintf(fg.body, "\t\t\tfor shift := uint(0); ; shift += 7 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif shift >= 64 {\n\t\t\t\t\treturn time.Time{}, fmt.Errorf(\"proto: integer overflow\")\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif iNdEx >= l {\n\t\t\t\t\treturn time.Time{}, io.ErrUnexpectedEOF\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tb := dAtA[iNdEx]\n")
	fmt.Fprintf(fg.body, "\t\t\t\tiNdEx++\n")
	fmt.Fprintf(fg.body, "\t\t\t\tv |= uint64(b&0x7F) << shift\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif b < 0x80 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tif shift == 63 && b > 1 {\n\t\t\t\t\t\treturn time.Time{}, fmt.Errorf(\"proto: varint overflow\")\n\t\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tbreak\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tseconds = int64(v)\n")
	// case 2: nanos (varint, int32)
	fmt.Fprintf(fg.body, "\t\tcase 2:\n")
	fmt.Fprintf(fg.body, "\t\t\tif wireType != 0 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\tn, err := skipValue(dAtA[iNdEx:], wireType, fieldNum)\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif err != nil {\n\t\t\t\t\treturn time.Time{}, err\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tiNdEx += n\n")
	fmt.Fprintf(fg.body, "\t\t\t\tcontinue\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tvar v uint64\n")
	fmt.Fprintf(fg.body, "\t\t\tfor shift := uint(0); ; shift += 7 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif shift >= 64 {\n\t\t\t\t\treturn time.Time{}, fmt.Errorf(\"proto: integer overflow\")\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif iNdEx >= l {\n\t\t\t\t\treturn time.Time{}, io.ErrUnexpectedEOF\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\tb := dAtA[iNdEx]\n")
	fmt.Fprintf(fg.body, "\t\t\t\tiNdEx++\n")
	fmt.Fprintf(fg.body, "\t\t\t\tv |= uint64(b&0x7F) << shift\n")
	fmt.Fprintf(fg.body, "\t\t\t\tif b < 0x80 {\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tif shift == 63 && b > 1 {\n\t\t\t\t\t\treturn time.Time{}, fmt.Errorf(\"proto: varint overflow\")\n\t\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t\t\tbreak\n\t\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tnanos = int32(v)\n")
	fmt.Fprintf(fg.body, "\t\tdefault:\n")
	fmt.Fprintf(fg.body, "\t\t\tn, err := skipValue(dAtA[iNdEx:], wireType, fieldNum)\n")
	fmt.Fprintf(fg.body, "\t\t\tif err != nil {\n\t\t\t\treturn time.Time{}, err\n\t\t\t}\n")
	fmt.Fprintf(fg.body, "\t\t\tiNdEx += n\n")
	fmt.Fprintf(fg.body, "\t\t}\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\tif iNdEx > l {\n\t\treturn time.Time{}, io.ErrUnexpectedEOF\n\t}\n")
	fmt.Fprintf(fg.body, "\treturn time.Unix(seconds, int64(nanos)).UTC(), nil\n")
	fmt.Fprintf(fg.body, "}\n\n")
}
