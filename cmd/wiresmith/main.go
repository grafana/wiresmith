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
	flag.Parse()

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
