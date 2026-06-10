package schema

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mtFile(t *testing.T, body string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("doc.md", []byte(body))
	require.NoError(t, err)
	return f
}

func TestHeadingCaptures_EdgeCases(t *testing.T) {
	dh := DocHeading{Level: 2, Text: "Step 1", Line: 1}

	ok, caps := headingCaptures(nil, dh, nil)
	assert.False(t, ok)
	assert.Nil(t, caps)

	// Invalid regex → compile error → non-match.
	ok, _ = headingCaptures(&Matcher{Regex: "("}, dh, nil)
	assert.False(t, ok)

	// Valid pattern, no submatch.
	ok, _ = headingCaptures(&Matcher{Regex: "Other"}, dh, nil)
	assert.False(t, ok)

	// Literal match, no named groups → ok, nil map.
	ok, caps = headingCaptures(&Matcher{Regex: "Step 1"}, dh, nil)
	assert.True(t, ok)
	assert.Nil(t, caps)
}

func TestHeadingStem_UnknownHelperSkipped(t *testing.T) {
	// `\#(bogus)` is neither `digits` nor an fmvar call, so the
	// parseFmvarCall branch is taken with ok == false.
	stem, fmvars, hasDigits := HeadingStem(&Scope{
		Heading: "x", Matcher: &Matcher{Regex: `\#(bogus)`},
	})
	assert.Equal(t, "", stem)
	assert.Nil(t, fmvars)
	assert.False(t, hasDigits)
}

func TestHeadingCaptures_NamedUnnamedAndMulti(t *testing.T) {
	dh := DocHeading{Level: 2, Text: "ab", Line: 1}

	// Two named groups: out is nil then non-nil across iterations.
	ok, caps := headingCaptures(&Matcher{Regex: `(?P<x>.)(?P<y>.)`}, dh, nil)
	assert.True(t, ok)
	assert.Equal(t, map[string]string{"x": "a", "y": "b"}, caps)

	// An unnamed group yields a "" SubexpName that is skipped.
	ok, caps = headingCaptures(&Matcher{Regex: `(.)(?P<z>.)`}, dh, nil)
	assert.True(t, ok)
	assert.Equal(t, map[string]string{"z": "b"}, caps)
}

func TestBuildMatchTree_BroadMatcherSkipped(t *testing.T) {
	// A broad `.+` matcher that is not a slot (no repeat) is still
	// skipped by the projection.
	sch := &Schema{RootLevel: 2, Sections: []Scope{
		{Heading: "any", Matcher: &Matcher{Regex: ".+"}},
		literalScope("Goal"),
	}}
	mt := BuildMatchTree(mtFile(t, "## Whatever\n\n## Goal\n"), sch, nil)
	require.Len(t, mt.Root.Children, 1)
	assert.Equal(t, "Goal", mt.Root.Children[0].Heading.Text)
}

func TestScopeCaptures_RegexAndFmvarMerge(t *testing.T) {
	dh := DocHeading{Level: 2, Text: "Step 1 RFC", Line: 1}
	sc := &Scope{
		Heading: "Step {n} {id}",
		Matcher: &Matcher{Regex: `Step \#(digits) \#(fmvar(id))`},
	}
	caps := scopeCaptures(sc, dh, map[string]any{"id": "RFC"})
	assert.Equal(t, "1", caps["n"])    // from the regex capture
	assert.Equal(t, "RFC", caps["id"]) // merged in with caps != nil
}

func TestCollectContent_ExhaustsNodes(t *testing.T) {
	// A required code-block with only a paragraph present: the
	// inner loop runs until nodeIdx reaches len(nodes).
	sc := Scope{
		Heading: "Goal",
		Matcher: &Matcher{Regex: "Goal"},
		Content: []ContentEntry{{Kind: ContentKindCodeBlock, Required: true}},
	}
	sch := &Schema{RootLevel: 2, Sections: []Scope{sc}}
	mt := BuildMatchTree(mtFile(t, "## Goal\n\njust prose\n"), sch, nil)
	require.Len(t, mt.Root.Children, 1)
	assert.Empty(t, mt.Root.Children[0].Content)
}

func TestCollectContent_LaterEntryNonMatch(t *testing.T) {
	// entry0=paragraph absent; the only later entry (list) does
	// not match the leading code block, so laterContentEntryMatches
	// evaluates nodeMatchesKind to false before yielding.
	sc := Scope{
		Heading: "Goal",
		Matcher: &Matcher{Regex: "Goal"},
		Content: []ContentEntry{
			{Kind: ContentKindParagraph, Required: false},
			{Kind: ContentKindList, Required: true},
		},
	}
	sch := &Schema{RootLevel: 2, Sections: []Scope{sc}}
	body := "## Goal\n\n```\nx\n```\n\n- a\n"
	mt := BuildMatchTree(mtFile(t, body), sch, nil)
	require.Len(t, mt.Root.Children, 1)
	got := mt.Root.Children[0].Content
	require.Len(t, got, 1)
	assert.Equal(t, ContentKindList, got[0].Entry.Kind)
}

