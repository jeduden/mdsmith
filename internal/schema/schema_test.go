package schema

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// diagsMessages flattens diags' messages so a failing test assertion
// can show every reported diagnostic without expanding the struct.
func diagsMessages(diags []lint.Diagnostic) []string {
	out := make([]string, len(diags))
	for i, d := range diags {
		out[i] = d.Message
	}
	return out
}

// makeDiagForTest is the diagnostic constructor the tests use; it
// matches MDS020's shape so message formats are exercised the same
// way the real rule emits them.
func makeDiagForTest(file string, line int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:    file,
		Line:    line,
		Column:  1,
		RuleID:  "MDS020",
		Message: msg,
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

func newDocFile(t *testing.T, path, source string) *lint.File {
	t.Helper()
	f, err := lint.NewFile(path, []byte(source))
	require.NoError(t, err)
	return f
}

// ---- ParseFile (compat with legacy heading-template) ----

func TestParseFile_FlatHeadings(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "# ?\n\n## Goal\n\n## Tasks\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	assert.Equal(t, 1, sch.RootLevel)
	require.Len(t, sch.Sections, 1)
	require.Equal(t, "?", sch.Sections[0].Heading)
	require.True(t, sch.Sections[0].Closed)
	require.Len(t, sch.Sections[0].Sections, 2)
	assert.Equal(t, "Goal", sch.Sections[0].Sections[0].Heading)
	assert.Equal(t, "Tasks", sch.Sections[0].Sections[1].Heading)
}

func TestParseFile_NoH1(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "## ...\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	assert.Equal(t, 2, sch.RootLevel)
	require.Len(t, sch.Sections, 1)
	assert.True(t, isSlotMatcher(sch.Sections[0].Matcher),
		"a `## ...` heading row desugars to a slot matcher")
}

func TestParseFile_FrontmatterCUE(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "---\nid: 'string'\n---\n# ?\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	assert.Equal(t, "string", sch.Frontmatter["id"])
}

func TestParseFile_RequireFilename(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"<?require\nfilename: \"foo-*.md\"\n?>\n# ?\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	assert.Equal(t, "foo-*.md", sch.Filename)
}

// ---- ParseInline ----

func TestParseInline_Empty(t *testing.T) {
	sch, err := ParseInline(map[string]any{}, "kind rfc")
	require.NoError(t, err)
	require.NotNil(t, sch)
	assert.Equal(t, 2, sch.RootLevel)
	assert.True(t, sch.IsEmpty())
}

func TestParseInline_FrontmatterAndSections(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"id":     `=~"^RFC-[0-9]{4}$"`,
			"title?": `string`,
		},
		"filename": "RFC-[0-9][0-9][0-9][0-9].md",
		"closed":   true,
		"sections": []any{
			map[string]any{"heading": "Overview"},
			map[string]any{
				"heading":  map[string]any{"regex": "Decision|Resolution"},
				"sections": []any{map[string]any{"heading": "Outcome"}},
			},
			map[string]any{
				"heading": map[string]any{
					"regex":  ".+",
					"repeat": map[string]any{"min": 0},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind rfc")
	require.NoError(t, err)
	assert.True(t, sch.Closed)
	assert.Equal(t, "RFC-[0-9][0-9][0-9][0-9].md", sch.Filename)
	require.Len(t, sch.Sections, 3)
	assert.Equal(t, "Overview", sch.Sections[0].Heading)
	require.NotNil(t, sch.Sections[1].Matcher)
	assert.Equal(t, "Decision|Resolution", sch.Sections[1].Matcher.Regex)
	require.Len(t, sch.Sections[1].Sections, 1)
	assert.Equal(t, "Outcome", sch.Sections[1].Sections[0].Heading)
	assert.True(t, isSlotMatcher(sch.Sections[2].Matcher))

	assert.Contains(t, sch.FrontmatterCUE(), `id: =~"^RFC-[0-9]{4}$"`)
	assert.Contains(t, sch.FrontmatterCUE(), `title?: string`)
}

func TestParseInline_UnknownTopKey(t *testing.T) {
	_, err := ParseInline(map[string]any{"foo": 1}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown schema key")
}

func TestParseInline_BadScopeKey(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Foo",
			"unknown": true,
		}},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown scope key")
}

// ---- Validate (legacy fixtures behaviour) ----

func TestValidate_MissingSection(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "# ?\n\n## Goal\n\n## Tasks\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# My Plan\n\n## Goal\n\nGoal text.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message,
		`## Tasks: got <missing>, expected section to be present`)
}

func TestValidate_ExtraSectionFlagged(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "# ?\n\n## Goal\n\n## Tasks\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# My Plan\n\n## Extra\n\nx\n\n## Goal\n\ny\n\n## Tasks\n\nz\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message,
		`## Extra: got <present>, expected not declared in schema`)
}

func TestValidate_OutOfOrder(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "# ?\n\n## Goal\n\n## Tasks\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# My Plan\n\n## Tasks\n\nx\n\n## Goal\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message,
		`## Tasks: got <out of order>, expected in declared order`)
	assert.Contains(t, diags[0].Message, `expected after "## Goal"`)
}

func TestValidate_WildcardH1LevelMismatch(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "# ?\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "## Title\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `Title: got h2, expected h1`)
}

func TestValidate_FilenameMismatch(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"<?require\nfilename: \"[0-9]*_*.md\"\n?>\n# ?\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	doc := newDocFile(t, "filename-mismatch.md", "# My Doc\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message,
		`filename: got "filename-mismatch.md", expected filename matching glob [0-9]*_*.md`)
}

// ---- Validate (inline schemas) ----

