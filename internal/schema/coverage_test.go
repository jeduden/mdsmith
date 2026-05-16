package schema

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// ---- ParseInline edge cases ----

func TestParseInline_RejectsMissingHeadingKey(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"closed": true},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must set a `heading:` key")
}

func TestParseInline_RejectsBlankHeading(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "   "},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty string")
}

func TestParseInline_AcceptsScopeRulesMapping(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Decision",
				"rules": map[string]any{
					"paragraph-readability": map[string]any{
						"max-index": 12.0,
					},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1)
	require.Contains(t, sch.Sections[0].Rules, "paragraph-readability")
}

func TestParseInline_FrontmatterExprAcceptsScalars(t *testing.T) {
	// Scalars (bool/number) become JSON-encoded CUE constants —
	// this exercises the frontmatterExpr non-string branches.
	raw := map[string]any{
		"frontmatter": map[string]any{
			"active":  true,
			"version": 1,
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	cue := sch.FrontmatterCUE()
	assert.Contains(t, cue, "active: true")
	assert.Contains(t, cue, "version: 1")
}

// ---- ParseInline error paths ----

func TestParseInline_RejectsBadStringEntry(t *testing.T) {
	// Bare strings are no longer accepted as section entries; the
	// section list is uniformly mappings now. Slots must use
	// `heading: {unlisted: true}` and preambles use `heading: null`.
	raw := map[string]any{
		"sections": []any{"any-string"},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scope must be a mapping")
}

func TestParseInline_RejectsBadScopeType(t *testing.T) {
	raw := map[string]any{"sections": []any{42}}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scope must be a mapping")
}

func TestParseInline_RejectsBadSectionsType(t *testing.T) {
	raw := map[string]any{"sections": "not-a-list"}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sections must be a list")
}

func TestParseInline_RejectsBadFrontmatterType(t *testing.T) {
	raw := map[string]any{"frontmatter": []any{"not-a-map"}}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "frontmatter must be a mapping")
}

func TestParseInline_RejectsLegacyRequireKey(t *testing.T) {
	// Plan 156 removed the `require:` wrapper; the parser rejects
	// the key with a migration-pointing diagnostic.
	raw := map[string]any{
		"require": map[string]any{"filename": "RFC-*.md"},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "`require:` removed")
	assert.Contains(t, err.Error(), "filename:")
}

func TestParseInline_RejectsBadFilenameType(t *testing.T) {
	raw := map[string]any{"filename": 42}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "filename must be a string")
}

func TestParseInline_RejectsBadClosedType(t *testing.T) {
	raw := map[string]any{"closed": "true"}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed must be a boolean")
}

func TestParseInline_RejectsBadHeadingType(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{"heading": 42}},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "heading must be a string")
}

func TestParseInline_RejectsLegacyRequiredKey(t *testing.T) {
	// Plan 156 removed `required:`; the rejection diagnostic
	// points at the `repeat:` replacement.
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "X", "required": false,
		}},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "`required:` removed")
	assert.Contains(t, err.Error(), "repeat:")
}

func TestParseInline_RejectsLegacyAliasesKey(t *testing.T) {
	// Plan 156 removed `aliases:`; encode disjunction in the regex.
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "X", "aliases": []any{"Y"},
		}},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "`aliases:` removed")
	assert.Contains(t, err.Error(), "regex:")
}

func TestParseInline_RejectsLegacyRepeatingPatternKeys(t *testing.T) {
	// Plan 156 removed the scope-level `repeats:` / `sequential:`
	// / `min:` / `max:` keys; each one carries its own pointer
	// at the replacement.
	cases := []struct {
		key string
		val any
	}{
		{"repeats", true},
		{"sequential", true},
		{"min", 1},
		{"max", 5},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			raw := map[string]any{
				"sections": []any{map[string]any{
					"heading": "X", tc.key: tc.val,
				}},
			}
			_, err := ParseInline(raw, "kind x")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.key)
			assert.Contains(t, err.Error(), "removed; see plan 156")
		})
	}
}

func TestParseInline_RejectsBadRulesType(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "X", "rules": "not-a-map",
		}},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rules must be a mapping")
}

func TestParseInline_RejectsBadRuleEntryType(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "X",
			"rules":   map[string]any{"line-length": "bad"},
		}},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rules.line-length must be a mapping")
}

func TestFrontmatterExpr_RejectsUnsupportedType(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"odd": struct{ Foo string }{Foo: "bar"},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported value type")
}

// ---- ParseFile include expansion ----

func TestParseFile_ExpandsInclude(t *testing.T) {
	dir := t.TempDir()
	// Fragment to include.
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "frag.md"),
		[]byte("## Tasks\n"), 0o644))
	main := writeFile(t, dir, "proto.md",
		"# ?\n\n## Goal\n\n<?include\nfile: frag.md\n?>\n")
	sch, err := ParseFile(&FileReader{}, main)
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1)
	children := sch.Sections[0].Sections
	require.Len(t, children, 2, "include should splice Tasks after Goal")
	assert.Equal(t, "Goal", children[0].Heading)
	assert.Equal(t, "Tasks", children[1].Heading)
}

