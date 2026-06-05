// Command protoc-gen-wiresmith is the protoc / buf-compatible plugin entry
// point for wiresmith. Functionally it's a thin shim: it converts the
// CodeGeneratorRequest into the descriptor set the generator already
// understands, calls (*generator.Generator).GenerateFromDescriptors, and
// hands the resulting files back via protogen.Plugin.NewGeneratedFile.
//
// All real work lives in compiler/generator. Bug reports about the
// generated code belong there.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/grafana/wiresmith/compiler/generator"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/pluginpb"
)

// version is overridden at build time via -ldflags "-X main.version=...".
// Falls back to runtime/debug build info so `go install` still produces
// something meaningful.
var version = ""

// pluginOpts collects the parsed plugin parameters. Materialised here
// (rather than as closure-captured locals in main) so the inner Run can
// be exercised from tests without going through stdin/stdout.
type pluginOpts struct {
	module    string
	overrides map[string]string
}

// paramFunc parses one `--wiresmith_opt=<name>=<value>` pair. protogen
// already split the pair on `=`, so values containing `=` (like
// `go_package`-style `dest;name` suffixes) arrive intact.
//
// `M<source>=<dest>` import overrides take a special path: protoc / buf
// users expect `Mfoo/bar.proto=example.com/x` to set source="foo/bar.proto"
// → dest="example.com/x" — matching protoc-gen-go. Routing it through
// flag.FlagSet would fail because protogen passes name=`Mfoo/bar.proto`
// and there's no flag of that name. Matching the prefix here is simpler
// than a one-off flag.Value implementation.
func (o *pluginOpts) paramFunc(flags *flag.FlagSet) func(name, value string) error {
	return func(name, value string) error {
		if strings.HasPrefix(name, "M") {
			src := strings.TrimPrefix(name, "M")
			if src == "" {
				return fmt.Errorf("-M requires a non-empty source key (got %q)", name+"="+value)
			}
			if _, dup := o.overrides[src]; dup {
				return fmt.Errorf("-M source %q given more than once", src)
			}
			o.overrides[src] = value
			return nil
		}
		return flags.Set(name, value)
	}
}

func main() {
	// Surface a non-protoc `--version` invocation (`protoc-gen-wiresmith
	// --version`) before handing stdin to protogen — convenient for
	// diagnosing which build of the plugin lives on a developer's PATH.
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(buildVersion())
		return
	}

	opts := pluginOpts{overrides: map[string]string{}}

	flags := flag.NewFlagSet("protoc-gen-wiresmith", flag.ContinueOnError)
	flags.StringVar(&opts.module, "module", "",
		`Go module path used as a fallback when a .proto file omits "option go_package".`)

	protogen.Options{ParamFunc: opts.paramFunc(flags)}.Run(func(plugin *protogen.Plugin) error {
		// Tell protoc we understand proto3 `optional` fields — wiresmith
		// emits Has*() and a presence bitmap for them. Without this flag
		// protoc rejects any .proto in the request that uses proto3
		// optional before invoking us. We do not advertise SUPPORTS_EDITIONS
		// — wiresmith is proto3-only by design (docs/design.md).
		plugin.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		if err := run(plugin, opts); err != nil {
			plugin.Error(err)
		}
		return nil
	})
}

// run translates the protogen Plugin into the generator API and writes the
// generator's outputs back through plugin.NewGeneratedFile. Split out from
// main so the unit tests can drive it with a synthetic protogen.Plugin.
func run(plugin *protogen.Plugin, opts pluginOpts) error {
	var (
		files []protoreflect.FileDescriptor
		emit  = map[string]bool{}
	)
	for _, f := range plugin.Files {
		files = append(files, f.Desc)
		if f.Generate {
			emit[f.Desc.Path()] = true
		}
	}
	if len(emit) == 0 {
		// Nothing requested — succeed silently, matching protoc-gen-go's
		// behaviour when invoked with an empty FilesToGenerate.
		return nil
	}

	gen := &generator.Generator{
		Module:    opts.module,
		OutDir:    "", // paths are source-relative; buf places them under its own out:
		Overrides: opts.overrides,
	}

	outputs, err := gen.GenerateFromDescriptors(context.Background(), files, emit)
	if err != nil {
		return err
	}

	for _, o := range outputs {
		gf := plugin.NewGeneratedFile(o.Path, "")
		if _, err := gf.Write(o.Content); err != nil {
			return fmt.Errorf("writing %s: %w", o.Path, err)
		}
	}
	return nil
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
