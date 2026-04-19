package types

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

// encoder is satisfied by all packable types for raw element encoding (no tag).
type encoder interface {
	EmitEncode(e Emitter, indent, access string)
}

// RepeatedField wraps a Type to handle repeated field semantics.
// Handles non-packable (message/string/bytes), packed, and unpacked encodings.
type RepeatedField struct {
	Inner    Type
	IsPacked bool
}

func (r *RepeatedField) RequiredImports() []string { return r.Inner.RequiredImports() }

func (r *RepeatedField) EmitSize(e Emitter, access string, tagSize int) {
	if !r.Inner.IsPackable() {
		if r.Inner.SizeByIndex() {
			e.Writef("\tfor i := range %s {\n", access)
			r.Inner.EmitValueSize(e, "\t\t", access+"[i]", tagSize, "n")
		} else {
			e.Writef("\tfor _, v := range %s {\n", access)
			r.Inner.EmitValueSize(e, "\t\t", "v", tagSize, "n")
		}
		e.Writef("\t}\n")
		return
	}
	if r.IsPacked {
		r.emitPackedSize(e, access, tagSize)
	} else {
		r.emitUnpackedSize(e, access, tagSize)
	}
}

func (r *RepeatedField) emitPackedSize(e Emitter, access string, tagSize int) {
	e.Writef("\tif len(%s) > 0 {\n", access)
	switch r.Inner.FixedSize() {
	case 8:
		e.Writef("\t\tdataLen := len(%s) * 8\n", access)
	case 4:
		e.Writef("\t\tdataLen := len(%s) * 4\n", access)
	case 1:
		e.Writef("\t\tdataLen := len(%s)\n", access)
	default:
		e.Writef("\t\tvar dataLen int\n")
		e.Writef("\t\tfor _, v := range %s {\n", access)
		e.Writef("\t\t\tdataLen += %s\n", r.Inner.VarintSizeExpr("v"))
		e.Writef("\t\t}\n")
	}
	e.Writef("\t\tn += %d + protowire.SizeVarint(uint64(dataLen)) + dataLen\n", tagSize)
	e.Writef("\t}\n")
}

func (r *RepeatedField) emitUnpackedSize(e Emitter, access string, tagSize int) {
	switch r.Inner.FixedSize() {
	case 8:
		e.Writef("\tn += len(%s) * %d\n", access, tagSize+8)
	case 4:
		e.Writef("\tn += len(%s) * %d\n", access, tagSize+4)
	case 1:
		e.Writef("\tn += len(%s) * %d\n", access, tagSize+1)
	default:
		e.Writef("\tfor _, v := range %s {\n", access)
		e.Writef("\t\tn += %d + %s\n", tagSize, r.Inner.VarintSizeExpr("v"))
		e.Writef("\t}\n")
	}
}

func (r *RepeatedField) EmitMarshal(e Emitter, access string, num protowire.Number) {
	AddTypeImports(e, r.Inner)
	if !r.Inner.IsPackable() {
		e.Writef("\tfor iNdEx := len(%s) - 1; iNdEx >= 0; iNdEx-- {\n", access)
		r.Inner.EmitValueMarshal(e, "\t\t", access+"[iNdEx]", num)
		e.Writef("\t}\n")
		return
	}
	if r.IsPacked {
		r.emitPackedMarshal(e, access, num)
	} else {
		e.Writef("\tfor iNdEx := len(%s) - 1; iNdEx >= 0; iNdEx-- {\n", access)
		r.Inner.EmitValueMarshal(e, "\t\t", access+"[iNdEx]", num)
		e.Writef("\t}\n")
	}
}

func (r *RepeatedField) emitPackedMarshal(e Emitter, access string, num protowire.Number) {
	e.Writef("\tif len(%s) > 0 {\n", access)

	if r.Inner.IsFixed64() || r.Inner.IsFixed32() {
		e.Writef("\t\tfor iNdEx := len(%s) - 1; iNdEx >= 0; iNdEx-- {\n", access)
		r.Inner.(encoder).EmitEncode(e, "\t\t\t", access+"[iNdEx]")
		e.Writef("\t\t}\n")
		if r.Inner.IsFixed64() {
			e.Writef("\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(len(%s)*8))\n", access)
		} else {
			e.Writef("\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(len(%s)*4))\n", access)
		}
	} else {
		e.Writef("\t\tvar j int\n")
		e.Writef("\t\tpStart := i\n")
		e.Writef("\t\tfor j = len(%s) - 1; j >= 0; j-- {\n", access)
		r.Inner.(encoder).EmitEncode(e, "\t\t\t", access+"[j]")
		e.Writef("\t\t}\n")
		e.Writef("\t\ti = protohelpers.EncodeVarint(dAtA, i, uint64(pStart-i))\n")
	}

	e.ReverseTag("\t\t", num, protowire.BytesType)
	e.Writef("\t}\n")
}

func (r *RepeatedField) EmitUnmarshal(e Emitter, access string, ctx FieldContext) {
	if r.Inner.IsPackable() {
		r.emitPackedUnmarshal(e, access, ctx)
	} else {
		r.emitNonPackableUnmarshal(e, access, ctx)
	}
}