func TestParseFile_RejectsAbsoluteIncludePath(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"# ?\n\n<?include\nfile: /etc/passwd\n?>\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute file path")
}

func TestParseFile_RejectsTraversalInIncludePath(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"# ?\n\n<?include\nfile: ../leak.md\n?>\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `".."`)
}

func TestParseFile_DetectsIncludeCycle(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "a.md"),
		[]byte("<?include\nfile: b.md\n?>\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "b.md"),
		[]byte("<?include\nfile: a.md\n?>\n"), 0o644))
	_, err := ParseFile(&FileReader{}, filepath.Join(dir, "a.md"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cyclic include")
}

func TestParseFile_MissingFileReturnsError(t *testing.T) {
	_, err := ParseFile(&FileReader{}, "/nonexistent/path/to/schema.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read schema")
}

func TestParseFile_NilReaderUsesOS(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "# ?\n\n## Goal\n")
	sch, err := ParseFile(nil, p)
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1)
}

func TestParseFile_InvalidFrontmatter(t *testing.T) {
	dir := t.TempDir()
	// A frontmatter value that fails frontmatterExpr (empty string).
	p := writeFile(t, dir, "proto.md", "---\nid: ''\n---\n# ?\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty")
}

func TestParseFile_IncludeMissingFileParam(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"# ?\n\n<?include\n?>\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required 'file' attribute")
}

func TestParseFile_IncludeMissingFile(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"# ?\n\n<?include\nfile: nope.md\n?>\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read schema include")
}

func TestParseFile_RequireSingleLine(t *testing.T) {
	// Exercises the single-line PI body branch in piYAMLBody.
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"<?require filename: \"plan-*.md\" ?>\n\n# ?\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	assert.Equal(t, "plan-*.md", sch.Filename)
}

func TestParseFile_RequireMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"<?require\nfilename: [unterminated\n?>\n\n# ?\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid <?require?>")
}

func TestParseFile_FrontmatterPropagatesToSchema(t *testing.T) {
	// Frontmatter CUE constraints declared in the proto.md surface
	// on the parsed Schema. lint.StripFrontMatter consumes the
	// "---\n…---\n" delimiters before stripDelimiters runs.
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"---\nid: 'string'\n---\n# ?\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	assert.Equal(t, "string", sch.Frontmatter["id"])
}

func TestParseFile_IncludeFragmentWithFilename(t *testing.T) {
	// A fragment that itself carries a <?require?> propagates the
	// filename pattern up to the host schema. Exercises the
	// fragment-fp branch in expandInclude / parseFileBytes.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "frag.md"),
		[]byte("<?require\nfilename: \"frag-*.md\"\n?>\n\n## Tasks\n"),
		0o644))
	p := writeFile(t, dir, "proto.md",
		"# ?\n\n<?include\nfile: frag.md\n?>\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	assert.Equal(t, "frag-*.md", sch.Filename,
		"fragment's filename pattern should win when host has none")
}

func TestParseFile_HostFilenameBeatsIncludeFilename(t *testing.T) {
	// When the host schema declares a filename, the fragment's
	// filename is ignored — covers the "fp != \"\" && cfg.Filename
	// == \"\"" guard.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "frag.md"),
		[]byte("<?require\nfilename: \"frag-*.md\"\n?>\n\n## Tasks\n"),
		0o644))
	p := writeFile(t, dir, "proto.md",
		"<?require\nfilename: \"plan-*.md\"\n?>\n\n# ?\n\n<?include\nfile: frag.md\n?>\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	assert.Equal(t, "plan-*.md", sch.Filename)
}

func TestParseFile_HeadingWithCodeSpan(t *testing.T) {
	// Exercises writeNodeText's CodeSpan and recursive-child
	// branches by giving a heading inline code.
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"# `id` Title\n\n## Goal\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1)
	// The heading text should include the inline code contents.
	assert.Contains(t, sch.Sections[0].Heading, "id")
}

func TestParseFile_RootFSRejectsAbsolute(t *testing.T) {
	r := &FileReader{RootFS: os.DirFS(t.TempDir())}
	_, err := ParseFile(r, "/absolute/path.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute schema path not allowed")
}

func TestParseFile_RootFSRejectsTraversal(t *testing.T) {
	r := &FileReader{RootFS: os.DirFS(t.TempDir())}
	_, err := ParseFile(r, "../escape.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes project root")
}

func TestParseFile_RootFSReadsRelativePath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "proto.md"),
		[]byte("# ?\n\n## Goal\n"), 0o644))
	r := &FileReader{RootFS: os.DirFS(dir)}
	sch, err := ParseFile(r, "proto.md")
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1)
}

func TestSchema_IsEmpty(t *testing.T) {
	assert.True(t, (*Schema)(nil).IsEmpty())
	assert.True(t, (&Schema{}).IsEmpty())
	assert.False(t, (&Schema{Sections: []Scope{{Heading: "X"}}}).IsEmpty())
	assert.False(t, (&Schema{Filename: "*.md"}).IsEmpty())
	assert.False(t, (&Schema{Frontmatter: map[string]string{"id": "string"}}).IsEmpty())
}

