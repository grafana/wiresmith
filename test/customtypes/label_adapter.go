package customtypes

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

// LabelAdapter is the Go type referenced by (wiresmith.options.customtype)
// on a *message* field in proto/basic/customtype_message.proto. The user
// type owns the inner Label submessage's wire encoding — the wiresmith-
// owned wrapper supplies the outer tag and length prefix, but the bytes
// it wraps are written and read by LabelAdapter itself.
//
// Wire format mirrors the plain `message Label { string name = 1; string
// value = 2; }`: a sequence of tag + length + payload pairs for whichever
// of `name` and `value` are non-empty. Both fields are length-delimited
// (wire type 2), so the encoder emits tag = (field<<3)|2.
//
// The implementation deliberately uses only Google's `protowire` package
// (no wiresmith-internal helpers) to keep the customtype contract
// explicit: a user type is responsible for any wire-compatible encoding,
// not one that has to depend on wiresmith's own emit-time helpers.
type LabelAdapter struct {
	Name  string
	Value string
}

const (
	labelAdapterTagName  = (1 << 3) | 2
	labelAdapterTagValue = (2 << 3) | 2
)

func (l LabelAdapter) SizeWiresmith() int {
	n := 0
	if len(l.Name) > 0 {
		n += 1 + protowire.SizeBytes(len(l.Name))
	}
	if len(l.Value) > 0 {
		n += 1 + protowire.SizeBytes(len(l.Value))
	}
	return n
}

// MarshalWiresmith writes forward into buf, which the caller has sized to
// exactly SizeWiresmith() bytes. Output order is field-tag order (1 then
// 2) — the encoded bytes are byte-for-byte identical to a plain
// `message Label`-encoded payload of the same Name/Value.
func (l LabelAdapter) MarshalWiresmith(buf []byte) (int, error) {
	if len(buf) < l.SizeWiresmith() {
		return 0, fmt.Errorf("LabelAdapter.MarshalWiresmith: buf too small: have %d, want %d", len(buf), l.SizeWiresmith())
	}
	pos := 0
	if len(l.Name) > 0 {
		buf[pos] = labelAdapterTagName
		pos++
		pos += binary_AppendVarint(buf[pos:], uint64(len(l.Name)))
		pos += copy(buf[pos:], l.Name)
	}
	if len(l.Value) > 0 {
		buf[pos] = labelAdapterTagValue
		pos++
		pos += binary_AppendVarint(buf[pos:], uint64(len(l.Value)))
		pos += copy(buf[pos:], l.Value)
	}
	return pos, nil
}

// UnmarshalWiresmith reads the exact payload window the wiresmith decoder
// sliced from the wire input. Unknown fields are skipped via protowire,
// matching the proto3 "ignore unknown" contract.
func (l *LabelAdapter) UnmarshalWiresmith(buf []byte) error {
	*l = LabelAdapter{}
	for len(buf) > 0 {
		num, wt, n := protowire.ConsumeTag(buf)
		if n < 0 {
			return protowire.ParseError(n)
		}
		buf = buf[n:]
		switch num {
		case 1:
			if wt != protowire.BytesType {
				return fmt.Errorf("LabelAdapter: field 1 (name) wrong wire type %d", wt)
			}
			v, n := protowire.ConsumeString(buf)
			if n < 0 {
				return protowire.ParseError(n)
			}
			l.Name = v
			buf = buf[n:]
		case 2:
			if wt != protowire.BytesType {
				return fmt.Errorf("LabelAdapter: field 2 (value) wrong wire type %d", wt)
			}
			v, n := protowire.ConsumeString(buf)
			if n < 0 {
				return protowire.ParseError(n)
			}
			l.Value = v
			buf = buf[n:]
		default:
			n := protowire.ConsumeFieldValue(num, wt, buf)
			if n < 0 {
				return protowire.ParseError(n)
			}
			buf = buf[n:]
		}
	}
	return nil
}

func (l LabelAdapter) EqualWiresmith(other any) bool {
	o, ok := other.(LabelAdapter)
	if !ok {
		return false
	}
	return l.Name == o.Name && l.Value == o.Value
}

func (l LabelAdapter) CompareWiresmith(other any) int {
	o, ok := other.(LabelAdapter)
	if !ok {
		return -1
	}
	if l.Name != o.Name {
		if l.Name < o.Name {
			return -1
		}
		return 1
	}
	if l.Value != o.Value {
		if l.Value < o.Value {
			return -1
		}
		return 1
	}
	return 0
}

// binary_AppendVarint writes a protobuf varint into buf, returning the
// number of bytes written. Local to this file so the customtype example
// reads as a self-contained encoding rather than a thin wrapper around
// wiresmith's protohelpers.
func binary_AppendVarint(buf []byte, v uint64) int {
	pos := 0
	for v >= 0x80 {
		buf[pos] = byte(v) | 0x80
		v >>= 7
		pos++
	}
	buf[pos] = byte(v)
	return pos + 1
}
