package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitAllGetterMethods(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, fg.emitGetters)
}

func (fg *FileGenerator) emitGetters(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	pm := presenceMap(md)

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

		goName := snakeToPascal(string(fd.Name()))

		if fd.IsMap() {
			goType := fg.imports.goType(fd)
			fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", name, goName, goType)
			fmt.Fprintf(fg.body, "\tif m != nil {\n\t\treturn m.%s\n\t}\n", goName)
			fmt.Fprintf(fg.body, "\treturn nil\n}\n\n")
			continue
		}

		if fd.IsList() {
			goType := fg.imports.goType(fd)
			fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", name, goName, goType)
			fmt.Fprintf(fg.body, "\tif m != nil {\n\t\treturn m.%s\n\t}\n", goName)
			fmt.Fprintf(fg.body, "\treturn nil\n}\n\n")
			continue
		}

		if fd.HasOptionalKeyword() {
			fg.emitOptionalGetter(name, fd, goName)
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

		// Scalar fields: nil-safe return.
		goType := fg.imports.goSingularType(fd)
		fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", name, goName, goType)
		fmt.Fprintf(fg.body, "\tif m != nil {\n\t\treturn m.%s\n\t}\n", goName)
		fmt.Fprintf(fg.body, "\treturn %s\n}\n\n", zeroLiteral(fd))
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
	fieldGoName := snakeToPascal(string(fd.Name()))
	variantType := oneofVariantName(md, fd)

	switch fd.Kind() {
	case protoreflect.MessageKind:
		msgType := fg.imports.goSingularType(fd)
		fmt.Fprintf(fg.body, "func (m *%s) Get%s() *%s {\n", name, fieldGoName, msgType)
		fmt.Fprintf(fg.body, "\tif x, ok := m.Get%s().(*%s); ok {\n", ooGoName, variantType)
		fmt.Fprintf(fg.body, "\t\treturn &x.%s\n\t}\n", fieldGoName)
		fmt.Fprintf(fg.body, "\treturn nil\n}\n\n")
	case protoreflect.BytesKind:
		fmt.Fprintf(fg.body, "func (m *%s) Get%s() []byte {\n", name, fieldGoName)
		fmt.Fprintf(fg.body, "\tif x, ok := m.Get%s().(*%s); ok {\n", ooGoName, variantType)
		fmt.Fprintf(fg.body, "\t\treturn x.%s\n\t}\n", fieldGoName)
		fmt.Fprintf(fg.body, "\treturn nil\n}\n\n")
	default:
		goType := fg.imports.goSingularType(fd)
		fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", name, fieldGoName, goType)
		fmt.Fprintf(fg.body, "\tif x, ok := m.Get%s().(*%s); ok {\n", ooGoName, variantType)
		fmt.Fprintf(fg.body, "\t\treturn x.%s\n\t}\n", fieldGoName)
		fmt.Fprintf(fg.body, "\treturn %s\n}\n\n", zeroLiteral(fd))
	}
}

// emitOptionalGetter emits a getter for an optional (pointer) field.
// Returns the dereferenced value, or zero if nil.
func (fg *FileGenerator) emitOptionalGetter(typeName string, fd protoreflect.FieldDescriptor, goName string) {
	if fd.Kind() == protoreflect.BytesKind {
		fmt.Fprintf(fg.body, "func (m *%s) Get%s() []byte {\n", typeName, goName)
		fmt.Fprintf(fg.body, "\tif m != nil {\n\t\treturn m.%s\n\t}\n", goName)
		fmt.Fprintf(fg.body, "\treturn nil\n}\n\n")
		return
	}
	goType := fg.imports.goSingularType(fd)
	fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", typeName, goName, goType)
	fmt.Fprintf(fg.body, "\tif m != nil && m.%s != nil {\n\t\treturn *m.%s\n\t}\n", goName, goName)
	fmt.Fprintf(fg.body, "\treturn %s\n}\n\n", zeroLiteral(fd))
}

func zeroLiteral(fd protoreflect.FieldDescriptor) string {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return "false"
	case protoreflect.StringKind:
		return `""`
	case protoreflect.BytesKind:
		return "nil"
	case protoreflect.EnumKind:
		return "0"
	default:
		return "0"
	}
}
