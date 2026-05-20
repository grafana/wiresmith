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
