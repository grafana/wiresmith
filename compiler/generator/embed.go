package generator

import _ "embed"

// embeddedOptionsProto is the source of `wiresmith/options.proto`, served by
// memResolver under that canonical import path so user .proto files can
// `import "wiresmith/options.proto";` without vendoring it themselves.
//
//go:embed embed/wiresmith/options.proto
var embeddedOptionsProto []byte

// embeddedOptionsPath is the canonical import path under which the resolver
// makes the embedded options proto available. It also doubles as the identity
// of the embedded schema file in code generation — the Generate loop skips
// any compiled file whose path matches this constant.
const embeddedOptionsPath = "wiresmith/options.proto"

// embeddedOptionsImportPath / embeddedOptionsPkgName are sentinel Go-side
// identifiers for `wiresmith/options.proto`. The file is generator-internal
// and has no Go output (computeDests skips internal schemas), so these
// strings never appear in any emitted code. They exist solely so emit_grpc
// can register a Dest entry for the embedded schema when it shows up as a
// transitive import of a user file — the grpc bridge's `buildParameter`
// fails closed on any transitive import without a Dest, by design.
const (
	embeddedOptionsImportPath = "github.com/grafana/wiresmith/internal/wiresmithoptions"
	embeddedOptionsPkgName    = "wiresmithoptionspb"
)
