package requiredstructure

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// An unquoted YAML date (`date: 2026-01-02`) decodes to time.Time.
// The kind `path-pattern:` fmvar, the proto.md `filename:` fmvar, and
// the `{date}` heading and body sync must all read it as the
// YYYY-MM-DD the author wrote.

func TestCheck_PathPatternFmvar_UnquotedDate(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, "posts/2026-01-02/index.md",
		"---\ndate: 2026-01-02\n---\n# Post\n")
	r := &Rule{PathPatterns: []PathPattern{
		newPathPattern("post", `posts/\#(fmvar(date))/index.md`),
	}}
	expectDiags(t, r.Check(f), 0)
}

func TestCheck_FileSchemaFilenameFmvar_UnquotedDate(t *testing.T) {
	root := t.TempDir()
	writeProtoAt(t, root, "proto.md",
		"<?require\nfilename: '\\#(fmvar(date))-*.md'\n?>\n# ?\n")
	f := newRootedFile(t, root, "2026-01-02-post.md",
		"---\ndate: 2026-01-02\n---\n# Post\n")
	r := &Rule{Schema: "proto.md", Sources: []SchemaSource{{File: "proto.md"}}}
	expectDiags(t, r.Check(f), 0)
}

func TestCheck_HeadingSyncUnquotedDate(t *testing.T) {
	schemaPath := writeSchema(t, "# Release {date}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\ndate: 2026-01-02\n---\n# Release 2026-01-02\n")
	expectDiags(t, r.Check(f), 0)
}

func TestFix_BodySync_UnquotedDate(t *testing.T) {
	schemaPath := writeSchema(t,
		"# ?\n\n## Meta-Information\n\n- **Released**: {date}\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\ndate: 2026-01-02\n---\n# My Rule\n\n## Meta-Information\n\n- **Released**: WRONG\n")
	result := r.Fix(f)
	assert.Contains(t, string(result), "- **Released**: 2026-01-02\n")
}
