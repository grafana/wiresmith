// Package protohelpers provides encoding helpers and reflection support for wiresmith-generated types.
package protohelpers

import (
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoiface"
	"google.golang.org/protobuf/runtime/protoimpl"
)

// MessageReflect implements protoreflect.Message for wiresmith-generated types.
//
// It supports metadata operations (Descriptor, Type, Interface, IsValid, New)
// but not field-level reflection. wiresmith uses value-type message fields
// which are incompatible with the official protobuf reflection field converters.
// Use wiresmith's own Marshal/Unmarshal/Size methods for serialization.
type MessageReflect struct {
	mi  *protoimpl.MessageInfo
	msg protoreflect.ProtoMessage
}

// NewMessageReflect returns a protoreflect.Message for a wiresmith-generated type.
func NewMessageReflect(mi *protoimpl.MessageInfo, msg protoreflect.ProtoMessage) protoreflect.Message {
	return &MessageReflect{mi: mi, msg: msg}
}

func (m *MessageReflect) Descriptor() protoreflect.MessageDescriptor { return m.mi.Desc }
func (m *MessageReflect) Type() protoreflect.MessageType             { return m.mi }
func (m *MessageReflect) New() protoreflect.Message                  { return m.mi.Zero() }
func (m *MessageReflect) Interface() protoreflect.ProtoMessage       { return m.msg }
func (m *MessageReflect) IsValid() bool                              { return m.msg != nil }
func (m *MessageReflect) ProtoMethods() *protoiface.Methods          { return nil }
func (m *MessageReflect) GetUnknown() protoreflect.RawFields         { return nil }
func (m *MessageReflect) SetUnknown(protoreflect.RawFields)          { panicReflect() }

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
	panic("wiresmith: field-level protobuf reflection is not supported; use wiresmith's own Marshal/Unmarshal methods")
}
