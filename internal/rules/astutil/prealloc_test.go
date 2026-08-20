package astutil

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// growthDoc builds a document with n headings, each followed by a
// paragraph, so both memoized collectors see n elements.
func growthDoc(n int) string {
	var b strings.Builder
	b.WriteString("# Title\n\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "## Section %d\n\n", i)
		b.WriteString("Some prose in this section for the collector.\n\n")
	}
	return b.String()
}

func newDocFile(t *testing.T, source string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("doc.md", []byte(source))
	require.NoError(t, err)
	return f
}

// TestCollectors_PreSizedNotRegrown pins the guideline's "Pre-size
// slices. make([]T, 0, n) when n is known. append doubles capacity up
// to ~1024 then grows ~25%, copying each step."
// (docs/development/high-performance-go.md#allocations).
//
// Both collectors are memoized once per *lint.File, so every growth
// step is one avoidable allocation on every file in a workspace.
//
// The assertion is on growth steps rather than a raw total: a
// collector that starts from a nil slice needs O(log n) allocations to
// reach n elements, so a 200-element document costs strictly more than
// a 4-element one. A pre-sized collector allocates its backing array
// once regardless of size.
//
// The capacity hint is the root's child count, which is an estimate
// rather than a bound — see TestCollectors_NestedHeadingsStillCorrect
// for the shape that exceeds it.
func TestCollectors_PreSizedNotRegrown(t *testing.T) {
	cases := []struct {
		name  string
		build func(f *lint.File) any
	}{
		{"headings", buildHeadingNodes},
		{"section paragraphs", buildSectionParagraphs},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			small := newDocFile(t, growthDoc(4))
			large := newDocFile(t, growthDoc(200))

			smallAllocs := testing.AllocsPerRun(20, func() {
				_ = tc.build(small)
			})
			largeAllocs := testing.AllocsPerRun(20, func() {
				_ = tc.build(large)
			})

			assert.Equalf(t, smallAllocs, largeAllocs,
				"allocations scale with element count (%v small vs %v large): "+
					"the collector still regrows from a nil slice",
				smallAllocs, largeAllocs)
		})
	}
}

// TestCollectors_ResultsUnchangedWhenPreSized guards the behaviour the
// capacity hint must not disturb: same elements, same order.
func TestCollectors_ResultsUnchangedWhenPreSized(t *testing.T) {
	f := newDocFile(t, growthDoc(12))

	paras := CollectSectionParagraphs(f)
	require.Len(t, paras, 12, "one paragraph per section")
	for i := 1; i < len(paras); i++ {
		assert.Greaterf(t, paras[i].Line, paras[i-1].Line,
			"paragraphs must stay in document order at index %d", i)
	}

	nodes := CollectHeadingNodes(f)
	require.Len(t, nodes, 13, "title plus one heading per section")
	for i := 1; i < len(nodes); i++ {
		assert.NotSame(t, nodes[i-1], nodes[i])
	}
}

// TestCollectors_NestedHeadingsStillCorrect covers the shape the
// capacity hint does not bound: the walk descends into blockquotes and
// list items, so a document can hold more headings than the root has
// children. The hint is only a hint — the result must stay complete
// and ordered when the slice regrows past it.
func TestCollectors_NestedHeadingsStillCorrect(t *testing.T) {
	f := newDocFile(t, "# Top\n\n"+
		"> ## Quoted A\n>\n> ## Quoted B\n>\n> ## Quoted C\n>\n> ## Quoted D\n")

	require.Less(t, f.AST.ChildCount(), 5,
		"this document must have fewer root children than headings")

	nodes := CollectHeadingNodes(f)
	require.Len(t, nodes, 5, "the top heading plus the four quoted ones")
	for i := 1; i < len(nodes); i++ {
		assert.NotSame(t, nodes[i-1], nodes[i])
	}
}

// TestCollectors_NilWhenNothingFound pins the project convention the
// capacity hint must not break: "Return nil, not []T{}"
// (docs/development/high-performance-go.md#allocations). A pre-sized
// slice is non-nil the moment make() runs, so a document that matches
// nothing would otherwise hand callers an empty-but-non-nil slice —
// distinguishable in tests, JSON and reflect — and pay two allocations
// where it used to pay none.
func TestCollectors_NilWhenNothingFound(t *testing.T) {
	t.Run("no headings", func(t *testing.T) {
		f := newDocFile(t, strings.Repeat("Some prose paragraph.\n\n", 40))

		got, ok := buildHeadingNodes(f).([]*ast.Heading)
		require.True(t, ok)
		assert.Nil(t, got, "no headings must yield nil, not an empty slice")

		allocs := testing.AllocsPerRun(20, func() { _ = buildHeadingNodes(f) })
		assert.Zero(t, allocs, "a heading-free document must not allocate")
	})

	t.Run("no paragraphs", func(t *testing.T) {
		var b strings.Builder
		for i := 0; i < 40; i++ {
			fmt.Fprintf(&b, "## Heading %d\n\n", i)
		}
		f := newDocFile(t, b.String())

		got, ok := buildSectionParagraphs(f).([]SectionParagraph)
		require.True(t, ok)
		assert.Nil(t, got, "no paragraphs must yield nil, not an empty slice")

		allocs := testing.AllocsPerRun(20, func() { _ = buildSectionParagraphs(f) })
		assert.Zero(t, allocs, "a paragraph-free document must not allocate")
	})
}
