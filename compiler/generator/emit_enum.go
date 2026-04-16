package generator

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func (fg *FileGenerator) emitEnum(ed protoreflect.EnumDescriptor) {
	typeName := goEnumTypeName(ed)
	prefix := fg.enumValuePrefix(ed)

	if c := leadingComment(ed); c != "" {
		fg.body.WriteString(c)
	}

	fmt.Fprintf(fg.body, "type %s int32\n\n", typeName)
	fmt.Fprintf(fg.body, "const (\n")
	for i := 0; i < ed.Values().Len(); i++ {
		v := ed.Values().Get(i)
		if c := leadingComment(v); c != "" {
			fg.body.WriteString(indentComment(c))
		}
		valueName := prefix + string(v.Name())
		fmt.Fprintf(fg.body, "\t%s %s = %d\n", valueName, typeName, v.Number())
	}
	fmt.Fprintf(fg.body, ")\n\n")
}

// enumValuePrefix returns the prefix for enum value names.
// When goproto_enum_prefix is true (gogoproto extension 62001) and the enum is
// nested inside a message, the prefix is "ParentMessage_". Otherwise empty.
func (fg *FileGenerator) enumValuePrefix(ed protoreflect.EnumDescriptor) string {
	if !fg.gen.GogoCompat {
		return ""
	}

	if hasEnumPrefix(ed) {
		// Prefix with parent message name if enum is nested.
		if md, ok := ed.Parent().(protoreflect.MessageDescriptor); ok {
			return goMessageTypeName(md) + "_"
		}
	}
	return ""
}

// hasEnumPrefix checks if the enum has (gogoproto.goproto_enum_prefix) = true.
func hasEnumPrefix(ed protoreflect.EnumDescriptor) bool {
	opts, ok := ed.Options().(*descriptorpb.EnumOptions)
	if !ok || opts == nil {
		return false
	}
	b, err := proto.Marshal(opts)
	if err != nil {
		return false
	}
	// Extension 62001: goproto_enum_prefix (bool, varint)
	// Check if it's set to true (value 1).
	return containsVarintField(b, 62001, 1)
}
