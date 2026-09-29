package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/backlinks"
)

func TestNormalizeWorkspacePath(t *testing.T) {
	assert.Equal(t, "docs/api.md", normalizeWorkspacePath("docs/api.md"))
	assert.Equal(t, "docs/api.md", normalizeWorkspacePath("./docs/api.md"))
}

func TestEmitBacklinks_Text(t *testing.T) {
	var buf bytes.Buffer
	records := []backlinks.Record{
		{Source: "a.md", Line: 1, Text: "ref", Target: "x.md"},
		{Source: "b.md", Line: 2, Text: "ref2", Target: "./x.md"},
	}
	code := emitBacklinks(&buf, records, "text", 0)
	assert.Equal(t, 0, code)
	out := buf.String()
	assert.Contains(t, out, "a.md:1: [ref](x.md)\n")
	assert.Contains(t, out, "b.md:2: [ref2](./x.md)\n")
}

func TestEmitBacklinks_JSON(t *testing.T) {
	var buf bytes.Buffer
	records := []backlinks.Record{
		{Source: "a.md", Line: 1, Text: "ref", Target: "x.md"},
	}
	code := emitBacklinks(&buf, records, "json", 0)
	assert.Equal(t, 0, code)
	var decoded []backlinks.Record
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	assert.Equal(t, records, decoded)
}

func TestEmitBacklinks_JSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	code := emitBacklinks(&buf, nil, "json", 0)
	assert.Equal(t, 1, code, "no records → exit 1")
	// `[]` is the documented stable shape; never `null`.
	assert.Contains(t, buf.String(), "[]")
	assert.NotContains(t, buf.String(), "null")
}

func TestEmitBacklinks_Limit(t *testing.T) {
	records := []backlinks.Record{
		{Source: "a.md", Line: 1},
		{Source: "b.md", Line: 1},
		{Source: "c.md", Line: 1},
	}
	var buf bytes.Buffer
	code := emitBacklinks(&buf, records, "text", 2)
	assert.Equal(t, 0, code)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Len(t, lines, 2)
}

func TestEmitBacklinks_UnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	code := emitBacklinks(&buf, []backlinks.Record{{Source: "a.md"}}, "yaml", 0)
	assert.Equal(t, 2, code)
}

func TestValidateIncludePatterns(t *testing.T) {
	assert.NoError(t, validateIncludePatterns(nil))
	assert.NoError(t, validateIncludePatterns([]string{"docs/**", "plan/*.md"}))
	// `[` opens a character class that's never closed; doublestar
	// would silently mismatch every path, so we reject it upfront.
	err := validateIncludePatterns([]string{"["})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --include glob")
}

func TestWorkspaceRelativePath_EmptyRootDir(t *testing.T) {
	// When rootDir is empty, the helper just strips a leading "./"
	// and forwards the path through.
	assert.Equal(t, "docs/api.md", workspaceRelativePath("./docs/api.md", ""))
	assert.Equal(t, "docs/api.md", workspaceRelativePath("docs/api.md", ""))
}

func TestEmitBacklinks_LimitZeroNoCap(t *testing.T) {
	// limit=0 means "no cap" — every record is emitted.
	records := make([]backlinks.Record, 5)
	for i := range records {
		records[i] = backlinks.Record{Source: "a.md", Line: i + 1}
	}
	var buf bytes.Buffer
	code := emitBacklinks(&buf, records, "text", 0)
	assert.Equal(t, 0, code)
	assert.Equal(t, 5, strings.Count(buf.String(), "\n"))
}

// failingWriter is an io.Writer that returns an error on every Write
// so tests can exercise emitBacklinks' write-error branches.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("simulated write failure")
}

func TestEmitBacklinks_TextWriteError(t *testing.T) {
	records := []backlinks.Record{{Source: "a.md", Line: 1, Text: "t", Target: "x.md"}}
	code := emitBacklinks(failingWriter{}, records, "text", 0)
	assert.Equal(t, 2, code)
}

func TestEmitBacklinks_JSONWriteError(t *testing.T) {
	code := emitBacklinks(failingWriter{}, []backlinks.Record{{Source: "a.md", Line: 1}}, "json", 0)
	assert.Equal(t, 2, code)
}

func TestFormatBacklinkTextLine_Wikilink(t *testing.T) {
	bare := backlinks.Record{
		Source: "from.md", Line: 4, Text: "page", Target: "page", Kind: "wikilink",
	}
	assert.Equal(t, "from.md:4: [[page]]", formatBacklinkTextLine(bare))

	alias := backlinks.Record{
		Source: "from.md", Line: 4, Text: "Display", Target: "page",
		Kind: "wikilink", Alias: "Display",
	}
	assert.Equal(t, "from.md:4: [[page|Display]]", formatBacklinkTextLine(alias))

	anchorNoAlias := backlinks.Record{
		Source: "from.md", Line: 4, Text: "page",
		Target: "page#Section", Kind: "wikilink",
	}
	// Anchor-only refs must not be rewritten as `|page` aliases; the
	// alias half is empty so the format emits only the target.
	assert.Equal(t, "from.md:4: [[page#Section]]", formatBacklinkTextLine(anchorNoAlias))

	embed := backlinks.Record{
		Source: "from.md", Line: 4, Text: "img.png",
		Target: "img.png", Kind: "wikilink", Embed: true,
	}
	assert.Equal(t, "from.md:4: ![[img.png]]", formatBacklinkTextLine(embed))

	std := backlinks.Record{
		Source: "from.md", Line: 1, Text: "ref", Target: "x.md",
	}
	assert.Equal(t, "from.md:1: [ref](x.md)", formatBacklinkTextLine(std))
}
