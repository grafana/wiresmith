# wiresmith

Custom protobuf compiler that generates high-performance Go code from `.proto` files, originally built for OpenTelemetry and observability workloads. Built on `google.golang.org/protobuf/encoding/protowire` and reverse-write marshaling.

## Status

Beta. Generated code and the CLI now follow protoc/buf conventions (proto_path-relative import keying, official-runtime registration with a documented `no_registration` opt-out), and the custom options schema is published on the Buf Schema Registry as `buf.build/grafana/wiresmith`. The compiler is exercised by four production-scale migrations — Mimir, Tempo, and Loki (draft PRs) plus a Prometheus fork PR — all building and CI-green against their pinned wiresmith versions. Breaking changes are no longer expected as routine; future ones will be deliberate and called out.

## Why

The official Go protobuf runtime uses reflection-based marshaling, which adds overhead that matters at scale. Existing alternatives like [vtprotobuf](https://github.com/planetscale/vtprotobuf) and [gogoproto](https://github.com/gogo/protobuf) generate faster code but still use pointer-based message fields, trading heap allocations for indirection on every access.

wiresmith takes a different approach: **value-type structs with generated marshal/unmarshal code and zero reflection**. The result is fewer allocations, better cache locality, and faster serialization across the board.

## Benchmarks

_As of 2026-05-22 ([`fffc26c`](https://github.com/grafana/wiresmith/commit/fffc26c))._ Reproduce locally with `make bench-compare`.

On a full trace payload (100 spans, Apple M4 Pro, 10 iterations), wiresmith marshals in **6.4 us/op** (vs 7.7 us for vtproto/gogoproto and 46.2 us for the official runtime) and unmarshals in **33.4 us/op** (vs ~36–39 us for vtproto/gogoproto and 70.1 us for the official runtime). Unmarshal also uses 30–40% less memory than the alternatives.

See [docs/comparison.md](docs/comparison.md) for the full per-benchmark tables, the feature matrix, and the methodology.

## Advantages

### Value-type struct fields

Message fields are value types (`Resource Resource`, not `*Resource`). This is the single biggest differentiator:

- **Fewer allocations** -- no `new(Span)` per element; the struct lives inline in the slice backing array.
- **Better cache locality** -- iterating `[]Span` reads contiguous memory instead of chasing pointers through `[]*Span`.
- Trade-off: slice growth copies larger elements. We mitigate this with pre-scan pre-allocation (see below).

### Reverse-write marshaling

`MarshalToSizedBuffer` writes from the end of the buffer backwards. Nested messages need their size as a varint prefix -- forward-write must compute size first, then write. Reverse-write writes the child, then prepends the length in one pass instead of two.

Same technique vtprotobuf uses, but combined with value types we avoid a pointer dereference per field.

### Pre-computed tag bytes

Tags are emitted as byte literals (`dAtA[i] = 0x0a`) at codegen time, not computed at runtime.

### No reflection

All marshal/unmarshal/size code is directly generated. The official protobuf library uses `protoreflect` interfaces at runtime, which is why it's 3-7x slower.

### Pre-scan pre-allocation

During unmarshal, a lightweight pre-scan counts repeated elements before allocating slices at exact capacity. This eliminates growth waste entirely:

- VTProto/GogoProto use pointer slices, so each wasted slot costs 8 bytes.
- Our value-type slices would waste `sizeof(struct)` per slot (e.g. 256 bytes for `Span`).
- The pre-scan makes this a non-issue: exact capacity, zero growth waste.

Net result: value-type cache locality benefits without the memory penalty -- 30-40% less memory than VTProto on unmarshal.

Unmarshal merges into a non-empty message (repeated fields append, map entries last-write-wins -- gogo parity), and the prealloc reuses a caller-provided backing array when it already has room (pooled messages keep their buffers across decodes). Call `Reset()` first for replace semantics -- see [docs/design.md](docs/design.md).

### Packed scalar exact-capacity allocation

For fixed-size packed fields (`uint64`, `float64`), we compute `len(data)/8` and allocate once.

See [docs/design.md](docs/design.md) for the full list of design decisions and the deliberate limitations they imply.

## Supported proto3 features

Messages, nested messages, enums (top-level and nested), oneof, optional, repeated (packed + non-packed), maps, reserved fields, cross-file imports, fully-qualified type references.

Scalar types: `string`, `bool`, `int32`, `int64`, `uint32`, `uint64`, `sint32`, `sint64`, `float`, `double`, `bytes`, `fixed32`, `fixed64`, `sfixed32`, `sfixed64`.

Map keys: all scalar types except `float`, `double`, and `bytes`. Map values: all scalars, enums, and messages.

Well-known types `google.protobuf.Timestamp` / `Duration` (via the `stdtime` / `stdduration` options) and `google.protobuf.Any` are supported, as are `service` definitions (emitting `<name>_grpc.pb.go` stubs) — see [docs/extensions.md](docs/extensions.md) and [docs/cli.md](docs/cli.md).

Not supported (not needed for OTel protos): other well-known types (Empty, Struct, FieldMask, wrappers), extensions (other than wiresmith's own options), proto2.

## Install

```sh
go install github.com/grafana/wiresmith/cmd/wiresmith@latest
```

Or build from a checkout:

```sh
git clone https://github.com/grafana/wiresmith.git
cd wiresmith
go build -o wiresmith ./cmd/wiresmith     # binary in ./wiresmith
# or, to drop it into $GOBIN:
go install ./cmd/wiresmith
```

(Use the SSH form `git@github.com:grafana/wiresmith.git` if you prefer.)

## Run

```sh
./wiresmith --proto_path=proto --out=gen --module=github.com/your/module
```

`--proto_path` walks the .proto tree, `--out` is the destination for generated `.pb.go` files (source-relative under that root), and `--module` is the **Go module prefix used in cross-file imports** — set it to your own module's path. Inside this repo, that's `github.com/grafana/wiresmith`; in your project, it's whatever your `go.mod` declares.

Passing one or more `.proto` paths as positional arguments scopes emission to just those files; the paths must live under `--proto_path` (a path outside that tree is rejected up front). Their imports are still resolved against the full `--proto_path` walk, but only the listed files and their transitive imports are actually compiled — unrelated siblings in the tree are never parsed, so a tree that still contains protos wiresmith can't compile (e.g. gogo-annotated ones mid-migration) doesn't block a scoped run.

`./wiresmith --help` lists every flag; `./wiresmith --version` prints the build version. Drop the `./` once the binary is on `$PATH`.

See [docs/cli.md](docs/cli.md) for the full CLI reference and a worked example.

## Development

Inside the repo, the Makefile is the canonical entry point:

```
make generate    # regenerate Go code from .proto files
make build       # build all packages
make test        # run round-trip correctness tests
make bench       # run comparative benchmarks
```

See `Makefile` for all targets.

## Claude Code plugin

This repo doubles as a [Claude Code](https://claude.com/claude-code) plugin marketplace (`.claude-plugin/`), shipping the `wiresmith-migrator` agent: guidance for migrating a Go repository from gogoproto-generated code to wiresmith, kept in sync with the compiler as it evolves. See [`agents/wiresmith-migrator.md`](agents/wiresmith-migrator.md).

## Documentation

- [CLI reference](docs/cli.md)
- [Design and tradeoffs](docs/design.md)
- [Comparison with vtproto, gogoproto, and official protobuf](docs/comparison.md)
- [Custom proto extensions](docs/extensions.md)
- [Generated Go API](docs/generated-api.md)
- [Testing strategy](docs/testing.md)
