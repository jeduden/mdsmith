package requiredstructure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestFile(
	t *testing.T, path, source string,
) *lint.File {
	t.Helper()
	f, err := lint.NewFileFromSource(path, []byte(source), true)
	require.NoError(t, err)
	return f
}

// writeSchema writes schema content to a temp file and
// returns its path.
func writeSchema(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "schema.md")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func expectDiags(
	t *testing.T, diags []lint.Diagnostic, count int,
) {
	t.Helper()
	if len(diags) != count {
		msgs := make([]string, len(diags))
		for i, d := range diags {
			msgs[i] = d.Message
		}
		t.Fatalf(
			"expected %d diagnostic(s), got %d: %v",
			count, len(diags), msgs,
		)
	}
}

func expectDiagMsg(
	t *testing.T, diags []lint.Diagnostic, msg string,
) {
	t.Helper()
	if len(diags) == 0 {
		t.Fatalf(
			"expected diagnostic with message %q, got none",
			msg,
		)
	}
	for _, d := range diags {
		if strings.Contains(d.Message, msg) {
			return
		}
	}
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	t.Errorf(
		"no diagnostic contains %q, got: %v", msg, msgs,
	)
}

// =====================================================================
// Rule metadata
// =====================================================================

func TestRule_ID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS020" {
		t.Errorf("expected ID MDS020, got %s", r.ID())
	}
}

func TestRule_Name(t *testing.T) {
	r := &Rule{}
	if r.Name() != "required-structure" {
		t.Errorf(
			"expected Name required-structure, got %s",
			r.Name(),
		)
	}
}

func TestRule_Category(t *testing.T) {
	r := &Rule{}
	if r.Category() != "meta" {
		t.Errorf(
			"expected Category meta, got %s", r.Category(),
		)
	}
}

// =====================================================================
// ApplySettings / DefaultSettings
// =====================================================================

func TestApplySettings_ValidSchema(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"schema": "foo.md"})
	require.NoError(t, err, "unexpected error: %v", err)
	if r.Schema != "foo.md" {
		t.Errorf(
			"expected Schema foo.md, got %s", r.Schema,
		)
	}
}

func TestApplySettings_InvalidSchema(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"schema": 42})
	require.Error(t, err, "expected error for non-string schema")
}

func TestApplySettings_UnknownSetting(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": true})
	require.Error(t, err, "expected error for unknown setting")
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["schema"] != "" {
		t.Errorf(
			"expected schema=\"\", got %v", ds["schema"],
		)
	}
}

// =====================================================================
// No-op when schema is empty
// =====================================================================

func TestCheck_NoSchemaIsNoop(t *testing.T) {
	r := &Rule{Schema: ""}
	f := newTestFile(t, "doc.md", "# Hello\n\nSome text.\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Schema parsing
// =====================================================================

func TestParseSchema_Headings(t *testing.T) {
	schemaSrc := `# ?

## Settings

## Examples

### Good

### Bad
`
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err, "unexpected error: %v", err)
	if len(tmpl.Headings) != 5 {
		t.Fatalf(
			"expected 5 headings, got %d",
			len(tmpl.Headings),
		)
	}
	if tmpl.Headings[0].Text != "?" {
		t.Errorf(
			"expected first heading ?, got %q",
			tmpl.Headings[0].Text,
		)
	}
	if tmpl.Headings[0].Level != 1 {
		t.Errorf(
			"expected first heading level 1, got %d",
			tmpl.Headings[0].Level,
		)
	}
}

func TestParseSchema_SyncPoints(t *testing.T) {
	schemaSrc := `# {id}: {name}

{description}
`
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err, "unexpected error: %v", err)
	headingSyncs := tmpl.SyncPoints[0]
	if len(headingSyncs) < 2 {
		t.Fatalf(
			"expected at least 2 heading sync points, got %d",
			len(headingSyncs),
		)
	}
	if headingSyncs[0].Field != "id" {
		t.Errorf(
			"expected first sync field 'id', got %q",
			headingSyncs[0].Field,
		)
	}
	if headingSyncs[1].Field != "name" {
		t.Errorf(
			"expected second sync field 'name', got %q",
			headingSyncs[1].Field,
		)
	}

	bodySyncs := 0
	for _, sp := range tmpl.SyncPoints[0] {
		if sp.InBody {
			bodySyncs++
		}
	}
	if bodySyncs < 1 {
		t.Error("expected at least 1 body sync point")
	}
}