func (r *RepeatedField) emitNonPackableUnmarshal(e Emitter, access string, ctx FieldContext) {
	r.Inner.EmitConsume(e)
	if ctx.MessageType != "" {
		e.Writef("\t\t\t%s = append(%s, %s{})\n", access, access, ctx.MessageType)
		sliceAccess := fmt.Sprintf("%s[len(%s)-1]", access, access)
		emitUnmarshalCall(e, sliceAccess, ctx.IsSamePackage)
	} else {
		e.Writef("\t\t\t%s = append(%s, %s)\n", access, access, r.Inner.CastExpr("dAtA[iNdEx:postIndex]", ctx))
	}
	e.Writef("\t\t\tiNdEx = postIndex\n")
}

func (r *RepeatedField) emitPackedUnmarshal(e Emitter, access string, ctx FieldContext) {
	e.Writef("\t\t\tif wireType == 2 {\n")

	// Inline length decode for the packed data envelope.
	emitConsumeBytesLenAt(e, "\t\t\t\t")
	e.Writef("\t\t\t\tdata := dAtA[iNdEx:postIndex]\n")

	// Pre-allocate with exact capacity
	switch r.Inner.FixedSize() {
	case 8:
		e.Writef("\t\t\t\tif elementCount := len(data) / 8; elementCount != 0 && len(%s) == 0 {\n", access)
		e.Writef("\t\t\t\t\t%s = make(%s, 0, elementCount)\n", access, ctx.SliceType)
		e.Writef("\t\t\t\t}\n")
	case 4:
		e.Writef("\t\t\t\tif elementCount := len(data) / 4; elementCount != 0 && len(%s) == 0 {\n", access)
		e.Writef("\t\t\t\t\t%s = make(%s, 0, elementCount)\n", access, ctx.SliceType)
		e.Writef("\t\t\t\t}\n")
	case 1:
		e.Writef("\t\t\t\tif elementCount := len(data); elementCount != 0 && len(%s) == 0 {\n", access)
		e.Writef("\t\t\t\t\t%s = make(%s, 0, elementCount)\n", access, ctx.SliceType)
		e.Writef("\t\t\t\t}\n")
	default:
		e.Writef("\t\t\t\tvar elementCount int\n")
		e.Writef("\t\t\t\tfor _, b := range data {\n")
		e.Writef("\t\t\t\t\tif b < 128 {\n")
		e.Writef("\t\t\t\t\t\telementCount++\n")
		e.Writef("\t\t\t\t\t}\n")
		e.Writef("\t\t\t\t}\n")
		e.Writef("\t\t\t\tif elementCount != 0 && len(%s) == 0 {\n", access)
		e.Writef("\t\t\t\t\t%s = make(%s, 0, elementCount)\n", access, ctx.SliceType)
		e.Writef("\t\t\t\t}\n")
	}

	// Decode loop (uses protowire on bounded data slice)
	e.Writef("\t\t\t\tfor len(data) > 0 {\n")
	if r.Inner.IsFixed64() {
		e.Writef("\t\t\t\t\tv, vn := protowire.ConsumeFixed64(data)\n")
		e.Writef("\t\t\t\t\tif vn < 0 {\n\t\t\t\t\t\treturn fmt.Errorf(\"invalid packed fixed64\")\n\t\t\t\t\t}\n")
	} else if r.Inner.IsFixed32() {
		e.Writef("\t\t\t\t\tv, vn := protowire.ConsumeFixed32(data)\n")
		e.Writef("\t\t\t\t\tif vn < 0 {\n\t\t\t\t\t\treturn fmt.Errorf(\"invalid packed fixed32\")\n\t\t\t\t\t}\n")
	} else {
		e.Writef("\t\t\t\t\tv, vn := protowire.ConsumeVarint(data)\n")
		e.Writef("\t\t\t\t\tif vn < 0 {\n\t\t\t\t\t\treturn fmt.Errorf(\"invalid packed varint\")\n\t\t\t\t\t}\n")
	}
	e.Writef("\t\t\t\t\t%s = append(%s, %s)\n", access, access, r.Inner.CastExpr("v", ctx))
	e.Writef("\t\t\t\t\tdata = data[vn:]\n")
	e.Writef("\t\t\t\t}\n")
	e.Writef("\t\t\t\tiNdEx = postIndex\n")

	// Non-packed single element path (inline decode)
	nativeWtInt := 0 // varint by default
	if r.Inner.IsFixed64() {
		nativeWtInt = 1
	} else if r.Inner.IsFixed32() {
		nativeWtInt = 5
	}
	e.Writef("\t\t\t} else if wireType == %d {\n", nativeWtInt)
	if r.Inner.IsFixed64() {
		emitConsumeFixed64At(e, "\t\t\t\t")
	} else if r.Inner.IsFixed32() {
		emitConsumeFixed32At(e, "\t\t\t\t")
	} else {
		emitConsumeVarintAt(e, "\t\t\t\t")
	}
	e.Writef("\t\t\t\t%s = append(%s, %s)\n", access, access, r.Inner.CastExpr("v", ctx))

	// Skip unknown wire type
	e.Writef("\t\t\t} else {\n")
	e.Writef("\t\t\t\tn, err := skipValue(dAtA[iNdEx:], wireType)\n")
	e.Writef("\t\t\t\tif err != nil {\n\t\t\t\t\treturn err\n\t\t\t\t}\n")
	e.Writef("\t\t\t\tiNdEx += n\n")
	e.Writef("\t\t\t}\n")
}
