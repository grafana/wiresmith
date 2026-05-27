package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"

	rec "wiresmith/gen/basic/recursive/v1"
	commonv1 "wiresmith/gen/opentelemetry/proto/common/v1"
)

// buildNestedAnyValueBytes constructs wire-format bytes for an AnyValue nested
// to the given depth via the AnyValue -> ArrayValue -> AnyValue cycle.
// The innermost AnyValue holds a string "hello".
func buildNestedAnyValueBytes(depth int) []byte {
	// Innermost: AnyValue with string_value = "hello"
	// Field 1 (string_value): tag(1, BytesType) + len + "hello"
	inner := protowire.AppendTag(nil, 1, protowire.BytesType)
	inner = protowire.AppendString(inner, "hello")

	for range depth {
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
		for i := range 5 {
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

// buildNestedLinkedListBytes builds a depth-N LinkedList wire payload by
// repeatedly wrapping the previous payload in field 2 (next, optional
// LinkedList). Each wrap adds one level of recursion when unmarshaling.
func buildNestedLinkedListBytes(depth int) []byte {
	// Innermost: a LinkedList with value = 42 (field 1, varint)
	inner := protowire.AppendTag(nil, 1, protowire.VarintType)
	inner = protowire.AppendVarint(inner, 42)

	for range depth {
		outer := protowire.AppendTag(nil, 2, protowire.BytesType)
		outer = protowire.AppendBytes(outer, inner)
		inner = outer
	}
	return inner
}

// buildNestedTreeNodeBytes builds a depth-N TreeNode wire payload by repeatedly
// wrapping the previous payload as a single child (field 3, repeated TreeNode).
func buildNestedTreeNodeBytes(depth int) []byte {
	inner := protowire.AppendTag(nil, 1, protowire.BytesType)
	inner = protowire.AppendString(inner, "leaf")

	for range depth {
		outer := protowire.AppendTag(nil, 3, protowire.BytesType)
		outer = protowire.AppendBytes(outer, inner)
		inner = outer
	}
	return inner
}

// buildNestedNodeABytes builds a NodeA payload that nests through the
// NodeA→NodeB→NodeA mutual-recursion chain. Each "step" wraps the current
// payload first as NodeB.parent (field 2, optional NodeA) and then as
// NodeA.peer (field 2, optional NodeB), adding 2 recursion increments per
// step.
func buildNestedNodeABytes(steps int) []byte {
	// Innermost: NodeA with name = "leaf" (field 1, string)
	inner := protowire.AppendTag(nil, 1, protowire.BytesType)
	inner = protowire.AppendString(inner, "leaf")

	// Wrap pairs: NodeB(parent=NodeA(peer=...))
	for range steps {
		// Wrap as NodeB.parent (field 2, optional NodeA)
		nb := protowire.AppendTag(nil, 2, protowire.BytesType)
		nb = protowire.AppendBytes(nb, inner)
		// Wrap as NodeA.peer (field 2, optional NodeB)
		na := protowire.AppendTag(nil, 2, protowire.BytesType)
		na = protowire.AppendBytes(na, nb)
		inner = na
	}
	return inner
}

func TestRecursiveUnmarshalDepthLimit(t *testing.T) {
	t.Run("LinkedList shallow", func(t *testing.T) {
		b := buildNestedLinkedListBytes(5)
		var ll rec.LinkedList
		require.NoError(t, ll.Unmarshal(b))
		require.NotNil(t, ll.Next)
	})

	t.Run("LinkedList exceeds limit", func(t *testing.T) {
		// One level per wrap; 10001 wraps exceed maxUnmarshalDepth=10000.
		b := buildNestedLinkedListBytes(10001)
		var ll rec.LinkedList
		err := ll.Unmarshal(b)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeded max recursion depth")
	})

	t.Run("TreeNode shallow", func(t *testing.T) {
		b := buildNestedTreeNodeBytes(5)
		var tn rec.TreeNode
		require.NoError(t, tn.Unmarshal(b))
		require.Len(t, tn.Children, 1)
	})

	t.Run("TreeNode exceeds limit", func(t *testing.T) {
		b := buildNestedTreeNodeBytes(10001)
		var tn rec.TreeNode
		err := tn.Unmarshal(b)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeded max recursion depth")
	})

	t.Run("NodeA shallow", func(t *testing.T) {
		b := buildNestedNodeABytes(3)
		var na rec.NodeA
		require.NoError(t, na.Unmarshal(b))
		require.NotNil(t, na.Peer)
		require.NotNil(t, na.Peer.Parent)
	})

	t.Run("NodeA exceeds limit", func(t *testing.T) {
		// Each step adds 2 recursion increments (NodeA→NodeB→NodeA), so 5001
		// steps = 10002 increments, past the 10000 limit.
		b := buildNestedNodeABytes(5001)
		var na rec.NodeA
		err := na.Unmarshal(b)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeded max recursion depth")
	})
}

// TestUnmarshalWithDepthRespectsStartingDepth pins the SEC-5 fix at the
// public-API level: UnmarshalWithDepth is the cross-package entry point,
// and a payload that would normally unmarshal cleanly at depth 0 must
// fail when entered close enough to the maxUnmarshalDepth ceiling.
// Without this contract, the cross-package emit site (which passes
// `depth+1` from the outer message) couldn't actually rate-limit nested
// recursion at the boundary — the receiver would silently restart.
func TestUnmarshalWithDepthRespectsStartingDepth(t *testing.T) {
	// One-level nesting: AnyValue containing an ArrayValue containing
	// one AnyValue("hello"). At depth 0 this trivially succeeds.
	b := buildNestedAnyValueBytes(1)

	t.Run("succeeds at depth 0", func(t *testing.T) {
		var av commonv1.AnyValue
		require.NoError(t, av.UnmarshalWithDepth(b, 0))
	})

	t.Run("fails when started near the ceiling", func(t *testing.T) {
		// buildNestedAnyValueBytes(1) recurses 2 levels (AnyValue ->
		// ArrayValue -> AnyValue). Entering at depth 9999 means the
		// deepest inner AnyValue lands at depth 10001 > maxUnmarshalDepth.
		// Pre-fix code routed cross-package callers through Unmarshal(b)
		// which discarded the caller's depth — that path would happily
		// process this payload no matter how deep the outer recursion
		// already was.
		var av commonv1.AnyValue
		err := av.UnmarshalWithDepth(b, 9999)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeded max recursion depth")
	})
}
