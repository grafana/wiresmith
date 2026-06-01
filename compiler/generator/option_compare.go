package generator

import (
	"fmt"

	"github.com/bufbuild/protocompile/linker"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Compare opt-in is split between two extension names so the option can
// be applied at either granularity. compare is a MessageOptions extension
// (per-message); compare_all is a FileOptions extension that opts every
// message in the file in. Both names live here as the single source of
// truth — keep them in sync with embed/wiresmith/options.proto.
const (
	compareExtensionName    = "wiresmith.options.compare"
	compareAllExtensionName = "wiresmith.options.compare_all"
)

// resolveCompareExtensions caches the two extension descriptors on the
// Generator. Mirrors resolvePointerExtension; one shared embedded options
// file holds all three extensions.
func (g *Generator) resolveCompareExtensions(results linker.Files) error {
	for _, fd := range results {
		if fd.Path() != embeddedOptionsPath {
			continue
		}
		exts := fd.Extensions()
		for i := 0; i < exts.Len(); i++ {
			x := exts.Get(i)
			switch string(x.FullName()) {
			case compareExtensionName:
				g.compareExt = x
			case compareAllExtensionName:
				g.compareAllExt = x
			}
		}
	}
	if g.compareExt == nil || g.compareAllExt == nil {
		return fmt.Errorf("internal error: compare extensions not found in compiled results — wiresmith/options.proto missing or malformed")
	}
	return nil
}

// computeCompareSet builds the set of message-descriptor full names that
// should receive a generated Compare method. Membership is the closure
// over two seeds — every message in a file with compare_all=true, and
// every message with compare=true on its own options — extended along
// message-typed field references. The closure lets a user opt in only
// the "root" message of a sub-tree and have the inner messages picked up
// automatically, which matches gogo's de-facto behavior where applying
// compare to one message tends to require flipping it on the surrounding
// type set anyway.
//
// Stored on the Generator so per-file emission can look it up in O(1).
// Includes messages from imported files (e.g. OTel common types pulled
// in by a compare-enabled file) so the transitive recursion compiles —
// otherwise the generated `inner.Compare(other)` call would fail at
// `go build` with "no Compare method".
func (g *Generator) computeCompareSet(results linker.Files) {
	g.compareSet = make(map[string]bool)

	// Seed: direct opt-ins via file-level compare_all and per-message
	// compare.
	for _, fd := range results {
		if isInternalSchemaFile(fd) {
			continue
		}
		all := fileCompareAll(g.compareAllExt, fd)
		walkAllMessages(fd, func(md protoreflect.MessageDescriptor) {
			if all || messageCompare(g.compareExt, md) {
				g.compareSet[string(md.FullName())] = true
			}
		})
	}

	// Closure over message-field references. Iterate until the set
	// stops growing; each pass extends membership to message-type fields
	// of already-included messages.
	for changed := true; changed; {
		changed = false
		for _, fd := range results {
			if isInternalSchemaFile(fd) {
				continue
			}
			walkAllMessages(fd, func(md protoreflect.MessageDescriptor) {
				if !g.compareSet[string(md.FullName())] {
					return
				}
				fields := md.Fields()
				for i := 0; i < fields.Len(); i++ {
					f := fields.Get(i)
					target := referencedMessage(f)
					if target == nil {
						continue
					}
					name := string(target.FullName())
					if g.compareSet[name] {
						continue
					}
					g.compareSet[name] = true
					changed = true
				}
			})
		}
	}
}

// referencedMessage returns the message descriptor a field points to, or
// nil for non-message fields. Handles maps by walking through to the
// value descriptor — a map<K, Msg> is still a message reference for the
// purposes of Compare closure.
func referencedMessage(f protoreflect.FieldDescriptor) protoreflect.MessageDescriptor {
	if f.IsMap() {
		v := f.MapValue()
		if v.Kind() == protoreflect.MessageKind {
			return v.Message()
		}
		return nil
	}
	if f.Kind() == protoreflect.MessageKind {
		return f.Message()
	}
	return nil
}

// walkAllMessages calls fn on every message reachable from fd in
// post-order, skipping map-entry pseudo-messages. Matches forEachMessage
// (in generator.go) but is callable from this file without the
// FileGenerator scope.
func walkAllMessages(fd protoreflect.FileDescriptor, fn func(protoreflect.MessageDescriptor)) {
	var visit func(md protoreflect.MessageDescriptor)
	visit = func(md protoreflect.MessageDescriptor) {
		for i := 0; i < md.Messages().Len(); i++ {
			nested := md.Messages().Get(i)
			if nested.IsMapEntry() {
				continue
			}
			visit(nested)
		}
		fn(md)
	}
	for i := 0; i < fd.Messages().Len(); i++ {
		visit(fd.Messages().Get(i))
	}
}

// fileCompareAll reads the `(wiresmith.options.compare_all)` FileOption
// off a file descriptor. Safe on any input — returns false when the
// extension is unresolved, the options are nil, or the option is unset.
func fileCompareAll(ext protoreflect.FieldDescriptor, fd protoreflect.FileDescriptor) bool {
	if ext == nil {
		return false
	}
	opts, ok := fd.Options().(*descriptorpb.FileOptions)
	if !ok || opts == nil {
		return false
	}
	xt := extensionType(ext)
	if !proto.HasExtension(opts, xt) {
		return false
	}
	v, _ := proto.GetExtension(opts, xt).(bool)
	return v
}

// messageCompare reads the `(wiresmith.options.compare)` MessageOption
// off a message descriptor. Same safety contract as fileCompareAll.
func messageCompare(ext protoreflect.FieldDescriptor, md protoreflect.MessageDescriptor) bool {
	if ext == nil {
		return false
	}
	opts, ok := md.Options().(*descriptorpb.MessageOptions)
	if !ok || opts == nil {
		return false
	}
	xt := extensionType(ext)
	if !proto.HasExtension(opts, xt) {
		return false
	}
	v, _ := proto.GetExtension(opts, xt).(bool)
	return v
}

// extensionType wraps the resolved descriptor into a protoreflect
// ExtensionType. Prefers the linker's ExtensionTypeDescriptor when
// available (gives a typed extension) and falls back to a dynamic
// extension otherwise — matches the pattern used by hasPointerOption.
func extensionType(ext protoreflect.FieldDescriptor) protoreflect.ExtensionType {
	if xd, ok := ext.(protoreflect.ExtensionTypeDescriptor); ok {
		return xd.Type()
	}
	return dynamicpb.NewExtensionType(ext)
}
