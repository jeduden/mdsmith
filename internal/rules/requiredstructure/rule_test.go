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

// expectRelatedMsg asserts that some diagnostic carries a related
// location whose Message contains substr. Since plan 230, the schema
// reference rides on RelatedLocations rather than the message body, so
// schema-ref assertions go through this helper.
func expectRelatedMsg(
	t *testing.T, diags []lint.Diagnostic, substr string,
) {
	t.Helper()
	for _, d := range diags {
		for _, rl := range d.RelatedLocations {
			if strings.Contains(rl.Message, substr) {
				return
			}
		}
	}
	t.Errorf("no related location contains %q", substr)
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
	if r.Category() != "structural" {
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

func TestApplySettings_RemovedArchetypeSettings(t *testing.T) {
	for _, key := range []string{"archetype", "archetype-roots"} {
		t.Run(key, func(t *testing.T) {
			r := &Rule{}
			err := r.ApplySettings(map[string]any{key: "anything"})
			require.Error(t, err)
			require.Contains(t, err.Error(), "has been removed")
			require.Contains(t, err.Error(), "kinds")
		})
	}
}

func TestApplySettings_BareNameSchemaIsRejected(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"schema": "story"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "looks like a bare name")
	require.Contains(t, err.Error(), "kinds")
}

func TestApplySettings_PathSchemaIsAccepted(t *testing.T) {
	for _, s := range []string{
		"schemas/story.md",
		"./story.md",
		"story.md",
		"plan/proto.md",
	} {
		t.Run(s, func(t *testing.T) {
			r := &Rule{}
			require.NoError(t, r.ApplySettings(map[string]any{"schema": s}))
			require.Equal(t, s, r.Schema)
		})
	}
}

func TestApplySettings_EmptySchemaIsAccepted(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"schema": ""}))
	require.Equal(t, "", r.Schema)
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
	expectDiagMsg(t, diags, `## Settings: got <missing>, expected section to be present`)
}

func TestCheck_ExtraSectionForbidden(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Goal\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"# My Plan\n\n## Prerequisites\n\n## Goal\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `## Prerequisites: got <present>, expected not declared in schema`)
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
	expectDiagMsg(t, diags, "expected h2")
}

// Level-mismatch diagnostics must name the offending heading so
// readers can locate it in documents with many headings.
func TestCheck_WrongLevel_NamesHeading(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Settings\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"# My Rule\n\n### Settings\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "Settings:")
	expectDiagMsg(t, diags, "got h3, expected h2")
}

// Unexpected-section diagnostics should tell the author which
// required heading was expected at that position.
func TestCheck_ExtraSection_NamesExpected(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Goal\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"# My Plan\n\n## Prerequisites\n\n## Goal\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `## Prerequisites: got <present>, expected not declared in schema`)
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
	expectDiagMsg(t, diags, `## Trailing: got <present>, expected not declared in schema`)
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
	expectDiagMsg(t, diags, `status: `)
	expectDiagMsg(t, diags, `expected one of: "🔲", "🔳", "✅"`)
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
	expectDiagMsg(t, diags, `status: got "in-progress"`)
	expectDiagMsg(t, diags, `expected one of: "🔲", "🔳", "✅"`)
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
	expectDiagMsg(t, diags, `extra:`)
	expectDiagMsg(t, diags, `not declared in schema`)
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
	expectDiagMsg(t, diags, `unknown:`)
	expectDiagMsg(t, diags, `not declared in schema`)
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
	expectDiagMsg(t, diags, `user-invocable`)
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
		`filename: got "my-plan.md", expected filename matching glob [0-9]*_*.md`)
}

func TestCheck_FilenamePatternSingleLinePI(t *testing.T) {
	schemaPath := writeSchema(t, `<?require filename: "[0-9]*_*.md" ?>
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "my-plan.md", "# My Plan\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`filename: got "my-plan.md", expected filename matching glob [0-9]*_*.md`)
}

func TestCheck_FilenamePatternPIWithTrailingContent(t *testing.T) {
	schemaPath := writeSchema(t, "<?require filename: \"[0-9]*_*.md\" ?>trailing\n# ?\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "my-plan.md", "# My Plan\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`filename: got "my-plan.md", expected filename matching glob [0-9]*_*.md`)
}

func TestCheck_FilenamePatternIndentedPI(t *testing.T) {
	schemaPath := writeSchema(t, `  <?require filename: "[0-9]*_*.md" ?>
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "my-plan.md", "# My Plan\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`filename: got "my-plan.md", expected filename matching glob [0-9]*_*.md`)
}

func TestCheck_FilenamePatternNotSet(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "anything.md", "# Title\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Kind-level path-pattern validation
// =====================================================================

// newRootedFile builds a *lint.File anchored at a workspace root so
// path-pattern matching has a workspace-relative path to compare
// against. The file is written under root/path so its absolute Path
// resolves correctly through filepath.Rel inside the rule.
func newRootedFile(t *testing.T, root, path, source string) *lint.File {
	t.Helper()
	abs := filepath.Join(root, path)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(source), 0o644))
	f, err := lint.NewFileFromSource(abs, []byte(source), true)
	require.NoError(t, err)
	f.SetRootDir(root)
	return f
}

// TestSchemaDataDeclaresExtends_DetectsExtends pins the happy
// path: a proto.md with `extends:` in front matter triggers the
// dispatch's compose-path re-route.
func TestSchemaDataDeclaresExtends_DetectsExtends(t *testing.T) {
	assert.True(t, schemaDataDeclaresExtends(
		[]byte("---\nextends: base.md\n---\n# ?\n")))
}

