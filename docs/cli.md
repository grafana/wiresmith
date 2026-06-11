# CLI reference

The `wiresmith` command compiles `.proto` files in a directory tree into Go packages of marshal/unmarshal/size code.

## Install

Install the latest version directly:

```sh
go install github.com/grafana/wiresmith/cmd/wiresmith@latest
```

Or build from a checkout:

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
only those files are emitted. Only the listed files and their transitive
imports are compiled — unrelated siblings in the tree are never parsed, so
gogo-annotated protos elsewhere in a mid-migration tree don't block a
scoped run. Their imports are still resolved against the
full `--proto_path` walk, so cross-file references in the unemitted set
keep working. When no files are given, wiresmith walks every
`--proto_path` root and emits every `.proto` it finds (the default).

Positional paths must live under some `--proto_path` root; passing a
`.proto` from outside every walked tree is rejected so a typo doesn't
silently produce an empty generation run. This matches the
positional-argument convention used by `protoc`,
`protoc-gen-go-vtproto`, and `protoc-gen-gogofast`.

`--proto_path` is repeatable for multi-root layouts (vendored protos
alongside project-local protos, etc.) and matches
`protoc -I=root1 -I=root2`. A single occurrence may also carry
list-separated entries using the OS path-list separator
(`--proto_path=root1:root2` on Unix, `--proto_path=root1;root2` on
Windows — Go's `os.PathListSeparator`). `-I` is accepted as a short
alias. Using the OS separator rather than a fixed `:` keeps Windows
drive-letter paths like `C:\proto` from being split into `C` and
`\proto`.

When the same import key would resolve to two different files across
the configured roots, wiresmith fails with a `duplicate import key`
error that names both candidate absolute paths. This is stricter than
`protoc`'s first-wins behaviour — the goal is to make accidental
shadowing (an older copy in one root masking a newer copy in another)
visible up front rather than as a downstream wire-format or build
mismatch.

## Flags

| Flag           | Default       | Description                                                |
|----------------|---------------|------------------------------------------------------------|
| `--proto_path`, `-I` | `proto` | Directory containing `.proto` files (walked recursively). Repeatable; a single occurrence may carry list-separated entries using `os.PathListSeparator` (`:` on Unix, `;` on Windows). |
| `--out`        | `gen`         | Output directory for generated Go packages.                |
| `--module`     | `github.com/grafana/wiresmith` | Go module name used as the prefix when emitting imports.   |
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

Two proto packages may share one Go package when they live in the same directory and their resolved `go_package` (import path **and** package name) agree — protoc parity; e.g. Loki's `indexgateway.proto` (`package indexgatewaypb`) cohabits `pkg/logproto` with the `package logproto` files. What a directory cannot hold is two import paths or two package clauses; both are rejected up front.

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
./wiresmith --proto_path=proto --out=gen --module=github.com/grafana/wiresmith
```

produces both `gen/example/v1/greeter.pb.go` and `gen/example/v1/notes.pb.go`,
importable as `github.com/grafana/wiresmith/gen/example/v1`.

Scoped mode emits only the listed file(s) while keeping the import graph:

```sh
./wiresmith --proto_path=proto --out=gen --module=github.com/grafana/wiresmith proto/example/v1/greeter.proto
```

produces only `gen/example/v1/greeter.pb.go`. Any imports from `greeter.proto`
into `notes.proto` (or any other file under `proto/`) still resolve, the
output file just isn't generated.

Multi-root mode pulls `.proto` sources from several trees that share an
import namespace. Given a layout like:

```
vendor/proto/
  lib/v1/foo.proto
internal/proto/
  app/v1/bar.proto         # imports "lib/v1/foo.proto"
```

both files compile in one run:

```sh
./wiresmith --proto_path=vendor/proto --proto_path=internal/proto --out=gen --module=example.com/myproject
# or equivalently with the list-separator shorthand and the -I alias
# (':' on Unix, ';' on Windows):
./wiresmith -I=vendor/proto:internal/proto --out=gen --module=example.com/myproject
```

To opt a field into pointer-shaped codegen, import `wiresmith/options.proto` from the `.proto` source — see [extensions.md](extensions.md) for the option's effect and the worked example in [`proto/basic/pointer.proto`](../proto/basic/pointer.proto).

## `protoc` / `buf` plugin

wiresmith also ships as a `protoc` plugin (`protoc-gen-wiresmith`) for `protoc`
and `buf generate` pipelines — see [buf.md](buf.md).
