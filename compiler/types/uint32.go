package types

import "google.golang.org/protobuf/reflect/protoreflect"

func init() {
	register(protoreflect.Uint32Kind, &varintBase{unmarshalCast: "uint32(%s)"})
}
