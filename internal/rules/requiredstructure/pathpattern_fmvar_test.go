package requiredstructure

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/schema"
)

// The APM skill contract: `.apm/skills/<name>/SKILL.md` requires the
// `name` frontmatter field to equal the directory name. A static glob
// cannot express that, so `path-pattern:` resolves
// `\#(fmvar(name))` against the document's own front matter first.

const apmSkillPattern = `.apm/skills/\#(fmvar(name))/SKILL.md`

func TestCheck_PathPatternFmvar_MatchesDirectory(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, ".apm/skills/code-review/SKILL.md",
		"---\nname: code-review\n---\n# Code review\n")
	r := &Rule{PathPatterns: []PathPattern{
		{Kind: "apm-skill", Pattern: apmSkillPattern},
	}}
	expectDiags(t, r.Check(f), 0)
}

func TestCheck_PathPatternFmvar_MismatchedDirectory(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, ".apm/skills/code-review/SKILL.md",
		"---\nname: reviewer\n---\n# Code review\n")
	r := &Rule{PathPatterns: []PathPattern{
		{Kind: "apm-skill", Pattern: apmSkillPattern},
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	assert.Contains(t, diags[0].Message,
		`path: got ".apm/skills/code-review/SKILL.md"`)
	assert.Contains(t, diags[0].Message,
		"with front matter applied: .apm/skills/reviewer/SKILL.md")
}

func TestCheck_PathPatternFmvar_MissingFieldReportsClearly(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, ".apm/skills/code-review/SKILL.md",
		"# Code review\n")
	r := &Rule{PathPatterns: []PathPattern{
		{Kind: "apm-skill", Pattern: apmSkillPattern},
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	assert.Contains(t, diags[0].Message, "fmvar(name)")
	assert.Contains(t, diags[0].Message, "frontmatter value missing")
	assert.Contains(t, diags[0].Message, apmSkillPattern,
		"the unresolved pattern still names the constraint")
}

// A frontmatter value carrying a glob metacharacter must match
// literally, not act as a wildcard.
func TestCheck_PathPatternFmvar_EscapesValueMetacharacters(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, ".apm/skills/axb/SKILL.md",
		"---\nname: \"a*b\"\n---\n# A\n")
	r := &Rule{PathPatterns: []PathPattern{
		{Kind: "apm-skill", Pattern: apmSkillPattern},
	}}
	expectDiags(t, r.Check(f), 1)
}

func TestCheck_PathPatternFmvar_NestedFieldPath(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, "docs/install.md",
		"---\nmeta:\n  slug: install\n---\n# Install\n")
	r := &Rule{PathPatterns: []PathPattern{
		{Kind: "doc", Pattern: `docs/\#(fmvar(meta.slug)).md`},
	}}
	expectDiags(t, r.Check(f), 0)
}

func TestParsePathPatterns_RejectsMalformedInterp(t *testing.T) {
	_, err := parsePathPatterns([]any{
		map[string]any{
			"kind":    "doc",
			"pattern": `docs/\#(fmvar(my-key)).md`,
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be quoted")
}

func TestParsePathPatterns_AcceptsWellFormedInterp(t *testing.T) {
	pp, err := parsePathPatterns([]any{
		map[string]any{"kind": "apm-skill", "pattern": apmSkillPattern},
	})
	require.NoError(t, err)
	require.Len(t, pp, 1)
	assert.Equal(t, apmSkillPattern, pp[0].Pattern)
}

// ---- schema `filename:` through the legacy proto.md path ----

func TestCheck_FileSchemaFilenameFmvar_MatchesFrontmatterValue(t *testing.T) {
	root := t.TempDir()
	writeProtoAt(t, root, "proto.md",
		"<?require\nfilename: '\\#(fmvar(id))-notes.md'\n?>\n# ?\n")
	f := newRootedFile(t, root, "rfc-7-notes.md",
		"---\nid: rfc-7\n---\n# Notes\n")
	r := &Rule{Schema: "proto.md", Sources: []SchemaSource{{File: "proto.md"}}}
	expectDiags(t, r.Check(f), 0)
}

func TestCheck_FileSchemaFilenameFmvar_Mismatch(t *testing.T) {
	root := t.TempDir()
	writeProtoAt(t, root, "proto.md",
		"<?require\nfilename: '\\#(fmvar(id))-notes.md'\n?>\n# ?\n")
	f := newRootedFile(t, root, "rfc-8-notes.md",
		"---\nid: rfc-7\n---\n# Notes\n")
	r := &Rule{Schema: "proto.md", Sources: []SchemaSource{{File: "proto.md"}}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	assert.Contains(t, diags[0].Message, `filename: got "rfc-8-notes.md"`)
	assert.Contains(t, diags[0].Message,
		"with front matter applied: rfc-7-notes.md")
}

func TestCheck_FileSchemaFilenameFmvar_MissingField(t *testing.T) {
	root := t.TempDir()
	writeProtoAt(t, root, "proto.md",
		"<?require\nfilename: '\\#(fmvar(id))-notes.md'\n?>\n# ?\n")
	f := newRootedFile(t, root, "rfc-7-notes.md", "# Notes\n")
	r := &Rule{Schema: "proto.md", Sources: []SchemaSource{{File: "proto.md"}}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	assert.Contains(t, diags[0].Message, "frontmatter value missing")
}

// writeProtoAt writes a schema file inside an existing workspace root
// so the rule resolves it through the same RootFS the document uses.
func writeProtoAt(t *testing.T, root, name, content string) {
	t.Helper()
	abs := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))
}

// An interpolating pattern must be matched on its raw text: on
// Windows filepath.ToSlash rewrites every `\`, including the one
// that opens `\#(fmvar(...))`, which would silently demote the
// pattern to a literal glob that no file can satisfy.
func TestCheck_PathPatternFmvar_RawPatternDrivesInterpDetection(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, ".apm/skills/code-review/SKILL.md",
		"---\nname: code-review\n---\n# Code review\n")
	r := &Rule{PathPatterns: []PathPattern{
		{Kind: "apm-skill", Pattern: apmSkillPattern},
	}}
	require.True(t, schema.PatternHasInterp(r.PathPatterns[0].Pattern),
		"the raw pattern is what PatternHasInterp must see")
	expectDiags(t, r.Check(f), 0)
}

// A `filename:` OR list in a proto.md keeps its OR semantics when
// one entry's `\#(fmvar(...))` reference cannot resolve.
func TestCheck_FileSchemaFilenameFmvar_UnresolvableEntryKeepsOR(t *testing.T) {
	root := t.TempDir()
	writeProtoAt(t, root, "proto.md",
		"<?require\nfilename:\n  - README.md\n  - '\\#(fmvar(id))-notes.md'\n?>\n# ?\n")
	f := newRootedFile(t, root, "README.md", "# Notes\n")
	r := &Rule{Schema: "proto.md", Sources: []SchemaSource{{File: "proto.md"}}}
	expectDiags(t, r.Check(f), 0)
}
