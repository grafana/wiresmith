package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/grafana/wiresmith/compiler/generator"
)

// stringSlice is a flag.Value that accumulates repeated -proto_path flags.
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	var protoPaths stringSlice
	flag.Var(&protoPaths, "proto_path", "directory containing .proto files (can be repeated; first is primary)")
	outDir := flag.String("out", "gen", "output directory for generated Go files")
	module := flag.String("module", "wiresmith", "Go module name")
	helpersImport := flag.String("helpers_import", "", "import path for protohelpers package (default: <module>/gen/protohelpers)")
	gogoCompat := flag.Bool("gogo_compat", false, "generate gogo protobuf compatibility (Reset, ProtoMessage, String, XXX_*, RegisterType, etc.)")
	flag.Parse()

	if len(protoPaths) == 0 {
		protoPaths = []string{"proto"}
	}

	g := &generator.Generator{
		Module:        *module,
		OutDir:        *outDir,
		ProtoPaths:    protoPaths,
		HelpersImport: *helpersImport,
		GogoCompat:    *gogoCompat,
	}

	if err := g.Generate(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
