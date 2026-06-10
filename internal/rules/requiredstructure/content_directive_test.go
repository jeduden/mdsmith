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

// TestIsPIOpenLine mirrors the block parser's opener rules: at most
// three spaces of indentation, a `<?` prefix, and a non-empty name.
// Any directive name opens a block (directive bodies are schema
// syntax, never body-sync template text); an indented code example
// or a nameless `<?` does not.
func TestIsPIOpenLine(t *testing.T) {
	assert.True(t, isPIOpenLine([]byte("<?content kind: paragraph ?>")))
	assert.True(t, isPIOpenLine([]byte("<?content?>")))
	assert.True(t, isPIOpenLine([]byte("<?content")))
	assert.True(t, isPIOpenLine([]byte("<?content-foo?>")))
	assert.True(t, isPIOpenLine([]byte("<?require filename: \"x.md\" ?>")))
	assert.True(t, isPIOpenLine([]byte("   <?include")))
	assert.False(t, isPIOpenLine([]byte("    <?content?>")),
		"4-space indent is a code line to the parser")
	assert.False(t, isPIOpenLine([]byte("<? content?>")),
		"whitespace after <? means no name")
	assert.False(t, isPIOpenLine([]byte("<??>")), "empty name")
	assert.False(t, isPIOpenLine([]byte("<?")), "bare opener, no name")
	assert.False(t, isPIOpenLine([]byte("plain body line")))
}

// TestParseSchema_PIBlockClosesOnExactLineOnly locks down the close
// rule shared with the block parser: a `?>` substring inside a YAML
// value does not end the block, so a `{field}` token on a later
// directive line still yields no body-sync point.
func TestParseSchema_PIBlockClosesOnExactLineOnly(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n<?content\nkind: paragraph\nbind: \"a?>b\"\nalso: tag-{id}\n?>\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	for idx, sps := range tmpl.SyncPoints {
		for _, sp := range sps {
			assert.False(t, sp.InBody,
				"heading %d: a `?>` substring must not end the block early (got field %q)",
				idx, sp.Field)
		}
	}
}

// TestParseSchema_AnyPIBlockSkipped verifies the skip is not
// content-specific: a `{field}` token inside any directive body
// (here `<?require?>`) stays opaque to the body-sync collector.
func TestParseSchema_AnyPIBlockSkipped(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n<?require\nfilename: \"{id}.md\"\n?>\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	for idx, sps := range tmpl.SyncPoints {
		for _, sp := range sps {
			assert.False(t, sp.InBody,
				"heading %d: <?require?> body must not yield a body sync point (got field %q)",
				idx, sp.Field)
		}
	}
}

// TestParseSchema_IndentedDirectiveExampleStaysBodyText is the
// positive control for the indent guard: a 4-space-indented line
// showing a directive is a code line to the parser, so its `{field}`
// token IS collected as a body-sync point.
func TestParseSchema_IndentedDirectiveExampleStaysBodyText(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n    <?content {id} ?>\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	found := false
	for _, sps := range tmpl.SyncPoints {
		for _, sp := range sps {
			if sp.InBody && sp.Field == "id" {
				found = true
			}
		}
	}
	assert.True(t, found,
		"an indented directive example is body text; its {id} must be collected")
}

// TestHeadingIndexForLine covers both outcomes: the index of the
// first schema heading matching a body heading line, and -1 when no
// schema heading matches.
func TestHeadingIndexForLine(t *testing.T) {
	heads := []docHeading{{Text: "Tagline"}, {Text: "Lead"}}
	assert.Equal(t, 1, headingIndexForLine(heads, "## Lead"))
	assert.Equal(t, -1, headingIndexForLine(heads, "## Other"))
}
