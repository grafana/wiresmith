// SPDX-License-Identifier: Apache-2.0
//
// bridge.go is the wiresmith-specific glue around the vendored upstream
// generator. It is the ONLY file in this package that wiresmith adds on top
// of the upstream source; everything in grpc.go is byte-identical to
// google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.0 so future upstream
// re-syncs are a straight `cp` plus a `package main` → `package grpc` swap.
package grpc

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// Dest is the Go destination wiresmith assigns to one .proto file. The
// caller looks identical to protoc's `M<source>=<importpath>;<pkgname>`
// flag form — wiresmith resolves it via its own goDest table and hands the
// resolved values in here, so protogen-internal go_package resolution
// never disagrees with what wiresmith emitted for the main .pb.go file.
type Dest struct {
	ImportPath string
	PkgName    string
}

// Generate emits the contents of `<file>_grpc.pb.go` for fd, or returns
// (nil, nil) when fd declares no services. dests must hold a Dest for fd
// itself and every file fd transitively imports — those are exactly the
// files protogen needs to resolve cross-package symbols in the generated
// stubs. A missing entry is rejected up front (see buildParameter) so the
// caller cannot silently regress to protogen's go_package fallback.
//
// The returned bytes are already gofmt-formatted by protogen (its
// GeneratedFile.Content parses + reprints via go/format), so wiresmith's
// caller can write them straight to disk without another format pass.
func Generate(fd protoreflect.FileDescriptor, dests map[string]Dest) ([]byte, error) {
	if fd.Services().Len() == 0 {
		return nil, nil
	}

	files := transitiveFiles(fd)

	protoFiles := make([]*descriptorpb.FileDescriptorProto, len(files))
	for i, f := range files {
		protoFiles[i] = protodesc.ToFileDescriptorProto(f)
	}

	parameter, err := buildParameter(files, dests)
	if err != nil {
		return nil, err
	}

	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{fd.Path()},
		Parameter:      proto.String(parameter),
		ProtoFile:      protoFiles,
	}

	plugin, err := protogen.Options{}.New(req)
	if err != nil {
		return nil, fmt.Errorf("protogen.New: %w", err)
	}
	// Mirror upstream protoc-gen-go-grpc's feature advertisement so the
	// generator behaves identically — proto3 optional support is the only
	// one that affects the emitted code via the vendored grpc.go path.
	plugin.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)

	var emitted bool
	for _, f := range plugin.Files {
		if !f.Generate {
			continue
		}
		generateFile(plugin, f)
		emitted = true
	}
	if !emitted {
		return nil, fmt.Errorf("protogen did not surface %q as a file to generate", fd.Path())
	}

	resp := plugin.Response()
	if e := resp.GetError(); e != "" {
		// protogen surfaces plugin failures as a string on the response,
		// not an `error` value, so there's nothing to `%w`-wrap. Add the
		// file path so the caller stack identifies which proto blew up
		// rather than just echoing protogen's bare message.
		return nil, fmt.Errorf("protogen reported error generating %q: %s", fd.Path(), e)
	}
	// generateFile is skipped when len(file.Services) == 0 (upstream); we
	// guarded the same case above. If we reach here with no Response files
	// the upstream generator's contract has changed — surface it loudly.
	if len(resp.File) == 0 {
		return nil, fmt.Errorf("protogen produced no output for %q", fd.Path())
	}
	if len(resp.File) > 1 {
		return nil, fmt.Errorf("protogen produced %d files for %q, want 1", len(resp.File), fd.Path())
	}
	return []byte(resp.File[0].GetContent()), nil
}

// transitiveFiles returns fd plus every file it (transitively) imports, in
// dependency order (a file appears after every file it depends on). The
// order matches protogen's gatherTransitiveDependencies expectation.
func transitiveFiles(fd protoreflect.FileDescriptor) []protoreflect.FileDescriptor {
	seen := make(map[string]bool)
	var out []protoreflect.FileDescriptor
	var walk func(f protoreflect.FileDescriptor)
	walk = func(f protoreflect.FileDescriptor) {
		if seen[f.Path()] {
			return
		}
		seen[f.Path()] = true
		imps := f.Imports()
		for i := 0; i < imps.Len(); i++ {
			walk(imps.Get(i).FileDescriptor)
		}
		out = append(out, f)
	}
	walk(fd)
	return out
}

// buildParameter renders the M-flag entries protogen consults when
// resolving go_package — namely for files protogen sees in ProtoFile
// (fd plus its transitive imports). dests entries for files outside that
// set are dropped: protogen never reads them, so building and parsing
// their M-params is pure overhead that grows with the total compiled-
// file count rather than the import depth of this service file.
//
// A transitive file missing from dests is an error: protogen would
// otherwise fall back to the file's own go_package option, which can
// diverge from the destination wiresmith resolved for that file and
// embed inconsistent imports / package names in the generated stub.
// Surface the gap loudly rather than letting it slip into the output.
//
// Sorted alphabetically so the same inputs always produce the same
// parameter string — `splitImportPathAndPackageName` is order-
// independent, but determinism makes plugin-mode behaviour reproducible
// and is cheap to provide here.
func buildParameter(files []protoreflect.FileDescriptor, dests map[string]Dest) (string, error) {
	parts := make([]string, 0, len(files))
	for _, f := range files {
		d, ok := dests[f.Path()]
		if !ok {
			return "", fmt.Errorf("no destination for transitive import %q", f.Path())
		}
		// Always emit the ;pkgname suffix so protogen's pkgName derivation
		// never falls back to `cleanPackageName(path.Base(importPath))` —
		// that fallback can rename `v1` segments and would diverge from
		// the package clause wiresmith wrote into the main .pb.go.
		parts = append(parts, fmt.Sprintf("M%s=%s;%s", f.Path(), d.ImportPath, d.PkgName))
	}
	sort.Strings(parts)
	return strings.Join(parts, ","), nil
}
