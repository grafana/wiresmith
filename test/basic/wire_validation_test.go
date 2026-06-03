package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grafana/wiresmith/test/testutil"
)

// SEC: Field number 0 must be rejected by every generated Unmarshal. The
// inline tag decode in `compiler/types/type.go` emits
// `if X>>3 < 1 || X>>3 > 0x1FFFFFFF { return fmt.Errorf("invalid field number") }`
// and CLAUDE.md "Wire format safety in unmarshal" lists this as a recurring
// caveat: a regression would let a `[]byte{0x00, ...}` payload silently
// dispatch on field 0 (an invalid protobuf field number).
//
// This test feeds the smallest payload that exercises the validation
// (`0x00` = tag varint = (field 0 << 3) | wire type 0) through every
// generated message constructor and asserts the documented error surface.
// AllPanicSafeConstructors covers both map-free and map-bearing types —
// the test only checks the returned error, so map randomization is
// irrelevant.
func TestUnmarshalRejectsFieldZero(t *testing.T) {
	for name, ctor := range testutil.AllPanicSafeConstructors() {
		t.Run(name, func(t *testing.T) {
			msg := ctor()
			err := msg.Unmarshal([]byte{0x00})
			if assert.Error(t, err, "expected error for field-0 tag") {
				assert.Contains(t, err.Error(), "invalid field number")
			}
		})
	}
}