func TestValidate_Inline_OpenScopeTolerates(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "Overview"},
			map[string]any{"heading": "Decision"},
		},
	}
	sch, err := ParseInline(raw, "kind rfc")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# Title\n\n## Overview\n\nx\n\n## Notes\n\ny\n\n## Decision\n\nz\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags,
		"open scope should tolerate the unlisted Notes heading")
}

func TestValidate_Inline_ClosedRejectsUnlisted(t *testing.T) {
	raw := map[string]any{
		"closed": true,
		"sections": []any{
			map[string]any{"heading": "Overview"},
			map[string]any{"heading": "Decision"},
		},
	}
	sch, err := ParseInline(raw, "kind rfc")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# Title\n\n## Overview\n\nx\n\n## Notes\n\ny\n\n## Decision\n\nz\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message,
		`## Notes: got <present>, expected not declared in schema`)
}

func TestValidate_Inline_WildcardSlotTolerates(t *testing.T) {
	raw := map[string]any{
		"closed": true,
		"sections": []any{
			map[string]any{"heading": "Overview"},
			map[string]any{"heading": map[string]any{
				"regex":  ".+",
				"repeat": map[string]any{"min": 0},
			}},
			map[string]any{"heading": "References"},
		},
	}
	sch, err := ParseInline(raw, "kind rfc")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# Title\n\n## Overview\n\nx\n\n## A\n\ny\n\n## B\n\nz\n\n## References\n\nw\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags,
		"wildcard slot should tolerate unlisted sections at that position")
}

func TestValidate_Inline_DisjunctionMatches(t *testing.T) {
	// Aliases are gone; encode the disjunction in the regex.
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{"regex": "Symptoms|Indicators"},
			},
		},
	}
	sch, err := ParseInline(raw, "kind runbook")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n\n## Indicators\n\nx\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags)
}

func TestValidate_Inline_NestedThreeLevels(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Diagnosis",
				"sections": []any{
					map[string]any{
						"heading": "Step",
						"sections": []any{
							map[string]any{"heading": "Check"},
							map[string]any{"heading": "Expected"},
						},
					},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind runbook")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Diagnosis\n\n### Step\n\n#### Check\n\nx\n\n#### Expected\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags, "three-level tree should validate cleanly")
}

func TestValidate_Inline_FrontmatterCUE(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"id": `=~"^RFC-[0-9]{4}$"`,
		},
	}
	sch, err := ParseInline(raw, "kind rfc")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n")
	// Document FM has the wrong shape.
	diags := Validate(doc, sch, map[string]any{"id": "BAD"}, false, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `id: got "BAD"`)
	assert.Contains(t, diags[0].Message, `string matching ^RFC-[0-9]{4}$`)
}

// ---- ParseInline (content:) ----

func TestParseInline_ContentEntryParses(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{
				map[string]any{"kind": "code-block", "lang": "yaml"},
				map[string]any{"kind": "table",
					"columns": []any{"Setting", "Default"}},
				map[string]any{"kind": "list",
					"ordered": true, "min-items": 2, "max-items": 5},
				map[string]any{"kind": "paragraph"},
				map[string]any{"kind": "unlisted"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1)
	entries := sch.Sections[0].Content
	require.Len(t, entries, 5)
	assert.Equal(t, "code-block", entries[0].Kind)
	assert.Equal(t, "yaml", entries[0].Lang)
	assert.True(t, entries[0].Required)
	assert.Equal(t, "table", entries[1].Kind)
	assert.Equal(t, []string{"Setting", "Default"}, entries[1].Columns)
	assert.Equal(t, "list", entries[2].Kind)
	assert.True(t, entries[2].OrderedSet)
	assert.True(t, entries[2].Ordered)
	assert.Equal(t, 2, entries[2].MinItems)
	assert.Equal(t, 5, entries[2].MaxItems)
	assert.Equal(t, "paragraph", entries[3].Kind)
	assert.Equal(t, "unlisted", entries[4].Kind)
	assert.False(t, entries[4].Required)
}

func TestParseInline_ContentProjectionInline(t *testing.T) {
	sch, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Headline",
			"content": []any{map[string]any{
				"kind": "paragraph", "projection": "inline",
			}},
		}},
	}, "kind x")
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1)
	require.Len(t, sch.Sections[0].Content, 1)
	assert.Equal(t, "inline", sch.Sections[0].Content[0].Projection)
}

func TestParseInline_ContentProjectionTextAndCode(t *testing.T) {
	sch, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{
				map[string]any{"kind": "paragraph", "projection": "text"},
				map[string]any{"kind": "code-block", "projection": "code"},
			},
		}},
	}, "kind x")
	require.NoError(t, err)
	entries := sch.Sections[0].Content
	require.Len(t, entries, 2)
	assert.Equal(t, "text", entries[0].Projection)
	assert.Equal(t, "code", entries[1].Projection)
}

func TestParseInline_ContentProjectionWrongType(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Headline",
			"content": []any{map[string]any{
				"kind": "paragraph", "projection": 42,
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "projection must be a string")
}

func TestParseInline_ContentProjectionUnknownValue(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Headline",
			"content": []any{map[string]any{
				"kind": "paragraph", "projection": "html",
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown projection")
}

// parseProjectionCombo is a tiny helper for the projection-matrix
// tests: it parses a single-section schema whose one content entry
// carries the given kind and projection.
func parseProjectionCombo(kind, projection string) (*Schema, error) {
	return ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": kind, "projection": projection,
			}},
		}},
	}, "kind x")
}

