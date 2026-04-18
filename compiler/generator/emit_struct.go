package generator

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitStruct(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)

	// Emit message-level leading comment.
	if c := leadingComment(md); c != "" {
		fg.body.WriteString(c)
	}

	fmt.Fprintf(fg.body, "type %s struct {\n", name)

	seenOneofs := map[string]bool{}
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)

		if oo := fd.ContainingOneof(); oo != nil && !oo.IsSynthetic() {
			ooName := string(oo.Name())
			if !seenOneofs[ooName] {
				seenOneofs[ooName] = true
				if c := leadingComment(oo); c != "" {
					fg.body.WriteString(indentComment(c))
				}
				goName := snakeToPascal(ooName)
				ifaceName := fg.resolveOneofInterfaceName(md, oo)
				if fg.gen.GogoCompat {
					// Emit "Types that are valid to be assigned to" comment.
					fmt.Fprintf(fg.body, "\t// Types that are valid to be assigned to %s:\n", goName)
					fmt.Fprintf(fg.body, "\t//\n")
					for j := 0; j < oo.Fields().Len(); j++ {
						varName := oneofVariantName(md, oo.Fields().Get(j))
						fmt.Fprintf(fg.body, "\t//\t*%s\n", varName)
					}
					fmt.Fprintf(fg.body, "\t%s %s `protobuf_oneof:\"%s\"`\n", goName, ifaceName, ooName)
				} else {
					fmt.Fprintf(fg.body, "\t%s %s\n", goName, ifaceName)
				}
			}
			continue
		}

		// Emit field-level leading comment.
		if c := leadingComment(fd); c != "" {
			fg.body.WriteString(indentComment(c))
		}

		goName := snakeToPascal(string(fd.Name()))
		goType := fg.imports.goType(fd)
		if fg.gen.GogoCompat {
			tag := fg.protoTag(fd)
			fmt.Fprintf(fg.body, "\t%s %s `%s`\n", goName, goType, tag)
		} else {
			fmt.Fprintf(fg.body, "\t%s %s\n", goName, goType)
		}
	}

	fmt.Fprintf(fg.body, "}\n\n")
}

// indentComment adds a tab prefix to each line of a Go comment block.
func indentComment(comment string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(comment, "\n"), "\n") {
		b.WriteByte('\t')
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// protoTag generates a protobuf struct tag for a field, e.g.:
// protobuf:"bytes,1,opt,name=replica,proto3" json:"replica,omitempty"
func (fg *FileGenerator) protoTag(fd protoreflect.FieldDescriptor) string {
	var parts []string

	// Wire type name
	parts = append(parts, protoWireTypeName(fd))

	// Field number
	parts = append(parts, fmt.Sprintf("%d", fd.Number()))

	// Cardinality
	if fd.IsList() {
		parts = append(parts, "rep")
	} else if fd.HasOptionalKeyword() {
		parts = append(parts, "opt")
	} else {
		parts = append(parts, "opt")
	}

	// Field name
	parts = append(parts, "name="+string(fd.Name()))

	// JSON name (if different from proto name)
	if fd.JSONName() != string(fd.Name()) {
		parts = append(parts, "json="+fd.JSONName())
	}

	// proto3
	if fd.ParentFile().Syntax() == protoreflect.Proto3 {
		parts = append(parts, "proto3")
	}

	protoTag := `protobuf:"` + strings.Join(parts, ",") + `"`

	// JSON tag: check for gogoproto.jsontag override first.
	if jt := getJsonTag(fd); jt != "" {
		return protoTag + fmt.Sprintf(` json:"%s"`, jt)
	}
	// gogoslick uses the proto field name (snake_case), not the JSON name (camelCase).
	jsonName := string(fd.Name())
	// gogoslick omits omitempty for non-nullable message/bytes/stdduration/stdtime fields.
	if !isFieldNullable(fd) && (fd.Kind() == protoreflect.MessageKind || isStdDuration(fd) || isStdTime(fd)) {
		return protoTag + fmt.Sprintf(` json:"%s"`, jsonName)
	}
	if fd.IsList() && !isFieldNullable(fd) && fd.Kind() == protoreflect.BytesKind {
		return protoTag + fmt.Sprintf(` json:"%s"`, jsonName)
	}
	return protoTag + fmt.Sprintf(` json:"%s,omitempty"`, jsonName)
}

// protoWireTypeName returns the protobuf wire type name for struct tags.
func protoWireTypeName(fd protoreflect.FieldDescriptor) string {
	// Sint types use zigzag encoding.
	switch fd.Kind() {
	case protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return "zigzag32"
	case protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return "zigzag64"
	}
	wt := wireTypeForKind(fd.Kind())
	switch wt {
	case protowire.VarintType:
		return "varint"
	case protowire.Fixed32Type:
		return "fixed32"
	case protowire.Fixed64Type:
		return "fixed64"
	case protowire.BytesType:
		if fd.Kind() == protoreflect.MessageKind {
			return "bytes"
		}
		return "bytes"
	default:
		return "bytes"
	}
}

// emitAllGetters generates Get* accessor methods for all messages.
// When skipFieldGetters is true, only oneof getters are generated (matching
// gogoproto.goproto_getters_all = false behavior where field getters are skipped
// but oneof getters are still needed).
func (fg *FileGenerator) emitAllGetters(fd protoreflect.FileDescriptor, skipFieldGetters bool) {
	for i := 0; i < fd.Messages().Len(); i++ {
		fg.emitGetterMethods(fd.Messages().Get(i), skipFieldGetters)
	}
}

func (fg *FileGenerator) emitGetterMethods(md protoreflect.MessageDescriptor, skipFieldGetters bool) {
	for i := 0; i < md.Messages().Len(); i++ {
		fg.emitGetterMethods(md.Messages().Get(i), skipFieldGetters)
	}

	name := goMessageTypeName(md)

	// Always emit oneof interface getters (e.g., GetResult() returns the oneof interface).
	seenOneofs := map[string]bool{}
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if oo := fd.ContainingOneof(); oo != nil && !oo.IsSynthetic() {
			ooName := string(oo.Name())
			if !seenOneofs[ooName] {
				seenOneofs[ooName] = true
				goName := snakeToPascal(ooName)
				ifaceName := fg.resolveOneofInterfaceName(md, oo)
				fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", name, goName, ifaceName)
				fmt.Fprintf(fg.body, "\tif m != nil {\n\t\treturn m.%s\n\t}\n", goName)
				fmt.Fprintf(fg.body, "\treturn nil\n")
				fmt.Fprintf(fg.body, "}\n\n")
			}
		}
	}

	// Always emit oneof variant getters.
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if isRealOneof(fd) {
			fg.emitOneofGetter(md, fd)
		}
	}

	if skipFieldGetters {
		return
	}

	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)

		if isRealOneof(fd) {
			continue
		}

		goName := snakeToPascal(string(fd.Name()))
		goType := fg.imports.goType(fd)

		fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", name, goName, goType)
		fmt.Fprintf(fg.body, "\tif m != nil {\n")
		fmt.Fprintf(fg.body, "\t\treturn m.%s\n", goName)
		fmt.Fprintf(fg.body, "\t}\n")

		// Return zero value. For message types, use the typed zero struct literal.
		fmt.Fprintf(fg.body, "\treturn %s\n", fg.zeroValueFor(fd))
		fmt.Fprintf(fg.body, "}\n\n")
	}
}

