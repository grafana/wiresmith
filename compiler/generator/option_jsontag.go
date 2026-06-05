package generator

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// jsontagExtensionName is the fully qualified name of the jsontag extension
// defined in the embedded wiresmith/options.proto. Mirrors pointerExtensionName.
const jsontagExtensionName = "wiresmith.options.jsontag"

// resolveJsontagExtension finds the `(wiresmith.options.jsontag)` extension
// descriptor among the linked files and caches it on the Generator. Mirrors
// resolvePointerExtension — same lookup-by-canonical-path approach so a user
// file declaring the same proto package cannot shadow the embedded definition.
func (g *Generator) resolveJsontagExtension(results []protoreflect.FileDescriptor) error {
	g.jsontagExt = nil
	for _, fd := range results {
		if fd.Path() != embeddedOptionsPath {
			continue
		}
		exts := fd.Extensions()
		for i := 0; i < exts.Len(); i++ {
			x := exts.Get(i)
			if string(x.FullName()) == jsontagExtensionName {
				g.jsontagExt = x
				return nil
			}
		}
	}
	// Same rationale as the FieldOption loop in generateFromFiles: in
	// plugin mode wiresmith/options.proto may legitimately not be part of
	// the request, so a missing descriptor here means "no jsontag overrides
	// in use" rather than an internal error. jsontagOverride already
	// short-circuits on nil ext.
	return nil
}

// jsontagOverride returns the user-supplied `json:"..."` tag value for fd, plus
// a boolean indicating whether the option was set at all. The empty string is
// a legal explicit value (verbatim passthrough — matches gogoproto.jsontag),
// so callers must rely on the boolean, not the string.
func (fg *FileGenerator) jsontagOverride(fd protoreflect.FieldDescriptor) (string, bool) {
	return jsontagOverride(fg.jsontagExt, fd)
}

func jsontagOverride(ext protoreflect.FieldDescriptor, fd protoreflect.FieldDescriptor) (string, bool) {
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

// validateJsontagOptions walks every field in every result file and rejects
// jsontag values that would produce malformed generated code. Mirrors the
// validatePointerOptions shape so errors surface as one combined diagnostic.
//
// Backticks are the only character we reject: they would terminate the
// raw-string struct tag emitted by fieldTag/mapFieldTag. Quotes inside the
// value are safe because tag emission goes through %q, which escapes them.
func (g *Generator) validateJsontagOptions(results []protoreflect.FileDescriptor) error {
	if g.jsontagExt == nil {
		return nil
	}
	var errs []string
	for _, fd := range results {
		if isInternalSchemaFile(fd) {
			continue
		}
		walkFields(fd, func(field protoreflect.FieldDescriptor) {
			v, ok := jsontagOverride(g.jsontagExt, field)
			if !ok {
				return
			}
			if strings.ContainsRune(v, '`') {
				errs = append(errs, fmt.Sprintf("%s: field %q: (wiresmith.options.jsontag) must not contain backticks (would terminate the struct tag)", fd.Path(), field.FullName()))
			}
		})
	}
	if len(errs) == 0 {
		return nil
	}
	out := "invalid (wiresmith.options.jsontag) value:\n"
	for _, e := range errs {
		out += "  - " + e + "\n"
	}
	return fmt.Errorf("%s", out)
}