func TestSchema_EffectiveRootLevel(t *testing.T) {
	assert.Equal(t, 2, (*Schema)(nil).EffectiveRootLevel())
	assert.Equal(t, 2, (&Schema{}).EffectiveRootLevel())
	assert.Equal(t, 1, (&Schema{RootLevel: 1}).EffectiveRootLevel())
	assert.Equal(t, 3, (&Schema{RootLevel: 3}).EffectiveRootLevel())
}

func TestParseInline_QuotedFrontmatterKey(t *testing.T) {
	// Keys that aren't bare CUE identifiers must be quoted in the
	// emitted CUE struct. This exercises cueFieldLabel + isCUEIdent
	// for the quoted branch.
	raw := map[string]any{
		"frontmatter": map[string]any{
			"my-key?": `string`,
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	cue := sch.FrontmatterCUE()
	assert.Contains(t, cue, `"my-key"?: string`)
}

// ---- ValidateFrontmatterSyntax ----

func TestValidateFrontmatterSyntax_AcceptsEmpty(t *testing.T) {
	require.NoError(t, ValidateFrontmatterSyntax(&Schema{}))
}

func TestValidateFrontmatterSyntax_AcceptsValid(t *testing.T) {
	sch := &Schema{Frontmatter: map[string]string{
		"id": `=~"^RFC-[0-9]{4}$"`,
	}}
	require.NoError(t, ValidateFrontmatterSyntax(sch))
}

func TestValidateFrontmatterSyntax_RejectsInvalidCUE(t *testing.T) {
	sch := &Schema{Frontmatter: map[string]string{
		"id": "int &",
	}}
	err := ValidateFrontmatterSyntax(sch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid schema frontmatter CUE")
}

// ---- Field-interpolated heading matching ----

func TestValidate_FieldInterpolatedHeadingMatches(t *testing.T) {
	// `# {id}: {name}` against `# MDS001: line-length` should match
	// once the document's front matter supplies the field values
	// the fmvar() helper resolves against.
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "# {id}: {name}\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# MDS001: line-length\n")
	diags := Validate(doc, sch,
		map[string]any{"id": "MDS001", "name": "line-length"},
		false, makeDiagForTest)
	assert.Empty(t, diags,
		"field-interpolated H1 pattern should match the concrete title")
}

func TestValidate_FieldInterpolatedHeadingMismatch(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"# ?\n\n## Step {n}\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# Plan\n\n## Wrong heading\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags,
		"non-matching text should still trigger structural diagnostics")
}

// ---- frontmatterExpr branch coverage ----

func TestParseInline_FrontmatterMapValue(t *testing.T) {
	// Map-valued frontmatter constraints get JSON-encoded by
	// frontmatterExpr — exercise that branch.
	raw := map[string]any{
		"frontmatter": map[string]any{
			"meta": map[string]any{"version": 1},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	assert.Contains(t, sch.Frontmatter["meta"], "version")
}

func TestParseInline_FrontmatterListValue(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"tags": []any{"draft", "internal"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	assert.Contains(t, sch.Frontmatter["tags"], "draft")
}

func TestParseInline_FrontmatterEmptyString(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{"id": ""},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty")
}

// ---- Validate edge cases ----

func TestMatchesHeading_Exported(t *testing.T) {
	// Exported wrapper used by the per-scope-rule walker.
	sc := literalScope("Goal")
	assert.True(t, MatchesHeading(sc, DocHeading{Text: "Goal", Level: 2}, nil))
	assert.False(t, MatchesHeading(sc, DocHeading{Text: "Other", Level: 2}, nil))
	// Slot scopes never match a specific heading.
	assert.False(t, MatchesHeading(slotScope(), DocHeading{Text: "Anything"}, nil))
	// A `.+` matcher (one-or-more wildcard) matches any text.
	wild := Scope{Heading: ".+", Matcher: &Matcher{Regex: ".+"}}
	assert.True(t, MatchesHeading(wild, DocHeading{Text: "Anything"}, nil))
	// Disjunction inside the regex replaces the old aliases field.
	sc2 := Scope{Heading: "Symptoms|Indicators",
		Matcher: &Matcher{Regex: "Symptoms|Indicators"}}
	assert.True(t, MatchesHeading(sc2, DocHeading{Text: "Indicators"}, nil))
	// fmvar() patterns now resolve against the supplied frontmatter
	// when the caller passes it in.
	sc3 := Scope{Heading: "fmvar",
		Matcher: &Matcher{Regex: `\#(fmvar(id))`}}
	assert.True(t, MatchesHeading(sc3, DocHeading{Text: "X-42"},
		map[string]any{"id": "X-42"}))
	assert.False(t, MatchesHeading(sc3, DocHeading{Text: "X-42"}, nil),
		"fmvar with nil FM must not match a non-empty heading text")
}

func TestMatcherCache_ReusesCompiled(t *testing.T) {
	// Two calls with the same pattern must hit the cache the second
	// time. Cover both the cache-miss and cache-hit branches.
	m := &Matcher{Regex: `Step \#(digits)`}
	first, err := cachedMatcher(m, nil)
	require.NoError(t, err)
	second, err := cachedMatcher(m, nil)
	require.NoError(t, err)
	assert.Same(t, first, second,
		"second call must return the cached compiled regex")
}

func TestValidate_NilSchemaShortCircuits(t *testing.T) {
	doc := newDocFile(t, "doc.md", "# T\n")
	assert.Empty(t, Validate(doc, nil, nil, false, makeDiagForTest))
	assert.Empty(t, Validate(doc, &Schema{}, nil, false, makeDiagForTest))
}

func TestValidate_OutOfOrderWithNestedSections(t *testing.T) {
	// Exercises claimOutOfOrder's recursion branch: when a doc
	// heading matches a later listed scope and that scope has
	// nested sections, the children must still be validated.
	raw := map[string]any{
		"closed": true,
		"sections": []any{
			map[string]any{"heading": "Goal"},
			map[string]any{
				"heading": "Tasks",
				"sections": []any{
					map[string]any{"heading": "Step A"},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind plan")
	require.NoError(t, err)
	// Tasks appears first (out-of-order); its Step A child still
	// validates within Tasks.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Tasks\n\n### Step A\n\nx\n\n## Goal\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags)
	// Expect the out-of-order diagnostic but no "missing Step A".
	var found bool
	for _, d := range diags {
		if strings.Contains(d.Message, `## Tasks: got <out of order>`) &&
			strings.Contains(d.Message, `expected after "## Goal"`) {
			found = true
		}
		assert.NotContains(t, d.Message, "Step A",
			"Step A should have been claimed inside out-of-order Tasks")
	}
	assert.True(t, found, "expected the Tasks out-of-order diagnostic")
}

func TestValidateFrontmatter_AcceptsEmptyConstraints(t *testing.T) {
	sch := &Schema{}
	assert.NoError(t, ValidateFrontmatter(sch, map[string]any{"id": "x"}))
}

func TestValidateFrontmatter_InvalidCUERejects(t *testing.T) {
	// matchesText with a malformed pattern should not panic; the
	// CUE compile path here exercises ValidateFrontmatter's error
	// branch on a bad CUE expression.
	sch := &Schema{Frontmatter: map[string]string{"id": "int &"}}
	err := ValidateFrontmatter(sch, map[string]any{"id": "x"})
	require.Error(t, err)
}

func TestValidate_ShallowLevelMismatchClaimedByText(t *testing.T) {
	// A scope at H3 matches a doc heading at H2 (shallower) by
	// text. Without the shallower-match branch in matchScope, this
	// would cascade into "missing required" + "unexpected" pair.
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Diagnosis",
				"sections": []any{
					map[string]any{"heading": "Step"},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind runbook")
	require.NoError(t, err)
	// Doc has Step at H2 instead of H3.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Diagnosis\n\n## Step\n\nx\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var levelDiag bool
	for _, d := range diags {
		if strings.Contains(d.Message, `Step: got h2, expected h3`) {
			levelDiag = true
		}
		assert.NotContains(t, d.Message, "expected section to be present",
			"shallow-match should claim the scope, not leave it missing")
	}
	assert.True(t, levelDiag,
		"expected a level-mismatch diagnostic for shallow Step")
}

func TestValidate_DeeperOrphanConsumedSilently(t *testing.T) {
	// A deeper heading whose text matches nothing should be skipped
	// silently (covers matchScope's dh.Level > expectedLevel
	// branch in the no-out-of-order path).
	raw := map[string]any{
		"closed": true,
		"sections": []any{
			map[string]any{"heading": "Goal"},
		},
	}
	sch, err := ParseInline(raw, "kind plan")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Goal\n\n### Orphan\n\nx\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags,
		"deeper orphan heading should be consumed silently")
}

func TestValidate_InvalidFilenamePattern(t *testing.T) {
	// A pattern that filepath.Match rejects (e.g., unmatched
	// bracket) surfaces as a diagnostic at the document level.
	raw := map[string]any{
		"filename": "[unterminated",
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "filename pattern:")
	assert.Contains(t, diags[0].Message, "expected valid glob")
}

func TestValidateFrontmatter_HandlesNilFM(t *testing.T) {
	sch := &Schema{Frontmatter: map[string]string{
		"id?": "string",
	}}
	// nil fm is normalised to an empty map; with optional fields
	// the schema still validates.
	assert.NoError(t, ValidateFrontmatter(sch, nil))
}

func TestParseFile_EmptySchemaYieldsNoSections(t *testing.T) {
	// A proto.md with no headings at all hits the rootLevel==0
	// branch in headingsToScopes.
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "Just prose, no headings.\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	assert.Empty(t, sch.Sections)
	assert.Equal(t, 2, sch.RootLevel)
}

func TestParseFile_HeadingWithEmphasis(t *testing.T) {
	// Heading with **strong** content exercises writeNodeText's
	// recursive child branch (neither Text nor CodeSpan).
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "# **Bold** Title\n\n## Goal\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1)
	assert.Contains(t, sch.Sections[0].Heading, "Bold")
}

func TestParseFile_IncludeEmptyFileParam(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"# ?\n\n<?include\nfile: \"\"\n?>\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required 'file' attribute")
}

func TestParseFile_IncludeMalformedYAMLDirective(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"# ?\n\n<?include\nfile: [unterminated\n?>\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid include directive YAML")
}

func TestStripDelimiters_HappyPath(t *testing.T) {
	// stripDelimiters matches the exact "---\n…---\n" shape that
	// lint.StripFrontMatter feeds it.
	got := stripDelimiters([]byte("---\nfoo: 1\n---\n"))
	assert.Equal(t, "foo: 1\n", string(got))
}

// TestStripDelimiters_BlockScalarFenceSequence regresses a
// Copilot review observation: a YAML block-scalar value can
// contain the literal `---\n` sequence inside its body. The
// earlier strings.Index search truncated at the first match;
// TrimSuffix on the canonical closing fence preserves the
// entire body.
func TestStripDelimiters_BlockScalarFenceSequence(t *testing.T) {
	in := []byte(
		"---\n" +
			"id: 1\n" +
			"notes: |\n" +
			"  ---\n" +
			"  more text\n" +
			"status: open\n" +
			"---\n")
	want := "id: 1\nnotes: |\n  ---\n  more text\nstatus: open\n"
	assert.Equal(t, want, string(stripDelimiters(in)))
}

func TestParseFile_FrontmatterEmptyBody(t *testing.T) {
	// "---\n---\n" yields empty content between delimiters; the
	// parser should accept it as an empty (no constraints) FM.
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "---\n---\n# ?\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	assert.Empty(t, sch.Frontmatter)
}

func TestParseFile_FrontmatterMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"---\nid: [unterminated\n---\n# ?\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing schema frontmatter")
}

func TestParseFile_IncludeMaxDepthExceeded(t *testing.T) {
	// Eleven nested files (a -> b -> ... -> k) push the chain
	// length past maxIncludeDepth (10).
	dir := t.TempDir()
	const n = 12
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("f%d.md", i)
		var body string
		if i+1 < n {
			body = fmt.Sprintf("<?include\nfile: f%d.md\n?>\n", i+1)
		} else {
			body = "## Tail\n"
		}
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, name), []byte(body), 0o644))
	}
	_, err := ParseFile(&FileReader{},
		filepath.Join(dir, "f0.md"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "include depth exceeds maximum")
}

func TestParseInline_NilRawReturnsEmpty(t *testing.T) {
	// Nil raw map exercises the early-return branch.
	sch, err := ParseInline(nil, "kind x")
	require.NoError(t, err)
	require.NotNil(t, sch)
	assert.True(t, sch.IsEmpty())
	assert.Equal(t, 2, sch.RootLevel)
}

func TestParseInline_NestedSectionsRejectsBadType(t *testing.T) {
	// Inner sections list with an invalid scope; exercises the
	// error propagation through setScopeSections.
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading":  "Parent",
				"sections": []any{42},
			},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scope must be a mapping")
}

