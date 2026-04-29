package requiredstructure

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

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

// =====================================================================
// Phase 4 coverage: cueExprForValue
// =====================================================================

func TestCueExprForValue_SliceArray(t *testing.T) {
	expr, err := cueExprForValue([]any{1, "hello", true})
	require.NoError(t, err)
	assert.Equal(t, `[1,"hello",true]`, expr)
}

func TestCueExprForValue_Array(t *testing.T) {
	expr, err := cueExprForValue([]any{"a", "b"})
	require.NoError(t, err)
	assert.Equal(t, `["a","b"]`, expr)
}

func TestCueExprForValue_MapStringAny(t *testing.T) {
	expr, err := cueExprForValue(map[string]any{"key": "string"})
	require.NoError(t, err)
	assert.Contains(t, expr, "key")
}

func TestCueExprForValue_String(t *testing.T) {
	expr, err := cueExprForValue("string")
	require.NoError(t, err)
	assert.Equal(t, "string", expr)
}

func TestCueExprForValue_EmptyString(t *testing.T) {
	_, err := cueExprForValue("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty")
}

func TestCueExprForValue_WhitespaceString(t *testing.T) {
	_, err := cueExprForValue("  ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty")
}

func TestCueExprForValue_Int(t *testing.T) {
	expr, err := cueExprForValue(42)
	require.NoError(t, err)
	assert.Equal(t, "42", expr)
}

func TestCueExprForValue_Bool(t *testing.T) {
	expr, err := cueExprForValue(true)
	require.NoError(t, err)
	assert.Equal(t, "true", expr)
}