func TestHeadingStem_NilCases(t *testing.T) {
	stem, fmvars, hasDigits := HeadingStem(nil)
	assert.Equal(t, "", stem)
	assert.Nil(t, fmvars)
	assert.False(t, hasDigits)

	// Nil matcher (preamble) falls back to the heading label.
	stem, _, _ = HeadingStem(&Scope{Heading: "Lead"})
	assert.Equal(t, "Lead", stem)
}

func TestScopeCaptures_FmvarResolution(t *testing.T) {
	dh := DocHeading{Level: 2, Text: "RFC-7", Line: 1}

	// Unresolvable fmvar (field absent) is skipped, leaving no
	// capture rather than a bogus one.
	sc := &Scope{Heading: "{id}", Matcher: &Matcher{Regex: `\#(fmvar(id))`}}
	assert.Nil(t, scopeCaptures(sc, dh, nil))

	// Resolvable fmvar is captured by field name.
	caps := scopeCaptures(sc, dh, map[string]any{"id": "RFC-7"})
	assert.Equal(t, "RFC-7", caps["id"])
}

// An fmvar with an unparseable CUE path is skipped rather than
// captured (scopeCaptures' len(path) == 0 guard).
func TestScopeCaptures_InvalidFmvarPath(t *testing.T) {
	dh := DocHeading{Level: 2, Text: "X", Line: 1}
	sc := &Scope{Heading: "{}", Matcher: &Matcher{Regex: `\#(fmvar())`}}
	assert.Nil(t, scopeCaptures(sc, dh, map[string]any{"": "v"}))
}

func TestBuildMatchTree_NilAndEmptySchema(t *testing.T) {
	f := mtFile(t, "## Goal\n")
	assert.NotNil(t, BuildMatchTree(f, nil, nil).Root)
	assert.NotNil(t, BuildMatchTree(f, &Schema{}, nil).Root)
}

func TestBuildMatchTree_LiteralAndNested(t *testing.T) {
	body := "## Goal\n\nthe goal\n\n## Steps\n\n### First\n\ndo it\n"
	sch := &Schema{
		RootLevel: 2,
		Sections: []Scope{
			literalScope("Goal"),
			nested("Steps", literalScope("First")),
		},
	}
	mt := BuildMatchTree(mtFile(t, body), sch, nil)
	require.NotNil(t, mt.Root)
	require.Len(t, mt.Root.Children, 2)

	assert.Equal(t, "Goal", mt.Root.Children[0].Heading.Text)
	steps := mt.Root.Children[1]
	assert.Equal(t, "Steps", steps.Heading.Text)
	require.Len(t, steps.Children, 1)
	assert.Equal(t, "First", steps.Children[0].Heading.Text)
}

func TestBuildMatchTree_PreambleAndWildcardSkipped(t *testing.T) {
	body := "## Intro\n\n## Random\n"
	sch := &Schema{
		RootLevel: 2,
		Sections: []Scope{
			preambleScope(),
			literalScope("Intro"),
			slotScope(),
		},
	}
	mt := BuildMatchTree(mtFile(t, body), sch, nil)
	// Preamble (no content here) + Intro; the wildcard slot and the
	// unlisted "Random" heading are skipped.
	require.Len(t, mt.Root.Children, 2)
	assert.True(t, mt.Root.Children[0].Preamble)
	assert.Equal(t, "Intro", mt.Root.Children[1].Heading.Text)
}

func TestBuildMatchTree_RepeatingCaptures(t *testing.T) {
	body := "## Step 1\n\n## Step 2\n"
	sch := &Schema{
		RootLevel: 2,
		Sections: []Scope{
			{
				Heading: "Step {n}",
				Matcher: &Matcher{
					Regex:  `Step \#(digits)`,
					Repeat: Repeat{Set: true, Min: 1},
				},
			},
		},
	}
	mt := BuildMatchTree(mtFile(t, body), sch, nil)
	require.Len(t, mt.Root.Children, 2)
	assert.Equal(t, "1", mt.Root.Children[0].Captures["n"])
	assert.Equal(t, "2", mt.Root.Children[1].Captures["n"])
}

