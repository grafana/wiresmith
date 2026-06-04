package generator

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/bufbuild/protocompile/linker"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// valueEdge is a single value-type-field reference from one message to
// another, used by the cycle check to build its directed graph.
type valueEdge struct {
	field protoreflect.FieldDescriptor
	to    protoreflect.FullName
}

// validateNoValueCycles rejects any cycle of value-type message fields. A
// cycle in this subgraph would make wiresmith emit a recursively-sized Go
// struct (e.g. `type Tree struct { Child Tree }`), which `go build` rejects
// as "invalid recursive type". The cycle becomes legal once any edge along
// the cycle is broken with `optional`, `repeated`, a `map<>`, a `oneof`, or
// `[(wiresmith.options.pointer) = true]`.
//
// Run after resolvePointerExtension and validatePointerOptions and before
// any emit pass, so cycle errors surface up front rather than as a confusing
// gofmt / go build failure on the generated output.
//
// Across-file recursion is detected too: the function walks every
// non-internal file in results, so a cycle involving messages from two
// proto packages is reported the same way as a same-file self-reference.
func (g *Generator) validateNoValueCycles(results linker.Files) error {
	mds := collectExternalMessages(results)
	if len(mds) == 0 {
		return nil
	}

	// Adjacency: message FullName -> outgoing value-type edges. We index
	// messages by FullName so cycles across files resolve through a single
	// node identity even when the same descriptor is encountered via different
	// file walks.
	nodes := make(map[protoreflect.FullName]protoreflect.MessageDescriptor, len(mds))
	adj := make(map[protoreflect.FullName][]valueEdge, len(mds))
	for _, md := range mds {
		name := md.FullName()
		nodes[name] = md
		for i := 0; i < md.Fields().Len(); i++ {
			fd := md.Fields().Get(i)
			if !g.isValueRecursionEdge(fd) {
				continue
			}
			adj[name] = append(adj[name], valueEdge{field: fd, to: fd.Message().FullName()})
		}
	}

	sccs := tarjanSCC(nodes, adj)

	// Report cycles: an SCC of size > 1 is always a cycle; an SCC of size 1
	// is a cycle iff the single node has a self-loop edge.
	var cycleMsgs []string
	for _, scc := range sccs {
		isCycle := len(scc) > 1
		if !isCycle {
			only := scc[0]
			for _, e := range adj[only] {
				if e.to == only {
					isCycle = true
					break
				}
			}
		}
		if !isCycle {
			continue
		}
		cycleMsgs = append(cycleMsgs, describeCycle(scc, adj))
	}
	if len(cycleMsgs) == 0 {
		return nil
	}
	slices.Sort(cycleMsgs)

	var out strings.Builder
	out.WriteString("value-type message field cycle detected — generated Go would be a recursively-sized struct that does not compile.\nBreak the cycle by marking one of the fields below with `optional` (becomes a pointer with proto3-optional semantics), `[(wiresmith.options.pointer) = true]` (becomes a pointer without optional semantics), or by changing it to `repeated`, `map<>`, or a oneof variant:\n")
	for _, m := range cycleMsgs {
		out.WriteString("  - ")
		out.WriteString(m)
		out.WriteByte('\n')
	}
	return fmt.Errorf("%s", out.String())
}

// isValueRecursionEdge reports whether fd produces a value-type Go struct
// field that would participate in a recursive-size compile error.
//
// The set of disqualifiers mirrors the goFieldType / fieldType dispatch: any
// shape that surfaces as a pointer, slice, map, or interface in Go is safe
// to recurse through. Specifically:
//
//   - non-message kinds can't introduce message recursion
//   - map fields surface as `map[K]V` (heap-allocated entries)
//   - oneof variants surface through an interface
//   - `optional` fields surface as `*T`
//   - repeated fields surface as `[]T`
//   - `(wiresmith.options.pointer) = true` fields surface as `*T` / `[]*T`
//
// Everything else is a singular value-type message field — exactly the shape
// that emits `Child Type` and breaks `go build` on recursion.
func (g *Generator) isValueRecursionEdge(fd protoreflect.FieldDescriptor) bool {
	if fd.Kind() != protoreflect.MessageKind {
		return false
	}
	if fd.IsMap() {
		return false
	}
	if isRealOneof(fd) {
		return false
	}
	if fd.HasOptionalKeyword() {
		return false
	}
	if fd.IsList() {
		return false
	}
	if opt := findOption[*pointerOption](g.options); opt != nil && opt.Has(fd) {
		return false
	}
	return true
}

// collectExternalMessages returns every non-map-entry message from every
// non-internal file in results. Internal schema files (the embedded
// wiresmith/options.proto) are skipped so the cycle check only considers
// user-facing types.
func collectExternalMessages(results linker.Files) []protoreflect.MessageDescriptor {
	var out []protoreflect.MessageDescriptor
	seen := make(map[protoreflect.FullName]bool)
	for _, fd := range results {
		if isInternalSchemaFile(fd) {
			continue
		}
		forEachMessage(fd, func(md protoreflect.MessageDescriptor) {
			name := md.FullName()
			if seen[name] {
				return
			}
			seen[name] = true
			out = append(out, md)
		})
	}
	return out
}