// TestSchemaDataDeclaresExtends_NoExtendsReturnsFalse covers the
// common case: a schema without inheritance stays on the legacy
// single-file path.
func TestSchemaDataDeclaresExtends_NoExtendsReturnsFalse(t *testing.T) {
	assert.False(t, schemaDataDeclaresExtends(
		[]byte("---\nid: 'string'\n---\n# ?\n")))
}

// TestSchemaDataDeclaresExtends_NullValueReturnsFalse matches
// schema.ParseFile's "no extends" treatment for `extends:` with a
// null body. The legacy single-file parser handles this case
// equivalently, so staying on the legacy path keeps diagnostics
// aligned.
func TestSchemaDataDeclaresExtends_NullValueReturnsFalse(t *testing.T) {
	assert.False(t, schemaDataDeclaresExtends(
		[]byte("---\nextends:\n---\n# ?\n")))
}

// TestSchemaDataDeclaresExtends_WhitespaceValueRoutesToCompose
// covers the malformed-but-present case: schema.ParseFile rejects
// whitespace-only extends with a clear error, so the dispatcher
// must route there instead of silently dropping the key via the
// legacy parser.
func TestSchemaDataDeclaresExtends_WhitespaceValueRoutesToCompose(t *testing.T) {
	assert.True(t, schemaDataDeclaresExtends(
		[]byte("---\nextends: \"   \"\n---\n# ?\n")))
}

// TestSchemaDataDeclaresExtends_NonStringValueRoutesToCompose
// catches typos like `extends: 42` — schema.ParseFile rejects
// these explicitly. Routing through the compose path surfaces
// that error to the user.
func TestSchemaDataDeclaresExtends_NonStringValueRoutesToCompose(t *testing.T) {
	assert.True(t, schemaDataDeclaresExtends(
		[]byte("---\nextends: 42\n---\n# ?\n")))
}

// TestCheck_MalformedExtendsSurfacesDiagnostic ensures the
// dispatch+compose path actually reports the error rather than
// swallowing it. Without this, a typoed `extends:` value would
// silently produce no parent schema.
func TestCheck_MalformedExtendsSurfacesDiagnostic(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "child.md"),
		[]byte("---\nextends: 42\n---\n"), 0o644))
	f := newRootedFile(t, root, "doc.md", "# Doc\n")
	r := &Rule{
		Schema:  "child.md",
		Sources: []SchemaSource{{File: "child.md"}},
	}
	diags := r.Check(f)
	require.NotEmpty(t, diags,
		"malformed `extends:` must surface a diagnostic, not be silently dropped")
}

// TestSchemaDataDeclaresExtends_NoFrontmatterReturnsFalse covers
// the `prefix == nil` branch: a schema file with no front matter
// has no `extends:` to report.
func TestSchemaDataDeclaresExtends_NoFrontmatterReturnsFalse(t *testing.T) {
	assert.False(t, schemaDataDeclaresExtends([]byte("# ?\n")))
}

// TestSchemaDataDeclaresExtends_InvalidYAMLReturnsFalse covers
// the YAML-decode error branch: a malformed front matter still
// returns `false` so the legacy parser surfaces the syntax error
// with its richer diagnostic.
func TestSchemaDataDeclaresExtends_InvalidYAMLReturnsFalse(t *testing.T) {
	assert.False(t, schemaDataDeclaresExtends(
		[]byte("---\n: bad: yaml:\n---\n# ?\n")))
}

// TestDispatchSingleFileSchema_MissingFileReportsDiagnostic covers
// the loadSchemaAt error path: a missing schema file surfaces as a
// single MDS020 diagnostic anchored at line 1 of the document.
func TestDispatchSingleFileSchema_MissingFileReportsDiagnostic(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, "doc.md", "# Doc\n")
	r := &Rule{}
	diags := r.dispatchSingleFileSchema(f, "nonexistent.md",
		[]SchemaSource{{File: "nonexistent.md"}})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "schema")
}

// TestCheck_FileSchemaWithExtendsRoutesThroughCompose pins the
// PR-365 review fix (Copilot comment on rule.go:1064): a single
// file source whose proto.md declares `extends:` must go through
// the modern compose path so plan-135 inheritance applies. Without
// this re-route the legacy parser would silently drop the parent's
// frontmatter constraints.
func TestCheck_FileSchemaWithExtendsRoutesThroughCompose(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "base.md"),
		[]byte("---\nid: '=~\"^A-[0-9]+$\"'\n---\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "child.md"),
		[]byte("---\nextends: base.md\n---\n"), 0o644))
	f := newRootedFile(t, root, "doc.md",
		"---\nid: \"WRONG\"\n---\n# Doc\n")
	r := &Rule{
		Schema:  "child.md",
		Sources: []SchemaSource{{File: "child.md"}},
	}
	diags := r.Check(f)
	require.NotEmpty(t, diags,
		"document violating the parent's `id` regex should flag a diagnostic via the inherited constraint")
}