func TestParseSchema_StrictOrder(t *testing.T) {
	schemaSrc := `# ?

## Goal

## Tasks

## Acceptance Criteria
`
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err, "unexpected error: %v", err)
	if len(tmpl.Headings) != 4 {
		t.Fatalf(
			"expected 4 headings, got %d",
			len(tmpl.Headings),
		)
	}
}

// =====================================================================
// Structure checking
// =====================================================================

func TestCheck_MissingHeading(t *testing.T) {
	schemaPath := writeSchema(t,
		"# ?\n\n## Settings\n\n## Examples\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md", "# My Rule\n\n## Examples\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `missing required section "## Settings"`)
}

func TestCheck_ExtraSectionForbidden(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Goal\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"# My Plan\n\n## Prerequisites\n\n## Goal\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `unexpected section "## Prerequisites"`)
}

func TestCheck_SectionWildcardAllowsExtras(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Goal\n\n## ...\n\n## Settings\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"# My Rule\n\n## Goal\n\n## Overview\n\n## Notes\n\n## Settings\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_SectionWildcardAllowsTrailingExtras(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Goal\n\n## ...\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"# My Rule\n\n## Goal\n\n## Notes\n\n## Risks\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_WrongLevel(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Settings\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"# My Rule\n\n### Settings\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "heading level mismatch")
}

// Level-mismatch diagnostics must name the offending heading so
// readers can locate it in documents with many headings.
func TestCheck_WrongLevel_NamesHeading(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Settings\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"# My Rule\n\n### Settings\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `"Settings"`)
	expectDiagMsg(t, diags, "expected h2, got h3")
}

// Unexpected-section diagnostics should tell the author which
// required heading was expected at that position.
func TestCheck_ExtraSection_NamesExpected(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Goal\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"# My Plan\n\n## Prerequisites\n\n## Goal\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `unexpected section "## Prerequisites"`)
	expectDiagMsg(t, diags, `expected "## Goal"`)
}

// Trailing extras (past the last required heading) have no
// "expected next" to name, so the diagnostic should still be
// emitted without an expected suffix.
func TestCheck_ExtraSection_TrailingNoExpected(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Goal\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"# My Plan\n\n## Goal\n\n## Trailing\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `unexpected section "## Trailing"`)
}

// When a required section appears but in the wrong order, the
// rule should report it as out-of-order rather than double-counting
// it as both "unexpected" and "missing required".
func TestCheck_OutOfOrderSection(t *testing.T) {
	schemaPath := writeSchema(t,
		"# ?\n\n## Goal\n\n## Tasks\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"# My Plan\n\n## Tasks\n\n## Goal\n")
	diags := r.Check(f)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	// The document holds both required sections, so the rule must
	// not emit a "missing required" diagnostic for either.
	for _, m := range msgs {
		if strings.Contains(m, "missing required section") {
			t.Errorf("unexpected missing-required diagnostic: %q (all: %v)", m, msgs)
		}
	}
	expectDiagMsg(t, diags, `out of order`)
}

func TestCheck_AllPresent(t *testing.T) {
	schemaPath := writeSchema(t,
		"# ?\n\n## Settings\n\n## Examples\n\n### Good\n\n### Bad\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"# MDS001: line-length\n\n## Settings\n\n## Examples\n\n### Good\n\n### Bad\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Frontmatter-body sync
// =====================================================================

func TestCheck_HeadingSyncMismatch(t *testing.T) {
	schemaPath := writeSchema(t, "# {id}: {name}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nid: MDS001\nname: line-length\n---\n# MDS002: line-length\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "heading does not match frontmatter")
}

func TestCheck_HeadingSyncMatch(t *testing.T) {
	schemaPath := writeSchema(t, "# {id}: {name}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nid: MDS001\nname: line-length\n---\n# MDS001: line-length\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_BodySyncMismatch(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n{description}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\ndescription: Line exceeds maximum length.\n---\n# My Rule\n\nWrong description here.\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`body does not match frontmatter field "description"`)
}

func TestCheck_BodySyncMatch(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n{description}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\ndescription: Line exceeds maximum length.\n---\n# My Rule\n\nLine exceeds maximum length.\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_WildcardHeading(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Goal\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"# Any Title Works\n\n## Goal\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Nested front-matter access (CUE paths)
// =====================================================================

func TestCheck_NestedFieldInHeading(t *testing.T) {
	schemaPath := writeSchema(t, "# {params.subtitle}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nparams:\n  subtitle: Overview\n---\n# Overview\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_NestedFieldInHeadingMismatch(t *testing.T) {
	schemaPath := writeSchema(t, "# {params.subtitle}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nparams:\n  subtitle: Overview\n---\n# Wrong Title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "heading does not match frontmatter")
}

func TestCheck_QuotedKeyInHeading(t *testing.T) {
	schemaPath := writeSchema(t, "# {\"my-key\"}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nmy-key: value\n---\n# value\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_NestedFieldInBody(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n{params.description}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nparams:\n  description: A detailed overview.\n---\n# My Doc\n\nA detailed overview.\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_NestedFieldInBodyMismatch(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n{params.description}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nparams:\n  description: A detailed overview.\n---\n# My Doc\n\nWrong content.\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`body does not match frontmatter field "params.description"`)
}

func TestCheck_MissingNestedKeyEmitsDiagnostic(t *testing.T) {
	// Schema references {params.missing} but front matter has params.subtitle.
	schemaPath := writeSchema(t, "# {params.missing}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nparams:\n  subtitle: Overview\n---\n# Overview\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "missing or invalid frontmatter path")
}

func TestCheck_MissingNestedKeyInBodyEmitsDiagnostic(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n{params.missing}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nparams:\n  subtitle: Overview\n---\n# My Doc\n\nSome content.\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "missing or invalid frontmatter path")
}

// =====================================================================
// Embedded front matter CUE schema
// =====================================================================

func TestCheck_FrontMatterCUESchemaMatch(t *testing.T) {
	schemaPath := writeSchema(t, `---
id: 'int & >=1'
status: '"🔲" | "🔳" | "✅"'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nid: 40\nstatus: \"✅\"\n---\n# Any title\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_FrontMatterCUESchemaMismatch(t *testing.T) {
	schemaPath := writeSchema(t, `---
id: 'int & >=1'
status: '"🔲" | "🔳" | "✅"'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nid: 40\n---\n# Any title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		"front matter does not satisfy schema CUE constraints")
}

func TestCheck_FrontMatterCUESchemaInvalidStatus(t *testing.T) {
	schemaPath := writeSchema(t, `---
id: 'int & >=1'
status: '"🔲" | "🔳" | "✅"'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nid: 40\nstatus: in-progress\n---\n# Any title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		"front matter does not satisfy schema CUE constraints")
}

func TestCheck_FrontMatterCUESchemaRejectsExtraFields(t *testing.T) {
	schemaPath := writeSchema(t, `---
id: 'int & >=1'
status: '"🔲" | "🔳" | "✅"'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nid: 40\nstatus: \"✅\"\nextra: true\n---\n# Any title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		"front matter does not satisfy schema CUE constraints")
}

func TestCheck_InvalidSchemaFrontMatterCUE(t *testing.T) {
	schemaPath := writeSchema(t, `---
id: 'int &'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md", "# Any title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "invalid schema")
}

func TestCheck_TemplateKeyInFrontmatterAsCUEField(t *testing.T) {
	// "template" is not a reserved key — it's a regular CUE schema field.
	schemaPath := writeSchema(t, `---
template: 'string'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\ntemplate: my-value\n---\n# Any title\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Optional frontmatter fields (key? suffix)
// =====================================================================

func TestCheck_OptionalFieldPresent(t *testing.T) {
	schemaPath := writeSchema(t, `---
name: 'string & != ""'
"description?": 'string'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nname: my-skill\ndescription: A helpful skill.\n---\n# My Skill\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_OptionalFieldAbsent(t *testing.T) {
	schemaPath := writeSchema(t, `---
name: 'string & != ""'
"description?": 'string'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nname: my-skill\n---\n# My Skill\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_OptionalFieldRejectsExtraFields(t *testing.T) {
	schemaPath := writeSchema(t, `---
name: 'string & != ""'
"description?": 'string'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nname: my-skill\nunknown: true\n---\n# My Skill\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		"front matter does not satisfy schema CUE constraints")
}

func TestCheck_OptionalFieldInvalidType(t *testing.T) {
	schemaPath := writeSchema(t, `---
name: 'string & != ""'
"user-invocable?": bool
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nname: my-skill\nuser-invocable: not-a-bool\n---\n# My Skill\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		"front matter does not satisfy schema CUE constraints")
}

// =====================================================================
// Filename validation (<?require filename?> directive)
// =====================================================================

func TestCheck_FilenamePatternMatch(t *testing.T) {
	schemaPath := writeSchema(t, `<?require
filename: "[0-9]*_*.md"
?>
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "50_my-plan.md", "# My Plan\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_FilenamePatternMismatch(t *testing.T) {
	schemaPath := writeSchema(t, `<?require
filename: "[0-9]*_*.md"
?>
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "my-plan.md", "# My Plan\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`filename "my-plan.md" does not match required pattern`)
}

func TestCheck_FilenamePatternSingleLinePI(t *testing.T) {
	schemaPath := writeSchema(t, `<?require filename: "[0-9]*_*.md" ?>
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "my-plan.md", "# My Plan\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`filename "my-plan.md" does not match required pattern`)
}

func TestCheck_FilenamePatternPIWithTrailingContent(t *testing.T) {
	schemaPath := writeSchema(t, "<?require filename: \"[0-9]*_*.md\" ?>trailing\n# ?\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "my-plan.md", "# My Plan\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`filename "my-plan.md" does not match required pattern`)
}

func TestCheck_FilenamePatternIndentedPI(t *testing.T) {
	schemaPath := writeSchema(t, `  <?require filename: "[0-9]*_*.md" ?>
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "my-plan.md", "# My Plan\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`filename "my-plan.md" does not match required pattern`)
}

func TestCheck_FilenamePatternNotSet(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "anything.md", "# Title\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Schema file skipping
// =====================================================================

// =====================================================================
// Schema composition via <?include?>
// =====================================================================

func TestParseSchema_SchemaInclude(t *testing.T) {
	dir := t.TempDir()
	// Write a fragment file with headings.
	fragDir := filepath.Join(dir, "common")
	require.NoError(t, os.MkdirAll(fragDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(fragDir, "acceptance.md"),
		[]byte("## Acceptance Criteria\n\n- [ ] All tests pass\n"),
		0o644,
	))

	// Write a schema that includes the fragment.
	schema := "# ?\n\n## Goal\n\n<?include\nfile: common/acceptance.md\n?>\n"
	schemaPath := filepath.Join(dir, "schema.md")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schema), 0o644))

	tmpl, err := parseSchema([]byte(schema), schemaPath, 0)
	require.NoError(t, err)
	require.Len(t, tmpl.Headings, 3)
	assert.Equal(t, "?", tmpl.Headings[0].Text)
	assert.Equal(t, "Goal", tmpl.Headings[1].Text)
	assert.Equal(t, "Acceptance Criteria", tmpl.Headings[2].Text)
}

func TestParseSchema_SchemaIncludeRequireMerge(t *testing.T) {
	dir := t.TempDir()
	// Fragment with a <?require?> directive.
	frag := "<?require\nfilename: \"[0-9]*_*.md\"\n?>\n## Tasks\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "frag.md"),
		[]byte(frag), 0o644,
	))

	schema := "# ?\n\n## Goal\n\n<?include\nfile: frag.md\n?>\n"
	schemaPath := filepath.Join(dir, "schema.md")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schema), 0o644))

	tmpl, err := parseSchema([]byte(schema), schemaPath, 0)
	require.NoError(t, err)
	assert.Equal(t, `[0-9]*_*.md`, tmpl.Config.FilenamePattern)
	require.Len(t, tmpl.Headings, 3)
	assert.Equal(t, "Tasks", tmpl.Headings[2].Text)
}

func TestParseSchema_SchemaIncludeIgnoresFragmentFM(t *testing.T) {
	dir := t.TempDir()
	// Fragment with frontmatter that should be ignored.
	frag := "---\nid: 99\n---\n## Extra\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "frag.md"),
		[]byte(frag), 0o644,
	))

	schema := "---\nid: 'int & >=1'\n---\n# ?\n\n<?include\nfile: frag.md\n?>\n"
	schemaPath := filepath.Join(dir, "schema.md")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schema), 0o644))

	tmpl, err := parseSchema([]byte(schema), schemaPath, 0)
	require.NoError(t, err)
	// CUE schema should only come from root, not fragment.
	assert.Contains(t, tmpl.Config.FrontMatterCUE, "id")
	require.Len(t, tmpl.Headings, 2)
	assert.Equal(t, "Extra", tmpl.Headings[1].Text)
}

func TestParseSchema_SchemaIncludeCycleDetected(t *testing.T) {
	dir := t.TempDir()
	// Schema includes itself.
	schema := "# ?\n\n<?include\nfile: schema.md\n?>\n"
	schemaPath := filepath.Join(dir, "schema.md")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schema), 0o644))

	_, err := parseSchema([]byte(schema), schemaPath, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cyclic include")
}

func TestParseSchema_SchemaIncludeIndirectCycle(t *testing.T) {
	dir := t.TempDir()
	// a.md includes b.md which includes a.md
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "b.md"),
		[]byte("## B\n\n<?include\nfile: a.md\n?>\n"),
		0o644,
	))

	schema := "# ?\n\n<?include\nfile: b.md\n?>\n"
	schemaPath := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schema), 0o644))

	_, err := parseSchema([]byte(schema), schemaPath, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cyclic include")
}

