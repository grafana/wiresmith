package protohelpers

import "reflect"

var messageTypes = map[string]reflect.Type{}
var enumNames = map[string]map[int32]string{}
var enumValues = map[string]map[string]int32{}

// RegisterType records a message type by its full proto name.
// Called from generated init() functions.
func RegisterType(msg any, fullName string) {
	t := reflect.TypeOf(msg)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	messageTypes[fullName] = t
}

// RegisterEnum records enum name/value maps by the enum's full proto name.
// Called from generated init() functions.
func RegisterEnum(fullName string, names map[int32]string, values map[string]int32) {
	enumNames[fullName] = names
	enumValues[fullName] = values
}

// MessageType returns the reflect.Type for a registered proto message name,
// or nil if not registered.
func MessageType(fullName string) reflect.Type {
	return messageTypes[fullName]
}

// EnumValueMap returns the string→int32 map for a registered enum name,
// or nil if not registered.
func EnumValueMap(fullName string) map[string]int32 {
	return enumValues[fullName]
}
