package requiredstructure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An unquoted YAML date (`date: 2026-01-02`) decodes to time.Time. The
// front-matter CUE check sees it as the text mdsmith renders for it
// everywhere else — `2026-01-02` — so a `string` or regex constraint
// accepts it without quoting, and a non-string constraint still rejects
// it with the value it checked.

func TestCheck_InlineFrontmatterString_UnquotedDatePasses(t *testing.T) {
	for _, expr := range []string{"string", "date", `=~"^2026-01-"`} {
		t.Run(expr, func(t *testing.T) {
			r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
				"frontmatter": map[string]any{"date": expr},
			})}
			f := newTestFile(t, "doc.md",
				"---\ndate: 2026-01-02\n---\n# Post\n")
			expectDiags(t, r.Check(f), 0)
		})
	}
}

// The combination that forced quoting before: `date: string` plus a
// `\#(fmvar(date))` filename glob, on a file with an unquoted date.
func TestCheck_InlineFrontmatterStringAndFmvarFilename_UnquotedDate(t *testing.T) {
	root := t.TempDir()
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"frontmatter": map[string]any{"date": "string"},
		"filename":    `\#(fmvar(date))-*.md`,
	})}
	good := newRootedFile(t, root, "2026-01-02-post.md",
		"---\ndate: 2026-01-02\n---\n# Post\n")
	expectDiags(t, r.Check(good), 0)

	bad := newRootedFile(t, root, "2026-01-03-post.md",
		"---\ndate: 2026-01-02\n---\n# Post\n")
	expectDiags(t, r.Check(bad), 1)
}

func TestCheck_InlineFrontmatterInt_UnquotedDateFails(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"frontmatter": map[string]any{"date": "int"},
	})}
	f := newTestFile(t, "doc.md",
		"---\ndate: 2026-01-02\n---\n# Post\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `date: got "2026-01-02", expected int`)
	assert.NotContains(t, diags[0].Message, "unsupported")
}

// A failing regex shows the value it checked, the rendered date, not
// Go's or JSON's form of the time.
func TestCheck_InlineFrontmatterRegex_UnquotedDateMismatchShowsRenderedValue(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"frontmatter": map[string]any{"date": `=~"^2025-"`},
	})}
	f := newTestFile(t, "doc.md",
		"---\ndate: 2026-01-02\n---\n# Post\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `date: got "2026-01-02"`)
	assert.NotContains(t, diags[0].Message, "T00:00:00")
}

// A near-miss against a string disjunction hints on the rendered date,
// the same way it hints on a quoted one.
func TestCheck_InlineFrontmatterDisjunction_UnquotedDateHints(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"frontmatter": map[string]any{"date": `"2026-01-03" | "2027-06-30"`},
	})}
	f := newTestFile(t, "doc.md",
		"---\ndate: 2026-01-02\n---\n# Post\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `did you mean "2026-01-03"`)
}

// The `date` and `datetime` shortcuts check an unquoted value in its
// rendered form, which is not always its source text.
func TestCheck_DateShortcuts_UnquotedValues(t *testing.T) {
	cases := []struct {
		shortcut, value string
		pass            bool
	}{
		{"date", "2026-01-02", true},
		// YAML reads a single-digit month and day as a date; it
		// renders zero-padded.
		{"date", "2026-1-2", true},
		{"date", "2026-01-02T10:00:00Z", false},
		{"datetime", "2026-01-02T10:00:00Z", true},
		{"datetime", "2026-01-02T10:00:00+02:00", true},
		// Space-separated, no zone: renders 2026-01-02T10:00:00Z.
		{"datetime", "2026-01-02 10:00:00", true},
		// Midnight UTC renders as the bare date.
		{"datetime", "2026-01-02T00:00:00Z", false},
		// Fractional seconds survive rendering; datetime rejects them.
		{"datetime", "2026-01-02T10:00:00.5Z", false},
	}
	for _, tc := range cases {
		t.Run(tc.shortcut+" "+tc.value, func(t *testing.T) {
			r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
				"frontmatter": map[string]any{"v": tc.shortcut},
			})}
			f := newTestFile(t, "doc.md",
				"---\nv: "+tc.value+"\n---\n# Post\n")
			want := 1
			if tc.pass {
				want = 0
			}
			expectDiags(t, r.Check(f), want)
		})
	}
}
