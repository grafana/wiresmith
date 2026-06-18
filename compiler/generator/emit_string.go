package generator

import (
	"bytes"
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// String() methods are emitted into the companion `<name>_string.pb.go` file,
// not the main `.pb.go`. Same icache/iTLB rationale as the reflect/compare/equal
// splits: String() is a debug method, never called from Marshal/Unmarshal/Size,
// and the per-field body is sizeable, so co-locating it with the hot path only
// costs cache locality. The file needs only fmt/strconv/sort/strings — never
// protoimpl.
//
// The body is hand-rolled per field (the gogoproto `stringer` model, mirroring
// emit_compare.go / emit_equal.go) and renders proto-text — the same shape
// protoc-gen-go / gogo users see, so migrators get a familiar dump:
//
//	field_name: value field_name: value nested: {inner: 1} rep: 1 rep: 2
//
// It deliberately does NOT use protoimpl.X.MessageStringOf / prototext: those
// walk fields through the protoreflect API, which wiresmith's ProtoReflect
// bridge panics on (value-typed message fields are incompatible with the
// official field converters — see protohelpers/message.go).
//
// Determinism: the old fmt.Sprintf("%v", *m) printed a %p hex address for every
// pointer field at depth >= 1 (optional scalars, oneof payloads, pointer-option,
// recursive) and leaked the XXX_fieldsPresent bitmap. The hand-rolled form
// dereferences every pointer, renders enums by name, sorts map entries, and
// recurses into nested messages via their own String(), so output is content-
// AND byte-deterministic (no prototext detrand whitespace jitter).
//
// Omission: fields walk in ascending field number (proto-text convention) and
// follow the same "is it on the wire" predicate the marshaler uses — singular
// scalars are omitted when zero, value messages when empty unless the presence
// bitmap says set, pointers/oneofs when nil, repeated/maps when empty. So the
// dump reflects exactly what Marshal would emit.
func (fg *FileGenerator) emitAllStringMethods(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, fg.emitString)
}

// stringSep is the proto-text field separator. Every rendered field appends a
// trailing space; the final result is TrimSpace'd, so there is no "first field"
// bookkeeping.
const stringSep = " "

func (fg *FileGenerator) emitString(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	out := fg.stringBody
	fg.stringImports.addImport("strings", "")

	fmt.Fprintf(out, "func (m *%s) String() string {\n", name)
	fmt.Fprintf(out, "\tif m == nil {\n\t\treturn \"<nil>\"\n\t}\n")
	fmt.Fprintf(out, "\tvar b strings.Builder\n")

	pm := fg.presenceMap(md)
	seenOneofs := map[string]bool{}
	for _, fld := range sortedByTag(md) {
		if isRealOneof(fld) {
			oo := fld.ContainingOneof()
			ooName := string(oo.Name())
			if seenOneofs[ooName] {
				continue
			}
			seenOneofs[ooName] = true
			fg.emitOneofString(out, md, oo)
			continue
		}
		fg.emitFieldString(out, fld, pm)
	}

	fmt.Fprintf(out, "\treturn strings.TrimSpace(b.String())\n}\n\n")
}

