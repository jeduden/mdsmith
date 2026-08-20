package astutil

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
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
