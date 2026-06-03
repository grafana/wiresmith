package generator

import (
	"fmt"

	"github.com/bufbuild/protocompile/linker"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// stdtimeExtensionName is the fully qualified name of the stdtime extension
// defined in the embedded wiresmith/options.proto. Mirrors
// pointerExtensionName / customtypeExtensionName — kept here as the single
// source of truth so a rename only touches one constant.
const stdtimeExtensionName = "wiresmith.options.stdtime"

// timestampMessageFullName is the canonical proto full name of Google's
// well-known Timestamp message. stdtime is only allowed on fields whose
// message type matches this name; user-defined messages that happen to share
// a Go shape cannot opt in.
const timestampMessageFullName = "google.protobuf.Timestamp"

// resolveStdtimeExtension caches the linked stdtime extension descriptor on
// the Generator. Same wiring as resolvePointerExtension; runs once per
// Generate after Compile completes.
func (g *Generator) resolveStdtimeExtension(results linker.Files) error {
	for _, fd := range results {
		if fd.Path() != embeddedOptionsPath {
			continue
		}
		exts := fd.Extensions()
		for i := 0; i < exts.Len(); i++ {
			x := exts.Get(i)
			if string(x.FullName()) == stdtimeExtensionName {
				g.stdtimeExt = x
				return nil
			}
		}
	}
	return fmt.Errorf("internal error: extension %q not found in compiled results — wiresmith/options.proto missing or malformed", stdtimeExtensionName)
}

// hasStdtimeOption reports whether the field is annotated with
// `[(wiresmith.options.stdtime) = true]`. Safe to call on any field; returns
// false when the option is absent, when the FieldOptions are nil, or when
// the extension descriptor has not been resolved.
func (fg *FileGenerator) hasStdtimeOption(fd protoreflect.FieldDescriptor) bool {
	return hasStdtimeOption(fg.stdtimeExt, fd)
}

func hasStdtimeOption(ext protoreflect.FieldDescriptor, fd protoreflect.FieldDescriptor) bool {
	if ext == nil {
		return false
	}
	opts, ok := fd.Options().(*descriptorpb.FieldOptions)
	if !ok || opts == nil {
		return false
	}
	xd, ok := ext.(protoreflect.ExtensionTypeDescriptor)
	var xt protoreflect.ExtensionType
	if ok {
		xt = xd.Type()
	} else {
		xt = dynamicpb.NewExtensionType(ext)
	}
	if !proto.HasExtension(opts, xt) {
		return false
	}
	v, _ := proto.GetExtension(opts, xt).(bool)
	return v
}

// validateStdtimeOptions walks every field in every result file and rejects
// invalid placements of `(wiresmith.options.stdtime)`. Runs after
// resolveStdtimeExtension and before any emit pass.
//
// v1 scope: only singular `google.protobuf.Timestamp` message fields.
// Map, oneof, repeated, proto3 `optional`, non-Timestamp messages, scalar
// kinds, and the combination with `(wiresmith.options.pointer) = true` are
// all rejected. Same combined-error shape as validatePointerOptions so one
// bad fixture lists every offending field.
func (g *Generator) validateStdtimeOptions(results linker.Files) error {
	if g.stdtimeExt == nil {
		return nil
	}
	var errs []string
	for _, fd := range results {
		if isInternalSchemaFile(fd) {
			continue
		}
		walkFields(fd, func(field protoreflect.FieldDescriptor) {
			if !hasStdtimeOption(g.stdtimeExt, field) {
				return
			}
			if reason := stdtimeOptionRejection(g.pointerExt, field); reason != "" {
				errs = append(errs, fmt.Sprintf("%s: field %q: %s", fd.Path(), field.FullName(), reason))
			}
		})
	}
	if len(errs) == 0 {
		return nil
	}
	out := "invalid (wiresmith.options.stdtime) placement:\n"
	for _, e := range errs {
		out += "  - " + e + "\n"
	}
	return fmt.Errorf("%s", out)
}

// stdtimeOptionRejection returns a human-readable reason if stdtime is not
// allowed on this field, or "" if the placement is valid. Mirrors
// pointerOptionRejection / customtypeOptionRejection.
//
// The pointer-option extension is threaded through so the combo with
// (wiresmith.options.pointer) gets a dedicated, less surprising error message
// than "not a Timestamp" — both options live on the same field but produce
// conflicting Go shapes (`*time.Time` vs `time.Time`), and silencing one
// would erase a user-meaningful intent.
func stdtimeOptionRejection(pointerExt protoreflect.FieldDescriptor, fd protoreflect.FieldDescriptor) string {
	if fd.IsMap() {
		return "(wiresmith.options.stdtime) is not supported on map fields"
	}
	if isRealOneof(fd) {
		return "(wiresmith.options.stdtime) is not supported on oneof variants"
	}
	if fd.IsList() {
		return "(wiresmith.options.stdtime) is not supported on repeated fields (v1 scope)"
	}
	if fd.HasOptionalKeyword() {
		return "(wiresmith.options.stdtime) is not supported on proto3 `optional` fields (v1 scope)"
	}
	if hasPointerOption(pointerExt, fd) {
		return "(wiresmith.options.stdtime) cannot combine with (wiresmith.options.pointer) — pick one"
	}
	if fd.Kind() != protoreflect.MessageKind {
		return fmt.Sprintf("(wiresmith.options.stdtime) only applies to %s fields, got %s", timestampMessageFullName, fd.Kind())
	}
	if string(fd.Message().FullName()) != timestampMessageFullName {
		return fmt.Sprintf("(wiresmith.options.stdtime) only applies to %s fields, got %s", timestampMessageFullName, fd.Message().FullName())
	}
	return ""
}
