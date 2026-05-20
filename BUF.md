# buf Compatibility: Required Changes

What it would take to make wiresmith usable from `buf generate` and (optionally) publishable to the buf.build plugin registry. Today wiresmith is invoked as a standalone CLI (`cmd/wiresmith/main.go`) that parses `.proto` files directly via `bufbuild/protocompile` — buf integration is a separate entry path, not a rewrite.

Layers are concentric: the plugin protocol is the minimum, `buf generate` integration makes it useful, and the registry layer is only needed for remote publishing.

## 1. Plugin protocol (`protoc-gen-wiresmith` binary)

These items are the wire contract `buf generate` uses to invoke plugins (identical to protoc plugins).

### New entry point
A `cmd/protoc-gen-wiresmith/main.go` using `google.golang.org/protobuf/compiler/protogen`:

```
protogen.Options{ParamFunc: f.Set}.Run(func(plugin *protogen.Plugin) error { ... })
```

Reads `CodeGeneratorRequest` from stdin, writes `CodeGeneratorResponse` to stdout. Reference shape: vtproto's `cmd/protoc-gen-go-vtproto/main.go`.

### Generator must accept descriptors, not a directory
`Generator.Generate()` at `compiler/generator/generator.go:68` is hardwired to `protocompile.Compile(importPaths...)`. For plugin mode, descriptors arrive pre-linked from buf — the generator needs a second entry path that takes `linker.Files` (or a `protoreflect.FileDescriptor` slice) directly. The downstream emit code is already descriptor-based, so this is a parser-detach rather than a rewrite.

### Stop writing to disk
Replace `os.MkdirAll` + `os.WriteFile` at `compiler/generator/generator.go:238-242` with appending `*pluginpb.CodeGeneratorResponse_File` entries. buf places the files; the plugin doesn't.

### Filter to `FilesToGenerate`
`CodeGeneratorRequest` ships every transitive import, but the plugin must only emit for files listed in `FilesToGenerate`. Current code generates for every result from `protocompile.Compile`.

### Plugin parameters
Parse `opt:` strings from `buf.gen.yaml` — at minimum `module=` (matching buf's strip-prefix semantic). Bind via `flag.FlagSet` and `protogen.Options.ParamFunc`.

### Return errors via the response
The validation errors at `compiler/generator/generator.go:101-108` (output collisions), `generator.go:155-159` (inconsistent `go_package`), and `generator.go:177` (`..` in `go_package`) must go into `CodeGeneratorResponse.Error` instead of bubbling up. stderr is fine for warnings; fatal results belong in the response.

### Don't emit empty files
If a generated file would be empty (e.g., proto contains only services that wiresmith doesn't emit), suppress it. vtproto exposes `--allow-empty` because protoc rejects empties — wiresmith should default to suppression.

## 2. `buf generate` integration

Required for the plugin to be useful inside a buf workflow, even though the protocol works without them.

### Honor `paths=source_relative`
buf's default places files at `<source_relative>/<basename>.pb.go`. wiresmith currently derives paths from `option go_package` + `--module` (see `outputPathFor` at `compiler/generator/generator.go:121`). Either support both modes or document a wiresmith-only convention and accept the divergence.

### Well-known types
buf-managed protos routinely import `google/protobuf/{timestamp,duration,any,empty,struct,fieldmask}.proto`. wiresmith currently lists these as unsupported (see CLAUDE.md "Supported proto3 features"). For buf use this is the most common blocker — either generate stubs for the WKT package, or alias to `google.golang.org/protobuf/types/known/*` (which means tolerating non-wiresmith struct types crossing field boundaries).

### Sample `buf.gen.yaml`
One snippet in the README:

```yaml
plugins:
  - local: protoc-gen-wiresmith
    out: gen
    opt:
      - module=wiresmith
```

## 3. buf.build remote plugin registry

Only required when publishing to the public buf.build registry.

### `buf.plugin.yaml`
Plugin metadata: name, version, runtime (Go binary or Docker), dependencies.

### Reproducible build
Dockerfile or pinned Go toolchain version that buf.build's CI can reproduce.

### Stable versioning
SemVer tags so `buf generate` can pin a version.

## Suggested order

1. Items 1.1–1.6 plus the doc snippet from 2.3 — minimum viable plugin.
2. WKT support (2.2) — biggest real-world unblocker; without it most buf modules won't compile their generated output.
3. `paths=source_relative` (2.1) — quality of life for buf-default users.
4. Registry layer (3.x) — only if/when remote publishing becomes a goal.

The disk-writing refactor in 1.3 is also a natural win for testing: `TestGenerateMatchesCheckedIn` could verify response contents in memory instead of round-tripping through `gen/`.
