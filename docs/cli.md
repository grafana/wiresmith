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
| `-M`           | _(repeatable)_| Override the Go import path for one `.proto` (see below).  |
| `--version`    | _(boolean)_   | Print the build version and exit.                          |

The flag set is defined in [`cmd/wiresmith/main.go`](../cmd/wiresmith/main.go). `--version` falls back to `runtime/debug.ReadBuildInfo()` when `-ldflags "-X main.version=..."` is not set.

### Import-path resolution

The Go import path of a generated file is resolved in this order, matching `protoc-gen-go`:

1. **`-M source=destpath[;name]`** (highest precedence). The source key is the file's import-mapping key — the same path that appears in `import` statements between `.proto` files. Mirrors `protoc`'s `M<source>=<dest>` semantics.
2. **`option go_package`** in the `.proto`. Honored literally — including the `;name` suffix.
3. **Default**: `<module>/<out>/<source-relative path>`, where the source-relative path is `fd.Path()` minus the basename.

The on-disk write location is always `<out>/<source-relative path>` regardless of which step above produced the import path, matching `paths=source_relative`. `-M` and `go_package` only influence the import-path string in the generated file, not where the file is written.

`-M` is the documented escape hatch when a vendored `.proto` declares a `go_package` that doesn't match the consumer's tree — for example wiresmith's own OTel build, where the upstream `go_package = "go.opentelemetry.io/proto/otlp/..."` is overridden to `github.com/grafana/wiresmith/gen/opentelemetry/proto/...;...` so the generated imports resolve under the local module.

An `-M`-pinned file is also exempt from the one-`go_package`-per-proto-package consistency check, so `-M` is the way to compile a proto package that legitimately spans multiple Go packages (e.g. Loki's `package logproto`, split between the standalone `pkg/push` module and `pkg/logproto`): pin the odd files out with `-M` and the rest of the package must still agree among itself. References between the two halves are emitted as qualified imports. An override that would split one output *directory* across two Go packages is still rejected — one directory maps to one Go package.

## Examples

Given a `.proto` tree like:

```
proto/
  example/v1/
    greeter.proto       # package example.v1; option go_package = "github.com/grafana/wiresmith/gen/example/v1";
    notes.proto         # package example.v1; option go_package = "github.com/grafana/wiresmith/gen/example/v1";
```

walk-and-emit-everything mode:

```sh
./wiresmith --proto_path=proto --out=gen --module=wiresmith
```

produces both `gen/example/v1/greeter.pb.go` and `gen/example/v1/notes.pb.go`,
importable as `github.com/grafana/wiresmith/gen/example/v1`.

Scoped mode emits only the listed file(s) while keeping the import graph:

```sh
./wiresmith --proto_path=proto --out=gen --module=wiresmith proto/example/v1/greeter.proto
```

produces only `gen/example/v1/greeter.pb.go`. Any imports from `greeter.proto`
into `notes.proto` (or any other file under `proto/`) still resolve, the
output file just isn't generated.

To opt a field into pointer-shaped codegen, import `wiresmith/options.proto` from the `.proto` source — see [extensions.md](extensions.md) for the option's effect and the worked example in [`proto/basic/pointer.proto`](../proto/basic/pointer.proto).

## `protoc` / `buf` plugin

The sibling binary at `cmd/protoc-gen-wiresmith` is a `protoc` plugin built on `google.golang.org/protobuf/compiler/protogen`. Once on `PATH`, both `protoc` and `buf generate` invoke it the same way they invoke `protoc-gen-go`:

```sh
go build -o /usr/local/bin/protoc-gen-wiresmith ./cmd/protoc-gen-wiresmith

protoc \
  --proto_path=proto \
  --wiresmith_out=gen \
  --wiresmith_opt=module=example.com/myproject \
  proto/example/v1/greeter.proto
```

Or via `buf.gen.yaml`:

```yaml
version: v2
plugins:
  - local: protoc-gen-wiresmith
    out: gen
    opt:
      - module=example.com/myproject
```

The plugin path is feature-equivalent to the CLI: it produces the same four files per `.proto` (`<name>.pb.go`, `<name>_reflect.pb.go`, `<name>_compare.pb.go`, `<name>_equal.pb.go`). Output paths are source-relative — the same scheme as `protoc-gen-go`'s `paths=source_relative` mode — so `buf`'s `out:` directive controls placement.

Supported `--wiresmith_opt` parameters:

| Parameter | Description |
|-----------|-------------|
| `module=…` | Go module path used as a fallback when a `.proto` omits `option go_package`. Matches the `--module` flag on the CLI. |
| `M<src>=<dest>` | Per-file import-path override; repeatable. Matches the `-M` flag on the CLI. |

The plugin and the CLI share the same generator core — bug fixes in either land in both at once.

To use `(wiresmith.options.*)` extensions in plugin mode, the consumer's proto module must make `wiresmith/options.proto` resolvable (vendor it, or add it as a `buf` module dependency). The plugin only auto-injects the embedded schema in CLI mode; `protoc`/`buf` need to see the file ahead of time to compile any `.proto` that references the extensions.
