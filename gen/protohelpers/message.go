// Package protohelpers provides encoding helpers and reflection support for wiresmith-generated types.
package protohelpers

import (
	"fmt"
	"reflect"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoiface"
	"google.golang.org/protobuf/runtime/protoimpl"
)

// Wiremessage is the interface implemented by every wiresmith-generated
// message type. It exposes the fast-path serialization methods that
// MessageReflect.ProtoMethods delegates to.
type Wiremessage interface {
	protoreflect.ProtoMessage
	Marshal() ([]byte, error)
	MarshalTo(buf []byte) (int, error)
	MarshalToSizedBuffer(buf []byte) (int, error)
	Unmarshal(b []byte) error
	Size() int
	Equal(that any) bool
}

// MessageReflect implements protoreflect.Message for wiresmith-generated types.
//
// Supported APIs (work as expected):
//   - proto.MessageName, proto.Marshal, proto.Unmarshal, proto.Size, proto.Equal
//   - Type/descriptor lookup via protoregistry.GlobalTypes / protoreflect.Descriptor
//
// Unsupported APIs (panic with a clear message):
//   - Field-level reflection: Range, Has, Get, Set, Mutable, Clear, NewField, WhichOneof
//   - Reflection-based helpers built on top of those: protojson, prototext, proto.Clone, proto.Merge
//
// wiresmith generates value-type message fields (`Resource Resource`, not
// `*Resource`) which are incompatible with the official protobuf reflection
// field converters (newMessageConverter expects pointer types). The fast-path
// serialization methods (proto.Marshal/Unmarshal/Size/Equal) work because
// they go through ProtoMethods rather than field-level reflection.
type MessageReflect struct {
	mi  *protoimpl.MessageInfo
	msg Wiremessage
}

// NewMessageReflect returns a protoreflect.Message wrapping a wiresmith-generated
// message. Called from generated ProtoReflect() methods.
func NewMessageReflect(mi *protoimpl.MessageInfo, msg Wiremessage) protoreflect.Message {
	return &MessageReflect{mi: mi, msg: msg}
}

func (m *MessageReflect) Descriptor() protoreflect.MessageDescriptor { return m.mi.Desc }
func (m *MessageReflect) Type() protoreflect.MessageType             { return m.mi }
func (m *MessageReflect) New() protoreflect.Message                  { return m.mi.New() }
func (m *MessageReflect) Interface() protoreflect.ProtoMessage       { return m.msg }
func (m *MessageReflect) ProtoMethods() *protoiface.Methods          { return wiresmithMethods }
func (m *MessageReflect) GetUnknown() protoreflect.RawFields         { return nil }
func (m *MessageReflect) SetUnknown(protoreflect.RawFields)          { panicReflect() }

// IsValid reports whether the message holds a non-nil pointer. It detects
// typed-nil interfaces (e.g. (*Resource)(nil) wrapped in Wiremessage) which
// would otherwise compare as non-nil through the interface.
func (m *MessageReflect) IsValid() bool {
	if m == nil || m.msg == nil {
		return false
	}
	v := reflect.ValueOf(m.msg)
	return v.Kind() != reflect.Pointer || !v.IsNil()
}

func (m *MessageReflect) Range(func(protoreflect.FieldDescriptor, protoreflect.Value) bool) {
	panicReflect()
}
func (m *MessageReflect) Has(protoreflect.FieldDescriptor) bool { panicReflect(); return false }
func (m *MessageReflect) Clear(protoreflect.FieldDescriptor)    { panicReflect() }
func (m *MessageReflect) Get(protoreflect.FieldDescriptor) protoreflect.Value {
	panicReflect()
	return protoreflect.Value{}
}
func (m *MessageReflect) Set(protoreflect.FieldDescriptor, protoreflect.Value) { panicReflect() }
func (m *MessageReflect) Mutable(protoreflect.FieldDescriptor) protoreflect.Value {
	panicReflect()
	return protoreflect.Value{}
}
func (m *MessageReflect) NewField(protoreflect.FieldDescriptor) protoreflect.Value {
	panicReflect()
	return protoreflect.Value{}
}
func (m *MessageReflect) WhichOneof(protoreflect.OneofDescriptor) protoreflect.FieldDescriptor {
	panicReflect()
	return nil
}

func panicReflect() {
	panic("wiresmith: field-level protobuf reflection is not supported (proto.Marshal/Unmarshal/Size/Equal work via ProtoMethods; protojson/prototext/proto.Clone/proto.Merge do not)")
}

// wiresmithMethods is the fast-path Methods table returned by ProtoMethods.
// Operations delegate to the wiresmith-generated methods on the underlying
// struct, avoiding field-level reflection entirely.
var wiresmithMethods = &protoiface.Methods{
	Flags: protoiface.SupportMarshalDeterministic,

	Size: func(in protoiface.SizeInput) protoiface.SizeOutput {
		mr, ok := in.Message.(*MessageReflect)
		if !ok {
			return protoiface.SizeOutput{}
		}
		return protoiface.SizeOutput{Size: mr.msg.Size()}
	},

	Marshal: func(in protoiface.MarshalInput) (protoiface.MarshalOutput, error) {
		mr, ok := in.Message.(*MessageReflect)
		if !ok {
			return protoiface.MarshalOutput{}, fmt.Errorf("wiresmith: Marshal called with non-wiresmith message %T", in.Message)
		}
		size := mr.msg.Size()
		oldLen := len(in.Buf)
		buf := append(in.Buf, make([]byte, size)...)
		// MarshalToSizedBuffer fills the entire passed slice with reverse-write encoding.
		n, err := mr.msg.MarshalToSizedBuffer(buf[oldLen:])
		if err != nil {
			return protoiface.MarshalOutput{}, err
		}
		return protoiface.MarshalOutput{Buf: buf[:oldLen+n]}, nil
	},

	Unmarshal: func(in protoiface.UnmarshalInput) (protoiface.UnmarshalOutput, error) {
		mr, ok := in.Message.(*MessageReflect)
		if !ok {
			return protoiface.UnmarshalOutput{}, fmt.Errorf("wiresmith: Unmarshal called with non-wiresmith message %T", in.Message)
		}
		if err := mr.msg.Unmarshal(in.Buf); err != nil {
			return protoiface.UnmarshalOutput{}, err
		}
		return protoiface.UnmarshalOutput{}, nil
	},

	Equal: func(in protoiface.EqualInput) protoiface.EqualOutput {
		a, okA := in.MessageA.(*MessageReflect)
		b, okB := in.MessageB.(*MessageReflect)
		if !okA || !okB {
			return protoiface.EqualOutput{Equal: false}
		}
		return protoiface.EqualOutput{Equal: a.msg.Equal(b.msg)}
	},

	CheckInitialized: func(protoiface.CheckInitializedInput) (protoiface.CheckInitializedOutput, error) {
		// wiresmith only supports proto3, which has no required fields.
		// Without this fast-path, proto.Marshal would call Range via the
		// slow-path checker and hit our field-reflection panic.
		return protoiface.CheckInitializedOutput{}, nil
	},
}
