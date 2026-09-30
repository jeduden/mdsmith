package requiredstructure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/schema"
)

// The legacy proto.md parser MDS020 uses for a single file schema
// reports a `frontmatter-closed:` key rather than reading it as a
// document field that silently leaves the front matter closed.
func TestCheck_ProtoFrontmatterClosedIsReported(t *testing.T) {
	schemaPath := writeSchema(t,
		"---\ntitle: string\nfrontmatter-closed: false\n---\n# ?\n")
	f := newTestFile(t, "doc.md", "---\ntitle: hi\nextra: 1\n---\n# Doc\n")
	r := &Rule{Schema: schemaPath}
	diags := r.Check(f)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message,
		"`frontmatter-closed:` is not supported in a proto.md schema")
}

// A proto.md that declares front matter cannot set
// `frontmatter-closed:`, so it always votes closed. When it keeps a
// composite closed that another kind opened, the undeclared-key hint
// names the proto.md rather than asking the author to set `false` on
// every kind, which the proto.md cannot do.
func TestCheck_ComposedProtoClosingFrontmatterIsNamed(t *testing.T) {
	root := t.TempDir()
	writeProtoAt(t, root, "proto.md", "---\ntitle: string\n---\n")
	open := false
	r := &Rule{Sources: []SchemaSource{
		{File: "proto.md"},
		{Inline: &schema.Schema{
			Frontmatter:       map[string]string{"title": "string"},
			FrontmatterClosed: &open,
			Source:            "kind open",
		}},
	}}
	f := newRootedFile(t, root, "doc.md", "---\ntitle: hi\nextra: 1\n---\n# Doc\n")
	diags := r.Check(f)
	require.Len(t, diags, 1, "got %v", messages(diags))
	msg := diags[0].Message
	assert.Contains(t, msg, "extra: got 1, expected not declared in schema")
	assert.Contains(t, msg, `proto.md schema "proto.md" declares front matter`)
	assert.NotContains(t, msg, "every kind composed")
}
