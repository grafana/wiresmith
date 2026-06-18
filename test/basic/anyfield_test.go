package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"

	af "github.com/grafana/wiresmith/gen/basic/anyfield/v1"
	"github.com/grafana/wiresmith/types/known/anypb"
)

func sampleAny(url string, val []byte) anypb.Any {
	return anypb.Any{TypeUrl: url, Value: val}
}

// google.protobuf.Any resolves to wiresmith's replacement and round-trips as a
// singular and a repeated field. Importing the generated package also exercises
// its protoimpl.TypeBuilder init (Any lands in goTypes via the hand-written
// delegating ProtoReflect) — a registration conflict there would panic before
// any test ran.
func TestAnyField_RoundTrip(t *testing.T) {
	msg := &af.Holder{
		Payload: sampleAny("type.googleapis.com/foo.Bar", []byte{1, 2, 3}),
		Items: []anypb.Any{
			sampleAny("type.googleapis.com/a.A", []byte("a")),
			sampleAny("type.googleapis.com/b.B", nil),
		},
	}
	b, err := msg.Marshal()
	require.NoError(t, err)

	dst := &af.Holder{}
	require.NoError(t, dst.Unmarshal(b))
	assert.True(t, msg.Equal(dst))
	assert.Equal(t, 0, msg.Compare(dst))
	assert.Equal(t, msg.Size(), len(b))
	// Field-level decode check (Holder keeps its presence bitmap, so a literal
	// reflect.DeepEqual would diff on XXX_fieldsPresent — Equal is the
	// semantic comparison).
	assert.Equal(t, "type.googleapis.com/foo.Bar", dst.Payload.TypeUrl)
	assert.Equal(t, []byte{1, 2, 3}, dst.Payload.Value)
	require.Len(t, dst.Items, 2)
	assert.Equal(t, "type.googleapis.com/a.A", dst.Items[0].TypeUrl)
}

// Getter shape follows the uniform message-field rule (der5): singular Any
// getter returns *anypb.Any, repeated returns the slice.
func TestAnyField_GetterShape(t *testing.T) {
	// Decode so the singular field's presence bit is set (Holder is a default
	// bitmap message; its getter gates on the bit).
	b, err := (&af.Holder{Payload: sampleAny("type.googleapis.com/x.X", []byte{9})}).Marshal()
	require.NoError(t, err)
	h := &af.Holder{}
	require.NoError(t, h.Unmarshal(b))

	var got *anypb.Any = h.GetPayload() // compile-time assertion of *anypb.Any
	require.NotNil(t, got)
	assert.Equal(t, "type.googleapis.com/x.X", got.GetTypeUrl())

	var nilHolder *af.Holder
	assert.Nil(t, nilHolder.GetPayload())
	assert.Nil(t, nilHolder.GetItems())
}

// The typed Any helpers pack/unpack a real registered message and resolve its
// type from the global registry — parity with the official/gogo Any API.
func TestAny_TypedHelpers(t *testing.T) {
	src := wrapperspb.String("hello")

	packed, err := anypb.New(src)
	require.NoError(t, err)
	assert.Equal(t, "google.protobuf.StringValue", packed.TypeName())
	assert.True(t, packed.MessageIs(&wrapperspb.StringValue{}))

	var dst wrapperspb.StringValue
	require.NoError(t, packed.UnmarshalTo(&dst))
	assert.Equal(t, "hello", dst.GetValue())

	// Mismatched destination type is rejected.
	require.Error(t, packed.UnmarshalTo(&wrapperspb.Int32Value{}))

	// UnmarshalNew resolves the type from the global registry.
	got, err := packed.UnmarshalNew()
	require.NoError(t, err)
	gotSV, ok := got.(*wrapperspb.StringValue)
	require.True(t, ok)
	assert.Equal(t, "hello", gotSV.GetValue())
}
