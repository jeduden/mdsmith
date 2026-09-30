package schema

import (
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// An unquoted YAML date (`date: 2026-01-02`) decodes to time.Time.
// Every `fmvar(...)` surface must substitute it in the form the
// author wrote, YYYY-MM-DD, or a date-prefixed name can never match.

// dateFM decodes front matter the way the rule does, so the value
// under test is the time.Time yaml.v3 produces, not a hand-built
// string.
func dateFM(t *testing.T, src string) map[string]any {
	t.Helper()
	var fm map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(src), &fm))
	return fm
}

func TestValidateFilename_FmvarUnquotedDateMatches(t *testing.T) {
	sch := &Schema{
		Filename: []string{`\#(fmvar(date))-*.md`},
		Source:   "kind post",
	}
	doc := newDocFile(t, "2026-01-02-post.md",
		"---\ndate: 2026-01-02\n---\n# T\n")
	diags := Validate(doc, sch, dateFM(t, "date: 2026-01-02"),
		false, makeDiagForTest)
	assert.Empty(t, diags, "got %v", diagsMessages(diags))
}

func TestResolveGlobPattern_UnquotedDateSubstitutesAsWritten(t *testing.T) {
	got, err := ResolveGlobPattern(`posts/\#(fmvar(date))/index.md`,
		dateFM(t, "date: 2026-01-02"))
	require.NoError(t, err)
	assert.Equal(t, "posts/2026-01-02/index.md", got)
	assert.True(t, doublestar.MatchUnvalidated(got, "posts/2026-01-02/index.md"))
}

func TestValidate_RegexFmvarUnquotedDateMatchesHeading(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex": `Release \#(fmvar(date))`,
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n\n## Release 2026-01-02\n\nx\n")
	diags := Validate(doc, sch, dateFM(t, "date: 2026-01-02"),
		false, makeDiagForTest)
	assert.Empty(t, diags, "got %v", diagsMessages(diags))
}

func TestScopeCaptures_FmvarUnquotedDate(t *testing.T) {
	dh := DocHeading{Level: 2, Text: "2026-01-02", Line: 1}
	sc := &Scope{Heading: "{date}", Matcher: &Matcher{Regex: `\#(fmvar(date))`}}
	caps := scopeCaptures(sc, dh, dateFM(t, "date: 2026-01-02"))
	assert.Equal(t, "2026-01-02", caps["date"])
}
