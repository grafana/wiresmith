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

func goPackageName(protoPkg, stripPrefix string) string {
	if stripPrefix != "" {
		if stripped, ok := strings.CutPrefix(protoPkg, stripPrefix+"."); ok {
			parts := strings.Split(stripped, ".")
			return parts[len(parts)-1]
		}
	}
	parts := strings.Split(protoPkg, ".")
	if len(parts) < 2 {
		return protoPkg
	}
	return parts[len(parts)-2] + parts[len(parts)-1]
}

func goPackageDir(protoPkg, stripPrefix string) string {
	if stripPrefix != "" {
		if stripped, ok := strings.CutPrefix(protoPkg, stripPrefix+"."); ok {
			return strings.ReplaceAll(stripped, ".", "/")
		}
	}
	parts := strings.Split(protoPkg, ".")
	if len(parts) >= 3 && parts[0] == "opentelemetry" && parts[1] == "proto" {
		return "otlp/" + strings.Join(parts[2:], "/")
	}
	return strings.Join(parts, "/")
}

func effectiveBase(module, importBase string) string {
	if importBase != "" {
		return importBase
	}
	return module + "/gen"
}

func goImportPath(module, protoPkg, stripPrefix, importBase string) string {
	if importBase != "" {
		return importBase + "/" + goPackageDir(protoPkg, stripPrefix)
	}
	return module + "/gen/" + goPackageDir(protoPkg, stripPrefix)
}

func helpersImportPath(module, helpersImport string) string {
	if helpersImport != "" {
		return helpersImport
	}
	return module + "/gen/protohelpers"
}

// parseGoPackage parses a go_package option value into its import path and
// package name components. The format is "import/path" or "import/path;name".
func parseGoPackage(goPackage string) (importPath, pkgName string) {
	if goPackage == "" {
		return "", ""
	}
	if i := strings.LastIndex(goPackage, ";"); i >= 0 {
		return goPackage[:i], goPackage[i+1:]
	}
	return goPackage, path.Base(goPackage)
}

// resolveGoPackage checks whether protoPkg has a go_package option matching
// the given base prefix. Returns the full import path, relative directory,
// package name, and whether a match was found.
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
		relDir = strings.TrimPrefix(importPath, base+"/")
		return importPath, relDir, pkgName, true
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

// disambiguateAlias returns a longer alias when the short package name
// collides with the current file's package name (e.g. both are "v1").
func disambiguateAlias(protoPkg, stripPrefix string) string {
	raw := protoPkg
	if stripPrefix != "" {
		if stripped, ok := strings.CutPrefix(protoPkg, stripPrefix+"."); ok {
			raw = stripped
		}
	}
	parts := strings.Split(raw, ".")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + parts[len(parts)-1]
	}
	return parts[0]
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