func TestIsCUEIdent_EmptyAndDigitFirst(t *testing.T) {
	// Empty string and digit-leading strings are not valid CUE
	// identifiers; cueFieldLabel quotes them.
	assert.False(t, isCUEIdent(""))
	assert.False(t, isCUEIdent("1foo"))
	assert.False(t, isCUEIdent("foo-bar"))
	assert.True(t, isCUEIdent("foo_bar"))
	assert.True(t, isCUEIdent("foo123"))
}

// TestValidate_OutOfOrderHonoursPlaceholderHeadings: out-of-order
// detection sees scopes whose matcher uses fmvar() interpolation
// once the document supplies the fields.
func TestValidate_OutOfOrderHonoursPlaceholderHeadings(t *testing.T) {
	dir := t.TempDir()
	// File-based schema with two scopes: literal "Goal" and
	// field-interpolated "{id}: {name}". Doc has them reversed.
	p := writeFile(t, dir, "proto.md",
		"# ?\n\n## Goal\n\n## {id}: {name}\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## MDS001: line-length\n\nx\n\n## Goal\n\ny\n")
	diags := Validate(doc, sch,
		map[string]any{"id": "MDS001", "name": "line-length"},
		false, makeDiagForTest)
	var oo bool
	var missing bool
	for _, d := range diags {
		if strings.Contains(d.Message, `## MDS001: line-length: got <out of order>`) &&
			strings.Contains(d.Message, `expected after "## Goal"`) {
			oo = true
		}
		if strings.Contains(d.Message, `## {id}: {name}: got <missing>`) {
			missing = true
		}
	}
	assert.True(t, oo,
		"placeholder-pattern scope must participate in out-of-order detection")
	assert.False(t, missing,
		"placeholder scope must not also surface as missing once claimed")
}

