package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"wiresmith/compiler/generator"
)

func main() {
	protoDir := flag.String("proto_path", "proto", "directory containing .proto files")
	outDir := flag.String("out", "gen", "output directory for generated Go files")
	module := flag.String("module", "wiresmith", "Go module name")
	stripPrefix := flag.String("strip_prefix", "", "proto package prefix to strip for Go paths")
	importBase := flag.String("import_base", "", "Go import path base (replaces module/gen/ prefix)")
	helpersImport := flag.String("helpers_import", "", "import path for protohelpers package")
	flag.Parse()

	g := &generator.Generator{
		Module:        *module,
		OutDir:        *outDir,
		ProtoDir:      *protoDir,
		StripPrefix:   *stripPrefix,
		ImportBase:    *importBase,
		HelpersImport: *helpersImport,
	}

	if err := g.Generate(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
