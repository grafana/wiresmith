package types

import (
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

// captureEmitter records Writef output and AddImport calls so tests can
// assert exact generated text per type without running the full generator.
type captureEmitter struct {
	buf     strings.Builder
	imports []string
}

func (c *captureEmitter) Writef(format string, args ...any) {
	fmt.Fprintf(&c.buf, format, args...)
}

func (c *captureEmitter) ReverseTag(string, protowire.Number, protowire.Type) {}

func (c *captureEmitter) AddImport(path, _ string) {
	c.imports = append(c.imports, path)
}

func TestEmitEqual_Scalars(t *testing.T) {
	cases := []struct {
		name string
		t    interface {
			EmitEqual(e Emitter, indent, lhs, rhs string)
		}
		want string
	}{
		{"bool", BoolType{}, "\tif a != b {\n\t\treturn false\n\t}\n"},
		{"string", StringType{}, "\tif a != b {\n\t\treturn false\n\t}\n"},
		{"varint", varintBase{}, "\tif a != b {\n\t\treturn false\n\t}\n"},
		{"sint32", Sint32Type{}, "\tif a != b {\n\t\treturn false\n\t}\n"},
		{"sint64", Sint64Type{}, "\tif a != b {\n\t\treturn false\n\t}\n"},
		{"fixed32", fixed32Base{}, "\tif a != b {\n\t\treturn false\n\t}\n"},
		{"fixed64", fixed64Base{}, "\tif a != b {\n\t\treturn false\n\t}\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := &captureEmitter{}
			c.t.EmitEqual(e, "\t", "a", "b")
			if got := e.buf.String(); got != c.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, c.want)
			}
		})
	}
}

func TestEmitEqual_Bytes(t *testing.T) {
	e := &captureEmitter{}
	BytesType{}.EmitEqual(e, "\t", "this.X", "that1.X")

	want := "\tif !bytes.Equal(this.X, that1.X) {\n\t\treturn false\n\t}\n"
	if got := e.buf.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
	// Bytes import must be registered lazily so callers don't need a
	// pre-scan to decide whether the generated body uses bytes.Equal.
	if len(e.imports) != 1 || e.imports[0] != "bytes" {
		t.Errorf("imports = %v, want [bytes]", e.imports)
	}
}