// TestValidate_LateListedScopeRecursesIntoChildren regresses the
// case where the trailing loop claims a late-arriving listed scope
// — it must also validate that scope's nested required sections so
// missing children still surface as diagnostics. The optional B is
// the trigger: matchScope skips it on the way past (optional-skip
// shortcut), so B only surfaces via the trailing loop.
func TestValidate_LateListedScopeRecursesIntoChildren(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "A"},
			map[string]any{
				"heading": map[string]any{
					"regex":  "B",
					"repeat": map[string]any{"min": 0, "max": 1},
				},
				"sections": []any{
					map[string]any{"heading": "B-child"},
				},
			},
			map[string]any{"heading": "C"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Doc: A, C, then late B without its B-child.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## A\n\nx\n\n## C\n\ny\n\n## B\n\nz\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var oo, missing bool
	for _, d := range diags {
		if strings.Contains(d.Message, `## B: got <out of order>`) &&
			strings.Contains(d.Message, "expected before this position") {
			oo = true
		}
		if strings.Contains(d.Message, `### B-child: got <missing>`) {
			missing = true
		}
	}
	assert.True(t, oo, "late optional B should be flagged out-of-order")
	assert.True(t, missing,
		"late B's nested required B-child must still be checked")
}

// TestParseInline_RejectsWildcardAsHeadingText regresses a
// confusing-input case: a bare-string `heading: "..."` would
// silently be treated as a literal section named "...". Reject it
// at parse time so the only path to a slot is the mapping form
// (`heading: { regex: '.+', repeat: { min: 0 } }`).
func TestParseInline_RejectsWildcardAsHeadingText(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "..."},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"..."`)
	assert.Contains(t, err.Error(), "regex: '.+'")
}

// ---- Unified heading: grammar (string / null / mapping) ----

func TestParseInline_HeadingString(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{"heading": "Goal"}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1)
	assert.Equal(t, "Goal", sch.Sections[0].Heading)
	assert.False(t, sch.Sections[0].Preamble)
	require.NotNil(t, sch.Sections[0].Matcher)
	assert.Equal(t, "Goal", sch.Sections[0].Matcher.Regex,
		"bare-string sugar populates Matcher.Regex with the literal text")
}

func TestParseInline_HeadingNullIsPreamble(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": nil},
			map[string]any{"heading": "Goal"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	require.Len(t, sch.Sections, 2)
	assert.True(t, sch.Sections[0].Preamble)
	assert.Equal(t, "Goal", sch.Sections[1].Heading)
}

func TestParseInline_HeadingMappingSlot(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "Overview"},
			map[string]any{"heading": map[string]any{
				"regex":  ".+",
				"repeat": map[string]any{"min": 0},
			}},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	require.Len(t, sch.Sections, 2)
	require.NotNil(t, sch.Sections[1].Matcher)
	assert.True(t, isSlotMatcher(sch.Sections[1].Matcher),
		"the mapping form `.+` plus `repeat: {min: 0}` desugars to a slot")
}

func TestParseInline_RejectsPreambleAfterFirst(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "Goal"},
			map[string]any{"heading": nil},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be the first entry")
}

func TestParseInline_RejectsSectionsOnPreamble(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading":  nil,
				"sections": []any{map[string]any{"heading": "Sub"}},
			},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "preamble")
	assert.Contains(t, err.Error(), "sections")
}

// TestParseInline_RejectsSlotForbiddenKeys: a slot's `sections:` /
// `rules:` / `closed:` / `content:` fields are not honoured by the
// validator. Reject them at parse time so authors see the
// misconfiguration immediately.
func TestParseInline_RejectsSlotForbiddenKeys(t *testing.T) {
	cases := []struct {
		key string
		val any
	}{
		{"sections", []any{map[string]any{"heading": "Sub"}}},
		{"rules", map[string]any{
			"paragraph-readability": map[string]any{"max-index": 12.0},
		}},
		{"closed", true},
		{"content", []any{map[string]any{"kind": "paragraph"}}},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			raw := map[string]any{
				"sections": []any{
					map[string]any{
						"heading": map[string]any{
							"regex":  ".+",
							"repeat": map[string]any{"min": 0},
						},
						tc.key: tc.val,
					},
				},
			}
			_, err := ParseInline(raw, "kind x")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.key)
			assert.Contains(t, err.Error(), "slot")
		})
	}
}

func TestParseInline_RejectsEmptyHeadingMapping(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{"heading": map[string]any{}}},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty mapping")
}

func TestParseInline_RejectsMappingMissingRegex(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"repeat": map[string]any{"min": 1},
			}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "`regex:` is required")
}

func TestParseInline_RejectsLegacyUnlistedKey(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{"unlisted": true}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "`heading.unlisted:` removed")
	assert.Contains(t, err.Error(), "regex: '.+'")
}

func TestParseInline_RejectsUnknownHeadingKind(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{"frobnicate": true}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown heading-mapping key")
}

func TestParseInline_RejectsEmptyRepeatMapping(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"regex":  ".+",
				"repeat": map[string]any{},
			}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty mapping")
}

func TestParseInline_RejectsMaxZero(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"regex":  ".+",
				"repeat": map[string]any{"max": 0},
			}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max:")
	assert.Contains(t, err.Error(), "greater than 0")
}

func TestParseInline_RejectsMinGreaterThanMax(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"regex":  ".+",
				"repeat": map[string]any{"min": 5, "max": 2},
			}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min=5 is greater than max=2")
}

func TestParseInline_RejectsSequentialWithoutDigits(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"regex":      "Step",
				"sequential": true,
			}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sequential")
	assert.Contains(t, err.Error(), "digits")
}

func TestParseInline_RejectsInvalidRegex(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"regex": "[unterminated",
			}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "regex")
}

func TestParseInline_RejectsClosedOnFrontmatterOnly(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{"id": "string"},
		"closed":      true,
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema.closed")
	assert.Contains(t, err.Error(), "sections")
}

// TestHeadingLine_FallbackTo1 covers the empty-Lines path of
// headingLine. Goldmark normally populates Lines() for any
// heading; a hand-constructed *ast.Heading with no Lines and no
// child Text nodes models the edge case where neither offset
// source is available, and the helper must return 1 (not 0) so
// the per-scope walker's line filter still sees the heading.
func TestHeadingLine_FallbackTo1(t *testing.T) {
	doc := newDocFile(t, "doc.md", "# T\n")
	empty := &ast.Heading{Level: 2}
	got := headingLine(empty, doc)
	assert.Equal(t, 1, got)
}

// TestValidate_WildcardHeadingParticipatesInOutOfOrder regresses
// the `.+` mapping fallback: a later listed scope whose Matcher
// accepts any text must still be claimable via out-of-order
// detection.
func TestValidate_WildcardHeadingParticipatesInOutOfOrder(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "A"},
			map[string]any{"heading": map[string]any{"regex": ".+"}},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Doc: B first (matches the `.+` mapping), then A.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## B\n\nx\n\n## A\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var oo bool
	for _, d := range diags {
		if strings.Contains(d.Message, `## B: got <out of order>`) &&
			strings.Contains(d.Message, `expected after "## A"`) {
			oo = true
		}
		assert.NotContains(t, d.Message, "expected section to be present",
			"the `.+` mapping must claim B via out-of-order, not stay missing")
		assert.NotContains(t, d.Message, "<present>",
			"B should not surface as unexpected when `.+` can claim it")
	}
	assert.True(t, oo,
		"a `.+` matcher must participate in out-of-order detection")
}

