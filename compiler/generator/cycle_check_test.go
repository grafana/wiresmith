package generator

import (
	"strings"
	"testing"
)

// expectCycleError fails the test if err is nil or does not look like the
// cycle-detection error produced by validateNoValueCycles. The header is the
// stable anchor; the per-cycle line is matched separately by reasonSubstr so
// callers can pin the specific cycle they expect to be reported.
func expectCycleError(t *testing.T, err error, reasonSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "value-type message field cycle detected") {
		t.Errorf("missing header in error: %s", msg)
	}
	if !strings.Contains(msg, reasonSubstr) {
		t.Errorf("missing reason %q in error: %s", reasonSubstr, msg)
	}
}

// TestCycleCheck_SelfReferenceRejected is the headline case from CR-4: a
// message with a value-typed self-reference would otherwise emit `Child Tree`
// inside `type Tree struct`, which `go build` rejects as a recursively-sized
// type. The cycle check must intercept this before any file is written.
func TestCycleCheck_SelfReferenceRejected(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message Tree {
  Tree child = 1;
}
`)
	expectCycleError(t, err, "test.v1.Tree.child")
}

// TestCycleCheck_MutualRecursionRejected covers the two-message cycle:
// A holds a value-typed B and B holds a value-typed A. Each struct's size
// would depend on the other's, so the cycle is just as fatal as a direct
// self-reference. The reported cycle path must mention both fields so the
// user can pick either to break.
func TestCycleCheck_MutualRecursionRejected(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message A {
  B b = 1;
}
message B {
  A a = 1;
}
`)
	expectCycleError(t, err, "test.v1.A")
	if err != nil && !strings.Contains(err.Error(), "test.v1.B") {
		t.Errorf("expected both A and B in cycle error, got: %s", err.Error())
	}
}

// TestCycleCheck_LongerCycleRejected pins the three-node cycle A -> B -> C -> A.
// Tarjan's SCC must collapse the entire cycle into a single component and the
// description walks the cycle so the user sees the full chain.
func TestCycleCheck_LongerCycleRejected(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message A { B b = 1; }
message B { C c = 1; }
message C { A a = 1; }
`)
	expectCycleError(t, err, "test.v1.A")
	if err != nil {
		msg := err.Error()
		for _, want := range []string{"test.v1.B", "test.v1.C"} {
			if !strings.Contains(msg, want) {
				t.Errorf("expected %q in cycle error, got: %s", want, msg)
			}
		}
	}
}

// TestCycleCheck_OptionalSelfReferenceAllowed reproduces the canonical
// linked-list shape from proto/basic/recursive.proto. `optional` makes the
// field a `*LinkedList`, which has a fixed size in Go, so the cycle is broken
// at the type level.
func TestCycleCheck_OptionalSelfReferenceAllowed(t *testing.T) {
	if err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message LinkedList {
  int64 value = 1;
  optional LinkedList next = 2;
}
`); err != nil {
		t.Fatalf("expected no error for optional self-reference, got: %v", err)
	}
}

// TestCycleCheck_RepeatedSelfReferenceAllowed pins the tree-via-slice shape.
// `repeated TreeNode children` surfaces as `[]TreeNode`, whose element type
// is `TreeNode` but the slice header itself has a fixed size, so the struct
// is well-formed.
func TestCycleCheck_RepeatedSelfReferenceAllowed(t *testing.T) {
	if err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message TreeNode {
  string label = 1;
  repeated TreeNode children = 2;
}
`); err != nil {
		t.Fatalf("expected no error for repeated self-reference, got: %v", err)
	}
}

// TestCycleCheck_PointerOptionSelfReferenceAllowed verifies the explicit
// opt-in via `(wiresmith.options.pointer) = true` also breaks the cycle.
// Without this exception, users would be unable to express "recursive
// message, no proto3-optional semantics" — only `optional` would work, and
// `optional` carries presence semantics they may not want.
func TestCycleCheck_PointerOptionSelfReferenceAllowed(t *testing.T) {
	if err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
import "wiresmith/options.proto";
message Node {
  int64 value = 1;
  Node next = 2 [(wiresmith.options.pointer) = true];
}
`); err != nil {
		t.Fatalf("expected no error for pointer-option self-reference, got: %v", err)
	}
}

