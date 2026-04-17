package generator

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitSize(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	fg.imports.addImport("google.golang.org/protobuf/encoding/protowire", "")

	fmt.Fprintf(fg.body, "func (m *%s) Size() int {\n", name)
	fmt.Fprintf(fg.body, "\tvar n int\n")

	seenOneofs := map[string]bool{}
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if isRealOneof(fd) {
			oo := fd.ContainingOneof()
			ooName := string(oo.Name())
			if !seenOneofs[ooName] {
				seenOneofs[ooName] = true
				fg.emitOneofSize(md, oo)
			}
			continue
		}
		fg.emitFieldSize(fd)
	}

	fmt.Fprintf(fg.body, "\treturn n\n")
	fmt.Fprintf(fg.body, "}\n\n")
}

func (fg *FileGenerator) emitFieldSize(fd protoreflect.FieldDescriptor) {
	goName := snakeToPascal(string(fd.Name()))
	access := "m." + goName
	tagSize := protowire.SizeTag(protowire.Number(fd.Number()))

	if fd.IsMap() {
		fg.emitMapFieldSize(access, fd, tagSize)
		return
	}
	if fd.IsList() {
		fg.emitRepeatedFieldSize(access, fd, tagSize)
		return
	}
	if fd.HasOptionalKeyword() {
		fg.emitOptionalFieldSize(access, fd, tagSize)
		return
	}
	fg.emitSingularFieldSize(access, fd, tagSize)
}

func (fg *FileGenerator) emitSingularFieldSize(access string, fd protoreflect.FieldDescriptor, tagSize int) {
	kind := fd.Kind()
	switch kind {
	case protoreflect.BoolKind:
		fmt.Fprintf(fg.body, "\tif %s {\n\t\tn += %d\n\t}\n", access, tagSize+1)
	case protoreflect.Int32Kind, protoreflect.Uint32Kind, protoreflect.Int64Kind, protoreflect.Uint64Kind, protoreflect.EnumKind:
		fmt.Fprintf(fg.body, "\tif %s != 0 {\n\t\tn += %d + protowire.SizeVarint(uint64(%s))\n\t}\n", access, tagSize, access)
	case protoreflect.Sint32Kind:
		fmt.Fprintf(fg.body, "\tif %s != 0 {\n\t\tn += %d + protowire.SizeVarint(protowire.EncodeZigZag(int64(%s)))\n\t}\n", access, tagSize, access)
	case protoreflect.Sint64Kind:
		fmt.Fprintf(fg.body, "\tif %s != 0 {\n\t\tn += %d + protowire.SizeVarint(protowire.EncodeZigZag(%s))\n\t}\n", access, tagSize, access)
	case protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind, protoreflect.FloatKind:
		fmt.Fprintf(fg.body, "\tif %s != 0 {\n\t\tn += %d\n\t}\n", access, tagSize+4)
	case protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind, protoreflect.DoubleKind:
		fmt.Fprintf(fg.body, "\tif %s != 0 {\n\t\tn += %d\n\t}\n", access, tagSize+8)
	case protoreflect.StringKind:
		fmt.Fprintf(fg.body, "\tif len(%s) > 0 {\n\t\tn += %d + protowire.SizeVarint(uint64(len(%s))) + len(%s)\n\t}\n", access, tagSize, access, access)
	case protoreflect.BytesKind:
		fmt.Fprintf(fg.body, "\tif len(%s) > 0 {\n\t\tn += %d + protowire.SizeVarint(uint64(len(%s))) + len(%s)\n\t}\n", access, tagSize, access, access)
	case protoreflect.MessageKind:
		fmt.Fprintf(fg.body, "\t{\n\t\ts := %s.Size()\n\t\tif s > 0 {\n\t\t\tn += %d + protowire.SizeVarint(uint64(s)) + s\n\t\t}\n\t}\n", access, tagSize)
	}
}

