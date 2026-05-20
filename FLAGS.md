# Proposal: Eliminate `--strip_prefix`, `--import_base`, `--helpers_import`

[TEMPO.md](TEMPO.md) lists three CLI flags as planned additions for the Tempo
integration:

```
--strip_prefix    Proto package prefix to strip for Go paths (e.g. "tempopb")
--import_base     Go import path base (replaces module/gen/ prefix)
--helpers_import  Import path for protohelpers package
```

Other Go protobuf compilers — `protoc-gen-go`, `protoc-gen-gogofaster`,
`protoc-gen-go-vtproto` — don't have flags like this. They get the same
configurability from two existing protoc-native mechanisms: the
`paths=source_relative` output mode and a fixed, published runtime helpers
import path. This proposal eliminates the three flags by adopting both.

## Why the flags exist

Wiresmith currently derives output locations from the proto **package**, not the
source **path**. In `compiler/generator/names.go`:

```go
func effectiveBase(module string) string {
    return module + "/gen"
}

func goPackageDir(protoPkg string) string {
    parts := strings.Split(protoPkg, ".")
    if len(parts) >= 3 && parts[0] == "opentelemetry" && parts[1] == "proto" {
        return "otlp/" + strings.Join(parts[2:], "/")
    }
    return strings.Join(parts, "/")
}
```

`destFor` builds the output directory from `effectiveBase(module) + goPackageDir(protoPkg)`,
and an `option go_package` is only honored when it falls under
`effectiveBase`. Two hardcodes are baked into this:

1. The `/gen` suffix on `effectiveBase` — needed `--import_base` to override.
2. The `opentelemetry.proto.*` → `otlp/...` special case in `goPackageDir`,
   plus the dotted-package-as-path fallback — needed `--strip_prefix` to
   re-shape for non-OTel package layouts.

The `protohelpers` import is hardcoded similarly in
`compiler/generator/emit_registration.go` and `types.go`:

```go
fg.imports.addImport(fg.module+"/gen/protohelpers", "")
```

— needed `--helpers_import` to override.

## How protoc-based plugins solve the same problem

### Output path: `paths=source_relative`

protoc-gen-go (and every plugin that wraps protoc) supports
`paths=source_relative`. The output location of a generated file is purely
`<--go_out>/<source-relative-path>.pb.go`. The proto package is *not*
consulted to compute the path.

Cross-file imports between generated files are resolved from each input
file's `option go_package`. The `go_package` is the single source of truth
for "what import path does this generated file have"; the on-disk location
just has to agree with it because Go's module layout requires it.

For Tempo this means: a patched proto at
`pkg/.patched-proto/common/v1/common.proto` with
`option go_package = "github.com/grafana/tempo/pkg/tempopb/common/v1";` and
`--go_out=pkg/tempopb --go_opt=paths=source_relative` produces
`pkg/tempopb/common/v1/common.pb.go`. No `gen/` in the path, no per-package
flags. The proto file's own metadata fully specifies the layout.

### Runtime helpers: fixed module path

vtprotobuf publishes its runtime support at the stable Go module path
`github.com/planetscale/vtprotobuf/protohelpers`. Generated code imports
that path verbatim. There is no per-codegen flag for "where do helpers
live" because the answer is always the same: at the published module path.

Consumers don't vendor helpers under their own tree; they take a Go module
dependency like any other library.

## Proposed changes to wiresmith

### 1. Drop proto-package-based path derivation; use source-relative paths

Remove `effectiveBase` and `goPackageDir` from
`compiler/generator/names.go`. The output path becomes:

```
<--out>/<source-file's-path-relative-to---proto_path>.pb.go
```

Concretely, for input `pkg/.patched-proto/common/v1/common.proto` invoked
with `--proto_path=pkg/.patched-proto --out=pkg/tempopb`, the output is
`pkg/tempopb/common/v1/common.pb.go`. This is the `paths=source_relative`
contract.

### 2. Always honor `go_package`; require it when imports cross packages