// emitFieldString renders one non-oneof field in proto-text, guarded by the
// same presence/zero predicate the marshaler uses so an absent field produces
// no output.
func (fg *FileGenerator) emitFieldString(out *bytes.Buffer, fld protoreflect.FieldDescriptor, pm map[protoreflect.FieldNumber]int) {
	protoName := string(fld.Name())
	access := "m." + fg.goFieldName(fld)

	switch {
	case fld.IsMap():
		fg.emitMapString(out, fld, protoName, access)

	case fld.IsList():
		fg.emitRepeatedString(out, fld, protoName, access)

	case fld.HasOptionalKeyword():
		// proto3 optional: scalar/message → *T (nil = absent); bytes stays
		// []byte (nil = absent). Guard on nil, then render the value.
		if fld.Kind() == protoreflect.BytesKind {
			fmt.Fprintf(out, "\tif %s != nil {\n", access)
			fg.emitLeafString(out, "\t\t", fld, protoName, access)
			fmt.Fprintf(out, "\t}\n")
			return
		}
		fmt.Fprintf(out, "\tif %s != nil {\n", access)
		// Dereference the pointer to the value for leaf formatting / recursion.
		fg.emitLeafString(out, "\t\t", fld, protoName, "(*"+access+")")
		fmt.Fprintf(out, "\t}\n")

	case fg.isWiresmithMessage(fld) && fg.hasPointerOption(fld):
		// Singular (pointer)-option message: *Msg, nil = absent.
		fmt.Fprintf(out, "\tif %s != nil {\n", access)
		fg.emitLeafString(out, "\t\t", fld, protoName, access)
		fmt.Fprintf(out, "\t}\n")

	case fg.isWiresmithMessage(fld):
		// Singular value message: emit when non-empty (Size()>0) OR the
		// presence bitmap says it was set — matching the marshaler's
		// present-but-empty handling.
		fmt.Fprintf(out, "\tif %s.Size() > 0", access)
		if bitIndex, ok := pm[fld.Number()]; ok {
			fmt.Fprintf(out, " || %s", presenceCheck(bitIndex))
		}
		fmt.Fprintf(out, " {\n")
		fg.emitLeafString(out, "\t\t", fld, protoName, access)
		fmt.Fprintf(out, "\t}\n")

	default:
		// Singular scalar / bytes / enum / option-substituted value type
		// (stdtime/stdduration/customtype/casttype). proto3 omits the zero
		// value on the wire; mirror that with a per-kind non-zero guard.
		guard := fg.scalarPresenceGuard(fld, access)
		if guard != "" {
			fmt.Fprintf(out, "\tif %s {\n", guard)
			fg.emitLeafString(out, "\t\t", fld, protoName, access)
			fmt.Fprintf(out, "\t}\n")
		} else {
			fg.emitLeafString(out, "\t", fld, protoName, access)
		}
	}
}

// scalarPresenceGuard returns a Go boolean expression that is true when the
// singular field should appear in the dump (proto3: non-zero value on the
// wire). Returns "" when the field is always rendered — option-substituted
// value types, whose zero is type-specific; rendering one unconditionally is
// harmless and deterministic.
func (fg *FileGenerator) scalarPresenceGuard(fld protoreflect.FieldDescriptor, access string) string {
	if fg.suppressMessageType(fld) {
		return ""
	}
	switch fld.Kind() {
	case protoreflect.StringKind, protoreflect.BytesKind:
		return "len(" + access + ") > 0"
	case protoreflect.BoolKind:
		return access
	default:
		// Integers, floats, enums: zero value is the additive identity.
		return access + " != 0"
	}
}

// emitLeafString writes `protoName: <value>` plus the trailing separator at the
// given indent. access already resolves to the VALUE (pointers dereferenced by
// the caller). Message values recurse through their String() wrapped in braces;
// scalars are formatted by emitScalarValueString.
func (fg *FileGenerator) emitLeafString(out *bytes.Buffer, indent string, fld protoreflect.FieldDescriptor, protoName, access string) {
	fmt.Fprintf(out, "%sb.WriteString(%q)\n", indent, protoName+": ")

	if fg.isWiresmithMessage(fld) {
		// Nested message in proto-text brace form: `name: {<body>}`. The child
		// String() is itself the brace-less field list, so wrap it. Go auto-
		// addresses an addressable value to call the pointer-receiver String();
		// String() is nil-safe for the pointer case.
		fmt.Fprintf(out, "%sb.WriteString(\"{\")\n", indent)
		fmt.Fprintf(out, "%sb.WriteString(%s.String())\n", indent, access)
		fmt.Fprintf(out, "%sb.WriteString(\"}\")\n", indent)
		fmt.Fprintf(out, "%sb.WriteString(%q)\n", indent, stringSep)
		return
	}

	fg.emitScalarValueString(out, indent, fld, access)
	fmt.Fprintf(out, "%sb.WriteString(%q)\n", indent, stringSep)
}

