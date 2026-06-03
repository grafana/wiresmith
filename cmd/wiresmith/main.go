package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/grafana/wiresmith/compiler/generator"
)

// overridesFlag implements flag.Value for the repeatable `-M src=dest`
// option. Each occurrence registers one source→Go-import-path mapping in
// the underlying map; the destination string may carry an optional
// `;name` suffix matching go_package syntax. Mirrors protoc's
// `M<source>=<destpath>` convention.
type overridesFlag struct{ m map[string]string }

func (o *overridesFlag) String() string {
	if o == nil || len(o.m) == 0 {
		return ""
	}
	var parts []string
	for k, v := range o.m {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func (o *overridesFlag) Set(v string) error {
	i := strings.Index(v, "=")
	if i <= 0 || i == len(v)-1 {
		return fmt.Errorf("-M expects source=dest, got %q", v)
	}
	src, dest := v[:i], v[i+1:]
	if _, dup := o.m[src]; dup {
		return fmt.Errorf("-M source %q given more than once", src)
	}
	o.m[src] = dest
	return nil
}

// version is overridden at build time via -ldflags "-X main.version=...".
// When unset, buildVersion falls back to runtime/debug build info so
// `go install` produces something meaningful too.
var version = ""

func main() {
	flag.Usage = func() {
		// Write to os.Stderr (the flag package's default output) rather than
		// flag.CommandLine.Output(): errcheck's default exclude list covers
		// fmt.Fprint(os.Stderr, ...) by type, but not the io.Writer return.
		fmt.Fprint(os.Stderr, `wiresmith — generate high-performance Go marshal/unmarshal code from .proto files.

Usage:
  wiresmith [flags] [files...]

When one or more .proto file paths are given as positional arguments,
only those files are emitted. Their imports are still resolved against
the full --proto_path walk. When no files are given, wiresmith walks
--proto_path and emits every .proto it finds (the default).

Positional .proto paths must live under --proto_path; a path that
points outside the walked tree is rejected up front so a typo can't
silently produce an empty generation run.

Flags:
`)
		flag.PrintDefaults()
	}

	protoDir := flag.String("proto_path", "proto", "directory containing .proto files")
	outDir := flag.String("out", "gen", "output directory for generated Go files")
	module := flag.String("module", "github.com/grafana/wiresmith", "Go module name")
	showVersion := flag.Bool("version", false, "print version and exit")
	overrides := &overridesFlag{m: map[string]string{}}
	flag.Var(overrides, "M",
		`override the Go import path for one .proto file (repeatable). Format: -M source=destpath[;name]. The source key matches the file's import-mapping key (the path used in 'import' statements); the destination wins over the file's own option go_package, mirroring protoc's M-flag semantics. Useful for vendored .protos whose go_package points outside the consumer's tree.`)
	flag.Parse()

	if *showVersion {
		fmt.Println(buildVersion())
		return
	}

	g := &generator.Generator{
		Module:    *module,
		OutDir:    *outDir,
		ProtoDir:  *protoDir,
		Files:     flag.Args(),
		Overrides: overrides.m,
	}

	if err := g.Generate(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "(devel)"
}
