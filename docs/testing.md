# Testing strategy

wiresmith's tests are organized by purpose, not by package. The split helps balance three competing goals: exercising every code path the generator can emit, catching wire-format regressions against real-world peers, and proving conformance against Google's reference suite.

All commands assume the repo root as the working directory. Use `GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn go test ./...` to run everything in one shot — see the "Known runtime conflict" section below.

## `test/` layout

### `testutil/` — shared helpers

A small message interface and constructor helpers used across the other subdirectories. No tests of its own; pulled in by `test/basic/`, `test/differential/`, `test/peer/`, and `test/fuzz/`.

### `test/basic/` — code-path exercise tests

Round-trip, equality, presence, and per-feature tests against the basic protos (`proto/basic/{numeric,enum,oneof,nesting,recursive,maps,pointer}.proto`) plus the `kitchen_sink.proto`. Each test targets a specific generator code path — e.g. `has_field_test.go` covers the presence bitmap, `pointer_test.go` covers `(wiresmith.options.pointer)`. Run via `make test` or `go test ./test/basic/...`.

### `test/fuzz/` — fuzz targets

Native Go fuzzing. Two files:

- `fuzz_test.go` — wiresmith-only targets that feed random bytes to generated `Unmarshal` methods and assert "no panic" plus structural validity on success.
- `differential_fuzz_test.go` — cross-library fuzz against the official runtime and gogoproto, catching divergence between implementations.

The fuzz corpus is checked in under `test/fuzz/testdata/`. Run via `make fuzz`, which auto-discovers `Fuzz*` functions and runs each for 30 seconds.

### `test/differential/` — cross-library comparison tests

Round-trip and parse comparisons against `google.golang.org/protobuf` and `go.opentelemetry.io/collector/pdata`. These are not benchmarks — they verify that wiresmith emits and accepts the same wire bytes as the reference implementations on representative payloads. Run via `go test ./test/differential/...`.

### `test/peer/` — patterns borrowed from vtproto / gogoproto

Tests transplanted from upstream vtprotobuf and gogoproto test suites. The goal is to inherit decades of corner-case coverage without re-deriving it. Run via `go test ./test/peer/...`.

### `test/conformance/` — Google conformance suite

The official `conformance_test_runner` (C++) run against a Go testee that uses wiresmith-generated code. Lives behind Docker so the C++ binary does not need to be built locally — see [`test/conformance/AGENTS.md`](../test/conformance/AGENTS.md) for the setup. Run via `make conformance`.

Current status: **696 passing, 5 expected failures** (3 from message-merge semantics on recursive messages, 2 from unknown-field preservation — the latter is intentional, see [design.md](design.md#limitations)). The runner fails fast if the failure list contains entries that now pass; update `test/conformance/failure_list.txt` by removing any line the runner flags as "is in the failure list, but test succeeded".

### `test/field_survival_test.go`

A top-level test that survives parsing through several proto versions and checks that wiresmith does not silently drop fields between rounds. Kept at the top level because it does not fit any single subdirectory's theme.

## Benchmarks

The benchmark suite lives in [`bench/`](../bench/) — comparative against the official runtime, vtproto, and gogoproto. See [`bench/AGENTS.md`](../bench/AGENTS.md) for the methodology (shared canonical inputs, per-library files, full benchmark variant list) and [`comparison.md`](comparison.md) for the headline numbers.

For changes that touch marshal/unmarshal/size hot paths, the project requires a baseline-vs-after `benchstat` comparison before merge. The detailed workflow (filter, `-count=20 -benchtime=1s`, noise-control benchmark) lives in [`AGENTS.md`](../AGENTS.md) under "Benchmarking before/after any change" — that section is the source of truth and changes there propagate to reviewers.

## Known runtime conflict

`go test ./...` panics with `proto: file ... is already registered` because three independently generated packages (`gen/bench/official`, `gen/bench/vtpb`, `gen/bench/gogopb`) plus wiresmith all register the same OpenTelemetry proto descriptors with `protoregistry.GlobalFiles`. The fix is upstream — `google.golang.org/protobuf` reads the `GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn` env var to downgrade the panic to a log line:

```sh
GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn go test ./...
```

To run only the conflict-free subset: `go test ./compiler/...`.
