# wiresmith — Improvement Recommendations

Synthesized from six parallel reviews on 2026-05-20: security & wire-format safety, performance & codegen efficiency, testing coverage & quality, maintainability & architecture, code-generator correctness, and documentation/API/DX. Every item cites a concrete file:line and a recommended remediation; many are verified by inspection of the actual code, not just summary claims.

Findings are grouped by theme and tagged by severity. A ranked **Top 10 by ROI** at the bottom gives a suggested execution order.

Severity legend:
- **Critical** — silent corruption, panic on documented-safe API, or compile-broken output for valid input
- **High** — correctness/security/perf issue that meaningfully degrades the project's goals
- **Medium** — friction or risk that adds up over many PRs
- **Low** — polish, doc drift, micro-opt

---

## 1. Correctness

### CR-1 (Critical) — `Reset()` panics on nil receiver
- **Location:** `compiler/generator/emit_reset.go:15`
- **What:** Emits `func (m *T) Reset() { *m = T{} }` with no nil guard, while the same generator emits `String()` *with* a nil guard at line 17–19. `CLAUDE.md` "Common review caveats" lists `Reset()` as one of the methods that must handle nil gracefully — this regressed silently because `String()` got the treatment and `Reset()` did not.
- **Fix:** `func (m *T) Reset() { if m == nil { return }; *m = T{} }`. One branch on a cold path; trivially testable.

### CR-2 (Critical) — Negative-zero floats are stripped on marshal (silent corruption)
- **Location:** `compiler/types/fixed64.go:42-43` (`EmitSize`, `EmitMarshal` use `if access != 0`); `compiler/types/fixed32.go:43` mirrors this for `float`.
- **What:** In Go, `-0.0 != 0` is `false`, so a singular `double`/`float` set to `-0.0` is treated as a default value and elided from the wire. On round-trip it reads back as `+0.0`. `math.Signbit` is lost. The official `google.golang.org/protobuf` library guards this with `if v == 0 && !math.Signbit(v) { return }` (see its `codec_gen.go`).
- **Fix:** Emit `if math.Float64bits(access) != 0` (or, equivalently, `Float32bits`) so any non-zero bit pattern — including negative zero — serialises. Add a unit test in `test/basic/numeric_test.go` that marshals `math.Copysign(0, -1)` and asserts `math.Signbit` survives the round-trip.

### CR-3 (Critical) — Empty `.proto` produces uncompilable Go
- **Location:** `compiler/generator/generator.go:271-309` (`generateFile`); pertinent imports added unconditionally at `emit_unmarshal.go:163-165`.
- **What:** A `syntax = "proto3"; package empty;` with no messages or enums still emits a `.pb.go` that imports `protohelpers`/`protowire` but never uses them. CLAUDE.md flags this as a known caveat; nothing in the generator guards it.
- **Fix:** In `generateFile`, short-circuit if `messages == 0 && enums == 0` — either skip the file entirely or emit only the package clause. Add a generator test with an empty fixture proto.

### CR-4 (Critical) — Self-referencing message without `optional` emits an infinite Go struct
- **Location:** `compiler/generator/emit_struct.go:51` (no cycle detection); generator emits value-type message fields by default.
- **What:** `message Tree { Tree child = 1; }` is valid proto3. wiresmith emits `Child Tree` (value type), which `go vet` and `go build` both reject as a recursively-sized struct. The pointer-option (`(wiresmith.options.pointer) = true`) and `optional` keyword both fix it, but the generator silently emits the broken struct rather than detecting the cycle.
- **Fix:** Add a SCC-based cycle check during planning, following only non-optional/non-repeated/non-pointer-option/non-oneof/non-map edges. On detection, return an actionable error pointing at the offending field and suggesting `optional` or `(wiresmith.options.pointer) = true`.

### CR-5 (High) — `Equal()` returns false for two identical NaN values
- **Location:** `compiler/types/fixed64.go:96-98` and `fixed32.go:96-98` — `scalarNotEqualGuard` emits `if lhs != rhs { return false }`. For floats, `NaN != NaN == true`.
- **What:** Two messages with identical NaN payloads compare unequal, while `proto.Equal` from the official library treats them as equal. This is a known divergence with gogoproto, but wiresmith's `Equal()` isn't documented as such. The same code path also makes the contract awkward once CR-2 lands (signbit-aware zero).
- **Fix:** Emit `if math.Float64bits(lhs) != math.Float64bits(rhs)` (resp. `Float32bits`). Bit-exact equality kills NaN-asymmetry and negative-zero edge cases in one stroke. Document the chosen contract in `AGENTS.md`.

### CR-6 (High) — `Size()` and `Marshal*` panic on nil receiver, but generated `Get*` and `Equal` are nil-safe
- **Location:** `compiler/generator/emit_size.go:15`; `emit_marshal.go:17,27,34`.
- **What:** Inconsistent nil-safety contract. Users who learn "wiresmith methods are nil-safe" from `Get*`/`Has*`/`Equal`/`Reset`/`String` will be surprised when `nil.Size()` panics. Either tighten the contract universally or document the cutoff explicitly.
- **Fix:** Emit `if m == nil { return 0 }` for `Size()` and `if m == nil { return nil, nil }` (or `(0, nil)`) for the `Marshal*` family. Cost: one branch in cold path. Alternative: explicitly document `Size`/`Marshal` as nil-unsafe in CLAUDE.md.

### CR-7 (Medium) — Optional fields lack `Has*()` methods
- **Location:** `compiler/generator/emit_has.go:30` (`fieldsForPresence` excludes optional/oneof/repeated/map).
- **What:** `optional int32 foo` users must write `m.Foo != nil` while `int32 bar` users write `m.HasBar()`. CLAUDE.md says "Optional fields dereference the pointer" for `Get*`, implying Has should exist. Official protobuf-go emits Has for optional fields.
- **Fix:** Extend `emitHasMethods` to also walk optional fields, emitting `Has<F>() bool { return m != nil && m.F != nil }`. No bitmap involvement; pure nil-check. Update CLAUDE.md description accordingly.

