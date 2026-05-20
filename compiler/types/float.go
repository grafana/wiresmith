package types

import "google.golang.org/protobuf/reflect/protoreflect"

func init() {
	register(protoreflect.FloatKind, &fixed32Base{
		putExpr:     "math.Float32bits(%s)",
		getExpr:     "math.Float32frombits(%s)",
		nonzeroExpr: "math.Float32bits(%s) != 0",
		imports:     []string{"encoding/binary", "math"},
	})
}
