package generator

import _ "embed"

// embeddedOptionsProto is the source of `wiresmith/options.proto`, served by
// memResolver under that canonical import path so user .proto files can
// `import "wiresmith/options.proto";` without vendoring it themselves.
//
//go:embed embed/wiresmith/options.proto
var embeddedOptionsProto []byte

// embeddedOptionsPath is the canonical import path under which the resolver
// makes the embedded options proto available.
const embeddedOptionsPath = "wiresmith/options.proto"

// embeddedOptionsPackage is the proto package declared inside
// embeddedOptionsProto. The Generate loop uses it to skip code generation —
// the file only carries the extension definition, not user types.
const embeddedOptionsPackage = "wiresmith.options"