// emitScalarValueString writes just the value (no name, no separator) for a
// non-message leaf: enum by name, string/bytes quoted, everything else bare.
// Option-substituted value types format via %v on the VALUE (never a pointer,
// so no address), honouring any Stringer they implement (time.Time,
// time.Duration, customtype/casttype types).
func (fg *FileGenerator) emitScalarValueString(out *bytes.Buffer, indent string, fld protoreflect.FieldDescriptor, access string) {
	// Option-substituted value types (stdtime/stdduration/customtype) and
	// casttype (a defined type over the natural scalar, e.g. `type UserID
	// int64` / `type TenantTag string`) carry a Go type that is NOT the plain
	// scalar, so strconv.Quote/etc. don't typecheck. Format via %v on the VALUE
	// — address-free, and honours any Stringer the type implements.
	if _, isCast := fg.casttypeGoFieldType(fld); isCast || fg.suppressMessageType(fld) {
		fg.stringImports.addImport("fmt", "")
		fmt.Fprintf(out, "%sfmt.Fprintf(&b, \"%%v\", %s)\n", indent, access)
		return
	}

	switch fld.Kind() {
	case protoreflect.EnumKind:
		// proto-text prints the enum value NAME; the generated enum String()
		// does the _name lookup with an integer fallback for unknown values.
		fmt.Fprintf(out, "%sb.WriteString(%s.String())\n", indent, access)
	case protoreflect.StringKind:
		fg.stringImports.addImport("strconv", "")
		fmt.Fprintf(out, "%sb.WriteString(strconv.Quote(%s))\n", indent, access)
	case protoreflect.BytesKind:
		// proto-text renders bytes as a quoted, escaped string.
		fg.stringImports.addImport("strconv", "")
		fmt.Fprintf(out, "%sb.WriteString(strconv.Quote(string(%s)))\n", indent, access)
	default:
		// Integers, floats, bool: bare value. %v is address-free for these
		// concrete value kinds and matches proto-text (true/false, decimals).
		fg.stringImports.addImport("fmt", "")
		fmt.Fprintf(out, "%sfmt.Fprintf(&b, \"%%v\", %s)\n", indent, access)
	}
}

// emitRepeatedString renders a repeated field as one `name: <elem>` entry per
// element (proto-text convention); an empty slice produces no output.
func (fg *FileGenerator) emitRepeatedString(out *bytes.Buffer, fld protoreflect.FieldDescriptor, protoName, access string) {
	fmt.Fprintf(out, "\tfor _, e := range %s {\n", access)
	if fg.isWiresmithMessage(fld) {
		// []Msg or []*Msg: render each as `name: {<body>}`. String() is
		// nil-safe, so a nil *Msg element renders "<nil>" inside the braces.
		fmt.Fprintf(out, "\t\tb.WriteString(%q)\n", protoName+": ")
		fmt.Fprintf(out, "\t\tb.WriteString(\"{\")\n")
		fmt.Fprintf(out, "\t\tb.WriteString(e.String())\n")
		fmt.Fprintf(out, "\t\tb.WriteString(\"}\")\n")
		fmt.Fprintf(out, "\t\tb.WriteString(%q)\n", stringSep)
	} else {
		fg.emitLeafString(out, "\t\t", fld, protoName, "e")
	}
	fmt.Fprintf(out, "\t}\n")
}

// emitMapString renders a map deterministically as one
// `name: {key: <k> value: <v>}` entry per key, with entries sorted by their
// rendered text (a total order that works for every key type, including bool,
// where `<` is undefined). Message values recurse via String().
func (fg *FileGenerator) emitMapString(out *bytes.Buffer, fld protoreflect.FieldDescriptor, protoName, access string) {
	fg.stringImports.addImport("sort", "")
	fg.stringImports.addImport("strings", "")

	keyKind := fld.MapKey().Kind()
	valKind := fld.MapValue().Kind()

	fmt.Fprintf(out, "\t{\n")
	fmt.Fprintf(out, "\t\tentries := make([]string, 0, len(%s))\n", access)
	fmt.Fprintf(out, "\t\tfor k, v := range %s {\n", access)
	fmt.Fprintf(out, "\t\t\tvar eb strings.Builder\n")
	fg.emitMapScalar(out, "\t\t\t", "eb", "key: ", keyKind, "k")
	if valKind == protoreflect.MessageKind {
		fmt.Fprintf(out, "\t\t\teb.WriteString(\"value: {\")\n")
		fmt.Fprintf(out, "\t\t\teb.WriteString(v.String())\n")
		fmt.Fprintf(out, "\t\t\teb.WriteString(\"}\")\n")
	} else {
		fg.emitMapScalar(out, "\t\t\t", "eb", "value: ", valKind, "v")
	}
	fmt.Fprintf(out, "\t\t\tentries = append(entries, strings.TrimSpace(eb.String()))\n")
	fmt.Fprintf(out, "\t\t}\n")
	fmt.Fprintf(out, "\t\tsort.Strings(entries)\n")
	fmt.Fprintf(out, "\t\tfor _, e := range entries {\n")
	fmt.Fprintf(out, "\t\t\tb.WriteString(%q)\n", protoName+": {")
	fmt.Fprintf(out, "\t\t\tb.WriteString(e)\n")
	fmt.Fprintf(out, "\t\t\tb.WriteString(\"}\")\n")
	fmt.Fprintf(out, "\t\t\tb.WriteString(%q)\n", stringSep)
	fmt.Fprintf(out, "\t\t}\n")
	fmt.Fprintf(out, "\t}\n")
}

