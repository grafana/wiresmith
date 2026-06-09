package basic

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"

	ct "github.com/grafana/wiresmith/gen/basic/casttype/v1"
	"github.com/grafana/wiresmith/test/casttypes"
)

// TestCastType_FieldTypesSwapped pins the struct field types: each
// annotated field surfaces as the user-supplied alias, while the unannotated
// plain controls keep their default Go shape. Together they exercise the
// "substitution is field-local" contract documented on the option.
func TestCastType_FieldTypesSwapped(t *testing.T) {
	holderType := reflect.TypeFor[ct.CastTypeHolder]()
	cases := []struct {
		field    string
		wantType string
	}{
		{"UserId", "casttypes.UserID"},
		{"TenantTag", "casttypes.TenantTag"},
		{"Payload", "casttypes.Payload"},
		{"PlainId", "int64"},
		{"PlainTag", "string"},
		{"PlainBytes", "[]uint8"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			f, ok := holderType.FieldByName(tc.field)
			require.True(t, ok, "field %q missing", tc.field)
			assert.Equal(t, tc.wantType, f.Type.String())
		})
	}
}

// TestCastType_RoundTrip pins that a representative payload survives
// Marshal/Unmarshal/Marshal without losing data. Confirms the casts at the
// marshal-time uint64() / size-time len() / unmarshal-time alias() sites
// bridge cleanly between the user types and the underlying scalars.
func TestCastType_RoundTrip(t *testing.T) {
	src := &ct.CastTypeHolder{
		UserId:     casttypes.UserID(42),
		TenantTag:  casttypes.TenantTag("acme-corp"),
		Payload:    casttypes.Payload{0xde, 0xad, 0xbe, 0xef},
		PlainId:    -7,
		PlainTag:   "plain",
		PlainBytes: []byte{0x01, 0x02},
	}
	roundTrip(t, src)
}

// TestCastType_ZeroValues pins that the alias zero values follow the same
// proto3 default-suppression rules the underlying scalar would: zero ints
// emit no bytes, empty strings emit no bytes, nil/empty []byte emit no
// bytes. Together with TestCastType_FieldTypesSwapped this proves the
// substitution doesn't change the wire envelope.
func TestCastType_ZeroValues(t *testing.T) {
	src := &ct.CastTypeHolder{}
	b, err := src.Marshal()
	require.NoError(t, err)
	assert.Empty(t, b, "all-default holder must produce no wire bytes")

	dst := &ct.CastTypeHolder{}
	require.NoError(t, dst.Unmarshal(b))
	assert.Equal(t, casttypes.UserID(0), dst.UserId)
	assert.Equal(t, casttypes.TenantTag(""), dst.TenantTag)
	assert.Empty(t, dst.Payload)
}

// TestCastType_WireCompatWithControl confirms the wire bytes a casttype
// field produces are byte-identical to the bytes the same proto kind
// produces without the option. The two fields differ only in their wire
// tag (field number 1 vs 4 for int64, 2 vs 5 for string, 3 vs 6 for bytes)
// — strip the tag and the rest must match.
func TestCastType_WireCompatWithControl(t *testing.T) {
	cases := []struct {
		name     string
		setCast  func(m *ct.CastTypeHolder)
		setPlain func(m *ct.CastTypeHolder)
	}{
		{
			"int64",
			func(m *ct.CastTypeHolder) { m.UserId = casttypes.UserID(0x1234567890ABCDEF) },
			func(m *ct.CastTypeHolder) { m.PlainId = 0x1234567890ABCDEF },
		},
		{
			"string",
			func(m *ct.CastTypeHolder) { m.TenantTag = casttypes.TenantTag("hello") },
			func(m *ct.CastTypeHolder) { m.PlainTag = "hello" },
		},
		{
			"bytes",
			func(m *ct.CastTypeHolder) { m.Payload = casttypes.Payload{0xde, 0xad} },
			func(m *ct.CastTypeHolder) { m.PlainBytes = []byte{0xde, 0xad} },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &ct.CastTypeHolder{}
			tc.setCast(a)
			bA, err := a.Marshal()
			require.NoError(t, err)

			b := &ct.CastTypeHolder{}
			tc.setPlain(b)
			bB, err := b.Marshal()
			require.NoError(t, err)

			// Both encodings start with the field tag, then identical payload
			// shape (varint or length-prefixed). Strip the leading tag byte;
			// the rest must match exactly — proving the casttype path emits
			// the same bytes as the un-aliased control would.
			require.Len(t, bA, len(bB), "%s: casttype and control wire byte counts must match", tc.name)
			assert.Equal(t, bA[1:], bB[1:], "%s: bytes after the wire tag must be identical", tc.name)
		})
	}
}

