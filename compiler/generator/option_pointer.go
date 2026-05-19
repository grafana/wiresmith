package generator

import (
	"fmt"
	"wiresmith/compiler/types"

	"github.com/bufbuild/protocompile/linker"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// pointerExtensionName is the fully qualified name of the extension defined in
// the embedded wiresmith/options.proto. Kept here as the single source of
// truth — bump alongside the .proto file if the extension is ever renamed.
const pointerExtensionName = "wiresmith.options.pointer"

// resolvePointerExtension finds the `(wiresmith.pointer)` extension descriptor
// among the linked files and caches it on the Generator. Always succeeds when
// the embedded options proto is in the input set; returns an error otherwise
// so callers see a clear failure instead of silently dropping the option.
func (g *Generator) resolvePointerExtension(results linker.Files) error {
	for _, fd := range results {
		if string(fd.Package()) != embeddedOptionsPackage {
			continue
		}
		exts := fd.Extensions()
		for i := 0; i < exts.Len(); i++ {
			x := exts.Get(i)
			if string(x.FullName()) == pointerExtensionName {
				g.pointerExt = x
				return nil
			}
		}
	}
	return fmt.Errorf("internal error: extension %q not found in compiled results — wiresmith/options.proto missing or malformed", pointerExtensionName)
}

// hasPointerOption reports whether the field is annotated with
// `[(wiresmith.pointer) = true]`. Safe to call on any field descriptor; returns
// false when the option is absent, when the FieldOptions are nil, or when the
// extension descriptor has not been resolved (e.g. unit tests that bypass
// Generate).
func (fg *FileGenerator) hasPointerOption(fd protoreflect.FieldDescriptor) bool {
	return hasPointerOption(fg.pointerExt, fd)
}

func hasPointerOption(ext protoreflect.FieldDescriptor, fd protoreflect.FieldDescriptor) bool {
	if ext == nil {
		return false
	}
	opts, ok := fd.Options().(*descriptorpb.FieldOptions)
	if !ok || opts == nil {
		return false
	}
	// Build a dynamic ExtensionType from the linked descriptor so we can use
	// the standard proto.GetExtension API without depending on a generated Go
	// extension. The cast to protoreflect.ExtensionDescriptor is safe — the
	// linker always returns extensions that satisfy that subinterface.
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

// validatePointerOptions walks every field in every result file and rejects
// invalid placements of `(wiresmith.pointer)`. Run after resolvePointerExtension
// and before any emit pass so problems surface as a single clear error rather
// than as garbled generated code.
//
// Allowed: singular or repeated message fields.
// Rejected: scalars/enums/bytes/strings, optional fields (redundant), oneof
// variants (already interface-boxed), map fields (out of scope in v1).
func (g *Generator) validatePointerOptions(results linker.Files) error {
	if g.pointerExt == nil {
		// resolvePointerExtension would have returned earlier; defensive only.
		return nil
	}
	var errs []string
	for _, fd := range results {
		if isInternalSchemaFile(fd) {
			continue
		}
		walkFields(fd, func(field protoreflect.FieldDescriptor) {
			if !hasPointerOption(g.pointerExt, field) {
				return
			}
			if reason := pointerOptionRejection(field); reason != "" {
				errs = append(errs, fmt.Sprintf("%s: field %q: %s", fd.Path(), field.FullName(), reason))
			}
		})
	}
	if len(errs) == 0 {
		return nil
	}
	// One combined error keeps the message useful when several offending
	// fields appear in the same input.
	out := "invalid (wiresmith.pointer) placement:\n"
	for _, e := range errs {
		out += "  - " + e + "\n"
	}
	return fmt.Errorf("%s", out)
}

// pointerOptionRejection returns a human-readable reason if the option is not
// allowed on this field, or "" if the placement is valid.
func pointerOptionRejection(fd protoreflect.FieldDescriptor) string {
	if fd.IsMap() {
		return "(wiresmith.pointer) is not supported on map fields"
	}
	if isRealOneof(fd) {
		return "(wiresmith.pointer) is not supported on oneof variants — variants are already interface-boxed"
	}
	if fd.HasOptionalKeyword() {
		return "(wiresmith.pointer) cannot combine with `optional` — `optional` already produces a pointer"
	}
	if fd.Kind() != protoreflect.MessageKind {
		return fmt.Sprintf("(wiresmith.pointer) only applies to message fields, got %s", fd.Kind())
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

// goFieldType returns the Go type string for the struct field declaration,
// applying the pointer prefix when `(wiresmith.pointer) = true`. Returns
// `*Msg` for singular pointer-message and `[]*Msg` for repeated; delegates to
// ImportTracker.goType for all other shapes.
//
// The struct-field type and the FieldType composite are intentionally chosen
// in two separate helpers: goFieldType yields the surface Go type, fieldType
// yields the emit behavior. They both gate on the same hasPointerOption
// predicate so they stay consistent.
func (fg *FileGenerator) goFieldType(fd protoreflect.FieldDescriptor) string {
	if !fg.hasPointerOption(fd) || fd.Kind() != protoreflect.MessageKind {
		return fg.imports.goType(fd)
	}
	pointed := "*" + fg.imports.goSingularType(fd)
	if fd.IsList() {
		return "[]" + pointed
	}
	return pointed
}

// fieldType returns the FieldType composite for a field, with one twist over
// types.ForField: when `(wiresmith.pointer) = true`, singular message fields
// route through PointerField and repeated message fields through
// RepeatedPointer. All other paths delegate unchanged.
//
// This is the single dispatch point used by emit_marshal, emit_size, and the
// unmarshal pointer branch — keeping the pointer option visible in exactly one
// place so future option-driven shape changes have a clear home.
func (fg *FileGenerator) fieldType(fd protoreflect.FieldDescriptor) types.FieldType {
	if !fg.hasPointerOption(fd) {
		return types.ForField(fd)
	}
	if fd.IsMap() || isRealOneof(fd) || fd.HasOptionalKeyword() || fd.Kind() != protoreflect.MessageKind {
		// Validation should have rejected these; fall back so the generator
		// degrades gracefully if validation is ever bypassed (e.g. from a
		// future test that constructs descriptors directly).
		return types.ForField(fd)
	}
	inner := types.Get(fd.Kind())
	if fd.IsList() {
		return &types.RepeatedPointer{Inner: inner}
	}
	return &types.PointerField{Inner: inner}
}