// TestValidate_PlaceholderRegexParticipatesInOutOfOrder verifies
// out-of-order detection works for a scope whose regex uses
// fmvar() interpolation.
func TestValidate_PlaceholderRegexParticipatesInOutOfOrder(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"user": `string & != ""`,
			"role": `string & != ""`,
		},
		"sections": []any{
			map[string]any{"heading": "A"},
			map[string]any{
				"heading": map[string]any{
					"regex": `\#(fmvar(user)): \#(fmvar(role))`,
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## alice: admin\n\nx\n\n## A\n\ny\n")
	diags := Validate(doc, sch,
		map[string]any{"user": "alice", "role": "admin"},
		false, makeDiagForTest)
	var oo bool
	for _, d := range diags {
		if strings.Contains(d.Message, `## alice: admin: got <out of order>`) &&
			strings.Contains(d.Message, `expected after "## A"`) {
			oo = true
		}
		assert.NotContains(t, d.Message, "expected section to be present",
			"fmvar regex must claim the heading, not leave the scope missing")
	}
	assert.True(t, oo,
		"fmvar regex must participate in out-of-order detection")
}

// TestParseFile_DefaultMaxBytesCapped regresses the FileReader
// MaxBytes default: a zero-value reader now defaults to
// lint.DefaultMaxInputBytes so a 5 MB schema file is rejected
// instead of silently read.
func TestParseFile_DefaultMaxBytesCapped(t *testing.T) {
	dir := t.TempDir()
	// Build a file larger than the 2 MB default.
	big := make([]byte, lint.DefaultMaxInputBytes+1)
	for i := range big {
		big[i] = 'a'
	}
	p := writeFile(t, dir, "proto.md", "# ?\n\n## "+string(big)+"\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file too large")
}

// TestValidate_StrayShallowerHeadingDoesNotTerminate regresses a
// matchScope bug: an H1 (or otherwise shallower) heading in the
// middle of the document used to terminate root-level matching,
// leaving subsequent required scopes flagged as missing even
// though they were present.
func TestValidate_StrayShallowerHeadingDoesNotTerminate(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "Overview"},
			map[string]any{"heading": "Decision"},
		},
	}
	sch, err := ParseInline(raw, "kind rfc")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Overview\n\nx\n\n# Another\n\n## Decision\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	for _, d := range diags {
		assert.NotContains(t, d.Message, "missing required",
			"stray H1 must not stop the root walk from claiming later scopes")
	}
}