func (fg *FileGenerator) emitOptionalFieldSize(access string, fd protoreflect.FieldDescriptor, tagSize int) {
	kind := fd.Kind()
	fmt.Fprintf(fg.body, "\tif %s != nil {\n", access)
	switch kind {
	case protoreflect.BoolKind:
		fmt.Fprintf(fg.body, "\t\tn += %d\n", tagSize+1)
	case protoreflect.Int32Kind, protoreflect.Uint32Kind, protoreflect.Int64Kind, protoreflect.Uint64Kind, protoreflect.EnumKind:
		fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(uint64(*%s))\n", tagSize, access)
	case protoreflect.Sint32Kind:
		fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(protowire.EncodeZigZag(int64(*%s)))\n", tagSize, access)
	case protoreflect.Sint64Kind:
		fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(protowire.EncodeZigZag(*%s))\n", tagSize, access)
	case protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind, protoreflect.FloatKind:
		fmt.Fprintf(fg.body, "\t\tn += %d\n", tagSize+4)
	case protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind, protoreflect.DoubleKind:
		fmt.Fprintf(fg.body, "\t\tn += %d\n", tagSize+8)
	case protoreflect.StringKind:
		fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(uint64(len(*%s))) + len(*%s)\n", tagSize, access, access)
	case protoreflect.BytesKind:
		// optional bytes is []byte (not *[]byte); nil guard is the same "!= nil" check.
		fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(uint64(len(%s))) + len(%s)\n", tagSize, access, access)
	}
	fmt.Fprintf(fg.body, "\t}\n")
}

func (fg *FileGenerator) emitRepeatedFieldSize(access string, fd protoreflect.FieldDescriptor, tagSize int) {
	kind := fd.Kind()

	switch {
	case kind == protoreflect.MessageKind:
		fmt.Fprintf(fg.body, "\tfor i := range %s {\n", access)
		fmt.Fprintf(fg.body, "\t\ts := %s[i].Size()\n", access)
		fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(uint64(s)) + s\n", tagSize)
		fmt.Fprintf(fg.body, "\t}\n")
	case kind == protoreflect.StringKind:
		fmt.Fprintf(fg.body, "\tfor _, v := range %s {\n", access)
		fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(uint64(len(v))) + len(v)\n", tagSize)
		fmt.Fprintf(fg.body, "\t}\n")
	case kind == protoreflect.BytesKind:
		fmt.Fprintf(fg.body, "\tfor _, v := range %s {\n", access)
		fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(uint64(len(v))) + len(v)\n", tagSize)
		fmt.Fprintf(fg.body, "\t}\n")
	case isPackable(kind) && fd.IsPacked():
		fmt.Fprintf(fg.body, "\tif len(%s) > 0 {\n", access)
		if isFixed64Kind(kind) {
			fmt.Fprintf(fg.body, "\t\tdataLen := len(%s) * 8\n", access)
		} else if isFixed32Kind(kind) {
			fmt.Fprintf(fg.body, "\t\tdataLen := len(%s) * 4\n", access)
		} else if kind == protoreflect.BoolKind {
			fmt.Fprintf(fg.body, "\t\tdataLen := len(%s)\n", access)
		} else {
			fmt.Fprintf(fg.body, "\t\tvar dataLen int\n")
			fmt.Fprintf(fg.body, "\t\tfor _, v := range %s {\n", access)
			switch kind {
			case protoreflect.Sint32Kind:
				fmt.Fprintf(fg.body, "\t\t\tdataLen += protowire.SizeVarint(protowire.EncodeZigZag(int64(v)))\n")
			case protoreflect.Sint64Kind:
				fmt.Fprintf(fg.body, "\t\t\tdataLen += protowire.SizeVarint(protowire.EncodeZigZag(v))\n")
			default:
				fmt.Fprintf(fg.body, "\t\t\tdataLen += protowire.SizeVarint(uint64(v))\n")
			}
			fmt.Fprintf(fg.body, "\t\t}\n")
		}
		fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(uint64(dataLen)) + dataLen\n", tagSize)
		fmt.Fprintf(fg.body, "\t}\n")

	case isPackable(kind) && !fd.IsPacked():
		fg.emitUnpackedRepeatedFieldSize(access, fd, tagSize)
	}
}

