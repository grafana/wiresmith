package generator

import (
	"google.golang.org/protobuf/reflect/protoreflect"
)

// The no_presence option pair lives outside the FieldOption registry: it
// annotates messages and files, not fields, so the registry's
// FieldType/GoFieldType dispatch never applies. Resolution mirrors
// jsontagExt — two inline findExtension calls in generateFromFiles bind the
// descriptors, and hasNoPresence is the single consumer (called from
// fieldsForPresence, which every bitmap-emitting site already routes
// through).

// noPresenceExtName / noPresenceAllExtName are the fully-qualified names of
// the message-level and file-level extensions in the embedded options proto.
const (
	noPresenceExtName    = "wiresmith.options.no_presence"
	noPresenceAllExtName = "wiresmith.options.no_presence_all"
)

// hasNoPresence reports whether md opts out of the fieldsPresent bitmap.
// A per-message no_presence value (true or false) wins; otherwise the
// file-level no_presence_all default applies — the same override layering
// as gogoproto's *_all options. Nested messages are independent: each
// consults its own MessageOptions, not its parent's. The two scopes share
// readBoolExt (see option.go); the explicit-false-vs-unset distinction is
// what lets a message flip the file default in either direction.
func (fg *FileGenerator) hasNoPresence(md protoreflect.MessageDescriptor) bool {
	if v, ok := readBoolExt(fg.noPresenceExt, md.Options()); ok {
		return v
	}
	v, _ := readBoolExt(fg.noPresenceAllExt, md.ParentFile().Options())
	return v
}
