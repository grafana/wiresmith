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

	if len(it.imports) != 1 {
		t.Errorf("expected 1 import, got %d", len(it.imports))
	}
}
