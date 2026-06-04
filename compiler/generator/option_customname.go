package generator

import (
	"fmt"
	"unicode"

	"github.com/bufbuild/protocompile/linker"
	"github.com/grafana/wiresmith/compiler/types"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// customnameExtensionName is the fully qualified name of the customname
// extension defined in the embedded wiresmith/options.proto.
const customnameExtensionName = "wiresmith.options.customname"

// customnameOption implements FieldOption for `(wiresmith.options.customname)`.
// The user-supplied identifier replaces the snake_to_pascal default for
// the Go field name, the Get<Name> / Has<Name> accessors, the oneof
// wrapper-struct slot, and every cross-reference in emit_*.go. The option
// does not influence FieldType / GoFieldType — those keep delegating to
// the default registry pass.
type customnameOption struct {
	ext protoreflect.FieldDescriptor
}

func (*customnameOption) Name() string                               { return customnameExtensionName }
func (o *customnameOption) Resolve(ext protoreflect.FieldDescriptor) { o.ext = ext }

// Value returns the user-supplied Go identifier for fd plus a presence
// boolean. Validation has already rejected malformed values at this
// point, so callers can use the string verbatim.
func (o *customnameOption) Value(fd protoreflect.FieldDescriptor) (string, bool) {
	return stringOption(o.ext, fd)
}

// FieldType / GoFieldType always return false: customname affects naming,
// not the field's Go-side type or wire encoding. Satisfies FieldOption so
// the option can ride the same resolve+validate loop as the others.
func (*customnameOption) FieldType(*FileGenerator, protoreflect.FieldDescriptor) (types.FieldType, bool) {
	return nil, false
}

func (*customnameOption) GoFieldType(*FileGenerator, protoreflect.FieldDescriptor) (string, bool) {
	return "", false
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

// Validate walks every message's fields and rejects customname values
// that aren't legal exported Go identifiers, that collide with always-
// generated methods, or that resolve to the same Go name as another field
// in the same message. Each class of failure would otherwise surface as a
// confusing `go build` error far from the .proto source; catching it here
// points back at the offending field.
func (o *customnameOption) Validate(g *Generator, results linker.Files) error {
	if o.ext == nil {
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
					if !seenOneofs[string(field.ContainingOneof().Name())] {
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
					if v, ok := o.Value(field); ok {
						if reason := customnameOptionRejection(v); reason != "" {
							errs = append(errs, fmt.Sprintf("%s: field %q: %s", fd.Path(), field.FullName(), reason))
						}
					}
					continue
				}
				resolved := snakeToPascal(string(field.Name()))
				if v, ok := o.Value(field); ok {
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
	return combinedOptionError(customnameExtensionName, "value", errs)
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
