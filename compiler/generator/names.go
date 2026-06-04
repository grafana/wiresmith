package generator

import (
	"path"
	"path/filepath"
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

// sourceRelDir returns the directory part of fdPath in forward-slash form.
// fdPath is the canonical import key from buildImportMapping — either the
// on-disk path relative to --proto_path (for nested files) or the
// package-as-path form (for flat files). Either way, dropping the trailing
// basename gives the source-relative directory the `paths=source_relative`
// contract writes under.
func sourceRelDir(fdPath string) string {
	dir := filepath.ToSlash(filepath.Dir(fdPath))
	if dir == "." {
		return ""
	}
	return dir
}

// joinImport concatenates Go import-path segments with "/", trimming any
// leading/trailing slashes so empty segments don't produce double slashes.
func joinImport(parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		p = strings.Trim(p, "/")
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, "/")
}

// goDest is the canonical Go destination for a proto file — every consumer
// (output paths, package clauses, cross-file import resolution, collision
// detection) reads from here so they can't disagree.
type goDest struct {
	importPath string // full Go import path of the destination
	relDir     string // path relative to OutDir
	pkgName    string // declared `package` clause in the generated file

	// protoPkg is the source `package` clause of the .proto file this
	// destination was derived from. Carried alongside the Go-side fields
	// so the import-alias collision fallback has a stable proto-pkg-
	// derived alternative to try without re-reading the FileDescriptor.
	protoPkg string
}

// destFor returns the canonical destination for fd. The on-disk directory is
// purely source-relative — filepath.Dir(fd.Path()) under the configured
// --out, matching the `paths=source_relative` contract — regardless of what
// go_package says.
//
// The Go import path follows protoc-gen-go's resolution order:
//
//  1. An entry in g.Overrides (set via the CLI's `--M source=dest` flag,
//     keyed by fd.Path()) wins. Matches protoc's `M<source>=<destpath>`
//     option: an out-of-tree go_package can be redirected without editing
//     the .proto.
//  2. The file's `option go_package` is honored literally — including its
//     `;name` suffix — with no "is it under the module base" gate.
//  3. The default `<module>/<outDir>/<source-relative>` applies when
//     neither of the above is set; the on-disk write path and the
//     declared import path agree under Go's directory-equals-import-path
//     rule.
//
// Production sites all live behind a *Generator (Module + OutDir +
// goPackages + Overrides are already in scope). Unit tests reach for
// destForPath instead to bypass the FileDescriptor construction overhead.
func (g *Generator) destFor(fd protoreflect.FileDescriptor) goDest {
	return destForPath(g.Module, g.OutDir, fd.Path(), string(fd.Package()), g.goPackages, g.Overrides)
}

// destForPath is the string-only variant of destFor — broken out so unit
// tests can drive the resolver without constructing a FileDescriptor.
func destForPath(module, outDir, fdPath, protoPkg string, goPackages, overrides map[string]string) goDest {
	relDir := sourceRelDir(fdPath)
	importPath := joinImport(module, outDir, relDir)
	pkgName := goPackageName(protoPkg)
	// The override key is fdPath (the import-mapping key produced by
	// buildImportMapping), which is also the path users see in import
	// statements. The same key shape protoc consumes via `Mkey=value`.
	if override, ok := overrides[fdPath]; ok && override != "" {
		importPath, pkgName = parseGoPackage(override)
	} else if goPkg, ok := goPackages[protoPkg]; ok && goPkg != "" {
		importPath, pkgName = parseGoPackage(goPkg)
	}
	return goDest{importPath: importPath, relDir: relDir, pkgName: pkgName, protoPkg: protoPkg}
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
