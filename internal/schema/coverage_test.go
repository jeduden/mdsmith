package schema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- ParseInline edge cases ----

func TestParseInline_RejectsNonIntegerFloat(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Repeating",
				"repeats": true,
				"min":     1.5,
			},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an integer")
}

func TestParseInline_AcceptsIntegerFloat(t *testing.T) {
	// YAML decoders surface plain numbers as float64; whole-number
	// floats must still pass as integers.
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Repeating",
				"repeats": true,
				"min":     1.0,
				"max":     3.0,
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1)
	assert.Equal(t, 1, sch.Sections[0].Min)
	assert.Equal(t, 3, sch.Sections[0].Max)
}

func TestParseInline_RejectsEmptyHeading(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"required": true},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty heading")
}

func TestParseInline_RejectsBlankHeading(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "   ", "required": true},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty heading")
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
	// A non-wildcard string in sections is rejected. This exercises
	// the parseInlineScopeEntry string branch.
	raw := map[string]any{
		"sections": []any{"not-a-wildcard"},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a mapping or the wildcard")
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

func TestParseInline_RejectsBadRequireType(t *testing.T) {
	raw := map[string]any{"require": "not-a-map"}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "require must be a mapping")
}

func TestParseInline_RejectsBadRequireFilename(t *testing.T) {
	raw := map[string]any{
		"require": map[string]any{"filename": 42},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "filename must be a string")
}

func TestParseInline_RejectsUnknownRequireKey(t *testing.T) {
	raw := map[string]any{
		"require": map[string]any{"unknown": "v"},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown schema.require key")
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

func TestParseInline_RejectsBadRequiredType(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "X", "required": "yes",
		}},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required must be a boolean")
}

func TestParseInline_RejectsBadAliasesType(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "X", "aliases": "not-a-list",
		}},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aliases must be a list")
}

func TestParseInline_RejectsBadAliasItemType(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "X", "aliases": []any{42},
		}},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aliases[0] must be a string")
}

func TestParseInline_RejectsBadIntType(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "X", "min": "two",
		}},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min must be an integer")
}

func TestParseInline_AcceptsInt64(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "X", "min": int64(2),
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	assert.Equal(t, 2, sch.Sections[0].Min)
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
	assert.Equal(t, "plan-*.md", sch.Require.Filename)
}

func TestParseFile_RequireMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"<?require\nfilename: [unterminated\n?>\n\n# ?\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid <?require?>")
}

func TestParseFile_FrontmatterWithoutTrailingNewline(t *testing.T) {
	// Exercises stripDelimiters fallback when the closing "---" has
	// no trailing newline.
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
	assert.Equal(t, "frag-*.md", sch.Require.Filename,
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
	assert.Equal(t, "plan-*.md", sch.Require.Filename)
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
	assert.False(t, (&Schema{Require: Require{Filename: "*.md"}}).IsEmpty())
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
	// via the regex path inside matchesText.
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "# {id}: {name}\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# MDS001: line-length\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags,
		"field-interpolated H1 pattern should match a concrete title")
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
	sc := Scope{Heading: "Goal"}
	assert.True(t, MatchesHeading(sc, DocHeading{Text: "Goal", Level: 2}))
	assert.False(t, MatchesHeading(sc, DocHeading{Text: "Other", Level: 2}))
	// Wildcard scopes never match a specific heading.
	assert.False(t, MatchesHeading(Scope{Wildcard: true}, DocHeading{Text: "Anything"}))
	// "?" matches any text.
	assert.True(t, MatchesHeading(Scope{Heading: "?"}, DocHeading{Text: "Anything"}))
	// Aliases match.
	sc2 := Scope{Heading: "Symptoms", Aliases: []string{"Indicators"}}
	assert.True(t, MatchesHeading(sc2, DocHeading{Text: "Indicators"}))
}

func TestPatternRegexCache_ReusesCompiled(t *testing.T) {
	// Two calls with the same pattern must hit the cache the second
	// time. Cover both the cache-miss and cache-hit branches.
	pattern := "Step {n}"
	first := patternRegex(pattern)
	require.NotNil(t, first)
	second := patternRegex(pattern)
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
		if d.Message == `section "## Tasks" out of order: expected after "## Goal"` {
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
