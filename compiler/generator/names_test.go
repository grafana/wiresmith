package generator

import (
	"testing"
)

func TestGoPackageName(t *testing.T) {
	tests := []struct {
		protoPkg    string
		stripPrefix string
		want        string
	}{
		{"opentelemetry.proto.common.v1", "", "commonv1"},
		{"com.example.v1", "", "examplev1"},
		{"mypkg", "", "mypkg"},
		// Strip prefix: last component only.
		{"tempopb.common.v1", "tempopb", "v1"},
		{"tempopb.trace.v1", "tempopb", "v1"},
		{"tempopb.v2", "tempopb", "v2"},
		// Prefix doesn't match: fall back to default.
		{"other.common.v1", "tempopb", "commonv1"},
	}
	for _, tt := range tests {
		got := goPackageName(tt.protoPkg, tt.stripPrefix)
		if got != tt.want {
			t.Errorf("goPackageName(%q, %q) = %q, want %q", tt.protoPkg, tt.stripPrefix, got, tt.want)
		}
	}
}

func TestGoPackageDir(t *testing.T) {
	tests := []struct {
		protoPkg    string
		stripPrefix string
		want        string
	}{
		// OTel special case preserved.
		{"opentelemetry.proto.common.v1", "", "otlp/common/v1"},
		{"opentelemetry.proto.trace.v1", "", "otlp/trace/v1"},
		// General default.
		{"com.example.v1", "", "com/example/v1"},
		{"test.kitchensink.v1", "", "test/kitchensink/v1"},
		// Strip prefix.
		{"tempopb.common.v1", "tempopb", "common/v1"},
		{"tempopb.trace.v1", "tempopb", "trace/v1"},
		// Strip prefix takes priority over OTel special case.
		{"opentelemetry.proto.common.v1", "opentelemetry.proto", "common/v1"},
		// Prefix doesn't match: fall back to default.
		{"other.common.v1", "tempopb", "other/common/v1"},
	}
	for _, tt := range tests {
		got := goPackageDir(tt.protoPkg, tt.stripPrefix)
		if got != tt.want {
			t.Errorf("goPackageDir(%q, %q) = %q, want %q", tt.protoPkg, tt.stripPrefix, got, tt.want)
		}
	}
}

func TestGoImportPath(t *testing.T) {
	tests := []struct {
		module      string
		protoPkg    string
		stripPrefix string
		importBase  string
		want        string
	}{
		// Default with OTel.
		{"wiresmith", "opentelemetry.proto.common.v1", "", "", "wiresmith/gen/otlp/common/v1"},
		// Default general.
		{"wiresmith", "test.kitchensink.v1", "", "", "wiresmith/gen/test/kitchensink/v1"},
		// Custom import base with strip prefix.
		{
			"github.com/grafana/tempo", "tempopb.common.v1", "tempopb",
			"github.com/grafana/tempo/pkg/tempopb",
			"github.com/grafana/tempo/pkg/tempopb/common/v1",
		},
		// Import base without strip prefix.
		{
			"github.com/grafana/tempo", "tempopb.common.v1", "",
			"github.com/grafana/tempo/pkg/tempopb",
			"github.com/grafana/tempo/pkg/tempopb/tempopb/common/v1",
		},
		// Strip prefix without import base — stripPrefix must be applied.
		{
			"mymod", "tempopb.common.v1", "tempopb", "",
			"mymod/gen/common/v1",
		},
	}
	for _, tt := range tests {
		got := goImportPath(tt.module, tt.protoPkg, tt.stripPrefix, tt.importBase)
		if got != tt.want {
			t.Errorf("goImportPath(%q, %q, %q, %q) = %q, want %q",
				tt.module, tt.protoPkg, tt.stripPrefix, tt.importBase, got, tt.want)
		}
	}
}

func TestHelpersImportPath(t *testing.T) {
	tests := []struct {
		module        string
		helpersImport string
		want          string
	}{
		{"wiresmith", "", "wiresmith/gen/protohelpers"},
		{"wiresmith", "github.com/grafana/tempo/pkg/tempopb/protohelpers", "github.com/grafana/tempo/pkg/tempopb/protohelpers"},
	}
	for _, tt := range tests {
		got := helpersImportPath(tt.module, tt.helpersImport)
		if got != tt.want {
			t.Errorf("helpersImportPath(%q, %q) = %q, want %q", tt.module, tt.helpersImport, got, tt.want)
		}
	}
}

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
	}
	for _, tt := range tests {
		gotPath, gotName := parseGoPackage(tt.input)
		if gotPath != tt.wantPath || gotName != tt.wantPkgNam {
			t.Errorf("parseGoPackage(%q) = (%q, %q), want (%q, %q)",
				tt.input, gotPath, gotName, tt.wantPath, tt.wantPkgNam)
		}
	}
}

