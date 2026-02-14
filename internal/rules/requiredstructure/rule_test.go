package requiredstructure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

func newTestFile(
	t *testing.T, path, source string,
) *lint.File {
	t.Helper()
	f, err := lint.NewFileFromSource(path, []byte(source), true)
	if err != nil {
		t.Fatal(err)
	}
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

func TestApplySettings_ValidTemplate(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"template": "foo.md"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Template != "foo.md" {
		t.Errorf(
			"expected Template foo.md, got %s", r.Template,
		)
	}
}

func TestApplySettings_InvalidTemplate(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"template": 42})
	if err == nil {
		t.Fatal("expected error for non-string template")
	}
}

func TestApplySettings_UnknownSetting(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": true})
	if err == nil {
		t.Fatal("expected error for unknown setting")
	}
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["template"] != "" {
		t.Errorf(
			"expected template=\"\", got %v", ds["template"],
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
	tmpl, err := parseTemplate([]byte(tmplSrc))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	tmplSrc := `# {{.id}}: {{.name}}

{{.description}}
`
	tmpl, err := parseTemplate([]byte(tmplSrc))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	tmpl, err := parseTemplate([]byte(tmplSrc))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	tmplPath := writeTmpl(t, "# {{.id}}: {{.name}}\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nid: MDS001\nname: line-length\n---\n# MDS002: line-length\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "heading does not match frontmatter")
}

func TestCheck_HeadingSyncMatch(t *testing.T) {
	tmplPath := writeTmpl(t, "# {{.id}}: {{.name}}\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\nid: MDS001\nname: line-length\n---\n# MDS001: line-length\n")
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_BodySyncMismatch(t *testing.T) {
	tmplPath := writeTmpl(t, "# ?\n\n{{.description}}\n")
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md",
		"---\ndescription: Line exceeds maximum length.\n---\n# My Rule\n\nWrong description here.\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`body does not match frontmatter field "description"`)
}

func TestCheck_BodySyncMatch(t *testing.T) {
	tmplPath := writeTmpl(t, "# ?\n\n{{.description}}\n")
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

func TestCheck_LegacyTemplateFrontMatterCUEIsRejected(t *testing.T) {
	tmplPath := writeTmpl(t, `---
template:
  allow-extra-sections: true
  front-matter-cue: |
    {
      id: int & >=1
    }
---
# ?
`)
	r := &Rule{Template: tmplPath}
	f := newTestFile(t, "doc.md", "# Any title\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "template.front-matter-cue is no longer supported")
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
