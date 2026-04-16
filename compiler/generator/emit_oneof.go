package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitOneof(md protoreflect.MessageDescriptor, oo protoreflect.OneofDescriptor) {
	ifaceName := oneofInterfaceName(md, oo)
	markerMethod := "is" + ifaceName

	// Interface
	fmt.Fprintf(fg.body, "type %s interface {\n", ifaceName)
	fmt.Fprintf(fg.body, "\t%s()\n", markerMethod)
	fmt.Fprintf(fg.body, "}\n\n")

	// Concrete types
	for i := 0; i < oo.Fields().Len(); i++ {
		fd := oo.Fields().Get(i)
		variantName := oneofVariantName(md, fd)
		fieldName := snakeToPascal(string(fd.Name()))
		fieldType := fg.imports.goSingularType(fd)

		// In gogo compat mode, message-type oneof fields use pointers.
		if fg.gen != nil && fg.gen.GogoCompat && fd.Kind() == protoreflect.MessageKind {
			fieldType = "*" + fieldType
		}

		fmt.Fprintf(fg.body, "type %s struct {\n", variantName)
		fmt.Fprintf(fg.body, "\t%s %s\n", fieldName, fieldType)
		fmt.Fprintf(fg.body, "}\n\n")
		fmt.Fprintf(fg.body, "func (*%s) %s() {}\n\n", variantName, markerMethod)
	}
}
