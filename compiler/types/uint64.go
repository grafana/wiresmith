package types

import "google.golang.org/protobuf/reflect/protoreflect"

func init() {
	register(protoreflect.Uint64Kind, &varintBase{unmarshalCast: "%s"})
}
