package generator

import (
	"fmt"

	"github.com/bufbuild/protocompile/linker"
	"github.com/grafana/wiresmith/compiler/types"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// pointerExtensionName is the fully qualified name of the extension defined in
// the embedded wiresmith/options.proto. Kept here as the single source of
// truth — bump alongside the .proto file if the extension is ever renamed.
const pointerExtensionName = "wiresmith.options.pointer"

// pointerOption implements FieldOption for `(wiresmith.options.pointer)`.
// The extension descriptor is bound at Resolve time and consulted by Has,
// FieldType, and GoFieldType to swap singular message fields to `*T` and
// repeated message fields to `[]*T`.
type pointerOption struct {
	ext protoreflect.FieldDescriptor
}

func (*pointerOption) Name() string                               { return pointerExtensionName }
func (o *pointerOption) Resolve(ext protoreflect.FieldDescriptor) { o.ext = ext }

// Has reports whether fd is annotated with `[(wiresmith.options.pointer) = true]`.
// Safe to call on any field descriptor; returns false when the option is
// absent or when the extension descriptor has not been bound (e.g. unit
// tests that bypass Generate).
func (o *pointerOption) Has(fd protoreflect.FieldDescriptor) bool {
	return hasBoolOption(o.ext, fd)
}

// Validate rejects invalid placements of the pointer option.
//
// Allowed: singular or repeated message fields.
// Rejected: scalars/enums/bytes/strings, optional fields (redundant), oneof
// variants (already interface-boxed), map fields (out of scope in v1).
func (o *pointerOption) Validate(g *Generator, results linker.Files) error {
	if o.ext == nil {
		return nil
	}
	var errs []string
	for _, fd := range results {
		if isInternalSchemaFile(fd) {
			continue
		}
		walkFields(fd, func(field protoreflect.FieldDescriptor) {
			if !o.Has(field) {
				return
			}
			if reason := pointerOptionRejection(field); reason != "" {
				errs = append(errs, fmt.Sprintf("%s: field %q: %s", fd.Path(), field.FullName(), reason))
			}
		})
	}
	return combinedOptionError(pointerExtensionName, "placement", errs)
}

// FieldType returns the PointerField / RepeatedPointer wrapper for pointer-
// annotated singular and repeated message fields. Returns (nil, false) for
// fields without the option so the registry dispatch falls through to the
// next option (and then the default emit path).
//
// Map fields, oneof variants, optional fields, and non-message fields fall
// back too — Validate has rejected these placements already, but degrading
// gracefully matters for tests that construct descriptors directly.
func (o *pointerOption) FieldType(fg *FileGenerator, fd protoreflect.FieldDescriptor) (types.FieldType, bool) {
	if !o.Has(fd) {
		return nil, false
	}
	if fd.IsMap() || isRealOneof(fd) || fd.HasOptionalKeyword() || fd.Kind() != protoreflect.MessageKind {
		return nil, false
	}
	inner := types.Get(fd.Kind())
	if fd.IsList() {
		return &types.RepeatedPointer{Inner: inner}, true
	}
	return &types.PointerField{Inner: inner}, true
}

// GoFieldType returns the `*Msg` / `[]*Msg` Go-side type for pointer-
// annotated message fields. Same fall-through rules as FieldType.
func (o *pointerOption) GoFieldType(fg *FileGenerator, fd protoreflect.FieldDescriptor) (string, bool) {
	if !o.Has(fd) {
		return "", false
	}
	if fd.Kind() != protoreflect.MessageKind {
		return "", false
	}
	pointed := "*" + fg.imports.goSingularType(fd)
	if fd.IsList() {
		return "[]" + pointed, true
	}
	return pointed, true
}

// pointerOptionRejection returns a human-readable reason if the option is not
// allowed on this field, or "" if the placement is valid.
func pointerOptionRejection(fd protoreflect.FieldDescriptor) string {
	if fd.IsMap() {
		return "(wiresmith.options.pointer) is not supported on map fields"
	}
	if isRealOneof(fd) {
		return "(wiresmith.options.pointer) is not supported on oneof variants — variants are already interface-boxed"
	}
	if fd.HasOptionalKeyword() {
		return "(wiresmith.options.pointer) cannot combine with `optional` — `optional` already produces a pointer"
	}
	if fd.Kind() != protoreflect.MessageKind {
		return fmt.Sprintf("(wiresmith.options.pointer) only applies to message fields, got %s", fd.Kind())
	}
	return ""
}

// walkFields invokes fn for every field of every (non-map-entry) message in fd.
func walkFields(fd protoreflect.FileDescriptor, fn func(protoreflect.FieldDescriptor)) {
	forEachMessage(fd, func(md protoreflect.MessageDescriptor) {
		for i := 0; i < md.Fields().Len(); i++ {
			fn(md.Fields().Get(i))
		}
	})
}
