package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mf "github.com/grafana/wiresmith/gen/basic/multifile/v1"
)

// Two .proto files generated into one Go package must coexist (no duplicate
// package-level helpers — they live in protohelpers now) and reference each
// other unqualified. The build of gen/basic/multifile is itself the main
// assertion; the round-trip exercises both files' Unmarshal paths.
func TestMultiFilePackage_RoundTrip(t *testing.T) {
	msg := &mf.BetaHolder{
		Entries: []mf.AlphaEntry{{Key: "a", Count: 1}, {Key: "b", Count: 2}},
		Note:    "spans two files",
	}
	b, err := msg.Marshal()
	require.NoError(t, err)

	dst := &mf.BetaHolder{}
	require.NoError(t, dst.Unmarshal(b))
	assert.True(t, msg.Equal(dst))
}

// Unknown fields route through protohelpers.SkipValue — decode a payload
// with an extra field to pin the shared skip path.
func TestMultiFilePackage_SkipsUnknownField(t *testing.T) {
	payload := append([]byte{0x12, 0x02, 'h', 'i'}, // field 2 (note)
		0xfa, 0x01, 0x03, 'x', 'y', 'z') // unknown field 31, length-delimited
	dst := &mf.BetaHolder{}
	require.NoError(t, dst.Unmarshal(payload))
	assert.Equal(t, "hi", dst.Note)
}
