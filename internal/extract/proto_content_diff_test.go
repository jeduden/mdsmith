package extract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtract_ProtoContentMatchesInline is plan 242's differential
// acceptance test: one schema expressed both ways — a proto.md file
// with `<?content?>` rows and the equivalent inline `content:` list —
// projects byte-identical section content from the same document body,
// modulo the proto form's H1 root wrapper.
//
// The proto schema roots at H1 (`# {title}`), so its section objects
// nest one level under the H1's projection key; the inline schema
// roots at H2 (it cannot express the `# {title}` heading-sync), so its
// section objects sit at the root. Comparing the proto's unwrapped
// section subtree against the inline root removes that level
// difference; the remaining content must match exactly.
func TestExtract_ProtoContentMatchesInline(t *testing.T) {
	body := "# My Title\n\n## Tagline\n\nThe tagline text.\n\n" +
		"## Snippet\n\n```go\nx := 1\n```\n"

	// Proto form: `<?content?>` rows under H1-rooted headings.
	dir := t.TempDir()
	protoPath := filepath.Join(dir, "schema.md")
	require.NoError(t, os.WriteFile(protoPath, []byte(
		"# {title}\n\n"+
			"## Tagline\n\n<?content\nkind: paragraph\n?>\n\n"+
			"## Snippet\n\n<?content\nkind: code-block\nlang: go\n?>\n"), 0o644))
	protoSch, err := schema.ParseFile(&schema.FileReader{}, protoPath)
	require.NoError(t, err)
	gotProto, diags := run(t, body, protoSch, map[string]any{"title": "My Title"})
	require.Empty(t, diags)
	protoRoot := gotProto.(map[string]any)

	// Inline form: the same two sections with `content:` lists,
	// rooted at H2.
	inlineSch, err := schema.ParseInline(map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Tagline",
				"content": []any{map[string]any{"kind": "paragraph"}},
			},
			map[string]any{
				"heading": "Snippet",
				"content": []any{map[string]any{"kind": "code-block", "lang": "go"}},
			},
		},
	}, "kind k")
	require.NoError(t, err)
	inlineBody := "## Tagline\n\nThe tagline text.\n\n## Snippet\n\n```go\nx := 1\n```\n"
	gotInline, diags := run(t, inlineBody, inlineSch, nil)
	require.Empty(t, diags)
	inlineRoot := gotInline.(map[string]any)

	// Unwrap the proto H1 scope (key derived from the `{title}` fmvar)
	// and drop its own heading-sync key so only the projected sections
	// remain — the same shape the inline root carries beside its
	// frontmatter object.
	h1, ok := protoRoot["title"].(map[string]any)
	require.True(t, ok, "proto root must project the H1 scope under `title`, got %T",
		protoRoot["title"])
	delete(h1, "title") // the H1's `# {title}` heading-sync value

	for _, key := range []string{"tagline", "snippet"} {
		require.Contains(t, h1, key,
			"proto projection must carry section %q", key)
		assert.Equal(t, jsonBytes(t, inlineRoot[key]), jsonBytes(t, h1[key]),
			"section %q must project identically from proto and inline schemas", key)
	}
}

// jsonBytes marshals v to canonical JSON for byte-level comparison.
func jsonBytes(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
