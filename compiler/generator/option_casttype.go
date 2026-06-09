package generator

import (
	"fmt"

	"github.com/grafana/wiresmith/compiler/types"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// casttypeExtensionName is the fully qualified name of the casttype
// extension defined in the embedded wiresmith/options.proto.
const casttypeExtensionName = "wiresmith.options.casttype"

// casttypeOption implements FieldOption for `(wiresmith.options.casttype)`.
// Unlike customtype, casttype does NOT change the wire encoding — only the
// Go-side field type name. The marshal/unmarshal hot path keeps the
// underlying scalar's logic and bridges via Go type conversions.
type casttypeOption struct {
	ext protoreflect.FieldDescriptor
}

func (*casttypeOption) Name() string                               { return casttypeExtensionName }
func (o *casttypeOption) Resolve(ext protoreflect.FieldDescriptor) { o.ext = ext }

// Value returns the raw `(wiresmith.options.casttype)` string for fd plus a
// presence boolean. Empty string with ok=true is not a valid placement and
// is rejected by Validate; callers below use the bool to skip the option
// entirely when the field is unannotated.
func (o *casttypeOption) Value(fd protoreflect.FieldDescriptor) (string, bool) {
	return stringOption(o.ext, fd)
}

// Has satisfies FieldOption. Casttype is a string-valued option, so
// presence equals "Value returned ok=true".
func (o *casttypeOption) Has(fd protoreflect.FieldDescriptor) bool {
	_, ok := o.Value(fd)
	return ok
}

// Validate rejects invalid placements of casttype.
//
// Allowed: singular int{32,64} / uint{32,64} / sint{32,64} /
// fixed{32,64} / sfixed{32,64} / bool / string / bytes. Repeated,
// optional, map, oneof, enum, message, and float/double are rejected.
// Float/double are deferred because the float emit path uses
// math.Float*bits which does not accept a defined-type argument; bridging
// it cleanly requires changing every emit site rather than wrapping at
// the FieldType boundary.
func (o *casttypeOption) Validate(g *Generator, results []protoreflect.FileDescriptor) error {
	if o.ext == nil {
		return nil
	}
	var errs []string
	for _, fd := range results {
		if isInternalSchemaFile(fd) {
			continue
		}
		walkFields(fd, func(field protoreflect.FieldDescriptor) {
			v, ok := o.Value(field)
			if !ok {
				return
			}
			if reason := casttypeOptionRejection(field, v); reason != "" {
				errs = append(errs, fmt.Sprintf("%s: field %q: %s", fd.Path(), field.FullName(), reason))
			}
		})
	}
	return combinedOptionError(casttypeExtensionName, "placement", errs)
}

// FieldType returns a CastType wrapper around the underlying scalar Type.
// The user-supplied import is registered as a side effect (same idempotent
// shape customtype uses) so the struct-field declaration and the
// size/marshal/unmarshal emit paths reach the user's package via the same
// alias.
func (o *casttypeOption) FieldType(fg *FileGenerator, fd protoreflect.FieldDescriptor) (types.FieldType, bool) {
	v, ok := o.Value(fd)
	if !ok {
		return nil, false
	}
	if !o.applies(fd) {
		return nil, false
	}
	importPath, typeName, err := parseCustomtypeValue(v)
	if err != nil {
		return nil, false
	}
	goType := typeName
	if importPath != "" {
		alias := fg.imports.addExplicitAliasImport(importPath)
		goType = alias + "." + typeName
	}
	inner, ok := types.Get(fd.Kind()).(types.Type)
	if !ok {
		return nil, false
	}
	return &types.CastType{Inner: inner, GoAlias: goType}, true
}

// GoFieldType resolves the casttype value to its Go type expression.
// Repeated/optional/map are rejected by Validate; this only fires on the
// singular path.
func (o *casttypeOption) GoFieldType(fg *FileGenerator, fd protoreflect.FieldDescriptor) (string, bool) {
	v, ok := o.Value(fd)
	if !ok {
		return "", false
	}
	if !o.applies(fd) {
		return "", false
	}
	importPath, typeName, err := parseCustomtypeValue(v)
	if err != nil {
		return "", false
	}
	if importPath != "" {
		alias := fg.imports.addExplicitAliasImport(importPath)
		return alias + "." + typeName, true
	}
	return typeName, true
}

// applies merges the "annotated" and "shape is acceptable" checks shared
// between FieldType and GoFieldType. Validate has already rejected most
// shapes, so this guards against direct descriptor construction in tests.
func (o *casttypeOption) applies(fd protoreflect.FieldDescriptor) bool {
	if fd.IsMap() || fd.IsList() || fd.HasOptionalKeyword() || isRealOneof(fd) {
		return false
	}
	return types.CasttypeAllowed(fd.Kind())
}

// casttypeOptionRejection returns a human-readable reason if the option is
// not allowed on this field, or "" if the placement is valid. Mirrors
// customtypeOptionRejection.
func casttypeOptionRejection(fd protoreflect.FieldDescriptor, value string) string {
	if _, _, err := parseCustomtypeValue(value); err != nil {
		return fmt.Sprintf("(wiresmith.options.casttype) %v", err)
	}
	if fd.IsMap() {
		return "(wiresmith.options.casttype) is not supported on map fields"
	}
	if isRealOneof(fd) {
		return "(wiresmith.options.casttype) is not supported on oneof variants"
	}
	if fd.IsList() {
		return "(wiresmith.options.casttype) is not supported on repeated fields (v1 scope)"
	}
	if fd.HasOptionalKeyword() {
		return "(wiresmith.options.casttype) is not supported on proto3 `optional` fields (v1 scope)"
	}
	switch fd.Kind() {
	case protoreflect.MessageKind:
		return "(wiresmith.options.casttype) is not supported on message fields — see (wiresmith.options.customtype) for message-valued user types"
	case protoreflect.EnumKind:
		return "(wiresmith.options.casttype) is not supported on enum fields (v1 scope)"
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return fmt.Sprintf("(wiresmith.options.casttype) is not supported on %s fields (v1 scope — float/double need bit-cast handling at every emit site)", fd.Kind())
	}
	if !types.CasttypeAllowed(fd.Kind()) {
		return fmt.Sprintf("(wiresmith.options.casttype) only applies to scalar kinds, got %s", fd.Kind())
	}
	return ""
}