// =====================================================================
// <?require?> in non-schema file warning
// =====================================================================

func TestCheck_RequireInNonSchemaFileWarns(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"<?require\nfilename: \"*.md\"\n?>\n# My Doc\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		"<?require?> is only recognized in schema files; this directive has no effect here")
	assert.Equal(t, lint.Warning, diags[0].Severity,
		"misplaced <?require?> should be a warning, not an error")
}

func TestCheck_RequireInNonSchemaFileNoSchemaSet(t *testing.T) {
	r := &Rule{Schema: ""}
	f := newTestFile(t, "doc.md",
		"<?require\nfilename: \"*.md\"\n?>\n# My Doc\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		"<?require?> is only recognized in schema files; this directive has no effect here")
	assert.Equal(t, lint.Warning, diags[0].Severity,
		"misplaced <?require?> should be a warning, not an error")
}

func TestCheck_RequireInSchemaFileNoWarning(t *testing.T) {
	schemaPath := writeSchema(t,
		"<?require\nfilename: \"*.md\"\n?>\n# ?\n")
	r := &Rule{Schema: schemaPath}
	// Check the schema file itself — should not warn.
	f := newTestFile(t, schemaPath,
		"<?require\nfilename: \"*.md\"\n?>\n# ?\n")
	diags := r.Check(f)
	// Should have no require warning.
	for _, d := range diags {
		if strings.Contains(d.Message, "<?require?>") {
			t.Errorf("unexpected require warning: %s", d.Message)
		}
	}
}

// =====================================================================
// Schema file skipping
// =====================================================================

func TestCheck_SkipsSchemaFiles(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Goal\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, schemaPath, "# ?\n\n## Settings\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_FrontMatterAnchorRejected(t *testing.T) {
	schemaPath := writeSchema(t, "---\nid: 'int'\n---\n# ?\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nbase: &base\n  id: 1\n---\n# Title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "anchors/aliases are not permitted")
}

func TestDeriveFrontMatterCUE_AnchorRejected(t *testing.T) {
	yml := []byte("base: &base\n  id: 1\n")
	_, err := deriveFrontMatterCUE(yml)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "anchors/aliases are not permitted")
}

func TestExtractRequireDirective_AnchorRejected(t *testing.T) {
	src := "<?require\nbase: &base\n  filename: \"*.md\"\n?>\n# Title\n"
	f := newTestFile(t, "schema.md", src)
	_, err := extractRequireDirective(f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "anchors/aliases are not permitted")
}

func TestParseSchemaFrontMatter_AnchorRejected(t *testing.T) {
	prefix := []byte("---\nbase: &base\n  id: 1\n---\n")
	_, err := parseSchemaFrontMatter(prefix)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "anchors/aliases are not permitted")
}

