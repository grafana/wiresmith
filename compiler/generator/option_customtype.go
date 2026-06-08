package generator

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/grafana/wiresmith/compiler/types"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// customtypeExtensionName is the fully qualified name of the customtype
// extension defined in the embedded wiresmith/options.proto.
const customtypeExtensionName = "wiresmith.options.customtype"

// customtypeOption implements FieldOption for `(wiresmith.options.customtype)`.
// The user-supplied "import/path.TypeName" string replaces the Go-side
// field type with an opaque type that owns its wire encoding via the
// Size/Marshal/Unmarshal/Equal/Compare-Wiresmith interface.
type customtypeOption struct {
	ext protoreflect.FieldDescriptor
}

func (*customtypeOption) Name() string                               { return customtypeExtensionName }
func (o *customtypeOption) Resolve(ext protoreflect.FieldDescriptor) { o.ext = ext }

// Value returns the raw `(wiresmith.options.customtype)` string for fd plus
// a presence boolean. Empty string with `ok=true` is not a valid placement
// and is rejected by Validate; callers below use the bool to skip the
// option entirely when the field is unannotated.
func (o *customtypeOption) Value(fd protoreflect.FieldDescriptor) (string, bool) {
	return stringOption(o.ext, fd)
}

// Has satisfies FieldOption. Customtype is a string-valued option, so
// presence equals "Value returned ok=true".
func (o *customtypeOption) Has(fd protoreflect.FieldDescriptor) bool {
	_, ok := o.Value(fd)
	return ok
}

// customtypeCompatiblePeers is the whitelist of peer wiresmith options that
// can coexist with customtype on the same field. Customtype owns the
// field's Go shape and wire-encode/decode entirely, so any option that
// would override the Go type or wire format is incompatible. Only the
// naming-only options (customname here; jsontag is outside the
// FieldOption registry and is implicitly compatible — it changes only
// the struct tag) make this list.
//
// Adding a new peer option means deciding here whether it can ride
// alongside customtype, which is the same gate as "does this option
// touch the Go type or wire bytes" — making the decision explicit and
// keeps the rejection list automatic.
var customtypeCompatiblePeers = map[string]bool{
	customnameExtensionName: true,
}

// Validate rejects invalid placements of customtype.
//
// Allowed: singular or repeated `bytes`, `string`, and message fields.
// Each per-element wire envelope is length-delimited regardless of source
// kind, so the user-supplied type's Size/Marshal/UnmarshalWiresmith
// contract is identical across the three — the bytes the type owns are
// raw bytes (`bytes`), UTF-8 (`string`), or an encoded submessage.
//
// Rejected: map values, oneof variants, optional fields, scalar non-
// bytes/string fields, enum fields, and any peer wiresmith option not in
// customtypeCompatiblePeers. Cross-option rejections live here so the
// compatibility whitelist reads as a single explicit list.
func (o *customtypeOption) Validate(g *Generator, results []protoreflect.FileDescriptor) error {
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
			if reason := customtypeOptionRejection(field, v); reason != "" {
				errs = append(errs, fmt.Sprintf("%s: field %q: %s", fd.Path(), field.FullName(), reason))
				return
			}
			for _, peer := range g.options {
				if peer.Name() == o.Name() {
					continue
				}
				if customtypeCompatiblePeers[peer.Name()] {
					continue
				}
				if peer.Has(field) {
					errs = append(errs, fmt.Sprintf("%s: field %q: (wiresmith.options.customtype) is not compatible with %s on the same field", fd.Path(), field.FullName(), peer.Name()))
				}
			}
		})
	}
	return combinedOptionError(customtypeExtensionName, "placement", errs)
}

// FieldType returns a CustomType wrapper (or RepeatedCustomType for a
// repeated customtype). Registers the user-supplied import (if any) under
// an explicit alias as a side effect so the marshal/unmarshal emission
// and the struct-field declaration reach the user's package via the same
// identifier.
//
// Dispatch matrix:
//   - singular bytes / string / message → CustomType
//   - repeated bytes / string / message → RepeatedCustomType
//
// Each per-element wire envelope is length-delimited (wire type 2), so
// the repeated path is kind-agnostic: the user type owns the payload bytes
// whether they originated as raw bytes, UTF-8 string, or encoded message.
//
// Other field shapes fall through to (nil, false); Validate has rejected
// them already, so this is purely defensive against direct descriptor
// construction in tests.
func (o *customtypeOption) FieldType(fg *FileGenerator, fd protoreflect.FieldDescriptor) (types.FieldType, bool) {
	v, ok := o.Value(fd)
	if !ok {
		return nil, false
	}
	if fd.IsMap() || fd.HasOptionalKeyword() || isRealOneof(fd) {
		return nil, false
	}
	switch fd.Kind() {
	case protoreflect.BytesKind, protoreflect.StringKind, protoreflect.MessageKind:
	default:
		return nil, false
	}
	importPath, typeName, err := parseCustomtypeValue(v)
	if err != nil {
		return nil, false
	}
	// Register the import even for the singular path. The struct-field
	// emitter goes through GoFieldType (not FieldType) and must reach the
	// user's package via the same alias the unmarshal code uses; doing
	// the registration here keeps both phases consistent.
	goType := typeName
	if importPath != "" {
		alias := fg.imports.addExplicitAliasImport(importPath)
		goType = alias + "." + typeName
	}
	if fd.IsList() {
		return &types.RepeatedCustomType{GoType: goType}, true
	}
	return &types.CustomType{}, true
}

