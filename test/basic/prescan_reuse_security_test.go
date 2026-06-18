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

// Security regression tests for the DB-6 pre-scan backing-array REUSE
// (`if len(m.X)==0 && cap(m.X)<c { make(...,0,c) }` in emitPreScan). These
// pin the invariant the whole reuse contract rests on: NO generated path may
// observe slice elements beyond `len`, even when the backing array physically
// retains a previous decode's data in `[len:cap]`. The threat model is a
// pooled message (the Mimir / gRPC-codec pattern: `m.X = m.X[:0]` keeps the
// capacity) decoding tenant-A data, being truncated to `[:0]`, then decoding
// shorter tenant-B data — tenant-A bytes survive in the stale tail and must
// never leak through Marshal, Size, Equal, Compare, String, or a getter.
//
// See docs/design.md "DB-6 reuse-safety review (wiresmith-u4qg)" for the
// adversarial analysis these tests encode. If a future codegen change starts
// touching `[:cap]` anywhere (e.g. a getter that returns `m.X[:cap(m.X)]`, or
// a Marshal that ranges past len), these assertions fail loudly.

// markerA / markerB are deliberately distinct so a leaked tenant-A byte is
// trivially detectable in any tenant-B observation.
const (
	markerA = "TENANT-A-SECRET"
	markerB = "tenant-b"
)

// poolReset emulates the pooled-message truncation that retains capacity —
// the realistic reuse trigger (NOT Reset(), which nils the slice via
// `*m = T{}`). Returning the original backing-array pointer lets callers
// assert the array was actually reused.
func poolReset(m *nest.Level0_Level1) unsafe.Pointer {
	m.Extras = m.Extras[:0]
	m.XXX_fieldsPresent = [1]uint64{}
	m.Value = ""
	return unsafe.Pointer(unsafe.SliceData(m.Extras))
}

// buildExtrasPayload marshals a Level1 carrying n Extras each with the given
// marker value, padded past preScanMinBytes so the pre-scan (and thus the
// reuse guard) runs.
func buildExtrasPayload(t *testing.T, marker string, n int) ([]byte, *nest.Level0_Level1) {
	t.Helper()
	src := &nest.Level0_Level1{Value: strings.Repeat("p", 300)}
	for i := 0; i < n; i++ {
		src.Extras = append(src.Extras, nest.Level0_Level1_Level2{Value: marker})
	}
	b, err := src.Marshal()
	require.NoError(t, err)
	require.Greater(t, len(b), 256, "payload must exceed preScanMinBytes so reuse guard runs")
	return b, src
}

