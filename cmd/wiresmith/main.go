package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"wiresmith/compiler/generator"
)

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
  wiresmith [flags]

Flags:
`)
		flag.PrintDefaults()
	}

	protoDir := flag.String("proto_path", "proto", "directory containing .proto files")
	outDir := flag.String("out", "gen", "output directory for generated Go files")
	module := flag.String("module", "wiresmith", "Go module name")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(buildVersion())
		return
	}

	g := &generator.Generator{
		Module:   *module,
		OutDir:   *outDir,
		ProtoDir: *protoDir,
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
