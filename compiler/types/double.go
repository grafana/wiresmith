package types

import "google.golang.org/protobuf/reflect/protoreflect"

func init() {
	register(protoreflect.DoubleKind, &fixed64Base{
		putExpr: "math.Float64bits(%s)",
		getExpr: "math.Float64frombits(%s)",
		imports: []string{"encoding/binary", "math"},
	})
}
