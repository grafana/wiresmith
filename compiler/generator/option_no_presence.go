package generator

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
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
// consults its own MessageOptions, not its parent's.
func (fg *FileGenerator) hasNoPresence(md protoreflect.MessageDescriptor) bool {
	if v, ok := boolMessageOption(fg.noPresenceExt, md); ok {
		return v
	}
	v, _ := boolFileOption(fg.noPresenceAllExt, md.ParentFile())
	return v
}

// boolMessageOption returns the bool-valued extension on md's
// MessageOptions plus a presence flag. Same shape as hasBoolOption, but an
// explicit `false` is distinguishable from "unset" so a message can
// override a file-level default in either direction.
func boolMessageOption(ext protoreflect.FieldDescriptor, md protoreflect.MessageDescriptor) (value, ok bool) {
	if ext == nil {
		return false, false
	}
	opts, k := md.Options().(*descriptorpb.MessageOptions)
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

// boolFileOption is boolMessageOption for FileOptions.
func boolFileOption(ext protoreflect.FieldDescriptor, fd protoreflect.FileDescriptor) (value, ok bool) {
	if ext == nil {
		return false, false
	}
	opts, k := fd.Options().(*descriptorpb.FileOptions)
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