// TestPreScanReuse_NoCrossTenantLeakViaAnyMethod is the headline cross-tenant
// scenario: decode tenant-A (8 elements), truncate to [:0] keeping the backing
// array, decode shorter tenant-B (3 elements). The reused array still holds
// tenant-A's secret in slots [3:8] (we prove that), yet NO generated method
// may surface those bytes.
func TestPreScanReuse_NoCrossTenantLeakViaAnyMethod(t *testing.T) {
	const aCount, bCount = 8, 3

	aPayload, _ := buildExtrasPayload(t, markerA, aCount)
	bPayload, bSrc := buildExtrasPayload(t, markerB, bCount)

	dst := &nest.Level0_Level1{}
	require.NoError(t, dst.Unmarshal(aPayload))
	require.Len(t, dst.Extras, aCount)
	require.GreaterOrEqual(t, cap(dst.Extras), aCount)

	// Pooled truncation: capacity (and tenant-A's data in the tail) retained.
	arrA := poolReset(dst)
	require.Equal(t, 0, len(dst.Extras))
	require.GreaterOrEqual(t, cap(dst.Extras), aCount, "pool reset must retain capacity")

	require.NoError(t, dst.Unmarshal(bPayload))

	// Reuse actually happened: same backing array (cap >= bCount held).
	require.Equal(t, arrA, unsafe.Pointer(unsafe.SliceData(dst.Extras)),
		"capacity >= bCount must reuse the tenant-A backing array (this is the scenario under test)")
	require.Len(t, dst.Extras, bCount)

	// PROOF the hazard is real: tenant-A bytes physically persist in the stale
	// tail [bCount:aCount] of the reused backing array. The ONLY thing that
	// keeps them invisible is every method honoring `len`. We read the tail via
	// an explicit reslice to cap — exactly what a buggy getter/marshal must NOT
	// do implicitly.
	tail := dst.Extras[:cap(dst.Extras)][bCount:aCount]
	leakedFound := false
	for i := range tail {
		if tail[i].Value == markerA {
			leakedFound = true
			break
		}
	}
	require.True(t, leakedFound,
		"sanity: tenant-A secret must still be present in the stale tail — "+
			"otherwise this test would pass vacuously")

	// Now assert every observable path sees ONLY tenant-B [:len], never the tail.

	// (1) Getter returns a len-bounded slice.
	got := dst.GetExtras()
	require.Len(t, got, bCount)
	for i := range got {
		assert.Equal(t, markerB, got[i].Value, "getter element %d", i)
	}

	// (2) Marshal must not emit tenant-A bytes, and must round-trip to exactly
	// tenant-B.
	out, err := dst.Marshal()
	require.NoError(t, err)
	assert.NotContains(t, string(out), markerA,
		"Marshal must not serialize stale tenant-A data from [len:cap]")
	var redecoded nest.Level0_Level1
	require.NoError(t, redecoded.Unmarshal(out))
	assert.Len(t, redecoded.Extras, bCount)

	// (3) Size counts only [:len]: a fresh tenant-B message marshals to the
	// same length.
	assert.Equal(t, bSrc.Size(), dst.Size(),
		"Size must count only [:len]; stale tail must not inflate it")

	// (4) Equal against a fresh tenant-B message: must be equal (tail ignored).
	assert.True(t, dst.Equal(bSrc), "Equal must compare only [:len]")
	// ...and unequal to tenant-A (defensive: confirms Equal is value-sensitive).
	aFresh := &nest.Level0_Level1{}
	require.NoError(t, aFresh.Unmarshal(aPayload))
	assert.False(t, dst.Equal(aFresh), "tenant-B must not equal tenant-A")

	// (5) Compare against a fresh tenant-B message: total order says equal.
	assert.Equal(t, 0, dst.Compare(bSrc), "Compare must see only [:len]")

	// (6) String must not render tenant-A bytes.
	assert.NotContains(t, dst.String(), markerA,
		"String must not render stale tenant-A data from [len:cap]")
	assert.Contains(t, dst.String(), markerB)
}

// TestPreScanReuse_AppendZeroesReusedSlot pins the mechanism that makes the
// above safe even WITHIN [:len]: the main decode loop appends a FRESH zero
// composite literal (`append(m.Extras, Level2{})`) before decoding into the
// slot, so a slot that previously held tenant-A data is zeroed first. Here
// tenant-B's elements have an EMPTY Value, so if the append reused the old
// slot's contents instead of zeroing, tenant-A's marker would survive in-band
// (inside [:len]) — a far worse leak than the tail case.
func TestPreScanReuse_AppendZeroesReusedSlot(t *testing.T) {
	const count = 4

	// Tenant A: non-empty marker values.
	aPayload, _ := buildExtrasPayload(t, markerA, count)

	dst := &nest.Level0_Level1{}
	require.NoError(t, dst.Unmarshal(aPayload))
	arrA := poolReset(dst)

	// Tenant B: SAME element count, but each element has an empty Value (the
	// proto3 default is not serialized), so the decode writes nothing into the
	// Value field. Only a zeroing append keeps tenant-A's marker out of [:len].
	src := &nest.Level0_Level1{Value: strings.Repeat("q", 300)}
	for i := 0; i < count; i++ {
		src.Extras = append(src.Extras, nest.Level0_Level1_Level2{}) // empty Value
	}
	bPayload, err := src.Marshal()
	require.NoError(t, err)
	require.Greater(t, len(bPayload), 256)

	require.NoError(t, dst.Unmarshal(bPayload))
	require.Equal(t, arrA, unsafe.Pointer(unsafe.SliceData(dst.Extras)),
		"same-count reuse must hit the retained backing array")
	require.Len(t, dst.Extras, count)

	for i := range dst.Extras {
		assert.Equal(t, "", dst.Extras[i].Value,
			"element %d: append must zero the reused slot before decode; "+
				"tenant-A marker must not survive in-band", i)
	}
	out, err := dst.Marshal()
	require.NoError(t, err)
	assert.NotContains(t, string(out), markerA, "no tenant-A leak in-band")
}