func TestCheck_PathPattern_Match(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, "plan/140_my-plan.md", "# My Plan\n")
	r := &Rule{PathPatterns: []PathPattern{
		{Kind: "plan", Pattern: "plan/[0-9][0-9]*_*.md"},
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_PathPattern_Mismatch(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, "plan/early-draft.md", "# Draft\n")
	r := &Rule{PathPatterns: []PathPattern{
		{Kind: "plan", Pattern: "plan/[0-9][0-9]*_*.md"},
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	// path-pattern checks the workspace-relative path, so the
	// field label is "path" (distinct from the "filename" field
	// used for basename-only checks).
	assert.Contains(t, diags[0].Message,
		`path: got "plan/early-draft.md"`)
	assert.Contains(t, diags[0].Message,
		`glob plan/[0-9][0-9]*_*.md`)
	require.Len(t, diags[0].RelatedLocations, 1)
	assert.Equal(t, "kinds[plan] / path-pattern",
		diags[0].RelatedLocations[0].Message)
	assert.Equal(t, "MDS020", diags[0].RuleID)
	assert.Equal(t, "required-structure", diags[0].RuleName)
	assert.Equal(t, 1, diags[0].Line)
}

// TestWorkspaceRelPath_RelativeRootDirResolvesViaAbs verifies that
// workspaceRelPath handles a relative f.RootDir (produced when the
// CLI is invoked with `--config sub/.mdsmith.yml`) by resolving
// both sides through filepath.Abs first. Without this, filepath.Rel
// would fail and the function would fall back to f.Path, producing
// false MDS020 path-pattern mismatches. The test asserts the
// returned relative path directly so it does not depend on the
// process working directory.
func TestWorkspaceRelPath_RelativeRootDirResolvesViaAbs(t *testing.T) {
	cases := []struct {
		name    string
		rootDir string
		path    string
		want    string
	}{
		{
			name:    "both relative",
			rootDir: ".",
			path:    "plan/140_x.md",
			want:    "plan/140_x.md",
		},
		{
			name:    "relative root, absolute file",
			rootDir: ".",
			path:    filepath.Join(mustAbs(t, "."), "plan", "140_x.md"),
			want:    "plan/140_x.md",
		},
		{
			name:    "empty root falls back to path",
			rootDir: "",
			path:    "plan/early-draft.md",
			want:    "plan/early-draft.md",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &lint.File{Path: tc.path, RootDir: tc.rootDir}
			assert.Equal(t, tc.want, workspaceRelPath(f))
		})
	}
}

// mustAbs is a tiny helper for tests that need a known-absolute
// path without inlining error handling.
func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	require.NoError(t, err)
	return abs
}

// TestCheck_PathPattern_RootAnchoredNotBasenameOnly verifies that
// a root-anchored pattern like `README.md` is matched against the
// full workspace-relative path, not its basename. Without this,
// `docs/README.md` would pass `path-pattern: README.md`, defeating
// the documented "workspace-relative path must match" semantics.
func TestCheck_PathPattern_RootAnchoredNotBasenameOnly(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, "docs/README.md", "# Docs\n")
	r := &Rule{PathPatterns: []PathPattern{
		{Kind: "readme", Pattern: "README.md"},
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	assert.Contains(t, diags[0].Message, `got "docs/README.md"`)
}

// TestCheck_PathPattern_DoublestarWildcardMatchesNested verifies
// the doublestar-specific `**` wildcard works against the full
// workspace-relative path so a pattern like `**/README.md` still
// matches a nested file.
func TestCheck_PathPattern_DoublestarWildcardMatchesNested(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, "docs/sub/README.md", "# Sub\n")
	r := &Rule{PathPatterns: []PathPattern{
		{Kind: "readme", Pattern: "**/README.md"},
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// TestCheck_PathPattern_EmptyRootDirFallsBackToPath covers the
// `f.RootDir == ""` branch of workspaceRelPath: with no workspace
// root configured, the glob compares against f.Path verbatim.
func TestCheck_PathPattern_EmptyRootDirFallsBackToPath(t *testing.T) {
	f, err := lint.NewFileFromSource(
		"plan/140_x.md", []byte("# x\n"), true)
	require.NoError(t, err)
	r := &Rule{PathPatterns: []PathPattern{
		{Kind: "plan", Pattern: "plan/[0-9][0-9]*_*.md"},
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_PathPattern_NoPatternsIsNoOp(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, "anywhere/anything.md", "# Hi\n")
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_PathPattern_MultipleKindsBothFail(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, "docs/notes.md", "# Notes\n")
	r := &Rule{PathPatterns: []PathPattern{
		{Kind: "plan", Pattern: "plan/[0-9]*_*.md"},
		{Kind: "rfc", Pattern: "docs/rfc/RFC-*.md"},
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 2)
	expectRelatedMsg(t, diags, "kinds[plan] / path-pattern")
	expectRelatedMsg(t, diags, "kinds[rfc] / path-pattern")
}

func TestCheck_PathPattern_PlusRequireFilenameBothEmit(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "proto.md"),
		[]byte(`<?require
filename: "[0-9]*_*.md"
?>
# ?
`), 0o644))
	f := newRootedFile(t, root, "plan/my-plan.md", "# My Plan\n")
	r := &Rule{
		Schema: "proto.md",
		PathPatterns: []PathPattern{
			{Kind: "plan", Pattern: "plan/[0-9][0-9]*_*.md"},
		},
	}
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`filename: got "my-plan.md", expected filename matching glob [0-9]*_*.md`)
	expectRelatedMsg(t, diags, "kinds[plan] / path-pattern")
}

func TestApplySettings_PathPatternsParsesList(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"path-patterns": []any{
			map[string]any{"kind": "plan", "pattern": "plan/*.md"},
			map[string]any{"kind": "rfc", "pattern": "docs/rfc/*.md"},
		},
	})
	require.NoError(t, err)
	require.Len(t, r.PathPatterns, 2)
	assert.Equal(t, "plan", r.PathPatterns[0].Kind)
	assert.Equal(t, "plan/*.md", r.PathPatterns[0].Pattern)
	assert.Equal(t, "rfc", r.PathPatterns[1].Kind)
}

func TestApplySettings_PathPatternsRejectsBadShape(t *testing.T) {
	cases := []struct {
		name string
		v    any
	}{
		{"not a list", "plan/*.md"},
		{"entry not a map", []any{"plan/*.md"}},
		{"missing pattern", []any{map[string]any{"kind": "plan"}}},
		{"missing kind", []any{map[string]any{"pattern": "plan/*.md"}}},
		{"empty kind", []any{map[string]any{"kind": "", "pattern": "x"}}},
		{"empty pattern", []any{map[string]any{"kind": "x", "pattern": ""}}},
		{"malformed glob",
			[]any{map[string]any{"kind": "x", "pattern": "[unclosed"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := (&Rule{}).ApplySettings(map[string]any{
				"path-patterns": tc.v,
			})
			assert.Error(t, err)
		})
	}
}

func TestSettingMergeMode_PathPatternsIsAppend(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeAppend, r.SettingMergeMode("path-patterns"))
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
	_, _, _, err := deriveFrontMatterCUE(yml)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "anchors/aliases are not permitted")
}

