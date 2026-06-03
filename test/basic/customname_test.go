package basic

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cn "github.com/grafana/wiresmith/gen/basic/customname/v1"
)

// TestCustomName_StructFieldNames pins the Go-side field identifiers. The
// customname-annotated fields must surface the user's exact spelling
// (initialisms preserved); the unannotated control field must keep the
// default snake_to_PascalCase conversion.
func TestCustomName_StructFieldNames(t *testing.T) {
	holderType := reflect.TypeFor[cn.CustomNameHolder]()
	for _, name := range []string{"BlockID", "HeadLeaf", "ByteSizes", "PlainField"} {
		t.Run(name, func(t *testing.T) {
			_, ok := holderType.FieldByName(name)
			assert.True(t, ok, "field %s missing — customname override not applied?", name)
		})
	}
	// Initialism mangling regression guard — `BlockId` (default conversion)
	// must NOT exist on a holder annotated with `BlockID`.
	_, hasMangled := holderType.FieldByName("BlockId")
	assert.False(t, hasMangled, "default-converted BlockId leaked through despite customname")
}

// TestCustomName_OneofVariantField verifies the renaming reaches inside oneof
// wrapper structs. The wrapper TYPE name (CustomNameHolder_TenantId) stays
// anchored to the proto field name; the FIELD inside the wrapper uses the
// customname.
func TestCustomName_OneofVariantField(t *testing.T) {
	variantType := reflect.TypeFor[cn.CustomNameHolder_TenantId]()
	_, ok := variantType.FieldByName("Tenant")
	require.True(t, ok, "wrapper struct must expose `Tenant` (customname), got fields: %v", structFieldNames(variantType))
	_, hasOriginal := variantType.FieldByName("TenantId")
	assert.False(t, hasOriginal, "wrapper struct must NOT keep the default `TenantId` alongside the customname")
}

func structFieldNames(t reflect.Type) []string {
	out := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		out = append(out, t.Field(i).Name)
	}
	return out
}

// TestCustomName_AccessorMethods pins that the Get/Has methods use the
// customname identifier, not the proto field name. This is the bead's
// "Test that emitted Go API matches `customname` value exactly across
// Get*, Has*" requirement.
func TestCustomName_AccessorMethods(t *testing.T) {
	holderType := reflect.TypeFor[*cn.CustomNameHolder]()
	for _, method := range []string{"GetBlockID", "HasBlockID", "GetHeadLeaf", "HasHeadLeaf", "GetByteSizes", "GetTenant", "GetNumericId"} {
		t.Run(method, func(t *testing.T) {
			_, ok := holderType.MethodByName(method)
			assert.True(t, ok, "method %s missing — customname override not applied to accessors?", method)
		})
	}
}

// TestCustomName_RoundTrip confirms the wire format is unchanged — every
// rename is a Go-side rebrand, and the marshal/unmarshal/equal paths
// continue to function with the customname identifiers.
func TestCustomName_RoundTrip(t *testing.T) {
	msg := &cn.CustomNameHolder{
		BlockID:    "01HX1Y2Z3...",
		HeadLeaf:   cn.Leaf{Id: 1, Name: "leaf"},
		ByteSizes:  []int32{10, 20, 30},
		Identity:   &cn.CustomNameHolder_TenantId{Tenant: "tenant-abc"},
		PlainField: "plain",
	}
	roundTrip(t, msg)
}

// TestCustomName_GetterOnNilReceiver pins the nil-safety contract — the
// renamed Get*() methods must not panic on a nil receiver.
func TestCustomName_GetterOnNilReceiver(t *testing.T) {
	var m *cn.CustomNameHolder
	assert.Empty(t, m.GetBlockID())
	assert.Nil(t, m.GetHeadLeaf())
	assert.Nil(t, m.GetByteSizes())
	assert.Empty(t, m.GetTenant())
	assert.Empty(t, m.GetPlainField())
}