// emitMapScalar writes `label<value> ` into the named builder for a map key or
// scalar value. Strings/bytes are quoted; everything else is bare via %v.
func (fg *FileGenerator) emitMapScalar(out *bytes.Buffer, indent, builder, label string, kind protoreflect.Kind, access string) {
	fmt.Fprintf(out, "%s%s.WriteString(%q)\n", indent, builder, label)
	switch kind {
	case protoreflect.StringKind:
		fg.stringImports.addImport("strconv", "")
		fmt.Fprintf(out, "%s%s.WriteString(strconv.Quote(%s))\n", indent, builder, access)
	case protoreflect.BytesKind:
		fg.stringImports.addImport("strconv", "")
		fmt.Fprintf(out, "%s%s.WriteString(strconv.Quote(string(%s)))\n", indent, builder, access)
	case protoreflect.EnumKind:
		fmt.Fprintf(out, "%s%s.WriteString(%s.String())\n", indent, builder, access)
	default:
		fg.stringImports.addImport("fmt", "")
		fmt.Fprintf(out, "%sfmt.Fprintf(&%s, \"%%v\", %s)\n", indent, builder, access)
	}
	fmt.Fprintf(out, "%s%s.WriteString(%q)\n", indent, builder, stringSep)
}

// isWiresmithMessage reports whether fld is a message field whose Go type is a
// wiresmith-generated message (so it owns a pointer-receiver String() to
// recurse through). Option substitutions (stdtime/stdduration/customtype on a
// message field) replace it with a value type and are NOT wiresmith messages.
func (fg *FileGenerator) isWiresmithMessage(fld protoreflect.FieldDescriptor) bool {
	return fld.Kind() == protoreflect.MessageKind && !fg.suppressMessageType(fld)
}

// emitOneofString renders the set oneof variant as its own proto field
// (`variant_name: value`), or nothing when unset.
func (fg *FileGenerator) emitOneofString(out *bytes.Buffer, md protoreflect.MessageDescriptor, oo protoreflect.OneofDescriptor) {
	access := "m." + snakeToPascal(string(oo.Name()))

	fmt.Fprintf(out, "\tswitch v := %s.(type) {\n", access)
	for _, fld := range sortedByTagOneof(oo) {
		variantType := oneofVariantName(md, fld)
		fieldName := fg.goFieldName(fld)
		protoName := string(fld.Name())
		fmt.Fprintf(out, "\tcase *%s:\n", variantType)
		// Oneof variants are never option-substituted (stdtime/customtype
		// reject oneof), so a message payload is always a wiresmith *Msg.
		fg.emitLeafString(out, "\t\t", fld, protoName, "v."+fieldName)
	}
	fmt.Fprintf(out, "\t}\n")
}

// sortedByTagOneof returns a oneof's variants in ascending field number.
func sortedByTagOneof(oo protoreflect.OneofDescriptor) []protoreflect.FieldDescriptor {
	n := oo.Fields().Len()
	out := make([]protoreflect.FieldDescriptor, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, oo.Fields().Get(i))
	}
	sortFieldsByNumber(out)
	return out
}

// sortFieldsByNumber sorts a field slice ascending by wire number in place
// (insertion sort — variant counts are tiny).
func sortFieldsByNumber(fs []protoreflect.FieldDescriptor) {
	for i := 1; i < len(fs); i++ {
		for j := i; j > 0 && fs[j-1].Number() > fs[j].Number(); j-- {
			fs[j-1], fs[j] = fs[j], fs[j-1]
		}
	}
}
