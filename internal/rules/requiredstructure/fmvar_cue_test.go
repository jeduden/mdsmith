package requiredstructure

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jeduden/mdsmith/internal/schema"
)

// Under the `cue-frontmatter` placeholder a template's front-matter
// values are CUE constraints (`name: string`), not data. An
// `fmvar(...)` reference therefore has no value to substitute: the
// constraint text must not be read as the name. Each reference
// matches any single path segment instead, and the rest of the glob
// is still checked.

var cuePlaceholders = []string{"cue-frontmatter"}

func TestCheck_PathPatternFmvar_CUEFrontmatterMatchesAnySegment(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, ".apm/skills/_template/SKILL.md",
		"---\nname: 'string & =~\"^[a-z-]+$\"'\n---\n# Skill\n")
	r := &Rule{
		PathPatterns: []PathPattern{{Kind: "apm-skill", Pattern: apmSkillPattern}},
		Placeholders: cuePlaceholders,
	}
	expectDiags(t, r.Check(f), 0)
}

func TestCheck_PathPatternFmvar_CUEFrontmatterStillChecksLiteralParts(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, ".apm/prompts/_template/SKILL.md",
		"---\nname: string\n---\n# Skill\n")
	r := &Rule{
		PathPatterns: []PathPattern{{Kind: "apm-skill", Pattern: apmSkillPattern}},
		Placeholders: cuePlaceholders,
	}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	assert.NotContains(t, diags[0].Message, "with front matter applied",
		"a CUE constraint is not a value that was applied")
}

func TestCheck_FileSchemaFilenameFmvar_CUEFrontmatter(t *testing.T) {
	root := t.TempDir()
	writeProtoAt(t, root, "proto.md",
		"<?require\nfilename: '\\#(fmvar(id))-notes.md'\n?>\n# ?\n")
	f := newRootedFile(t, root, "template-notes.md",
		"---\nid: string\n---\n# Notes\n")
	r := &Rule{
		Schema:       "proto.md",
		Sources:      []SchemaSource{{File: "proto.md"}},
		Placeholders: cuePlaceholders,
	}
	expectDiags(t, r.Check(f), 0)
}

func TestCheck_InlineSchemaFilenameFmvar_CUEFrontmatter(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, "template-notes.md",
		"---\nid: string\n---\n# Notes\n")
	r := &Rule{
		Sources: []SchemaSource{{Inline: &schema.Schema{
			Filename: []string{`\#(fmvar(id))-notes.md`},
			Source:   "kind note",
		}}},
		Placeholders: cuePlaceholders,
	}
	expectDiags(t, r.Check(f), 0)
}
