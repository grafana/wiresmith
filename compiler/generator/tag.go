package generator

import (
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func computeTagBytes(num protowire.Number, typ protowire.Type) []byte {
	return protowire.AppendTag(nil, num, typ)
}

func isPackable(k protoreflect.Kind) bool {
	switch k {
	case protoreflect.BoolKind,
		protoreflect.EnumKind,
		protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Uint32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Uint64Kind,
		protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind, protoreflect.FloatKind,
		protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind, protoreflect.DoubleKind:
		return true
	default:
		return false
	}
}

func isFixed32Kind(k protoreflect.Kind) bool {
	switch k {
	case protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind, protoreflect.FloatKind:
		return true
	default:
		return false
	}
}

func isFixed64Kind(k protoreflect.Kind) bool {
	switch k {
	case protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind, protoreflect.DoubleKind:
		return true
	default:
		return false
	}
}
