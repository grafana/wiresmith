package generator

import (
	"fmt"

	"github.com/grafana/wiresmith/compiler/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// FieldOption is one custom (wiresmith.options.*) extension and the
// generator behavior tied to it. The registry walks every registered
// FieldOption twice per Generate — first to bind each option's extension
// descriptor (Resolve), then to reject malformed placements (Validate) —
// and during field emission asks each option in turn whether it overrides
// the Go-side type expression (GoFieldType) or the FieldType composite
// (FieldType) for the field at hand.
//
// Implementations live in option_<name>.go alongside the option-specific
// validation and value-decode helpers. Adding a new option means defining
// one new file and registering one new instance in newOptionRegistry —
// the dispatch sites (fieldType, goFieldType, Generate's resolve+validate
// loops) don't change.
type FieldOption interface {
	// Name returns the fully-qualified extension name, e.g.
	// "wiresmith.options.pointer". Used to locate the extension
	// descriptor in the embedded options proto during Resolve.
	Name() string

	// Resolve binds the linked extension descriptor for this option.
	// Called once per Generate after Compile. `ext` is nil when
	// wiresmith/options.proto is not part of the compiled set — the common
	// case in plugin mode, where the consumer's protoc/buf invocation
	// hasn't been given the wiresmith schema. Implementations must store
	// the value (including nil) and short-circuit subsequent Has* / Value*
	// checks on a nil descriptor.
	Resolve(ext protoreflect.FieldDescriptor)

	// Validate walks `results` and rejects invalid placements. Each
	// implementation returns a single combined error (one entry per
	// offending field) prefixed by the extension name, matching the
	// pre-registry per-option validation shape.
	Validate(g *Generator, results []protoreflect.FileDescriptor) error

	// FieldType returns the types.FieldType override for `fd` if this
	// option applies, else (nil, false). The dispatch loop in
	// FileGenerator.fieldType takes the first option whose FieldType
	// returns ok — order in newOptionRegistry is therefore load-bearing
	// (stdtime / customtype must run before pointer, which doesn't have
	// a FieldType override and reaches the fallback path inline).
	FieldType(fg *FileGenerator, fd protoreflect.FieldDescriptor) (types.FieldType, bool)

	// GoFieldType returns the Go type expression for the struct-field
	// declaration if this option applies, else ("", false). Same
	// short-circuit semantics as FieldType.
	GoFieldType(fg *FileGenerator, fd protoreflect.FieldDescriptor) (string, bool)

	// Has reports whether the option is set on fd. Used by peer options
	// to enforce cross-option compatibility whitelists without each
	// option having to know the others' representations (bool vs string
	// extension, etc.).
	Has(fd protoreflect.FieldDescriptor) bool
}

// newOptionRegistry returns the default registry of field options. Order
// is load-bearing: FieldType / GoFieldType dispatch takes the first
// match, so the more-specific overrides (stdtime, customtype) precede
// the pointer pass that delegates to the default emit shape.
func newOptionRegistry() []FieldOption {
	return []FieldOption{
		&stdtimeOption{},
		&customtypeOption{},
		&pointerOption{},
		&customnameOption{},
	}
}

// findExtension returns the linked extension descriptor whose full name
// matches name. The lookup matches the embedded options file by canonical
// import path so a user file declaring the same proto package cannot
// shadow it.
func findExtension(results []protoreflect.FileDescriptor, name string) protoreflect.FieldDescriptor {
	for _, fd := range results {
		if fd.Path() != embeddedOptionsPath {
			continue
		}
		exts := fd.Extensions()
		for i := 0; i < exts.Len(); i++ {
			x := exts.Get(i)
			if string(x.FullName()) == name {
				return x
			}
		}
	}
	return nil
}

// hasBoolOption reports whether fd carries the bool-valued extension
// described by ext set to true. Used by the singleton-per-option
// boilerplate (pointer, stdtime) which all share the same shape: build a
// dynamic ExtensionType from the linked descriptor, check presence, read
// the bool. The ExtensionTypeDescriptor branch reuses the linker's own
// type when available so the registered type identity matches what other
// proto reflection helpers see.
func hasBoolOption(ext, fd protoreflect.FieldDescriptor) bool {
	if ext == nil {
		return false
	}
	opts, ok := fd.Options().(*descriptorpb.FieldOptions)
	if !ok || opts == nil {
		return false
	}
	xt := extensionType(ext)
	if !proto.HasExtension(opts, xt) {
		return false
	}
	v, _ := proto.GetExtension(opts, xt).(bool)
	return v
}

// stringOption returns the string-valued extension on fd plus a presence
// flag. Empty string with ok=true is a legal explicit value (jsontag uses
// "" verbatim to suppress the default json tag); callers must rely on the
// boolean.
func stringOption(ext, fd protoreflect.FieldDescriptor) (string, bool) {
	if ext == nil {
		return "", false
	}
	opts, ok := fd.Options().(*descriptorpb.FieldOptions)
	if !ok || opts == nil {
		return "", false
	}
	xt := extensionType(ext)
	if !proto.HasExtension(opts, xt) {
		return "", false
	}
	v, _ := proto.GetExtension(opts, xt).(string)
	return v, true
}

// extensionType wraps the linked descriptor in an ExtensionType so the
// proto.HasExtension / proto.GetExtension APIs accept it. Cast through
// ExtensionTypeDescriptor first — the linker returns descriptors that
// already satisfy that subinterface, and reusing the existing type
// preserves the registered identity. Fall back to dynamicpb for any
// descriptor (e.g. a hand-constructed one in tests) that doesn't.
func extensionType(ext protoreflect.FieldDescriptor) protoreflect.ExtensionType {
	if xd, ok := ext.(protoreflect.ExtensionTypeDescriptor); ok {
		return xd.Type()
	}
	return dynamicpb.NewExtensionType(ext)
}

// combinedOptionError joins per-field validation errors into one diagnostic
// with one entry per offending field. category is the noun that
// distinguishes "the option is in a place it doesn't belong" (placement)
// from "the option's string payload is malformed" (value); customname
// and jsontag use "value" because their failures are about the payload,
// the rest use "placement".
func combinedOptionError(extensionName, category string, errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	out := fmt.Sprintf("invalid (%s) %s:\n", extensionName, category)
	for _, e := range errs {
		out += "  - " + e + "\n"
	}
	return fmt.Errorf("%s", out)
}

// findOption returns the registered option that matches the concrete type
// T, or zero-value if no such option is registered. Used for the one
// cross-option dependency in the registry — stdtime's Validate needs to
// ask the pointer option whether a given field also has the pointer
// annotation, so that combining the two surfaces a clearer error than
// "stdtime only applies to Timestamp" would.
func findOption[T FieldOption](options []FieldOption) T {
	for _, o := range options {
		if t, ok := o.(T); ok {
			return t
		}
	}
	var zero T
	return zero
}

// fieldType dispatches FieldType resolution across the option registry.
// The first option whose FieldType returns ok wins; otherwise we fall
// through to the default emit path (maps, then types.ForField). Order in
// newOptionRegistry is therefore load-bearing.
func (fg *FileGenerator) fieldType(fd protoreflect.FieldDescriptor) types.FieldType {
	if fd.IsMap() {
		// MapField needs the Go-side key/value type names for emitters that
		// build typed locals (e.g. Compare's sorted-key slices). ForField
		// returns one with only Key/Val populated, which is enough for size
		// and the Equal len-check but not for Compare. Populating it here
		// keeps emit_compare from needing its own map-construction path.
		return &types.MapField{
			Key:       types.Get(fd.MapKey().Kind()),
			Val:       types.Get(fd.MapValue().Kind()),
			MapType:   fg.imports.goType(fd),
			KeyGoType: fg.imports.goSingularType(fd.MapKey()),
			ValGoType: fg.imports.goSingularType(fd.MapValue()),
			KeyCtx:    fg.fieldContext(fd.MapKey()),
			ValCtx:    fg.fieldContext(fd.MapValue()),
		}
	}
	for _, opt := range fg.options {
		if ft, ok := opt.FieldType(fg, fd); ok {
			return ft
		}
	}
	return types.ForField(fd)
}

// goFieldType dispatches GoFieldType resolution across the option registry.
// The first option whose GoFieldType returns ok wins; otherwise we fall
// back to ImportTracker.goType (which handles maps, repeated, optional,
// and singular types).
func (fg *FileGenerator) goFieldType(fd protoreflect.FieldDescriptor) string {
	for _, opt := range fg.options {
		if goType, ok := opt.GoFieldType(fg, fd); ok {
			return goType
		}
	}
	return fg.imports.goType(fd)
}

// suppressMessageType reports whether any registered option replaces the
// Go-side type for fd with something that isn't a wiresmith-generated
// message struct. fieldContext consults this to skip populating
// ctx.MessageType — emitters that would otherwise lookup the message's
// import alias don't reach into a string-typed `time.Time` or a customtype
// the same way.
//
// Currently only stdtime returns true; customtype substitutes for a
// scalar (bytes / string), so the MessageType branch never reaches it.
func (fg *FileGenerator) suppressMessageType(fd protoreflect.FieldDescriptor) bool {
	if opt := findOption[*stdtimeOption](fg.options); opt != nil {
		return opt.Has(fd)
	}
	return false
}

// goFieldName returns the identifier to use for the Go field, accessor
// methods (Get<Name>, Has<Name>), oneof wrapper-struct field, and equality
// comparisons. Falls back to the snake-to-PascalCase default when the
// field has no customname annotation.
func (fg *FileGenerator) goFieldName(fd protoreflect.FieldDescriptor) string {
	if opt := findOption[*customnameOption](fg.options); opt != nil {
		if v, ok := opt.Value(fd); ok {
			return v
		}
	}
	return snakeToPascal(string(fd.Name()))
}

// hasPointerOption reports whether the field is annotated with
// `(wiresmith.options.pointer) = true`. Thin wrapper over the registered
// pointerOption; kept on FileGenerator so the emit_*.go call sites read
// naturally.
func (fg *FileGenerator) hasPointerOption(fd protoreflect.FieldDescriptor) bool {
	opt := findOption[*pointerOption](fg.options)
	if opt == nil {
		return false
	}
	return opt.Has(fd)
}

// hasStdtimeOption reports whether the field is annotated with
// `(wiresmith.options.stdtime) = true`. Thin wrapper over the registered
// stdtimeOption.
func (fg *FileGenerator) hasStdtimeOption(fd protoreflect.FieldDescriptor) bool {
	opt := findOption[*stdtimeOption](fg.options)
	if opt == nil {
		return false
	}
	return opt.Has(fd)
}

// stdtimeGoFieldType returns the Go-side struct-field type for an stdtime-
// annotated field. Thin pass-through to the registered stdtimeOption; the
// emit_getter / emit_struct call sites read more naturally as
// `fg.stdtimeGoFieldType(fd)` than as a generic registry lookup.
func (fg *FileGenerator) stdtimeGoFieldType(fd protoreflect.FieldDescriptor) (string, bool) {
	opt := findOption[*stdtimeOption](fg.options)
	if opt == nil {
		return "", false
	}
	return opt.GoFieldType(fg, fd)
}

// customtypeGoFieldType returns the Go-side struct-field type for a
// customtype-annotated field. Same pass-through shape as
// stdtimeGoFieldType — keeps the emit-site call sites readable.
func (fg *FileGenerator) customtypeGoFieldType(fd protoreflect.FieldDescriptor) (string, bool) {
	opt := findOption[*customtypeOption](fg.options)
	if opt == nil {
		return "", false
	}
	return opt.GoFieldType(fg, fd)
}

// stdtimeFieldType returns the FieldType wrapper for an stdtime-annotated
// field. Used by emit_unmarshal to type-assert the singular branch.
func (fg *FileGenerator) stdtimeFieldType(fd protoreflect.FieldDescriptor) (types.FieldType, bool) {
	opt := findOption[*stdtimeOption](fg.options)
	if opt == nil {
		return nil, false
	}
	return opt.FieldType(fg, fd)
}

// customtypeFieldType returns the FieldType wrapper for a customtype-
// annotated field. Used by emit_unmarshal to take the customtype branch
// for singular bytes / string fields.
func (fg *FileGenerator) customtypeFieldType(fd protoreflect.FieldDescriptor) (types.FieldType, bool) {
	opt := findOption[*customtypeOption](fg.options)
	if opt == nil {
		return nil, false
	}
	return opt.FieldType(fg, fd)
}
