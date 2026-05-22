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

## Flags

| Flag           | Default       | Description                                                |
|----------------|---------------|------------------------------------------------------------|
| `--proto_path` | `proto`       | Directory containing `.proto` files (walked recursively).  |
| `--out`        | `gen`         | Output directory for generated Go packages.                |
| `--module`     | `wiresmith`   | Go module name used as the prefix when emitting imports.   |
| `--version`    | _(boolean)_   | Print the build version and exit.                          |

The flag set is defined in [`cmd/wiresmith/main.go`](../cmd/wiresmith/main.go). `--version` falls back to `runtime/debug.ReadBuildInfo()` when `-ldflags "-X main.version=..."` is not set.

## Example

Given a `.proto` tree like:

```
proto/
  example/v1/
    greeter.proto       # package example.v1; option go_package = "wiresmith/gen/example/v1";
```

run:

```sh
./wiresmith --proto_path=proto --out=gen --module=wiresmith
```

The resulting `gen/example/v1/greeter.pb.go` is importable as `wiresmith/gen/example/v1`.

To opt a field into pointer-shaped codegen, import `wiresmith/options.proto` from the `.proto` source — see [extensions.md](extensions.md) for the option's effect and the worked example in [`proto/basic/pointer.proto`](../proto/basic/pointer.proto).
