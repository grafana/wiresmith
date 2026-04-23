package protohelpers

import "google.golang.org/protobuf/reflect/protoreflect"

// FindMessageDescriptor searches a file descriptor tree for a message by full name.
func FindMessageDescriptor(fd protoreflect.FileDescriptor, name protoreflect.FullName) protoreflect.MessageDescriptor {
	return findMessageInDescriptors(fd.Messages(), name)
}

func findMessageInDescriptors(msgs protoreflect.MessageDescriptors, name protoreflect.FullName) protoreflect.MessageDescriptor {
	for i := 0; i < msgs.Len(); i++ {
		md := msgs.Get(i)
		if md.FullName() == name {
			return md
		}
		if found := findMessageInDescriptors(md.Messages(), name); found != nil {
			return found
		}
	}
	return nil
}

// FindEnumDescriptor searches a file descriptor tree for an enum by full name.
func FindEnumDescriptor(fd protoreflect.FileDescriptor, name protoreflect.FullName) protoreflect.EnumDescriptor {
	for i := 0; i < fd.Enums().Len(); i++ {
		if fd.Enums().Get(i).FullName() == name {
			return fd.Enums().Get(i)
		}
	}
	return findEnumInMessages(fd.Messages(), name)
}

func findEnumInMessages(msgs protoreflect.MessageDescriptors, name protoreflect.FullName) protoreflect.EnumDescriptor {
	for i := 0; i < msgs.Len(); i++ {
		md := msgs.Get(i)
		for j := 0; j < md.Enums().Len(); j++ {
			if md.Enums().Get(j).FullName() == name {
				return md.Enums().Get(j)
			}
		}
		if found := findEnumInMessages(md.Messages(), name); found != nil {
			return found
		}
	}
	return nil
}
