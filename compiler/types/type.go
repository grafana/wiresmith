package types

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// FieldType is the unified interface for all field code generation.
// Callers use this to emit size, marshal, and unmarshal code.
type FieldType interface {
	EmitSize(e Emitter, access string, tagSize int)
	EmitMarshal(e Emitter, access string, num protowire.Number)
	EmitUnmarshal(e Emitter, access string, ctx FieldContext)
	RequiredImports() []string
}

// Type is the per-kind interface for scalar protobuf types.
// It implements FieldType for the singular field case (with zero-value guard).
type Type interface {
	FieldType

	// Classification
	WireType() string
	IsPackable() bool
	IsFixed32() bool
	IsFixed64() bool
	FixedSize() int    // 0=variable, 1=bool, 4=fixed32, 8=fixed64
	SizeByIndex() bool // true if repeated size loop uses index access

	// Value-level emission (no zero-guard, used by composites).
	// target is the accumulation variable ("n" or "entrySize").
	EmitValueSize(e Emitter, indent, access string, tagSize int, target string)
	EmitValueMarshal(e Emitter, indent, access string, num protowire.Number)
	VarintSizeExpr(access string) string

	// For Optional composite
	OptionalAccess(access string) string

	// Unmarshal building blocks used by composites.
	EmitConsume(e Emitter)                            // emit consume + error check (sets v, n)
	CastExpr(varName string, ctx FieldContext) string // expression to convert decoded value
	EmitMapEntryUnmarshal(e Emitter, varName, indent string, ctx FieldContext)
}

// Emitter provides code-emission primitives. Implemented by FileGenerator.
type Emitter interface {
	Writef(format string, args ...any)
	ReverseTag(indent string, num protowire.Number, wt protowire.Type)
	AddImport(path, alias string)
}

// FieldContext carries field-specific metadata for unmarshal.
type FieldContext struct {
	EnumType      string
	MessageType   string
	IsSamePackage bool
	SliceType     string
}

// registry maps protoreflect.Kind values to Type implementations.
var registry [30]Type

// Get returns the Type for a protoreflect.Kind.
// It panics with a clear error if the kind is unsupported or unregistered.
func Get(kind protoreflect.Kind) Type {
	if int(kind) >= len(registry) {
		panic(fmt.Sprintf("unsupported protoreflect.Kind: %v", kind))
	}
	t := registry[kind]
	if t == nil {
		panic(fmt.Sprintf("unregistered protoreflect.Kind: %v", kind))
	}
	return t
}

func register(kind protoreflect.Kind, t Type) {
	registry[kind] = t
}

// ForField constructs the appropriate FieldType for a field descriptor.
// For singular fields it returns the scalar Type directly.
// For optional/repeated/map fields it wraps in a composite.
func ForField(fd protoreflect.FieldDescriptor) FieldType {
	if fd.IsMap() {
		return &MapField{
			Key: Get(fd.MapKey().Kind()),
			Val: Get(fd.MapValue().Kind()),
		}
	}
	inner := Get(fd.Kind())
	if fd.IsList() {
		return &RepeatedField{Inner: inner, IsPacked: fd.IsPacked()}
	}
	if fd.HasOptionalKeyword() {
		return &OptionalField{Inner: inner}
	}
	return inner
}

// addTypeImports registers all imports required by a FieldType on the Emitter.
func AddTypeImports(e Emitter, ft FieldType) {
	for _, imp := range ft.RequiredImports() {
		e.AddImport(imp, "")
	}
}

// --- Inline consume helpers ---
// These emit inline decoding using the iNdEx/dAtA/l variables that are
// in scope in the generated unmarshal method. They replace the old
// protowire.ConsumeXxx function calls to eliminate call overhead.

// emitConsumeVarintAt emits an inline varint decode loop at the given indent.
// Sets v (uint64) in generated code, advances iNdEx.
func emitConsumeVarintAt(e Emitter, indent string) {
	e.AddImport("io", "")
	e.Writef("%svar v uint64\n", indent)
	e.Writef("%sfor shift := uint(0); ; shift += 7 {\n", indent)
	e.Writef("%s\tif shift >= 64 {\n%s\t\treturn fmt.Errorf(\"proto: integer overflow\")\n%s\t}\n", indent, indent, indent)
	e.Writef("%s\tif iNdEx >= l {\n%s\t\treturn io.ErrUnexpectedEOF\n%s\t}\n", indent, indent, indent)
	e.Writef("%s\tb := dAtA[iNdEx]\n", indent)
	e.Writef("%s\tiNdEx++\n", indent)
	e.Writef("%s\tv |= uint64(b&0x7F) << shift\n", indent)
	e.Writef("%s\tif b < 0x80 {\n%s\t\tbreak\n%s\t}\n", indent, indent, indent)
	e.Writef("%s}\n", indent)
}