// TestParseInline_ProjectionMatrixAllowed pins every (kind,
// projection) pair the schema accepts: paragraph takes text or
// inline, code-block takes code, list takes tree.
func TestParseInline_ProjectionMatrixAllowed(t *testing.T) {
	allowed := []struct{ kind, projection string }{
		{"paragraph", "text"},
		{"paragraph", "inline"},
		{"code-block", "code"},
		{"list", "tree"},
	}
	for _, c := range allowed {
		t.Run(c.kind+"/"+c.projection, func(t *testing.T) {
			sch, err := parseProjectionCombo(c.kind, c.projection)
			require.NoError(t, err)
			require.Len(t, sch.Sections[0].Content, 1)
			assert.Equal(t, c.projection, sch.Sections[0].Content[0].Projection)
		})
	}
}

// TestParseInline_ProjectionCodeOnParagraphRejected rejects
// `projection: code` on a paragraph — paragraphs project text or
// inline, never code.
func TestParseInline_ProjectionCodeOnParagraphRejected(t *testing.T) {
	_, err := parseProjectionCombo("paragraph", "code")
	require.Error(t, err)
	assert.Contains(t, err.Error(),
		"kind: paragraph allows projection text or inline, not code")
}

// TestParseInline_ProjectionTextOnCodeBlockRejected rejects
// `projection: text` on a code-block — code blocks project only code.
func TestParseInline_ProjectionTextOnCodeBlockRejected(t *testing.T) {
	_, err := parseProjectionCombo("code-block", "text")
	require.Error(t, err)
	assert.Contains(t, err.Error(),
		"kind: code-block allows projection code, not text")
}

// TestParseInline_ProjectionInlineOnCodeBlockRejected rejects
// `projection: inline` on a code-block. This subsumes the old
// inline-only check with the unified matrix message.
func TestParseInline_ProjectionInlineOnCodeBlockRejected(t *testing.T) {
	_, err := parseProjectionCombo("code-block", "inline")
	require.Error(t, err)
	assert.Contains(t, err.Error(),
		"kind: code-block allows projection code, not inline")
}

// TestParseInline_ProjectionOnTableRejected rejects projection values
// that are not valid for a table (text, code, inline are paragraph/code
// only; unknown values are rejected).
func TestParseInline_ProjectionOnTableRejected(t *testing.T) {
	for _, p := range []string{"text", "code", "inline"} {
		t.Run(p, func(t *testing.T) {
			_, err := parseProjectionCombo("table", p)
			require.Error(t, err)
		})
	}
}

// TestParseInline_ProjectionNonTreeOnListRejected rejects the
// paragraph/code-block projections on a list — a list projects only
// `tree` (or its flat default when projection is omitted).
func TestParseInline_ProjectionNonTreeOnListRejected(t *testing.T) {
	for _, p := range []string{"text", "code", "inline"} {
		t.Run(p, func(t *testing.T) {
			_, err := parseProjectionCombo("list", p)
			require.Error(t, err)
			assert.Contains(t, err.Error(),
				"kind: list allows projection tree, not "+p)
		})
	}
}

// TestParseInline_ProjectionTreeOnNonListRejected rejects
// `projection: tree` everywhere except a list — tree is the list's
// structured mode and has no meaning on a paragraph, code-block,
// table, or unlisted slot.
func TestParseInline_ProjectionTreeOnNonListRejected(t *testing.T) {
	cases := []struct{ kind, want string }{
		{"paragraph", "kind: paragraph allows projection text or inline, not tree"},
		{"code-block", "kind: code-block allows projection code, not tree"},
		{"table", "kind: table allows projection records or rows, not tree"},
		{"unlisted", "projection is not allowed on kind: unlisted"},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			_, err := parseProjectionCombo(c.kind, "tree")
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
		})
	}
}

// TestParseInline_TableProjectionRecords accepts projection: records on
// kind: table and stores it on the ContentEntry.
func TestParseInline_TableProjectionRecords(t *testing.T) {
	sch, err := parseProjectionCombo("table", "records")
	require.NoError(t, err)
	require.Len(t, sch.Sections[0].Content, 1)
	assert.Equal(t, "records", sch.Sections[0].Content[0].Projection)
}

// TestParseInline_TableProjectionRows accepts projection: rows on
// kind: table and stores it on the ContentEntry.
func TestParseInline_TableProjectionRows(t *testing.T) {
	sch, err := parseProjectionCombo("table", "rows")
	require.NoError(t, err)
	require.Len(t, sch.Sections[0].Content, 1)
	assert.Equal(t, "rows", sch.Sections[0].Content[0].Projection)
}

// TestParseInline_TableProjectionUnknownRejected verifies that an
// unknown projection value on a table is still rejected.
func TestParseInline_TableProjectionUnknownRejected(t *testing.T) {
	_, err := parseProjectionCombo("table", "html")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown projection")
}

// TestParseInline_ProjectionOnUnlistedRejected rejects any projection
// on an unlisted slot.
func TestParseInline_ProjectionOnUnlistedRejected(t *testing.T) {
	_, err := parseProjectionCombo("unlisted", "code")
	require.Error(t, err)
	assert.Contains(t, err.Error(),
		"projection is not allowed on kind: unlisted")
}

func TestParseInline_ContentUnknownKind(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{"kind": "blockquote"}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown content kind")
}

func TestParseInline_ContentMisplacedField(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": "paragraph", "lang": "yaml",
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only valid on `kind: code-block`")
}

func TestParseInline_ContentRequiredOnUnlistedRejected(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": "unlisted", "required": true, // tests rejection
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(),
		"`required:` is not allowed on a `kind: unlisted`")
}

func TestParseInline_ContentRejectedOnWildcard(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": map[string]any{
				"regex":  ".+",
				"repeat": map[string]any{"min": 0},
			},
			"content": []any{map[string]any{"kind": "paragraph"}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(),
		"not allowed on a slot")
}

