package basic

import (
	"strings"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	numericv1 "github.com/grafana/wiresmith/gen/basic/numeric/v1"
	tracev1 "github.com/grafana/wiresmith/gen/opentelemetry/proto/trace/v1"
)

// UnmarshalNoPrescan (DB-18) decodes identically to Unmarshal but skips the
// TOP-LEVEL repeated-field counting pre-scan. It is the escape hatch for
// callers that unmarshal into a REUSED/pooled message (len>0, or reset-to-[:0]
// with retained cap), where the pre-scan's `len==0 && cap<count` prealloc guard
// never fires and the scan is pure overhead. Fresh-message and nested
// pre-scans, which genuinely pay, are preserved.

// TestUnmarshalNoPrescan_DecodesIdenticallyButSkipsPrealloc proves the two
// halves of the top-level contract on a fresh message:
//
//	(a) the decoded result is deep-equal to a normal Unmarshal; and
//	(b) the repeated slice is NOT exact-prealloced — i.e. it was grown by the
//	    main loop's amortized append rather than the pre-scan's count-exact
//	    make([]T, 0, count). We detect this via the cap: the normal Unmarshal
//	    path exact-fits cap==len, whereas append-growth of a count that is not
//	    a power of two over-allocates (cap > len).
func TestUnmarshalNoPrescan_DecodesIdenticallyButSkipsPrealloc(t *testing.T) {
	// 9 entries: not a power of two, so amortized append lands on cap 16
	// (cap > len), while the pre-scan exact-fits cap == 9.
	const entries = 9
	src := &numericv1.MixedModifiers{}
	for i := 0; i < entries; i++ {
		src.RepeatedString = append(src.RepeatedString, strings.Repeat("x", 40))
	}
	b, err := src.Marshal()
	require.NoError(t, err)
	require.Greater(t, len(b), 256, "payload must exceed preScanMinBytes so the pre-scan would run")

	// Reference decode through the normal (pre-scan) path.
	var withPrescan numericv1.MixedModifiers
	require.NoError(t, withPrescan.Unmarshal(b))
	require.Equal(t, entries, cap(withPrescan.RepeatedString),
		"sanity: the normal Unmarshal pre-scan must exact-fit a fresh slice (cap == count)")

	// Decode through the no-prescan path.
	var noPrescan numericv1.MixedModifiers
	require.NoError(t, noPrescan.UnmarshalNoPrescan(b))

	// (a) deep-equal result.
	assert.True(t, noPrescan.Equal(&withPrescan),
		"UnmarshalNoPrescan must decode identically to Unmarshal")
	require.Len(t, noPrescan.RepeatedString, entries)

	// (b) no exact-fit prealloc: append-growth of 9 elements over-allocates.
	assert.Greater(t, cap(noPrescan.RepeatedString), len(noPrescan.RepeatedString),
		"UnmarshalNoPrescan must skip the top-level exact-fit prealloc; the slice should be append-grown")
}

// TestUnmarshalNoPrescan_PooledReuseStillSkipped pins the actual motivating
// case (DB-18): unmarshalling into a pooled message whose repeated slice was
// reset to [:0] with retained capacity. With normal Unmarshal the pre-scan's
// scan runs even though its prealloc guard (`len==0 && cap<count`) won't fire
// because cap already suffices — pure overhead. UnmarshalNoPrescan skips it and
// must still reuse the pooled backing array (decode correctness unchanged).
func TestUnmarshalNoPrescan_PooledReuseStillSkipped(t *testing.T) {
	const entries = 8
	src := &numericv1.MixedModifiers{}
	for i := 0; i < entries; i++ {
		src.RepeatedString = append(src.RepeatedString, strings.Repeat("y", 40))
	}
	b, err := src.Marshal()
	require.NoError(t, err)
	require.Greater(t, len(b), 256, "payload must exceed preScanMinBytes")

	dst := &numericv1.MixedModifiers{RepeatedString: make([]string, 0, 64)}
	pooled := unsafe.SliceData(dst.RepeatedString)
	require.NoError(t, dst.UnmarshalNoPrescan(b))

	require.Len(t, dst.RepeatedString, entries)
	assert.Same(t, pooled, unsafe.SliceData(dst.RepeatedString),
		"a pooled backing array with sufficient capacity must be reused")
}

// TestUnmarshalNoPrescan_NestedPreScansStillRun is the critical guarantee: the
// skip is TOP-LEVEL ONLY. UnmarshalNoPrescan on a ScopeSpans (repeated Span
// spans) payload must skip the top-level `spans` prealloc, but each nested
// Span's own pre-scan (repeated Event events) must still run and exact-prealloc
// its events slice. This is the property that keeps the tempo OTLP win
// (−2.9% time / −4.8% B/op from fresh nested-slice prealloc) intact.
func TestUnmarshalNoPrescan_NestedPreScansStillRun(t *testing.T) {
	// Each Span needs enough wire bytes to clear the 256-byte nested pre-scan
	// threshold AND enough events (count not a power of two) that exact-fit
	// vs append-growth is distinguishable by cap.
	const eventsPerSpan = 7 // not a power of two
	const spanCount = 4

	src := &tracev1.ScopeSpans{}
	for s := 0; s < spanCount; s++ {
		span := tracev1.Span{
			Name: strings.Repeat("s", 64), // pad the span past 256 bytes
		}
		for e := 0; e < eventsPerSpan; e++ {
			span.Events = append(span.Events, tracev1.Span_Event{
				Name: strings.Repeat("e", 40),
			})
		}
		src.Spans = append(src.Spans, span)
	}
	b, err := src.Marshal()
	require.NoError(t, err)
	require.Greater(t, len(b), 256, "ScopeSpans payload must exceed preScanMinBytes")

	// Confirm each span's marshaled size clears the nested pre-scan threshold,
	// otherwise the nested-prealloc assertion below would be vacuous.
	for i := range src.Spans {
		sb, err := src.Spans[i].Marshal()
		require.NoError(t, err)
		require.Greater(t, len(sb), 256, "span %d must exceed preScanMinBytes for its nested pre-scan to run", i)
	}

	var dst tracev1.ScopeSpans
	require.NoError(t, dst.UnmarshalNoPrescan(b))

	// Decode correctness.
	require.True(t, dst.Equal(src), "UnmarshalNoPrescan must decode ScopeSpans identically")
	require.Len(t, dst.Spans, spanCount)

	// Nested pre-scan preserved: each Span.events slice must be exact-fit
	// (cap == count) because the nested unmarshal advances depth from -1 to 0
	// and runs its own pre-scan. Append-growth of 7 elements would land on
	// cap 8 (cap > len), so cap == len == 7 proves the nested pre-scan fired.
	for i := range dst.Spans {
		require.Len(t, dst.Spans[i].Events, eventsPerSpan)
		assert.Equal(t, eventsPerSpan, cap(dst.Spans[i].Events),
			"span %d: nested pre-scan must exact-prealloc events (cap == count) even under UnmarshalNoPrescan", i)
	}
}
