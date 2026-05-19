package generator

import (
	"testing"
)

func TestParseGoPackage(t *testing.T) {
	tests := []struct {
		input      string
		wantPath   string
		wantPkgNam string
	}{
		{"path/to/pkg", "path/to/pkg", "pkg"},
		{"path/to/pkg;mypkg", "path/to/pkg", "mypkg"},
		{"wiresmith/gen/test/kitchensink/v1", "wiresmith/gen/test/kitchensink/v1", "v1"},
		{"go.opentelemetry.io/proto/otlp/common/v1", "go.opentelemetry.io/proto/otlp/common/v1", "v1"},
		{"", "", ""},
		// Empty semicolon name falls back to path.Base.
		{"path/to/pkg;", "path/to/pkg", "pkg"},
		// Dashes in path are sanitized to underscores.
		{"example.com/my-pkg", "example.com/my-pkg", "my_pkg"},
		{"example.com/my-pkg;clean", "example.com/my-pkg", "clean"},
		// Explicit semicolon pkgName is also sanitized — authors don't
		// always notice that '-' is invalid in Go identifiers.
		{"example.com/foo;my-name", "example.com/foo", "my_name"},
		// Go keywords are escaped wherever the pkgName comes from.
		{"example.com/foo;type", "example.com/foo", "type_"},
		{"example.com/foo/type", "example.com/foo/type", "type_"},
	}
	for _, tt := range tests {
		gotPath, gotName := parseGoPackage(tt.input)
		if gotPath != tt.wantPath || gotName != tt.wantPkgNam {
			t.Errorf("parseGoPackage(%q) = (%q, %q), want (%q, %q)",
				tt.input, gotPath, gotName, tt.wantPath, tt.wantPkgNam)
		}
	}
}

func TestCleanPackageName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"pkg", "pkg"},
		{"my-pkg", "my_pkg"},
		{"my.pkg", "my_pkg"},
		{"123pkg", "_23pkg"},
		{"", "_"},
		{"v1", "v1"},
		{"my_pkg", "my_pkg"},
		{"pkg-with-many-dashes", "pkg_with_many_dashes"},
		// Go keywords must be escaped — `package type` is a syntax error.
		{"type", "type_"},
		{"func", "func_"},
		{"package", "package_"},
		{"range", "range_"},
	}
	for _, tt := range tests {
		got := cleanPackageName(tt.input)
		if got != tt.want {
			t.Errorf("cleanPackageName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEffectiveBase(t *testing.T) {
	tests := []struct {
		module string
		want   string
	}{
		{"wiresmith", "wiresmith/gen"},
		{"example.com/mod", "example.com/mod/gen"},
		{"github.com/grafana/tempo", "github.com/grafana/tempo/gen"},
	}
	for _, tt := range tests {
		got := effectiveBase(tt.module)
		if got != tt.want {
			t.Errorf("effectiveBase(%q) = %q, want %q", tt.module, got, tt.want)
		}
	}
}

func TestResolveGoPackage(t *testing.T) {
	goPackages := map[string]string{
		"myapp.svc":     "example.com/app/gen/myapp/svc;service",
		"basic.maps.v1": "wiresmith/gen/basic/maps/v1",
		"external.pkg":  "some.other/module/pkg",
		"root.pkg":      "wiresmith/gen",
	}

	tests := []struct {
		name       string
		protoPkg   string
		base       string
		wantImport string
		wantDir    string
		wantPkg    string
		wantOk     bool
	}{
		{
			name:       "matches base with subdir",
			protoPkg:   "basic.maps.v1",
			base:       "wiresmith/gen",
			wantImport: "wiresmith/gen/basic/maps/v1",
			wantDir:    "basic/maps/v1",
			wantPkg:    "v1",
			wantOk:     true,
		},
		{
			name:       "semicolon overrides pkg name",
			protoPkg:   "myapp.svc",
			base:       "example.com/app/gen",
			wantImport: "example.com/app/gen/myapp/svc",
			wantDir:    "myapp/svc",
			wantPkg:    "service",
			wantOk:     true,
		},
		{
			name:     "outside base falls through",
			protoPkg: "external.pkg",
			base:     "wiresmith/gen",
			wantOk:   false,
		},
		{
			name:     "unknown package",
			protoPkg: "unknown.pkg",
			base:     "wiresmith/gen",
			wantOk:   false,
		},
		{
			name:       "exact base match",
			protoPkg:   "root.pkg",
			base:       "wiresmith/gen",
			wantImport: "wiresmith/gen",
			wantDir:    "",
			wantPkg:    "gen",
			wantOk:     true,
		},
		{
			name:     "base prefix without separator must not match",
			protoPkg: "external.pkg",
			// "some.other/module/pk" is a string prefix of "some.other/module/pkg"
			// but not a path-component prefix, so it must NOT match.
			base:   "some.other/module/pk",
			wantOk: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotImport, gotDir, gotPkg, gotOk := resolveGoPackage(tt.protoPkg, goPackages, tt.base)
			if gotOk != tt.wantOk {
				t.Fatalf("ok = %v, want %v", gotOk, tt.wantOk)
			}
			if !gotOk {
				return
			}
			if gotImport != tt.wantImport || gotDir != tt.wantDir || gotPkg != tt.wantPkg {
				t.Errorf("got (%q, %q, %q), want (%q, %q, %q)",
					gotImport, gotDir, gotPkg, tt.wantImport, tt.wantDir, tt.wantPkg)
			}
		})
	}
}

func TestDestFor(t *testing.T) {
	goPackages := map[string]string{
		"basic.maps.v1":                 "wiresmith/gen/basic/maps/v1",              // matches base
		"myapp.svc":                     "example.com/app/gen/myapp/svc;service",    // wrong base
		"external.pkg":                  "some.other/module/pkg",                    // outside base
		"opentelemetry.proto.common.v1": "go.opentelemetry.io/proto/otlp/common/v1", // outside base, OTel special-case applies
	}

	tests := []struct {
		name       string
		module     string
		protoPkg   string
		wantImport string
		wantRelDir string
		wantPkg    string
	}{
		{
			name:       "go_package under base",
			module:     "wiresmith",
			protoPkg:   "basic.maps.v1",
			wantImport: "wiresmith/gen/basic/maps/v1",
			wantRelDir: "basic/maps/v1",
			wantPkg:    "v1",
		},
		{
			name:       "go_package outside base falls back to default",
			module:     "wiresmith",
			protoPkg:   "external.pkg",
			wantImport: "wiresmith/gen/external/pkg",
			wantRelDir: "external/pkg",
			wantPkg:    "externalpkg",
		},
		{
			name:       "OTel special case in default mapping",
			module:     "wiresmith",
			protoPkg:   "opentelemetry.proto.common.v1",
			wantImport: "wiresmith/gen/otlp/common/v1",
			wantRelDir: "otlp/common/v1",
			wantPkg:    "commonv1",
		},
		{
			name:       "no go_package, default mapping",
			module:     "testmod",
			protoPkg:   "x.y.v1",
			wantImport: "testmod/gen/x/y/v1",
			wantRelDir: "x/y/v1",
			wantPkg:    "yv1",
		},
		{
			name:       "go_package matches different module's base",
			module:     "example.com/app",
			protoPkg:   "myapp.svc",
			wantImport: "example.com/app/gen/myapp/svc",
			wantRelDir: "myapp/svc",
			wantPkg:    "service",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := destFor(tt.module, tt.protoPkg, goPackages)
			if got.importPath != tt.wantImport ||
				got.relDir != tt.wantRelDir ||
				got.pkgName != tt.wantPkg {
				t.Errorf("destFor(%q, %q) = {%q, %q, %q}, want {%q, %q, %q}",
					tt.module, tt.protoPkg,
					got.importPath, got.relDir, got.pkgName,
					tt.wantImport, tt.wantRelDir, tt.wantPkg)
			}
		})
	}
}