func TestCueExprForValue_UnsupportedType(t *testing.T) {
	_, err := cueExprForValue(uint(42))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestCueExprForValue_UnsupportedStruct(t *testing.T) {
	_, err := cueExprForValue(struct{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported schema value type")
}

// =====================================================================
// Phase 4 coverage: extractYAML
// =====================================================================

func TestExtractYAML_NormalCase(t *testing.T) {
	input := []byte("---\nkey: value\n---\n")
	result := extractYAML(input)
	assert.Equal(t, []byte("key: value\n"), result)
}

func TestExtractYAML_Normal(t *testing.T) {
	input := []byte("---\ntitle: hello\nauthor: world\n---\n")
	got := extractYAML(input)
	assert.Equal(t, "title: hello\nauthor: world\n", string(got))
}

func TestExtractYAML_ClosingWithoutNewline(t *testing.T) {
	input := []byte("---\nkey: value\n---")
	result := extractYAML(input)
	assert.Equal(t, []byte("key: value\n"), result)
}

func TestExtractYAML_NoTrailingNewline(t *testing.T) {
	input := []byte("---\ntitle: hello\n---")
	got := extractYAML(input)
	assert.Equal(t, "title: hello\n", string(got))
}

func TestExtractYAML_NoClosingDelimiter(t *testing.T) {
	input := []byte("---\nkey: value\n")
	result := extractYAML(input)
	assert.Nil(t, result)
}

func TestExtractYAML_UnclosedFrontMatter(t *testing.T) {
	input := []byte("---\ntitle: hello\n")
	got := extractYAML(input)
	assert.Nil(t, got, "unclosed front matter should return nil")
}

// =====================================================================
// Phase 4 coverage: writeNodeText via headingText (CodeSpan branch)
// =====================================================================

func TestHeadingText_WithCodeSpan(t *testing.T) {
	f := newTestFile(t, "doc.md", "# Heading with `code`\n")
	headings := extractHeadings(f)
	require.Len(t, headings, 1)
	assert.Equal(t, "Heading with code", headings[0].Text)
}

// =====================================================================
// Phase 4 coverage: advanceToMatch
// =====================================================================

func TestAdvanceToMatch_NoMatch(t *testing.T) {
	req := schemaHeading{Level: 2, Text: "Nonexistent"}
	headings := []docHeading{
		{Level: 1, Text: "Intro", Line: 1},
		{Level: 2, Text: "Details", Line: 3},
	}
	matched, nextIdx := advanceToMatch(req, headings, 0)
	assert.Equal(t, -1, matched)
	assert.Equal(t, 2, nextIdx)
}

func TestAdvanceToMatch_EmptyList(t *testing.T) {
	req := schemaHeading{Level: 2, Text: "Test"}
	matched, nextIdx := advanceToMatch(req, nil, 0)
	assert.Equal(t, -1, matched)
	assert.Equal(t, 0, nextIdx)
}

// =====================================================================
// Phase 4 coverage: extractPIFileParam multi-line
// =====================================================================

func TestExtractPIFileParam_MultiLine(t *testing.T) {
	src := "<?include\nfile: other.md\n?>"
	f, err := lint.NewFileFromSource("schema.md", []byte(src), true)
	require.NoError(t, err)
	var pi *lint.ProcessingInstruction
	for c := f.AST.FirstChild(); c != nil; c = c.NextSibling() {
		if p, ok := c.(*lint.ProcessingInstruction); ok {
			pi = p
			break
		}
	}
	require.NotNil(t, pi, "expected ProcessingInstruction in parsed AST")
	result, err := extractPIFileParam(pi, []byte(src))
	require.NoError(t, err)
	assert.Equal(t, "other.md", result)
}

// =====================================================================
// Plan 61 hardening: additional edge-case tests
// =====================================================================

// Wildcard heading (`# ?`) must still enforce the correct level.
// A document h2 where h1 is required produces a level-mismatch diagnostic.
func TestCheck_WildcardHeadingLevelMismatch(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md", "## Title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `heading level mismatch for "Title": expected h1, got h2`)
}

// Soft-wrapped body paragraph (multiple lines joined by space) must
// match the front matter field value when concatenated.
func TestCheck_BodySyncSoftWrapped(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n{description}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\ndescription: Line exceeds maximum length.\n---\n# My Rule\n\n"+
			"Line exceeds\nmaximum length.\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// The improved body sync diagnostic must include the expected value so
// authors know what text to write.
func TestCheck_BodySyncDiagnosticIncludesExpected(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n{description}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\ndescription: Correct description.\n---\n# My Rule\n\nWrong text.\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `expected "Correct description."`)
}

// Integer front matter values are stringified for heading sync.
func TestCheck_SyncIntegerFrontMatterValue(t *testing.T) {
	schemaPath := writeSchema(t, "# {id}: {name}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nid: 42\nname: line-length\n---\n# 42: line-length\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// When a synced heading is absent from the document, checkSync must
// not emit a spurious diagnostic; only checkStructure reports it.
func TestCheck_SyncNotFiredForMissingHeading(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## {title}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\ntitle: My Section\n---\n# Title\n")
	diags := r.Check(f)
	// Exactly one diagnostic: missing required section, no sync error.
	require.Len(t, diags, 1)
	expectDiagMsg(t, diags, "missing required section")
	for _, d := range diags {
		assert.NotContains(t, d.Message, "sync")
	}
}

// When several required sections are all absent, each gets its own
// "missing required section" diagnostic.
func TestCheck_MultipleMissingSections(t *testing.T) {
	schemaPath := writeSchema(t,
		"# ?\n\n## Goal\n\n## Tasks\n\n## Acceptance Criteria\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md", "# Title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `missing required section "## Goal"`)
	expectDiagMsg(t, diags, `missing required section "## Tasks"`)
	expectDiagMsg(t, diags, `missing required section "## Acceptance Criteria"`)
}

// A section that is both out of order AND at the wrong level must
// produce both the out-of-order and the level-mismatch diagnostic.
func TestCheck_OutOfOrderAlsoReportsLevelMismatch(t *testing.T) {
	schemaPath := writeSchema(t,
		"# ?\n\n## Goal\n\n## Tasks\n")
	r := &Rule{Schema: schemaPath}
	// Tasks (h2) appears before Goal; Goal appears at h3 (wrong level).
	f := newTestFile(t, "doc.md",
		"# Title\n\n## Tasks\n\n### Goal\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `out of order`)
	expectDiagMsg(t, diags, `heading level mismatch`)
}

// =====================================================================
// Phase 5 coverage: additional branch coverage
// =====================================================================

// deriveFrontMatterCUE: empty map → return "", nil
func TestDeriveFrontMatterCUE_EmptyMap(t *testing.T) {
	// "{}" unmarshals to an empty map → len(raw)==0 branch.
	result, err := deriveFrontMatterCUE([]byte("{}\n"))
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

// deriveFrontMatterCUE: cueExprForMap error via null YAML value
func TestDeriveFrontMatterCUE_NullValueError(t *testing.T) {
	// YAML null (represented as nil in Go) is not a supported schema type.
	_, err := deriveFrontMatterCUE([]byte("key: ~\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing schema frontmatter constraints")
}

// extractRequireDirective: single-line require with empty body → continue
func TestExtractRequireDirective_SingleLineEmpty(t *testing.T) {
	// Single-line PI with no body content after trimming.
	src := "<?require ?>\n# Title\n"
	f := newTestFile(t, "schema.md", src)
	result, err := extractRequireDirective(f)
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

// extractPIFileParam: single-line form
func TestExtractPIFileParam_SingleLine(t *testing.T) {
	// Single-line include PI: <?include file: other.md ?>
	src := "<?include file: other.md ?>\ncontent\n<?/include?>"
	f, err := lint.NewFileFromSource("schema.md", []byte(src), true)
	require.NoError(t, err)
	var pi *lint.ProcessingInstruction
	for c := f.AST.FirstChild(); c != nil; c = c.NextSibling() {
		if p, ok := c.(*lint.ProcessingInstruction); ok {
			pi = p
			break
		}
	}
	require.NotNil(t, pi, "expected ProcessingInstruction in parsed AST")
	result, err := extractPIFileParam(pi, []byte(src))
	require.NoError(t, err)
	assert.Equal(t, "other.md", result)
}

// resolveSchemaIncludePath: empty file param
func TestResolveSchemaIncludePath_EmptyFileParam(t *testing.T) {
	// include with empty file param → error
	src := "<?include\nfile: \"\"\n?>\ncontent\n<?/include?>"
	f, err := lint.NewFileFromSource("schema.md", []byte(src), true)
	require.NoError(t, err)
	var pi *lint.ProcessingInstruction
	for c := f.AST.FirstChild(); c != nil; c = c.NextSibling() {
		if p, ok := c.(*lint.ProcessingInstruction); ok {
			pi = p
			break
		}
	}
	require.NotNil(t, pi, "expected ProcessingInstruction in parsed AST")
	_, err = resolveSchemaIncludePath(pi, []byte(src), "schema.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required 'file' attribute")
}

// resolveSchemaIncludePath: absolute path → error
func TestResolveSchemaIncludePath_AbsolutePath(t *testing.T) {
	src := "<?include\nfile: /abs/path.md\n?>\ncontent\n<?/include?>"
	f, err := lint.NewFileFromSource("schema.md", []byte(src), true)
	require.NoError(t, err)
	var pi *lint.ProcessingInstruction
	for c := f.AST.FirstChild(); c != nil; c = c.NextSibling() {
		if p, ok := c.(*lint.ProcessingInstruction); ok {
			pi = p
			break
		}
	}
	require.NotNil(t, pi)
	_, err = resolveSchemaIncludePath(pi, []byte(src), "schema.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute file path")
}

// resolveSchemaIncludePath: path with .. traversal → error
func TestResolveSchemaIncludePath_DotDotTraversal(t *testing.T) {
	src := "<?include\nfile: ../parent.md\n?>\ncontent\n<?/include?>"
	f, err := lint.NewFileFromSource("schema.md", []byte(src), true)
	require.NoError(t, err)
	var pi *lint.ProcessingInstruction
	for c := f.AST.FirstChild(); c != nil; c = c.NextSibling() {
		if p, ok := c.(*lint.ProcessingInstruction); ok {
			pi = p
			break
		}
	}
	require.NotNil(t, pi)
	_, err = resolveSchemaIncludePath(pi, []byte(src), "schema.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `".." traversal`)
}

// expandSchemaInclude: depth exceeds maximum
func TestParseSchema_SchemaIncludeDepthExceeded(t *testing.T) {
	dir := t.TempDir()
	// Build a long chain: schema → d1 → d2 → ... → d(maxDepth+1)
	// Each fragment includes the next one.
	const depth = maxSchemaIncludeDepth + 1
	for i := depth; i >= 1; i-- {
		var content string
		if i == depth {
			content = "## Leaf\n"
		} else {
			content = fmt.Sprintf("## Level%d\n\n<?include\nfile: d%d.md\n?>\n", i, i+1)
		}
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, fmt.Sprintf("d%d.md", i)),
			[]byte(content), 0o644,
		))
	}
	schema := "# ?\n\n<?include\nfile: d1.md\n?>\n"
	schemaPath := filepath.Join(dir, "schema.md")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schema), 0o644))

	_, err := parseSchema([]byte(schema), schemaPath, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "depth exceeds maximum")
}

// expandSchemaInclude: ReadFileLimited error (file not found)
func TestParseSchema_SchemaIncludeMissingFile(t *testing.T) {
	dir := t.TempDir()
	schema := "# ?\n\n<?include\nfile: nonexistent.md\n?>\n"
	schemaPath := filepath.Join(dir, "schema.md")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schema), 0o644))

	_, err := parseSchema([]byte(schema), schemaPath, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read schema include file")
}

// expandSchemaInclude: fp2 propagation — include's include has a require directive
func TestParseSchema_SchemaIncludeNestedRequirePropagated(t *testing.T) {
	dir := t.TempDir()
	// frag2.md has a <?require?> that sets filename pattern.
	frag2 := "<?require\nfilename: \"nested-*.md\"\n?>\n## Nested\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "frag2.md"), []byte(frag2), 0o644))

	// frag1.md includes frag2.md but has no require.
	frag1 := "## Level1\n\n<?include\nfile: frag2.md\n?>\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "frag1.md"), []byte(frag1), 0o644))

	// schema includes frag1.
	schema := "# ?\n\n<?include\nfile: frag1.md\n?>\n"
	schemaPath := filepath.Join(dir, "schema.md")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schema), 0o644))

	tmpl, err := parseSchema([]byte(schema), schemaPath, 0)
	require.NoError(t, err)
	// The filename pattern from the nested include should propagate up.
	assert.Equal(t, "nested-*.md", tmpl.Config.FilenamePattern)
}

// validateFrontMatterCUE: invalid CUE schema error
func TestValidateFrontMatterCUE_InvalidSchema(t *testing.T) {
	err := validateFrontMatterCUE("this is not valid CUE {{{{", map[string]any{"key": "val"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid CUE schema")
}

// validateFrontMatterCUE: type mismatch between schema and front-matter value.
func TestValidateFrontMatterCUE_TypeMismatch(t *testing.T) {
	err := validateFrontMatterCUE(`close({id: string})`, map[string]any{"id": 42})
	require.Error(t, err) // CUE unification fails: int != string
}

// readDocFrontMatterRaw: extractYAML returns nil when FrontMatter has no closing delimiter
func TestReadDocFrontMatterRaw_ExtractYAMLNil(t *testing.T) {
	// Manually set FrontMatter to content without proper --- delimiter pair.
	f := &lint.File{FrontMatter: []byte("no-closing-delimiter content")}
	raw, diags := readDocFrontMatterRaw(f)
	assert.Nil(t, raw)
	assert.Nil(t, diags)
}

// checkBodySync: headingIdx+1 < len(allHeadings) constrains endLine
func TestCheck_BodySyncWithFollowingHeading(t *testing.T) {
	// Schema: two headings, first has body sync.
	// Doc has body text followed by a second heading.
	schemaPath := writeSchema(t, "# ?\n\n{description}\n\n## Details\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\ndescription: Expected body text.\n---\n# Title\n\nExpected body text.\n\n## Details\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// checkBodySync: paragraph match (multi-line body matching joined text)
func TestCheck_BodySyncParagraphMatch(t *testing.T) {
	// Body is wrapped across two lines but together matches the expected value.
	schemaPath := writeSchema(t, "# ?\n\n{description}\n")
	r := &Rule{Schema: schemaPath}
	// The body content spans two lines that join to match the description.
	f := newTestFile(t, "doc.md",
		"---\ndescription: First line second line.\n---\n# Title\n\nFirst line\nsecond line.\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// checkFilenamePattern: invalid glob pattern returns diagnostic
func TestCheck_FilenamePatternInvalidGlob(t *testing.T) {
	// The schema require directive has an invalid glob pattern "[" which filepath.Match rejects.
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.md")
	require.NoError(t, os.WriteFile(schemaPath,
		[]byte("<?require\nfilename: \"[\"\n?>\n# ?\n"), 0o644))
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md", "# Title\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "invalid filename pattern")
}

// checkSync: isSectionWildcard continue branch
func TestCheck_SyncWithSectionWildcard(t *testing.T) {
	// Schema has a wildcard section (...) before a sync heading.
	// The wildcard section is skipped in sync checking.
	schemaPath := writeSchema(t, "# {title}\n\n## ...\n\n## Summary\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\ntitle: My Doc\n---\n# My Doc\n\n## Optional\n\n## Summary\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// checkSync: matchedDoc < 0 (section heading not found in doc)
func TestCheck_SyncHeadingNotFoundInDoc(t *testing.T) {
	// Schema has heading {id} but doc doesn't have that heading at all.
	// checkSync should skip gracefully (matchedDoc < 0).
	schemaPath := writeSchema(t, "# ?\n\n## {id}\n")
	r := &Rule{Schema: schemaPath}
	// Doc has first heading but not the second.
	f := newTestFile(t, "doc.md",
		"---\nid: MDS001\n---\n# My Doc\n")
	diags := r.Check(f)
	// Should report missing required section, not panic.
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "missing required section")
}

// checkSyncPoint: invalid CUE path (call directly since the fieldPattern
// guards make this unreachable through normal schema parsing)
func TestCheckSyncPoint_InvalidCUEPath(t *testing.T) {
	f := newTestFile(t, "doc.md", "---\nfoo: bar\n---\n# My Title\n")
	sp := syncPoint{Field: ""} // empty string → ParseCUEPath returns nil
	req := schemaHeading{Level: 1, Text: "My Title"}
	dh := docHeading{Level: 1, Text: "My Title", Line: 1}
	diags := checkSyncPoint(f, sp, req, dh, 0, nil, nil)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "invalid CUE path")
}

// writeNodeText: recursive fallthrough branch
func TestHeadingText_WithLink(t *testing.T) {
	// A heading with a link node exercises the recursive fallthrough.
	f := newTestFile(t, "doc.md", "# Heading with [link text](url)\n")
	headings := extractHeadings(f)
	require.Len(t, headings, 1)
	assert.Contains(t, headings[0].Text, "link text")
}

// matchRequired: level mismatch for out-of-order heading
func TestCheck_OutOfOrderSectionLevelMismatch(t *testing.T) {
	// Schema requires ## Alpha then ## Beta.
	// Doc has ### Beta (out of order AND wrong level) then ## Alpha.
	schemaPath := writeSchema(t, "## Alpha\n\n## Beta\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md", "### Beta\n\n## Alpha\n")
	diags := r.Check(f)
	require.NotEmpty(t, diags)
	// Should see both an out-of-order and a level-mismatch diagnostic.
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "out of order") || strings.Contains(d.Message, "level mismatch") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected out-of-order or level-mismatch diagnostic, got: %v", diags)
}

// validateCUESchemaSyntax: with a valid non-empty schema (and also invalid CUE)
func TestValidateCUESchemaSyntax_InvalidCUE(t *testing.T) {
	err := validateCUESchemaSyntax("{{{not valid CUE}}}")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid schema frontmatter CUE")
}

// --- Placeholder tests ---

func TestCheck_Placeholder_CUEFrontmatter_Suppressed(t *testing.T) {
	// When cue-frontmatter is configured, the front-matter CUE validation
	// is skipped, so a document with CUE-pattern front-matter values passes.
	schemaPath := writeSchema(t, `---
id: 'int & >=1'
status: '"🔲" | "🔳" | "✅"'
---
# ?
`)
	r := &Rule{Schema: schemaPath, Placeholders: []string{"cue-frontmatter"}}
	// The document's front matter contains CUE expressions, not concrete values.
	f := newTestFile(t, "proto.md",
		"---\nid: 'int & >=1'\nstatus: '\"🔲\" | \"🔳\" | \"✅\"'\n---\n# Template\n")
	diags := r.Check(f)
	require.Empty(t, diags, "expected no diagnostics when cue-frontmatter placeholder is configured")
}

func TestCheck_Placeholder_CUEFrontmatter_EmptyList_StillValidates(t *testing.T) {
	// Without cue-frontmatter in placeholders, CUE validation runs normally.
	schemaPath := writeSchema(t, `---
id: 'int & >=1'
status: '"🔲" | "🔳" | "✅"'
---
# ?
`)
	r := &Rule{Schema: schemaPath, Placeholders: []string{}}
	// Document with a CUE expression as a value violates the int constraint.
	f := newTestFile(t, "doc.md",
		"---\nid: 'int & >=1'\nstatus: \"✅\"\n---\n# Title\n")
	diags := r.Check(f)
	var hasCUEDiag bool
	for _, d := range diags {
		if strings.Contains(d.Message, "CUE constraints") {
			hasCUEDiag = true
		}
	}
	assert.True(t, hasCUEDiag, "CUE constraint violation should be reported without placeholder")
}

func TestApplySettings_Placeholders_RequiredStructure(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"placeholders": []any{"cue-frontmatter"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"cue-frontmatter"}, r.Placeholders)
}

func TestApplySettings_Placeholders_UnknownToken_RequiredStructure(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"placeholders": []any{"bad"}})
	require.Error(t, err)
}

func TestApplySettings_Placeholders_NonList_RequiredStructure(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"placeholders": "not-a-list"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "list of strings")
}

func TestDefaultSettings_RequiredStructure_IncludesPlaceholders(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	require.Equal(t, []string{}, ds["placeholders"])
}

func TestSettingMergeMode_RequiredStructure(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeAppend, r.SettingMergeMode("placeholders"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("schema"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("unknown"))
}