### CR-8 (Medium) — `mapValueStart` reference in `map.go` is fragile
- **Location:** `compiler/types/map.go:106` reads `mapValueStart` declared in `compiler/types/message.go:81`.
- **What:** `mapValueStart` is defined inside `MessageType.EmitMapEntryUnmarshal` but read in `map.go`'s merge path. Today, only `MessageType` triggers the path, so compile-time it works. A future contributor adding a different message-shaped map value will hit a confusing "undeclared `mapValueStart`" compile error in *generated* code.
- **Fix:** Hoist `mapValueStart` declaration into `MapField.EmitUnmarshal` so the symbol's scope is local to one file. Same generated bytes, more local reasoning.

### CR-9 (Low) — Map marshal iteration order is non-deterministic
- **Location:** `compiler/types/map.go:54` (`for k, v := range %s`).
- **What:** Go map iteration order is randomized; two marshals of the same message produce different byte slices when maps are present. Matches official protobuf-go's default but breaks content-addressed storage / cryptographic signing use cases.
- **Fix:** Either (a) sort keys before iteration (cost: per-marshal allocation + sort); (b) add an opt-in `--deterministic-maps` flag; or (c) just document the non-determinism in `AGENTS.md` "Design decisions". Pick based on whether OTel collector hot path needs determinism.

### CR-10 (Low) — UTF-8 not validated on string unmarshal
- **Location:** `compiler/types/string.go:67` does `string(dAtA[iNdEx:postIndex])` without `utf8.Valid`.
- **What:** Proto3 spec says string fields *must* be valid UTF-8; spec-compliant decoders reject invalid sequences. Conformance suite doesn't test it for proto3, and pdata also doesn't validate, so this matches the OTel ecosystem norm.
- **Fix:** Document the divergence in `AGENTS.md`. If strict validation is ever desired, gate it behind a flag — never make it default (the perf cost is non-trivial for log streams full of strings).

---

## 2. Security & Wire-Format Safety

### SEC-1 (Critical) — Pre-scan slice pre-allocation is attacker-amplifiable (~125× memory blow-up)
- **Location:** `compiler/generator/emit_unmarshal.go:144-156` (pre-scan size hint), `99-141` (pre-scan loop).
- **What:** The pre-scan counts field-number occurrences then does `m.X = make([]Span, 0, count)`. A malicious 1 MB payload of tiny tag-for-field-1 + zero-length entries can produce ~500 000 occurrences, allocating ~125 MB of capacity for `[]Span` (struct size ≈ 250 B). Combined with SEC-2 below, the count itself becomes attacker-controlled.
- **Fix:** Cap pre-allocated capacity by a sanity bound: `cap = min(count, (l-iNdEx)/minBytesPerElement)` where `minBytesPerElement` is the smallest legal wire encoding for the element type (e.g., `1` for varint scalar tag + minimum body). For message-typed repeated fields, the minimum is 2 bytes (tag + zero-length). Easy formula: `cap = min(count, l/2)`. Add a fuzz seed that exercises this exact shape.

### SEC-2 (Critical) — Pre-scan `default: break` only exits the switch, not the outer loop
- **Location:** `compiler/generator/emit_unmarshal.go:138-141`.
- **What:** When `preTyp ∈ {3,4,6,7}` (legacy groups + reserved), the `default: break` only terminates the inner `switch`; `preIdx` is unchanged, so the next iteration treats payload bytes as a new tag. This inflates field counts and feeds SEC-1. The post-switch guard at line 141 only catches genuinely out-of-bounds `preIdx`.
- **Fix:** Either label the outer `for` and `break OUTER`, or set `preIdx = l` in the default branch, or simply abort the pre-scan entirely on unknown wire type (the main loop is the source of truth — pre-scan is an optimisation). The "abort the optimisation" option is safest.

### SEC-3 (Critical) — 32-bit length-decode truncation in `skipValue` and inline length helper
- **Location:** `compiler/generator/emit_unmarshal.go:32-43` (skipValue case 2); `compiler/types/type.go:227-232` (`emitConsumeBytesLenAt`).
- **What:** `byteLen` is decoded into `uint64` but immediately converted with `int(byteLen)` and checked only for `< 0`. On `GOARCH=386/arm/wasm` (32-bit `int`), values in `(MaxInt32, MaxInt64]` truncate to small positive ints, bypassing both the `< 0` check and the `postIndex > l` bound. CLAUDE.md "Common review caveats" lists this exact pitfall under "32-bit length safety".
- **Fix:** Before `int(byteLen)`, add `if byteLen > uint64(math.MaxInt) { return io.ErrUnexpectedEOF }`. Apply in both `skipValue` and `emitConsumeBytesLenAt`. Add a test that runs under `GOARCH=386`.

### SEC-4 (High) — Inline varint decoder lacks 10-byte boundary overflow check (`b & 0x7F ≤ 1`)
- **Location:** `compiler/types/type.go:174-185` (`emitConsumeVarintAt`), `216-232` (`emitConsumeBytesLenAt`); skip path in `emit_unmarshal.go:31-40`.
- **What:** The loop terminates on `shift >= 64` (next iteration), so iteration 10 still runs with `shift == 63` and does `v |= uint64(b & 0x7F) << 63`. Bits 1–6 of the 10th byte silently overflow past the uint64 boundary. The wire-format spec requires that on the 10th byte, `b & 0x7F` MUST be ≤ 1. CLAUDE.md flags this exact requirement. The tag decoder (line 157-170) is fine — its 5-byte cap + field-number range check at line 169 cover it.
- **Fix:** After reading `b` inside the loop, emit `if shift == 63 && b > 1 { return fmt.Errorf("proto: varint overflow") }`. Apply to both `emitConsumeVarintAt` and `emitConsumeBytesLenAt`. Cheap; one branch in the cold path.

### SEC-5 (High) — Cross-package message unmarshal resets recursion-depth counter
- **Location:** `compiler/generator/emit_unmarshal.go:167-169` (public `Unmarshal` wrapper) + `compiler/types/type.go:236-242` (`emitUnmarshalCall`).
- **What:** Same-package nested messages call `.unmarshal(b, depth+1)`; cross-package nested messages call `.Unmarshal(b)` which restarts depth at 0. A graph bouncing between packages can recurse to `maxUnmarshalDepth × pkgCount` levels. For OTel today the package count is small enough to be safe, but the protection silently weakens as packages multiply. Documented contract ("unmarshal depth is bounded") is broken across the boundary.
- **Fix:** Give every message a single internal entry like `unmarshalWithDepth(b, depth)` and have cross-package callers invoke it via an exported wrapper that accepts the counter, or wrap depth in a context-style parameter. Concrete sketch: emit `UnmarshalWithDepth(b []byte, depth int) error` as the cross-package surface and call it from `emitUnmarshalCall` when `!isSamePackage`.

