package generator

import (
	"fmt"

	"github.com/grafana/wiresmith/compiler/generator/grpc"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// emitGRPC writes the companion `<name>_grpc.pb.go` file for fd via the
// vendored protoc-gen-go-grpc generator. Returns silently when fd
// declares no services — the grpc bridge short-circuits the same case,
// but checking up front avoids walking allFiles to build the dest map.
//
// Unlike the other emitters in this package, emit_grpc does not write
// into a FileGenerator buffer: the upstream generator owns its own
// gofmt-formatted output via protogen.GeneratedFile.Content, and the
// stubs reference grpc / status / codes packages that have no overlap
// with the main .pb.go's import block. Recording the bytes directly as
// a new GeneratedFile keeps the icache rationale documented for the
// _reflect / _compare / _equal splits intact (the gRPC file is also a
// cold-path companion).
func (g *Generator) emitGRPC(fd protoreflect.FileDescriptor) error {
	if fd.Services().Len() == 0 {
		return nil
	}

	dests := make(map[string]grpc.Dest, len(g.destinations))
	for path, d := range g.destinations {
		dests[path] = grpc.Dest{ImportPath: d.importPath, PkgName: d.pkgName}
	}

	content, err := grpc.Generate(fd, dests)
	if err != nil {
		return fmt.Errorf("generating grpc stubs: %w", err)
	}
	if content == nil {
		// Defensive: the Services().Len() guard above means we should never
		// reach here, but the bridge's contract permits a nil return when
		// the file has no services. Surface the divergence loudly rather
		// than silently skipping a file the collision-check expected.
		return fmt.Errorf("grpc bridge returned no content for %q despite %d services", fd.Path(), fd.Services().Len())
	}

	// grpc.Generate returns gofmt-formatted bytes already; bypass
	// writeFormatted (which would re-run go/format on already-clean
	// source) and record the output directly.
	g.outputs = append(g.outputs, GeneratedFile{
		Path:    g.outputGrpcPathFor(fd),
		Content: content,
	})
	return nil
}
