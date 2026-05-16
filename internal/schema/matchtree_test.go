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
