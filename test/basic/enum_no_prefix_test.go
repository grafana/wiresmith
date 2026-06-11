package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	enp "github.com/grafana/wiresmith/gen/basic/enumnopfx/v1"
)

// The annotated enum's constants are bare identifiers; the control enum in
// the same file keeps the prefix. Compilation of this file is most of the
// assertion.
func TestEnumNoPrefix_ConstantsAndRoundTrip(t *testing.T) {
	assert.Equal(t, enp.MetricType(0), enp.UNKNOWN)
	assert.Equal(t, enp.MetricType(1), enp.COUNTER)
	assert.Equal(t, enp.PrefixedColor(1), enp.PrefixedColor_PREFIXED_COLOR_BLUE)

	// String() and the name/value maps use bare proto names regardless of
	// the constant identifier shape.
	assert.Equal(t, "COUNTER", enp.COUNTER.String())
	assert.Equal(t, int32(2), enp.MetricType_value["GAUGE"])

	msg := &enp.MetricInfo{Type: enp.GAUGE, Name: "x"}
	b, err := msg.Marshal()
	require.NoError(t, err)
	dst := &enp.MetricInfo{}
	require.NoError(t, dst.Unmarshal(b))
	assert.Equal(t, enp.GAUGE, dst.Type)
}