// ---- Validate (content:) ----

func TestValidate_Content_MissingCodeBlock(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{
				map[string]any{"kind": "code-block", "lang": "yaml"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Examples\n\nNo code block here.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	require.Contains(t, diags[0].Message,
		`missing required content "code-block lang=yaml" inside ## Examples`)
}

func TestValidate_Content_CodeBlockMatches(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{
				map[string]any{"kind": "code-block", "lang": "yaml"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Examples\n\n```yaml\nfoo: bar\n```\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags)
}

func TestValidate_Content_CodeBlockWrongLang(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{
				map[string]any{"kind": "code-block", "lang": "yaml"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Examples\n\n```json\n{}\n```\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message,
		`code block language "json" does not match required "yaml"`)
}

func TestValidate_Content_TableColumnsMatch(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Settings",
			"content": []any{
				map[string]any{"kind": "table",
					"columns": []any{"Setting", "Default"}},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Settings\n\n| Setting | Default |\n|---------|---------|\n| foo     | 1       |\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags)
}

func TestValidate_Content_TableColumnsMismatch(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Settings",
			"content": []any{
				map[string]any{"kind": "table",
					"columns": []any{"Setting", "Default"}},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Settings\n\n| Key | Value |\n|-----|-------|\n| foo | 1     |\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message,
		"table headers [Key Value] do not match required [Setting Default]")
}

func TestValidate_Content_ListMinItems(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Steps",
			"content": []any{
				map[string]any{"kind": "list",
					"ordered": true, "min-items": 2},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Steps\n\n1. Only one\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message,
		"list has 1 items, required at least 2")
}

func TestValidate_Content_ListOrderedMismatch(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Steps",
			"content": []any{
				map[string]any{"kind": "list", "ordered": true},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Steps\n\n- a\n- b\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message,
		"list ordered=false does not match required ordered=true")
}

func TestValidate_Content_OutOfOrder(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{
				map[string]any{"kind": "code-block", "lang": "yaml"},
				map[string]any{"kind": "table"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Examples\n\n"+
			"| A | B |\n|---|---|\n| x | y |\n\n"+
			"```yaml\nfoo: bar\n```\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message,
		`content "table" out of order: expected after "code-block lang=yaml"`)
}

func TestValidate_Content_ClosedFlagsUnlisted(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"closed":  true,
			"content": []any{
				map[string]any{"kind": "code-block"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Examples\n\n```\nx\n```\n\nExtra paragraph here.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message,
		`unexpected content "paragraph" inside ## Examples`)
}

func TestValidate_Content_UnlistedSlotTolerates(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"closed":  true,
			"content": []any{
				map[string]any{"kind": "code-block"},
				map[string]any{"kind": "unlisted"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Examples\n\n```\nx\n```\n\nExtra trailing paragraph.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags)
}

func TestValidate_Content_OpenScopeTolerates(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{
				map[string]any{"kind": "code-block"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Trailing paragraph is silently tolerated by the default open scope.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Examples\n\n```\nx\n```\n\nTrailing.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags)
}

// TestValidate_Content_PreambleLabelAndLine regresses a Copilot
// review on PR #285: a preamble scope (`heading: null`) carrying a
// `content:` entry must anchor a missing-required diagnostic at
// line 1 (not line 0) and label the parent as "preamble" rather
// than rendering an empty heading like "## ".
func TestValidate_Content_PreambleLabelAndLine(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": nil,
				"content": []any{
					map[string]any{"kind": "code-block", "lang": "yaml"},
				},
			},
			map[string]any{"heading": "Body"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Doc has no preamble code block; we expect one missing-required
	// diagnostic anchored at line 1, naming "preamble" — not "## ".
	doc := newDocFile(t, "doc.md",
		"# T\n\nPreamble prose without the required code block.\n\n## Body\n\nx\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	var preamble *lint.Diagnostic
	for i, d := range diags {
		if strings.Contains(d.Message, "missing required content") {
			preamble = &diags[i]
			break
		}
	}
	require.NotNil(t, preamble, "expected a missing-required content diagnostic")
	assert.GreaterOrEqual(t, preamble.Line, 1,
		"preamble diagnostic must not anchor at line 0")
	assert.Contains(t, preamble.Message, "inside preamble",
		"preamble diagnostic must label the parent as preamble")
	assert.NotContains(t, preamble.Message, "## ",
		"preamble diagnostic must not render an empty heading")
}

// ---- ParseInline content: error paths ----

func TestParseInline_ContentNotAList(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": "not-a-list",
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content must be a list")
}

func TestParseInline_ContentEntryNotAMapping(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{"string-not-map"},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content entry must be a mapping")
}

func TestParseInline_ContentMissingKind(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{"lang": "yaml"}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must set a `kind:` key")
}

func TestParseInline_ContentKindNotAString(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{"kind": 42}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kind must be a string")
}

func TestParseInline_ContentUnknownField(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": "code-block", "bogus": 1,
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown content key "bogus"`)
}

func TestParseInline_ContentLangWrongType(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": "code-block", "lang": 42,
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lang must be a string")
}

func TestParseInline_ContentColumnsWrongKind(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": "paragraph", "columns": []any{"A"},
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only valid on `kind: table`")
}

func TestParseInline_ContentColumnsNotAList(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": "table", "columns": "A,B",
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a list")
}

func TestParseInline_ContentOrderedWrongKind(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": "paragraph", "ordered": true,
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only valid on `kind: list`")
}

func TestParseInline_ContentOrderedWrongType(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": "list", "ordered": "yes",
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ordered must be a boolean")
}

func TestParseInline_ContentItemBoundWrongKind(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": "paragraph", "min-items": 2,
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only valid on `kind: list`")
}

func TestParseInline_ContentItemBoundNegative(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": "list", "min-items": -1,
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-negative")
}

func TestParseInline_ContentItemBoundFloat(t *testing.T) {
	// goyaml may decode integers as float64 — accept whole floats,
	// reject non-integer floats with a clear message.
	sch, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": "list", "min-items": float64(3),
			}},
		}},
	}, "kind x")
	require.NoError(t, err)
	require.Len(t, sch.Sections[0].Content, 1)
	assert.Equal(t, 3, sch.Sections[0].Content[0].MinItems)

	_, err = ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": "list", "min-items": 2.5,
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a non-negative integer")
}

func TestParseInline_ContentItemBoundInt64Negative(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": "list", "min-items": int64(-2),
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-negative")
}

func TestParseInline_ContentItemBoundWrongType(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": "list", "min-items": "two",
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an integer")
}

func TestParseInline_ContentItemBoundMinExceedsMax(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Steps",
			"content": []any{map[string]any{
				"kind": "list", "min-items": 5, "max-items": 2,
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(),
		"min-items=5 is greater than max-items=2")
}

func TestParseInline_ContentItemBoundInt64Overflow(t *testing.T) {
	// On 64-bit builds (the common case) int == int64 so the
	// overflow guard's int64 branch is unreachable from valid YAML.
	// Skip when int is already the same width as int64; the float
	// path test below exercises the equivalent guard for that type.
	maxInt := int64(math.MaxInt)
	if maxInt == math.MaxInt64 {
		t.Skip("int is 64-bit on this platform; overflow guard is unreachable")
	}
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Steps",
			"content": []any{map[string]any{
				"kind": "list", "min-items": maxInt + 1,
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds int range")
}

func TestParseInline_ContentItemBoundFloatOverflow(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Steps",
			"content": []any{map[string]any{
				"kind": "list", "max-items": float64(math.MaxInt64) * 2,
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	// Huge float values that aren't representable as int trip the
	// "not an integer" path before the platform overflow check; the
	// important thing is that the overflow does NOT silently wrap.
	assert.True(t,
		strings.Contains(err.Error(), "exceeds int range") ||
			strings.Contains(err.Error(), "must be a non-negative integer"),
		"want overflow or non-integer error, got %q", err.Error())
}

func TestParseInline_ContentItemBoundFloatNaN(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Steps",
			"content": []any{map[string]any{
				"kind": "list", "min-items": math.NaN(),
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a finite integer")
}

func TestParseInline_ContentItemBoundFloatInf(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Steps",
			"content": []any{map[string]any{
				"kind": "list", "min-items": math.Inf(1),
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a finite integer")
}

func TestParseInline_ContentItemBoundFloatHuge(t *testing.T) {
	// 1e100 is finite and integer-valued in float64 representation
	// (its mantissa exhausts long before the fractional bit) but it
	// dwarfs math.MaxInt. The check must reject it via the explicit
	// range comparison, not via the implementation-defined float->
	// int64 cast.
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Steps",
			"content": []any{map[string]any{
				"kind": "list", "min-items": 1e100,
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds int range")
}

func TestParseInline_ContentItemBoundFloatNegative(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Steps",
			"content": []any{map[string]any{
				"kind": "list", "min-items": float64(-3),
			}},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-negative")
}

func TestParseInline_ContentItemBoundInt64Accepted(t *testing.T) {
	sch, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{map[string]any{
				"kind": "list", "max-items": int64(7),
			}},
		}},
	}, "kind x")
	require.NoError(t, err)
	assert.Equal(t, 7, sch.Sections[0].Content[0].MaxItems)
}

// ---- Validate content: edge-case branches ----

func TestValidate_Content_NestedSectionRecurses(t *testing.T) {
	// A scope whose nested child carries a `content:` entry exercises
	// walkContentScopes' recursion branch and the deeper-level
	// content match.
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Parent",
			"sections": []any{map[string]any{
				"heading": "Child",
				"content": []any{
					map[string]any{"kind": "code-block", "lang": "go"},
				},
			}},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Parent\n\n### Child\n\nNo code block here.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message,
		`missing required content "code-block lang=go" inside ### Child`)
}

func TestValidate_Content_WildcardSiblingSkipped(t *testing.T) {
	// A slot scope sibling to a content-bearing scope must be skipped
	// by walkContentScopes without panicking.
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"regex":  ".+",
				"repeat": map[string]any{"min": 0},
			}},
			map[string]any{
				"heading": "Body",
				"content": []any{map[string]any{"kind": "paragraph"}},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n\n## Body\n\nA paragraph.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags)
}

func TestValidate_Content_HeadingLevelMismatchFallback(t *testing.T) {
	// Schema lists `## Body` (H2 at root) but doc emits `### Body`.
	// findContentMatchingHead's level-mismatch fallback must still
	// pair the scope so its content runs.
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"content": []any{
				map[string]any{"kind": "code-block", "lang": "yaml"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n### Body\n\nNo code block.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	// Heading-tree diagnostics first, then the content miss.
	var contentMiss bool
	for _, d := range diags {
		if strings.Contains(d.Message, "missing required content") {
			contentMiss = true
		}
	}
	assert.True(t, contentMiss,
		"level-mismatch fallback should still run content check")
}

func TestValidate_Content_TableEmptyHeaderTolerated(t *testing.T) {
	// A table entry with no `columns:` constraint accepts any header.
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"content": []any{map[string]any{"kind": "table"}},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Body\n\n| A | B |\n|---|---|\n| x | y |\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags)
}

func TestValidate_Content_ListMaxItems(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Steps",
			"content": []any{
				map[string]any{"kind": "list", "max-items": 1},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Steps\n\n- one\n- two\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message,
		"list has 2 items, required at most 1")
}

func TestValidate_Content_ParagraphMatches(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Notes",
			"content": []any{map[string]any{"kind": "paragraph"}},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Notes\n\nA paragraph.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags)
}

func TestValidate_Content_OptionalEntryDeferToLater(t *testing.T) {
	// An optional entry sees a later listed entry's match first. It
	// must NOT pre-claim the node out-of-order — the outer loop
	// should advance to the matching entry on its own iteration.
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"content": []any{
				map[string]any{"kind": "code-block", "required": false},
				map[string]any{"kind": "table"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Body\n\n| A | B |\n|---|---|\n| x | y |\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	// The optional code-block is absent, the table matches its slot.
	// No diagnostic should fire.
	assert.Empty(t, diags)
}

func TestValidate_Content_DescribeListWithBounds(t *testing.T) {
	// describeEntry should render every list constraint that fires.
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"content": []any{
				map[string]any{"kind": "list",
					"ordered": true, "min-items": 2, "max-items": 5},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Body\n\nNo list.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message,
		"list ordered=true min-items=2 max-items=5")
}

// describeNodeCase pins one expected describeNode rendering to the
// document body that should produce it. Used as a table for
// TestValidate_Content_DescribeNodeKinds.
type describeNodeCase struct {
	name string
	body string
	want string
}

var describeNodeCases = []describeNodeCase{
	{
		name: "code-block-no-lang",
		body: "Hello.\n\n```\nx\n```\n",
		want: `unexpected content "code-block"`,
	},
	{
		name: "code-block-with-lang",
		body: "Hello.\n\n```go\nfoo()\n```\n",
		want: `unexpected content "code-block lang=go"`,
	},
	{
		name: "table",
		body: "Hello.\n\n| A | B |\n|---|---|\n| x | y |\n",
		want: `unexpected content "table"`,
	},
	{
		name: "list-ordered",
		body: "Hello.\n\n1. one\n2. two\n",
		want: `unexpected content "list ordered=true"`,
	},
	{
		name: "list-unordered",
		body: "Hello.\n\n- a\n- b\n",
		want: `unexpected content "list ordered=false"`,
	},
}

// TestValidate_Content_ScopeWithoutMatchingHeading covers the
// matched<0 branch in walkContentScopes: a content-bearing scope
// whose heading is absent from the doc is silently skipped by the
// content walker (the missing-required-section diagnostic comes
// from the heading-tree walker).
func TestValidate_Content_ScopeWithoutMatchingHeading(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": map[string]any{
				"regex":  "Missing",
				"repeat": map[string]any{"min": 0, "max": 1},
			},
			"content": []any{
				map[string]any{"kind": "code-block", "lang": "yaml"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Doc has no `## Missing` heading at all.
	doc := newDocFile(t, "doc.md", "# T\n\nNo H2 sections.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	// Content validator emits nothing because the scope didn't match;
	// the optional scope produces no missing-section message either.
	assert.Empty(t, diags)
}

// TestValidate_Content_DescribeMissingTableAndParagraph covers
// describeEntry's table-with-columns and paragraph branches by
// surfacing them in missing-required diagnostics.
func TestValidate_Content_DescribeMissingTableAndParagraph(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"content": []any{
				map[string]any{"kind": "table",
					"columns": []any{"A", "B"}},
				map[string]any{"kind": "paragraph"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n\n## Body\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var sawTable, sawPara bool
	for _, d := range diags {
		if strings.Contains(d.Message, "table columns=[A B]") {
			sawTable = true
		}
		if strings.Contains(d.Message, `"paragraph" inside`) {
			sawPara = true
		}
	}
	assert.True(t, sawTable,
		"want describeEntry to render table columns")
	assert.True(t, sawPara,
		"want describeEntry to render paragraph")
}

// TestValidate_Content_ClosedScopeMidstreamUnexpected exercises the
// matchEntry unexpected-content branch — a closed scope, intervening
// node that matches neither the current entry nor any later listed
// entry, no open slot. The walker advances past it with a diagnostic.
func TestValidate_Content_ClosedScopeMidstreamUnexpected(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"closed":  true,
			"content": []any{
				map[string]any{"kind": "code-block"},
				map[string]any{"kind": "code-block"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Two code blocks bracket an unexpected paragraph; closed scope
	// must flag the paragraph mid-sequence.
	src := "# T\n\n## Body\n\n" +
		"```\nx\n```\n\n" +
		"An intruding paragraph between the two slots.\n\n" +
		"```\ny\n```\n"
	doc := newDocFile(t, "doc.md", src)
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var midstream bool
	for _, d := range diags {
		if strings.Contains(d.Message, `unexpected content "paragraph"`) {
			midstream = true
		}
	}
	assert.True(t, midstream,
		"closed scope must flag a non-matching node between two listed entries")
}

// TestValidate_Content_FencedBlockNoInfoString regresses a Copilot
// review on PR #285: blockLine() must anchor a fenced block whose
// info string is absent at the opening fence line, not at the first
// content line. Lint.FindFencedOpenLine does the right thing — this
// test pins blockLine to that contract by relying on blocksInRange
// filtering, which would drop the block if the line landed inside
// the section body but past the section's end (or before its
// start).
func TestValidate_Content_FencedBlockNoInfoString(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"content": []any{map[string]any{"kind": "code-block"}},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Code block has no info string. The opening fence sits on line
	// 5; the first content line is line 6. If blockLine returned
	// line 6, blocksInRange would still include the block, but the
	// diagnostic for an unexpected-content-style mismatch would
	// anchor one line off. We assert no diagnostic fires here — the
	// block is recognised at its real position inside Body.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Body\n\n```\nbody\n```\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags,
		"a fenced block without info string must still be recognised inside the section")
}

// TestValidate_Content_ConcurrentSafety regresses a Copilot
// review on PR #285: the content walker reuses a goldmark
// parser.Parser from a pool, and parser.Parser is documented to
// be safe only when reused sequentially per goroutine. Running
// Validate across many goroutines under -race must not surface a
// data race or panic.
func TestValidate_Content_ConcurrentSafety(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"content": []any{
				map[string]any{"kind": "code-block", "lang": "yaml"},
				map[string]any{"kind": "paragraph"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	src := "# T\n\n## Body\n\n```yaml\nfoo: bar\n```\n\nA paragraph.\n"
	const goroutines = 16
	const iterations = 8
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				doc := newDocFile(t, "doc.md", src)
				diags := Validate(doc, sch, nil, false, makeDiagForTest)
				if len(diags) != 0 {
					t.Errorf("unexpected diagnostics: %v", diagsMessages(diags))
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestValidate_Content_NoLineZeroDiagnostics regresses the
// position-inference contract: blockLine may return 0 for a
// position-less empty fenced block, but topLevelBlocks back-fills
// that to a sibling-derived line. No content diagnostic should
// anchor at a non-existent line.
func TestValidate_Content_NoLineZeroDiagnostics(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"closed":  true,
			"content": []any{map[string]any{"kind": "list"}},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Empty fenced block + trailing paragraph under a closed scope
	// expecting a list. Several diagnostics will fire; every one
	// must anchor on a real source line.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Body\n\n```\n```\n\nTrailing.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	for _, d := range diags {
		assert.GreaterOrEqual(t, d.Line, 1,
			"diagnostic %q must not anchor at line 0", d.Message)
	}
}

// TestValidate_Content_EmptyFencedBlockClaimsSlot regresses a
// Copilot review on PR #285: an empty fenced code block with no
// info string and no content lines must still participate in
// section-body filtering. The prior clamp-to-1 implementation
// dropped it from blocksInRange, leading to spurious missing-
// required-content diagnostics. Position-inference in
// topLevelBlocks anchors the block inside its section so the
// `kind: code-block` entry can claim it.
func TestValidate_Content_EmptyFencedBlockClaimsSlot(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"content": []any{map[string]any{"kind": "code-block"}},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Body\n\n```\n```\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags,
		"an empty fenced block must claim the code-block slot in its section")
}

// TestValidate_Content_DescribeNodeFallback covers describeNode's
// default branch: a top-level block node we don't enumerate (e.g.
// a Blockquote) renders via n.Kind().String() rather than matching
// one of the kind-specific cases.
func TestValidate_Content_DescribeNodeFallback(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"closed":  true,
			"content": []any{map[string]any{"kind": "paragraph"}},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// A blockquote at top level is neither a paragraph nor any of
	// the recognised kinds; the closed scope must surface it as
	// unexpected content via describeNode's fallback.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Body\n\n> blockquoted line\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var sawFallback bool
	for _, d := range diags {
		if strings.Contains(d.Message, `unexpected content "Blockquote"`) ||
			strings.Contains(d.Message, `unexpected content "Block"`) {
			sawFallback = true
		}
	}
	assert.True(t, sawFallback,
		"unrecognised node kinds must surface through describeNode's fallback; got %v",
		diagsMessages(diags))
}

// TestValidate_Content_FindLaterEntrySkipsClaimed covers
// findLaterEntry's claimed-skip branch. With three required listed
// entries and out-of-order doc arrivals, the first doc node claims
// the second listed entry; a later findLaterEntry call must skip
// that already-claimed slot when searching for the next match.
func TestValidate_Content_FindLaterEntrySkipsClaimed(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"content": []any{
				map[string]any{"kind": "code-block"},
				map[string]any{"kind": "table"},
				map[string]any{"kind": "list"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Doc: table, then list, then code-block. Walker on code-block
	// (entry 0) sees the table — out-of-order claims entry 1; then
	// sees the list — findLaterEntry(1, list) must skip the now-
	// claimed entry 1 and find entry 2.
	src := "# T\n\n## Body\n\n" +
		"| A | B |\n|---|---|\n| x | y |\n\n" +
		"- one\n- two\n\n" +
		"```\nx\n```\n"
	doc := newDocFile(t, "doc.md", src)
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	// Both table and list arrive out of order; both should be
	// claimed without the code-block flagging as missing.
	var ooCount int
	var missing bool
	for _, d := range diags {
		if strings.Contains(d.Message, "out of order") {
			ooCount++
		}
		if strings.Contains(d.Message, "missing required") {
			missing = true
		}
	}
	assert.Equal(t, 2, ooCount,
		"both out-of-order nodes should produce a diagnostic")
	assert.False(t, missing,
		"code-block should be claimed at the end, not reported missing")
}

// TestValidate_Content_TableColumnsCountMismatch covers
// stringSlicesEqual's length-difference branch. A doc table with
// more columns than required fails the slice-equality check on
// length before any element comparison runs.
func TestValidate_Content_TableColumnsCountMismatch(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Settings",
			"content": []any{
				map[string]any{"kind": "table",
					"columns": []any{"Setting", "Default"}},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Settings\n\n"+
			"| Setting | Default | Notes |\n"+
			"|---------|---------|-------|\n"+
			"| foo     | 1       | n/a   |\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message,
		"do not match required")
}

// TestValidate_Content_MissingPlainTable covers describeEntry's
// `table` branch with no `columns:` set — the empty-columns return
// path that "table columns=[A B]" tests do not.
func TestValidate_Content_MissingPlainTable(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"content": []any{map[string]any{"kind": "table"}},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n\n## Body\n\nNo table.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message,
		`missing required content "table"`)
}

// TestValidate_Content_NestedHeadingsInsideSection covers
// contentScopeEndLine's deeper-heading branch: nested headings
// stay inside the section, a sibling at the matched level
// terminates it.
func TestValidate_Content_NestedHeadingsInsideSection(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"sections": []any{map[string]any{
				"heading": "Sub",
				"content": []any{map[string]any{"kind": "code-block"}},
			}},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// `## Body` contains `### Sub`, then another `## Trailing` at
	// the matched level. The nested Sub scope's range should end
	// before `## Trailing` even though the heading tree has multiple
	// levels in between.
	src := "# T\n\n" +
		"## Body\n\n### Sub\n\n```\nx\n```\n\n" +
		"## Trailing\n\nUnrelated.\n"
	doc := newDocFile(t, "doc.md", src)
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags,
		"sub-section's code block must be claimed inside Body's range")
}

// TestValidate_Content_DescribeNodeKinds exercises describeNode for
// every node kind — the closed-scope "unexpected content" message
// embeds describeNode's output for the offending block.
func TestValidate_Content_DescribeNodeKinds(t *testing.T) {
	for _, tc := range describeNodeCases {
		t.Run(tc.name, func(t *testing.T) {
			runDescribeNodeCase(t, tc)
		})
	}
}

func runDescribeNodeCase(t *testing.T, tc describeNodeCase) {
	t.Helper()
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"closed":  true,
			"content": []any{map[string]any{"kind": "paragraph"}},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n\n## Body\n\n"+tc.body)
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var found bool
	for _, d := range diags {
		if strings.Contains(d.Message, tc.want) {
			found = true
		}
	}
	assert.True(t, found,
		"want diagnostic containing %q, got %v",
		tc.want, diagsMessages(diags))
}

// TestValidate_Content_TableHeaderWithCodeSpan regresses a Copilot
// review on PR #285: inline code spans (and other non-Text inline
// nodes) inside table header cells used to drop their text content,
// causing false column-mismatch diagnostics even when the rendered
// header matched. mdtext.ExtractPlainText preserves the labels.
func TestValidate_Content_TableHeaderWithCodeSpan(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Settings",
			"content": []any{
				map[string]any{"kind": "table",
					"columns": []any{"Setting", "Default"}},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// The header cells use code spans around the labels. The walker
	// must still extract `Setting` and `Default` and treat the table
	// as matching.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Settings\n\n"+
			"| `Setting` | `Default` |\n"+
			"|-----------|-----------|\n"+
			"| timeout   | 30s       |\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags,
		"code-span table headers must round-trip through plain-text extraction")
}

func TestValidate_Content_FindLaterEntrySkipsUnlistedSlot(t *testing.T) {
	// findLaterEntry must skip unlisted slots when searching for an
	// out-of-order match. Place an unlisted slot between two listed
	// entries; a doc node matching the later listed entry should be
	// claimed as out-of-order, not absorbed by the slot.
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"content": []any{
				map[string]any{"kind": "code-block", "lang": "yaml"},
				map[string]any{"kind": "unlisted"},
				map[string]any{"kind": "table"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Body\n\n"+
			"| A | B |\n|---|---|\n| x | y |\n\n"+
			"```yaml\nfoo: bar\n```\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var ooo bool
	for _, d := range diags {
		if strings.Contains(d.Message, "out of order") {
			ooo = true
		}
	}
	assert.True(t, ooo,
		"unlisted slot must not shadow later listed entries for out-of-order detection")
}

func TestValidate_Content_PreambleAtEndOfDocument(t *testing.T) {
	// firstContentHeadingLine returns parentEnd when no heading
	// follows the preamble in its window — exercise that path with a
	// doc that has no subsequent H2 sections.
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": nil,
				"content": []any{
					map[string]any{"kind": "code-block", "lang": "yaml"},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\nPreamble prose only — no following H2.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "missing required content")
}

func TestValidate_Content_DescribeUnlistedEntry(t *testing.T) {
	// describeEntry covers `unlisted` indirectly via diagnostic
	// shaping; this confirms a closed scope with a slot still
	// renders properly.
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"closed":  true,
			"content": []any{
				map[string]any{"kind": "unlisted"},
				map[string]any{"kind": "code-block"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Body\n\nA paragraph then nothing else.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message,
		`missing required content "code-block" inside ## Body`)
}

// TestValidate_Content_TolerateDirectivesInOpenScope regresses a
// Copilot review on PR #285: the content walker re-parses the
// document with the GFM table extension, and that parser must also
// register the PI block parser so `<?include?>`/`<?catalog?>`
// directives appear as ProcessingInstruction nodes — not as HTML
// blocks the walker might misclassify in a closed scope.
func TestValidate_Content_TolerateDirectivesInOpenScope(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Body",
			"content": []any{
				map[string]any{"kind": "code-block"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Directives surround the required code block. Under the open
	// scope they must not flag — the regression is that the table-
	// enabled parser would have parsed `<?include?>` as an HTML
	// block, leaving the walker to treat it as some other AST kind.
	src := "# T\n\n## Body\n\n" +
		"<?catalog\nsource-dir: \".\"\nglob: [\"*.md\"]\n?>\n" +
		"- generated\n" +
		"<?/catalog?>\n\n" +
		"```\nx\n```\n"
	doc := newDocFile(t, "doc.md", src)
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags,
		"directives must not be misclassified as unexpected content")
}
