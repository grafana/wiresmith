package generator

import (
	"strings"
	"unicode"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func snakeToPascal(s string) string {
	var b strings.Builder
	upper := true
	for _, c := range s {
		if c == '_' {
			upper = true
			continue
		}
		if upper {
			b.WriteRune(unicode.ToUpper(c))
			upper = false
		} else {
			b.WriteRune(c)
		}
	}
	result := b.String()
	// Append underscore if the ORIGINAL name (before conversion) is a Go keyword
	// or predeclared identifier (matching gogoslick behavior).
	// E.g., proto field "string" → "String_", but "type" → "Type" (not a conflict).
	if isGoKeyword(s) {
		result += "_"
	}
	return result
}

// isGoKeyword returns true if the identifier, when PascalCased, would
// collide with a common Go method name or predeclared identifier.
// gogoslick appends underscore for these cases.
// NOTE: not all Go keywords need this — e.g., "type" → "Type" is fine.
// This list matches gogoslick's actual behavior.
func isGoKeyword(s string) bool {
	// Only proto field names that gogoslick actually appends underscore to.
	// This is a narrow list matching observed gogoslick behavior.
	switch s {
	case "string":
		return true
	}
	return false
}

func goPackageName(protoPkg string) string {
	parts := strings.Split(protoPkg, ".")
	if len(parts) < 2 {
		return protoPkg
	}
	return parts[len(parts)-2] + parts[len(parts)-1]
}

func goPackageDir(protoPkg string) string {
	parts := strings.Split(protoPkg, ".")
	if len(parts) >= 3 && parts[0] == "opentelemetry" && parts[1] == "proto" {
		return "otlp/" + strings.Join(parts[2:], "/")
	}
	return strings.Join(parts, "/")
}

func goImportPath(module, protoPkg string) string {
	return module + "/gen/" + goPackageDir(protoPkg)
}

func goMessageTypeName(md protoreflect.MessageDescriptor) string {
	name := string(md.Name())
	parent := md.Parent()
	for {
		pm, ok := parent.(protoreflect.MessageDescriptor)
		if !ok {
			break
		}
		name = string(pm.Name()) + "_" + name
		parent = pm.Parent()
	}
	return name
}

func goEnumTypeName(ed protoreflect.EnumDescriptor) string {
	name := string(ed.Name())
	parent := ed.Parent()
	for {
		pm, ok := parent.(protoreflect.MessageDescriptor)
		if !ok {
			break
		}
		name = string(pm.Name()) + "_" + name
		parent = pm.Parent()
	}
	return name
}
