package basic

import (
	"strings"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nest "github.com/grafana/wiresmith/gen/basic/nesting/v1"
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
// Mimir hands wiresmith messages whose slices come from a sync.Pool) must be
// reused by the pre-scan prealloc, not discarded for a fresh allocation.
func TestUnmarshal_PreScanReusesCallerCapacity(t *testing.T) {
	b := buildLevel1Payload(t, 8)

	dst := &nest.Level0_Level1{Extras: make([]nest.Level0_Level1_Level2, 0, 64)}
	pooled := unsafe.SliceData(dst.Extras)
	require.NoError(t, dst.Unmarshal(b))

	assert.Len(t, dst.Extras, 8)
	assert.Same(t, pooled, unsafe.SliceData(dst.Extras),
		"pre-scan prealloc must reuse a caller-provided backing array with sufficient capacity")
}

// When the caller's capacity is insufficient, the prealloc allocates fresh —
// the count-exact sizing that the pre-scan exists for.
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

// Reuse truncates: stale elements beyond the decoded length must not leak
// through. (The backing array is reused, so the *bytes* may persist, but the
// slice must be rebuilt from length zero.)
func TestUnmarshal_PreScanReuseTruncatesPriorContents(t *testing.T) {
	b := buildLevel1Payload(t, 3)

	dst := &nest.Level0_Level1{Extras: make([]nest.Level0_Level1_Level2, 0, 16)}
	for i := 0; i < 10; i++ {
		dst.Extras = append(dst.Extras, nest.Level0_Level1_Level2{Value: "stale"})
	}
	dst.Extras = dst.Extras[:0]
	require.NoError(t, dst.Unmarshal(b))

	require.Len(t, dst.Extras, 3)
	for _, e := range dst.Extras {
		assert.Equal(t, "e", e.Value)
	}
}
