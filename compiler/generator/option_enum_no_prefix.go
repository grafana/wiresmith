package generator

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// The enum_no_prefix option pair sits outside the FieldOption registry for
// the same reason as no_presence: it annotates enums and files, not fields.
// hasEnumNoPrefix is the single consumer, called from emitEnum where the
// constant identifiers are built.

const (
	enumNoPrefixExtName    = "wiresmith.options.enum_no_prefix"
	enumNoPrefixAllExtName = "wiresmith.options.enum_no_prefix_all"
)

// hasEnumNoPrefix reports whether ed's value constants drop the
// `EnumName_` prefix. A per-enum value (true or false) wins; otherwise the
// file-level enum_no_prefix_all default applies — same layering as
// no_presence / gogoproto's *_all options.
func (fg *FileGenerator) hasEnumNoPrefix(ed protoreflect.EnumDescriptor) bool {
	if v, ok := boolEnumOption(fg.enumNoPrefixExt, ed); ok {
		return v
	}
	v, _ := boolFileOption(fg.enumNoPrefixAllExt, ed.ParentFile())
	return v
}

// boolEnumOption is boolMessageOption for EnumOptions.
func boolEnumOption(ext protoreflect.FieldDescriptor, ed protoreflect.EnumDescriptor) (value, ok bool) {
	if ext == nil {
		return false, false
	}
	opts, k := ed.Options().(*descriptorpb.EnumOptions)
	if !k || opts == nil {
		return false, false
	}
	xt := extensionType(ext)
	if !proto.HasExtension(opts, xt) {
		return false, false
	}
	v, _ := proto.GetExtension(opts, xt).(bool)
	return v, true
}
