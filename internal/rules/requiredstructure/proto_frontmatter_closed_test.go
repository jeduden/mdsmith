package requiredstructure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