func TestBuildMatchTree_FmvarCapture(t *testing.T) {
	body := "## RFC-0001\n\nbody\n"
	sch := &Schema{
		RootLevel: 2,
		Sections: []Scope{
			{
				Heading: "{id}",
				Matcher: &Matcher{Regex: `\#(fmvar(id))`},
			},
		},
	}
	mt := BuildMatchTree(mtFile(t, body), sch, map[string]any{"id": "RFC-0001"})
	require.Len(t, mt.Root.Children, 1)
	assert.Equal(t, "RFC-0001", mt.Root.Children[0].Captures["id"])
}

func TestBuildMatchTree_OptionalContentYieldsToRequired(t *testing.T) {
	// Absent optional paragraph before a required code block must
	// not consume the code block (exercises collectContent's
	// later-entry yield and laterContentEntryMatches).
	sc := Scope{
		Heading: "Goal",
		Matcher: &Matcher{Regex: "Goal"},
		Content: []ContentEntry{
			{Kind: ContentKindParagraph, Required: false},
			{Kind: ContentKindCodeBlock, Required: true},
		},
	}
	sch := &Schema{RootLevel: 2, Sections: []Scope{sc}}
	mt := BuildMatchTree(mtFile(t, "## Goal\n\n```\nx\n```\n"), sch, nil)
	require.Len(t, mt.Root.Children, 1)
	got := mt.Root.Children[0].Content
	require.Len(t, got, 1)
	assert.Equal(t, ContentKindCodeBlock, got[0].Entry.Kind)
}

// An `unlisted` content entry is skipped (collectContent's
// e.Kind == ContentKindUnlisted continue), and a body node that
// matches neither the current nor any later entry is consumed
// (the trailing nodeIdx++).
func TestCollectContent_UnlistedSkipAndNodeAdvance(t *testing.T) {
	sc := Scope{
		Heading: "Goal",
		Matcher: &Matcher{Regex: "Goal"},
		Content: []ContentEntry{
			{Kind: ContentKindUnlisted},
			{Kind: ContentKindCodeBlock, Required: true},
		},
	}
	sch := &Schema{RootLevel: 2, Sections: []Scope{sc}}
	// A leading paragraph matches neither the unlisted entry (skipped)
	// nor the code-block, so it is advanced past before the code is
	// matched.
	mt := BuildMatchTree(mtFile(t, "## Goal\n\nintro\n\n```\nx\n```\n"), sch, nil)
	require.Len(t, mt.Root.Children, 1)
	got := mt.Root.Children[0].Content
	require.Len(t, got, 1)
	assert.Equal(t, ContentKindCodeBlock, got[0].Entry.Kind)
}

func TestLaterContentEntryMatches(t *testing.T) {
	content := []ContentEntry{
		{Kind: ContentKindUnlisted},
		{Kind: ContentKindList},
	}
	f := mtFile(t, "- a\n")
	lst := f.AST.FirstChild()
	assert.True(t, laterContentEntryMatches(content, 0, lst))
	assert.False(t, laterContentEntryMatches(content, 2, lst))
}

func TestBuildMatchTree_Content(t *testing.T) {
	body := "## Goal\n\n```go\ncode\n```\n\n- a\n- b\n"
	sch := &Schema{
		RootLevel: 2,
		Sections: []Scope{
			{
				Heading: "Goal",
				Matcher: &Matcher{Regex: "Goal"},
				Content: []ContentEntry{
					{Kind: ContentKindCodeBlock, Required: true},
					{Kind: ContentKindList, Required: true},
				},
			},
		},
	}
	mt := BuildMatchTree(mtFile(t, body), sch, nil)
	require.Len(t, mt.Root.Children, 1)
	require.Len(t, mt.Root.Children[0].Content, 2)
	assert.Equal(t, ContentKindCodeBlock, mt.Root.Children[0].Content[0].Entry.Kind)
	assert.Equal(t, ContentKindList, mt.Root.Children[0].Content[1].Entry.Kind)
}

// A scope with `projection: blocks` captures its whole body on the
// match (ProjectsBlocks set, Body populated with deeper headings
// kept). Plan 246.
func TestBuildMatchTree_ScopeBlocksBody(t *testing.T) {
	body := "## Notes\n\npara\n\n### Sub\n\nx\n"
	sch := &Schema{
		RootLevel: 2,
		Sections: []Scope{{
			Heading:    "Notes",
			Matcher:    &Matcher{Regex: "Notes"},
			Projection: ProjectionBlocks,
		}},
	}
	mt := BuildMatchTree(mtFile(t, body), sch, nil)
	require.Len(t, mt.Root.Children, 1)
	sm := mt.Root.Children[0]
	assert.True(t, sm.ProjectsBlocks)
	// para paragraph + the ### Sub heading (kept for section nesting)
	// + the nested paragraph -> three body nodes.
	require.Len(t, sm.Body, 3)
}

