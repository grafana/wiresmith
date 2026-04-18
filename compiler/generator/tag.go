package generator

import (
	"google.golang.org/protobuf/encoding/protowire"
)

func computeTagBytes(num protowire.Number, typ protowire.Type) []byte {
	return protowire.AppendTag(nil, num, typ)
}
