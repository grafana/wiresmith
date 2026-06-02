package generator

import (
	"fmt"
	"unicode"

	"github.com/bufbuild/protocompile/linker"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// customnameExtensionName is the fully qualified name of the customname
// extension defined in the embedded wiresmith/options.proto.
const customnameExtensionName = "wiresmith.options.customname"

// resolveCustomnameExtension caches the linked extension descriptor on the
// Generator. Mirrors resolvePointerExtension.
func (g *Generator) resolveCustomnameExtension(results linker.Files) error {
	for _, fd := range results {
		if fd.Path() != embeddedOptionsPath {
			continue
		}
		exts := fd.Extensions()
		for i := 0; i < exts.Len(); i++ {
			x := exts.Get(i)
			if string(x.FullName()) == customnameExtensionName {
				g.customnameExt = x
				return nil
			}
		}
	}
	return fmt.Errorf("internal error: extension %q not found in compiled results — wiresmith/options.proto missing or malformed", customnameExtensionName)
}

// customnameValue returns the user-supplied Go identifier for fd plus a
// presence boolean. Validation has already rejected malformed values at
// this point, so callers can use the string verbatim.
func (fg *FileGenerator) customnameValue(fd protoreflect.FieldDescriptor) (string, bool) {
	return customnameValue(fg.customnameExt, fd)
}

func customnameValue(ext protoreflect.FieldDescriptor, fd protoreflect.FieldDescriptor) (string, bool) {
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

// goFieldName returns the identifier to use for the Go field, accessor
// methods (Get<Name>, Has<Name>), oneof wrapper-struct field, and equality
// comparisons. Replaces every `snakeToPascal(string(fd.Name()))` call site
// — keeping the lookup in one place means a future renaming rule won't
// have 17 places to remember.
//
// Falls back to the snake-to-PascalCase default when the field has no
// customname annotation, matching protoc-gen-go's behavior for fields
// without overrides.
func (fg *FileGenerator) goFieldName(fd protoreflect.FieldDescriptor) string {
	if v, ok := fg.customnameValue(fd); ok {
		return v
	}
	return snakeToPascal(string(fd.Name()))
}

// reservedCustomnameMethods enumerates the message-level methods the
// generator always emits with these exact names. A customname that matches
// one of these would shadow the method with a struct field of the same
// name and produce an "ambiguous selector" compile error far from the
// proto source; rejecting at codegen time keeps the diagnostic close to
// the offending field.
//
// Field-derived accessors (Get<Name>, Has<Name>) aren't here because their
// names depend on other fields in the same message — the field-pair
// collision check below catches those.
var reservedCustomnameMethods = map[string]bool{
	"Reset":                true,
	"String":               true,
	"ProtoMessage":         true,
	"ProtoReflect":         true,
	"Marshal":              true,
	"MarshalTo":            true,
	"MarshalToSizedBuffer": true,
	"Unmarshal":            true,
	"UnmarshalWithDepth":   true,
	"Size":                 true,
	"Equal":                true,
	"Compare":              true,
}

// validateCustomnameOptions walks every message's fields and rejects
// customname values that aren't legal exported Go identifiers, that
// collide with always-generated methods, or that resolve to the same Go
// name as another field in the same message. Each class of failure would
// otherwise surface as a confusing `go build` error far from the .proto
// source; catching it here points back at the offending field.
func (g *Generator) validateCustomnameOptions(results linker.Files) error {
	if g.customnameExt == nil {
		return nil
	}
	var errs []string
	for _, fd := range results {
		if isInternalSchemaFile(fd) {
			continue
		}
		forEachMessage(fd, func(md protoreflect.MessageDescriptor) {
			// usedNames tracks every Go identifier that will appear as a
			// struct field in the generated `type <Msg> struct { ... }` —
			// the resolved name (customname or default) for each
			// non-oneof field, plus the parent-struct slot for each
			// oneof. Two entries pointing at the same name flag a
			// guaranteed Go-compile error in the generated file.
			usedNames := map[string]string{}
			fields := md.Fields()
			seenOneofs := map[string]bool{}
			for i := 0; i < fields.Len(); i++ {
				field := fields.Get(i)
				if isRealOneof(field) {
					ooName := snakeToPascal(string(field.ContainingOneof().Name()))
					if seenOneofs[string(field.ContainingOneof().Name())] {
						// Each oneof contributes one struct slot —
						// account for it once across all variants.
					} else {
						seenOneofs[string(field.ContainingOneof().Name())] = true
						if prev, dup := usedNames[ooName]; dup {
							errs = append(errs, fmt.Sprintf("%s: oneof %q: resolved Go name %q collides with %s in the same message", fd.Path(), field.ContainingOneof().FullName(), ooName, prev))
						} else {
							usedNames[ooName] = fmt.Sprintf("oneof %q", field.ContainingOneof().FullName())
						}
					}
					// Validate the variant's own customname (its name
					// lives inside the wrapper struct, so it doesn't
					// participate in the message-struct collision check —
					// but it still has to pass syntactic + reserved-name
					// validation).
					if v, ok := customnameValue(g.customnameExt, field); ok {
						if reason := customnameOptionRejection(v); reason != "" {
							errs = append(errs, fmt.Sprintf("%s: field %q: %s", fd.Path(), field.FullName(), reason))
						}
					}
					continue
				}
				resolved := snakeToPascal(string(field.Name()))
				if v, ok := customnameValue(g.customnameExt, field); ok {
					if reason := customnameOptionRejection(v); reason != "" {
						errs = append(errs, fmt.Sprintf("%s: field %q: %s", fd.Path(), field.FullName(), reason))
						continue
					}
					resolved = v
				}
				if prev, dup := usedNames[resolved]; dup {
					errs = append(errs, fmt.Sprintf("%s: field %q: resolved Go name %q collides with %s in the same message", fd.Path(), field.FullName(), resolved, prev))
				} else {
					usedNames[resolved] = fmt.Sprintf("field %q", field.FullName())
				}
			}
		})
	}
	if len(errs) == 0 {
		return nil
	}
	out := "invalid (wiresmith.options.customname) value:\n"
	for _, e := range errs {
		out += "  - " + e + "\n"
	}
	return fmt.Errorf("%s", out)
}

// customnameOptionRejection returns a human-readable reason if value is not
// a legal exported Go identifier — or if it collides with a name the
// generator always emits as a method on every message — and "" otherwise.
// The exported-only rule matches the rest of the generated API: every
// accessor it touches is on a `*Holder` receiver, so a lowercase first
// letter would make cross-package callers unable to read or set the
// field.
func customnameOptionRejection(value string) string {
	if value == "" {
		return "(wiresmith.options.customname) value must not be empty"
	}
	// value[0] reads only the first *byte*, which is a UTF-8 continuation
	// byte for any non-ASCII identifier (Σ → 0xCE, etc.) and would
	// misclassify valid exported names. Decode the first rune properly.
	var first rune
	for _, r := range value {
		first = r
		break
	}
	if !unicode.IsUpper(first) {
		return fmt.Sprintf("(wiresmith.options.customname) value %q must start with an uppercase letter (Go exported identifier)", value)
	}
	for i, r := range value {
		if i == 0 {
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return fmt.Sprintf("(wiresmith.options.customname) value %q contains invalid character %q", value, r)
		}
	}
	if reservedCustomnameMethods[value] {
		return fmt.Sprintf("(wiresmith.options.customname) value %q collides with an always-generated method (Reset, String, Marshal, Unmarshal, Equal, Compare, …) — a field of that name would shadow the method", value)
	}
	return ""
}
