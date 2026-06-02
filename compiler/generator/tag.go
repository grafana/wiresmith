package generator

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func computeTagBytes(num protowire.Number, typ protowire.Type) []byte {
	return protowire.AppendTag(nil, num, typ)
}

// wireTypeName returns the protobuf struct-tag wire type name for a Kind.
func wireTypeName(kind protoreflect.Kind) string {
	switch kind {
	case protoreflect.BoolKind, protoreflect.EnumKind,
		protoreflect.Int32Kind, protoreflect.Uint32Kind,
		protoreflect.Int64Kind, protoreflect.Uint64Kind:
		return "varint"
	case protoreflect.Sint32Kind:
		return "zigzag32"
	case protoreflect.Sint64Kind:
		return "zigzag64"
	case protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind, protoreflect.FloatKind:
		return "fixed32"
	case protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind, protoreflect.DoubleKind:
		return "fixed64"
	default: // String, Bytes, Message
		return "bytes"
	}
}

// jsonStructTag returns the value to emit inside `json:"..."` for fd. The
// default is the proto field name plus `,omitempty`; when
// `(wiresmith.options.jsontag)` is set, the supplied value is used verbatim
// (no `,omitempty` is appended — callers include it themselves if they want
// it, matching gogoproto.jsontag semantics).
func (fg *FileGenerator) jsonStructTag(fd protoreflect.FieldDescriptor) string {
	if v, ok := fg.jsontagOverride(fd); ok {
		return v
	}
	return string(fd.Name()) + ",omitempty"
}

// fieldTag returns the full backtick-enclosed struct tag for a field.
func (fg *FileGenerator) fieldTag(fd protoreflect.FieldDescriptor) string {
	parts := []string{
		wireTypeName(fd.Kind()),
		fmt.Sprintf("%d", fd.Number()),
	}

	if fd.IsList() || fd.IsMap() {
		parts = append(parts, "rep")
	} else {
		parts = append(parts, "opt")
	}

	if fd.IsPacked() {
		parts = append(parts, "packed")
	}

	parts = append(parts, "name="+string(fd.Name()))

	jsonName := fd.JSONName()
	if jsonName != string(fd.Name()) {
		parts = append(parts, "json="+jsonName)
	}

	parts = append(parts, "proto3")

	if fd.Kind() == protoreflect.EnumKind {
		parts = append(parts, "enum="+string(fd.Enum().FullName()))
	}

	if fd.ContainingOneof() != nil {
		parts = append(parts, "oneof")
	}

	return fmt.Sprintf("`protobuf:%q json:%q`",
		strings.Join(parts, ","),
		fg.jsonStructTag(fd))
}

// mapFieldTag returns the struct tag for a map field, including protobuf_key and protobuf_val.
func (fg *FileGenerator) mapFieldTag(fd protoreflect.FieldDescriptor) string {
	// Main protobuf tag (maps are length-delimited, rep)
	parts := []string{
		"bytes",
		fmt.Sprintf("%d", fd.Number()),
		"rep",
		"name=" + string(fd.Name()),
	}

	jsonName := fd.JSONName()
	if jsonName != string(fd.Name()) {
		parts = append(parts, "json="+jsonName)
	}

	parts = append(parts, "proto3")

	keyFd := fd.MapKey()
	valFd := fd.MapValue()

	keyTag := fmt.Sprintf("%s,1,opt,name=key", wireTypeName(keyFd.Kind()))

	valTag := fmt.Sprintf("%s,2,opt,name=value", wireTypeName(valFd.Kind()))
	if valFd.Kind() == protoreflect.EnumKind {
		valTag += ",enum=" + string(valFd.Enum().FullName())
	}

	return fmt.Sprintf("`protobuf:%q json:%q protobuf_key:%q protobuf_val:%q`",
		strings.Join(parts, ","),
		fg.jsonStructTag(fd),
		keyTag,
		valTag)
}

// oneofInterfaceTag returns the struct tag for the oneof interface field in the parent message.
func oneofInterfaceTag(oo protoreflect.OneofDescriptor) string {
	return fmt.Sprintf("`protobuf_oneof:%q`", string(oo.Name()))
}
