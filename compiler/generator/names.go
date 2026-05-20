package generator

import (
	"path"
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
	return b.String()
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

// effectiveBase returns the Go import path prefix under which wiresmith
// generates code. A go_package option counts as "ours" only if its import
// path falls under this base.
func effectiveBase(module string) string {
	return module + "/gen"
}

// goDest is the canonical Go destination for a proto package — every
// consumer (output paths, package clauses, cross-file import resolution,
// collision detection) reads from here so they can't disagree.
type goDest struct {
	importPath string // full Go import path of the destination
	relDir     string // path relative to OutDir / effectiveBase
	pkgName    string // declared `package` clause in the generated file
}

// destFor returns the canonical destination for protoPkg, preferring the
// proto's `go_package` option when it falls under our base and falling
// back to the default `<base>/<dotted-package-as-path>` mapping otherwise.
// This is the single function callers should ask "where does proto pkg
// X land in Go?" — every other resolver in the package routes through here.
func destFor(module, protoPkg string, goPackages map[string]string) goDest {
	base := effectiveBase(module)
	if importPath, relDir, pkgName, ok := resolveGoPackage(protoPkg, goPackages, base); ok {
		return goDest{importPath: importPath, relDir: relDir, pkgName: pkgName}
	}
	relDir := goPackageDir(protoPkg)
	return goDest{
		importPath: base + "/" + relDir,
		relDir:     relDir,
		pkgName:    goPackageName(protoPkg),
	}
}

// parseGoPackage parses a go_package option value. The proto3 format is
// "import/path" or "import/path;name" — the optional semicolon form lets
// the .proto author override the Go package name independently of the
// import path's last component. The explicit pkgName is sanitized too:
// an author who writes ";my-pkg" probably means "my_pkg".
func parseGoPackage(goPackage string) (importPath, pkgName string) {
	if goPackage == "" {
		return "", ""
	}
	if i := strings.LastIndex(goPackage, ";"); i >= 0 {
		importPath = goPackage[:i]
		raw := goPackage[i+1:]
		if raw == "" {
			raw = path.Base(importPath)
		}
		return importPath, cleanPackageName(raw)
	}
	return goPackage, cleanPackageName(path.Base(goPackage))
}

// goKeywords lists every reserved word in the Go language spec — none can
// stand alone as a package clause, so cleanPackageName escapes them.
var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

// cleanPackageName replaces characters that are not valid in a Go identifier
// with underscores. A leading digit is also replaced because Go identifiers
// must not start with a digit. Reserved Go keywords are escaped with a
// trailing underscore so they can be used as `package` clauses. Matches
// protogen/gogoproto behavior.
func cleanPackageName(name string) string {
	if name == "" {
		return "_"
	}
	var b strings.Builder
	b.Grow(len(name))
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
			b.WriteRune(r)
		case i > 0 && r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if goKeywords[out] {
		out += "_"
	}
	return out
}

// resolveGoPackage looks up the go_package option for protoPkg and, if it
// falls under base, returns the import path, the directory relative to base,
// and the Go package name. ok is false when no go_package is set or when it
// points outside base — callers should fall back to the default scheme.
func resolveGoPackage(protoPkg string, goPackages map[string]string, base string) (importPath, relDir, pkgName string, ok bool) {
	goPkg, exists := goPackages[protoPkg]
	if !exists {
		return "", "", "", false
	}
	importPath, pkgName = parseGoPackage(goPkg)
	switch {
	case importPath == base:
		return importPath, "", pkgName, true
	case strings.HasPrefix(importPath, base+"/"):
		return importPath, strings.TrimPrefix(importPath, base+"/"), pkgName, true
	default:
		return "", "", "", false
	}
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

// goEnumValuePrefix returns the prefix for enum constant names, matching
// protoc-gen-go: parent message chain for nested enums, enum name for
// top-level enums.
func goEnumValuePrefix(ed protoreflect.EnumDescriptor) string {
	pm, ok := ed.Parent().(protoreflect.MessageDescriptor)
	if !ok {
		return goEnumTypeName(ed)
	}
	return goMessageTypeName(pm)
}

// leadingComment extracts the leading comment from a proto descriptor's
// source location and formats it as a Go comment block.
func leadingComment(d protoreflect.Descriptor) string {
	loc := d.ParentFile().SourceLocations().ByDescriptor(d)
	comment := strings.TrimSpace(loc.LeadingComments)
	if comment == "" {
		return ""
	}
	var b strings.Builder
	for _, line := range strings.Split(comment, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			b.WriteString("//\n")
		} else {
			b.WriteString("// ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// indentComment adds a tab prefix to each line of a Go comment block.
func indentComment(comment string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(comment, "\n"), "\n") {
		b.WriteByte('\t')
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