### SEC-6 (Medium) — Bytes-map merge path retains a sub-slice of caller's buffer
- **Location:** `compiler/types/map.go:106, 123-128`.
- **What:** For message-valued maps with duplicate keys, the merge path stashes `mapValueBytes := dAtA[mapValueStart:iNdEx]` (a sub-slice of the input buffer) and feeds it back into `existing.unmarshal(mapValueBytes, depth+1)`. The merge is synchronous, so today this is safe — but the contract is implicit. Any future change introducing lazy decode or background processing would turn this into a use-after-recycle for callers who reuse buffers (sync.Pool patterns are common in OTel collector).
- **Fix:** Either copy: `mapValueBytes := append([]byte(nil), dAtA[mapValueStart:iNdEx]...)` (one extra alloc per duplicate-key event, which is rare in well-formed wire), or add a code comment at `map.go:106` documenting the "must be consumed synchronously before `Unmarshal` returns" invariant.

### SEC-7 (Medium) — `collectGoPackages` `..`-segment guard misses unparented paths
- **Location:** `compiler/generator/generator.go:222-240`; `compiler/generator/names.go:141-155`.
- **What:** The `..`-segment check only fires when the parsed `go_package` import path is *inside* `<module>/gen`. A `go_package = "external/path/with/../etc"` falls through the in-base early-out at line 231-233 and the segment check is never run. The fallback path uses the validated proto-package name so this is not currently exploitable, but the guard is misleading — a future change could trust the unvalidated string.
- **Fix:** Move the `..` segment check before the in-base early-out so every `go_package` value is validated regardless of where it points. One extra string scan per file; negligible.

### SEC-8 (Low) — `gen/protohelpers/` registry has no synchronisation
- **Location:** `gen/protohelpers/registry.go:7-42`.
- **What:** Docs say "not safe for concurrent use" but make no statement about ordering. Reads after `init()`-time writes are safe under Go's memory model, but a caller who spawns `go register()` from inside an `init()` (legal, if odd) races undetected.
- **Fix:** Either guard the maps with a `sync.RWMutex` or tighten the package doc: "Register* MUST be called from package init only; any goroutine-spawned registration is undefined behavior."

---

## 3. Performance

### PERF-1 (High) — Packed-scalar unmarshal hot loop calls `protowire.ConsumeFixed64`/`ConsumeVarint` instead of the inline decoders
- **Location:** `compiler/types/repeated.go:184-211` (packed-decode emit). The non-packed path already uses `emitConsumeFixed64At` / `emitConsumeVarintAt` (the inline decoders); the packed path falls back to `protowire.ConsumeFixed64(data)` + `data = data[vn:]`.
- **What:** Inconsistency penalises exactly the hot path: OTel histogram `bucket_counts` and `explicit_bounds` are packed `fixed64`/`uint64` slices iterated thousands of times per second. The reslice `data = data[vn:]` is cheap but not free; the function-call boundary inhibits BCE.
- **Fix:** Emit `emitConsumeFixed{32,64}At` (or `emitConsumeVarintAt`) in the packed inner loop, indexing against `postIndex` directly. Same machinery, same correctness, ~5–15% expected speedup on `UnmarshalHistogram`/`UnmarshalExpHistogram`.
- **Validate:** `make bench-compare COUNT=-count=10` on the histogram benches.

### PERF-2 (High) — Tag decode has no 1-byte fast-path
- **Location:** `compiler/types/type.go:157-170` (`EmitConsumeTagAt`); fires every iteration of the main parse loop.
- **What:** 95%+ of OTel proto fields use field numbers 1–15 (single-byte tag, high bit clear). The generic varint loop pays loop setup + two bounds checks per byte even on the single-byte path.
- **Fix:** Emit a peel: `if iNdEx < l && dAtA[iNdEx] < 0x80 { wire = uint64(dAtA[iNdEx]); iNdEx++ } else { /* generic loop */ }`. Same trick for `byteLen` decode when length < 128 (short strings, small nested messages). Neither vtproto nor gogoproto does this — clean differentiator.
- **Validate:** Microbench on `UnmarshalSingleSpan` should improve 10–20%.

### PERF-3 (High) — Generated `MarshalTo` calls `m.Size()` redundantly, defeating its purpose
- **Location:** `compiler/types/repeated.go:23-34` and generator-side equivalent.
- **What:** `MarshalTo` is meant for caller-allocated buffers — the caller already sized them. Calling `m.Size()` again walks the entire message tree a second time; for nested OTel messages that's the same O(N) work done twice. Bench harness doesn't currently isolate this (`bench/ours_bench_test.go:14-22` always calls the size-allocating `Marshal()`), masking the regression.
- **Fix:** Strip the internal `m.Size()` from generated `MarshalTo`. Document the precondition: `len(dAtA) >= m.Size()`. Add `BenchmarkMarshalTracesPreallocated_*` variants that pre-allocate the buffer outside the timer.
- **Validate:** Expect ~30% improvement on the new pre-allocated marshal benchmarks.

### PERF-4 (Medium) — `EncodeVarint` always loops, even for known-small length prefixes
- **Location:** `gen/protohelpers/protohelpers.go:9-19`; called per nested-message length prefix.
- **What:** Length prefixes for small nested messages are frequently < 128 bytes (1-byte varint). The current path still enters the `for v >= 1<<7` loop. Inlining helps but doesn't kill the branch.
- **Fix:** Emit a specialized 1-byte fast-path at the call site for length prefixes: `if size <= 0x7F { i--; dAtA[i] = byte(size) } else { i = protohelpers.EncodeVarint(dAtA, i, uint64(size)) }`. Same pattern works for tag writes that are already single-byte literals — those are fine.
- **Validate:** `MarshalSingleSpan` / `MarshalTraces` ns/op, 5–10% expected.