// TestCastType_NegativeInt pins that signed casttype values round-trip
// correctly through the unsigned-varint encoding. proto3 int64 stores
// negatives as the 64-bit two's-complement reinterpreted as uint64 (giving
// a 10-byte varint), and the alias cast preserves the sign through the
// round trip.
func TestCastType_NegativeInt(t *testing.T) {
	src := &ct.CastTypeHolder{UserId: casttypes.UserID(-12345)}
	b, err := src.Marshal()
	require.NoError(t, err)

	// The first byte is the field tag (0x08 = field 1, varint).
	require.Equal(t, byte(0x08), b[0])
	// The remaining bytes are the 10-byte varint for a negative int64.
	v, n := protowire.ConsumeVarint(b[1:])
	require.Equal(t, 10, n, "negative int64 must encode as a 10-byte varint")
	assert.Equal(t, uint64(-12345&0xFFFFFFFFFFFFFFFF), v)

	dst := &ct.CastTypeHolder{}
	require.NoError(t, dst.Unmarshal(b))
	assert.Equal(t, casttypes.UserID(-12345), dst.UserId)
}

// TestCastType_Equal pins the generated Equal contract across all three
// alias kinds. Integer / string compare via Go's `==`; bytes compare via
// bytes.Equal with an explicit `[]byte` cast.
func TestCastType_Equal(t *testing.T) {
	a := &ct.CastTypeHolder{
		UserId:    casttypes.UserID(1),
		TenantTag: casttypes.TenantTag("acme"),
		Payload:   casttypes.Payload{0xde, 0xad},
	}
	b := &ct.CastTypeHolder{
		UserId:    casttypes.UserID(1),
		TenantTag: casttypes.TenantTag("acme"),
		Payload:   casttypes.Payload{0xde, 0xad},
	}
	assert.True(t, a.Equal(b), "byte-identical alias values must compare equal")

	// Per-field inequality drift detection.
	c := *a
	c.UserId = 2
	assert.False(t, a.Equal(&c), "alias int mismatch must compare unequal")

	d := *a
	d.TenantTag = "other"
	assert.False(t, a.Equal(&d), "alias string mismatch must compare unequal")

	e := *a
	e.Payload = casttypes.Payload{0xff}
	assert.False(t, a.Equal(&e), "alias bytes mismatch must compare unequal")
}

// TestCastType_Compare pins -1/0/+1 across the alias kinds. Ordering follows
// the underlying scalar contract: numeric for int, lexicographic for string,
// byte-wise for bytes.
func TestCastType_Compare(t *testing.T) {
	a := &ct.CastTypeHolder{UserId: 1, TenantTag: "a", Payload: casttypes.Payload{0x00}}
	b := &ct.CastTypeHolder{UserId: 2, TenantTag: "a", Payload: casttypes.Payload{0x00}}
	c := &ct.CastTypeHolder{UserId: 1, TenantTag: "b", Payload: casttypes.Payload{0x00}}
	d := &ct.CastTypeHolder{UserId: 1, TenantTag: "a", Payload: casttypes.Payload{0x01}}

	assert.Equal(t, -1, a.Compare(b), "smaller int compares less")
	assert.Equal(t, 1, b.Compare(a), "larger int compares greater")
	assert.Equal(t, -1, a.Compare(c), "lexicographic string ordering on alias")
	assert.Equal(t, -1, a.Compare(d), "byte-wise ordering on alias bytes")
	assert.Equal(t, 0, a.Compare(a), "identical alias values compare equal")
}

// TestCastType_GetterOnNilReceiver pins the nil-safety contract for each
// alias getter. The generated getters use `var zero T; return zero` because
// the alias's zero literal is unknown to the generator.
func TestCastType_GetterOnNilReceiver(t *testing.T) {
	var m *ct.CastTypeHolder
	assert.Equal(t, casttypes.UserID(0), m.GetUserId())
	assert.Equal(t, casttypes.TenantTag(""), m.GetTenantTag())
	assert.Nil(t, m.GetPayload())
}