// TestDeriveFrontMatterCUE_FieldMetaError covers the error path
// when ExtractFieldMeta rejects a malformed metadata mapping in
// the proto.md frontmatter (plan 136). Without this test the
// `if err != nil` branch after the ExtractFieldMeta call stays
// uncovered in the legacy path.
func TestDeriveFrontMatterCUE_FieldMetaError(t *testing.T) {
	yml := []byte("legacy_owner:\n" +
		"  type: string\n" +
		"  deprecated: true\n" +
		"  tipo: owner\n")
	_, _, _, err := deriveFrontMatterCUE(yml)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "legacy_owner")
	assert.Contains(t, err.Error(), "tipo")
}

// TestDeriveFrontMatterCUE_FieldMetaCapturesMetadata regresses
// the happy path through deriveFrontMatterCUE's new metadata
// branch: a deprecated field returns the embedded `type:`
// constraint and populates the meta map.
func TestDeriveFrontMatterCUE_FieldMetaCapturesMetadata(t *testing.T) {
	yml := []byte("legacy_owner:\n" +
		"  type: string\n" +
		"  deprecated: true\n" +
		"  message: 'use owner'\n" +
		"owner: string\n")
	cueExpr, perKey, meta, err := deriveFrontMatterCUE(yml)
	require.NoError(t, err)
	assert.Contains(t, cueExpr, "legacy_owner: string")
	assert.Equal(t, "string", perKey["legacy_owner"])
	require.Contains(t, meta, "legacy_owner")
	assert.True(t, meta["legacy_owner"].Deprecated)
	assert.Equal(t, "use owner", meta["legacy_owner"].Message)
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
	_, err := parseSchemaFrontMatter(prefix, nil)
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

func TestIsSchemaFile_NormalizesAgainstRootDir(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "plan")
	require.NoError(t, os.MkdirAll(subdir, 0o755))
	schemaPath := filepath.Join(subdir, "proto.md")
	require.NoError(t, os.WriteFile(schemaPath, []byte("# Title\n"), 0o644))

	r := &Rule{Schema: "plan/proto.md"}
	f := newTestFile(t, schemaPath, "# Title\n")
	f.SetRootDir(dir)

	require.True(t, r.isSchemaFile(f),
		"schema file with absolute Path should match root-relative Schema setting")
}