### PERF-5 (Medium) — Wire-type-mismatch check duplicated at every case site (~500 LOC × file)
- **Location:** `compiler/generator/emit_unmarshal.go:333-342` (`emitWireTypeCheck`); ~71 sites per `metrics.pb.go`.
- **What:** Each case starts with a 7-line `if wireType != X { skipValue(...); continue }` block. `HistogramDataPoint.unmarshal` compiles to ~7.3 KB of machine code. icache density suffers; recursive walks evict the parent's hot code between nested calls.
- **Fix:** Emit a per-package helper `skipMismatchedWire(dAtA, iNdEx, wireType, fieldNum) (newIdx int, err error)` and shrink each case to a 2-line call. The wire-mismatch path is cold; function-call overhead is negligible. Expected: ~30% shrink in unmarshal function size.
- **Validate:** `go tool objdump` to confirm shrinkage; incidental UnmarshalTraces wins.

### PERF-6 (Medium) — Optional scalar unmarshal heap-allocates `tmp` per occurrence
- **Location:** `compiler/types/optional.go:42-47`; observable in `gen/otlp/metrics/v1/metrics.pb.go` (HistogramDataPoint has 3 optional floats — 3 allocs × N points).
- **What:** `tmp := math.Float64frombits(v); m.Sum = &tmp` forces `tmp` to escape. For `Histogram-50` that's ~150 allocs alone.
- **Fix:** Two options. (a) Add a private `sumStore` field; on unmarshal set `m.sumStore = ...; m.Sum = &m.sumStore`. Costs `sizeof(T)` per optional field even when absent. (b) Use `sync.Pool` for the optional-value sidecar. Option (a) is simpler; option (b) is friendlier to long-lived pdata trees.
- **Validate:** `UnmarshalHistogram` allocs/op should drop ~30%.

### PERF-7 (Low) — Pre-scan threshold `preScanMinBytes = 256` is unjustified
- **Location:** `compiler/generator/emit_unmarshal.go:83`.
- **What:** Below 256 bytes, no pre-scan; above it, full scan. The threshold is uncalibrated and a single message with 1 attribute at 257 B walks all bytes for no payoff.
- **Fix:** Either tune empirically per benchmark, or scale the threshold by `len(fieldsForPreScan(md))` (more fields → benefit threshold is higher). Add a microbench at boundary sizes.

### PERF-8 (Low) — No `unsafe.String` opt-in for borrowed-buffer unmarshal
- **Location:** `compiler/types/string.go:67`.
- **What:** OTel collectors typically own input buffers for the duration of pipeline processing. A `string(dAtA[...])` copy is pure overhead in that scenario.
- **Fix:** Gate an `unsafe.String(unsafe.SliceData(dAtA[iNdEx:]), n)` path behind a CLI flag (`--unsafe-borrow-input`) or proto option. Document the lifetime contract precisely (input buffer must outlive the parsed message).
- **Validate:** `UnmarshalLogs` (string-heavy); expect 20–30% alloc reduction.

### PERF-9 (Low) — Presence-bitmap RMW on every covered field unmarshal regardless of caller need
- **Location:** `compiler/generator/emit_unmarshal.go:194-196`; ~38 RMW sites in `metrics.pb.go`.
- **What:** `m.fieldsPresent[0] |= 1 << X` runs unconditionally. OTel hot-path consumers rarely call `Has*`.
- **Fix:** CLI flag `--no-presence-bitmap` and/or per-message proto option to skip the bitmap. Getters then fall back to zero-value semantics (matching gogoproto).
- **Validate:** Re-bench with flag enabled; expect 1–3% on UnmarshalTraces.

### PERF-10 (Low) — Group wire types (3, 4) emit dead code in every `skipValue`
- **Location:** `compiler/generator/emit_unmarshal.go:44-47`.
- **What:** Proto3 forbids groups; every generated `skipValue` still emits a `case 3:` calling `protowire.ConsumeGroup`. Brings in protowire as a hard dependency just for the never-fired branch.
- **Fix:** Emit `case 3, 4: return fmt.Errorf("groups not supported in proto3")`. Reduces icache pressure and binary size. Verify protowire is still needed elsewhere (it is, for `SizeVarint`).

---

## 4. Testing

### TEST-1 (High) — Zero per-`emit_*.go` unit tests; only `EmitEqual` is tested (via `compiler/types/equal_test.go`)
- **Location:** missing — should live under `compiler/generator/emit_*_test.go`.
- **What:** Of 11 emit files (~1067 LOC), only `emit_equal.go` has unit-level coverage, and that test sits in the wrong package because `EmitEqual` is on the `Type` interface. Every other emit phase — has, getter, reset, struct, marshal, size, unmarshal, oneof, registration, enum — is only exercised transitively via the round-trip `TestGenerateMatchesCheckedIn` smoke check. When that fails, you cannot localise to a phase.
- **Fix:** Build a small test harness in `compiler/generator/testutil_test.go` that captures `body strings.Builder` against a synthetic `protoreflect.MessageDescriptor`. Add one table-driven file per emit:
  - `emit_has_test.go`: 0 / 1 / 65 presence fields; pointer-option excluded; bitmap word count.
  - `emit_oneof_test.go`: variant interface method name; scalar/bytes/message payload switch; single-variant oneof.
  - `emit_registration_test.go`: empty file → no `init()`; only enums; `allow_alias` enum dedup.
  - `emit_unmarshal_test.go`: wire-type mismatch dispatch; pre-scan emission threshold.
  - `emit_reset_test.go`: pin the nil-guard from CR-1 once it lands.

### TEST-2 (High) — 22 of 23 `compiler/types/*.go` files have no test file
- **Location:** missing — `compiler/types/{bool,bytes,double,float,fixed*,sint*,int*,uint*,varint,map,message,oneof,optional,pointer,repeated*,string}_test.go`.
- **What:** Only `equal_test.go` exists. The other `Type` interface methods (`EmitSize`, `EmitMarshal`, `EmitUnmarshal`, `EmitValueSize`, `EmitValueMarshal`, `CastExpr`, `IsPackable`, `WireType`, `SizeByIndex`, `FixedSize`, `VarintSizeExpr`, `ZeroLiteral`) are per-kind invariants that should be locked down individually.
- **Fix:** Extend the `captureEmitter` pattern from `equal_test.go`. Highest-value cases:
  - `bytes_test.go`: confirms `EmitUnmarshal` uses `append([]byte(nil), ...)` (bytes-aliasing guard).
  - `map_test.go`: confirms merge-of-empty-but-present message; confirms key/value tag ordering.
  - `oneof_test.go`: confirms `EmitSize` BytesType/FixedSize branches.
  - `fixed64_test.go` / `fixed32_test.go`: confirm signbit-aware emit after CR-2.

