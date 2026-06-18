package basic

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ptr "github.com/grafana/wiresmith/gen/basic/pointer/v1"
	rec "github.com/grafana/wiresmith/gen/basic/recursive/v1"
	commonv1 "github.com/grafana/wiresmith/gen/opentelemetry/proto/common/v1"
	resourcev1 "github.com/grafana/wiresmith/gen/opentelemetry/proto/resource/v1"
	tracev1 "github.com/grafana/wiresmith/gen/opentelemetry/proto/trace/v1"
	ks "github.com/grafana/wiresmith/gen/test/kitchensink/v1"
)

// hexAddrRe matches a Go heap-pointer print (e.g. "0xc000123abc"). The
// pre-fix String() (fmt.Sprintf("%v", *m)) emitted these for every pointer
// field at depth >= 1 (optional scalars, oneofs, pointer-option, recursive),
// making the output allocation/run-dependent. The hand-rolled String()
// dereferences instead, so no address must ever appear.
var hexAddrRe = regexp.MustCompile(`0x[0-9a-fA-F]+`)

// stringer is the subset of the generated API the determinism tests exercise.
type stringer interface{ String() string }

// TestString_NoPointerAddresses is the core wiresmith-qcp1 regression: for
// every pointer-bearing field shape (and a value-typed-message-field message,
// the common OTel case that MessageStringOf would have panicked on), String()
// must render the VALUE, never a heap address, must not panic, and must be
// byte-stable across two independent renders of equal messages.
func TestString_NoPointerAddresses(t *testing.T) {
	int32v := int32(7)
	strv := "opt-value"

	cases := []struct {
		name string
		// mk builds a fresh, equal message each call so we can prove two
		// independent allocations render byte-identically (the old %v form
		// would differ because the embedded heap addresses differ).
		mk func() stringer
		// substrings the rendered text must contain (proves the value was
		// dereferenced, not printed as an address).
		wantContains []string
	}{
		{
			name:         "optional scalar",
			mk:           func() stringer { return &ks.AllOptionalScalars{FieldInt32: &int32v, FieldString: &strv} },
			wantContains: []string{"7", "opt-value"},
		},
		{
			name: "oneof",
			mk: func() stringer {
				return &ks.OneofVariants{Value: &ks.OneofVariants_StringValue{StringValue: "oneof-payload"}}
			},
			wantContains: []string{"oneof-payload"},
		},
		{
			name: "pointer-option (singular + repeated)",
			mk: func() stringer {
				return &ptr.PointerHolder{
					Name:  "holder",
					Head:  &ptr.Leaf{Id: 11, Name: "head-leaf"},
					Items: []*ptr.Leaf{{Id: 22, Name: "item-leaf"}},
				}
			},
			wantContains: []string{"head-leaf", "item-leaf"},
		},
		{
			name: "recursive",
			mk: func() stringer {
				return &rec.LinkedList{Value: 1, Next: &rec.LinkedList{Value: 2, Next: &rec.LinkedList{Value: 3}}}
			},
			wantContains: []string{"1", "2", "3"},
		},
		{
			name: "value-typed message field (OTel ResourceSpans)",
			mk: func() stringer {
				return &tracev1.ResourceSpans{
					Resource: resourcev1.Resource{
						Attributes: []commonv1.KeyValue{
							{Key: "service.name", Value: commonv1.AnyValue{
								Value: &commonv1.AnyValue_StringValue{StringValue: "checkout"},
							}},
						},
						DroppedAttributesCount: 3,
					},
					SchemaUrl: "https://schema",
				}
			},
			wantContains: []string{"service.name", "checkout", "https://schema"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var s string
			require.NotPanics(t, func() { s = tc.mk().String() }, "String() must not panic")
			assert.NotEmpty(t, s)
			assert.NotRegexp(t, hexAddrRe, s,
				"String() leaked a heap address for shape %q: %q", tc.name, s)
			for _, want := range tc.wantContains {
				assert.Contains(t, s, want, "String() must render the field value, not an address")
			}
			// Byte-stable across two independent allocations: the hand-rolled
			// String() has no prototext detrand jitter, so equal messages must
			// produce byte-identical output regardless of where they were
			// allocated.
			assert.Equal(t, tc.mk().String(), tc.mk().String(),
				"String() must be byte-deterministic across independent allocations")
		})
	}
}

// TestString_NilReceiver pins the nil-safety contract: a nil receiver returns
// "<nil>" and never panics.
func TestString_NilReceiver(t *testing.T) {
	var optional *ks.AllOptionalScalars
	var oneof *ks.OneofVariants
	var holder *ptr.PointerHolder
	var list *rec.LinkedList
	var spans *tracev1.ResourceSpans

	assert.NotPanics(t, func() {
		assert.Equal(t, "<nil>", optional.String())
		assert.Equal(t, "<nil>", oneof.String())
		assert.Equal(t, "<nil>", holder.String())
		assert.Equal(t, "<nil>", list.String())
		assert.Equal(t, "<nil>", spans.String())
	})
}

// TestString_UnsetOneofNoPanic confirms a message with an unset oneof renders
// without panic (an unset oneof contributes no output).
func TestString_UnsetOneofNoPanic(t *testing.T) {
	m := &ks.OneofVariants{}
	var s string
	require.NotPanics(t, func() { s = m.String() })
	assert.NotRegexp(t, hexAddrRe, s)
}

// TestString_ProtoTextShape confirms the rendered form is proto-text: proto
// field names with `: ` labels, nested messages in braces, strings quoted, and
// the set oneof variant by its proto field name.
func TestString_ProtoTextShape(t *testing.T) {
	m := &tracev1.ResourceSpans{
		Resource: resourcev1.Resource{
			Attributes: []commonv1.KeyValue{
				{Key: "service.name", Value: commonv1.AnyValue{
					Value: &commonv1.AnyValue_StringValue{StringValue: "checkout"},
				}},
			},
		},
		SchemaUrl: "https://schema",
	}
	s := m.String()
	// proto field names with proto-text labels.
	assert.Contains(t, s, "resource: {")
	assert.Contains(t, s, "schema_url: ")
	assert.Contains(t, s, "attributes: {")
	assert.Contains(t, s, `key: "service.name"`)
	// oneof variant rendered by its proto field name.
	assert.Contains(t, s, "string_value: ")
	assert.Contains(t, s, `"checkout"`)
	// strings are quoted.
	assert.Contains(t, s, `"https://schema"`)
	// no Go-internal/struct leakage or addresses.
	assert.NotRegexp(t, hexAddrRe, s)
	assert.NotContains(t, s, "XXX_")
}

// TestString_EnumByName confirms enum fields render by their value NAME (via
// the enum's String()), not the raw integer.
func TestString_EnumByName(t *testing.T) {
	m := &ks.WithEnum{Color: ks.Color_COLOR_BLUE}
	s := m.String()
	assert.Contains(t, s, "color: ")
	assert.Contains(t, s, "COLOR_BLUE")
	assert.NotContains(t, s, "color: 2")
}
