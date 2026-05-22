package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	numericv1 "wiresmith/gen/basic/numeric/v1"
)

// TestPreScanAbortsOnUnknownWireType is a regression test for SEC-2
// (wiresmith-51b). The pre-scan inner switch on `preTyp` used `default: break`,
// which only exits the switch — not the surrounding for-loop. When the payload
// contained a tag with an unknown wire type (3, 4, 6, or 7), the pre-scan
// failed to advance past the payload bytes, then re-read those payload bytes
// as more tags on subsequent iterations. The result: an attacker could inflate
// the per-field occurrence counter (and the downstream slice pre-allocation)
// by an unbounded amount with one wire-type-7 tag and a tail of bytes that
// happen to look like the tracked field's tag.
//
// The fix aborts the pre-scan loop when an unknown wire type is encountered.
// The main unmarshal loop is the source of truth — the pre-scan is only an
// allocation optimisation, so it is safe to bail.
func TestPreScanAbortsOnUnknownWireType(t *testing.T) {
	// Build a >= 256-byte payload (the pre-scan threshold). The body uses
	// `MixedModifiers` because it has both a pre-scan-tracked field
	// (field 9 = repeated_string, wire type 2) and a field number we can
	// safely treat as unknown filler (field 15).

	// Filler tag: field 15, wire type 2 (length-delimited). The pre-scan
	// handles wire type 2 correctly (case 2 in switch preTyp), so this
	// section is navigated cleanly. The main loop also skips it via
	// skipValue. Use 204 bytes of zero payload so the entire filler block
	// (1-byte tag + 2-byte length + 204-byte body = 207 bytes) consumes
	// well over half the threshold.
	const fillerLen = 204
	payload := []byte{0x7A, 0xCC, 0x01} // tag=0x7A (field 15 wt 2), length=204
	payload = append(payload, make([]byte, fillerLen)...)

	// Attack: a run of 0x4F bytes. Each 0x4F decodes as tag (field 9, wire
	// type 7). On the buggy generator:
	//   - switch preNum: case 9 -> field9count++
	//   - switch preTyp: default -> bogus `break` (exits switch only)
	//   - bounds check on preIdx passes -> outer loop continues
	//   - next iteration reads the *next* 0x4F as another tag
	// So N attack bytes inflate field9count by N, which then becomes the
	// pre-allocated capacity of m.RepeatedString.
	const attackBytes = 100
	for range attackBytes {
		payload = append(payload, 0x4F)
	}
	require.GreaterOrEqual(t, len(payload), 256, "payload must exceed preScanMinBytes")

	var m numericv1.MixedModifiers
	err := m.Unmarshal(payload)
	// The main loop rejects wire type 7 via skipValue, so an error is
	// expected. The slice was already pre-allocated by the pre-scan; we
	// inspect its capacity to detect SEC-2 amplification.
	require.Error(t, err, "main loop must reject wire type 7")

	// With the fix: cap(m.RepeatedString) is exactly 1 (the first 0x4F
	// increments field9count once before the abort fires).
	// Without the fix: cap == attackBytes (100), which would fail this
	// assertion.
	assert.LessOrEqual(t, cap(m.RepeatedString), 1,
		"pre-scan must abort on unknown wire type; cap inflated by SEC-2 amplification")
}

