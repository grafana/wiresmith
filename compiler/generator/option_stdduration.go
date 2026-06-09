package generator

import (
	"fmt"

	"github.com/grafana/wiresmith/compiler/types"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// stddurationExtensionName is the fully qualified name of the stdduration
// extension defined in the embedded wiresmith/options.proto. Mirrors
// stdtimeExtensionName — kept here as the single source of truth so a
// rename only touches one constant.
const stddurationExtensionName = "wiresmith.options.stdduration"

// durationMessageFullName is the canonical proto full name of Google's
// well-known Duration message. stdduration is only allowed on fields whose
// message type matches this name; user-defined messages that happen to
// share a Go shape cannot opt in.
const durationMessageFullName = "google.protobuf.Duration"

// stddurationOption implements FieldOption for `(wiresmith.options.stdduration)`.
// When set on a singular google.protobuf.Duration field the Go-side field
// becomes a stdlib `time.Duration`; on-wire format is unaffected.
type stddurationOption struct {
	ext protoreflect.FieldDescriptor
}

func (*stddurationOption) Name() string                               { return stddurationExtensionName }
func (o *stddurationOption) Resolve(ext protoreflect.FieldDescriptor) { o.ext = ext }

// Has reports whether the field is annotated with `[(wiresmith.options.stdduration) = true]`.
func (o *stddurationOption) Has(fd protoreflect.FieldDescriptor) bool {
	return hasBoolOption(o.ext, fd)
}

// Validate rejects invalid placements of stdduration.
//
// v1 scope: only singular `google.protobuf.Duration` message fields.
// Map, oneof, repeated, proto3 `optional`, non-Duration messages, scalar
// kinds, and the combination with `(wiresmith.options.pointer) = true`
// are all rejected. Mirrors stdtime's validation list.
func (o *stddurationOption) Validate(g *Generator, results []protoreflect.FileDescriptor) error {
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
			if reason := stddurationOptionRejection(pointerOpt, field); reason != "" {
				errs = append(errs, fmt.Sprintf("%s: field %q: %s", fd.Path(), field.FullName(), reason))
			}
		})
	}
	return combinedOptionError(stddurationExtensionName, "placement", errs)
}

// FieldType returns a StdDurationType wrapper when the field is annotated
// AND is a singular google.protobuf.Duration. Registers the "time" import
// as a side effect — same idempotent shape as GoFieldType, since the
// struct-field declaration and the Size/Marshal/Unmarshal emitters need
// the same import set and either path may run first.
func (o *stddurationOption) FieldType(fg *FileGenerator, fd protoreflect.FieldDescriptor) (types.FieldType, bool) {
	if !o.applies(fd) {
		return nil, false
	}
	fg.imports.addImport("time", "")
	return &types.StdDurationType{}, true
}

// GoFieldType returns "time.Duration" for stdduration-annotated singular
// Duration fields. Validate has rejected every other placement, so the
// kind/shape guards in applies() are defensive against direct descriptor
// construction in tests.
func (o *stddurationOption) GoFieldType(fg *FileGenerator, fd protoreflect.FieldDescriptor) (string, bool) {
	if !o.applies(fd) {
		return "", false
	}
	fg.imports.addImport("time", "")
	return "time.Duration", true
}

// applies merges the "annotated" and "shape is acceptable" checks shared
// between FieldType and GoFieldType so a future shape rule lands in one
// place rather than two.
func (o *stddurationOption) applies(fd protoreflect.FieldDescriptor) bool {
	if !o.Has(fd) {
		return false
	}
	if fd.Kind() != protoreflect.MessageKind {
		return false
	}
	if string(fd.Message().FullName()) != durationMessageFullName {
		return false
	}
	if fd.IsMap() || fd.IsList() || fd.HasOptionalKeyword() || isRealOneof(fd) {
		return false
	}
	return true
}

// stddurationOptionRejection returns a human-readable reason if stdduration
// is not allowed on this field, or "" if the placement is valid. Mirrors
// stdtimeOptionRejection.
//
// pointerOpt is threaded through so the combo with
// (wiresmith.options.pointer) gets a dedicated, less surprising error
// message than "not a Duration" would.
func stddurationOptionRejection(pointerOpt *pointerOption, fd protoreflect.FieldDescriptor) string {
	if fd.IsMap() {
		return "(wiresmith.options.stdduration) is not supported on map fields"
	}
	if isRealOneof(fd) {
		return "(wiresmith.options.stdduration) is not supported on oneof variants"
	}
	if fd.IsList() {
		return "(wiresmith.options.stdduration) is not supported on repeated fields (v1 scope)"
	}
	if fd.HasOptionalKeyword() {
		return "(wiresmith.options.stdduration) is not supported on proto3 `optional` fields (v1 scope)"
	}
	if pointerOpt != nil && pointerOpt.Has(fd) {
		return "(wiresmith.options.stdduration) cannot combine with (wiresmith.options.pointer) — pick one"
	}
	if fd.Kind() != protoreflect.MessageKind {
		return fmt.Sprintf("(wiresmith.options.stdduration) only applies to %s fields, got %s", durationMessageFullName, fd.Kind())
	}
	if string(fd.Message().FullName()) != durationMessageFullName {
		return fmt.Sprintf("(wiresmith.options.stdduration) only applies to %s fields, got %s", durationMessageFullName, fd.Message().FullName())
	}
	return ""
}
