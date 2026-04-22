package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// presenceField stores the bit index for a field that needs presence tracking.
type presenceField struct {
	fd       protoreflect.FieldDescriptor
	bitIndex int
}

// fieldsForPresence returns message fields that need presence-bitmap tracking.
// These are singular fields without their own presence semantics:
// not repeated, not map, not optional (pointer), not oneof (interface).
func fieldsForPresence(md protoreflect.MessageDescriptor) []presenceField {
	var fields []presenceField
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if fd.IsList() || fd.IsMap() || fd.HasOptionalKeyword() || isRealOneof(fd) {
			continue
		}
		fields = append(fields, presenceField{fd: fd, bitIndex: len(fields)})
	}
	return fields
}

// presenceBitmapWords returns the number of uint64 words needed to store
// the presence bitmap for a message. Returns 0 if no fields need tracking.
func presenceBitmapWords(md protoreflect.MessageDescriptor) int {
	pf := fieldsForPresence(md)
	if len(pf) == 0 {
		return 0
	}
	return (len(pf) + 63) / 64
}

// presenceMap builds a lookup from proto field number to bit index
// for fields that need presence tracking.
func presenceMap(md protoreflect.MessageDescriptor) map[protoreflect.FieldNumber]int {
	fields := fieldsForPresence(md)
	if len(fields) == 0 {
		return nil
	}
	m := make(map[protoreflect.FieldNumber]int, len(fields))
	for _, pf := range fields {
		m[pf.fd.Number()] = pf.bitIndex
	}
	return m
}

// presenceCheck returns the Go expression "m.XXX_fieldsPresent[W]&(1<<B) != 0"
// for the given bit index.
func presenceCheck(bitIndex int) string {
	return fmt.Sprintf("m.XXX_fieldsPresent[%d]&(1<<%d) != 0", bitIndex/64, bitIndex%64)
}

// presenceSet returns the Go statement "m.XXX_fieldsPresent[W] |= 1 << B"
// for the given bit index.
func presenceSet(bitIndex int) string {
	return fmt.Sprintf("m.XXX_fieldsPresent[%d] |= 1 << %d", bitIndex/64, bitIndex%64)
}

func (fg *FileGenerator) emitHasMethods(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	for _, pf := range fieldsForPresence(md) {
		goName := snakeToPascal(string(pf.fd.Name()))
		fmt.Fprintf(fg.body, "func (m *%s) Has%s() bool {\n", name, goName)
		fmt.Fprintf(fg.body, "\tif m == nil {\n\t\treturn false\n\t}\n")
		fmt.Fprintf(fg.body, "\treturn %s\n", presenceCheck(pf.bitIndex))
		fmt.Fprintf(fg.body, "}\n\n")
	}
}