// TestPreScanReuse_ErrorPathNoStaleData covers the error-path partial-state
// scenario: a decode that fails partway through a reused array must leave the
// message holding only zeroed/partial tenant-B data in [:len] — never stale
// tenant-A bytes. We craft a tenant-B payload whose final repeated element is
// truncated (its declared length runs past the buffer), so unmarshal returns
// io.ErrUnexpectedEOF after appending some elements.
func TestPreScanReuse_ErrorPathNoStaleData(t *testing.T) {
	const aCount = 8

	aPayload, _ := buildExtrasPayload(t, markerA, aCount)
	dst := &nest.Level0_Level1{}
	require.NoError(t, dst.Unmarshal(aPayload))
	arrA := poolReset(dst)

	// Build a valid 4-element tenant-B payload, then corrupt the LAST element's
	// length prefix so decode of that element overruns the buffer. We do this
	// by truncating the byte stream mid-final-element: the decoder appends the
	// first elements, then hits ErrUnexpectedEOF.
	good, _ := buildExtrasPayload(t, markerB, 4)
	// Truncate the last few bytes so the final length-delimited element is
	// short. The exact cut point only needs to land inside the last element.
	corrupt := good[:len(good)-3]

	err := dst.Unmarshal(corrupt)
	require.Error(t, err, "truncated final element must error")

	// Not vacuous: the decoder must have made partial progress (appended some
	// tenant-B elements) before erroring — that's the partial-state condition
	// the scenario is about.
	require.NotEmpty(t, dst.Extras, "decode must leave partial state to exercise the scenario")

	// Whatever partial state remains, [:len] must contain NO tenant-A marker.
	require.Equal(t, arrA, unsafe.Pointer(unsafe.SliceData(dst.Extras)),
		"error path still reuses the backing array")
	for i := range dst.Extras {
		assert.NotEqual(t, markerA, dst.Extras[i].Value,
			"error-path element %d must not expose stale tenant-A data", i)
	}
	// And Marshal of the partial message must not leak tenant-A either.
	out, mErr := dst.Marshal()
	require.NoError(t, mErr)
	assert.NotContains(t, string(out), markerA,
		"partial-state Marshal must not serialize stale tenant-A data")
}

// TestPreScanReuse_RepeatedStringTail extends the no-leak property to a
// repeated STRING field (Names) and a repeated MESSAGE field (Items) on a
// different message, ensuring the invariant is not specific to one struct
// shape. Repeated strings are appended as fresh `string(dAtA[...])` copies, so
// there is no aliasing of the wire buffer; the stale tail must still be
// invisible.
func TestPreScanReuse_RepeatedStringTail(t *testing.T) {
	buildNames := func(marker string, n int) []byte {
		src := &ks.OnlyRepeated{}
		// Pad each name so even the small (3-element) tenant-B payload clears
		// preScanMinBytes (256) and the reuse guard runs on BOTH decodes.
		for i := 0; i < n; i++ {
			src.Names = append(src.Names, marker+strings.Repeat("z", 110))
		}
		b, err := src.Marshal()
		require.NoError(t, err)
		require.Greater(t, len(b), 256)
		return b
	}

	dst := &ks.OnlyRepeated{}
	require.NoError(t, dst.Unmarshal(buildNames(markerA, 8)))
	require.Len(t, dst.Names, 8)

	// Pooled truncation keeping capacity.
	dst.Names = dst.Names[:0]
	arr := unsafe.Pointer(unsafe.SliceData(dst.Names))

	require.NoError(t, dst.Unmarshal(buildNames(markerB, 3)))
	require.Equal(t, arr, unsafe.Pointer(unsafe.SliceData(dst.Names)),
		"sufficient capacity must reuse the backing array")
	require.Len(t, dst.Names, 3)

	// Tail still holds tenant-A strings (sanity), but no method may surface them.
	tail := dst.Names[:cap(dst.Names)][3:8]
	staleSeen := false
	for _, s := range tail {
		if strings.Contains(s, markerA) {
			staleSeen = true
		}
	}
	require.True(t, staleSeen, "sanity: tenant-A strings persist in the stale tail")

	for _, s := range dst.GetNames() {
		assert.NotContains(t, s, markerA)
		assert.Contains(t, s, markerB)
	}
	out, err := dst.Marshal()
	require.NoError(t, err)
	assert.NotContains(t, string(out), markerA,
		"Marshal must not serialize stale tenant-A strings from [len:cap]")
}
