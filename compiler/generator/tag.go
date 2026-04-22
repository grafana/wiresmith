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

// fieldTag returns the full backtick-enclosed struct tag for a field.
func fieldTag(fd protoreflect.FieldDescriptor) string {
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

	jsonTag := jsonName + ",omitempty"
	if is64BitKind(fd.Kind()) {
		jsonTag += ",string"
	}

	return fmt.Sprintf("`protobuf:%q json:%q`",
		strings.Join(parts, ","),
		jsonTag)
}

// is64BitKind returns true for proto kinds that map to Go int64/uint64,
// which the proto JSON spec encodes as strings.
func is64BitKind(k protoreflect.Kind) bool {
	switch k {
	case protoreflect.Int64Kind, protoreflect.Uint64Kind,
		protoreflect.Sint64Kind, protoreflect.Fixed64Kind,
		protoreflect.Sfixed64Kind:
		return true
	}
	return false
}

// mapFieldTag returns the struct tag for a map field, including protobuf_key and protobuf_val.
func mapFieldTag(fd protoreflect.FieldDescriptor) string {
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
		jsonName+",omitempty",
		keyTag,
		valTag)
}

// oneofInterfaceTag returns the struct tag for the oneof interface field in the parent message.
func oneofInterfaceTag(oo protoreflect.OneofDescriptor) string {
	return fmt.Sprintf("`protobuf_oneof:%q`", string(oo.Name()))
}