func TestUniqueAlias(t *testing.T) {
	it := newImportTracker("mod", "self.pkg", nil)
	it.addImport("example.com/a/v1", "v1")
	it.addImport("example.com/b/v1", "v1_other")

	// First collision: numeric suffix starts at 1.
	got := it.uniqueAlias("v1", "example.com/c/v1", "selfName")
	if got != "v11" {
		t.Errorf("first collision: got %q, want %q", got, "v11")
	}

	// Self-name collision also triggers suffixing.
	got = it.uniqueAlias("selfName", "example.com/d", "selfName")
	if got != "selfName1" {
		t.Errorf("self-name collision: got %q, want %q", got, "selfName1")
	}

	// No collision: pass through unchanged.
	got = it.uniqueAlias("fresh", "example.com/e", "selfName")
	if got != "fresh" {
		t.Errorf("no collision: got %q, want %q", got, "fresh")
	}
}

func TestAliasInUseEmpty(t *testing.T) {
	it := newImportTracker("mod", "self.pkg", nil)
	// Several imports registered with the "use natural name" sentinel (empty
	// alias). aliasInUse must not treat them as a single occupied slot —
	// otherwise the second empty-alias caller would falsely be told the
	// alias is taken.
	it.addImport("std/lib/a", "")
	it.addImport("std/lib/b", "")
	if it.aliasInUse("", "std/lib/c") {
		t.Error("aliasInUse(\"\", ...) must report false; empty alias is a sentinel, not a real name")
	}
}

func TestAddImportIdempotent(t *testing.T) {
	it := newImportTracker("mod", "self.pkg", nil)

	got := it.addImport("example.com/pkg/v1", "v1")
	if got != "v1" {
		t.Errorf("first addImport: got %q, want %q", got, "v1")
	}

	// Second call with the same path returns the cached alias and ignores
	// the new value — emitter helpers can register a path more than once
	// without coordinating.
	got = it.addImport("example.com/pkg/v1", "different")
	if got != "v1" {
		t.Errorf("second addImport: got %q, want %q (should return cached)", got, "v1")
	}

	// Count requested entries; pre-reserved stdlib entries (created in
	// newImportTracker so the alias pool knows them) don't count here.
	requested := 0
	for _, e := range it.imports {
		if e.requested {
			requested++
		}
	}
	if requested != 1 {
		t.Errorf("expected 1 requested import, got %d", requested)
	}
}