// GoFieldType resolves the customtype value to its Go type expression
// (`alias.TypeName` for an external type, `TypeName` for a same-package
// type) and registers the supporting import. Repeated customtype fields
// wrap the resolved type in a slice. Validate has rejected malformed
// values at this point so the parse-error path is purely defensive
// against direct descriptor construction in tests.
func (o *customtypeOption) GoFieldType(fg *FileGenerator, fd protoreflect.FieldDescriptor) (string, bool) {
	v, ok := o.Value(fd)
	if !ok {
		return "", false
	}
	importPath, typeName, err := parseCustomtypeValue(v)
	if err != nil {
		return "", false
	}
	goType := typeName
	if importPath != "" {
		alias := fg.imports.addExplicitAliasImport(importPath)
		goType = alias + "." + typeName
	}
	if fd.IsList() {
		return "[]" + goType, true
	}
	return goType, true
}

// parseCustomtypeValue splits a value like "github.com/foo/bar.LabelAdapter"
// into ("github.com/foo/bar", "LabelAdapter"). For a same-package type with no
// dot, importPath is empty and typeName is the value verbatim.
//
// The split point is the LAST `.` after the LAST `/` — Go import paths contain
// dots in domain components (e.g. "github.com") so a naive last-dot split
// would mis-identify them as the type-name separator.
func parseCustomtypeValue(value string) (importPath, typeName string, err error) {
	if value == "" {
		return "", "", fmt.Errorf("value must not be empty")
	}
	// Whitespace inside the value (including the path prefix that
	// validateGoIdentifier doesn't see) would silently become part of the
	// import path emitted into the generated file and break the build at
	// `go build` time, far from the source of the typo. Reject it up front.
	if strings.ContainsAny(value, " \t\r\n") {
		return "", "", fmt.Errorf("value %q must not contain whitespace", value)
	}
	lastSlash := strings.LastIndex(value, "/")
	pathPrefix := ""
	packageAndType := value
	if lastSlash >= 0 {
		pathPrefix = value[:lastSlash+1]
		packageAndType = value[lastSlash+1:]
	}
	lastDot := strings.LastIndex(packageAndType, ".")
	if lastDot < 0 {
		// A slash in the input means the user wrote an import path. Without
		// a trailing ".TypeName" we'd otherwise silently swallow the path
		// and treat the basename as a same-package type — turning a typo
		// like "github.com/foo/bar" into "bar" with no import. Require the
		// dot in this case so the malformed value is rejected explicitly.
		if lastSlash >= 0 {
			return "", "", fmt.Errorf("value %q: import path is missing a \".TypeName\" suffix (use \"path/to/pkg.TypeName\" for an external type, or drop the path for a same-package type)", value)
		}
		if err := validateGoIdentifier(packageAndType); err != nil {
			return "", "", fmt.Errorf("type name %v", err)
		}
		return "", packageAndType, nil
	}
	pkgPart := packageAndType[:lastDot]
	typeName = packageAndType[lastDot+1:]
	if pkgPart == "" {
		return "", "", fmt.Errorf("value %q: package segment is empty (use \"path/to/pkg.TypeName\" or \"TypeName\" for same-package)", value)
	}
	if err := validateGoIdentifier(typeName); err != nil {
		return "", "", fmt.Errorf("type name %v", err)
	}
	// pkgPart becomes the *seed* for the import alias the generator emits
	// (ImportTracker.addExplicitAliasImport runs it through uniqueAlias to
	// disambiguate against other imports). The alias is always spelled out
	// in the generated import block, so the upstream package's declared
	// `package` name can differ from pkgPart without breaking the build —
	// but pkgPart itself must still be a valid Go identifier, since it
	// (or a numeric-suffixed variant) appears verbatim as the qualifier in
	// generated code. A path base like `github.com/foo/bar-baz` would
	// otherwise emit `bar-baz.Type` and fail to compile.
	if err := validateGoIdentifier(pkgPart); err != nil {
		return "", "", fmt.Errorf("value %q: package alias derived from import path: %v", value, err)
	}
	importPath = pathPrefix + pkgPart
	return importPath, typeName, nil
}

// validateGoIdentifier rejects values that wouldn't compile as a Go
// identifier. The check is intentionally narrow — it only catches obvious
// typos (empty string, leading digit, embedded whitespace, reserved
// punctuation). Anything subtler shows up as a Go compile error in the
// generated file, which is a clearer signal than us trying to second-guess
// the user's intent.
//
// The returned error is role-neutral ("is empty", "%q must start with..."),
// so callers wrap it with whatever role this identifier plays (`type name`,
// `package alias`, etc.) for a message that reads cleanly in either context.
func validateGoIdentifier(s string) error {
	if s == "" {
		return fmt.Errorf("is empty")
	}
	for i, r := range s {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return fmt.Errorf("%q must start with a letter or underscore", s)
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return fmt.Errorf("%q contains invalid character %q", s, r)
		}
	}
	return nil
}

// customtypeOptionRejection returns a human-readable reason if the option is
// not allowed on this field, or "" if the placement is valid. Mirrors
// pointerOptionRejection.
func customtypeOptionRejection(fd protoreflect.FieldDescriptor, value string) string {
	if _, _, err := parseCustomtypeValue(value); err != nil {
		return fmt.Sprintf("(wiresmith.options.customtype) %v", err)
	}
	if fd.IsMap() {
		return "(wiresmith.options.customtype) is not supported on map fields"
	}
	if isRealOneof(fd) {
		return "(wiresmith.options.customtype) is not supported on oneof variants"
	}
	if fd.HasOptionalKeyword() {
		return "(wiresmith.options.customtype) is not supported on proto3 `optional` fields"
	}
	switch fd.Kind() {
	case protoreflect.BytesKind, protoreflect.StringKind, protoreflect.MessageKind:
		return ""
	default:
		return fmt.Sprintf("(wiresmith.options.customtype) only applies to bytes, string, or message fields, got %s", fd.Kind())
	}
}
