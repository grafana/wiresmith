package generator

import (
	"google.golang.org/protobuf/reflect/protoreflect"
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
// no_presence / gogoproto's *_all options, and the same shared readBoolExt
// primitive (see option.go).
func (fg *FileGenerator) hasEnumNoPrefix(ed protoreflect.EnumDescriptor) bool {
	if v, ok := readBoolExt(fg.enumNoPrefixExt, ed.Options()); ok {
		return v
	}
	v, _ := readBoolExt(fg.enumNoPrefixAllExt, ed.ParentFile().Options())
	return v
}