### TEST-3 (High) — Field number 0 rejection is documented but untested
- **Location:** missing — should live under `test/basic/wire_validation_test.go`.
- **What:** `type.go:169` emits `if X>>3 < 1 || X>>3 > 0x1FFFFFFF { return ... }`. CLAUDE.md lists this as a recurring caveat. A regression would be silent today.
- **Fix:** One table-driven test: feed `[]byte{0x00, ...}` (tag = field 0, wire type 0) through every constructor in `testutil.AllMessageConstructors()` and assert a non-nil error containing "invalid field number".

### TEST-4 (Medium) — `Marshal → Unmarshal → Equal` consistency is not a fuzz target
- **Location:** missing — should be `test/fuzz/marshal_equal_roundtrip_fuzz_test.go`.
- **What:** Existing fuzz targets compare *bytes* across the wire boundary. A bug where `Equal()` returns false for two messages that round-trip through wire identically would not be caught.
- **Fix:** Add `FuzzMarshalEqualRoundTrip`: unmarshal input → marshal to bytes → unmarshal again into `msg2` → assert `msg1.Equal(msg2) == true`. This exercises Equal and wire logic against each other.

### TEST-5 (Medium) — Fuzz corpus is thin and OTel-only
- **Location:** `test/fuzz/fuzz_test.go:marshaledSeeds` and `test/fuzz/testdata/fuzz/*`.
- **What:** `FuzzRoundTrip`/`FuzzMarshalSize`/`FuzzCrossLibrary`/`FuzzDifferentialBytewise` share **3 seeds** (Traces/Metrics/Logs). `proto/basic/*` (oneof, recursive, kitchen sink, maps) seeds are absent. `FuzzMarshalSize` and `FuzzDifferentialBytewise` have **zero** persisted corpus files.
- **Fix:** Extend `marshaledSeeds()` with kitchen_sink, oneof, recursive, and the new edge-case fixtures (from TEST-7 below).

### TEST-6 (Medium) — `TestGenerateMatchesCheckedIn` can miss entirely-deleted package directories
- **Location:** `compiler/generator/generator_test.go:238-261`.
- **What:** The reverse check iterates `genDirs` and flags `.pb.go` files in those dirs that weren't produced. If the generator drops an entire directory (e.g. `gen/basic/oneof/v1`), it never appears in `genDirs` and the test passes silently.
- **Fix:** After the loop, walk each proto-package root (`gen/otlp`, `gen/basic`, `gen/test`) and assert every `.pb.go` belongs to *some* `genDirs` entry. Equivalent: take the union of all `.pb.go` paths under each root and assert it's a subset of `generatedFiles`.

### TEST-7 (Medium) — Several edge-case proto fixtures are missing
- **Location:** suggested new `proto/basic/edge_cases.proto`.
- **What:**
  - Single-variant oneof: no proto in fixtures exercises `oneof X { string only = 1; }`. Exercises a different branch in `isRealOneof` / `emit_oneof.go`.
  - Messages with only optional fields: presence bitmap should be size 0; not tested.
  - Same message used both singular and repeated in one file: `Foo singular = 1; repeated Foo many = 2; map<string,Foo> by_key = 3;` — not tested.
  - Cross-package import with same-name types (e.g. `pkgA.Foo` + `pkgB.Foo`) — not tested.
- **Fix:** Add the proto, plus a single roundtrip test in `test/basic/edge_cases_test.go` confirming Marshal → Unmarshal cleanly.

### TEST-8 (Medium) — `cmd/wiresmith/main.go` is untested
- **Location:** missing — should be `cmd/wiresmith/main_test.go`.
- **What:** 66-line `main.go` with flag parsing, `--version`, `--help`, `buildVersion()` fallback to `debug.ReadBuildInfo`. Most likely silent-regression surface.
- **Fix:** Table-driven `TestBuildVersion`; `os/exec` integration test on the built binary asserting `--version` exit code 0 and version-string format; `--help` snapshot test.

### TEST-9 (Medium) — Profiles proto has no pdata/cross-library differential test
- **Location:** missing — `test/differential/pdata_profiles_test.go`.
- **What:** Traces/Metrics/Logs each have differential coverage (`pdata_roundtrip_test.go`). Profiles has only `test/basic/roundtrip_test.go:TestProfilesDataRoundTrip` plus field-survival. `FuzzCrossLibrary` excludes ProfilesData.
- **Fix:** Mirror `TestPdataTracesRoundTrip` for Profiles, against `otlpprofiles` from `go.opentelemetry.io/proto/otlp`. If pdata lacks profile support, use only the official library for comparison.

### TEST-10 (Medium) — Three of five expected conformance failures should be removable
- **Location:** `proto/conformance/test_messages_proto3.proto:28,63` (deliberately removed `corecursive` and `recursive_message`); `test/conformance/failure_list.txt`.
- **What:** The 3 recursive-merge entries exist because the conformance proto deliberately omits self-referencing fields. But wiresmith already supports recursive messages via `optional` (see `proto/basic/recursive.proto`), and the merge path in `emit_unmarshal.go:300-309` exists. Re-introducing the fields with `optional` should fix all three failures.
- **Fix:** Re-add `optional NestedMessage corecursive = 2;` to `NestedMessage` and `optional TestAllTypesProto3 recursive_message = 27;` to `TestAllTypesProto3`. Run conformance, prune `failure_list.txt`. The 2 unknown-field-preservation failures are by-design.

### TEST-11 (Low) — Stale coverage profile
- **Location:** `coverage.out` (dated 2026-05-07).
- **What:** Profile shows many compiler-side functions as 0% that are clearly live (`compiler/generator/types.go:goType` etc.) — the profile cannot be trusted for triage.
- **Fix:** Either re-run before each coverage audit, or wire `make coverage` into CI so the file is regenerated automatically.

