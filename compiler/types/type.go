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

// unexported helpers for emit methods
func emitConsumeVarint(e Emitter) {
	e.Writef("\t\t\tv, n := protowire.ConsumeVarint(b)\n")
	e.Writef("\t\t\tif n < 0 {\n\t\t\t\treturn fmt.Errorf(\"invalid varint\")\n\t\t\t}\n")
}

func emitConsumeFixed32(e Emitter) {
	e.Writef("\t\t\tv, n := protowire.ConsumeFixed32(b)\n")
	e.Writef("\t\t\tif n < 0 {\n\t\t\t\treturn fmt.Errorf(\"invalid fixed32\")\n\t\t\t}\n")
}

func emitConsumeFixed64(e Emitter) {
	e.Writef("\t\t\tv, n := protowire.ConsumeFixed64(b)\n")
	e.Writef("\t\t\tif n < 0 {\n\t\t\t\treturn fmt.Errorf(\"invalid fixed64\")\n\t\t\t}\n")
}

func emitConsumeBytes(e Emitter) {
	e.Writef("\t\t\tv, n := protowire.ConsumeBytes(b)\n")
	e.Writef("\t\t\tif n < 0 {\n\t\t\t\treturn fmt.Errorf(\"invalid bytes\")\n\t\t\t}\n")
}

func emitConsumeString(e Emitter) {
	e.Writef("\t\t\tv, n := protowire.ConsumeString(b)\n")
	e.Writef("\t\t\tif n < 0 {\n\t\t\t\treturn fmt.Errorf(\"invalid string\")\n\t\t\t}\n")
}

func emitAdvanceBytes(e Emitter) {
	e.Writef("\t\t\tb = b[n:]\n")
}

func emitUnmarshalCall(e Emitter, access string, isSamePackage bool) {
	if isSamePackage {
		e.Writef("\t\t\tif err := %s.unmarshal(v, depth+1); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", access)
	} else {
		e.Writef("\t\t\tif err := %s.Unmarshal(v); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n", access)
	}
}

// panicNotPackable is used by non-packable types for methods they should never receive.
func panicNotPackable(method string) {
	panic(fmt.Sprintf("%s called on non-packable type", method))
}