func TestCheck_InvalidYAMLFrontMatterDiagnostic(t *testing.T) {
	schemaPath := writeSchema(t, "---\nid: 'int'\n---\n# ?\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\n: invalid: yaml: [unclosed\n---\n# Title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "front matter: invalid YAML")
}

func TestCheck_IncludeWithAnchorInSchema(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.md")
	require.NoError(t, os.WriteFile(schemaPath, []byte(
		"# ?\n\n<?include\nbase: &base\n  file: fragment.md\n?>\n<?/include?>\n",
	), 0o644))
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nid: 1\n---\n# Title\n")
	diags := r.Check(f)
	// Should produce a diagnostic about the anchor in the include directive.
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "anchors/aliases are not permitted") {
			found = true
			break
		}
	}
	assert.True(t, found,
		"expected diagnostic rejecting anchors/aliases, got: %v", diags)
}

// =====================================================================
// Schema read via RootFS
// =====================================================================

func TestCheck_SchemaViaRootFS(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.md")
	if err := os.WriteFile(schemaPath, []byte("# Title\n\n## Goal\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Rule{Schema: "schema.md"}
	f := newTestFile(t, filepath.Join(dir, "doc.md"),
		"---\ntitle: test\n---\n# Title\n\n## Goal\n")
	f.SetRootDir(dir)

	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_SchemaRejectsAbsolutePathWithRootFS(t *testing.T) {
	dir := t.TempDir()
	r := &Rule{Schema: "/etc/passwd"}
	f := newTestFile(t, filepath.Join(dir, "doc.md"), "# Title\n")
	f.SetRootDir(dir)

	diags := r.Check(f)
	require.Len(t, diags, 1)
	require.Contains(t, diags[0].Message, "absolute schema path not allowed")
}

func TestCheck_SchemaRejectsParentTraversalWithRootFS(t *testing.T) {
	dir := t.TempDir()
	r := &Rule{Schema: "../escape/schema.md"}
	f := newTestFile(t, filepath.Join(dir, "doc.md"), "# Title\n")
	f.SetRootDir(dir)

	diags := r.Check(f)
	require.Len(t, diags, 1)
	require.Contains(t, diags[0].Message, "escapes project root")
}
