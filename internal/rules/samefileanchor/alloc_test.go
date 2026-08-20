package samefileanchor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
)

// headingDoc builds a document with n headings and no fragment links.
// Every Markdown file has an ATX heading, so this is the shape of
// essentially every file in a workspace.
func headingDoc(n int) string {
	var b strings.Builder
	b.WriteString("# Title\n\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "## Section %d\n\nProse under the section.\n\n", i)
	}
	return b.String()
}

func newFile(t testing.TB, source string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("doc.md", []byte(source))
	require.NoError(t, err)
	return f
}

// TestCheck_NoSlugSetWithoutFragmentLinks pins the fix. MDS070's only
// gate was "does the source contain a '#' byte?", which every ATX
// heading satisfies — so the rule built the full slug set (a recursive
// AST descent plus two maps, with a string per heading) on virtually
// every file in a workspace, to answer fragment links that almost none
// of them contain.
//
// The slug set is now built on the first same-file fragment link, so a
// document without one must not pay for it. The assertion is
// scale-invariant: if the set is still built eagerly, allocations grow
// with the number of headings.
//
// Guideline: "Gate expensive analyzers behind a cheap pre-check" and
// "the cheapest call is the one you never make" —
// docs/development/high-performance-go.md#skip-work-you-dont-need.
func TestCheck_NoSlugSetWithoutFragmentLinks(t *testing.T) {
	r := &Rule{}

	few := newFile(t, headingDoc(3))
	many := newFile(t, headingDoc(150))

	require.Nil(t, r.Check(few))
	require.Nil(t, r.Check(many))

	fewAllocs := testing.AllocsPerRun(20, func() { _ = r.Check(few) })
	manyAllocs := testing.AllocsPerRun(20, func() { _ = r.Check(many) })

	assert.Equalf(t, fewAllocs, manyAllocs,
		"allocations scale with heading count (%v vs %v): the slug set is "+
			"still built on files that have no fragment link to check",
		fewAllocs, manyAllocs)
}

// TestCheck_StillValidatesFragmentLinks is the behavioural half: the
// lazy build must not change which anchors are reported.
func TestCheck_StillValidatesFragmentLinks(t *testing.T) {
	r := &Rule{}

	t.Run("resolves against a heading", func(t *testing.T) {
		f := newFile(t, "# Title\n\n## Real Section\n\nSee [x](#real-section).\n")
		assert.Nil(t, r.Check(f))
	})

	t.Run("flags a missing anchor", func(t *testing.T) {
		f := newFile(t, "# Title\n\n## Real Section\n\nSee [x](#nope).\n")
		diags := r.Check(f)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "nope")
	})

	t.Run("duplicate headings disambiguate", func(t *testing.T) {
		f := newFile(t, "# Title\n\n## Intro\n\n## Intro\n\nSee [x](#intro-1).\n")
		assert.Nil(t, r.Check(f))
	})

	t.Run("several fragments build the set once", func(t *testing.T) {
		f := newFile(t, "# Title\n\n## A\n\n## B\n\n"+
			"See [x](#a), [y](#b), [z](#missing).\n")
		diags := r.Check(f)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "missing")
	})

	t.Run("image fragment is checked too", func(t *testing.T) {
		f := newFile(t, "# Title\n\n## A\n\n![alt](#gone)\n")
		diags := r.Check(f)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "gone")
	})

	t.Run("bare hash is always valid", func(t *testing.T) {
		f := newFile(t, "# Title\n\nBack to [top](#).\n")
		assert.Nil(t, r.Check(f))
	})

	t.Run("file with no headings flags the fragment", func(t *testing.T) {
		f := newFile(t, "Just prose linking to [x](#anywhere).\n")
		diags := r.Check(f)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "anywhere")
	})
}
