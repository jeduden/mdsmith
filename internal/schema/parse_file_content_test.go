package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseFile_ContentDirectiveParagraph is the happy path: a
// `<?content kind: paragraph ?>` directive row in a proto.md section
// body declares one content entry on the enclosing section's scope,
// matching what the inline `content: [{ kind: paragraph }]` form
// produces.
func TestParseFile_ContentDirectiveParagraph(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.md",
		"# {title}\n\n## Tagline\n\n<?content\nkind: paragraph\n?>\n")
	sch, err := ParseFile(&FileReader{}, path)
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1, "one H1 root scope")
	h1 := sch.Sections[0]
	require.Len(t, h1.Sections, 1, "one H2 under the H1")
	tagline := h1.Sections[0]
	require.Len(t, tagline.Content, 1,
		"the <?content?> row must declare one content entry")
	assert.Equal(t, ContentKindParagraph, tagline.Content[0].Kind)
	assert.True(t, tagline.Content[0].Required,
		"a literal content entry defaults to required")
}

// TestParseFile_ContentDirectiveTwoOrdered verifies two directives in
// one section declare two ordered entries (the `text` / `text-2`
// projection keys the inline form derives from positional order).
func TestParseFile_ContentDirectiveTwoOrdered(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.md",
		"## Body\n\n<?content\nkind: paragraph\n?>\n\n<?content\nkind: code-block\nlang: go\n?>\n")
	sch, err := ParseFile(&FileReader{}, path)
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1)
	body := sch.Sections[0]
	require.Len(t, body.Content, 2, "two directives, two ordered entries")
	assert.Equal(t, ContentKindParagraph, body.Content[0].Kind)
	assert.Equal(t, ContentKindCodeBlock, body.Content[1].Kind)
	assert.Equal(t, "go", body.Content[1].Lang)
}

// TestParseFile_ContentDirectiveProjectionInline verifies the
// `projection:` key passes through and is validated by the inline
// rules (inline is legal on a paragraph).
func TestParseFile_ContentDirectiveProjectionInline(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.md",
		"## Tagline\n\n<?content\nkind: paragraph\nprojection: inline\n?>\n")
	sch, err := ParseFile(&FileReader{}, path)
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1)
	require.Len(t, sch.Sections[0].Content, 1)
	assert.Equal(t, ProjectionInline, sch.Sections[0].Content[0].Projection)
}

// TestParseFile_ContentDirectiveRejectsUnknownKind locks down that an
// invalid kind fails at parse time with the inline form's diagnostic.
func TestParseFile_ContentDirectiveRejectsUnknownKind(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.md",
		"## Tagline\n\n<?content\nkind: bogus\n?>\n")
	_, err := ParseFile(&FileReader{}, path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown content kind")
}

// TestParseFile_ContentDirectiveRejectsInlineOnCodeBlock locks down
// that `projection: inline` on a code-block fails at parse time, the
// same as the inline form.
func TestParseFile_ContentDirectiveRejectsInlineOnCodeBlock(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.md",
		"## Snippet\n\n<?content\nkind: code-block\nprojection: inline\n?>\n")
	_, err := ParseFile(&FileReader{}, path)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "projection")
}

// TestParseFile_ContentDirectiveRejectsEmptyBind locks down that
// `bind: ""` on a content entry fails at parse time — a content entry
// has no children to hoist, the same rule the inline form enforces.
func TestParseFile_ContentDirectiveRejectsEmptyBind(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.md",
		"## Tagline\n\n<?content\nkind: paragraph\nbind: \"\"\n?>\n")
	_, err := ParseFile(&FileReader{}, path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bind")
}

// TestParseFile_ContentDirectiveRejectsUnknownKey locks down that an
// unknown key on the directive fails at parse time, matching the
// inline content parser's unknown-key diagnostic.
func TestParseFile_ContentDirectiveRejectsUnknownKey(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.md",
		"## Tagline\n\n<?content\nkind: paragraph\nbogus: 1\n?>\n")
	_, err := ParseFile(&FileReader{}, path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown content key")
}

// TestParseFile_ContentDirectiveRequiresKind locks down that a bare
// `<?content ?>` with no body fails at parse time instead of
// declaring an empty entry.
func TestParseFile_ContentDirectiveRequiresKind(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.md",
		"## Tagline\n\n<?content ?>\n")
	_, err := ParseFile(&FileReader{}, path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing a `kind:` key")
}

// TestParseFile_ContentDirectiveRejectsInvalidYAML locks down that a
// directive body that is not valid YAML fails at parse time with the
// invalid-directive diagnostic.
func TestParseFile_ContentDirectiveRejectsInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.md",
		"## Tagline\n\n<?content\nkind: [unclosed\n?>\n")
	_, err := ParseFile(&FileReader{}, path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid <?content?> directive")
}

// TestParseFile_ContentDirectiveBeforeHeadingFails locks down that a
// `<?content?>` row above the first heading is rejected — the entry
// would have no section to attach to.
func TestParseFile_ContentDirectiveBeforeHeadingFails(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "schema.md",
		"<?content\nkind: paragraph\n?>\n\n## Tagline\n")
	_, err := ParseFile(&FileReader{}, path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must appear inside a section")
}