func (fg *FileGenerator) emitOneofGetter(md protoreflect.MessageDescriptor, fd protoreflect.FieldDescriptor) {
	name := goMessageTypeName(md)
	oo := fd.ContainingOneof()
	ooFieldName := snakeToPascal(string(oo.Name()))
	goName := snakeToPascal(string(fd.Name()))
	goType := fg.imports.goSingularType(fd)
	variantName := oneofVariantName(md, fd)

	// In gogo compat mode, oneof message fields are pointers.
	returnType := goType
	zeroVal := zeroValueForKind(fd.Kind())
	if fg.gen != nil && fg.gen.GogoCompat && fd.Kind() == protoreflect.MessageKind {
		returnType = "*" + goType
		zeroVal = "nil"
	}

	fmt.Fprintf(fg.body, "func (m *%s) Get%s() %s {\n", name, goName, returnType)
	fmt.Fprintf(fg.body, "\tif x, ok := m.Get%s().(*%s); ok {\n", ooFieldName, variantName)
	fmt.Fprintf(fg.body, "\t\treturn x.%s\n", goName)
	fmt.Fprintf(fg.body, "\t}\n")
	fmt.Fprintf(fg.body, "\treturn %s\n", zeroVal)
	fmt.Fprintf(fg.body, "}\n\n")
}

// zeroValueFor returns the zero value string for a field, using the resolved
// Go type for message fields (e.g., "core.LabelMatcher{}" instead of "{}").
func (fg *FileGenerator) zeroValueFor(fd protoreflect.FieldDescriptor) string {
	if fd.IsList() {
		return "nil"
	}
	if fd.HasOptionalKeyword() {
		return "nil"
	}
	if isStdDuration(fd) {
		return "0"
	}
	if isStdTime(fd) {
		if isFieldNullable(fd) {
			return "nil"
		}
		fg.imports.addStdImport("time")
		return "time.Time{}"
	}
	if fd.Kind() == protoreflect.MessageKind {
		if isGogoPointerField(fg.gen, fd) || isGogoPointerCustomType(fg.gen, fd) {
			return "nil"
		}
		return fg.imports.goMessageType(fd.Message()) + "{}"
	}
	return zeroValueForKind(fd.Kind())
}

// zeroValue returns the zero value string for a field.
func zeroValue(fd protoreflect.FieldDescriptor) string {
	if fd.IsList() {
		return "nil"
	}
	if fd.HasOptionalKeyword() {
		return "nil"
	}
	return zeroValueForKind(fd.Kind())
}

func zeroValueForKind(kind protoreflect.Kind) string {
	switch kind {
	case protoreflect.BoolKind:
		return "false"
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return "0"
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return "0"
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return "0"
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return "0"
	case protoreflect.FloatKind:
		return "0"
	case protoreflect.DoubleKind:
		return "0"
	case protoreflect.StringKind:
		return `""`
	case protoreflect.BytesKind:
		return "nil"
	case protoreflect.EnumKind:
		return "0"
	case protoreflect.MessageKind:
		return "{}"
	default:
		return "nil"
	}
}