// TestPreScanCapBoundedByPayload is an end-to-end check for SEC-1
// (wiresmith-bmp). The bound it asserts — `cap(slice) ≤ len(payload)/2`
// — is also satisfied by a *well-formed* payload of length-delimited
// entries regardless of whether the generator emits the cap, so this
// test alone cannot catch a regression that drops the cap. The actual
// regression guard lives in `compiler/generator/prescan_cap_test.go`
// (TestPreScanEmitsCapClamp), which inspects the generated source for
// the cap pattern; this test exercises the runtime behaviour so a
// non-functional regression (e.g. emitted code that doesn't compile or
// produces wrong counts) is caught too.
//
// The pre-scan counts wire-format occurrences of any repeated
// length-delimited field (string, bytes, message, map entry) and uses
// the count directly as slice capacity (`make([]T, 0, count)`).
// The amplification potential scales with the size of the Go element
// type: for a repeated-message field over a large value-type struct
// (e.g. OTel `Span` ≈ 250 B), a payload packed with 2-byte zero-length
// entries achieves ~payload/2 occurrences, so capacity allocation is
// ~payload × elementSize/2 — a 1MB payload requesting 125MB of memory.
// Combined with SEC-2 the count itself can run unbounded.
//
// This test uses a repeated *string* field (`MixedModifiers.repeated_string`,
// field 9) as the test vehicle because the bound applies uniformly to
// every pre-scan-tracked element type; the string element happens to be
// the smallest Go type the pre-scan handles and is convenient to drive
// with a synthetic payload. The asserted invariant — `cap(slice) ≤
// len(payload)/2` — is the same one that bounds the worst-case allocation
// for the large-struct message case.
//
// The fix caps the pre-allocated capacity at len(payload)/2: every
// length-delimited element consumes at least 2 bytes on the wire (tag
// varint ≥1 byte plus length varint ≥1 byte for length 0), so no
// compliant payload can produce more than len/2 elements. The cap is
// defense-in-depth — it makes the bound explicit in the generated code
// even if upstream amplification regressed.
func TestPreScanCapBoundedByPayload(t *testing.T) {
	// 1KB of `0x4A 0x00` repeats: tag for field 9 (repeated_string) wire
	// type 2, length 0. 512 entries, all empty strings.
	const entries = 512
	payload := make([]byte, 0, entries*2)
	for range entries {
		payload = append(payload, 0x4A, 0x00)
	}
	require.GreaterOrEqual(t, len(payload), 256, "payload must exceed preScanMinBytes")

	var m numericv1.MixedModifiers
	require.NoError(t, m.Unmarshal(payload))

	// Sanity: the message did decode into entries.
	require.Equal(t, entries, len(m.RepeatedString))

	// SEC-1 invariant: pre-allocated capacity is bounded by len/2.
	// A 1MB payload of large-struct elements would otherwise allocate
	// hundreds of MB of capacity.
	assert.LessOrEqual(t, cap(m.RepeatedString), len(payload)/2,
		"SEC-1: pre-scan capacity must be bounded by payload/2")
}

// TestPreScanAmplificationThroughGroupTag confirms the abort fires for every
// wire type in the default branch of the pre-scan switch (3, 4, 6, 7). Wire
// type 3 is particularly insidious because the main loop *does* handle it
// (via protowire.ConsumeGroup), so unmarshal could succeed while pre-scan
// allocated an attacker-controlled capacity.
func TestPreScanAmplificationThroughGroupTag(t *testing.T) {
	for _, wireType := range []byte{3, 4, 6, 7} {
		t.Run("wireType="+string('0'+wireType), func(t *testing.T) {
			// Tag byte for field 9 with the chosen wire type.
			attackTag := byte(9<<3) | wireType

			// Pre-scan filler as in the previous test.
			payload := []byte{0x7A, 0xCC, 0x01}
			payload = append(payload, make([]byte, 204)...)

			const attackBytes = 100
			for range attackBytes {
				payload = append(payload, attackTag)
			}

			var m numericv1.MixedModifiers
			_ = m.Unmarshal(payload)

			// Regardless of whether Unmarshal returned an error, the
			// pre-scan must have stopped at the first unknown-wire-type
			// tag rather than iterating across all 100 attack bytes.
			// With the fix: count is incremented exactly once before the
			// default branch fires, so cap is at most 1.
			assert.LessOrEqual(t, cap(m.RepeatedString), 1,
				"pre-scan must abort at first wire type %d; observed inflated cap", wireType)
		})
	}
}
