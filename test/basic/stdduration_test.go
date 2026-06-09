package basic

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	st "github.com/grafana/wiresmith/gen/basic/stdtime/v1"
)

// TestStdDuration_FieldTypeIsTimeDuration pins the struct field type: the
// stdduration-annotated `lookback` must surface as a stdlib `time.Duration`,
// and the unannotated `name`/`retries` scalars must stay their default Go
// shape. Together they cover the contract documented on the option: the
// substitution is field-local.
func TestStdDuration_FieldTypeIsTimeDuration(t *testing.T) {
	holderType := reflect.TypeFor[st.StdDurationHolder]()
	cases := []struct {
		field    string
		wantType string
	}{
		{"Name", "string"},
		{"Retries", "uint32"},
		{"Lookback", "time.Duration"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			f, ok := holderType.FieldByName(tc.field)
			require.True(t, ok, "field %q missing", tc.field)
			assert.Equal(t, tc.wantType, f.Type.String())
		})
	}
}

// TestStdDuration_RoundTrip confirms a representative duration survives
// Marshal/Unmarshal/Marshal without losing data.
func TestStdDuration_RoundTrip(t *testing.T) {
	src := &st.StdDurationHolder{
		Name:     "query-lookback",
		Retries:  3,
		Lookback: 5*time.Hour + 23*time.Minute + 17*time.Second + 123456789*time.Nanosecond,
	}
	roundTrip(t, src)
}

// TestStdDuration_ZeroIsAbsent locks down the gogoproto-compatible presence
// rule: `time.Duration(0)` marshals to zero bytes for the Lookback field.
// On the wire it's indistinguishable from an unset Duration, and the round-
// trip reproduces the same zero.
func TestStdDuration_ZeroIsAbsent(t *testing.T) {
	src := &st.StdDurationHolder{Name: "no-duration"}
	require.Equal(t, time.Duration(0), src.Lookback)
	b, err := src.Marshal()
	require.NoError(t, err)

	// Field 3 (lookback) absent: only field 1 (name) bytes should appear.
	// 0x0a = field 1, length-delimited; 0x0b = length 11; then "no-duration".
	want := []byte{0x0a, 0x0b, 'n', 'o', '-', 'd', 'u', 'r', 'a', 't', 'i', 'o', 'n'}
	assert.Equal(t, want, b, "zero time.Duration must not emit a Lookback tag")

	dst := &st.StdDurationHolder{}
	require.NoError(t, dst.Unmarshal(b))
	assert.Equal(t, time.Duration(0), dst.Lookback, "absent Duration must decode as zero")
}

// TestStdDuration_Negative pins that negative durations round-trip
// correctly. proto Duration has signed seconds and signed nanos with
// matching sign — integer division on Go's int64-backed time.Duration
// preserves that invariant naturally.
func TestStdDuration_Negative(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
	}{
		{"neg_seconds", -42 * time.Second},
		{"neg_seconds_and_nanos", -(2*time.Hour + 30*time.Minute + 1*time.Nanosecond)},
		{"neg_nanos_only", -123456789 * time.Nanosecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := &st.StdDurationHolder{Lookback: tc.d}
			b, err := src.Marshal()
			require.NoError(t, err)
			dst := &st.StdDurationHolder{}
			require.NoError(t, dst.Unmarshal(b))
			assert.Equal(t, tc.d, dst.Lookback)
		})
	}
}

