package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewFixtureFile_DocFrontMatter pins the fixture `front-matter:`
// key: its YAML becomes the linted document's own front matter, which
// the harness otherwise strips with the fixture's settings block.
func TestNewFixtureFile_DocFrontMatter(t *testing.T) {
	raw := []byte("---\nfront-matter: |\n  id: alpha\n---\n# T\n")
	_, _, content, docFM := parseFixtureFrontMatter(t, raw, false)
	assert.Equal(t, "# T\n", string(content))
	f, err := newFixtureFile("doc.md", content, docFM)
	require.NoError(t, err)
	assert.Equal(t, "---\nid: alpha\n---\n", string(f.FrontMatter))
	assert.Equal(t, "# T\n", string(f.Source))
	assert.Equal(t, 3, f.LineOffset, "diagnostic lines count the front matter")
}

// TestNewFixtureFile_NoDocFrontMatter pins that a fixture without the
// key parses its content as is: a leading `---` block (a Slidev deck's
// headmatter) stays in the source.
func TestNewFixtureFile_NoDocFrontMatter(t *testing.T) {
	content := []byte("---\nlayout: cover\n---\n# T\n")
	f, err := newFixtureFile("deck.md", content, "")
	require.NoError(t, err)
	assert.Empty(t, f.FrontMatter)
	assert.Equal(t, string(content), string(f.Source))
}
