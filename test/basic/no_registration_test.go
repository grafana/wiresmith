package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	nr "github.com/grafana/wiresmith/gen/basic/noregistration/v1"
)

// TestNoRegistrationAbsentFromGlobalRegistry pins the core guarantee of
// (wiresmith.options.no_registration): importing the package (which runs its
// init) mutates none of the official global registries, so the file, its
// messages, and its enum are all absent from protoregistry.GlobalFiles /
// GlobalTypes. This is what lets the same proto package be owned globally by
// another module (e.g. prometheus/client_model) without an init-time panic.
func TestNoRegistrationAbsentFromGlobalRegistry(t *testing.T) {
	_, err := protoregistry.GlobalFiles.FindFileByPath("basic/noregistration/v1/no_registration.proto")
	require.Error(t, err)
	assert.ErrorIs(t, err, protoregistry.NotFound)

	_, err = protoregistry.GlobalTypes.FindMessageByName("basic.noregistration.v1.Widget")
	assert.ErrorIs(t, err, protoregistry.NotFound)
	_, err = protoregistry.GlobalTypes.FindMessageByName("basic.noregistration.v1.Part")
	assert.ErrorIs(t, err, protoregistry.NotFound)
	_, err = protoregistry.GlobalTypes.FindEnumByName("basic.noregistration.v1.Color")
	assert.ErrorIs(t, err, protoregistry.NotFound)
}

// TestNoRegistrationWireRoundTrip pins that the generated types stay valid,
// fully-functional messages: the wiresmith wire methods (Marshal / Unmarshal /
// Equal / Compare / String) all keep working even though the type is invisible
// to the global registry.
func TestNoRegistrationWireRoundTrip(t *testing.T) {
	w := &nr.Widget{
		Name:  "gizmo",
		Count: 42,
		Color: nr.Color_COLOR_GREEN,
		Part:  nr.Part{Sku: "sku-1"},
	}

	b, err := w.Marshal()
	require.NoError(t, err)

	got := &nr.Widget{}
	require.NoError(t, got.Unmarshal(b))
	assert.True(t, got.Equal(w))
	assert.Equal(t, 0, got.Compare(w))
	assert.NotEmpty(t, got.String())
}

// TestNoRegistrationProtoReflect pins that reflection goes through the file's
// package-LOCAL descriptor: ProtoReflect resolves the message's full name, and
// proto.Marshal / proto.Unmarshal / proto.Equal all round-trip via that local
// descriptor without any global-registry involvement.
func TestNoRegistrationProtoReflect(t *testing.T) {
	w := &nr.Widget{
		Name:  "widget",
		Count: 7,
		Color: nr.Color_COLOR_RED,
		Part:  nr.Part{Sku: "abc"},
	}

	assert.Equal(t,
		protoreflect.FullName("basic.noregistration.v1.Widget"),
		w.ProtoReflect().Descriptor().FullName())

	b, err := proto.Marshal(w)
	require.NoError(t, err)
	got := &nr.Widget{}
	require.NoError(t, proto.Unmarshal(b, got))
	assert.True(t, proto.Equal(got, w))
}