func emitConsumeVarint(e Emitter) { emitConsumeVarintAt(e, "\t\t\t") }

// emitConsumeFixed32At emits inline fixed32 decoding at the given indent.
// Sets v (uint32) in generated code, advances iNdEx by 4.
func emitConsumeFixed32At(e Emitter, indent string) {
	e.AddImport("io", "")
	e.AddImport("encoding/binary", "")
	e.Writef("%sif (iNdEx + 4) > l {\n%s\treturn io.ErrUnexpectedEOF\n%s}\n", indent, indent, indent)
	e.Writef("%sv := binary.LittleEndian.Uint32(dAtA[iNdEx:])\n", indent)
	e.Writef("%siNdEx += 4\n", indent)
}

func emitConsumeFixed32(e Emitter) { emitConsumeFixed32At(e, "\t\t\t") }

// emitConsumeFixed64At emits inline fixed64 decoding at the given indent.
// Sets v (uint64) in generated code, advances iNdEx by 8.
func emitConsumeFixed64At(e Emitter, indent string) {
	e.AddImport("io", "")
	e.AddImport("encoding/binary", "")
	e.Writef("%sif (iNdEx + 8) > l {\n%s\treturn io.ErrUnexpectedEOF\n%s}\n", indent, indent, indent)
	e.Writef("%sv := binary.LittleEndian.Uint64(dAtA[iNdEx:])\n", indent)
	e.Writef("%siNdEx += 8\n", indent)
}

func emitConsumeFixed64(e Emitter) { emitConsumeFixed64At(e, "\t\t\t") }

// emitConsumeBytesLenAt emits inline length-delimited header decoding.
// Sets postIndex in generated code. Caller uses dAtA[iNdEx:postIndex]
// for the payload, then advances with iNdEx = postIndex.
func emitConsumeBytesLenAt(e Emitter, indent string) {
	e.AddImport("io", "")
	e.Writef("%svar byteLen uint64\n", indent)
	e.Writef("%sfor shift := uint(0); ; shift += 7 {\n", indent)
	e.Writef("%s\tif shift >= 64 {\n%s\t\treturn fmt.Errorf(\"proto: integer overflow\")\n%s\t}\n", indent, indent, indent)
	e.Writef("%s\tif iNdEx >= l {\n%s\t\treturn io.ErrUnexpectedEOF\n%s\t}\n", indent, indent, indent)
	e.Writef("%s\tb := dAtA[iNdEx]\n", indent)
	e.Writef("%s\tiNdEx++\n", indent)
	e.Writef("%s\tbyteLen |= uint64(b&0x7F) << shift\n", indent)
	e.Writef("%s\tif b < 0x80 {\n%s\t\tbreak\n%s\t}\n", indent, indent, indent)
	e.Writef("%s}\n", indent)
	e.Writef("%sintByteLen := int(byteLen)\n", indent)
	e.Writef("%sif intByteLen < 0 {\n%s\treturn fmt.Errorf(\"proto: negative length\")\n%s}\n", indent, indent, indent)
	e.Writef("%spostIndex := iNdEx + intByteLen\n", indent)
	e.Writef("%sif postIndex < 0 {\n%s\treturn fmt.Errorf(\"proto: negative length\")\n%s}\n", indent, indent, indent)
	e.Writef("%sif postIndex > l {\n%s\treturn io.ErrUnexpectedEOF\n%s}\n", indent, indent, indent)
}

func emitConsumeBytesLen(e Emitter) { emitConsumeBytesLenAt(e, "\t\t\t") }

func emitUnmarshalCall(e Emitter, access string, isSamePackage bool) {
	if isSamePackage {
		e.Writef("\t\t\tif err := %s.unmarshal(dAtA[iNdEx:postIndex], depth+1); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", access)
	} else {
		e.Writef("\t\t\tif err := %s.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", access)
	}
}

// WireTypeInt returns the protobuf wire type number for a kind.
func WireTypeInt(kind protoreflect.Kind) int {
	switch kind {
	case protoreflect.BoolKind, protoreflect.EnumKind,
		protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Uint32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Uint64Kind:
		return 0 // varint
	case protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind, protoreflect.DoubleKind:
		return 1 // fixed64
	case protoreflect.StringKind, protoreflect.BytesKind, protoreflect.MessageKind:
		return 2 // length-delimited
	case protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind, protoreflect.FloatKind:
		return 5 // fixed32
	default:
		return 2
	}
}

// panicNotPackable is used by non-packable types for methods they should never receive.
func panicNotPackable(method string) {
	panic(fmt.Sprintf("%s called on non-packable type", method))
}
