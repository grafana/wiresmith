package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	em "github.com/grafana/wiresmith/gen/basic/emptymsg/v1"
)

// TestEmptyMessageRoundTrip exercises the field-less-message path end to end:
// the emptymsg package compiling at all proves the main .pb.go was emitted
// without the unused protowire import (an unused import fails to compile). The
// round-trips confirm the field-less Marshal/Unmarshal/Equal bodies work.
func TestEmptyMessageRoundTrip(t *testing.T) {
	e := &em.Empty{}
	b, err := e.Marshal()
	require.NoError(t, err)
	assert.Empty(t, b)

	gotE := &em.Empty{}
	require.NoError(t, gotE.Unmarshal(b))
	assert.True(t, gotE.Equal(e))

	a := &em.AlsoEmpty{}
	ab, err := a.Marshal()
	require.NoError(t, err)
	assert.Empty(t, ab)

	gotA := &em.AlsoEmpty{}
	require.NoError(t, gotA.Unmarshal(ab))
	assert.True(t, gotA.Equal(a))
}
