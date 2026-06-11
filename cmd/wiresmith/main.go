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

// protoPathsFlag implements flag.Value for the repeatable `--proto_path`
// (and `-I` alias) option. Each occurrence appends one or more roots to
// the underlying slice; a single value may carry list-separated entries
// (`--proto_path=a:b:c` on Unix, `--proto_path=a;b;c` on Windows) using
// os.PathListSeparator so Windows drive-letter paths like `C:\proto`
// stay intact. Order is preserved — it doesn't affect import resolution
// (collisions are errors, not first-wins) but it does show up in error
// messages.
//
// The slice is seeded with the historical "proto" default so flag.PrintDefaults
// (`-h`) shows it; the first user-supplied occurrence replaces that seed rather
// than appending to it, tracked via `userSet`.
type protoPathsFlag struct {
	dirs    []string
	userSet bool
}

func (p *protoPathsFlag) String() string {
	if p == nil || len(p.dirs) == 0 {
		return ""
	}
	return strings.Join(p.dirs, string(os.PathListSeparator))
}

func (p *protoPathsFlag) Set(v string) error {
	if v == "" {
		return fmt.Errorf("--proto_path: empty value")
	}
	if !p.userSet {
		// Drop the seeded default so the user's roots fully replace it.
		p.dirs = nil
		p.userSet = true
	}
	for part := range strings.SplitSeq(v, string(os.PathListSeparator)) {
		if part == "" {
			return fmt.Errorf("--proto_path: empty entry in %q", v)
		}
		p.dirs = append(p.dirs, part)
	}
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
every --proto_path root and emits every .proto it finds (the default).

--proto_path (-I) may be given multiple times to compile across several
roots, matching 'protoc -I=root1 -I=root2'. A single occurrence may
carry list-separated entries using the OS path-list separator (':' on
Unix, ';' on Windows). If the same import key is produced by two
different files across the roots, wiresmith fails loudly with both
paths rather than silently letting one shadow the other.

Positional .proto paths must live under some --proto_path root; a path
that points outside every walked tree is rejected up front so a typo
can't silently produce an empty generation run.

Flags:
`)
		flag.PrintDefaults()
	}

	// Seed with the historical default ("proto") so `-h` reports it; the first
	// user-supplied --proto_path/-I replaces it (see protoPathsFlag.Set).
	protoPaths := &protoPathsFlag{dirs: []string{"proto"}}
	flag.Var(protoPaths, "proto_path",
		`directory containing .proto files (walked recursively). Repeatable; a single occurrence may carry list-separated entries (':' on Unix, ';' on Windows, via os.PathListSeparator). Defaults to "proto" when omitted.`)
	flag.Var(protoPaths, "I", `alias for --proto_path.`)
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
		ProtoDirs: protoPaths.dirs,
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
