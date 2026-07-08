# wiresmith documentation

Reference docs for wiresmith — the custom protobuf compiler. For an overview and benchmarks, see the [repo README](../README.md).

- [Overview](overview.md) — performance advantages and the full list of supported proto3 features.
- [CLI reference](cli.md) — flags, install, example invocation.
- [`protoc` / `buf` plugin](buf.md) — using `protoc-gen-wiresmith` from `protoc` and `buf generate`.
- [Design and tradeoffs](design.md) — why value-type structs, reverse-write marshaling, no reflection — and what that costs.
- [Comparison with vtproto, gogoproto, and the official runtime](comparison.md) — feature matrix and full benchmark numbers.
- [Custom proto extensions](extensions.md) — `wiresmith/options.proto` and its field/message/enum/file options (`pointer`, `jsontag`, `customtype`, `customname`, `casttype`, `stdtime`, `stdduration`, `no_presence`, `enum_no_prefix`, `no_registration`), plus `google.protobuf.Any` handling and the AnyAdapter bridge for gogo-registry payloads.
- [Generated Go API](generated-api.md) — the method surface wiresmith emits per message and per enum, plus reflection caveats.
- [Testing strategy](testing.md) — what each subtree under `test/` covers, conformance status, fuzz, and benchmark workflow.