### TEST-12 (Low) — Mixed packed + unpacked in the same field is untested
- **Location:** `test/peer/vtprotobuf_patterns_test.go` covers concatenated packed chunks; no test interleaves packed and individual unpacked elements.
- **What:** Proto3 wire spec allows both encodings to appear in the same field concurrently.
- **Fix:** Build a hand-crafted wire: `chunk1 (packed [1,2]) + single unpacked 3 + chunk2 (packed [4,5])`, assert all 5 values arrive in order.

---

## 5. Architecture & Maintainability

### ARCH-1 (High) — `emitFieldUnmarshal` still type-switches on `Kind` for message-shaped logic that belongs on `Type`
- **Location:** `compiler/generator/emit_unmarshal.go:210-330` (~120 lines, five nested decision branches).
- **What:** Three `case protoreflect.MessageKind` blocks at lines 269-283, 295-309, 321-326 re-implement "consume length-delimited, allocate or reuse pointer/value, call Unmarshal" with subtle differences (oneof reuses-or-replaces; optional `new()`s; pointer-option goes through `PointerField`). The recent move of equality into `Type` (commit `428fce8`) did not get applied here.
- **Fix:** Add `EmitOneofVariantUnmarshal(e, ooField, variantName, fieldName, ctx)` to `Type`. Make `OneofField` a composite wrapping `Inner`, analogous to `OptionalField`/`PointerField`. `emitFieldUnmarshal` then becomes ~30 lines of pure dispatch.

### ARCH-2 (Medium) — `Type` interface has methods that some kinds answer by panic
- **Location:** `compiler/types/type.go:27-51`; `fixed32.go:36-38` (`VarintSizeExpr` panics); `fixed64.go:36-38` (same); `message.go:26-29,68-70` (`CastExpr` panics).
- **What:** Classic interface-segregation violation. Call sites in `repeated.go` already guard via type checks (`switch r.Inner.FixedSize()`) so the panics are unreachable today — but the methods exist purely to satisfy `Type`, creating compile-time obligations that can never run.
- **Fix:** Split `Type` into smaller interfaces:
  - `VarintSized` (subset implemented by `varintBase`/`Bool`/`Enum`/sint variants) carries `VarintSizeExpr`.
  - `ScalarDecoder` (implemented by everything except `MessageType`) carries `CastExpr`.
  - `PackedEncoder` (`EmitEncode`) — see ARCH-3.
  Then `repeated.go` and `emit_unmarshal.go` type-assert via the narrow interface rather than guarding panics.

### ARCH-3 (Medium) — Private `encoder` interface bypasses `Type` for packed marshal
- **Location:** `compiler/types/repeated.go:10-12, 98, 109`.
- **What:** A private `encoder` interface is defined in `repeated.go`, then `r.Inner.(encoder).EmitEncode(...)` does an unchecked assertion in the packed-marshal emit. Adding a packable type that forgets `EmitEncode` compiles fine but panics at codegen time. Also: 7 files (`bool.go`, `bytes.go`, `fixed32.go`, `fixed64.go`, `sint32.go`, `sint64.go`, `string.go`, `varint.go`) duplicate the same 4-line `EmitValueMarshal { EmitEncode; ReverseTag }` boilerplate.
- **Fix:** Lift `EmitEncode(Emitter, indent, access)` onto `Type` (or a `PackedEncoder` sub-interface implied by `IsPackable()`). `repeated.go` calls it statically. Pull the 4-line `EmitValueMarshal` into a free helper or default method to dedupe.

### ARCH-4 (Medium) — `Sint32`/`Sint64` are 87-line standalone types where 7-line is sufficient
- **Location:** `compiler/types/sint32.go`, `sint64.go`.
- **What:** `varintBase` already parameterises on `unmarshalCast`. `int32`/`int64`/`uint32`/`uint64` are 7-line files. `sint32`/`sint64` could share a `sintBase` parameterised on `(width, shift, cast)` and shrink ~140 lines total.
- **Fix:** Introduce `sintBase` matching the `varintBase` pattern. The "standalone due to zigzag encoding" justification in `compiler/types/AGENTS.md` is overstated — zigzag differs by shift amount only.

### ARCH-5 (Medium) — `PointerField` vs `OptionalField`, and `RepeatedPointer` vs `RepeatedField`, exist as workarounds for ARCH-2
- **Location:** `compiler/types/optional.go:7`, `pointer.go:12`, `repeated_pointer.go:24`.
- **What:** `OptionalField` handles `proto3 optional` (struct field is `*T`). `PointerField` handles `(wiresmith.options.pointer) = true` on messages only. The split exists because `OptionalField` calls into `MessageType.CastExpr` which panics. Once ARCH-2 lands (Message stops panicking), `OptionalField` can subsume `PointerField` and `RepeatedField` can subsume `RepeatedPointer`. Two narrow composites disappear.
- **Fix:** Sequenced after ARCH-2. Merge composites; update `compiler/types/AGENTS.md` enumeration. `compiler/generator/option_pointer.go:172-187` "field-shape selector" also collapses.

### ARCH-6 (Medium) — Generator-runtime ABI is module-path-mangled
- **Location:** `compiler/generator/emit_registration.go:14,28`; `compiler/types/varint.go:47`; ~19 emit sites.
- **What:** Every call site emits `fg.module+"/gen/protohelpers"` as the import. Consumers running `wiresmith --module=tempo --out=tempopb` get `tempo/gen/protohelpers` imports pointing at a non-existent directory. The "stable runtime helpers at a fixed path" model that `FLAGS.md` proposes is currently unimplementable.
- **Fix:** Move helpers to a standalone module (`github.com/wiresmith/protohelpers` or under the wiresmith module root but at a fixed `wiresmith/protohelpers` path). Replace all `fg.module+"/gen/protohelpers"` with a single constant. Link the resolved design from `FLAGS.md` once committed.

