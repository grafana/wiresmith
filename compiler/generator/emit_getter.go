package generator

import (
	"fmt"

	"github.com/grafana/wiresmith/compiler/types"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitAllGetterMethods(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, fg.emitGetters)
}

func (fg *FileGenerator) emitGetters(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	pm := fg.presenceMap(md)

	seenOneofs := map[string]bool{}
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)

		if isRealOneof(fd) {
			oo := fd.ContainingOneof()
			ooName := string(oo.Name())
			if !seenOneofs[ooName] {
				seenOneofs[ooName] = true
				fg.emitOneofGetter(md, oo)
			}
			fg.emitOneofVariantGetter(md, fd)
			continue
		}

		goName := fg.goFieldName(fd)

		if fd.IsMap() {
			goType := fg.imports.goType(fd)
			fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", name, goName, goType)
			fmt.Fprintf(fg.body, "\tif m != nil {\n\t\treturn m.%s\n\t}\n", goName)
			fmt.Fprintf(fg.body, "\treturn nil\n}\n\n")
			continue
		}

		if fd.IsList() {
			goType := fg.goFieldType(fd)
			fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", name, goName, goType)
			fmt.Fprintf(fg.body, "\tif m != nil {\n\t\treturn m.%s\n\t}\n", goName)
			fmt.Fprintf(fg.body, "\treturn nil\n}\n\n")
			continue
		}

		if fd.HasOptionalKeyword() {
			fg.emitOptionalGetter(name, fd, goName)
			continue
		}

		// Singular `(wiresmith.options.pointer) = true` message: the struct field is
		// already `*Msg`, so the getter returns it directly with the standard
		// nil-receiver guard. No bitmap involvement — pointer-message fields
		// are excluded from fieldsForPresence (presence is the nil check).
		if fd.Kind() == protoreflect.MessageKind && fg.hasPointerOption(fd) {
			msgType := fg.imports.goSingularType(fd)
			fmt.Fprintf(fg.body, "func (m *%s) Get%s() *%s {\n", name, goName, msgType)
			fmt.Fprintf(fg.body, "\tif m != nil {\n\t\treturn m.%s\n\t}\n", goName)
			fmt.Fprintf(fg.body, "\treturn nil\n}\n\n")
			continue
		}

		// `(wiresmith.options.stdtime) = true` on a singular Timestamp field
		// produces a `time.Time` Go field, so the getter returns the value
		// directly with the standard nil-receiver guard. fieldsForPresence
		// excludes stdtime fields (Go's `time.Time{}` is the presence sentinel),
		// so the bitmap path is bypassed here even though the field is
		// MessageKind on the wire.
		if stdType, ok := fg.stdtimeGoFieldType(fd); ok {
			fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", name, goName, stdType)
			fmt.Fprintf(fg.body, "\tif m != nil {\n\t\treturn m.%s\n\t}\n", goName)
			fmt.Fprintf(fg.body, "\treturn %s{}\n}\n\n", stdType)
			continue
		}

		// Message value-type fields: return pointer, nil when bitmap says absent.
		// Currently the bitmap entry always exists here (same filter predicate
		// as fieldsForPresence), but we keep the fallback so the getter
		// degrades to a plain nil-safe accessor if the predicates ever diverge.
		if fd.Kind() == protoreflect.MessageKind {
			msgType := fg.imports.goSingularType(fd)
			bitIndex, hasBit := pm[fd.Number()]
			fmt.Fprintf(fg.body, "func (m *%s) Get%s() *%s {\n", name, goName, msgType)
			if hasBit {
				fmt.Fprintf(fg.body, "\tif m != nil && %s {\n", presenceCheck(bitIndex))
			} else {
				// Defensive: emitted when the field has no presence-bitmap entry.
				fmt.Fprintf(fg.body, "\tif m != nil {\n")
			}
			fmt.Fprintf(fg.body, "\t\treturn &m.%s\n\t}\n", goName)
			fmt.Fprintf(fg.body, "\treturn nil\n}\n\n")
			continue
		}

		// `(wiresmith.options.customtype)` swaps the field type for a
		// user-supplied Go type whose zero literal we don't know — use a
		// `var zero T` declaration so the getter works for any kind shape
		// (named primitive alias, struct, pointer wrapper, …).
		if ctType, ok := fg.customtypeGoFieldType(fd); ok {
			fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", name, goName, ctType)
			fmt.Fprintf(fg.body, "\tif m != nil {\n\t\treturn m.%s\n\t}\n", goName)
			fmt.Fprintf(fg.body, "\tvar zero %s\n\treturn zero\n}\n\n", ctType)
			continue
		}

		// Scalar fields: nil-safe return.
		goType := fg.imports.goSingularType(fd)
		fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", name, goName, goType)
		fmt.Fprintf(fg.body, "\tif m != nil {\n\t\treturn m.%s\n\t}\n", goName)
		fmt.Fprintf(fg.body, "\treturn %s\n}\n\n", types.ScalarZeroLiteral(fd.Kind()))
	}
}

