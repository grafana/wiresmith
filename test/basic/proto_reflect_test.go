package basic_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	commonv1 "wiresmith/gen/otlp/common/v1"
	resourcev1 "wiresmith/gen/otlp/resource/v1"
)

// TestProtoReflectNewReturnsWrappedFresh verifies that ProtoReflect().New()
// returns a freshly allocated message wrapped in wiresmith's MessageReflect,
// not the protoimpl-backed reflection — which would be unsafe for value-typed
// message fields. The check is: New().Interface() returns a *Resource whose
// fields are zero-valued, and the returned protoreflect.Message itself does
// not panic on the documented fast-path operations.
func TestProtoReflectNewReturnsWrappedFresh(t *testing.T) {
	src := &resourcev1.Resource{
		DroppedAttributesCount: 7,
	}
	fresh := src.ProtoReflect().New()
	if fresh == nil {
		t.Fatal("ProtoReflect().New() returned nil")
	}

	gotMsg, ok := fresh.Interface().(*resourcev1.Resource)
	if !ok {
		t.Fatalf("New().Interface() = %T, want *Resource", fresh.Interface())
	}
	if gotMsg == src {
		t.Fatal("New() returned the same pointer as the source — it must allocate a fresh instance")
	}
	if gotMsg.DroppedAttributesCount != 0 {
		t.Fatalf("New() returned non-zero field: DroppedAttributesCount=%d, want 0", gotMsg.DroppedAttributesCount)
	}

	// IsValid must report true for a freshly-allocated, non-nil message.
	if !fresh.IsValid() {
		t.Fatal("IsValid() reports false on a fresh non-nil message")
	}
}

// TestProtoReflectIsValidDetectsTypedNil ensures the IsValid implementation
// handles typed-nil pointers correctly. (*Resource)(nil) wrapped in an
// interface is non-nil at the interface level, but the underlying pointer
// is nil; protoreflect contract requires IsValid to report false for that.
func TestProtoReflectIsValidDetectsTypedNil(t *testing.T) {
	var nilResource *resourcev1.Resource
	got := nilResource.ProtoReflect().IsValid()
	if got {
		t.Fatal("IsValid() returned true for a typed-nil receiver — must return false")
	}

	// Sanity check: a real instance reports valid.
	real := &resourcev1.Resource{}
	if !real.ProtoReflect().IsValid() {
		t.Fatal("IsValid() returned false for a real instance")
	}
}

// TestProtoMarshalUnmarshalRoundTripsViaReflect verifies that proto.Marshal
// and proto.Unmarshal work on wiresmith-generated messages by going through
// the ProtoMethods fast-path (avoiding field-level reflection, which would
// panic on our value-typed message fields).
func TestProtoMarshalUnmarshalRoundTripsViaReflect(t *testing.T) {
	src := &resourcev1.Resource{
		DroppedAttributesCount: 42,
		Attributes: []commonv1.KeyValue{
			{Key: "service.name", Value: commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "test"}}},
		},
	}

	data, err := proto.Marshal(src)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}

	dst := &resourcev1.Resource{}
	if err := proto.Unmarshal(data, dst); err != nil {
		t.Fatalf("proto.Unmarshal: %v", err)
	}

	if dst.DroppedAttributesCount != src.DroppedAttributesCount {
		t.Errorf("DroppedAttributesCount: got %d, want %d", dst.DroppedAttributesCount, src.DroppedAttributesCount)
	}
	if len(dst.Attributes) != 1 || dst.Attributes[0].Key != "service.name" {
		t.Errorf("Attributes mismatch: %+v", dst.Attributes)
	}
}

// TestProtoEqualViaReflect verifies proto.Equal works via the ProtoMethods
// fast-path. Without our Equal hook in wiresmithMethods, proto.Equal would
// fall back to field-level reflection and panic.
func TestProtoEqualViaReflect(t *testing.T) {
	a := &resourcev1.Resource{DroppedAttributesCount: 3}
	b := &resourcev1.Resource{DroppedAttributesCount: 3}
	c := &resourcev1.Resource{DroppedAttributesCount: 4}

	if !proto.Equal(a, b) {
		t.Error("proto.Equal returned false for identical resources")
	}
	if proto.Equal(a, c) {
		t.Error("proto.Equal returned true for distinct resources")
	}
}

// TestProtoMessageNameDescriptorWorks verifies metadata reflection works:
// proto.MessageName and Descriptor.FullName both go through the registered
// MessageDescriptor on protoimpl.MessageInfo.
func TestProtoMessageNameDescriptorWorks(t *testing.T) {
	r := &resourcev1.Resource{}
	got := proto.MessageName(r)
	want := "opentelemetry.proto.resource.v1.Resource"
	if string(got) != want {
		t.Errorf("MessageName: got %q, want %q", got, want)
	}
	descName := r.ProtoReflect().Descriptor().FullName()
	if string(descName) != want {
		t.Errorf("Descriptor().FullName(): got %q, want %q", descName, want)
	}
}