`resolveGoPackage` currently gates `go_package` behind "is the import path
under our `effectiveBase`?". Drop that gate. If a proto file sets
`option go_package`, that string is the generated file's import path,
unconditionally. Cross-file imports between wiresmith-generated files read
each other's `go_package` directly, the same way protoc does.

For a single-file generation with no cross-file imports, `go_package` can
be omitted and the import path is derived from `--out` + source-relative
path + the consumer's Go module root. (Or we can require `go_package`
always, matching protoc-gen-go's strictness — easier to reason about.)

### 3. Drop the `opentelemetry.proto.*` special-case

Once `go_package` is the sole authority, the `otlp/` prefix shortcut in
`goPackageDir` is dead code. The OTLP protos already set
`option go_package = "go.opentelemetry.io/proto/otlp/common/v1"`, which
under the new model is exactly what determines their output location.
Remove the special-case branch.

### 4. Publish protohelpers as a stable Go module

Move `gen/protohelpers/` out of the wiresmith generated tree and publish it
as a standalone Go module — for example `github.com/wiresmith/protohelpers`
(name/path TBD; the key property is that it's importable by any consumer
without being co-located with their generated code).

Update `compiler/generator/emit_registration.go:28` and `types.go:62` so
the generated import is the fixed published path, not `fg.module +
"/gen/protohelpers"`. The `--module` flag stops influencing the helpers
import entirely.

This is what `vtprotobuf/protohelpers`, `gogoproto/proto`, and
`google.golang.org/protobuf/runtime/...` all do — runtime support lives at
a stable, public import path.

## Resulting consumer experience

Tempo's invocation becomes:

```bash
wiresmith \
  --proto_path=pkg/.patched-proto \
  --out=pkg/tempopb
```

— with each patched proto setting its `option go_package` to
`github.com/grafana/tempo/pkg/tempopb/<X>/v1` (which Tempo's `gen-proto`
make target already does for the gogofast pass). The `--module`,
`--strip_prefix`, `--import_base`, and `--helpers_import` flags all
disappear; the only remaining flags are `--proto_path` and `--out`, both of
which match protoc's `--proto_path` / `--go_out` and need no explanation
to anyone who has used protoc before.

Generated files land at `pkg/tempopb/common/v1/common.pb.go` directly —
no `gen/` subdirectory and no 101-file import rewrite of Tempo consumers,
which is the diff-pollution cost of the current workaround documented in
[TEMPO.md](TEMPO.md).

## What this proposal does not change

- The `--module` flag stays in service of other things if needed (currently
  it only feeds `effectiveBase` and the helpers path, both of which this
  proposal removes — so `--module` becomes dead and should also be
  dropped).
- The recursive directory scan in `buildImportMapping` and the
  `outputPathFor` collision detection stay as-is; they already work in
  source-relative terms.
- The `go_package` parsing in `parseGoPackage` (including `;name` form and
  Go-keyword sanitization in `cleanPackageName`) is unaffected.

## Migration path for existing wiresmith consumers

The wiresmith codebase itself uses `--module=wiresmith --out=gen` and
relies on `effectiveBase = "wiresmith/gen"` plus the OTel special-case to
produce `gen/otlp/common/v1/common.pb.go` from
`package opentelemetry.proto.common.v1`. Under this proposal, those protos
need `option go_package = "wiresmith/gen/otlp/common/v1";` (or wherever
wiresmith wants them to land) and invocation drops `--module`.

This is a one-time edit per generated proto package in the wiresmith repo,
done at the same commit that lands the codegen change. After that, the
codegen layer has no module-level configuration at all.

## Why this is better than just adding the flags

The flags solve the immediate Tempo need but bake in the proto-package →
directory derivation as a permanent feature of wiresmith's design. Every
future consumer with a layout that doesn't match `module/gen/<package>`
has to learn the flag combination. The protoc-native model has no
equivalent learning curve because the same `paths=source_relative`
semantics work for every consumer — the proto file's own `go_package`
carries the information, and there's nothing to override at the CLI.

The helpers-as-published-module change is also strictly cleaner: it
removes a class of "where is protohelpers" questions for downstream
consumers, makes wiresmith play nicely with `go mod tidy`, and matches
how every other Go protobuf runtime is distributed.
