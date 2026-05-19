package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitAllEqualMethods(fd protoreflect.FileDescriptor) {
	forEachMessage(fd, fg.emitEqual)
}

func (fg *FileGenerator) emitEqual(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)

	if needsBytesImportForEqual(md) {
		fg.imports.addImport("bytes", "")
	}

	fmt.Fprintf(fg.body, "func (this *%s) Equal(that interface{}) bool {\n", name)
	fmt.Fprintf(fg.body, "\tif that == nil {\n\t\treturn this == nil\n\t}\n\n")

	fmt.Fprintf(fg.body, "\tthat1, ok := that.(*%s)\n", name)
	fmt.Fprintf(fg.body, "\tif !ok {\n")
	fmt.Fprintf(fg.body, "\t\tthat2, ok := that.(%s)\n", name)
	fmt.Fprintf(fg.body, "\t\tif ok {\n\t\t\tthat1 = &that2\n\t\t} else {\n\t\t\treturn false\n\t\t}\n")
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\tif that1 == nil {\n\t\treturn this == nil\n\t} else if this == nil {\n\t\treturn false\n\t}\n")

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
		fg.emitFieldEqual(fd, goName)
	}

	fmt.Fprintf(fg.body, "\treturn true\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

func (fg *FileGenerator) emitOneofEqual(md protoreflect.MessageDescriptor, oo protoreflect.OneofDescriptor) {
	goName := snakeToPascal(string(oo.Name()))

	fmt.Fprintf(fg.body, "\tif (this.%s == nil) != (that1.%s == nil) {\n", goName, goName)
	fmt.Fprintf(fg.body, "\t\treturn false\n\t}\n")
	fmt.Fprintf(fg.body, "\tif this.%s != nil {\n", goName)
	fmt.Fprintf(fg.body, "\t\tswitch v := this.%s.(type) {\n", goName)

	for i := 0; i < oo.Fields().Len(); i++ {
		fd := oo.Fields().Get(i)
		variantType := oneofVariantName(md, fd)
		fieldName := snakeToPascal(string(fd.Name()))

		fmt.Fprintf(fg.body, "\t\tcase *%s:\n", variantType)
		fmt.Fprintf(fg.body, "\t\t\tv2, ok := that1.%s.(*%s)\n", goName, variantType)
		fmt.Fprintf(fg.body, "\t\t\tif !ok {\n\t\t\t\treturn false\n\t\t\t}\n")

		switch fd.Kind() {
		case protoreflect.BytesKind:
			fmt.Fprintf(fg.body, "\t\t\tif !bytes.Equal(v.%s, v2.%s) {\n\t\t\t\treturn false\n\t\t\t}\n", fieldName, fieldName)
		case protoreflect.MessageKind:
			fmt.Fprintf(fg.body, "\t\t\tif !v.%s.Equal(v2.%s) {\n\t\t\t\treturn false\n\t\t\t}\n", fieldName, fieldName)
		default:
			fmt.Fprintf(fg.body, "\t\t\tif v.%s != v2.%s {\n\t\t\t\treturn false\n\t\t\t}\n", fieldName, fieldName)
		}
	}

	fmt.Fprintf(fg.body, "\t\tdefault:\n\t\t\treturn false\n")
	fmt.Fprintf(fg.body, "\t\t}\n")
	fmt.Fprintf(fg.body, "\t}\n")
}

func (fg *FileGenerator) emitFieldEqual(fd protoreflect.FieldDescriptor, goName string) {
	if fd.IsList() {
		fg.emitRepeatedFieldEqual(fd, goName)
		return
	}
	if fd.HasOptionalKeyword() {
		// optional bytes: distinguish nil (unset) from []byte{} (set to empty).
		if fd.Kind() == protoreflect.BytesKind {
			fmt.Fprintf(fg.body, "\tif (this.%s == nil) != (that1.%s == nil) {\n\t\treturn false\n\t}\n", goName, goName)
			fmt.Fprintf(fg.body, "\tif !bytes.Equal(this.%s, that1.%s) {\n\t\treturn false\n\t}\n", goName, goName)
			return
		}
		// optional message: nil checks + deep Equal.
		if fd.Kind() == protoreflect.MessageKind {
			fmt.Fprintf(fg.body, "\tif (this.%s == nil) != (that1.%s == nil) {\n\t\treturn false\n\t}\n", goName, goName)
			fmt.Fprintf(fg.body, "\tif this.%s != nil && !this.%s.Equal(that1.%s) {\n\t\treturn false\n\t}\n", goName, goName, goName)
			return
		}
		fg.emitOptionalFieldEqual(fd, goName)
		return
	}
	if fd.IsMap() {
		fg.emitMapFieldEqual(fd, goName)
		return
	}

	// `(wiresmith.options.pointer) = true` singular message: same nil-check + deep
	// Equal as optional-message. Equal is nil-safe on the generated method, so
	// the `this.F != nil` guard could be elided — kept for symmetry with the
	// optional path and to make the "nil vs &Msg{}" distinction explicit.
	if fg.hasPointerOption(fd) && fd.Kind() == protoreflect.MessageKind {
		fmt.Fprintf(fg.body, "\tif (this.%s == nil) != (that1.%s == nil) {\n\t\treturn false\n\t}\n", goName, goName)
		fmt.Fprintf(fg.body, "\tif this.%s != nil && !this.%s.Equal(that1.%s) {\n\t\treturn false\n\t}\n", goName, goName, goName)
		return
	}

	switch fd.Kind() {
	case protoreflect.BytesKind:
		fmt.Fprintf(fg.body, "\tif !bytes.Equal(this.%s, that1.%s) {\n\t\treturn false\n\t}\n", goName, goName)
	case protoreflect.MessageKind:
		fmt.Fprintf(fg.body, "\tif !this.%s.Equal(that1.%s) {\n\t\treturn false\n\t}\n", goName, goName)
	default:
		fmt.Fprintf(fg.body, "\tif this.%s != that1.%s {\n\t\treturn false\n\t}\n", goName, goName)
	}
}

func (fg *FileGenerator) emitOptionalFieldEqual(_ protoreflect.FieldDescriptor, goName string) {
	fmt.Fprintf(fg.body, "\tif this.%s != that1.%s {\n", goName, goName)
	fmt.Fprintf(fg.body, "\t\tif this.%s == nil || that1.%s == nil {\n\t\t\treturn false\n\t\t}\n", goName, goName)
	fmt.Fprintf(fg.body, "\t\tif *this.%s != *that1.%s {\n\t\t\treturn false\n\t\t}\n", goName, goName)
	fmt.Fprintf(fg.body, "\t}\n")
}

func (fg *FileGenerator) emitMapFieldEqual(fd protoreflect.FieldDescriptor, goName string) {
	fmt.Fprintf(fg.body, "\tif len(this.%s) != len(that1.%s) {\n\t\treturn false\n\t}\n", goName, goName)
	fmt.Fprintf(fg.body, "\tfor k, v := range this.%s {\n", goName)
	fmt.Fprintf(fg.body, "\t\tv2, ok := that1.%s[k]\n", goName)
	fmt.Fprintf(fg.body, "\t\tif !ok {\n\t\t\treturn false\n\t\t}\n")

	valFd := fd.Message().Fields().ByNumber(2)
	switch valFd.Kind() {
	case protoreflect.MessageKind:
		fmt.Fprintf(fg.body, "\t\tif !v.Equal(v2) {\n\t\t\treturn false\n\t\t}\n")
	case protoreflect.BytesKind:
		fmt.Fprintf(fg.body, "\t\tif !bytes.Equal(v, v2) {\n\t\t\treturn false\n\t\t}\n")
	default:
		fmt.Fprintf(fg.body, "\t\tif v != v2 {\n\t\t\treturn false\n\t\t}\n")
	}
	fmt.Fprintf(fg.body, "\t}\n")
}

func (fg *FileGenerator) emitRepeatedFieldEqual(fd protoreflect.FieldDescriptor, goName string) {
	fmt.Fprintf(fg.body, "\tif len(this.%s) != len(that1.%s) {\n\t\treturn false\n\t}\n", goName, goName)
	fmt.Fprintf(fg.body, "\tfor i := range this.%s {\n", goName)

	switch fd.Kind() {
	case protoreflect.BytesKind:
		fmt.Fprintf(fg.body, "\t\tif !bytes.Equal(this.%s[i], that1.%s[i]) {\n\t\t\treturn false\n\t\t}\n", goName, goName)
	case protoreflect.MessageKind:
		if fg.hasPointerOption(fd) {
			// []*Msg can hold nil entries. The generated Equal is nil-safe on
			// the receiver, but a nil element on one side and non-nil on the
			// other must compare unequal — match the optional-message Equal
			// shape applied per-element.
			fmt.Fprintf(fg.body, "\t\tif (this.%s[i] == nil) != (that1.%s[i] == nil) {\n\t\t\treturn false\n\t\t}\n", goName, goName)
			fmt.Fprintf(fg.body, "\t\tif this.%s[i] != nil && !this.%s[i].Equal(that1.%s[i]) {\n\t\t\treturn false\n\t\t}\n", goName, goName, goName)
		} else {
			fmt.Fprintf(fg.body, "\t\tif !this.%s[i].Equal(that1.%s[i]) {\n\t\t\treturn false\n\t\t}\n", goName, goName)
		}
	default:
		fmt.Fprintf(fg.body, "\t\tif this.%s[i] != that1.%s[i] {\n\t\t\treturn false\n\t\t}\n", goName, goName)
	}

	fmt.Fprintf(fg.body, "\t}\n")
}

func needsBytesImportForEqual(md protoreflect.MessageDescriptor) bool {
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if fd.Kind() == protoreflect.BytesKind {
			return true
		}
		if fd.IsMap() {
			valFd := fd.Message().Fields().ByNumber(2)
			if valFd.Kind() == protoreflect.BytesKind {
				return true
			}
		}
	}
	return false
}
