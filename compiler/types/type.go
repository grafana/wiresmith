package types

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// FieldType is the unified interface for all field code generation.
// Callers use this to emit size, marshal, unmarshal, and equality code.
type FieldType interface {
	EmitSize(e Emitter, access string, tagSize int)
	EmitMarshal(e Emitter, access string, num protowire.Number)
	EmitUnmarshal(e Emitter, access string, ctx FieldContext)
	// EmitEqual emits an inequality guard that returns false from the
	// enclosing Equal method when lhs and rhs differ. For non-comparable
	// types (bytes, message) this emits the appropriate deep-equality
	// form; for scalars it emits `lhs != rhs`. `indent` is the prefix
	// applied to every emitted line, matching the EmitValueSize style.
	EmitEqual(e Emitter, indent, lhs, rhs string)
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

// ScalarType is implemented by every leaf Type whose getter returns its
// value by value (bool, all integers, floats, string, bytes, enum). The
// only Type that is NOT a ScalarType is MessageType, whose getter
// returns *Msg with nil for absent — a "zero literal" makes no sense
// there. Callers that need ZeroLiteral must filter out MessageKind first
// (or use ScalarZeroLiteral, which handles the type-assertion).
type ScalarType interface {
	Type
	// ZeroLiteral returns the Go zero-value literal for this type, used
	// by nil-safe getters ("false", `""`, "nil", "0").
	ZeroLiteral() string
}

// ScalarZeroLiteral returns the Go zero-value literal for a non-message
// kind. Panics if the kind's Type isn't a ScalarType — i.e. callers must
// rule out MessageKind before calling. The panic doubles as a defensive
// guard against a future Type that forgets to implement ScalarType.
func ScalarZeroLiteral(kind protoreflect.Kind) string {
	t := Get(kind)
	s, ok := t.(ScalarType)
	if !ok {
		panic(fmt.Sprintf("ScalarZeroLiteral called on non-scalar kind: %v", kind))
	}
	return s.ZeroLiteral()
}

// scalarNotEqualGuard emits `if lhs != rhs { return false }` at the
// given indent. Shared by every Type whose Go form is `==`-comparable.
func scalarNotEqualGuard(e Emitter, indent, lhs, rhs string) {
	e.Writef("%sif %s != %s {\n%s\treturn false\n%s}\n", indent, lhs, rhs, indent, indent)
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

// EmitConsumeTagAt emits an inline tag decode at the given indent.
// Declares varName (uint64) in generated code, advances iNdEx.
// Tags are 32-bit field numbers so the varint is rejected after 5 bytes.
// Also validates the field number range (1..0x1FFFFFFF) before the caller
// switches on the decoded field number.
func EmitConsumeTagAt(e Emitter, indent, varName string) {
	e.AddImport("io", "")
	e.AddImport("fmt", "")
	e.Writef("%svar %s uint64\n", indent, varName)
	e.Writef("%sfor shift := uint(0); ; shift += 7 {\n", indent)
	e.Writef("%s\tif shift >= 35 {\n%s\t\treturn fmt.Errorf(\"proto: integer overflow\")\n%s\t}\n", indent, indent, indent)
	e.Writef("%s\tif iNdEx >= l {\n%s\t\treturn io.ErrUnexpectedEOF\n%s\t}\n", indent, indent, indent)
	e.Writef("%s\tb := dAtA[iNdEx]\n", indent)
	e.Writef("%s\tiNdEx++\n", indent)
	e.Writef("%s\t%s |= uint64(b&0x7F) << shift\n", indent, varName)
	e.Writef("%s\tif b < 0x80 {\n%s\t\tbreak\n%s\t}\n", indent, indent, indent)
	e.Writef("%s}\n", indent)
	e.Writef("%sif %s>>3 < 1 || %s>>3 > 0x1FFFFFFF {\n%s\treturn fmt.Errorf(\"invalid field number\")\n%s}\n", indent, varName, varName, indent, indent)
}

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
	// Wire format: on the 10th byte (shift==63), only bit 0 of the payload may
	// be set. A high continuation bit means an 11th byte was indicated, and any
	// other data bit overflows past uint64. Without this check the shift below
	// silently drops bits 1-6, producing a wrong but accepted value.
	e.Writef("%s\tif shift == 63 && b > 1 {\n%s\t\treturn fmt.Errorf(\"proto: varint overflow\")\n%s\t}\n", indent, indent, indent)
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
	e.AddImport("math", "")
	e.Writef("%svar byteLen uint64\n", indent)
	e.Writef("%sfor shift := uint(0); ; shift += 7 {\n", indent)
	e.Writef("%s\tif shift >= 64 {\n%s\t\treturn fmt.Errorf(\"proto: integer overflow\")\n%s\t}\n", indent, indent, indent)
	e.Writef("%s\tif iNdEx >= l {\n%s\t\treturn io.ErrUnexpectedEOF\n%s\t}\n", indent, indent, indent)
	e.Writef("%s\tb := dAtA[iNdEx]\n", indent)
	e.Writef("%s\tiNdEx++\n", indent)
	// See emitConsumeVarintAt for the 10th-byte overflow rationale.
	e.Writef("%s\tif shift == 63 && b > 1 {\n%s\t\treturn fmt.Errorf(\"proto: varint overflow\")\n%s\t}\n", indent, indent, indent)
	e.Writef("%s\tbyteLen |= uint64(b&0x7F) << shift\n", indent)
	e.Writef("%s\tif b < 0x80 {\n%s\t\tbreak\n%s\t}\n", indent, indent, indent)
	e.Writef("%s}\n", indent)
	// Guard against int truncation on 32-bit platforms (GOARCH=386/arm/wasm).
	// Without this, a uint64 length above MaxInt32 would silently wrap to a
	// small positive int and bypass the postIndex>l bound check. The guard
	// subsumes the historical `int(byteLen) < 0` check: byteLen <= MaxInt
	// implies int(byteLen) >= 0 on every supported GOARCH.
	e.Writef("%sif byteLen > uint64(math.MaxInt) {\n%s\treturn io.ErrUnexpectedEOF\n%s}\n", indent, indent, indent)
	e.Writef("%sintByteLen := int(byteLen)\n", indent)
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
