package requiredstructure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertNoInBodySyncPoints fails when any collected sync point is an
// in-body one — the shared assertion of the PI-opacity tests.
func assertNoInBodySyncPoints(t *testing.T, tmpl *parsedSchema) {
	t.Helper()
	for idx, sps := range tmpl.SyncPoints {
		for _, sp := range sps {
			assert.False(t, sp.InBody,
				"heading %d: a directive body must not yield a body sync point (got field %q)",
				idx, sp.Field)
		}
	}
}

// assertHasInBodySyncPoint fails unless some in-body sync point was
// collected for field — the positive control of the PI-opacity tests.
func assertHasInBodySyncPoint(t *testing.T, tmpl *parsedSchema, field, msg string) {
	t.Helper()
	for _, sps := range tmpl.SyncPoints {
		for _, sp := range sps {
			if sp.InBody && sp.Field == field {
				return
			}
		}
	}
	assert.Failf(t, msg,
		"no in-body sync point for field %q; collected sync points: %v",
		field, tmpl.SyncPoints)
}

// TestParseSchema_ContentDirectiveNoBodySync locks down plan 242: the
// legacy MDS020 proto parser must treat a `<?content?>` directive row
// as opaque. A `{field}`-looking token inside the directive body
// (here `bind: tag-{id}`) must NOT become a body-sync point — the row
// is a schema directive, not body-sync template text.
func TestParseSchema_ContentDirectiveNoBodySync(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n<?content\nkind: paragraph\nbind: tag-{id}\n?>\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	assertNoInBodySyncPoints(t, tmpl)
}

// TestParseSchema_ContentDirectiveSingleLineNoBodySync covers the
// single-line directive form `<?content ... ?>`: a `{field}` token on
// that one line must also stay opaque to the body-sync collector.
func TestParseSchema_ContentDirectiveSingleLineNoBodySync(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n<?content kind: paragraph bind: tag-{id} ?>\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	assertNoInBodySyncPoints(t, tmpl)
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
	assertNoInBodySyncPoints(t, tmpl)
}

// TestParseSchema_AnyPIBlockSkipped verifies the skip is not
// content-specific: a `{field}` token inside any directive body
// (here `<?require?>`) stays opaque to the body-sync collector.
func TestParseSchema_AnyPIBlockSkipped(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n<?require\nfilename: \"{id}.md\"\n?>\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	assertNoInBodySyncPoints(t, tmpl)
}

// TestParseSchema_FencedDirectiveExampleDoesNotOpenPIBlock locks
// down fence precedence: the block parser never opens a PI inside a
// fenced code block, so a directive opener shown in a fence must not
// flip the scanner into PI-skip state — a `{field}` body line after
// the fence is still collected.
func TestParseSchema_FencedDirectiveExampleDoesNotOpenPIBlock(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n```markdown\n<?content\nkind: paragraph\n```\n\ntag-{id} body line\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	assertHasInBodySyncPoint(t, tmpl, "id",
		"the {id} line after the fence is body text; the fenced directive example must not suppress it")
}

// TestParseSchema_FenceCloseRequiresMatchingChar locks down the
// CommonMark close rule the block parser applies: a tilde line inside
// a backtick fence does not close it, so a directive line that is
// still fence content stays body text and its `{field}` is collected.
func TestParseSchema_FenceCloseRequiresMatchingChar(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n```go\n~~~\n<?content bind: tag-{inner} ?>\n```\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	assertHasInBodySyncPoint(t, tmpl, "inner",
		"a ~~~ line must not close a backtick fence; the directive line is fence content")
}

// TestParseSchema_FenceCloseRequiresOpenerLength locks down the
// length half of the close rule: a three-backtick line does not close
// a four-backtick fence.
func TestParseSchema_FenceCloseRequiresOpenerLength(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n````\n```\n<?content bind: tag-{inner} ?>\n````\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	assertHasInBodySyncPoint(t, tmpl, "inner",
		"an inner ``` must not close a ```` fence; the directive line is fence content")
}

// TestParseSchema_FenceCloseRejectsTrailingContent locks down that a
// marker line followed by other text is not a closer.
func TestParseSchema_FenceCloseRejectsTrailingContent(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n```\n``` not-a-closer\n<?content bind: tag-{inner} ?>\n```\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	assertHasInBodySyncPoint(t, tmpl, "inner",
		"a ``` with trailing text must not close the fence; the directive line is fence content")
}

// TestParseSchema_FenceCloseRejectsNonASCIISpace locks down that only
// CommonMark whitespace may follow a closing run: an NBSP after the
// run is content, so the line does not close the fence and the
// directive line below it is still fence content.
func TestParseSchema_FenceCloseRejectsNonASCIISpace(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n```\n```\u00a0\n<?content bind: tag-{inner} ?>\n```\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	assertHasInBodySyncPoint(t, tmpl, "inner",
		"a ``` followed by an NBSP must not close the fence; the directive line is fence content")
}

// TestParseSchema_NBSPIndentedMarkerDoesNotOpenFence locks down that
// only spaces count as fence indentation: a marker after an NBSP is
// paragraph text, so the directive that follows is a real PI.
func TestParseSchema_NBSPIndentedMarkerDoesNotOpenFence(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n\u00a0```\n\n<?content bind: tag-{inner} ?>\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	assertNoInBodySyncPoints(t, tmpl)
}

// TestParseSchema_IndentedFenceMarkerDoesNotOpenFence locks down the
// indent half of the open rule: a 4-space-indented marker is indented
// code to the parser, so it must not flip the scanner into fence
// state — the directive that follows is a real PI and stays opaque.
func TestParseSchema_IndentedFenceMarkerDoesNotOpenFence(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n    ```\n\n<?content bind: tag-{inner} ?>\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	assertNoInBodySyncPoints(t, tmpl)
}

// TestParseSchema_TabIndentedFenceMarkers locks down that a tab before
// a marker reaches column four: a tab-indented marker neither opens
// nor closes a fence.
func TestParseSchema_TabIndentedFenceMarkers(t *testing.T) {
	open := "# {id}\n\n## Tagline\n\n\t```\n\n<?content bind: tag-{inner} ?>\n"
	tmpl, err := parseSchema([]byte(open), "", 0)
	require.NoError(t, err)
	assertNoInBodySyncPoints(t, tmpl)

	closing := "# {id}\n\n## Tagline\n\n```\n  \t```\n<?content bind: tag-{inner} ?>\n```\n"
	tmpl, err = parseSchema([]byte(closing), "", 0)
	require.NoError(t, err)
	assertHasInBodySyncPoint(t, tmpl, "inner",
		"a tab-indented ``` must not close the fence; the directive line is fence content")
}

// TestParseSchema_IndentedDirectiveExampleStaysBodyText is the
// positive control for the indent guard: a 4-space-indented line
// showing a directive is a code line to the parser, so its `{field}`
// token IS collected as a body-sync point.
func TestParseSchema_IndentedDirectiveExampleStaysBodyText(t *testing.T) {
	schemaSrc := "# {id}\n\n## Tagline\n\n    <?content {id} ?>\n"
	tmpl, err := parseSchema([]byte(schemaSrc), "", 0)
	require.NoError(t, err)
	assertHasInBodySyncPoint(t, tmpl, "id",
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