// TestStdDuration_TruncationBoundary pins the saturation contract on the
// upper edge of time.Duration. A protobuf Duration encoding more seconds
// than `time.Duration` can hold (~292 years max) must clamp to MaxInt64
// rather than wrap silently — matches (*durationpb.Duration).AsDuration().
//
// The encoded payload is built by hand because we cannot construct a Go
// time.Duration that overflows; the helper layer turns those wire bytes
// into the saturated value.
func TestStdDuration_TruncationBoundary(t *testing.T) {
	// Build a wire payload representing a Duration with seconds = 1<<62
	// (~146 trillion seconds, far past the ~9.22e9 seconds that fit in
	// int64-nanos). Then decode via Unmarshal and assert saturation.
	buf := []byte{0x1a} // field 3, length-delim
	inner := []byte{0x08}
	inner = protowire.AppendVarint(inner, 1<<62)
	buf = protowire.AppendVarint(buf, uint64(len(inner)))
	buf = append(buf, inner...)

	dst := &st.StdDurationHolder{}
	require.NoError(t, dst.Unmarshal(buf))
	assert.Equal(t, time.Duration(math.MaxInt64), dst.Lookback,
		"overflow must saturate to MaxInt64 nanoseconds")
}

// TestStdDuration_CrossLibraryWireFormat pins on-wire compatibility with the
// official google.golang.org/protobuf durationpb. Both libraries must
// produce byte-identical encodings of the same duration — otherwise
// stdduration silently breaks interop with the rest of the protobuf
// ecosystem.
//
// We compare the Duration envelope bytes by re-marshaling durationpb.Duration
// and slicing off the corresponding section of the wiresmith output.
func TestStdDuration_CrossLibraryWireFormat(t *testing.T) {
	d := 5*time.Hour + 23*time.Minute + 17*time.Second + 123456789*time.Nanosecond
	src := &st.StdDurationHolder{Lookback: d}
	ours, err := src.Marshal()
	require.NoError(t, err)

	// Field 3 envelope: tag byte 0x1a, then a varint length, then the
	// payload. Decode the length as a proper varint.
	require.GreaterOrEqual(t, len(ours), 2)
	require.Equal(t, byte(0x1a), ours[0])
	payloadLen, n := protowire.ConsumeVarint(ours[1:])
	require.Greater(t, n, 0, "malformed length varint at ours[1:]")
	payloadStart := 1 + n
	require.Equal(t, len(ours), payloadStart+int(payloadLen), "exactly one field, no trailing bytes")

	wantPayload, err := proto.Marshal(durationpb.New(d))
	require.NoError(t, err)
	assert.Equal(t, wantPayload, ours[payloadStart:], "wire payload must match google.golang.org/protobuf")
}

// TestStdDuration_Equal pins that the generated Equal compares by value —
// two durations representing the same nanosecond count compare equal.
func TestStdDuration_Equal(t *testing.T) {
	a := &st.StdDurationHolder{Lookback: 90 * time.Second}
	b := &st.StdDurationHolder{Lookback: time.Minute + 30*time.Second}
	assert.True(t, a.Equal(b), "1m30s and 90s must compare equal")

	c := &st.StdDurationHolder{Lookback: 90*time.Second + 1*time.Nanosecond}
	assert.False(t, a.Equal(c), "1ns apart must compare unequal")
}

// TestStdDuration_Compare pins -1/0/+1 against numeric ordering. The
// generated Compare uses a plain numeric comparison on the int64-backed
// time.Duration, which is consistent with the rest of wiresmith's Compare
// contract.
func TestStdDuration_Compare(t *testing.T) {
	shorter := &st.StdDurationHolder{Lookback: 10 * time.Second}
	longer := &st.StdDurationHolder{Lookback: time.Hour}
	assert.Equal(t, -1, shorter.Compare(longer))
	assert.Equal(t, 1, longer.Compare(shorter))
	assert.Equal(t, 0, shorter.Compare(shorter))
}

// TestStdDuration_GetterOnNilReceiver pins the nil-safety contract — the
// generated GetLookback must not panic on a nil receiver and returns the
// Go-zero duration. Matches the rest of the nil-safety contract documented
// in CLAUDE.md.
func TestStdDuration_GetterOnNilReceiver(t *testing.T) {
	var m *st.StdDurationHolder
	assert.Equal(t, time.Duration(0), m.GetLookback())
	assert.Empty(t, m.GetName())
	assert.Zero(t, m.GetRetries())
}