### ARCH-7 (Low) — `bytes.Buffer` body + ad-hoc imports tracking; phase ordering is undocumented
- **Location:** `compiler/generator/generator.go:40-50, 271-309`.
- **What:** `FileGenerator.body *bytes.Buffer` plus `imports` is fine, but `generateFile` invokes phases in a hardcoded order whose constraints are only readable by tracing every emit-site's `addImport` call. `presenceMap(md)` is recomputed in every phase (`emit_struct.go:54`, `emit_size.go:18`, `emit_marshal.go:46`, `emit_unmarshal.go:190`, `emit_getter.go:16`).
- **Fix:** Introduce a `MessageContext` computed once per message holding presence map, sorted fields, pre-scan field list. Thread it through emit phases. Document phase-ordering invariants at the top of `generateFile` (one comment per edge: "Equal after structs so embedded MessageType.EmitEqual has type names").

### ARCH-8 (Low) — `field_survival_test.go` lives at `test/` root, not under any subpackage
- **Location:** `test/field_survival_test.go`.
- **What:** It's a roundtrip-survival test — same conceptual category as `test/basic/roundtrip_test.go`. Currently the only file at the top level of `test/`.
- **Fix:** Move into `test/basic/`. Update any package-name references.

### ARCH-9 (Low) — Validation errors fail-fast where they could accumulate
- **Location:** `compiler/generator/option_pointer.go:85-114` accumulates `[]string` then joins; `generator.go:80-159` is fail-fast everywhere else.
- **What:** Users who fix one validation error then rerun and discover the next have slow iteration loops. The string-concat-error pattern also loses structure — IDE integration / future tooling cannot read per-field details.
- **Fix:** Use `errors.Join` (Go 1.20+) for the validation phases (`buildImportMapping`, `collectGoPackages`, `validateDestinations`, `validatePointerOptions`). Keep `Generate` fail-fast on the first compilation/IO error.

### ARCH-10 (Low) — `compiler/generator/AGENTS.md` doesn't exist; type-side AGENTS.md is stale
- **Location:** missing `compiler/generator/AGENTS.md`; `compiler/types/AGENTS.md:6-19` omits `PointerField`/`RepeatedPointer`.
- **What:** A contributor adding a new emit phase has nothing to read. Type-side doc lists 4 composites; codebase has 6.
- **Fix:** Add `compiler/generator/AGENTS.md` listing phases in order with invariants (e.g. "enums before structs"). Update `compiler/types/AGENTS.md` to enumerate all six composites with one-line predicates, and extend the "Adding a new type" checklist to mention `ZeroLiteral`, `EmitEqual`, `EmitEncode`, `EmitMapEntryUnmarshal`.

---

## 6. Documentation & DX

### DOC-1 (High) — `README.md` scalar list is incomplete and inconsistent with `AGENTS.md`
- **Location:** `README.md:84`; cross-check `AGENTS.md:63`.
- **What:** README lists `string, bool, int32, int64, uint32, uint64, sint32, double, bytes, fixed32, fixed64, sfixed64`. Missing `sint64`, `float`, `sfixed32`. Code in `compiler/types/{sfixed32,sint64,float}.go` confirms support. README also omits maps entirely (line 82) despite full support.
- **Fix:** Sync README scalar list to AGENTS.md. Add maps to the feature list.

### DOC-2 (High) — `README.md` Usage section omits install + actual CLI invocation
- **Location:** `README.md:88-97`.
- **What:** Only "Usage" content is `make generate/build/test/bench`. No `go install`, no example `wiresmith --proto_path=... --out=...`, no mention of `--version` or `--help`. Reader cannot use the binary outside this repo's Makefile.
- **Fix:** Add "Install" section with `go install wiresmith/cmd/wiresmith@latest` and "Run" with the three real flags. Reference `--help` and `--version`.

### DOC-3 (High) — `(wiresmith.options.pointer)` field option is undocumented in user-facing docs
- **Location:** missing from `README.md`, `AGENTS.md`, `MIMIR.md`, `TEMPO.md`, `BUF.md`, `FLAGS.md`.
- **What:** PR #59 added the public extension option. Users must `import "wiresmith/options.proto"` in their own protos. Only references today are the test fixture (`proto/basic/pointer.proto`) and a source comment in `compiler/generator/embed/wiresmith/options.proto`. CLAUDE.md "Keep documentation in sync" caveat applies directly.
- **Fix:** Add "Custom options" section to README and `AGENTS.md` "Design decisions" with: import path, no-vendoring guarantee, syntax (`(wiresmith.options.pointer) = true`), what it does (`*T`/`[]*T`), where it's invalid (optional/oneof/map/scalar).

### DOC-4 (High) — `TEMPO.md` documents flags that don't exist
- **Location:** `TEMPO.md:6-23`.
- **What:** Promises `--strip_prefix`, `--import_base`, `--helpers_import`. None exist in `cmd/wiresmith/main.go`. `FLAGS.md` (untracked) proposes *not* adding them. Users following TEMPO.md will see `flag provided but not defined: -strip_prefix`.
- **Fix:** Either commit `FLAGS.md` and link to it as the resolution, or rewrite `TEMPO.md` to say "proposed; superseded by FLAGS.md". Don't leave contradictory truth claims in the tree.

### DOC-5 (Medium) — `BUF.md` and `FLAGS.md` are untracked
- **Location:** repo root (git status `??`).
- **What:** Both are substantive (4.7 KB and 8.6 KB respectively). Off-tree means they don't show in `git log`, can't be referenced from PR descriptions, and the "Keep documentation in sync" rule cannot enforce them.
- **Fix:** Commit both with clear status headers: BUF.md → "Future work / not started"; FLAGS.md → "Accepted; TEMPO.md superseded" (or "Proposal; open"). Add references in `AGENTS.md` "Design proposals" subsection.

### DOC-6 (Medium) — Generated files emit no generator version
- **Location:** `compiler/generator/generator.go:313-314` (`emitHeader`).
- **What:** Header is just `// Code generated by wiresmith. DO NOT EDIT.` + `// source: <path>`. No generator version. When a bug report arrives ("wiresmith generated wrong code"), there's no way to tell *which* wiresmith. `protoc-gen-go` emits a `// versions:` block for this exact reason.
- **Fix:** Thread `buildVersion()` from `cmd/wiresmith` into the generator and emit a `// wiresmith: v0.0.0-...` line between source and package clause.

