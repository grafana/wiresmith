package basic

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	st "github.com/grafana/wiresmith/gen/basic/stdtime/v1"
)

// TestStdtime_FieldTypeIsTimeTime pins the struct field type: the stdtime-
// annotated `created` must surface as a stdlib `time.Time`, and the
// unannotated `name`/`version` scalars must stay their default Go shape.
// Together they cover the contract documented on the option: the
// substitution is field-local.
func TestStdtime_FieldTypeIsTimeTime(t *testing.T) {
	holderType := reflect.TypeFor[st.StdtimeHolder]()
	cases := []struct {
		field    string
		wantType string
	}{
		{"Name", "string"},
		{"Version", "uint64"},
		{"Created", "time.Time"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			f, ok := holderType.FieldByName(tc.field)
			require.True(t, ok, "field %q missing", tc.field)
			assert.Equal(t, tc.wantType, f.Type.String())
		})
	}
}

// TestStdtime_RoundTrip confirms a representative timestamp survives
// Marshal/Unmarshal/Marshal without losing data. UTC is the canonical zone
// per the option contract — decoded times are always in UTC even if the
// input was Local.
func TestStdtime_RoundTrip(t *testing.T) {
	src := &st.StdtimeHolder{
		Name:    "snapshot",
		Version: 42,
		Created: time.Date(2026, 6, 3, 12, 30, 45, 123456789, time.UTC),
	}
	roundTrip(t, src)
}

// TestStdtime_ZeroIsAbsent locks down the gogoproto-compatible presence
// rule: a Go-zero `time.Time{}` (year 1, the only value where IsZero
// returns true) marshals to zero bytes for the Created field. On the wire
// it's indistinguishable from an unset Timestamp, and the round-trip
// reproduces the same zero.
func TestStdtime_ZeroIsAbsent(t *testing.T) {
	src := &st.StdtimeHolder{Name: "no-time"}
	require.True(t, src.Created.IsZero())
	b, err := src.Marshal()
	require.NoError(t, err)

	// Field 3 (created) absent: only field 1 (name) bytes should appear.
	// 0x0a = field 1, length-delimited; 0x07 = length 7; then "no-time".
	want := []byte{0x0a, 0x07, 'n', 'o', '-', 't', 'i', 'm', 'e'}
	assert.Equal(t, want, b, "zero time.Time must not emit a Created tag")

	dst := &st.StdtimeHolder{}
	require.NoError(t, dst.Unmarshal(b))
	assert.True(t, dst.Created.IsZero(), "absent Timestamp must decode as zero time.Time")
}

// TestStdtime_EpochIsExplicit pins the distinction between two "looks
// empty" cases: the Unix epoch (1970-01-01 00:00:00 UTC) marshals as an
// explicit Timestamp envelope with an EMPTY payload (both seconds and
// nanos default to 0, suppressed under proto3 default rules). On decode,
// `time.Unix(0, 0).UTC()` is recovered — which is NOT a Go-zero time, so
// IsZero must return false.
func TestStdtime_EpochIsExplicit(t *testing.T) {
	src := &st.StdtimeHolder{Created: time.Unix(0, 0).UTC()}
	b, err := src.Marshal()
	require.NoError(t, err)
	// 0x1a (field 3, length-delim), 0x00 (length 0): explicit empty envelope.
	assert.Equal(t, []byte{0x1a, 0x00}, b)

	dst := &st.StdtimeHolder{}
	require.NoError(t, dst.Unmarshal(b))
	assert.False(t, dst.Created.IsZero(), "Unix epoch must decode as a non-zero time.Time")
	assert.True(t, dst.Created.Equal(time.Unix(0, 0).UTC()))
}

// TestStdtime_DecodeIsUTC pins the zone normalization: a Timestamp encoded
// from a Local-zoned time.Time must decode in UTC. UTC is the canonical
// protobuf timezone — peers using other libraries assume seconds/nanos are
// against UTC, and reading back as Local would silently shift the wall
// clock when downstream code formats with a TZ assumption.
func TestStdtime_DecodeIsUTC(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	src := &st.StdtimeHolder{
		Created: time.Date(2026, 6, 3, 21, 0, 0, 0, tokyo),
	}
	b, err := src.Marshal()
	require.NoError(t, err)

	dst := &st.StdtimeHolder{}
	require.NoError(t, dst.Unmarshal(b))
	assert.Equal(t, time.UTC, dst.Created.Location(), "decoded Timestamps must be in UTC")
	assert.True(t, dst.Created.Equal(src.Created), "instant must round-trip even though zone is normalised")
}

