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

// resolvePointerExtension finds the `(wiresmith.options.pointer)` extension
// descriptor among the linked files and caches it on the Generator. Always
// succeeds when the embedded options proto is in the input set; returns an
// error otherwise so callers see a clear failure instead of silently dropping
// the option. The lookup matches the embedded file by canonical import path so
// a user file declaring the same proto package can never shadow it.
func (g *Generator) resolvePointerExtension(results linker.Files) error {
	for _, fd := range results {
		if fd.Path() != embeddedOptionsPath {
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
// `[(wiresmith.options.pointer) = true]`. Safe to call on any field descriptor;
// returns false when the option is absent, when the FieldOptions are nil, or
// when the extension descriptor has not been resolved (e.g. unit tests that
// bypass Generate).
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
// invalid placements of `(wiresmith.options.pointer)`. Run after
// resolvePointerExtension and before any emit pass so problems surface as a
// single clear error rather than as garbled generated code.
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
	out := "invalid (wiresmith.options.pointer) placement:\n"
	for _, e := range errs {
		out += "  - " + e + "\n"
	}
	return fmt.Errorf("%s", out)
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

// goFieldType returns the Go type string for the struct field declaration,
// applying the pointer prefix when `(wiresmith.options.pointer) = true`.
// Returns `*Msg` for singular pointer-message and `[]*Msg` for repeated;
// delegates to ImportTracker.goType for all other shapes.
//
// The struct-field type and the FieldType composite are intentionally chosen
// in two separate helpers: goFieldType yields the surface Go type, fieldType
// yields the emit behavior. They both gate on the same hasPointerOption
// predicate so they stay consistent.
func (fg *FileGenerator) goFieldType(fd protoreflect.FieldDescriptor) string {
	if goType, ok := fg.customtypeGoFieldType(fd); ok {
		return goType
	}
	if !fg.hasPointerOption(fd) || fd.Kind() != protoreflect.MessageKind {
		return fg.imports.goType(fd)
	}
	pointed := "*" + fg.imports.goSingularType(fd)
	if fd.IsList() {
		return "[]" + pointed
	}
	return pointed
}

// customtypeGoFieldType resolves `(wiresmith.options.customtype)` to the Go
// type expression for the struct-field declaration and registers the
// supporting import. Returns ok=false when the option is absent or invalid;
// validateCustomtypeOptions has already rejected malformed values at this
// point so the parse-error path is purely defensive.
func (fg *FileGenerator) customtypeGoFieldType(fd protoreflect.FieldDescriptor) (string, bool) {
	v, ok := fg.customtypeValue(fd)
	if !ok {
		return "", false
	}
	importPath, typeName, err := parseCustomtypeValue(v)
	if err != nil {
		return "", false
	}
	if importPath == "" {
		return typeName, true
	}
	alias := fg.customtypeAlias(importPath)
	return alias + "." + typeName, true
}

// customtypeAlias registers importPath with the ImportTracker under a
// collision-free, explicitly-spelled alias and returns it for use as the Go
// qualifier in generated code. The explicit-alias form is required because
// the option value gives us only the import path — not the package's `package`
// declaration — so we cannot rely on `path.Base` matching the identifier Go
// would bind to an unaliased import (it doesn't for module major-version
// paths like `.../foo/v2`, or for packages whose directory name differs from
// their declared name). The lookup is idempotent: calling it from both
// goFieldType and fieldType for the same field is harmless.
func (fg *FileGenerator) customtypeAlias(importPath string) string {
	return fg.imports.addExplicitAliasImport(importPath)
}

// fieldType returns the FieldType composite for a field, with one twist over
// types.ForField: when `(wiresmith.options.pointer) = true`, singular message
// fields route through PointerField and repeated message fields through
// RepeatedPointer. All other paths delegate unchanged.
//
// This is the single dispatch point used by emit_marshal, emit_size, and the
// list/singular branches of emit_unmarshal — keeping the pointer option
// visible in exactly one place so future option-driven shape changes have a
// clear home.
func (fg *FileGenerator) fieldType(fd protoreflect.FieldDescriptor) types.FieldType {
	if ft, ok := fg.customtypeFieldType(fd); ok {
		return ft
	}
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
	if !fg.hasPointerOption(fd) {
		return types.ForField(fd)
	}
	if isRealOneof(fd) || fd.HasOptionalKeyword() || fd.Kind() != protoreflect.MessageKind {
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

// customtypeFieldType returns a CustomType FieldType when the field is
// annotated with `(wiresmith.options.customtype)`. The same import-path
// resolution as customtypeGoFieldType happens here so the struct-field
// declaration and the marshal/unmarshal emission stay consistent.
//
// Restricts to singular bytes/string in v1 — validation has already rejected
// other kinds at this point, so the guard is defensive against direct
// descriptor construction in tests.
func (fg *FileGenerator) customtypeFieldType(fd protoreflect.FieldDescriptor) (types.FieldType, bool) {
	v, ok := fg.customtypeValue(fd)
	if !ok {
		return nil, false
	}
	if fd.Kind() != protoreflect.BytesKind && fd.Kind() != protoreflect.StringKind {
		return nil, false
	}
	if fd.IsMap() || fd.IsList() || fd.HasOptionalKeyword() || isRealOneof(fd) {
		return nil, false
	}
	importPath, _, err := parseCustomtypeValue(v)
	if err != nil {
		return nil, false
	}
	// Register the import so the companion `customtypeGoFieldType` (used for
	// the struct field declaration) and the marshal/unmarshal emitters reach
	// the user's package via the same alias. Side-effect-only on the
	// ImportTracker; the resolved identifier is consumed at the struct-field
	// emit site, not stored on CustomType.
	if importPath != "" {
		fg.customtypeAlias(importPath)
	}
	return &types.CustomType{}, true
}