func TestEmitEqual_Message(t *testing.T) {
	e := &captureEmitter{}
	MessageType{}.EmitEqual(e, "\t", "this.X", "that1.X")

	want := "\tif !this.X.Equal(that1.X) {\n\t\treturn false\n\t}\n"
	if got := e.buf.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEmitEqual_Indent(t *testing.T) {
	// Deeper indent (inside a loop) must propagate to every emitted line.
	e := &captureEmitter{}
	BoolType{}.EmitEqual(e, "\t\t", "a", "b")
	want := "\t\tif a != b {\n\t\t\treturn false\n\t\t}\n"
	if got := e.buf.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEmitEqual_OptionalScalar(t *testing.T) {
	e := &captureEmitter{}
	(&OptionalField{Inner: BoolType{}}).EmitEqual(e, "\t", "this.X", "that1.X")

	want := strings.Join([]string{
		"\tif this.X != that1.X {",
		"\t\tif this.X == nil || that1.X == nil {",
		"\t\t\treturn false",
		"\t\t}",
		"\t\tif *this.X != *that1.X {",
		"\t\t\treturn false",
		"\t\t}",
		"\t}",
		"",
	}, "\n")
	if got := e.buf.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEmitEqual_OptionalBytes(t *testing.T) {
	e := &captureEmitter{}
	// Use the same pointer registration the type registry uses so the
	// *BytesType assertion inside OptionalField.EmitEqual catches it.
	(&OptionalField{Inner: &BytesType{}}).EmitEqual(e, "\t", "this.X", "that1.X")

	want := strings.Join([]string{
		"\tif (this.X == nil) != (that1.X == nil) {",
		"\t\treturn false",
		"\t}",
		"\tif !bytes.Equal(this.X, that1.X) {",
		"\t\treturn false",
		"\t}",
		"",
	}, "\n")
	if got := e.buf.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEmitEqual_OptionalMessage(t *testing.T) {
	e := &captureEmitter{}
	(&OptionalField{Inner: &MessageType{}}).EmitEqual(e, "\t", "this.X", "that1.X")

	want := strings.Join([]string{
		"\tif (this.X == nil) != (that1.X == nil) {",
		"\t\treturn false",
		"\t}",
		"\tif this.X != nil && !this.X.Equal(that1.X) {",
		"\t\treturn false",
		"\t}",
		"",
	}, "\n")
	if got := e.buf.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEmitEqual_RepeatedMessage(t *testing.T) {
	e := &captureEmitter{}
	(&RepeatedField{Inner: &MessageType{}}).EmitEqual(e, "\t", "this.X", "that1.X")

	want := strings.Join([]string{
		"\tif len(this.X) != len(that1.X) {",
		"\t\treturn false",
		"\t}",
		"\tfor i := range this.X {",
		"\t\tif !this.X[i].Equal(that1.X[i]) {",
		"\t\t\treturn false",
		"\t\t}",
		"\t}",
		"",
	}, "\n")
	if got := e.buf.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEmitEqual_RepeatedPointer(t *testing.T) {
	e := &captureEmitter{}
	(&RepeatedPointer{Inner: &MessageType{}}).EmitEqual(e, "\t", "this.X", "that1.X")

	want := strings.Join([]string{
		"\tif len(this.X) != len(that1.X) {",
		"\t\treturn false",
		"\t}",
		"\tfor i := range this.X {",
		"\t\tif (this.X[i] == nil) != (that1.X[i] == nil) {",
		"\t\t\treturn false",
		"\t\t}",
		"\t\tif this.X[i] != nil && !this.X[i].Equal(that1.X[i]) {",
		"\t\t\treturn false",
		"\t\t}",
		"\t}",
		"",
	}, "\n")
	if got := e.buf.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEmitEqual_PointerMessage(t *testing.T) {
	e := &captureEmitter{}
	(&PointerField{Inner: &MessageType{}}).EmitEqual(e, "\t", "this.X", "that1.X")

	want := strings.Join([]string{
		"\tif (this.X == nil) != (that1.X == nil) {",
		"\t\treturn false",
		"\t}",
		"\tif this.X != nil && !this.X.Equal(that1.X) {",
		"\t\treturn false",
		"\t}",
		"",
	}, "\n")
	if got := e.buf.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEmitEqual_MapMessageValue(t *testing.T) {
	e := &captureEmitter{}
	(&MapField{Key: StringType{}, Val: &MessageType{}}).EmitEqual(e, "\t", "this.M", "that1.M")

	want := strings.Join([]string{
		"\tif len(this.M) != len(that1.M) {",
		"\t\treturn false",
		"\t}",
		"\tfor k, v := range this.M {",
		"\t\tv2, ok := that1.M[k]",
		"\t\tif !ok {",
		"\t\t\treturn false",
		"\t\t}",
		"\t\tif !v.Equal(v2) {",
		"\t\t\treturn false",
		"\t\t}",
		"\t}",
		"",
	}, "\n")
	if got := e.buf.String(); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestZeroLiteral(t *testing.T) {
	cases := []struct {
		name string
		t    Type
		want string
	}{
		{"bool", BoolType{}, "false"},
		{"string", StringType{}, `""`},
		{"bytes", BytesType{}, "nil"},
		{"message", &MessageType{}, "nil"},
		{"varint", varintBase{}, "0"},
		{"sint32", Sint32Type{}, "0"},
		{"sint64", Sint64Type{}, "0"},
		{"fixed32", fixed32Base{}, "0"},
		{"fixed64", fixed64Base{}, "0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.t.ZeroLiteral(); got != c.want {
				t.Errorf("ZeroLiteral() = %q, want %q", got, c.want)
			}
		})
	}
}
