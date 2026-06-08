// Package customtypes provides the user-defined Go types referenced by
// proto/basic/customtype.proto via (wiresmith.options.customtype). The types
// satisfy the wiresmith reverse-write CustomMarshaler shape:
//
//	SizeWiresmith() int
//	MarshalWiresmith(buf []byte) (int, error)
//	UnmarshalWiresmith(buf []byte) error
//	EqualWiresmith(other any) bool
//	CompareWiresmith(other any) int    // -1/0/+1 like bytes.Compare
//
// The test fixture exercises both bytes and string field kinds; the types
// below are deliberately trivial wrappers that just round-trip the underlying
// representation so the test can verify the option's wiring, not the user
// code.
package customtypes

import (
	"bytes"
	"fmt"
	"strings"
)

// LabelPairs is a wrapper around []byte that stands in for a typical
// gogoproto `customtype = "...LabelAdapter"` use case. Marshal/Unmarshal are
// the identity transform on the payload — the generator owns the
// length-delimited proto envelope, so this type just shuffles raw bytes.
type LabelPairs struct {
	Pairs []byte
}

func (l LabelPairs) SizeWiresmith() int { return len(l.Pairs) }

func (l LabelPairs) MarshalWiresmith(buf []byte) (int, error) {
	if len(buf) < len(l.Pairs) {
		return 0, fmt.Errorf("buf too small: have %d, want %d", len(buf), len(l.Pairs))
	}
	return copy(buf, l.Pairs), nil
}

func (l *LabelPairs) UnmarshalWiresmith(buf []byte) error {
	l.Pairs = append(l.Pairs[:0], buf...)
	return nil
}

func (l LabelPairs) EqualWiresmith(other any) bool {
	o, ok := other.(LabelPairs)
	if !ok {
		return false
	}
	return bytes.Equal(l.Pairs, o.Pairs)
}

func (l LabelPairs) CompareWiresmith(other any) int {
	o, ok := other.(LabelPairs)
	if !ok {
		// A wrong-type comparison ranks unknown before known so the
		// generator's Compare-the-wrapper path stays total.
		return -1
	}
	return bytes.Compare(l.Pairs, o.Pairs)
}

// TenantID is a string-backed customtype demonstrating the option on a
// proto3 `string` field. Identical wire shape to a plain `string`; the
// distinct Go type lets the caller attach methods (validation, formatting)
// that bare strings can't carry.
type TenantID string

func (t TenantID) SizeWiresmith() int { return len(t) }

func (t TenantID) MarshalWiresmith(buf []byte) (int, error) {
	if len(buf) < len(t) {
		return 0, fmt.Errorf("buf too small: have %d, want %d", len(buf), len(t))
	}
	return copy(buf, t), nil
}

func (t *TenantID) UnmarshalWiresmith(buf []byte) error {
	*t = TenantID(buf)
	return nil
}

func (t TenantID) EqualWiresmith(other any) bool {
	o, ok := other.(TenantID)
	if !ok {
		return false
	}
	return t == o
}

func (t TenantID) CompareWiresmith(other any) int {
	o, ok := other.(TenantID)
	if !ok {
		return -1
	}
	return strings.Compare(string(t), string(o))
}

// UUID is a fixed-size [16]byte customtype intended for repeated `bytes`
// fields — the canonical "I want a strongly-typed slice element" case.
// The wire payload is always the raw 16 bytes: a repeated customtype
// element appears on the wire as `tag + 16 + payload`, including the
// all-zero UUID — proto3 repeated semantics preserve every slice
// element verbatim.
type UUID [16]byte

func (u UUID) SizeWiresmith() int { return len(u) }

func (u UUID) MarshalWiresmith(buf []byte) (int, error) {
	if len(buf) < len(u) {
		return 0, fmt.Errorf("UUID.MarshalWiresmith: buf too small: have %d, want %d", len(buf), len(u))
	}
	return copy(buf, u[:]), nil
}

func (u *UUID) UnmarshalWiresmith(buf []byte) error {
	if len(buf) != len(*u) {
		return fmt.Errorf("UUID.UnmarshalWiresmith: expected %d bytes, got %d", len(*u), len(buf))
	}
	copy(u[:], buf)
	return nil
}

func (u UUID) EqualWiresmith(other any) bool {
	o, ok := other.(UUID)
	if !ok {
		return false
	}
	return u == o
}

func (u UUID) CompareWiresmith(other any) int {
	o, ok := other.(UUID)
	if !ok {
		return -1
	}
	return bytes.Compare(u[:], o[:])
}

// Tag is a string-backed customtype intended for repeated `string` fields
// — same shape as TenantID but exercised under the repeated emit path.
type Tag string

func (t Tag) SizeWiresmith() int { return len(t) }

func (t Tag) MarshalWiresmith(buf []byte) (int, error) {
	if len(buf) < len(t) {
		return 0, fmt.Errorf("Tag.MarshalWiresmith: buf too small: have %d, want %d", len(buf), len(t))
	}
	return copy(buf, t), nil
}

func (t *Tag) UnmarshalWiresmith(buf []byte) error {
	*t = Tag(buf)
	return nil
}

func (t Tag) EqualWiresmith(other any) bool {
	o, ok := other.(Tag)
	if !ok {
		return false
	}
	return t == o
}

func (t Tag) CompareWiresmith(other any) int {
	o, ok := other.(Tag)
	if !ok {
		return -1
	}
	return strings.Compare(string(t), string(o))
}