### DOC-7 (Medium) — `MIMIR.md` references a nonexistent `GogoCompat` flag
- **Location:** `MIMIR.md:13`.
- **What:** "These... require GogoCompat flag" — no such flag exists. Most listed items (`Reset`, `ProtoMessage`, `String`, `Get*`, `Register*`, `Equal`, struct tags) ship unconditionally today.
- **Fix:** Either delete `MIMIR.md` (most of it is now obsolete) or rewrite to say "Currently emitted unconditionally; previously contemplated as gated".

### DOC-8 (Medium) — `AGENTS.md` "Generated API surface" is implicit and incomplete
- **Location:** `AGENTS.md` "Design decisions".
- **What:** Mentions `Has`, `Get`, `Reset`, `ProtoMessage`, `String` but not `Equal`, `Marshal`, `MarshalTo`, `MarshalToSizedBuffer`, `Unmarshal`, `Size`. The full surface is only knowable by reading the generated `.pb.go`.
- **Fix:** Dedicated "Generated API surface" subsection listing every method with a one-line semantics + nil-safety statement.

### DOC-9 (Medium) — CLI error for missing `proto_path` leaks OS syscall name
- **Location:** `cmd/wiresmith/main.go:51`; resulting error: `building import mapping: lstat /nonexistent: no such file or directory`.
- **What:** "lstat" is a filesystem syscall — leaks implementation detail to end users.
- **Fix:** Detect `errors.Is(err, fs.ErrNotExist)` in `Generate` and return `fmt.Errorf("proto_path %q: directory does not exist", g.ProtoDir)`. Same treatment for `out` creation failures.

### DOC-10 (Medium) — README benchmark numbers lack a date/commit anchor
- **Location:** `README.md:17`; `benchstat.txt` dated 2026-04-16.
- **What:** "+19% vs vtproto on sec/op (geomean)" is a load-bearing claim. There's no date stamp or commit reference. Reader has no way to know if numbers reflect current code.
- **Fix:** Add "as of <date>, commit <SHA>" header to the benchmark section. Optionally CI job to regenerate `benchstat.txt`.

### DOC-11 (Low) — Conformance count is duplicated across docs (fragility)
- **Location:** `AGENTS.md:69`; `test/conformance/failure_list.txt` summary; (README is silent — good).
- **What:** "695 passing, 5 expected failures" appears in both places. Today accurate; one will drift on next change.
- **Fix:** Single source of truth — `failure_list.txt` summary comment. `AGENTS.md` should say "see `test/conformance/failure_list.txt`" instead of restating the count.

### DOC-12 (Low) — `gen/protohelpers/` has minimal package documentation and no stability marker
- **Location:** `gen/protohelpers/protohelpers.go:1-3`; `registry.go` has none.
- **What:** It's the ABI between wiresmith binary and every generated file, but unmarked re: stability. `MessageType`/`EnumValueMap` are exported but never used inside the repo — reflection escape hatches with no callers and no docs.
- **Fix:** Add a `doc.go` covering: package purpose, ABI status (experimental until v0.1), and a note that `MessageType`/`EnumValueMap` are reflection escape hatches.

### DOC-13 (Low) — No end-to-end "your first .proto" example
- **Location:** missing.
- **What:** Onboarding from "I have a .proto" to "I have Go code" is undocumented. No `examples/` directory.
- **Fix:** Add `examples/hello/` with one .proto, the wiresmith command, and a snippet of the resulting `.pb.go`. Reference from README "Usage".

### DOC-14 (Low) — README and AGENTS.md frame wiresmith as OTel-only despite being a generic proto3 compiler
- **Location:** `README.md:3`; `AGENTS.md:3`.
- **What:** Both top-line descriptions say "OpenTelemetry .proto files". The generator (`compiler/generator/generator.go`) handles any proto3. One OTel heuristic in `names.go` for `opentelemetry.proto.*` paths, but that's path-mapping, not a constraint.
- **Fix:** Reword to "Custom protobuf compiler... originally designed for OpenTelemetry .proto files but supports any proto3 schema." Opens the door for non-OTel adopters.

---

## Top 10 by ROI

Ranked by `(impact × likelihood) / cost`. Read top-down to plan the next sprint of work.

| # | Item | Theme | Estimated effort | Why now |
|---|------|-------|------------------|---------|
| 1 | **CR-1** — `Reset()` nil-guard | Correctness | 5 min + test | Critical, trivial, regression-prone — already documented contract violated |
| 2 | **SEC-1 + SEC-2** — Pre-scan bound + `default: break` | Security | 1 hr + fuzz seed | DoS vector triggerable with one malicious 1 MB payload; both bugs share one diff |
| 3 | **CR-2** — Signbit-aware float zero | Correctness | 1 hr + test | Silent corruption of valid input |
| 4 | **TEST-10** — Re-enable corecursive conformance fields | Testing | 30 min | Removes 3 of 5 "expected failures" with no new code |
| 5 | **PERF-2** — 1-byte tag fast-path | Performance | 1 day + bench | 10–20% on `UnmarshalSingleSpan`; no competitor does this; clean win |
| 6 | **SEC-4** — Varint 10-byte overflow guard | Security | 30 min + test | Wire-spec compliance; CLAUDE.md already documents the requirement |
| 7 | **ARCH-1 + ARCH-2 + ARCH-3** — `Type` interface cleanup | Architecture | 1–2 days | Unlocks merging `PointerField`/`OptionalField` (ARCH-5); reduces `emitFieldUnmarshal` from ~120 to ~30 LOC |
| 8 | **TEST-1 + TEST-2** — Per-emit + per-type unit tests | Testing | 2–3 days | Localises regressions; covers CR-1, CR-2, CR-7 going forward |
| 9 | **PERF-3 + bench harness fix** — Drop `Size()` from `MarshalTo` + pre-allocated benches | Performance | 4 hr | Honest baseline + ~30% on the buffer-reuse path |
| 10 | **DOC-1 + DOC-2 + DOC-3 + DOC-4** — README & TEMPO.md alignment | Docs | 2 hr | Currently shipping incorrect docs — users hit it immediately |

Items #1, #2, #3, #6 are short, isolated, and reduce real risk — bundle them into one PR sequence. Item #7 needs a design discussion; the prior PR #60 (move equality into `Type`) is a precedent and partial first step.