// A schema-level `projection: blocks` default sets ProjectsBlocks on
// a matched scope that does not itself set a projection. Plan 246.
func TestBuildMatchTree_SchemaBlocksDefault(t *testing.T) {
	sch := &Schema{
		RootLevel:  2,
		Projection: ProjectionBlocks,
		Sections:   []Scope{{Heading: "Notes", Matcher: &Matcher{Regex: "Notes"}}},
	}
	mt := BuildMatchTree(mtFile(t, "## Notes\n\npara\n"), sch, nil)
	require.Len(t, mt.Root.Children, 1)
	assert.True(t, mt.Root.Children[0].ProjectsBlocks)
}

// A schema-level `projection: blocks` default appends a synthetic
// unlisted ScopeMatch for every root-level heading no declared scope
// claimed, carrying the heading, Unlisted, ProjectsBlocks, and the
// section body (exercising collectUnlistedBlockMatches' append). Plan
// 246.
func TestBuildMatchTree_CollectsUnlistedBlockMatches(t *testing.T) {
	body := "## Goal\n\ng\n\n## Background\n\nb1\n\nb2\n"
	sch := &Schema{
		RootLevel:  2,
		Projection: ProjectionBlocks,
		Sections:   []Scope{{Heading: "Goal", Matcher: &Matcher{Regex: "Goal"}}},
	}
	mt := BuildMatchTree(mtFile(t, body), sch, nil)
	require.Len(t, mt.Root.Children, 2)
	// Declared Goal scope first, then the synthetic unlisted Background.
	assert.False(t, mt.Root.Children[0].Unlisted)
	bg := mt.Root.Children[1]
	require.True(t, bg.Unlisted, "Background must be a synthetic unlisted match")
	assert.True(t, bg.ProjectsBlocks)
	assert.Equal(t, "Background", bg.Heading.Text)
	assert.Nil(t, bg.Scope, "unlisted match has a nil Scope")
	// Body spans the two background paragraphs.
	require.Len(t, bg.Body, 2)
}

// collectUnlistedBlockMatches skips a heading a declared scope already
// claimed (the claimed[i] continue) and a heading shallower/deeper than
// the root level (the dh.Level != rootLevel continue): only the
// unclaimed root-level Extra heading becomes a synthetic match.
func TestCollectUnlistedBlockMatches_SkipsClaimedAndOffLevel(t *testing.T) {
	// H1 (off level, skipped) + claimed H2 Goal + H3 under it (off
	// level) + unclaimed H2 Extra (collected).
	body := "# Title\n\n## Goal\n\ng\n\n### Deep\n\nd\n\n## Extra\n\ne\n"
	sch := &Schema{
		RootLevel:  2,
		Projection: ProjectionBlocks,
		Sections:   []Scope{{Heading: "Goal", Matcher: &Matcher{Regex: "Goal"}}},
	}
	mt := BuildMatchTree(mtFile(t, body), sch, nil)
	var unlisted []string
	for _, c := range mt.Root.Children {
		if c.Unlisted {
			unlisted = append(unlisted, c.Heading.Text)
		}
	}
	assert.Equal(t, []string{"Extra"}, unlisted,
		"only the unclaimed root-level heading is collected")
}

func TestAnyScopeProjectsBlocks(t *testing.T) {
	assert.False(t, anyScopeProjectsBlocks(nil))
	assert.False(t, anyScopeProjectsBlocks([]Scope{{Heading: "A"}}))
	assert.True(t, anyScopeProjectsBlocks([]Scope{{Projection: ProjectionBlocks}}))
	// Nested scope projects blocks -> the recursive arm reports true.
	parent := Scope{Heading: "P", Sections: []Scope{{Projection: ProjectionBlocks}}}
	assert.True(t, anyScopeProjectsBlocks([]Scope{parent}))
}

func TestBodyBlocksInRange_KeepsHeadings(t *testing.T) {
	f := mtFile(t, "## Notes\n\npara\n\n### Sub\n\nx\n")
	blocks := topLevelBlocks(f, parseWithTableExt(f.Source))
	// Range covering the whole document keeps the ### Sub heading,
	// unlike blocksInRange which strips every heading.
	body := bodyBlocksInRange(blocks, 1, len(f.Lines)+1)
	stripped := blocksInRange(blocks, 1, len(f.Lines)+1)
	assert.Greater(t, len(body), len(stripped))
}
