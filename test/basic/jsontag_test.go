package basic

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	jt "github.com/grafana/wiresmith/gen/basic/jsontag/v1"
)

// TestJsonTag_StructTags asserts the generated `json:"..."` struct tag for
// every field in JsonTagHolder. This is the acceptance test for
// (wiresmith.options.jsontag): the user-supplied value is taken verbatim, and
// unannotated fields keep the default `<proto_field_name>,omitempty` shape.
func TestJsonTag_StructTags(t *testing.T) {
	holderType := reflect.TypeFor[jt.JsonTagHolder]()
	cases := []struct {
		field string
		want  string
	}{
		{"BlockId", "blockID"},                     // custom, no ,omitempty
		{"PlainName", "plain_name,omitempty"},      // default
		{"TotalObjects", "totalObjects,omitempty"}, // custom with explicit ,omitempty
		{"Head", "headLeaf"},                       // message field override
		{"Sizes", "sizeList"},                      // repeated scalar override
		{"Counters", "counterMap"},                 // map field override
		{"InternalOnly", "-"},                      // `-` override: encoding/json skips this field
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			f, ok := holderType.FieldByName(tc.field)
			require.True(t, ok, "field %s not found on JsonTagHolder", tc.field)
			assert.Equal(t, tc.want, f.Tag.Get("json"), "json struct tag mismatch on %s", tc.field)
		})
	}
}

// TestJsonTag_WireFormatUnchanged confirms the option only touches the Go-side
// struct tag — marshal output and round-trip behavior must be identical to a
// proto without any jsontag annotation.
func TestJsonTag_WireFormatUnchanged(t *testing.T) {
	msg := &jt.JsonTagHolder{
		BlockId:      "abc123",
		PlainName:    "demo",
		TotalObjects: 42,
		Head:         jt.Leaf{Id: 1, Name: "head"},
		Sizes:        []int32{10, 20, 30},
		Counters:     map[string]int64{"a": 1}, // single entry: wiresmith doesn't sort map iteration by design
		InternalOnly: "secret",
	}
	roundTrip(t, msg)
}

// TestJsonTag_ControlFieldDefault pins the default tag shape so a regression
// in tag.go's default branch would surface here rather than silently changing
// the JSON contract for unannotated fields.
func TestJsonTag_ControlFieldDefault(t *testing.T) {
	f, ok := reflect.TypeFor[jt.JsonTagHolder]().FieldByName("PlainName")
	require.True(t, ok)
	assert.Equal(t, "plain_name,omitempty", f.Tag.Get("json"),
		"default jsontag must remain `<proto_name>,omitempty` for unannotated fields")
}

// TestJsonTag_OneofVariants pins jsontag behavior on oneof variant wrapper
// structs. emit_oneof.go consults FileGenerator.fieldTag the same way
// emit_struct.go does, so the annotated variant gets `json:"sourceID"` and
// the control variant keeps the default `<proto_name>,omitempty` shape.
func TestJsonTag_OneofVariants(t *testing.T) {
	cases := []struct {
		wrapperType reflect.Type
		field       string
		want        string
	}{
		{reflect.TypeFor[jt.JsonTagHolder_SourceId](), "SourceId", "sourceID"},
		{reflect.TypeFor[jt.JsonTagHolder_RawSource](), "RawSource", "raw_source,omitempty"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			f, ok := tc.wrapperType.FieldByName(tc.field)
			require.True(t, ok, "field %s not found on %s", tc.field, tc.wrapperType)
			assert.Equal(t, tc.want, f.Tag.Get("json"))
		})
	}
}