func (fg *FileGenerator) emitMapFieldSize(access string, fd protoreflect.FieldDescriptor, tagSize int) {
	keyFd := fd.MapKey()
	valFd := fd.MapValue()
	keyTagSize := protowire.SizeTag(1)
	valTagSize := protowire.SizeTag(2)

	keyUsed := mapElementSizeUsesVar(keyFd.Kind())
	valUsed := mapElementSizeUsesVar(valFd.Kind())
	switch {
	case keyUsed && valUsed:
		fmt.Fprintf(fg.body, "\tfor k, v := range %s {\n", access)
	case keyUsed:
		fmt.Fprintf(fg.body, "\tfor k := range %s {\n", access)
	case valUsed:
		fmt.Fprintf(fg.body, "\tfor _, v := range %s {\n", access)
	default:
		fmt.Fprintf(fg.body, "\tfor range %s {\n", access)
	}
	fmt.Fprintf(fg.body, "\t\tentrySize := 0\n")

	fg.emitMapElementSize("k", keyFd, keyTagSize, "\t\t")
	fg.emitMapElementSize("v", valFd, valTagSize, "\t\t")

	fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(uint64(entrySize)) + entrySize\n", tagSize)
	fmt.Fprintf(fg.body, "\t}\n")
}

// mapElementSizeUsesVar returns true if computing the size of this kind
// requires referencing the variable (i.e. it's not a constant-size type).
func mapElementSizeUsesVar(kind protoreflect.Kind) bool {
	switch kind {
	case protoreflect.BoolKind,
		protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind, protoreflect.FloatKind,
		protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind, protoreflect.DoubleKind:
		return false
	default:
		return true
	}
}

func (fg *FileGenerator) emitMapElementSize(varName string, fd protoreflect.FieldDescriptor, tagSize int, indent string) {
	kind := fd.Kind()
	switch kind {
	case protoreflect.BoolKind:
		fmt.Fprintf(fg.body, "%sentrySize += %d\n", indent, tagSize+1)
	case protoreflect.Int32Kind, protoreflect.Uint32Kind, protoreflect.Int64Kind, protoreflect.Uint64Kind, protoreflect.EnumKind:
		fmt.Fprintf(fg.body, "%sentrySize += %d + protowire.SizeVarint(uint64(%s))\n", indent, tagSize, varName)
	case protoreflect.Sint32Kind:
		fmt.Fprintf(fg.body, "%sentrySize += %d + protowire.SizeVarint(protowire.EncodeZigZag(int64(%s)))\n", indent, tagSize, varName)
	case protoreflect.Sint64Kind:
		fmt.Fprintf(fg.body, "%sentrySize += %d + protowire.SizeVarint(protowire.EncodeZigZag(%s))\n", indent, tagSize, varName)
	case protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind, protoreflect.FloatKind:
		fmt.Fprintf(fg.body, "%sentrySize += %d\n", indent, tagSize+4)
	case protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind, protoreflect.DoubleKind:
		fmt.Fprintf(fg.body, "%sentrySize += %d\n", indent, tagSize+8)
	case protoreflect.StringKind:
		fmt.Fprintf(fg.body, "%sentrySize += %d + protowire.SizeVarint(uint64(len(%s))) + len(%s)\n", indent, tagSize, varName, varName)
	case protoreflect.BytesKind:
		fmt.Fprintf(fg.body, "%sentrySize += %d + protowire.SizeVarint(uint64(len(%s))) + len(%s)\n", indent, tagSize, varName, varName)
	case protoreflect.MessageKind:
		fmt.Fprintf(fg.body, "%ss := %s.Size()\n", indent, varName)
		fmt.Fprintf(fg.body, "%sentrySize += %d + protowire.SizeVarint(uint64(s)) + s\n", indent, tagSize)
	}
}