// TestCycleCheck_OneofSelfReferenceAllowed verifies that a oneof variant
// pointing back to the parent message does not trigger the cycle check.
// Oneofs surface as a Go interface, breaking the size dependency.
func TestCycleCheck_OneofSelfReferenceAllowed(t *testing.T) {
	if err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message Box {
  oneof choice {
    Box inner = 1;
    int32 leaf = 2;
  }
}
`); err != nil {
		t.Fatalf("expected no error for oneof self-reference, got: %v", err)
	}
}

// TestCycleCheck_MapSelfReferenceAllowed verifies map value type pointing
// back to the containing message is allowed. Maps surface as
// `map[K]V` in Go — the map header is fixed-size and the value is stored
// on the heap, so there's no size-recursion problem.
func TestCycleCheck_MapSelfReferenceAllowed(t *testing.T) {
	if err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message Tree {
  string name = 1;
  map<string, Tree> children = 2;
}
`); err != nil {
		t.Fatalf("expected no error for map-self-reference, got: %v", err)
	}
}

// TestCycleCheck_NoCycleHonoursAcyclicReferences confirms that ordinary
// nested-but-acyclic message hierarchies aren't flagged. This is the
// regression guard that protects every well-formed proto in the existing
// gen/ tree — failing it would mean the cycle check is over-broad.
func TestCycleCheck_NoCycleHonoursAcyclicReferences(t *testing.T) {
	if err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message Inner { int32 x = 1; }
message Outer { Inner i = 1; }
`); err != nil {
		t.Fatalf("expected no error for acyclic chain, got: %v", err)
	}
}

// TestCycleCheck_OneEdgeBrokenAvoidsCycle tests the mixed case: a two-message
// cycle where one direction is `optional`. The optional edge breaks the SCC
// in the value-recursion graph even though the underlying message graph
// still has a cycle. This is the smallest case proving the check correctly
// excludes optional edges from the graph.
func TestCycleCheck_OneEdgeBrokenAvoidsCycle(t *testing.T) {
	if err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message A {
  optional B b = 1;
}
message B {
  A a = 1;
}
`); err != nil {
		t.Fatalf("expected no error when one edge is optional, got: %v", err)
	}
}

// TestCycleCheck_DisjointCyclesAllReported confirms that two independent
// cycles in the same proto file both surface in a single error. Without
// this, fixing one cycle would just unmask the next on the following run,
// turning what should be one fix-up into several.
func TestCycleCheck_DisjointCyclesAllReported(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message Loop1 { Loop1 self = 1; }
message Loop2 { Loop2 self = 1; }
`)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"test.v1.Loop1", "test.v1.Loop2"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected %q in error, got: %s", want, msg)
		}
	}
}

// TestCycleCheck_NestedMessageSelfReferenceRejected covers a self-loop on a
// nested message type. forEachMessage walks nested-before-parent, so this
// also confirms our message collection traverses into nested definitions.
func TestCycleCheck_NestedMessageSelfReferenceRejected(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message Outer {
  message Inner {
    Inner child = 1;
  }
  Inner i = 1;
}
`)
	expectCycleError(t, err, "test.v1.Outer.Inner")
}

// TestCycleCheck_OneofAndSelfTogether documents that having BOTH a oneof
// variant self-reference AND a direct value-typed self-reference still
// fails: the cycle check considers only the direct value field, ignoring
// the oneof. Without this, a user could accidentally add a non-oneof
// recursive field next to the oneof and the broken struct would slip
// through.
func TestCycleCheck_OneofAndSelfTogether(t *testing.T) {
	err := runGenerator(t, `
syntax = "proto3";
package test.v1;
option go_package = "github.com/grafana/wiresmith/gen/test/v1";
message Tangle {
  oneof choice {
    Tangle inner = 1;
    int32 leaf = 2;
  }
  Tangle direct = 3;
}
`)
	expectCycleError(t, err, "test.v1.Tangle.direct")
}
