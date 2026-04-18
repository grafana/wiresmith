package types

import "google.golang.org/protobuf/reflect/protoreflect"

func init() {
	register(protoreflect.Int64Kind, &varintBase{unmarshalCast: "int64(%s)"})
}
