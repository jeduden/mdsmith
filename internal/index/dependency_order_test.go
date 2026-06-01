package index

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// include builds an <?include?> directive body targeting file.
func include(file string) []byte {
	return []byte("# Doc\n\n<?include\nfile: " + file + "\n?>\n<?/include?>\n")
}

// TestDependencyOrder_IncludeLeavesFirst pins the core contract: a file
// that <?include?>s another is placed after its target, so a single fix
// sweep regenerates the upstream file before the downstream file that
// embeds it. top -> mid -> leaf must come out leaf, mid, top.
func TestDependencyOrder_IncludeLeavesFirst(t *testing.T) {
	idx := New("/root")
	idx.Update("top.md", include("mid.md"))
	idx.Update("mid.md", include("leaf.md"))
	idx.Update("leaf.md", []byte("# Leaf\n"))
	idx.Update("lonely.md", []byte("# Lonely\n"))

	in := []string{"top.md", "mid.md", "leaf.md", "lonely.md"}
	got := idx.DependencyOrder(in)

	require.ElementsMatch(t, in, got, "must return the same set of paths")
	pos := make(map[string]int, len(got))
	for i, p := range got {
		pos[p] = i
	}
	assert.Less(t, pos["leaf.md"], pos["mid.md"],
		"leaf must precede mid (mid depends on leaf)")
	assert.Less(t, pos["mid.md"], pos["top.md"],
		"mid must precede top (top depends on mid)")
}

// TestDependencyOrder_IgnoresTargetsOutsideSet verifies that an edge to
// a file not in the input imposes no constraint: such targets are read
// from disk as-is and never fixed, so they cannot reorder the set. With
// no in-set constraint the original order is preserved.
func TestDependencyOrder_IgnoresTargetsOutsideSet(t *testing.T) {
	idx := New("/root")
	idx.Update("top.md", include("external.md")) // external.md not in input
	idx.Update("other.md", []byte("# Other\n"))

	in := []string{"top.md", "other.md"}
	got := idx.DependencyOrder(in)

	assert.Equal(t, in, got,
		"a target outside the input set must not reorder anything")
}

// TestDependencyOrder_BreaksCyclesDeterministically verifies that a
// dependency cycle does not drop files or loop: cycle members fall back
// to input order, and the fix workspace fixpoint loop converges them.
func TestDependencyOrder_BreaksCyclesDeterministically(t *testing.T) {
	idx := New("/root")
	idx.Update("a.md", include("b.md"))
	idx.Update("b.md", include("a.md"))

	in := []string{"a.md", "b.md"}
	got := idx.DependencyOrder(in)

	require.ElementsMatch(t, in, got, "cycle members must all survive")
	assert.Equal(t, in, got, "cycle members keep input order")
}

// TestDependencyOrder_ShortInputUnchanged verifies the fast path: zero
// or one path has nothing to order, so the input is returned as-is.
func TestDependencyOrder_ShortInputUnchanged(t *testing.T) {
	idx := New("/root")
	idx.Update("only.md", include("missing.md"))

	assert.Nil(t, idx.DependencyOrder(nil))
	assert.Equal(t, []string{"only.md"}, idx.DependencyOrder([]string{"only.md"}))
}

// TestDependencyOrder_DoesNotMutateInput verifies the input slice is
// left untouched; callers (the CLI map back to absolute paths) rely on
// their original slice staying intact.
func TestDependencyOrder_DoesNotMutateInput(t *testing.T) {
	idx := New("/root")
	idx.Update("top.md", include("leaf.md"))
	idx.Update("leaf.md", []byte("# Leaf\n"))

	in := []string{"top.md", "leaf.md"}
	_ = idx.DependencyOrder(in)
	assert.Equal(t, []string{"top.md", "leaf.md"}, in, "input slice must not be mutated")
}

// TestDependencyOrder_IgnoresNonDependencyEdges verifies that only
// resolved include/build edges constrain order: a <?catalog?> (an
// Unresolved glob edge) and a Markdown file link impose no constraint,
// so a file reached only by those is not forced before its referrer.
// Only the include edge to dep.md orders dep before host.
func TestDependencyOrder_IgnoresNonDependencyEdges(t *testing.T) {
	idx := New("/root")
	idx.Update("host.md", []byte(
		"# Host\n\n"+
			"<?include\nfile: dep.md\n?>\n<?/include?>\n\n"+
			"<?catalog\nglob: \"*.md\"\nrow: \"- {filename}\"\n?>\n<?/catalog?>\n\n"+
			"See [other](other.md).\n"))
	idx.Update("dep.md", []byte("# Dep\n"))
	idx.Update("other.md", []byte("# Other\n"))

	in := []string{"host.md", "dep.md", "other.md"}
	got := idx.DependencyOrder(in)

	require.ElementsMatch(t, in, got, "must return the same set of paths")
	pos := make(map[string]int, len(got))
	for i, p := range got {
		pos[p] = i
	}
	assert.Less(t, pos["dep.md"], pos["host.md"],
		"only the include edge constrains order: dep before host")
	// other.md is merely link-referenced, so it carries no ordering
	// constraint; presence (ElementsMatch above) is all that's required.
}

// TestDependencyOrder_IgnoresSelfInclude verifies a self-edge (a file
// that includes itself) imposes no constraint and does not drop or
// duplicate the file.
func TestDependencyOrder_IgnoresSelfInclude(t *testing.T) {
	idx := New("/root")
	idx.Update("self.md", include("self.md"))
	idx.Update("leaf.md", []byte("# Leaf\n"))

	in := []string{"self.md", "leaf.md"}
	got := idx.DependencyOrder(in)
	require.ElementsMatch(t, in, got, "self-include must not drop or duplicate a file")
}

// TestDependencyOrder_DeduplicatesRepeatedDependency verifies that a
// file including the same target twice contributes one ordering
// constraint, not two: the target still precedes the host.
func TestDependencyOrder_DeduplicatesRepeatedDependency(t *testing.T) {
	idx := New("/root")
	idx.Update("host.md", []byte(
		"# Host\n\n"+
			"<?include\nfile: leaf.md\n?>\n<?/include?>\n\n"+
			"<?include\nfile: leaf.md\n?>\n<?/include?>\n"))
	idx.Update("leaf.md", []byte("# Leaf\n"))

	in := []string{"host.md", "leaf.md"}
	got := idx.DependencyOrder(in)
	require.ElementsMatch(t, in, got)
	pos := make(map[string]int, len(got))
	for i, p := range got {
		pos[p] = i
	}
	assert.Less(t, pos["leaf.md"], pos["host.md"],
		"a repeated include still orders the target before the host")
}
