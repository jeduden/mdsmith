package requiredstructure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseSchema_ContentDirectiveNoBodySync locks down plan 242: the
// legacy MDS020 proto parser must treat a `<?content?>` directive row
// as opaque. A `{field}`-looking token inside the directive body
// (here `bind: tag-{id}`) must NOT become a body-sync point — the row
// is a schema directive, not body-sync template text.
func TestParseSchema_ContentDirectiveNoBodySync(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n<?content\nkind: paragraph\nbind: tag-{id}\n?>\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	for idx, sps := range tmpl.SyncPoints {
		for _, sp := range sps {
			assert.False(t, sp.InBody,
				"heading %d: <?content?> body must not yield a body sync point (got field %q)",
				idx, sp.Field)
		}
	}
}

// TestParseSchema_ContentDirectiveSingleLineNoBodySync covers the
// single-line directive form `<?content ... ?>`: a `{field}` token on
// that one line must also stay opaque to the body-sync collector.
func TestParseSchema_ContentDirectiveSingleLineNoBodySync(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n<?content kind: paragraph bind: tag-{id} ?>\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	for idx, sps := range tmpl.SyncPoints {
		for _, sp := range sps {
			assert.False(t, sp.InBody,
				"heading %d: single-line <?content?> must not yield a body sync point (got field %q)",
				idx, sp.Field)
		}
	}
}

// TestParseSchema_ContentDirectiveNotAHeading verifies the legacy
// parser never treats a `<?content?>` row as a required heading: the
// schema below declares exactly one heading (the H2), so the directive
// row must not inflate the heading count.
func TestParseSchema_ContentDirectiveNotAHeading(t *testing.T) {
	schemaSrc := "## Tagline\n\n<?content\nkind: paragraph\n?>\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	require.Len(t, tmpl.Headings, 1,
		"only the H2 is a heading; the <?content?> row is a directive")
	assert.Equal(t, "Tagline", tmpl.Headings[0].Text)
}

// TestIsContentDirectiveOpen pins the directive-name boundary: only
// `<?content` followed by a non-identifier byte (or nothing) opens a
// block, so a differently named directive like `<?content-foo?>` is
// not mistaken for one.
func TestIsContentDirectiveOpen(t *testing.T) {
	assert.True(t, isContentDirectiveOpen([]byte("<?content kind: paragraph ?>")))
	assert.True(t, isContentDirectiveOpen([]byte("<?content?>")))
	assert.True(t, isContentDirectiveOpen([]byte("<?content")))
	assert.False(t, isContentDirectiveOpen([]byte("<?content-foo?>")))
	assert.False(t, isContentDirectiveOpen([]byte("<?contents kind: x ?>")))
	assert.False(t, isContentDirectiveOpen([]byte("plain body line")))
}

// TestHeadingIndexForLine covers both outcomes: the index of the
// first schema heading matching a body heading line, and -1 when no
// schema heading matches.
func TestHeadingIndexForLine(t *testing.T) {
	heads := []docHeading{{Text: "Tagline"}, {Text: "Lead"}}
	assert.Equal(t, 1, headingIndexForLine(heads, "## Lead"))
	assert.Equal(t, -1, headingIndexForLine(heads, "## Other"))
}
