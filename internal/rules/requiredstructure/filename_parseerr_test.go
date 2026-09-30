package requiredstructure

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/schema"
)

// A `filename:` reference to a field whose front matter failed to
// parse must name the YAML error, as the path-pattern surface does.
// The file gets the parse diagnostic and a filename diagnostic that
// names it, and nothing reports the field as missing: it is present,
// it just did not parse.
func TestCheck_FilenameFmvar_UnparseableFrontMatterNamesParseError(t *testing.T) {
	const doc = "---\nid: [unclosed\n---\n# Notes\n"
	cases := map[string]func(root string) *Rule{
		"proto.md": func(root string) *Rule {
			writeProtoAt(t, root, "proto.md",
				"---\nid: string\n---\n<?require\nfilename: '\\#(fmvar(id)).md'\n?>\n# ?\n")
			return &Rule{Schema: "proto.md", Sources: []SchemaSource{{File: "proto.md"}}}
		},
		"inline": func(string) *Rule {
			return &Rule{Sources: []SchemaSource{{Inline: &schema.Schema{
				Filename:    []string{`\#(fmvar(id)).md`},
				Frontmatter: map[string]string{"id": "string"},
				Source:      "kind note",
			}}}}
		},
		"composed": func(string) *Rule {
			return &Rule{Sources: []SchemaSource{
				{Inline: &schema.Schema{
					Filename: []string{`\#(fmvar(id)).md`},
					Source:   "kind named",
				}},
				{Inline: &schema.Schema{
					Frontmatter: map[string]string{"id": "string"},
					Source:      "kind note",
				}},
			}}
		},
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			r := mk(root)
			f := newRootedFile(t, root, "x.md", doc)
			diags := r.Check(f)
			require.Len(t, diags, 2, "got %v", messages(diags))
			assert.True(t, strings.HasPrefix(diags[0].Message,
				"front matter: invalid YAML"), diags[0].Message)
			assert.Contains(t, diags[1].Message, `filename: got "x.md"`)
			assert.Contains(t, diags[1].Message, "(front matter: invalid YAML")
			for _, d := range diags {
				assert.NotContains(t, d.Message, "missing")
			}
		})
	}
}

// frontMatterParseErr turns the parse diagnostic readDocFrontMatterRaw
// returns into the error the filename and path-pattern hints name,
// and reports nil when the front matter parsed.
func TestFrontMatterParseErr(t *testing.T) {
	assert.NoError(t, frontMatterParseErr(nil))
	err := frontMatterParseErr([]lint.Diagnostic{{Message: "front matter: boom"}})
	require.Error(t, err)
	assert.Equal(t, "front matter: boom", err.Error())
}

func messages(diags []lint.Diagnostic) []string {
	out := make([]string, len(diags))
	for i, d := range diags {
		out[i] = d.Message
	}
	return out
}

// The schema's own CUE is compiled whatever the document holds, so a
// compile failure is reported next to the YAML error rather than
// hidden by it, for an inline schema and for composed inline ones.
func TestCheck_UnparseableFrontMatterKeepsSchemaCUEError(t *testing.T) {
	bad := &schema.Schema{
		Frontmatter: map[string]string{"id": "string &"},
		Source:      "kind badcue",
	}
	cases := map[string][]SchemaSource{
		"inline": {{Inline: bad}},
		"composed": {
			{Inline: bad},
			{Inline: &schema.Schema{
				Frontmatter: map[string]string{"title?": "string"},
				Source:      "kind other",
			}},
		},
	}
	for name, sources := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			f := newRootedFile(t, root, "g/bad.md",
				"---\nid: [unclosed\n---\n# Heading\n")
			diags := (&Rule{Sources: sources}).Check(f)
			msgs := messages(diags)
			require.Len(t, msgs, 2, "got %v", msgs)
			assert.Contains(t, msgs[0], "front matter: invalid YAML")
			assert.Contains(t, msgs[1], "expected valid schema CUE")
		})
	}
}
