package basic

import (
	"strings"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nest "github.com/grafana/wiresmith/gen/basic/nesting/v1"
	ks "github.com/grafana/wiresmith/gen/test/kitchensink/v1"
)

// buildLevel1Payload returns a marshaled Level1 with n extras, padded past
// the pre-scan threshold (256 bytes) so the prealloc path runs.
func buildLevel1Payload(t *testing.T, n int) []byte {
	t.Helper()
	src := &nest.Level0_Level1{Value: strings.Repeat("x", 300)}
	for i := 0; i < n; i++ {
		src.Extras = append(src.Extras, nest.Level0_Level1_Level2{Value: "e"})
	}
	b, err := src.Marshal()
	require.NoError(t, err)
	require.Greater(t, len(b), 256, "payload must exceed preScanMinBytes")
	return b
}

// A caller-provided slice with enough capacity (the pooled-buffer pattern —
// Mimir hands wiresmith messages whose slices come from a sync.Pool, reset
// to length zero) must be reused by the pre-scan prealloc, not discarded
// for a fresh allocation.
func TestUnmarshal_PreScanReusesCallerCapacity(t *testing.T) {
	b := buildLevel1Payload(t, 8)

	dst := &nest.Level0_Level1{Extras: make([]nest.Level0_Level1_Level2, 0, 64)}
	pooled := unsafe.SliceData(dst.Extras)
	require.NoError(t, dst.Unmarshal(b))

	assert.Len(t, dst.Extras, 8)
	assert.Same(t, pooled, unsafe.SliceData(dst.Extras),
		"pre-scan prealloc must reuse a caller-provided backing array with sufficient capacity")
}

// When the caller's capacity is insufficient, the prealloc grows to exactly
// len+count — the count-exact sizing that the pre-scan exists for.
func TestUnmarshal_PreScanGrowsWhenCapacityInsufficient(t *testing.T) {
	b := buildLevel1Payload(t, 8)

	dst := &nest.Level0_Level1{Extras: make([]nest.Level0_Level1_Level2, 0, 2)}
	small := unsafe.SliceData(dst.Extras)
	require.NoError(t, dst.Unmarshal(b))

	assert.Len(t, dst.Extras, 8)
	assert.NotSame(t, small, unsafe.SliceData(dst.Extras),
		"capacity 2 cannot hold 8 elements — a fresh backing array is required")
	assert.GreaterOrEqual(t, cap(dst.Extras), 8)
}

// Unmarshal into a non-empty message APPENDS to repeated fields — gogo
// merge parity (user decision 2026-06-10, bead DB-13). The pre-scan
// prealloc must preserve existing elements when growing, and the result
// must be identical whether or not the pre-scan ran.
func TestUnmarshal_MergeAppendsRepeated_PreScanPath(t *testing.T) {
	b := buildLevel1Payload(t, 3) // > 256 bytes: pre-scan runs

	dst := &nest.Level0_Level1{}
	for i := 0; i < 4; i++ {
		dst.Extras = append(dst.Extras, nest.Level0_Level1_Level2{Value: "old"})
	}
	require.NoError(t, dst.Unmarshal(b))

	require.Len(t, dst.Extras, 7, "3 decoded elements must append after the 4 existing ones")
	for i := 0; i < 4; i++ {
		assert.Equal(t, "old", dst.Extras[i].Value, "existing element %d must survive the merge", i)
	}
	for i := 4; i < 7; i++ {
		assert.Equal(t, "e", dst.Extras[i].Value)
	}
}

// Same merge, payload below the pre-scan threshold — pins that the
// semantics are size-independent.
func TestUnmarshal_MergeAppendsRepeated_SmallPath(t *testing.T) {
	src := &nest.Level0_Level1{
		Extras: []nest.Level0_Level1_Level2{{Value: "e"}, {Value: "e"}, {Value: "e"}},
	}
	b, err := src.Marshal()
	require.NoError(t, err)
	require.Less(t, len(b), 256, "payload must stay below preScanMinBytes")

	dst := &nest.Level0_Level1{}
	for i := 0; i < 4; i++ {
		dst.Extras = append(dst.Extras, nest.Level0_Level1_Level2{Value: "old"})
	}
	require.NoError(t, dst.Unmarshal(b))

	require.Len(t, dst.Extras, 7)
	for i := 0; i < 4; i++ {
		assert.Equal(t, "old", dst.Extras[i].Value)
	}
	for i := 4; i < 7; i++ {
		assert.Equal(t, "e", dst.Extras[i].Value)
	}
}

// The merge must also preserve a pooled backing array when it already has
// room for len+count — pooling and merge semantics compose.
func TestUnmarshal_MergePreservesPooledCapacity(t *testing.T) {
	b := buildLevel1Payload(t, 3)

	dst := &nest.Level0_Level1{Extras: make([]nest.Level0_Level1_Level2, 0, 32)}
	dst.Extras = append(dst.Extras, nest.Level0_Level1_Level2{Value: "old"})
	pooled := unsafe.SliceData(dst.Extras)
	require.NoError(t, dst.Unmarshal(b))

	require.Len(t, dst.Extras, 4)
	assert.Equal(t, "old", dst.Extras[0].Value)
	assert.Same(t, pooled, unsafe.SliceData(dst.Extras),
		"capacity 32 fits 1+3 elements — the pooled array must be reused")
}

// Map fields merge too: the pre-scan must not discard an existing map.
// Wire entries overwrite same-key entries (last-write-wins) and add new
// ones — the same semantics the sub-threshold path always had.
func TestUnmarshal_MergePreservesMapEntries_PreScanPath(t *testing.T) {
	src := &ks.AllMaps{MapStringString: map[string]string{}}
	for r := 'a'; r <= 'z'; r++ {
		src.MapStringString[strings.Repeat(string(r), 8)] = strings.Repeat(string(r), 8)
	}
	b, err := src.Marshal()
	require.NoError(t, err)
	require.Greater(t, len(b), 256, "payload must exceed preScanMinBytes")

	dst := &ks.AllMaps{MapStringString: map[string]string{
		"pre-existing": "kept",
		"aaaaaaaa":     "overwritten",
	}}
	require.NoError(t, dst.Unmarshal(b))

	assert.Equal(t, "kept", dst.MapStringString["pre-existing"],
		"existing entry with a key not on the wire must survive")
	assert.Equal(t, "aaaaaaaa", dst.MapStringString["aaaaaaaa"],
		"wire entry must overwrite the same-key existing entry")
	assert.Len(t, dst.MapStringString, 27)
}
