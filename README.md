# wiresmith

Custom protobuf compiler that generates high-performance Go code from `.proto` files, originally built for OpenTelemetry and observability workloads. Built on `google.golang.org/protobuf/encoding/protowire` and reverse-write marshaling.

## Status

Beta. Generated code and the CLI now follow protoc/buf conventions (proto_path-relative import keying, official-runtime registration with a documented `no_registration` opt-out), and the custom options schema is published on the Buf Schema Registry as `buf.build/grafana/wiresmith`. The compiler is exercised by four production-scale migrations — Mimir, Tempo, and Loki (draft PRs) plus a Prometheus fork PR — all building and CI-green against their pinned wiresmith versions. Breaking changes are no longer expected as routine; future ones will be deliberate and called out.

## What it does

The official Go protobuf runtime marshals via reflection, which is slow at scale. wiresmith takes a different approach: **value-type structs with generated, reflection-free marshal/unmarshal code**. Message fields are values (`Resource Resource`, not `*Resource`), so there are fewer allocations and better cache locality, and pre-scan pre-allocation removes the slice-growth penalty value types would otherwise pay.

On a full trace payload (100 spans, Apple M4 Pro) wiresmith marshals in **6.4 µs/op** and unmarshals in **33.4 µs/op** — vs ~7.7 / ~37 µs for vtproto/gogoproto and 46 / 70 µs for the official runtime — using 30–40% less memory on unmarshal ([2026-05-22 numbers; reproduce with `make bench-compare`](docs/comparison.md)).

See [docs/overview.md](docs/overview.md) for the performance advantages and the full list of supported proto3 features.

## Install

```sh
go install github.com/grafana/wiresmith/cmd/wiresmith@latest
```

Or build from a checkout:

```sh
git clone https://github.com/grafana/wiresmith.git
cd wiresmith
go build -o wiresmith ./cmd/wiresmith
```

## Run

```sh
./wiresmith --proto_path=proto --out=gen --module=github.com/your/module
```

`--proto_path` walks the `.proto` tree, `--out` is the destination for generated `.pb.go` files (source-relative under that root), and `--module` is the Go module prefix used in cross-file imports. Passing one or more `.proto` paths as positional arguments scopes emission to just those files and their transitive imports. See [docs/cli.md](docs/cli.md) for the full reference, and [docs/buf.md](docs/buf.md) to run it as a `protoc` / `buf` plugin.

Inside the repo, the Makefile is the canonical entry point: `make generate`, `make build`, `make test`, `make bench`. See `Makefile` for all targets.

## Claude Code plugin

This repo doubles as a [Claude Code](https://claude.com/claude-code) plugin marketplace (`.claude-plugin/`), shipping the `wiresmith-migrator` agent: guidance for migrating a Go repository from gogoproto-generated code to wiresmith, kept in sync with the compiler as it evolves. See [`agents/wiresmith-migrator.md`](agents/wiresmith-migrator.md).

## Documentation

- [Overview](docs/overview.md) — why value-type structs, the performance advantages, and supported proto3 features.
- [CLI reference](docs/cli.md)
- [`protoc` / `buf` plugin](docs/buf.md)
- [Design and tradeoffs](docs/design.md)
- [Comparison with vtproto, gogoproto, and official protobuf](docs/comparison.md)
- [Custom proto extensions](docs/extensions.md)
- [Generated Go API](docs/generated-api.md)
- [Testing strategy](docs/testing.md)
