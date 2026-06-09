package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// presenceField stores the bit index for a field that needs presence tracking.
type presenceField struct {
	fd       protoreflect.FieldDescriptor
	bitIndex int
}

// fieldsForPresence returns message fields that need presence-bitmap tracking.
// These are singular fields without their own presence semantics:
// not repeated, not map, not optional (pointer), not oneof (interface),
// and not a singular message field annotated with
// `(wiresmith.options.pointer) = true` (which makes the field a pointer with
// nil-check presence, same shape as optional).
//
// The pointer-option skip is gated on MessageKind to mirror fieldType's actual
// shape change. If validation is ever bypassed and the option is mistakenly
// applied to a scalar, fieldType falls back to the value-type emit — losing
// the bitmap bit here would desynchronize unmarshal/Has tracking.
func (fg *FileGenerator) fieldsForPresence(md protoreflect.MessageDescriptor) []presenceField {
	var fields []presenceField
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if fd.IsList() || fd.IsMap() || fd.HasOptionalKeyword() || isRealOneof(fd) {
			continue
		}
		if fd.Kind() == protoreflect.MessageKind && fg.hasPointerOption(fd) {
			continue
		}
		// `(wiresmith.options.stdtime)` swaps a Timestamp field for a value-
		// type `time.Time`, where presence is `!t.IsZero()` rather than a
		// separate bitmap bit (same role nil plays for pointer-option above).
		// Skipping here keeps Size/Marshal/Unmarshal from emitting bitmap ops
		// for a field whose presence semantics are carried by the value.
		if fd.Kind() == protoreflect.MessageKind && fg.hasStdtimeOption(fd) {
			continue
		}
		// `(wiresmith.options.customtype)` on a singular message field hands
		// the field's wire encoding to the user type entirely — presence is
		// carried by their `SizeWiresmith() > 0` gate, not by a bitmap bit.
		// Without this skip the bitmap would say "present" for a `tag+0`
		// payload that the user's SizeWiresmith() reports as zero, but the
		// customtype Size/Marshal emitters wouldn't honour that bit on
		// re-marshal — a present-but-empty value would silently round-trip
		// to absent.
		if fd.Kind() == protoreflect.MessageKind && fg.hasCustomtypeOption(fd) {
			continue
		}
		// Same shape as stdtime above: `(wiresmith.options.stdduration)`
		// swaps a Duration field for a value-type `time.Duration`, with
		// presence carried by `d != 0` rather than a bitmap bit.
		if fd.Kind() == protoreflect.MessageKind && fg.hasStdDurationOption(fd) {
			continue
		}
		fields = append(fields, presenceField{fd: fd, bitIndex: len(fields)})
	}
	return fields
}

// presenceBitmapWords returns the number of uint64 words needed to store
// the presence bitmap for a message. Returns 0 if no fields need tracking.
func (fg *FileGenerator) presenceBitmapWords(md protoreflect.MessageDescriptor) int {
	pf := fg.fieldsForPresence(md)
	if len(pf) == 0 {
		return 0
	}
	return (len(pf) + 63) / 64
}

// presenceMap builds a lookup from proto field number to bit index
// for fields that need presence tracking.
func (fg *FileGenerator) presenceMap(md protoreflect.MessageDescriptor) map[protoreflect.FieldNumber]int {
	fields := fg.fieldsForPresence(md)
	if len(fields) == 0 {
		return nil
	}
	m := make(map[protoreflect.FieldNumber]int, len(fields))
	for _, pf := range fields {
		m[pf.fd.Number()] = pf.bitIndex
	}
	return m
}

// presenceCheck returns the Go expression "m.fieldsPresent[W]&(1<<B) != 0"
// for the given bit index.
func presenceCheck(bitIndex int) string {
	return fmt.Sprintf("m.fieldsPresent[%d]&(1<<%d) != 0", bitIndex/64, bitIndex%64)
}

// presenceSet returns the Go statement "m.fieldsPresent[W] |= 1 << B"
// for the given bit index.
func presenceSet(bitIndex int) string {
	return fmt.Sprintf("m.fieldsPresent[%d] |= 1 << %d", bitIndex/64, bitIndex%64)
}

func (fg *FileGenerator) emitHasMethods(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	for _, pf := range fg.fieldsForPresence(md) {
		goName := fg.goFieldName(pf.fd)
		fmt.Fprintf(fg.body, "func (m *%s) Has%s() bool {\n", name, goName)
		fmt.Fprintf(fg.body, "\tif m == nil {\n\t\treturn false\n\t}\n")
		fmt.Fprintf(fg.body, "\treturn %s\n", presenceCheck(pf.bitIndex))
		fmt.Fprintf(fg.body, "}\n\n")
	}
	// Optional fields carry presence in the Go field's nil-ability rather
	// than the bitmap — scalar/enum/message optionals surface as `*T`, but
	// `optional bytes` stays a nil-able `[]byte` (matching protoc-gen-go's
	// shape). Either way the test is `m.F != nil`, and these fields are
	// excluded from fieldsForPresence. Emit a separate pure-nil-check
	// Has*() here so callers don't have to drop down to `m.X != nil` for
	// optionals while using `m.HasY()` for value-type fields — matches
	// protobuf-go's emission shape.
	//
	// Excludes oneof variants, maps, and repeated fields by design:
	//   - oneof presence flows through the interface-typed wrapper field
	//     (consumer calls `m.Choice.(*Foo_Bar)` or `m.GetX() != nil`).
	//   - map and repeated have no "set vs unset" — they're either nil/empty
	//     or populated, and len() conveys that without a Has accessor.
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if !fd.HasOptionalKeyword() || isRealOneof(fd) {
			continue
		}
		goName := fg.goFieldName(fd)
		fmt.Fprintf(fg.body, "func (m *%s) Has%s() bool {\n", name, goName)
		fmt.Fprintf(fg.body, "\treturn m != nil && m.%s != nil\n", goName)
		fmt.Fprintf(fg.body, "}\n\n")
	}
}
