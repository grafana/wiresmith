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

func TestSourceRelDir(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"foo.proto", ""},
		{"a/foo.proto", "a"},
		{"a/b/c/foo.proto", "a/b/c"},
	}
	for _, tt := range tests {
		got := sourceRelDir(tt.input)
		if got != tt.want {
			t.Errorf("sourceRelDir(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestJoinImport(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  string
	}{
		{"all parts", []string{"wiresmith", "gen", "otlp/common/v1"}, "wiresmith/gen/otlp/common/v1"},
		{"empty middle", []string{"wiresmith", "", "otlp"}, "wiresmith/otlp"},
		{"empty trailing relDir", []string{"wiresmith", "gen", ""}, "wiresmith/gen"},
		{"leading/trailing slashes trimmed", []string{"/wiresmith/", "/gen/", "/otlp/"}, "wiresmith/gen/otlp"},
		{"all empty", []string{"", ""}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinImport(tt.parts...); got != tt.want {
				t.Errorf("joinImport(%v) = %q, want %q", tt.parts, got, tt.want)
			}
		})
	}
}

func TestDestForPath(t *testing.T) {
	goPackages := map[string]string{
		// Under module/gen — same destination as the default.
		"basic.maps.v1": "wiresmith/gen/basic/maps/v1",
		// `;name` form lets the proto author pick a package name independent
		// of the import path's basename.
		"myapp.svc": "wiresmith/gen/myapp/svc;service",
		// Honored under a non-gen outDir too — exercises the outDir-composes case.
		"tempo.svc": "github.com/grafana/tempo/pkg/tempopb/svc",
		// Outside module/gen — honored literally (no base gate).
		"opentelemetry.proto.common.v1": "go.opentelemetry.io/proto/otlp/common/v1",
		// File whose go_package will be M-overridden in a test below.
		"vendor.otel.common.v1": "go.opentelemetry.io/proto/otlp/common/v1",
	}
	overrides := map[string]string{
		// Override beats go_package — matches protoc's M-flag precedence.
		"opentelemetry/proto/common/v1/common.proto": "wiresmith/gen/opentelemetry/proto/common/v1",
		// Override supplies `;name` form for a file that has no go_package.
		"no_pkg/no_pkg.proto": "example.com/redirect;chosen",
	}

	tests := []struct {
		name       string
		module     string
		outDir     string
		fdPath     string
		protoPkg   string
		wantImport string
		wantRelDir string
		wantPkg    string
	}{
		{
			name:       "go_package matching default — honored",
			module:     "wiresmith",
			outDir:     "gen",
			fdPath:     "basic/maps/v1/maps.proto",
			protoPkg:   "basic.maps.v1",
			wantImport: "wiresmith/gen/basic/maps/v1",
			wantRelDir: "basic/maps/v1",
			wantPkg:    "v1",
		},
		{
			name:       "go_package `;name` form is honored",
			module:     "wiresmith",
			outDir:     "gen",
			fdPath:     "myapp/svc/svc.proto",
			protoPkg:   "myapp.svc",
			wantImport: "wiresmith/gen/myapp/svc",
			wantRelDir: "myapp/svc",
			wantPkg:    "service",
		},
		{
			name:       "multi-segment outDir composes into the base, honoring go_package under it",
			module:     "github.com/grafana/tempo",
			outDir:     "pkg/tempopb",
			fdPath:     "svc/svc.proto",
			protoPkg:   "tempo.svc",
			wantImport: "github.com/grafana/tempo/pkg/tempopb/svc",
			wantRelDir: "svc",
			wantPkg:    "svc",
		},
		{
			name:       "go_package outside module/gen — now honored literally",
			module:     "wiresmith",
			outDir:     "gen",
			fdPath:     "vendor/otel/common/v1/common.proto",
			protoPkg:   "vendor.otel.common.v1",
			wantImport: "go.opentelemetry.io/proto/otlp/common/v1",
			wantRelDir: "vendor/otel/common/v1",
			wantPkg:    "v1",
		},
		{
			name:       "M-override beats go_package",
			module:     "wiresmith",
			outDir:     "gen",
			fdPath:     "opentelemetry/proto/common/v1/common.proto",
			protoPkg:   "opentelemetry.proto.common.v1",
			wantImport: "wiresmith/gen/opentelemetry/proto/common/v1",
			wantRelDir: "opentelemetry/proto/common/v1",
			wantPkg:    "v1",
		},
		{
			name:       "M-override `;name` form on file with no go_package",
			module:     "wiresmith",
			outDir:     "gen",
			fdPath:     "no_pkg/no_pkg.proto",
			protoPkg:   "no.pkg",
			wantImport: "example.com/redirect",
			wantRelDir: "no_pkg",
			wantPkg:    "chosen",
		},
		{
			name:       "no go_package, no override — default mapping",
			module:     "testmod",
			outDir:     "gen",
			fdPath:     "x/y/v1/foo.proto",
			protoPkg:   "x.y.v1",
			wantImport: "testmod/gen/x/y/v1",
			wantRelDir: "x/y/v1",
			wantPkg:    "yv1",
		},
		{
			name:       "non-gen outDir flows into the import base too",
			module:     "github.com/grafana/tempo",
			outDir:     "pkg/tempopb",
			fdPath:     "common/v1/common.proto",
			protoPkg:   "tempo.common.v1",
			wantImport: "github.com/grafana/tempo/pkg/tempopb/common/v1",
			wantRelDir: "common/v1",
			wantPkg:    "commonv1",
		},
		{
			name:       "flat file (no source dir) lands at the import base",
			module:     "wiresmith",
			outDir:     "gen",
			fdPath:     "root.proto",
			protoPkg:   "root",
			wantImport: "wiresmith/gen",
			wantRelDir: "",
			wantPkg:    "root",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := destForPath(tt.module, tt.outDir, tt.fdPath, tt.protoPkg, goPackages, overrides)
			if got.importPath != tt.wantImport ||
				got.relDir != tt.wantRelDir ||
				got.pkgName != tt.wantPkg {
				t.Errorf("destForPath(%q, %q, %q, %q) = {%q, %q, %q}, want {%q, %q, %q}",
					tt.module, tt.outDir, tt.fdPath, tt.protoPkg,
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
