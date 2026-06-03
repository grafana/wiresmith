package generator

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/bufbuild/protocompile/linker"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// customtypeExtensionName is the fully qualified name of the customtype
// extension defined in the embedded wiresmith/options.proto.
const customtypeExtensionName = "wiresmith.options.customtype"

// resolveCustomtypeExtension caches the linked extension descriptor on the
// Generator. Mirrors resolvePointerExtension; runs once per Generate.
func (g *Generator) resolveCustomtypeExtension(results linker.Files) error {
	for _, fd := range results {
		if fd.Path() != embeddedOptionsPath {
			continue
		}
		exts := fd.Extensions()
		for i := 0; i < exts.Len(); i++ {
			x := exts.Get(i)
			if string(x.FullName()) == customtypeExtensionName {
				g.customtypeExt = x
				return nil
			}
		}
	}
	return fmt.Errorf("internal error: extension %q not found in compiled results — wiresmith/options.proto missing or malformed", customtypeExtensionName)
}

// customtypeValue returns the raw `(wiresmith.options.customtype)` string for
// fd plus a presence boolean. Empty string with `ok=true` is not a valid
// placement and is rejected by validateCustomtypeOptions; callers below use
// the bool to skip the option entirely when the field is unannotated.
func (fg *FileGenerator) customtypeValue(fd protoreflect.FieldDescriptor) (string, bool) {
	return customtypeValue(fg.customtypeExt, fd)
}

func customtypeValue(ext protoreflect.FieldDescriptor, fd protoreflect.FieldDescriptor) (string, bool) {
	if ext == nil {
		return "", false
	}
	opts, ok := fd.Options().(*descriptorpb.FieldOptions)
	if !ok || opts == nil {
		return "", false
	}
	xd, ok := ext.(protoreflect.ExtensionTypeDescriptor)
	var xt protoreflect.ExtensionType
	if ok {
		xt = xd.Type()
	} else {
		xt = dynamicpb.NewExtensionType(ext)
	}
	if !proto.HasExtension(opts, xt) {
		return "", false
	}
	v, _ := proto.GetExtension(opts, xt).(string)
	return v, true
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
	// The package's path base is used verbatim as the Go identifier in
	// generated code (it's what an unaliased `import "..."` binds to), so
	// it must itself be a valid Go identifier — `github.com/foo/bar-baz`
	// would otherwise emit `bar-baz.Type` and fail to compile. We only
	// reject obvious "won't compile" cases; mismatches between path base
	// and the package's `package` declaration are out of scope (callers
	// whose directory name differs from their package name should rename
	// the directory).
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

// validateCustomtypeOptions walks every field in every result file and
// rejects invalid placements of `(wiresmith.options.customtype)`. Runs after
// resolveCustomtypeExtension and before any emit pass.
//
// v1 scope: only singular `bytes` and `string` fields. Map values, oneof
// variants, repeated, optional, scalar non-bytes/string, message, and enum
// fields are all rejected. Same combined-error shape as
// validatePointerOptions so one bad fixture lists every offending field.
func (g *Generator) validateCustomtypeOptions(results linker.Files) error {
	if g.customtypeExt == nil {
		return nil
	}
	var errs []string
	for _, fd := range results {
		if isInternalSchemaFile(fd) {
			continue
		}
		walkFields(fd, func(field protoreflect.FieldDescriptor) {
			v, ok := customtypeValue(g.customtypeExt, field)
			if !ok {
				return
			}
			if reason := customtypeOptionRejection(field, v); reason != "" {
				errs = append(errs, fmt.Sprintf("%s: field %q: %s", fd.Path(), field.FullName(), reason))
			}
		})
	}
	if len(errs) == 0 {
		return nil
	}
	out := "invalid (wiresmith.options.customtype) placement:\n"
	for _, e := range errs {
		out += "  - " + e + "\n"
	}
	return fmt.Errorf("%s", out)
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
	if fd.IsList() {
		return "(wiresmith.options.customtype) is not supported on repeated fields (v1 scope)"
	}
	if fd.HasOptionalKeyword() {
		return "(wiresmith.options.customtype) is not supported on proto3 `optional` fields"
	}
	switch fd.Kind() {
	case protoreflect.BytesKind, protoreflect.StringKind:
		return ""
	default:
		return fmt.Sprintf("(wiresmith.options.customtype) only applies to singular bytes or string fields, got %s", fd.Kind())
	}
}
