package generator

import (
	"fmt"

	"github.com/bufbuild/protocompile/linker"
	"github.com/grafana/wiresmith/compiler/types"
	"google.golang.org/protobuf/reflect/protoreflect"
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

// stdtimeOption implements FieldOption for `(wiresmith.options.stdtime)`.
// When set on a singular google.protobuf.Timestamp field the Go-side field
// becomes a stdlib `time.Time`; on-wire format is unaffected.
type stdtimeOption struct {
	ext protoreflect.FieldDescriptor
}

func (*stdtimeOption) Name() string                               { return stdtimeExtensionName }
func (o *stdtimeOption) Resolve(ext protoreflect.FieldDescriptor) { o.ext = ext }

// Has reports whether the field is annotated with `[(wiresmith.options.stdtime) = true]`.
func (o *stdtimeOption) Has(fd protoreflect.FieldDescriptor) bool {
	return hasBoolOption(o.ext, fd)
}

// Validate rejects invalid placements of stdtime.
//
// v1 scope: only singular `google.protobuf.Timestamp` message fields.
// Map, oneof, repeated, proto3 `optional`, non-Timestamp messages, scalar
// kinds, and the combination with `(wiresmith.options.pointer) = true` are
// all rejected. The pointer combo gets a dedicated message rather than the
// fallback "not a Timestamp" — both options live on the same field but
// produce conflicting Go shapes (`*time.Time` vs `time.Time`), and
// silencing one would erase a user-meaningful intent.
func (o *stdtimeOption) Validate(g *Generator, results linker.Files) error {
	if o.ext == nil {
		return nil
	}
	pointerOpt := findOption[*pointerOption](g.options)
	var errs []string
	for _, fd := range results {
		if isInternalSchemaFile(fd) {
			continue
		}
		walkFields(fd, func(field protoreflect.FieldDescriptor) {
			if !o.Has(field) {
				return
			}
			if reason := stdtimeOptionRejection(pointerOpt, field); reason != "" {
				errs = append(errs, fmt.Sprintf("%s: field %q: %s", fd.Path(), field.FullName(), reason))
			}
		})
	}
	return combinedOptionError(stdtimeExtensionName, "placement", errs)
}

// FieldType returns a StdtimeType wrapper when the field is annotated AND
// is a singular google.protobuf.Timestamp. Registers the "time" import as
// a side effect — same idempotent shape as GoFieldType, since the struct-
// field declaration and the Size/Marshal/Unmarshal emitters need the same
// import set and either path may run first.
func (o *stdtimeOption) FieldType(fg *FileGenerator, fd protoreflect.FieldDescriptor) (types.FieldType, bool) {
	if !o.applies(fd) {
		return nil, false
	}
	fg.imports.addImport("time", "")
	return &types.StdtimeType{}, true
}

// GoFieldType returns "time.Time" for stdtime-annotated singular Timestamp
// fields. validateStdtime has rejected every other placement, so the
// kind/shape guards here are defensive against direct descriptor
// construction in tests.
func (o *stdtimeOption) GoFieldType(fg *FileGenerator, fd protoreflect.FieldDescriptor) (string, bool) {
	if !o.applies(fd) {
		return "", false
	}
	fg.imports.addImport("time", "")
	return "time.Time", true
}

// applies merges the "annotated" and "shape is acceptable" checks shared
// between FieldType and GoFieldType so a future shape rule lands in one
// place rather than two.
func (o *stdtimeOption) applies(fd protoreflect.FieldDescriptor) bool {
	if !o.Has(fd) {
		return false
	}
	if fd.Kind() != protoreflect.MessageKind {
		return false
	}
	if string(fd.Message().FullName()) != timestampMessageFullName {
		return false
	}
	if fd.IsMap() || fd.IsList() || fd.HasOptionalKeyword() || isRealOneof(fd) {
		return false
	}
	return true
}

// stdtimeOptionRejection returns a human-readable reason if stdtime is not
// allowed on this field, or "" if the placement is valid. Mirrors
// pointerOptionRejection / customtypeOptionRejection.
//
// pointerOpt is threaded through so the combo with
// (wiresmith.options.pointer) gets a dedicated, less surprising error
// message than "not a Timestamp" would.
func stdtimeOptionRejection(pointerOpt *pointerOption, fd protoreflect.FieldDescriptor) string {
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
	if pointerOpt != nil && pointerOpt.Has(fd) {
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
