# CLI reference

The `wiresmith` command compiles `.proto` files in a directory tree into Go packages of marshal/unmarshal/size code.

## Build

The module path is `wiresmith` (no host prefix), so the CLI is built from a checkout rather than installed via `go install`:

```sh
git clone git@github.com:grafana/wiresmith.git
cd wiresmith
go build -o wiresmith ./cmd/wiresmith
```

Inside the repo itself, `make generate-ours` regenerates everything under `gen/` and is the canonical entry point during development.

## Usage

```
wiresmith [flags] [files...]
```

When one or more `.proto` file paths are given as positional arguments,
only those files are emitted. Their imports are still resolved against the
full `--proto_path` walk, so cross-file references in the unemitted set
keep working. When no files are given, wiresmith walks `--proto_path` and
emits every `.proto` it finds (the default).

Positional paths must live under `--proto_path`; passing a `.proto` from
outside that tree is rejected so a typo doesn't silently produce an empty
generation run. This matches the positional-argument convention used by
`protoc`, `protoc-gen-go-vtproto`, and `protoc-gen-gogofast`.

## Flags

| Flag           | Default       | Description                                                |
|----------------|---------------|------------------------------------------------------------|
| `--proto_path` | `proto`       | Directory containing `.proto` files (walked recursively).  |
| `--out`        | `gen`         | Output directory for generated Go packages.                |
| `--module`     | `wiresmith`   | Go module name used as the prefix when emitting imports.   |
| `--version`    | _(boolean)_   | Print the build version and exit.                          |

The flag set is defined in [`cmd/wiresmith/main.go`](../cmd/wiresmith/main.go). `--version` falls back to `runtime/debug.ReadBuildInfo()` when `-ldflags "-X main.version=..."` is not set.

## Examples

Given a `.proto` tree like:

```
proto/
  example/v1/
    greeter.proto       # package example.v1; option go_package = "wiresmith/gen/example/v1";
    notes.proto         # package example.v1; option go_package = "wiresmith/gen/example/v1";
```

walk-and-emit-everything mode:

```sh
./wiresmith --proto_path=proto --out=gen --module=wiresmith
```

produces both `gen/example/v1/greeter.pb.go` and `gen/example/v1/notes.pb.go`,
importable as `wiresmith/gen/example/v1`.

Scoped mode emits only the listed file(s) while keeping the import graph:

```sh
./wiresmith --proto_path=proto --out=gen --module=wiresmith proto/example/v1/greeter.proto
```

produces only `gen/example/v1/greeter.pb.go`. Any imports from `greeter.proto`
into `notes.proto` (or any other file under `proto/`) still resolve, the
output file just isn't generated.

To opt a field into pointer-shaped codegen, import `wiresmith/options.proto` from the `.proto` source — see [extensions.md](extensions.md) for the option's effect and the worked example in [`proto/basic/pointer.proto`](../proto/basic/pointer.proto).
