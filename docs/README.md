# wiresmith documentation

Reference docs for wiresmith — the custom protobuf compiler. For an overview and benchmarks, see the [repo README](../README.md).

- [CLI reference](cli.md) — flags, install, example invocation.
- [Design and tradeoffs](design.md) — why value-type structs, reverse-write marshaling, no reflection — and what that costs.
- [Comparison with vtproto, gogoproto, and the official runtime](comparison.md) — feature matrix and full benchmark numbers.
- [Custom proto extensions](extensions.md) — the `wiresmith/options.proto` field/message/enum options, plus `google.protobuf.Any` handling and the AnyAdapter bridge for gogo-registry payloads.
- [Generated Go API](generated-api.md) — the method surface wiresmith emits per message and per enum, plus reflection caveats.
- [Testing strategy](testing.md) — what each subtree under `test/` covers, conformance status, fuzz, and benchmark workflow.