// TestValidate_OpenScopeFlagsLateListedScope regresses a Copilot
// finding: schema [A(req), B(opt), C(req)] with doc A → C → B
// previously silently accepted B because the open trailing loop
// only flagged unexpected when closed=true. The leftover-listed
// check now surfaces B as out-of-order regardless of closed.
func TestValidate_OpenScopeFlagsLateListedScope(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "A"},
			map[string]any{"heading": map[string]any{
				"regex":  "B",
				"repeat": map[string]any{"min": 0, "max": 1},
			}},
			map[string]any{"heading": "C"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## A\n\nx\n\n## C\n\ny\n\n## B\n\nz\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var found bool
	for _, d := range diags {
		if strings.Contains(d.Message, `## B: got <out of order>`) &&
			strings.Contains(d.Message, "expected before this position") {
			found = true
		}
	}
	assert.True(t, found,
		"late-arriving listed scope must surface out-of-order in open scopes too")
}

// TestValidate_OptionalScopeNotOutOfOrder regresses a Copilot
// review finding: when the current scope is optional and the doc
// only contains a later listed scope's heading, matchScope should
// not surface an "out of order" diagnostic. Omitting an optional
// section is legitimate; the next scope picks up the heading on
// its own iteration.
func TestValidate_OptionalScopeNotOutOfOrder(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"regex":  "A",
				"repeat": map[string]any{"min": 0, "max": 1},
			}},
			map[string]any{"heading": map[string]any{
				"regex":  "B",
				"repeat": map[string]any{"min": 0, "max": 1},
			}},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n\n## B\n\nx\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	for _, d := range diags {
		assert.NotContains(t, d.Message, "out of order",
			"optional A omitted should not trigger out-of-order on B")
	}
}

