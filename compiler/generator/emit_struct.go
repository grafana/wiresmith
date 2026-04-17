package generator

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

func (fg *FileGenerator) emitStruct(md protoreflect.MessageDescriptor) {
	name := goMessageTypeName(md)
	fmt.Fprintf(fg.body, "type %s struct {\n", name)

	seenOneofs := map[string]bool{}
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)

		if fd.IsMap() {
			goName := snakeToPascal(string(fd.Name()))
			goType := fg.imports.goType(fd)
			fmt.Fprintf(fg.body, "\t%s %s\n", goName, goType)
			continue
		}

		if oo := fd.ContainingOneof(); oo != nil && !oo.IsSynthetic() {
			ooName := string(oo.Name())
			if !seenOneofs[ooName] {
				seenOneofs[ooName] = true
				goName := snakeToPascal(ooName)
				ifaceName := oneofInterfaceName(md, oo)
				fmt.Fprintf(fg.body, "\t%s %s\n", goName, ifaceName)
			}
			continue
		}

		goName := snakeToPascal(string(fd.Name()))
		goType := fg.imports.goType(fd)
		fmt.Fprintf(fg.body, "\t%s %s\n", goName, goType)
	}

	fmt.Fprintf(fg.body, "}\n\n")
}
