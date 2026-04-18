package types

import "google.golang.org/protobuf/reflect/protoreflect"

func init() {
	register(protoreflect.Sfixed64Kind, &fixed64Base{
		putExpr: "uint64(%s)",
		getExpr: "int64(%s)",
		imports: []string{"encoding/binary"},
	})
}