func (fg *FileGenerator) emitUnpackedRepeatedFieldSize(access string, fd protoreflect.FieldDescriptor, tagSize int) {
	kind := fd.Kind()

	switch {
	case isFixed64Kind(kind):
		fmt.Fprintf(fg.body, "\tn += len(%s) * %d\n", access, tagSize+8)
	case isFixed32Kind(kind):
		fmt.Fprintf(fg.body, "\tn += len(%s) * %d\n", access, tagSize+4)
	case kind == protoreflect.BoolKind:
		fmt.Fprintf(fg.body, "\tn += len(%s) * %d\n", access, tagSize+1)
	default:
		fmt.Fprintf(fg.body, "\tfor _, v := range %s {\n", access)
		switch kind {
		case protoreflect.Sint32Kind:
			fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(protowire.EncodeZigZag(int64(v)))\n", tagSize)
		case protoreflect.Sint64Kind:
			fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(protowire.EncodeZigZag(v))\n", tagSize)
		default:
			fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(uint64(v))\n", tagSize)
		}
		fmt.Fprintf(fg.body, "\t}\n")
	}
}

func (fg *FileGenerator) emitOneofSize(md protoreflect.MessageDescriptor, oo protoreflect.OneofDescriptor) {
	ooFieldName := snakeToPascal(string(oo.Name()))
	fmt.Fprintf(fg.body, "\tswitch v := m.%s.(type) {\n", ooFieldName)

	for i := 0; i < oo.Fields().Len(); i++ {
		fd := oo.Fields().Get(i)
		variantName := oneofVariantName(md, fd)
		fieldName := snakeToPascal(string(fd.Name()))
		tagSize := protowire.SizeTag(protowire.Number(fd.Number()))
		access := "v." + fieldName

		fmt.Fprintf(fg.body, "\tcase *%s:\n", variantName)

		switch fd.Kind() {
		case protoreflect.BoolKind:
			fmt.Fprintf(fg.body, "\t\t_ = %s\n\t\tn += %d\n", access, tagSize+1)
		case protoreflect.Int32Kind, protoreflect.Uint32Kind, protoreflect.Int64Kind, protoreflect.Uint64Kind, protoreflect.EnumKind:
			fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(uint64(%s))\n", tagSize, access)
		case protoreflect.Sint32Kind:
			fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(protowire.EncodeZigZag(int64(%s)))\n", tagSize, access)
		case protoreflect.Sint64Kind:
			fmt.Fprintf(fg.body, "\t\tn += %d + protowire.SizeVarint(protowire.EncodeZigZag(%s))\n", tagSize, access)
		case protoreflect.Fixed32Kind, protoreflect.Sfixed32Kind, protoreflect.FloatKind:
			fmt.Fprintf(fg.body, "\t\t_ = %s\n\t\tn += %d\n", access, tagSize+4)
		case protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind, protoreflect.DoubleKind:
			fmt.Fprintf(fg.body, "\t\t_ = %s\n\t\tn += %d\n", access, tagSize+8)
		case protoreflect.StringKind:
			fmt.Fprintf(fg.body, "\t\tl := len(%s)\n\t\tn += %d + protowire.SizeVarint(uint64(l)) + l\n", access, tagSize)
		case protoreflect.BytesKind:
			fmt.Fprintf(fg.body, "\t\tl := len(%s)\n\t\tn += %d + protowire.SizeVarint(uint64(l)) + l\n", access, tagSize)
		case protoreflect.MessageKind:
			fmt.Fprintf(fg.body, "\t\ts := %s.Size()\n\t\tn += %d + protowire.SizeVarint(uint64(s)) + s\n", access, tagSize)
		}
	}

	fmt.Fprintf(fg.body, "\t}\n")
}