func TestIsSchemaFile_RelativeRootDirGivesNoMatch(t *testing.T) {
	// filepath.Rel returns an error when base is relative and target
	// is absolute; verify isSchemaFile returns false rather than panicking.
	r := &Rule{Schema: "plan/proto.md"}
	f := newTestFile(t, "/some/abs/plan/proto.md", "# Title\n")
	f.RootDir = "relative/rootdir"

	require.False(t, r.isSchemaFile(f))
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

// TestExtractYAML_BlockScalarFenceSequence regresses a Copilot
// review observation: a YAML block-scalar value (e.g. `notes:
// |`) can contain the literal `---\n` sequence inside its body.
// The earlier strings.Index search would truncate at the first
// match; TrimSuffix on the canonical closing fence keeps the
// full body intact.
func TestExtractYAML_BlockScalarFenceSequence(t *testing.T) {
	input := []byte(
		"---\n" +
			"id: 1\n" +
			"notes: |\n" +
			"  ---\n" +
			"  more text\n" +
			"status: open\n" +
			"---\n")
	got := extractYAML(input)
	want := "id: 1\nnotes: |\n  ---\n  more text\nstatus: open\n"
	assert.Equal(t, want, string(got))
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
	expectDiagMsg(t, diags, `Title: got h2, expected h1`)
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
	expectDiagMsg(t, diags, "expected section to be present")
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
	expectDiagMsg(t, diags, `## Goal: got <missing>`)
	expectDiagMsg(t, diags, `## Tasks: got <missing>`)
	expectDiagMsg(t, diags, `## Acceptance Criteria: got <missing>`)
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
	expectDiagMsg(t, diags, `Goal: got h3, expected h2`)
}

// =====================================================================
// Phase 5 coverage: additional branch coverage
// =====================================================================

// deriveFrontMatterCUE: empty map → return "", nil
func TestDeriveFrontMatterCUE_EmptyMap(t *testing.T) {
	// "{}" unmarshals to an empty map → len(raw)==0 branch.
	result, _, _, err := deriveFrontMatterCUE([]byte("{}\n"))
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

// deriveFrontMatterCUE: cueExprForMap error via null YAML value
func TestDeriveFrontMatterCUE_NullValueError(t *testing.T) {
	// YAML null (represented as nil in Go) is not a supported schema type.
	_, _, _, err := deriveFrontMatterCUE([]byte("key: ~\n"))
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

// validateFrontMatterCUE: an empty (whitespace-only) schema is
// treated as "no constraint" and short-circuits to nil before any
// compile.
func TestValidateFrontMatterCUE_EmptySchemaSkips(t *testing.T) {
	require.NoError(t,
		validateFrontMatterCUE("   \n\t", map[string]any{"any": "thing"}))
}

// validateFrontMatterCUE: nil front matter is normalised to an
// empty map before json.Marshal, so an optional-only schema
// validates cleanly without an explicit empty literal at the call
// site.
func TestValidateFrontMatterCUE_NilFrontMatterTreatedAsEmpty(t *testing.T) {
	require.NoError(t,
		validateFrontMatterCUE(`{id?: string}`, nil))
}

// validateFrontMatterCUE: a front-matter value that json.Marshal
// cannot serialize (a channel is the canonical example) surfaces
// the marshal error wrapped with the "serialize front matter"
// prefix. Covers the defensive json.Marshal error branch.
func TestValidateFrontMatterCUE_NonMarshalableFrontMatter(t *testing.T) {
	err := validateFrontMatterCUE(`{id?: string}`,
		map[string]any{"ch": make(chan int)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "serialize front matter")
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
	assert.Contains(t, diags[0].Message, "filename pattern:")
	assert.Contains(t, diags[0].Message, "expected valid glob")
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
	assert.Contains(t, diags[0].Message, "expected section to be present")
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

// matchesSchema: schemaHeading with field text but compiled == nil falls
// through to the exact-text comparison and does not match, without panicking.
func TestMatchesSchema_CompiledNil_NoMatch(t *testing.T) {
	req := schemaHeading{Level: 2, Text: "{id}: {name}", compiled: nil}
	dh := docHeading{Level: 2, Text: "42: Alice", Line: 1}
	assert.False(t, matchesSchema(req, dh))
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
		if strings.Contains(d.Message, "id:") && len(d.RelatedLocations) == 1 {
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

// =====================================================================
// Plan 147: actionable schema diagnostics
// =====================================================================

// TestCheck_FrontMatter_Hint covers acceptance criterion 3: a
// short-disjunction violation with a typo close to a valid literal
// produces a "did you mean" hint pointing at the nearest match.
func TestCheck_FrontMatter_Hint(t *testing.T) {
	schemaPath := writeSchema(t, `---
status: '"draft" | "open" | "done"'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nstatus: draf\n---\n# Any title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `status: got "draf"`)
	expectDiagMsg(t, diags, `expected one of: "draft", "open", "done"`)
	expectDiagMsg(t, diags, `did you mean "draft"?`)
}

// TestCheck_FrontMatter_IntRange covers acceptance criterion 4: a
// numeric-range violation is rendered with the user-facing
// `int between N and M` form, not raw CUE syntax.
func TestCheck_FrontMatter_IntRange(t *testing.T) {
	schemaPath := writeSchema(t, `---
priority: 'int & >=1 & <=5'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\npriority: 9\n---\n# Any title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `priority: got 9`)
	expectDiagMsg(t, diags, `expected int between 1 and 5`)
	expectDiagMsg(t, diags, `try 5`)
}

// TestCheck_FrontMatter_Regex covers acceptance criterion 5: a
// regex violation renders as `string matching <pattern>`, with
// the pattern body unquoted so the user sees what to type.
func TestCheck_FrontMatter_Regex(t *testing.T) {
	schemaPath := writeSchema(t, `---
id: '=~"^FOO-[0-9]{4}$"'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nid: BAR-0001\n---\n# Any title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `id: got "BAR-0001"`)
	expectDiagMsg(t, diags, `expected string matching ^FOO-[0-9]{4}$`)
}

// TestCheck_FrontMatter_OneDiagnosticPerField covers acceptance
// criterion 2: three failed FM constraints produce three
// diagnostics rather than collapsing into one.
func TestCheck_FrontMatter_OneDiagnosticPerField(t *testing.T) {
	schemaPath := writeSchema(t, `---
a: '"x" | "y"'
b: 'int & >=1'
c: '=~"^Z"'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\na: wrong\nb: 0\nc: nope\n---\n# Any title\n")
	diags := r.Check(f)
	// Filter to MDS020 only (other rules don't run with the
	// minimal schema).
	var our []lint.Diagnostic
	for _, d := range diags {
		if d.RuleID == "MDS020" {
			our = append(our, d)
		}
	}
	require.Len(t, our, 3,
		"three FM violations should produce three separate diagnostics")
	fieldSeen := map[string]bool{}
	for _, d := range our {
		for _, f := range []string{"a:", "b:", "c:"} {
			if strings.HasPrefix(d.Message, f) {
				fieldSeen[f] = true
			}
		}
	}
	for _, f := range []string{"a:", "b:", "c:"} {
		assert.True(t, fieldSeen[f], "expected a diagnostic for field %s", f)
	}
}

// TestCheck_FrontMatter_SchemaRefIncludesLine covers acceptance
// criterion 8 for file-based schemas: the trailing schema
// reference points at the constraint's line in proto.md so users
// jump straight to the rule they violated.
func TestCheck_FrontMatter_SchemaRefIncludesLine(t *testing.T) {
	schemaPath := writeSchema(t, `---
id: 'int & >=1'
status: '"open" | "done"'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nstatus: bogus\n---\n# Any title\n")
	diags := r.Check(f)
	// status is on line 3 of the schema (line 1 is "---",
	// line 2 is "id:", line 3 is "status:"). The schema reference
	// now rides on a related location pointing at that line.
	var found bool
	for _, d := range diags {
		for _, rl := range d.RelatedLocations {
			if rl.Line == 3 && strings.Contains(rl.File, ".md") {
				found = true
			}
		}
	}
	assert.True(t, found, "a related location points at schema line 3")
}

// TestCheck_EveryMDS020DiagnosticCarriesRelatedLocation pins plan 230's
// invariant: every MDS020 diagnostic surfaces the schema reference as a
// related location. One document here triggers several distinct emit
// paths (unexpected section + two missing sections); reverting any of
// them to the old message-trailer form would drop the navigation and
// fail this test.
func TestCheck_EveryMDS020DiagnosticCarriesRelatedLocation(t *testing.T) {
	schemaPath := writeSchema(t, "---\n---\n# ?\n\n## Goal\n\n## Tasks\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md", "# Title\n\n## Random\n\nbody\n")
	diags := r.Check(f)
	require.GreaterOrEqual(t, len(diags), 2, "expected several violations")
	for i, d := range diags {
		assert.Len(t, d.RelatedLocations, 1,
			"diagnostic %d %q must carry a related location", i, d.Message)
	}
}

// TestCheck_FrontMatter_DiagnosticLineAtKey regresses the
// Copilot review comment: a FM violation should anchor its
// diagnostic at the offending key's line, not at the start of
// the body. The rule emits a body-coord line; the file's
// LineOffset shift (applied by lint.File.AdjustDiagnostics)
// turns it back into the absolute file line for the key.
func TestCheck_FrontMatter_DiagnosticLineAtKey(t *testing.T) {
	schemaPath := writeSchema(t, `---
status: '"open" | "done"'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	// Build the file through NewFileFromSource so f.FrontMatter
	// and f.LineOffset land in the same shape they do under the
	// LSP / CLI lint path. The doc's status key sits on file
	// line 2; we expect the diagnostic to anchor there after
	// AdjustDiagnostics shifts the rule's body-coord output.
	src := []byte("---\nstatus: bogus\n---\n# Any title\n")
	f, err := lint.NewFileFromSource("doc.md", src, true)
	require.NoError(t, err)
	diags := r.Check(f)
	f.AdjustDiagnostics(diags)
	var statusLine int
	for _, d := range diags {
		if strings.HasPrefix(d.Message, "status:") {
			statusLine = d.Line
		}
	}
	assert.Equal(t, 2, statusLine,
		"FM diagnostic should anchor at the offending key's file line")
}

// TestCheck_FrontMatter_MissingFieldShowsMissing regresses the
// Copilot review comment: a required FM field absent from the
// document should render with `<missing>` in the actual slot so
// the diagnostic format stays consistent.
func TestCheck_FrontMatter_MissingFieldShowsMissing(t *testing.T) {
	schemaPath := writeSchema(t, `---
id: 'int & >=1'
---
# ?
`)
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md", "# Any title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "id: got <missing>")
	expectDiagMsg(t, diags, "expected int >= 1")
}

// =====================================================================
// plan 169: Meta-Information ordering and body-sync Fix
// =====================================================================

// TestCheck_SectionAfterMetaInformation_Fails verifies that a section
// appearing after ## Meta-Information in a schema with no trailing
// wildcard is flagged as not declared in schema.
func TestCheck_SectionAfterMetaInformation_Fails(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Meta-Information\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"# My Rule\n\n## Meta-Information\n\n## See also\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`## See also: got <present>, expected not declared in schema`)
}

// TestFix_BodySync_RewritesStaleLine verifies that Fix replaces a body
// line whose template matches but whose value disagrees with front matter.
func TestFix_BodySync_RewritesStaleLine(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Meta-Information\n\n- **Category**: {category}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\ncategory: structural\n---\n# My Rule\n\n## Meta-Information\n\n- **Category**: WRONG\n")
	result := r.Fix(f)
	assert.Contains(t, string(result), "- **Category**: structural")
}

// TestFix_BodySync_LeavesCorrectLine verifies that Fix does not touch a
// body line that already matches its front-matter value.
func TestFix_BodySync_LeavesCorrectLine(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Meta-Information\n\n- **Category**: {category}\n")
	r := &Rule{Schema: schemaPath}
	src := "---\ncategory: structural\n---\n# My Rule\n\n## Meta-Information\n\n- **Category**: structural\n"
	f := newTestFile(t, "doc.md", src)
	result := r.Fix(f)
	assert.Equal(t, string(f.Source), string(result))
}

// TestFix_BodySync_SkipsCorrectAndFixesStale verifies that Fix skips a
// correct template-matching line and continues scanning to patch the
// subsequent stale duplicate. (Regresses the early-break fix from
// Copilot review comment on PR #315.)
func TestFix_BodySync_SkipsCorrectAndFixesStale(t *testing.T) {
	schemaPath := writeSchema(t,
		"# ?\n\n## Meta-Information\n\n- **Category**: {category}\n")
	r := &Rule{Schema: schemaPath}
	// Two bullets that match the template: the first is already correct,
	// the second is stale. Fix must patch the second one.
	f := newTestFile(t, "doc.md",
		"---\ncategory: structural\n---\n# My Rule\n\n## Meta-Information\n\n"+
			"- **Category**: structural\n- **Category**: WRONG\n")
	result := r.Fix(f)
	assert.Equal(t, 2, strings.Count(string(result), "- **Category**: structural"),
		"both bullets should equal the front-matter value after fix")
}

// TestFix_BodySync_NoFrontMatter verifies that Fix is a no-op when the
// document has no front matter (empty docFMRaw path).
func TestFix_BodySync_NoFrontMatter(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Meta-Information\n\n- **Category**: {category}\n")
	r := &Rule{Schema: schemaPath}
	src := "# My Rule\n\n## Meta-Information\n\n- **Category**: WRONG\n"
	f := newTestFile(t, "doc.md", src)
	result := r.Fix(f)
	assert.Equal(t, string(f.Source), string(result))
}

// TestFix_BodySync_NonBodySyncPoint verifies that a sync point that is
// NOT in the body (InBody == false, heading sync) does not cause Fix to
// rewrite any line.
func TestFix_BodySync_NonBodySyncPoint(t *testing.T) {
	// Schema has only a heading sync ({id} in the title), no body sync.
	schemaPath := writeSchema(t, "# {id}: {name}\n")
	r := &Rule{Schema: schemaPath}
	src := "---\nid: MDS001\nname: line-length\n---\n# MDS001: line-length\n"
	f := newTestFile(t, "doc.md", src)
	result := r.Fix(f)
	assert.Equal(t, string(f.Source), string(result))
}

// TestFix_BodySync_MissingField verifies that Fix skips a body sync
// point whose field is absent from the document's front matter.
func TestFix_BodySync_MissingField(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Meta-Information\n\n- **Category**: {category}\n")
	r := &Rule{Schema: schemaPath}
	// front matter has no "category" key
	src := "---\nid: MDS001\n---\n# My Rule\n\n## Meta-Information\n\n- **Category**: WRONG\n"
	f := newTestFile(t, "doc.md", src)
	result := r.Fix(f)
	assert.Equal(t, string(f.Source), string(result))
}

// TestFix_BodySync_NoMatchingLine verifies that Fix returns f.Source
// unchanged when the heading is present but no body line matches the
// field template pattern (e.g., the bullet is formatted differently).
func TestFix_BodySync_NoMatchingLine(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Meta-Information\n\n- **Category**: {category}\n")
	r := &Rule{Schema: schemaPath}
	// Body has a line that does NOT match "- **Category**: .+"
	src := "---\ncategory: structural\n---\n# My Rule\n\n## Meta-Information\n\nSomething else entirely.\n"
	f := newTestFile(t, "doc.md", src)
	result := r.Fix(f)
	assert.Equal(t, string(f.Source), string(result))
}

// TestFix_BodySync_MultiSource verifies that Fix does not rewrite body
// lines when the rule is configured with multiple schema sources
// (composed schemas skip the Fix body rewrite path).
func TestFix_BodySync_MultiSource(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Meta-Information\n\n- **Category**: {category}\n")
	schemaPath2 := writeSchema(t, "# ?\n")
	r := &Rule{Sources: []SchemaSource{
		{File: schemaPath},
		{File: schemaPath2},
	}}
	f := newTestFile(t, "doc.md",
		"---\ncategory: structural\n---\n# My Rule\n\n## Meta-Information\n\n- **Category**: WRONG\n")
	result := r.Fix(f)
	// Multi-source: Fix should NOT rewrite the stale line.
	assert.Equal(t, string(f.Source), string(result))
}

// TestFix_BodySync_WildcardBeforeSection covers the isSectionWildcard
// branch in fixBodySyncIn: a schema with ## ... before the sync heading.
func TestFix_BodySync_WildcardBeforeSection(t *testing.T) {
	schemaPath := writeSchema(t,
		"# ?\n\n## ...\n\n## Meta-Information\n\n- **Category**: {category}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\ncategory: structural\n---\n# My Rule\n\n## Optional\n\n## Meta-Information\n\n- **Category**: WRONG\n")
	result := r.Fix(f)
	assert.Contains(t, string(result), "- **Category**: structural")
}

// TestFix_BodySync_RequiredHeadingAbsent covers the matchedDoc < 0
// branch in fixBodySyncIn: the schema requires a heading with body
// sync but the document does not contain that heading.
func TestFix_BodySync_RequiredHeadingAbsent(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Meta-Information\n\n- **Category**: {category}\n")
	r := &Rule{Schema: schemaPath}
	// Doc has no ## Meta-Information heading.
	src := "---\ncategory: structural\n---\n# My Rule\n"
	f := newTestFile(t, "doc.md", src)
	result := r.Fix(f)
	assert.Equal(t, string(f.Source), string(result))
}

// TestFix_BodySync_HeadingFollowedByAnother covers the
// matchedDoc+1 < len(docHeadings) branch: the matched heading is
// not the last heading in the document.
func TestFix_BodySync_HeadingFollowedByAnother(t *testing.T) {
	schemaPath := writeSchema(t, "# ?\n\n## Meta-Information\n\n- **Category**: {category}\n")
	r := &Rule{Schema: schemaPath}
	// Doc has ## Appendix after ## Meta-Information.
	f := newTestFile(t, "doc.md",
		"---\ncategory: structural\n---\n# My Rule\n\n## Meta-Information\n\n- **Category**: WRONG\n\n## Appendix\n")
	result := r.Fix(f)
	assert.Contains(t, string(result), "- **Category**: structural")
}

// TestResolveBodySyncLine_NilPath covers the path == nil branch by
// calling resolveBodySyncLine directly with an empty-string field.
func TestResolveBodySyncLine_NilPath(t *testing.T) {
	sp := syncPoint{
		Field: "", InBody: true, BodyText: "- **Category**: {category}",
		compiled: buildFieldPattern("- **Category**: {category}"),
	}
	_, ok := resolveBodySyncLine(sp, map[string]any{"category": "structural"}, nil, 1, 1)
	assert.False(t, ok, "empty field should yield ok=false")
}

// TestIsLikelyArchetypeName_AllBranches gives the helper direct
// coverage of every branch. Previously it was only exercised
// indirectly through ApplySettings, so test churn elsewhere
// silently dropped its coverage (the recurring codecov/changes
// signal). A dedicated table keeps it pinned.
func TestIsLikelyArchetypeName_AllBranches(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty string", "", false},
		{"forward slash path", "schemas/story.md", false},
		{"forward slash, no ext", "schemas/story", false},
		{"backslash path", `schemas\story`, false},
		{"has extension", "story.md", false},
		{"dotfile-style extension", "story.proto.md", false},
		{"bare name no ext no slash", "story", true},
		{"bare name with dash", "rule-readme", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isLikelyArchetypeName(tc.in))
		})
	}
}

// TestExtractSchemaSourceFromSettings_AllBranches replaces the
// removed rulesDeclareSchema's branch coverage. The schema-source
// detection logic moved here during the SettingsTranslator
// refactor; this table exercises every return path directly
// instead of relying on TranslateLayerSettings call sites.
func TestExtractSchemaSourceFromSettings_AllBranches(t *testing.T) {
	tests := []struct {
		name    string
		in      map[string]any
		wantSrc any
		wantKey bool
	}{
		{"no schema keys", map[string]any{"placeholders": []any{"x"}}, nil, false},
		{"empty settings", map[string]any{}, nil, false},
		{"schema non-empty string", map[string]any{"schema": "schemas/a.md"},
			map[string]any{"file": "schemas/a.md"}, true},
		{"schema empty string", map[string]any{"schema": ""}, nil, true},
		{"schema wrong type", map[string]any{"schema": 42}, nil, true},
		{"inline-schema non-empty map", map[string]any{"inline-schema": map[string]any{
			"sections": []any{map[string]any{"heading": "X"}},
		}}, map[string]any{"inline": map[string]any{
			"sections": []any{map[string]any{"heading": "X"}},
		}}, true},
		{"inline-schema empty map",
			map[string]any{"inline-schema": map[string]any{}}, nil, true},
		{"inline-schema wrong type",
			map[string]any{"inline-schema": "not-a-map"}, nil, true},
		{"schema wins over inline (file first)", map[string]any{
			"schema":        "a.md",
			"inline-schema": map[string]any{"sections": []any{}},
		}, map[string]any{"file": "a.md"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src, hadKey := extractSchemaSourceFromSettings(tc.in)
			assert.Equal(t, tc.wantSrc, src)
			assert.Equal(t, tc.wantKey, hadKey)
		})
	}
}

// TestExtractSchemaSourceFromSettings_InlineDeepCopied verifies the
// extracted inline source does not alias the caller's nested maps,
// so a later layer mutation cannot reach the captured schema.
func TestExtractSchemaSourceFromSettings_InlineDeepCopied(t *testing.T) {
	inline := map[string]any{"sections": []any{
		map[string]any{"heading": "Goal"},
	}}
	src, _ := extractSchemaSourceFromSettings(
		map[string]any{"inline-schema": inline})
	got := src.(map[string]any)["inline"].(map[string]any)
	inline["sections"].([]any)[0].(map[string]any)["heading"] = "MUT"
	sec := got["sections"].([]any)[0].(map[string]any)
	assert.Equal(t, "Goal", sec["heading"],
		"extracted inline source must not alias the input map")
}

// --- buildSchemaRefForLegacy ---

// TestBuildSchemaRefForLegacy pins both branches: an empty source
// falls back to the generic "schema" label so every diagnostic
// still carries a reference suffix; a non-empty source is
// returned verbatim. The Check loop only ever drives the
// non-empty case via real schema-bound fixtures.
func TestBuildSchemaRefForLegacy(t *testing.T) {
	if got := buildSchemaRefForLegacy(""); got != "schema" {
		t.Errorf(`buildSchemaRefForLegacy("") = %q, want "schema"`, got)
	}
	if got := buildSchemaRefForLegacy("kinds/agent"); got != "kinds/agent" {
		t.Errorf("buildSchemaRefForLegacy(%q) = %q, want %q",
			"kinds/agent", got, "kinds/agent")
	}
	if got := buildSchemaRefForLegacy("proto.md"); got != "proto.md" {
		t.Errorf("buildSchemaRefForLegacy(%q) = %q, want %q",
			"proto.md", got, "proto.md")
	}
}

// TestLegacyPrecedingLine covers the file-schema path's document-order
// lookup mirror of precedingHeadingLine.
func TestLegacyPrecedingLine(t *testing.T) {
	heads := []docHeading{{Line: 2}, {Line: 5}}
	assert.Equal(t, 0, legacyPrecedingLine(heads, 0))
	assert.Equal(t, 2, legacyPrecedingLine(heads, 1))
	assert.Equal(t, 5, legacyPrecedingLine(heads, 2))
}
