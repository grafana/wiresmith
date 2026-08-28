package generator

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// unsupportedWKTFullNames is the proto full name of every well-known type
// wiresmith never generates code for — no wiresmith struct, no shipped
// replacement (like anypb.Any), and no value-type substitution option
// (like stdtime/stdduration for Timestamp/Duration). See docs/design.md /
// CLAUDE.md "Supported proto3 features".
var unsupportedWKTFullNames = map[string]bool{
	"google.protobuf.Empty":         true,
	"google.protobuf.Struct":        true,
	"google.protobuf.Value":         true,
	"google.protobuf.ListValue":     true,
	"google.protobuf.NullValue":     true,
	"google.protobuf.FieldMask":     true,
	"google.protobuf.DoubleValue":   true,
	"google.protobuf.FloatValue":    true,
	"google.protobuf.Int64Value":    true,
	"google.protobuf.UInt64Value":   true,
	"google.protobuf.Int32Value":    true,
	"google.protobuf.UInt32Value":   true,
	"google.protobuf.BoolValue":     true,
	"google.protobuf.StringValue":   true,
	"google.protobuf.BytesValue":    true,
	"google.protobuf.Api":           true,
	"google.protobuf.Method":        true,
	"google.protobuf.Mixin":         true,
	"google.protobuf.Type":          true,
	"google.protobuf.Field":         true,
	"google.protobuf.Enum":          true,
	"google.protobuf.EnumValue":     true,
	"google.protobuf.Option":        true,
	"google.protobuf.SourceContext": true,
}

// validateWellKnownFields rejects message fields that reference a
// well-known type wiresmith cannot generate working code for:
//   - Empty, Struct/Value/wrappers, Api/Type/SourceContext, FieldMask —
//     unconditionally unsupported (unsupportedWKTFullNames).
//   - Timestamp/Duration without the matching stdtime/stdduration option —
//     these ARE supported, just require the annotation.
//
// google.protobuf.Any is excluded: wellknown.go resolves it to a full
// wiresmith-owned replacement package, so it needs no option and no entry
// here.
//
// Without this check, an unannotated field of one of these types silently
// resolves to the official google.golang.org/protobuf Go type via the
// normal cross-file import path, while wiresmith still emits its own
// wire-method calls on it (.Size()/.MarshalToSizedBuffer()/
// .UnmarshalWithDepth()/.Equal()/.Compare()/.Clone()) — methods the
// official type doesn't implement. The failure then only surfaces at
// `go build`, far from — and much less clear than — the actual mistake.
func (g *Generator) validateWellKnownFields(results []protoreflect.FileDescriptor) error {
	stdtimeOpt := findOption[*stdtimeOption](g.options)
	stddurationOpt := findOption[*stddurationOption](g.options)

	var errs []string
	check := func(fdPath string, field protoreflect.FieldDescriptor, kind protoreflect.Kind, msg protoreflect.MessageDescriptor) {
		if kind != protoreflect.MessageKind {
			return
		}
		fullName := string(msg.FullName())
		if unsupportedWKTFullNames[fullName] {
			errs = append(errs, fmt.Sprintf("%s: field %q references unsupported well-known type %s (see docs/design.md)",
				fdPath, field.FullName(), fullName))
			return
		}
		switch fullName {
		case timestampMessageFullName:
			if stdtimeOpt == nil || !stdtimeOpt.Has(field) {
				errs = append(errs, fmt.Sprintf("%s: field %q references google.protobuf.Timestamp without (wiresmith.options.stdtime) = true — wiresmith has no native Timestamp message type",
					fdPath, field.FullName()))
			}
		case durationMessageFullName:
			if stddurationOpt == nil || !stddurationOpt.Has(field) {
				errs = append(errs, fmt.Sprintf("%s: field %q references google.protobuf.Duration without (wiresmith.options.stdduration) = true — wiresmith has no native Duration message type",
					fdPath, field.FullName()))
			}
		}
	}

	for _, fd := range results {
		if isInternalSchemaFile(fd) {
			continue
		}
		walkFields(fd, func(field protoreflect.FieldDescriptor) {
			if field.IsMap() {
				// The map field's own Kind()/Message() describe the
				// synthetic MapEntry wrapper, not the value type — check
				// the value descriptor directly instead.
				check(fd.Path(), field, field.MapValue().Kind(), field.MapValue().Message())
				return
			}
			check(fd.Path(), field, field.Kind(), field.Message())
		})
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("unsupported well-known type field(s):\n  - %s", strings.Join(errs, "\n  - "))
}
