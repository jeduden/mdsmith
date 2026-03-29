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

// writeTmpl writes template content to a temp file and
// returns its path.
func writeTmpl(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "tmpl.md")
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
	if r.Template != "foo.md" {
		t.Errorf(
			"expected Template foo.md, got %s", r.Template,
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
// No-op when template is empty
// =====================================================================

func TestCheck_NoTemplateIsNoop(t *testing.T) {
	r := &Rule{Template: ""}
	f := newTestFile(t, "doc.md", "# Hello\n\nSome text.\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Template parsing
// =====================================================================

func TestParseTemplate_Headings(t *testing.T) {
	tmplSrc := `# ?

## Settings

## Examples

### Good

### Bad
`
	tmpl, err := parseTemplate([]byte(tmplSrc), "")
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

func TestParseTemplate_SyncPoints(t *testing.T) {
	tmplSrc := `# {id}: {name}

{description}
`
	tmpl, err := parseTemplate([]byte(tmplSrc), "")
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

func TestParseTemplate_StrictOrder(t *testing.T) {
	tmplSrc := `# ?

## Goal

## Tasks

## Acceptance Criteria
`
	tmpl, err := parseTemplate([]byte(tmplSrc), "")
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
	tmplPath := writeTmpl(t,
		"# ?\n\n## Settings\n\n## Examples\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md", "# My Rule\n\n## Examples\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `missing required section "## Settings"`)
}

func TestCheck_ExtraSectionForbidden(t *testing.T) {
	tmplPath := writeTmpl(t, "# ?\n\n## Goal\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"# My Plan\n\n## Prerequisites\n\n## Goal\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `unexpected section "## Prerequisites"`)
}

func TestCheck_SectionWildcardAllowsExtras(t *testing.T) {
	tmplPath := writeTmpl(t, "# ?\n\n## Goal\n\n## ...\n\n## Settings\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"# My Rule\n\n## Goal\n\n## Overview\n\n## Notes\n\n## Settings\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_SectionWildcardAllowsTrailingExtras(t *testing.T) {
	tmplPath := writeTmpl(t, "# ?\n\n## Goal\n\n## ...\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"# My Rule\n\n## Goal\n\n## Notes\n\n## Risks\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_WrongLevel(t *testing.T) {
	tmplPath := writeTmpl(t, "# ?\n\n## Settings\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"# My Rule\n\n### Settings\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "heading level mismatch")
}

func TestCheck_AllPresent(t *testing.T) {
	tmplPath := writeTmpl(t,
		"# ?\n\n## Settings\n\n## Examples\n\n### Good\n\n### Bad\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"# MDS001: line-length\n\n## Settings\n\n## Examples\n\n### Good\n\n### Bad\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Frontmatter-body sync
// =====================================================================

func TestCheck_HeadingSyncMismatch(t *testing.T) {
	tmplPath := writeTmpl(t, "# {id}: {name}\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nid: MDS001\nname: line-length\n---\n# MDS002: line-length\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "heading does not match frontmatter")
}

func TestCheck_HeadingSyncMatch(t *testing.T) {
	tmplPath := writeTmpl(t, "# {id}: {name}\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nid: MDS001\nname: line-length\n---\n# MDS001: line-length\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_BodySyncMismatch(t *testing.T) {
	tmplPath := writeTmpl(t, "# ?\n\n{description}\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\ndescription: Line exceeds maximum length.\n---\n# My Rule\n\nWrong description here.\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`body does not match frontmatter field "description"`)
}

func TestCheck_BodySyncMatch(t *testing.T) {
	tmplPath := writeTmpl(t, "# ?\n\n{description}\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\ndescription: Line exceeds maximum length.\n---\n# My Rule\n\nLine exceeds maximum length.\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_WildcardHeading(t *testing.T) {
	tmplPath := writeTmpl(t, "# ?\n\n## Goal\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"# Any Title Works\n\n## Goal\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Nested front-matter access (CUE paths)
// =====================================================================

func TestCheck_NestedFieldInHeading(t *testing.T) {
	tmplPath := writeTmpl(t, "# {params.subtitle}\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nparams:\n  subtitle: Overview\n---\n# Overview\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_NestedFieldInHeadingMismatch(t *testing.T) {
	tmplPath := writeTmpl(t, "# {params.subtitle}\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nparams:\n  subtitle: Overview\n---\n# Wrong Title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "heading does not match frontmatter")
}

func TestCheck_QuotedKeyInHeading(t *testing.T) {
	tmplPath := writeTmpl(t, "# {\"my-key\"}\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nmy-key: value\n---\n# value\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_NestedFieldInBody(t *testing.T) {
	tmplPath := writeTmpl(t, "# ?\n\n{params.description}\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nparams:\n  description: A detailed overview.\n---\n# My Doc\n\nA detailed overview.\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_NestedFieldInBodyMismatch(t *testing.T) {
	tmplPath := writeTmpl(t, "# ?\n\n{params.description}\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nparams:\n  description: A detailed overview.\n---\n# My Doc\n\nWrong content.\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`body does not match frontmatter field "params.description"`)
}

func TestCheck_MissingNestedKeyEmitsDiagnostic(t *testing.T) {
	// Template references {params.missing} but front matter has params.subtitle.
	tmplPath := writeTmpl(t, "# {params.missing}\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nparams:\n  subtitle: Overview\n---\n# Overview\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "missing or invalid frontmatter path")
}

func TestCheck_MissingNestedKeyInBodyEmitsDiagnostic(t *testing.T) {
	tmplPath := writeTmpl(t, "# ?\n\n{params.missing}\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nparams:\n  subtitle: Overview\n---\n# My Doc\n\nSome content.\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "missing or invalid frontmatter path")
}

// =====================================================================
// Embedded front matter CUE schema
// =====================================================================

func TestCheck_FrontMatterCUESchemaMatch(t *testing.T) {
	tmplPath := writeTmpl(t, `---
id: 'int & >=1'
status: '"🔲" | "🔳" | "✅"'
---
# ?
`)
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nid: 40\nstatus: \"✅\"\n---\n# Any title\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_FrontMatterCUESchemaMismatch(t *testing.T) {
	tmplPath := writeTmpl(t, `---
id: 'int & >=1'
status: '"🔲" | "🔳" | "✅"'
---
# ?
`)
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nid: 40\n---\n# Any title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		"front matter does not satisfy template CUE schema")
}

func TestCheck_FrontMatterCUESchemaInvalidStatus(t *testing.T) {
	tmplPath := writeTmpl(t, `---
id: 'int & >=1'
status: '"🔲" | "🔳" | "✅"'
---
# ?
`)
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nid: 40\nstatus: in-progress\n---\n# Any title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		"front matter does not satisfy template CUE schema")
}

func TestCheck_FrontMatterCUESchemaRejectsExtraFields(t *testing.T) {
	tmplPath := writeTmpl(t, `---
id: 'int & >=1'
status: '"🔲" | "🔳" | "✅"'
---
# ?
`)
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nid: 40\nstatus: \"✅\"\nextra: true\n---\n# Any title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		"front matter does not satisfy template CUE schema")
}

func TestCheck_InvalidTemplateFrontMatterCUESchema(t *testing.T) {
	tmplPath := writeTmpl(t, `---
id: 'int &'
---
# ?
`)
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md", "# Any title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "invalid template")
}

func TestCheck_TemplateKeyInFrontmatterAsCUESchema(t *testing.T) {
	// template is no longer a reserved key — it's a regular CUE schema field.
	tmplPath := writeTmpl(t, `---
template: 'string'
---
# ?
`)
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\ntemplate: my-value\n---\n# Any title\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Optional frontmatter fields (key? suffix)
// =====================================================================

func TestCheck_OptionalFieldPresent(t *testing.T) {
	tmplPath := writeTmpl(t, `---
name: 'string & != ""'
"description?": 'string'
---
# ?
`)
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nname: my-skill\ndescription: A helpful skill.\n---\n# My Skill\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_OptionalFieldAbsent(t *testing.T) {
	tmplPath := writeTmpl(t, `---
name: 'string & != ""'
"description?": 'string'
---
# ?
`)
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nname: my-skill\n---\n# My Skill\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_OptionalFieldRejectsExtraFields(t *testing.T) {
	tmplPath := writeTmpl(t, `---
name: 'string & != ""'
"description?": 'string'
---
# ?
`)
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nname: my-skill\nunknown: true\n---\n# My Skill\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		"front matter does not satisfy template CUE schema")
}

func TestCheck_OptionalFieldInvalidType(t *testing.T) {
	tmplPath := writeTmpl(t, `---
name: 'string & != ""'
"user-invocable?": bool
---
# ?
`)
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nname: my-skill\nuser-invocable: not-a-bool\n---\n# My Skill\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		"front matter does not satisfy template CUE schema")
}

// =====================================================================
// Filename validation (<?require filename?> directive)
// =====================================================================

func TestCheck_FilenamePatternMatch(t *testing.T) {
	tmplPath := writeTmpl(t, `<?require
filename: "[0-9]*_*.md"
?>
# ?
`)
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "50_my-plan.md", "# My Plan\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_FilenamePatternMismatch(t *testing.T) {
	tmplPath := writeTmpl(t, `<?require
filename: "[0-9]*_*.md"
?>
# ?
`)
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "my-plan.md", "# My Plan\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`filename "my-plan.md" does not match required pattern`)
}

func TestCheck_FilenamePatternSingleLinePI(t *testing.T) {
	tmplPath := writeTmpl(t, `<?require filename: "[0-9]*_*.md" ?>
# ?
`)
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "my-plan.md", "# My Plan\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`filename "my-plan.md" does not match required pattern`)
}

func TestCheck_FilenamePatternPIWithTrailingContent(t *testing.T) {
	tmplPath := writeTmpl(t, "<?require filename: \"[0-9]*_*.md\" ?>trailing\n# ?\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "my-plan.md", "# My Plan\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`filename "my-plan.md" does not match required pattern`)
}

func TestCheck_FilenamePatternIndentedPI(t *testing.T) {
	tmplPath := writeTmpl(t, `  <?require filename: "[0-9]*_*.md" ?>
# ?
`)
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "my-plan.md", "# My Plan\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`filename "my-plan.md" does not match required pattern`)
}

func TestCheck_FilenamePatternNotSet(t *testing.T) {
	tmplPath := writeTmpl(t, "# ?\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "anything.md", "# Title\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Template file skipping
// =====================================================================

// =====================================================================
// Schema composition via <?include?>
// =====================================================================

func TestParseTemplate_SchemaInclude(t *testing.T) {
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

	tmpl, err := parseTemplate([]byte(schema), schemaPath)
	require.NoError(t, err)
	require.Len(t, tmpl.Headings, 3)
	assert.Equal(t, "?", tmpl.Headings[0].Text)
	assert.Equal(t, "Goal", tmpl.Headings[1].Text)
	assert.Equal(t, "Acceptance Criteria", tmpl.Headings[2].Text)
}

func TestParseTemplate_SchemaIncludeRequireMerge(t *testing.T) {
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

	tmpl, err := parseTemplate([]byte(schema), schemaPath)
	require.NoError(t, err)
	assert.Equal(t, `[0-9]*_*.md`, tmpl.Config.FilenamePattern)
	require.Len(t, tmpl.Headings, 3)
	assert.Equal(t, "Tasks", tmpl.Headings[2].Text)
}

func TestParseTemplate_SchemaIncludeIgnoresFragmentFM(t *testing.T) {
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

	tmpl, err := parseTemplate([]byte(schema), schemaPath)
	require.NoError(t, err)
	// CUE schema should only come from root, not fragment.
	assert.Contains(t, tmpl.Config.FrontMatterCUE, "id")
	require.Len(t, tmpl.Headings, 2)
	assert.Equal(t, "Extra", tmpl.Headings[1].Text)
}

func TestParseTemplate_SchemaIncludeCycleDetected(t *testing.T) {
	dir := t.TempDir()
	// Schema includes itself.
	schema := "# ?\n\n<?include\nfile: schema.md\n?>\n"
	schemaPath := filepath.Join(dir, "schema.md")
	require.NoError(t, os.WriteFile(schemaPath, []byte(schema), 0o644))

	_, err := parseTemplate([]byte(schema), schemaPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cyclic include")
}

func TestParseTemplate_SchemaIncludeIndirectCycle(t *testing.T) {
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

	_, err := parseTemplate([]byte(schema), schemaPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cyclic include")
}

// =====================================================================
// <?require?> in non-schema file warning
// =====================================================================

func TestCheck_RequireInNonSchemaFileWarns(t *testing.T) {
	tmplPath := writeTmpl(t, "# ?\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"<?require\nfilename: \"*.md\"\n?>\n# My Doc\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		"<?require?> is only recognized in schema files; this directive has no effect here")
}

func TestCheck_RequireInNonSchemaFileNoTemplateSet(t *testing.T) {
	r := &Rule{Template: ""}
	f := newTestFile(t, "doc.md",
		"<?require\nfilename: \"*.md\"\n?>\n# My Doc\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		"<?require?> is only recognized in schema files; this directive has no effect here")
}

func TestCheck_RequireInSchemaFileNoWarning(t *testing.T) {
	tmplPath := writeTmpl(t,
		"<?require\nfilename: \"*.md\"\n?>\n# ?\n")
	r := &Rule{Template: tmplPath}
	// Check the schema file itself — should not warn.
	f := newTestFile(t, tmplPath,
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
// Template file skipping
// =====================================================================

func TestCheck_SkipsTemplateFiles(t *testing.T) {
	tmplPath := writeTmpl(t, "# ?\n\n## Goal\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, tmplPath, "# ?\n\n## Settings\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}
