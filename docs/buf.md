# `protoc` / `buf` plugin

Besides the standalone CLI (see [cli.md](cli.md)), wiresmith ships a `protoc`
plugin, `protoc-gen-wiresmith`, so it slots into existing `protoc` and
[`buf`](https://buf.build) pipelines the same way `protoc-gen-go` does.

The sibling binary at `cmd/protoc-gen-wiresmith` is built on
`google.golang.org/protobuf/compiler/protogen`. Once on `PATH`, both `protoc`
and `buf generate` invoke it the same way they invoke `protoc-gen-go`:

```sh
go install github.com/grafana/wiresmith/cmd/protoc-gen-wiresmith@latest

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

The plugin path is feature-equivalent to the CLI. Which files it emits for a given `.proto` depends on what that file declares:

- `<name>.pb.go` — message structs with marshal/unmarshal/size; emitted when the file declares messages or enums.
- `<name>_compare.pb.go` — `Equal()` + `Compare()`; emitted when the file declares messages.
- `<name>_util.pb.go` — the consolidated cold companion: reflection/registration glue plus per-message `String()`.
- `<name>_grpc.pb.go` — gRPC service stubs; emitted when the file declares `service` blocks.

So a file with both messages and services produces all four, while a service-only file produces just `<name>_grpc.pb.go` and `<name>_util.pb.go`. Output paths are source-relative — the same scheme as `protoc-gen-go`'s `paths=source_relative` mode — so `buf`'s `out:` directive controls placement.

Supported `--wiresmith_opt` parameters:

| Parameter | Description |
|-----------|-------------|
| `module=…` | Go module path used as a fallback when a `.proto` omits `option go_package`. Matches the `--module` flag on the CLI. |
| `M<src>=<dest>` | Per-file import-path override; repeatable. Matches the `-M` flag on the CLI. |

The plugin and the CLI share the same generator core — bug fixes in either land in both at once.

To use `(wiresmith.options.*)` extensions in plugin mode, the consumer's proto module must make `wiresmith/options.proto` resolvable (vendor it, or add it as a `buf` module dependency). The plugin only auto-injects the embedded schema in CLI mode; `protoc`/`buf` need to see the file ahead of time to compile any `.proto` that references the extensions.