// TestValidate_RequiredScopeStillFlagsOutOfOrder keeps the genuine
// out-of-order case working: A is required and B appears before
// it, so B must still be flagged.
func TestValidate_RequiredScopeStillFlagsOutOfOrder(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "A"},
			map[string]any{"heading": "B"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n\n## B\n\nx\n\n## A\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var ooFound bool
	for _, d := range diags {
		if strings.Contains(d.Message, `## B: got <out of order>`) &&
			strings.Contains(d.Message, `expected after "## A"`) {
			ooFound = true
		}
	}
	assert.True(t, ooFound, "required scope must still surface out-of-order")
}

func TestValidate_ClosedTrailingExtra(t *testing.T) {
	// Trailing heading past all required scopes with closed=true
	// produces the trailing-loop "unexpected section" diagnostic
	// (validateScopes line 151-155).
	raw := map[string]any{
		"closed": true,
		"sections": []any{
			map[string]any{"heading": "Goal"},
		},
	}
	sch, err := ParseInline(raw, "kind plan")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Goal\n\nx\n\n## Trailing\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var trailing bool
	for _, d := range diags {
		if strings.Contains(d.Message, `## Trailing: got <present>`) &&
			strings.Contains(d.Message, "not declared in schema") {
			trailing = true
		}
	}
	assert.True(t, trailing,
		"expected the trailing-extra diagnostic from validateScopes")
}

func TestValidate_ShallowNonMatchEndsScopeList(t *testing.T) {
	// In a nested validateScopes call, when the next doc heading is
	// shallower than expected AND doesn't match the current scope's
	// text, matchScope returns false without claiming. This covers
	// the "return docIdx, diags, false" line at validate.go:204.
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Parent",
				"sections": []any{
					map[string]any{"heading": "Child"},
				},
			},
			map[string]any{
				"heading": "Sibling",
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Doc: Parent then Sibling at H2 — Child is missing, and
	// Sibling is shallower than the H3 the nested call expects.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Parent\n\nx\n\n## Sibling\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var missing bool
	for _, d := range diags {
		if strings.Contains(d.Message, `### Child: got <missing>`) {
			missing = true
		}
	}
	assert.True(t, missing, "expected missing-Child diagnostic")
}

func TestValidate_DeeperHeadingConsumedAsOrphan(t *testing.T) {
	// Doc has a deeper heading that doesn't match the scope and
	// doesn't match any later listed scope. matchScope must skip
	// it silently (validate.go:220-222).
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "Goal"},
		},
	}
	sch, err := ParseInline(raw, "kind plan")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n### Orphan\n\nx\n\n## Goal\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	for _, d := range diags {
		assert.NotContains(t, d.Message, "Orphan",
			"deeper orphan should not trip a diagnostic")
	}
}

func TestParseInline_ScopeLevelClosed(t *testing.T) {
	// `closed:` on a scope (not the root) — covers the
	// applyScopeFields "closed" branch.
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Parent",
				"closed":  true,
				"sections": []any{
					map[string]any{"heading": "Child"},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1)
	assert.True(t, sch.Sections[0].Closed)
}

func TestSetScopeSections_RejectsNonList(t *testing.T) {
	// `sections:` at a nested scope set to a string instead of a
	// list — exercises setScopeSections' error branch.
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading":  "Parent",
				"sections": "not-a-list",
			},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sections must be a list")
}

func TestValidate_FilenameMatchesPattern(t *testing.T) {
	// The "matched" return branch of validateFilename.
	raw := map[string]any{
		"filename": "doc.md",
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags)
}

func TestCompileMatcher_RejectsInvalidRegex(t *testing.T) {
	// Patterns that don't compile as RE2 surface as compileMatcher
	// errors; the validator falls back to "no match" so a malformed
	// pattern still produces a missing-required diagnostic rather
	// than panicking.
	m := &Matcher{Regex: "[unterminated"}
	_, err := compileMatcher(m, nil)
	require.Error(t, err)
}

// ---- Validate frontmatter CUE-placeholder skip ----

func TestValidate_SkipsCUECheckWhenFmIsCUE(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"id": `=~"^RFC-[0-9]{4}$"`,
		},
	}
	sch, err := ParseInline(raw, "kind rfc")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"---\nid: NOT-AN-RFC\n---\n# T\n")
	diags := Validate(doc, sch, map[string]any{"id": "NOT-AN-RFC"}, true, makeDiagForTest)
	assert.Empty(t, diags,
		"fmIsCUE=true should skip the CUE check entirely")
}