func TestDisambiguateAlias(t *testing.T) {
	tests := []struct {
		protoPkg    string
		stripPrefix string
		want        string
	}{
		// With strip prefix, use second-to-last + last component.
		{"tempopb.common.v1", "tempopb", "commonv1"},
		{"tempopb.trace.v1", "tempopb", "tracev1"},
		// Without strip prefix.
		{"com.example.v1", "", "examplev1"},
		// Single component after strip.
		{"tempopb.v1", "tempopb", "v1"},
	}
	for _, tt := range tests {
		got := disambiguateAlias(tt.protoPkg, tt.stripPrefix)
		if got != tt.want {
			t.Errorf("disambiguateAlias(%q, %q) = %q, want %q", tt.protoPkg, tt.stripPrefix, got, tt.want)
		}
	}
}

func TestEffectiveBase(t *testing.T) {
	tests := []struct {
		module     string
		importBase string
		want       string
	}{
		{"wiresmith", "", "wiresmith/gen"},
		{"wiresmith", "github.com/grafana/tempo/pkg/tempopb", "github.com/grafana/tempo/pkg/tempopb"},
		{"example.com/mod", "", "example.com/mod/gen"},
	}
	for _, tt := range tests {
		got := effectiveBase(tt.module, tt.importBase)
		if got != tt.want {
			t.Errorf("effectiveBase(%q, %q) = %q, want %q", tt.module, tt.importBase, got, tt.want)
		}
	}
}

func TestResolveGoPackage(t *testing.T) {
	goPackages := map[string]string{
		"tempopb.common.v1": "github.com/grafana/tempo/pkg/tempopb/common/v1",
		"myapp.svc":         "example.com/app/gen/myapp/svc;service",
		"external.pkg":      "some.other/module/pkg",
	}

	tests := []struct {
		protoPkg   string
		base       string
		wantImport string
		wantDir    string
		wantPkg    string
		wantOk     bool
	}{
		// Matches base.
		{"tempopb.common.v1", "github.com/grafana/tempo/pkg/tempopb", "github.com/grafana/tempo/pkg/tempopb/common/v1", "common/v1", "v1", true},
		// Semicolon form.
		{"myapp.svc", "example.com/app/gen", "example.com/app/gen/myapp/svc", "myapp/svc", "service", true},
		// Doesn't match base.
		{"external.pkg", "wiresmith/gen", "", "", "", false},
		// Not in map.
		{"unknown.pkg", "wiresmith/gen", "", "", "", false},
		// Exact base match (proto at root of import base).
		{"external.pkg", "some.other/module/pkg", "some.other/module/pkg", "", "pkg", true},
	}
	for _, tt := range tests {
		gotImport, gotDir, gotPkg, gotOk := resolveGoPackage(tt.protoPkg, goPackages, tt.base)
		if gotImport != tt.wantImport || gotDir != tt.wantDir || gotPkg != tt.wantPkg || gotOk != tt.wantOk {
			t.Errorf("resolveGoPackage(%q, ..., %q) = (%q, %q, %q, %v), want (%q, %q, %q, %v)",
				tt.protoPkg, tt.base, gotImport, gotDir, gotPkg, gotOk,
				tt.wantImport, tt.wantDir, tt.wantPkg, tt.wantOk)
		}
	}
}

func TestAddImportIdempotent(t *testing.T) {
	it := newImportTracker("mod", "self.pkg", "", "", "", nil)

	// First call registers the alias.
	got := it.addImport("example.com/pkg/v1", "v1")
	if got != "v1" {
		t.Errorf("first addImport: got %q, want %q", got, "v1")
	}

	// Second call with same path returns the cached alias, ignoring the new one.
	got = it.addImport("example.com/pkg/v1", "different")
	if got != "v1" {
		t.Errorf("second addImport: got %q, want %q (should return cached)", got, "v1")
	}

	if len(it.imports) != 1 {
		t.Errorf("expected 1 import, got %d", len(it.imports))
	}
}

func TestUniqueAliasNumericSuffix(t *testing.T) {
	it := newImportTracker("mod", "self.pkg", "", "", "", nil)

	// Occupy "commonv1" with a different path.
	it.addImport("example.com/a/common/v1", "commonv1")

	// uniqueAlias should add numeric suffix to avoid collision.
	got := it.uniqueAlias("commonv1", "example.com/b/common/v1", "selfpkg")
	if got != "commonv11" {
		t.Errorf("uniqueAlias: got %q, want %q", got, "commonv11")
	}

	// Occupy "commonv11" too.
	it.addImport("example.com/b/common/v1", "commonv11")

	// Third collision should get "commonv12".
	got = it.uniqueAlias("commonv1", "example.com/c/common/v1", "selfpkg")
	if got != "commonv12" {
		t.Errorf("uniqueAlias (second collision): got %q, want %q", got, "commonv12")
	}
}

func TestUniqueAliasSelfNameCollision(t *testing.T) {
	it := newImportTracker("mod", "self.pkg", "", "", "", nil)

	// Alias equals selfName — should get numeric suffix.
	got := it.uniqueAlias("v1", "example.com/common/v1", "v1")
	if got != "v11" {
		t.Errorf("uniqueAlias with selfName collision: got %q, want %q", got, "v11")
	}
}
