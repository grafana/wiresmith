# wiresmith

Custom protobuf compiler that generates high-performance Go code from OpenTelemetry .proto files using `google.golang.org/protobuf/encoding/protowire` and reverse-write marshaling.

## Project structure

- `proto/` - All .proto source files, organized by purpose:
  - `otlp/` - OpenTelemetry protos (common, resource, metrics, trace, logs, profiles)
  - `test/` - Test protos (kitchen_sink)
  - `basic/` - Basic type coverage protos (maps, numeric, enum, oneof, nesting, recursive)
  - `conformance/` - Conformance protos (protocol envelope + test messages)
- `compiler/generator/` - Code generator: reads proto descriptors via `bufbuild/protocompile`, emits Go structs + marshal/unmarshal/size methods
- `compiler/types/` - Per-kind type dispatch for code emission, see [compiler/types/AGENTS.md](compiler/types/AGENTS.md)
- `cmd/wiresmith/` - CLI entry point
- `gen/opentelemetry/proto/` - Generated OTel Go packages (one per proto file; source-relative output)
- `gen/basic/` - Generated Go packages for basic type coverage protos
- `gen/vtpb/` - vtproto-generated code for benchmark comparison
- `gen/gogopb/` - gogoproto-generated code for benchmark comparison
- `protohelpers/` - Shared reverse-write encoding helpers, checked-in source (based on vtprotobuf's protohelpers, Apache 2.0)
- `test/` - All tests, organized by purpose:
  - `testutil/` - Shared test helpers (message interface, constructors)
  - `basic/` - Code-path exercise tests (roundtrip, equal, has_field, kitchen_sink, map, etc.)
  - `fuzz/` - Fuzz tests (unmarshal, roundtrip, cross-library, structured, differential)
  - `differential/` - Cross-library comparison tests (official protobuf, pdata)
  - `peer/` - Tests sourced from vtproto/gogoproto patterns
  - `conformance/` - Google protobuf conformance tests (Docker-based), see [test/conformance/AGENTS.md](test/conformance/AGENTS.md)
- `bench/` - Comparative benchmarks (ours vs official protobuf vs vtproto vs gogoproto)

## Commands

All commands are available via `make`:

| Target | Description |
|--------|-------------|
| `make build` | Build all packages |
| `make test` | Run correctness tests |
| `make fuzz` | Fuzz `Unmarshal` methods (30s) — feeds random bytes to verify errors, not panics |
| `make generate` | Regenerate all code (ours + vtproto + gogoproto). Requires `protoc`, `protoc-gen-go`, `protoc-gen-go-vtproto`, `protoc-gen-gogofast` |
| `make generate-ours` | Regenerate all wiresmith + conformance code. Requires `protoc`, `protoc-gen-go` |
| `make coverage` | Run tests with coverage report |
| `make bench` | Run comparative benchmarks (5 iterations) |
| `make bench-compare` | Run per-library benchmarks and compare with `benchstat`. Accepts `COUNT=-count=N` |
| `make conformance` | Run Google protobuf conformance tests in Docker |
| `make clean` | Remove all generated code under `gen/` |

## Benchmarking before/after any change

Any change that touches the marshal, unmarshal, size, or codegen hot paths — even
correctness fixes — must be validated with a baseline-vs-after benchstat
comparison before being merged. The Go SSA backend often shifts cost in
non-obvious ways (CSE, register pressure, inlining cutoffs), so the only safe
assumption is "measure it."

The required workflow:

1. From a clean tree, capture a baseline (filter to the benchmarks the change is
   most likely to affect; include at least one unaffected benchmark as a noise
   control):
   ```sh
   GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn \
     go test ./bench/ -bench='<filter>' -benchmem \
     -count=20 -benchtime=1s -run='^$' > /tmp/before.txt
   ```
2. Apply the change and `make generate-ours` if codegen changed.
3. Capture the after-numbers with the *same* filter, count, and benchtime:
   ```sh
   GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn \
     go test ./bench/ -bench='<filter>' -benchmem \
     -count=20 -benchtime=1s -run='^$' > /tmp/after.txt
   ```
4. Compare: `benchstat /tmp/before.txt /tmp/after.txt`.

Treat moves on benchmarks the change *cannot* touch (e.g. `SizeTraces_Ours` for
a float-only fix) as the noise floor; only call out regressions that exceed it
on the targeted benchmarks. `count=20, benchtime=1s` is the minimum for
statistically meaningful p-values from `benchstat`; higher is fine for
borderline cases.

## Design decisions

See [docs/design.md](docs/design.md) for the canonical list of design decisions and the deliberate limitations they imply (no unknown-field preservation, no deterministic marshaling, no field-level reflection, etc.). Update that file when a design decision changes; this section intentionally stays a pointer to avoid two sources of truth.

## Custom field options

wiresmith ships custom field options in `compiler/generator/embed/wiresmith/options.proto`, served from the canonical import path `wiresmith/options.proto` (embedded in the compiler, no vendoring required). On-wire format is unaffected by any of them — they only change the generated Go shape.

- `(wiresmith.options.pointer) = true` — switches a singular message field from `T` to `*T` and a repeated message field from `[]T` to `[]*T`. Rejected on scalar, enum, bytes, string, map, oneof, and proto3-`optional` fields.
- `(wiresmith.options.jsontag) = "..."` — overrides the `json:"..."` struct tag verbatim (no `,omitempty` appended); applies to every field kind.
- `(wiresmith.options.customtype) = "import/path.TypeName"` — replaces the Go field type with a user-supplied type that owns its wire encoding (must satisfy `SizeWiresmith/MarshalWiresmith/UnmarshalWiresmith/EqualWiresmith/CompareWiresmith`). Applies to singular or repeated `bytes`, `string`, and message fields. Peer-option compatibility is gated by an explicit whitelist (`customtypeCompatiblePeers` in `compiler/generator/option_customtype.go`); customname / jsontag are compatible, pointer / stdtime are not.
- `(wiresmith.options.customname) = "Identifier"` — overrides the Go field name and every derived accessor (`Get*`, `Has*`, oneof wrapper field). Useful for preserving initialisms (`BlockID` instead of `BlockId`). Applies to every field kind.
- `(wiresmith.options.stdtime) = true` — swaps a singular `google.protobuf.Timestamp` field for a stdlib `time.Time`. Go zero `time.Time{}` is treated as "not set" (gogoproto-compatible); decoded times are normalised to UTC. v1 scope is singular only — repeated, optional, oneof, and pointer-option combinations are rejected.

See [docs/extensions.md](docs/extensions.md) for the full rules and worked examples.

## Generated Compare() method

Every generated message receives a `Compare(other interface{}) int` method (-1/0/+1 like `bytes.Compare`, gogoproto-compatible nil/wrong-type preamble). Fields walk in ascending wire-tag order; floats compare bit-exact via `math.Float{32,64}bits`; maps sort their keys for a total order; oneof variants compare by declaration index then payload. The methods are emitted into a sibling `<name>_compare.pb.go` rather than the main `.pb.go` — emitting Compare next to the hot Marshal/Unmarshal in one file pushed the hot functions onto different cache sets and produced a measured +9% geomean regression on OTel hot benchmarks (UnmarshalMap +14%, MarshalSingleSpan +13%), the same icache-pressure failure mode the `_reflect.pb.go` split handles. See `compiler/generator/emit_compare.go` and the banner at the top of any `_compare.pb.go`.

## Supported proto3 features

Messages, nested messages, enums (top-level and nested), oneof, optional, repeated (packed + non-packed), maps, reserved fields, cross-file imports, fully-qualified type references. Scalar types: string, bool, int32, int64, uint32, uint64, sint32, sint64, float, double, bytes, fixed32, fixed64, sfixed32, sfixed64. Map keys: all scalar types except float/double/bytes. Map values: all scalars, enums, messages.

Not supported (not needed for OTel protos): services/RPCs, extensions, well-known types, proto2.

## Conformance test status

700 passing, 2 expected failures (both from unknown field preservation — see `test/conformance/failure_list.txt`). Unknown fields are intentionally discarded for performance. Run with `make conformance`.

**Updating the failure list:** The conformance runner errors when a failure list entry matches a now-passing test ("is in the failure list, but test succeeded"). After fixing conformance-related bugs, run conformance and remove entries that the runner flags:

```
docker run --rm --entrypoint conformance_test_runner wiresmith-conformance /usr/local/bin/testee
```

Compare the `unexpected failures` output against `test/conformance/failure_list.txt` and remove entries that no longer appear. The expected failure count in the runner output should equal the number of entries in the file.

## Common review caveats

Recurring themes from PR reviews. Keep these in mind when modifying the generator or generated code.

### Nil-safety on all generated receiver methods
Every generated method with a pointer receiver (`String()`, `Has<Field>()`, `Get<Field>()`, `Equal()`, `Reset()`) must handle `nil` receivers gracefully — return zero value or `"<nil>"`, never panic. Callers assume uniform nil-safety across the generated API.

### Generated code must compile for edge-case protos
- **`allow_alias` enums**: Multiple enum names can map to the same numeric value. Emitting a map literal with duplicate keys fails compilation — deduplicate by number or use assignment statements.
- **Empty .proto files**: Files with no messages or enums must not emit an empty `init()` or unused imports — both cause compilation errors.
- **`[packed = false]`**: Repeated scalar fields with explicit `[packed = false]` must be marshaled/sized as individual tag+value pairs, not as packed length-delimited blobs.

### Wire format safety in unmarshal
- **Reject field number 0**: Inline tag decode must validate `fieldNum >= 1` before dispatching. Zero is not a valid protobuf field number.
- **Varint byte limits**: Enforce 10-byte maximum for varint decoding. The 10th byte must have `b & 0x7F <= 1` at `shift == 63` to prevent silent overflow.
- **Packed field bounds**: Packed element reads must check against `postIndex` (packed payload boundary), not `l` (message length). Malformed packed data must not consume bytes from subsequent fields.
- **32-bit length safety**: Length-delimited varint decode into `int` truncates on 32-bit architectures when `shift >= 32`. Decode into `uint64` first, then bounds-check before converting.

### Map field correctness
- **Duplicate-key semantics**: proto3 maps use REPLACE / last-write-wins for duplicate keys on the wire (see `protobuf-go::internal/impl/codec_map.go::consumeMapOfMessage`, which allocates a fresh value per entry and `SetMapIndex`-s unconditionally). Each call to `MapField.EmitUnmarshal` decodes one entry into a fresh `mapvalue` and ends with `m[mapkey] = mapvalue` — the outer loop calls us again per wire-tag, and a later entry with the same key just overwrites. Don't reintroduce a post-loop `existing.unmarshal(...)` merge call; the recursion-depth threading that one had (wiresmith-1c0) lives at the *initial* value decode in `MessageType.EmitMapEntryUnmarshal`. wiresmith-05d removed the merge branch.
- **Bytes aliasing**: Bytes map values must allocate a fresh slice per entry (`append([]byte(nil), ...)` or `slices.Clone`). Reusing a backing array via `append(varName[:0], ...)` corrupts previously stored entries.

### Oneof equality requires value comparison
Comparing oneof fields with `!=` checks interface pointer identity, not semantic equality. Two independently allocated oneofs with identical payloads compare as unequal. Use a type-switch that compares variant type and payload.

### Keep documentation in sync with code
AGENTS.md (and `docs/` — especially `docs/cli.md`, `docs/design.md`, `docs/comparison.md`, and `docs/testing.md`) track conformance counts, feature status, and CLI flags. Update them when features land or conformance results change. Reviewers consistently flag stale docs.

### Generator test coverage
The generator smoke test (`TestGenerateMatchesCheckedIn`) only checks files that were produced — it does not fail when a file stops being generated. Significant new generator features (Equal, presence bitmap, registration) should have dedicated tests, not just regeneration checks.

## Known issues

- `go test ./...` panics with `proto: file ... is already registered` due to conflicting proto registrations between wiresmith types and official protobuf types (in `test/...`) and between `gen/bench/official`, `gen/bench/vtpb`, and `gen/bench/gogopb` (in `bench/`). Use `GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn go test ./...` to run all tests, or `go test ./compiler/...` for the conflict-free subset.

## Rejected approaches

Approaches that were investigated and deliberately not adopted. Documented to save future contributors from re-discovering the same dead ends.

### `//go:fix inline` for hot-path helpers

**Idea:** Have the generator emit calls to small helpers (`EncodeVarint`, `ConsumeVarint`, `ConsumeFixed32`, etc.) marked `//go:fix inline`, then run `go fix -inline ./...` as a codegen post-process step. Generator stays simple (single helper call per emit site); committed `.pb.go` ends up with the helper bodies inlined; we keep today's manual-inline performance.

**Why it doesn't work:** Go's `inline` analyzer (Go 1.26, both `go fix -inline` and the standalone `golang.org/x/tools/go/analysis/passes/inline/cmd/inline` tool) only applies a fix when the call can be **reduced to an expression**. Multi-statement bodies — loops, early-return error paths, slice mutations — would require *literalization* (wrapping the body in `func(){...}()`), which the analyzer "discards unconditionally, on grounds of style" (per `go tool fix help inline`). The diagnostic still fires (`Call of X should be inlined [inline_call]`), but no source rewrite happens.

For wiresmith specifically:

- `protohelpers.EncodeVarint` has a `for` loop and slice writes → not inlined despite the directive. Confirmed: `go fix -inline ./gen/...` left every call site untouched.
- `protohelpers.SizeOfVarint` is single-expression and *is* inlinable, but the generator already emits its body inline (`(bits.Len64(x|1)+6)/7`) so there are no call sites to fix in `gen/opentelemetry/proto/`, `gen/test/`, or `gen/protobuf_test_messages/`.
- The proposed unmarshal-side helpers (`ConsumeVarint`, `ConsumeBytesLen`, `ConsumeFixed32`, `ConsumeFixed64`, `ConsumeTag`) all have loops and/or `return 0, 0, err` early-return paths → not inlinable. Extracting them would convert today's inlined unmarshal hot path into uninlined function calls, regressing the 25-28% speedup from commit `594501d` ("perf: inline varint/fixed decoding").

**When this could become viable:** if the Go inliner gains the ability to apply non-literalized rewrites for multi-statement bodies at statement-level call sites (i.e. expand the body inline as a labeled block instead of refusing). Tracking issue: <https://github.com/golang/go/issues/32816> (the original `//go:fix inline` proposal). Until then, wiresmith's "emit the loop directly" approach in `compiler/types/type.go:114-196` and `compiler/generator/emit_unmarshal.go` is the only way to keep these paths inlined.

### Unrolled per-byte varint decoder (protobuf-go style)

**Idea:** Replace the 10-iteration `for { ... }` varint decoder in `compiler/types/type.go::emitConsumeVarintAt` and the multi-byte fallback in `emitConsumeBytesLenAt` with a flat 10-branch unrolled body modelled on `google.golang.org/protobuf/encoding/protowire.ConsumeVarint`. Each byte position becomes its own `if iNdEx >= l … b := dAtA[iNdEx]; iNdEx++; v += uint64(b) << shift; if b < 0x80 { break }; v -= 0x80 << shift` block, written as a single-iteration `for { … }` so each per-byte termination uses `break` without goto labels. Hypothesis (per wiresmith-kgq): the per-iteration `shift >= 64` guard and the in-loop terminator check cost ~1% geomean / +7.7% on `UnmarshalMap_Ours`; unrolling moves the overflow guard to a single branch (byte 9 only) and lets the CPU branch-predict the dominant single-byte fast path.

**Why it doesn't work in practice:** Empirically the change is a wash to a slight regression on the OTel macros, *including the benchmark the bead specifically expected to win* (Maps). Measured on an Apple M4 Pro, count=20 benchstat with paired baseline (`bd-wiresmith-kgq-unrolled-varint`):

| Bench                          | Δ          | p     |
|--------------------------------|-----------:|------:|
| `UnmarshalMap_Ours`            | **+2.17%** | 0.000 |
| `UnmarshalSummary_Ours`        | +0.79%     | 0.000 |
| `UnmarshalSingleSpan_Ours`     | −1.39%     | 0.000 |
| `UnmarshalLogs_Ours`           | −0.51%     | 0.004 |
| `UnmarshalProfiles_Ours`       | −1.32%     | 0.000 |
| `UnmarshalTraces/Histogram/Gauge/Sum/ExpHistogram_Ours` | flat (p ≥ 0.09) | — |
| **geomean**                    | **+0.05%** | — |

Generated `.pb.go` lines grow by **~70%** (47,906 → 81,396 across all wiresmith-owned packages); the linked bench binary grows by **~1.4%** (13.12 MiB → 13.30 MiB). Allocations and B/op are unchanged on every macro.

Why the hypothesis fails:

- `EmitConsumeTagAt` and `emitConsumeBytesLenAt` already peel the single-byte fast path (most OTel tags use field numbers 1–15 and most lengths are < 128). The fallback varint loop is *cold*, so unrolling it touches code that's rarely on the executed path. The single-byte unrolled form is no shorter than one iteration of the old generic loop — both pay a bound check, a load, a terminator check, and a store on the dominant 0..127 case.
- The extra ~30 lines of dead-path code per call site pushes other functions out of the i-cache. With wiresmith generating tens of varint sites per OTel proto, the cumulative i-cache pressure measurably regresses the densest unmarshal paths — Maps in particular, which has many small varints back-to-back and benefits most from i-cache density.
- The Go SSA backend already does a competent job on the loop form post-inline: the loop body is hoisted to a basic block per iteration after escape analysis and BCE, and the `shift >= 64` guard is branch-predicted false through every legitimate varint.

**When this could become viable:** if the Go compiler stops branch-predicting the `shift >= 64` guard cheaply (e.g. a future cost-model change), or if a future micro-arch makes branch-target buffer pressure dominate i-cache pressure, the calculus could flip. The unrolled approach also pairs better with `//go:fix inline`-style rewriting (see above) if that ever materialises, since each per-byte block is an expression-shaped chunk. Until then, keep the loop form.

The matching prototype lives in branch `bd-wiresmith-kgq-unrolled-varint` (left unmerged); see PR linked from wiresmith-kgq for the full benchstat run.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:7510c1e2 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.

## Session Completion

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Sync beads export** (if any `bd` state changed this session — created, claimed, closed, etc.):
   ```bash
   bd export -o .beads/issues.jsonl
   git diff --quiet .beads/issues.jsonl || { git add .beads/issues.jsonl && git commit -m "bd: sync issue export"; }
   ```
   `.beads/issues.jsonl` is the only sync path for collaborators without Dolt remote access — do not push without it.
5. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   git push
   git status  # MUST show "up to date with origin"
   ```
6. **Clean up** - Clear stashes, prune remote branches
7. **Verify** - All changes committed AND pushed
8. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
<!-- END BEADS INTEGRATION -->