// tarjanSCC returns the strongly connected components of the message graph
// described by nodes and adj. Implementation is iterative to avoid blowing
// the Go stack on pathological import depths. Edges whose target is not in
// nodes are simply skipped — they can't participate in a cycle that re-enters
// the user-facing graph.
//
// Output ordering is stable: outer iteration starts from sorted node names
// so the resulting SCC ordering is reproducible across runs.
func tarjanSCC(
	nodes map[protoreflect.FullName]protoreflect.MessageDescriptor,
	adj map[protoreflect.FullName][]valueEdge,
) [][]protoreflect.FullName {
	type info struct {
		index, lowlink int
		onStack        bool
		visited        bool
	}
	state := make(map[protoreflect.FullName]*info, len(nodes))
	var stack []protoreflect.FullName
	idx := 0
	var sccs [][]protoreflect.FullName

	type frame struct {
		node    protoreflect.FullName
		edgeIdx int
	}

	visit := func(start protoreflect.FullName) {
		if s := state[start]; s != nil && s.visited {
			return
		}
		state[start] = &info{index: idx, lowlink: idx, onStack: true, visited: true}
		idx++
		stack = append(stack, start)
		frames := []frame{{node: start, edgeIdx: 0}}

		for len(frames) > 0 {
			top := &frames[len(frames)-1]
			vInfo := state[top.node]
			edges := adj[top.node]

			if top.edgeIdx < len(edges) {
				w := edges[top.edgeIdx].to
				top.edgeIdx++
				if _, known := nodes[w]; !known {
					continue
				}
				wInfo := state[w]
				if wInfo == nil {
					state[w] = &info{index: idx, lowlink: idx, onStack: true, visited: true}
					idx++
					stack = append(stack, w)
					frames = append(frames, frame{node: w, edgeIdx: 0})
					continue
				}
				if wInfo.onStack && wInfo.index < vInfo.lowlink {
					vInfo.lowlink = wInfo.index
				}
				continue
			}

			if vInfo.lowlink == vInfo.index {
				var scc []protoreflect.FullName
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					state[w].onStack = false
					scc = append(scc, w)
					if w == top.node {
						break
					}
				}
				sccs = append(sccs, scc)
			}

			poppedNode := top.node
			frames = frames[:len(frames)-1]
			if len(frames) > 0 {
				parent := state[frames[len(frames)-1].node]
				if state[poppedNode].lowlink < parent.lowlink {
					parent.lowlink = state[poppedNode].lowlink
				}
			}
		}
	}

	starts := make([]protoreflect.FullName, 0, len(nodes))
	for name := range nodes {
		starts = append(starts, name)
	}
	slices.Sort(starts)
	for _, name := range starts {
		visit(name)
	}
	return sccs
}

// describeCycle renders a single SCC as a human-readable cycle path.
// For a self-loop, the output is `pkg.Msg.field (type pkg.Msg)`. For a
// longer cycle, we walk one DFS path through the SCC starting from its
// alphabetically-first node and chain its edges back to that start.
//
// We can't always find a single Hamiltonian path through an arbitrary SCC,
// but a cycle is guaranteed to exist — so we just pick any cycle by greedy
// DFS over edges that stay inside the SCC.
func describeCycle(scc []protoreflect.FullName, adj map[protoreflect.FullName][]valueEdge) string {
	if len(scc) == 1 {
		only := scc[0]
		for _, e := range adj[only] {
			if e.to == only {
				return fmt.Sprintf("%s.%s (type %s)", only, e.field.Name(), only)
			}
		}
		return fmt.Sprintf("%s (self-cycle)", only)
	}

	inScc := make(map[protoreflect.FullName]bool, len(scc))
	for _, n := range scc {
		inScc[n] = true
	}

	names := make([]protoreflect.FullName, len(scc))
	copy(names, scc)
	slices.Sort(names)
	start := names[0]

	type step struct {
		from  protoreflect.FullName
		field protoreflect.FieldDescriptor
		to    protoreflect.FullName
	}
	var path []step
	visited := map[protoreflect.FullName]bool{}
	var dfs func(node protoreflect.FullName) bool
	dfs = func(node protoreflect.FullName) bool {
		visited[node] = true
		edges := make([]valueEdge, len(adj[node]))
		copy(edges, adj[node])
		sort.Slice(edges, func(i, j int) bool {
			if edges[i].to == edges[j].to {
				return edges[i].field.Name() < edges[j].field.Name()
			}
			return edges[i].to < edges[j].to
		})
		for _, e := range edges {
			if !inScc[e.to] {
				continue
			}
			if e.to == start && len(path) > 0 {
				path = append(path, step{from: node, field: e.field, to: e.to})
				return true
			}
			if visited[e.to] {
				continue
			}
			path = append(path, step{from: node, field: e.field, to: e.to})
			if dfs(e.to) {
				return true
			}
			path = path[:len(path)-1]
		}
		return false
	}
	dfs(start)

	if len(path) == 0 {
		parts := make([]string, len(scc))
		for i, n := range scc {
			parts[i] = string(n)
		}
		return strings.Join(parts, " <-> ")
	}

	parts := make([]string, 0, len(path)+1)
	parts = append(parts, string(start))
	for _, s := range path {
		parts = append(parts, fmt.Sprintf("--(field %q)--> %s", s.field.Name(), s.to))
	}
	return strings.Join(parts, " ")
}
