package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"

	commonv1 "wiresmith/gen/otlp/common/v1"
)

// buildNestedAnyValueBytes constructs wire-format bytes for an AnyValue nested
// to the given depth via the AnyValue -> ArrayValue -> AnyValue cycle.
// The innermost AnyValue holds a string "hello".
func buildNestedAnyValueBytes(depth int) []byte {
	// Innermost: AnyValue with string_value = "hello"
	// Field 1 (string_value): tag(1, BytesType) + len + "hello"
	inner := protowire.AppendTag(nil, 1, protowire.BytesType)
	inner = protowire.AppendString(inner, "hello")

	for i := 0; i < depth; i++ {
		// Wrap inner AnyValue bytes in an ArrayValue.
		// ArrayValue field 1 (repeated AnyValue): tag(1, BytesType) + len + inner
		arrayValue := protowire.AppendTag(nil, 1, protowire.BytesType)
		arrayValue = protowire.AppendBytes(arrayValue, inner)

		// Wrap ArrayValue in an AnyValue.
		// AnyValue field 5 (array_value): tag(5, BytesType) + len + arrayValue
		anyValue := protowire.AppendTag(nil, 5, protowire.BytesType)
		anyValue = protowire.AppendBytes(anyValue, arrayValue)

		inner = anyValue
	}

	return inner
}

func TestUnmarshalDepthLimit(t *testing.T) {
	t.Run("shallow nesting succeeds", func(t *testing.T) {
		// 5 levels of nesting should unmarshal without error.
		b := buildNestedAnyValueBytes(5)
		var av commonv1.AnyValue
		err := av.Unmarshal(b)
		require.NoError(t, err)

		// Verify the innermost value is accessible.
		cur := &av
		for i := 0; i < 5; i++ {
			variant, ok := cur.Value.(*commonv1.AnyValue_ArrayValue)
			require.True(t, ok, "expected ArrayValue at depth %d", i)
			require.Len(t, variant.ArrayValue.Values, 1)
			cur = &variant.ArrayValue.Values[0]
		}
		sv, ok := cur.Value.(*commonv1.AnyValue_StringValue)
		require.True(t, ok)
		assert.Equal(t, "hello", sv.StringValue)
	})

	t.Run("deep nesting returns error", func(t *testing.T) {
		// Each nesting level adds 2 recursion depth increments
		// (AnyValue -> ArrayValue -> AnyValue), so 10001 wraps = 20002
		// depth increments, well past the 10000 limit.
		b := buildNestedAnyValueBytes(10001)
		var av commonv1.AnyValue
		err := av.Unmarshal(b)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeded max recursion depth")
	})
}