// emitOneofGetter emits the getter for the oneof interface field itself.
func (fg *FileGenerator) emitOneofGetter(md protoreflect.MessageDescriptor, oo protoreflect.OneofDescriptor) {
	name := goMessageTypeName(md)
	goName := snakeToPascal(string(oo.Name()))
	ifaceName := oneofInterfaceName(md, oo)
	fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", name, goName, ifaceName)
	fmt.Fprintf(fg.body, "\tif m != nil {\n\t\treturn m.%s\n\t}\n", goName)
	fmt.Fprintf(fg.body, "\treturn nil\n}\n\n")
}

// emitOneofVariantGetter emits a typed getter for one oneof variant.
func (fg *FileGenerator) emitOneofVariantGetter(md protoreflect.MessageDescriptor, fd protoreflect.FieldDescriptor) {
	name := goMessageTypeName(md)
	oo := fd.ContainingOneof()
	ooGoName := snakeToPascal(string(oo.Name()))
	fieldGoName := fg.goFieldName(fd)
	variantType := oneofVariantName(md, fd)

	// Message variants return a pointer into the variant struct so callers
	// can mutate the embedded message; every other kind returns the field
	// by value with a typed zero fallback (bytes' zero literal `nil`
	// matches the []byte return type, so it falls into the default path).
	if fd.Kind() == protoreflect.MessageKind {
		msgType := fg.imports.goSingularType(fd)
		fmt.Fprintf(fg.body, "func (m *%s) Get%s() *%s {\n", name, fieldGoName, msgType)
		fmt.Fprintf(fg.body, "\tif x, ok := m.Get%s().(*%s); ok {\n", ooGoName, variantType)
		fmt.Fprintf(fg.body, "\t\treturn &x.%s\n\t}\n", fieldGoName)
		fmt.Fprintf(fg.body, "\treturn nil\n}\n\n")
		return
	}
	goType := fg.imports.goSingularType(fd)
	fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", name, fieldGoName, goType)
	fmt.Fprintf(fg.body, "\tif x, ok := m.Get%s().(*%s); ok {\n", ooGoName, variantType)
	fmt.Fprintf(fg.body, "\t\treturn x.%s\n\t}\n", fieldGoName)
	fmt.Fprintf(fg.body, "\treturn %s\n}\n\n", types.ScalarZeroLiteral(fd.Kind()))
}

// emitOptionalGetter emits a getter for an optional (pointer) field.
// For scalar and enum fields, it returns the dereferenced value or the zero value if unset.
// For message and bytes fields, it returns the stored pointer/slice value, or nil if unset.
func (fg *FileGenerator) emitOptionalGetter(typeName string, fd protoreflect.FieldDescriptor, goName string) {
	if fd.Kind() == protoreflect.MessageKind {
		msgType := fg.imports.goSingularType(fd)
		fmt.Fprintf(fg.body, "func (m *%s) Get%s() *%s {\n", typeName, goName, msgType)
		fmt.Fprintf(fg.body, "\tif m != nil {\n\t\treturn m.%s\n\t}\n", goName)
		fmt.Fprintf(fg.body, "\treturn nil\n}\n\n")
		return
	}
	if fd.Kind() == protoreflect.BytesKind {
		fmt.Fprintf(fg.body, "func (m *%s) Get%s() []byte {\n", typeName, goName)
		fmt.Fprintf(fg.body, "\tif m != nil {\n\t\treturn m.%s\n\t}\n", goName)
		fmt.Fprintf(fg.body, "\treturn nil\n}\n\n")
		return
	}
	goType := fg.imports.goSingularType(fd)
	fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", typeName, goName, goType)
	fmt.Fprintf(fg.body, "\tif m != nil && m.%s != nil {\n\t\treturn *m.%s\n\t}\n", goName, goName)
	fmt.Fprintf(fg.body, "\treturn %s\n}\n\n", types.ScalarZeroLiteral(fd.Kind()))
}
