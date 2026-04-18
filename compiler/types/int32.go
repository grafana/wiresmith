package types

import "google.golang.org/protobuf/reflect/protoreflect"

func init() {
	register(protoreflect.Int32Kind, &varintBase{unmarshalCast: "int32(%s)"})
}
