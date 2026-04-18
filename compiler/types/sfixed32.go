package types

import "google.golang.org/protobuf/reflect/protoreflect"

func init() {
	register(protoreflect.Sfixed32Kind, &fixed32Base{
		putExpr: "uint32(%s)",
		getExpr: "int32(%s)",
		imports: []string{"encoding/binary"},
	})
}