// TestStdtime_FarPastAndFuture exercises the int64-seconds boundary cases.
// year 1 (`time.Time{}`) is the IsZero-suppressed case (covered above); the
// next-smallest representable year (year 2 at January 1) does cross the wire.
// year 9999 is the largest protobuf-canonical Timestamp upper bound.
func TestStdtime_FarPastAndFuture(t *testing.T) {
	cases := []struct {
		name string
		t    time.Time
	}{
		{"year_2_january", time.Date(2, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"year_9999_december", time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC)},
		{"negative_unix", time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := &st.StdtimeHolder{Name: tc.name, Created: tc.t}
			b, err := src.Marshal()
			require.NoError(t, err)
			dst := &st.StdtimeHolder{}
			require.NoError(t, dst.Unmarshal(b))
			assert.True(t, dst.Created.Equal(tc.t), "instant must round-trip for %s", tc.name)
		})
	}
}

// TestStdtime_CrossLibraryWireFormat pins on-wire compatibility with the
// official google.golang.org/protobuf timestamppb. Both libraries must
// produce byte-identical encodings of the same instant — otherwise stdtime
// silently breaks interop with the rest of the protobuf ecosystem.
//
// We compare just the Timestamp envelope bytes (no outer field tag) by
// re-marshaling timestamppb.Timestamp and slicing off the corresponding
// section of the wiresmith output.
func TestStdtime_CrossLibraryWireFormat(t *testing.T) {
	instant := time.Date(2026, 6, 3, 12, 30, 45, 123456789, time.UTC)
	src := &st.StdtimeHolder{Created: instant}
	ours, err := src.Marshal()
	require.NoError(t, err)

	// Field 3 envelope: tag byte 0x1a, then a varint length, then the
	// payload. Locate the payload window via the length prefix.
	require.GreaterOrEqual(t, len(ours), 2)
	require.Equal(t, byte(0x1a), ours[0])
	payloadLen := int(ours[1])
	require.Equal(t, len(ours), 2+payloadLen, "exactly one field, no trailing bytes")

	// Official timestamppb.Marshal of the same instant.
	wantPayload, err := proto.Marshal(timestamppb.New(instant))
	require.NoError(t, err)
	assert.Equal(t, wantPayload, ours[2:], "wire payload must match google.golang.org/protobuf")
}

// TestStdtime_Equal pins that the generated Equal compares by instant — two
// times that refer to the same wall-clock moment but were constructed via
// different paths still compare equal. Equal returning true is required for
// `roundTrip`'s Equal-vs-assert.Equal consistency check too.
func TestStdtime_Equal(t *testing.T) {
	a := &st.StdtimeHolder{Created: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	// time.Unix(1767225600, 0) is January 1, 2026 UTC — same instant as
	// `a`, constructed via a different path so Equal must agree.
	b := &st.StdtimeHolder{Created: time.Unix(1767225600, 0).UTC()}
	assert.True(t, a.Equal(b))

	c := &st.StdtimeHolder{Created: time.Date(2026, 1, 1, 0, 0, 0, 1, time.UTC)}
	assert.False(t, a.Equal(c), "1ns apart must compare unequal")
}

// TestStdtime_Compare pins -1/0/+1 against the chronological order. The
// generated Compare uses `time.Time.Compare`, which returns -1 for earlier,
// 0 for equal-instant, and +1 for later.
func TestStdtime_Compare(t *testing.T) {
	earlier := &st.StdtimeHolder{Created: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	later := &st.StdtimeHolder{Created: time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)}
	assert.Equal(t, -1, earlier.Compare(later))
	assert.Equal(t, 1, later.Compare(earlier))
	assert.Equal(t, 0, earlier.Compare(earlier))
}

// TestStdtime_GetterOnNilReceiver pins the nil-safety contract — the
// generated GetCreated must not panic on a nil receiver. Returns the Go-
// zero time, matching the customtype getter shape and the rest of the
// nil-safety contract documented on CLAUDE.md.
func TestStdtime_GetterOnNilReceiver(t *testing.T) {
	var m *st.StdtimeHolder
	assert.True(t, m.GetCreated().IsZero())
	assert.Empty(t, m.GetName())
	assert.Zero(t, m.GetVersion())
}
