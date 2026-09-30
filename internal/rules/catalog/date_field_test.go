package catalog

import (
	"testing"
	"testing/fstest"
)

// An unquoted YAML date (`date: 2026-01-02`) decodes to time.Time.
// A `{date}` row placeholder must render it as the YYYY-MM-DD the
// author wrote, and `sort: date` must order it against a quoted date
// on that same text.
func TestRendering_UnquotedDateRendersAsWritten(t *testing.T) {
	src := `<?catalog
glob: "posts/*.md"
sort: date
row: "- {date} [{title}]({filename})"
?>
- 2026-01-02 [First](posts/a.md)
- 2026-01-03 [Second](posts/b.md)
<?/catalog?>
`
	mapFS := fstest.MapFS{
		"posts/a.md": {Data: []byte("---\ntitle: First\ndate: 2026-01-02\n---\n# A\n")},
		"posts/b.md": {Data: []byte("---\ntitle: Second\ndate: \"2026-01-03\"\n---\n# B\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	expectDiags(t, r.Check(f), 0)
}

// `sort: date` orders unquoted timestamps by the instant they name,
// not by their rendered text. Rendered, `2026-01-02` sorts before
// `2026-01-02T01:00:00+05:00` (2026-01-01T20:00Z), and
// `2026-01-02T10:00:00-05:00` (15:00Z) before `2026-01-02T12:00:00Z`.
// The rows keep the rendered form; only the order changes.
func TestSort_TimestampsOrderChronologically(t *testing.T) {
	mapFS := fstest.MapFS{
		"posts/a.md": {Data: []byte("---\ndate: 2026-01-02T10:00:00-05:00\n---\n# A\n")},
		"posts/b.md": {Data: []byte("---\ndate: 2026-01-02T12:00:00Z\n---\n# B\n")},
		"posts/c.md": {Data: []byte("---\ndate: 2026-01-02\n---\n# C\n")},
		"posts/d.md": {Data: []byte("---\ndate: 2026-01-02T00:00:00Z\n---\n# D\n")},
		"posts/e.md": {Data: []byte("---\ndate: 2026-01-02T01:00:00+05:00\n---\n# E\n")},
	}
	for _, tc := range []struct{ sort, body string }{
		{"date", `- 2026-01-02T01:00:00+05:00 posts/e.md
- 2026-01-02 posts/c.md
- 2026-01-02 posts/d.md
- 2026-01-02T12:00:00Z posts/b.md
- 2026-01-02T10:00:00-05:00 posts/a.md
`},
		// Descending reverses the instants; equal instants keep the
		// ascending path tiebreaker.
		{"-date", `- 2026-01-02T10:00:00-05:00 posts/a.md
- 2026-01-02T12:00:00Z posts/b.md
- 2026-01-02 posts/c.md
- 2026-01-02 posts/d.md
- 2026-01-02T01:00:00+05:00 posts/e.md
`},
	} {
		t.Run(tc.sort, func(t *testing.T) {
			src := "<?catalog\nglob: \"posts/*.md\"\nsort: " + tc.sort +
				"\nrow: \"- {date} {filename}\"\n?>\n" + tc.body +
				"<?/catalog?>\n"
			f := newTestFile(t, "index.md", src, mapFS)
			r := &Rule{}
			expectDiags(t, r.Check(f), 0)
		})
	}
}

// A quoted RFC 3339 string in the same `sort:` column interleaves
// with unquoted timestamps by instant: `"...10:00:00-05:00"` is 15:00
// UTC, after an unquoted 14:00 UTC, though its text sorts first. The
// row keeps the quoted text as written.
func TestSort_QuotedTimestampInterleavesWithUnquoted(t *testing.T) {
	mapFS := fstest.MapFS{
		"posts/a.md": {Data: []byte("---\ndate: \"2026-01-02T10:00:00-05:00\"\n---\n# A\n")},
		"posts/b.md": {Data: []byte("---\ndate: 2026-01-02T14:00:00Z\n---\n# B\n")},
		"posts/c.md": {Data: []byte("---\ndate: 2026-01-02T16:00:00Z\n---\n# C\n")},
	}
	src := "<?catalog\nglob: \"posts/*.md\"\nsort: date\n" +
		"row: \"- {date} {filename}\"\n?>\n" +
		"- 2026-01-02T14:00:00Z posts/b.md\n" +
		"- 2026-01-02T10:00:00-05:00 posts/a.md\n" +
		"- 2026-01-02T16:00:00Z posts/c.md\n" +
		"<?/catalog?>\n"
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	expectDiags(t, r.Check(f), 0)
}

// A quoted `YYYY-MM-DDTHH:MM` or `YYYY-MM-DD HH:MM:SS` names an
// instant too. As text, `2026-01-02 09:00:00` sorts before an
// unquoted 08:00 UTC, since a space sorts before the `T` of a time
// key; each value keys on its UTC instant instead, a zone-less value
// counting as UTC.
func TestSort_QuotedClockFormsInterleaveByInstant(t *testing.T) {
	mapFS := fstest.MapFS{
		"posts/a.md": {Data: []byte("---\ndate: \"2026-01-02T10:00\"\n---\n# A\n")},
		"posts/b.md": {Data: []byte("---\ndate: \"2026-01-02 09:00:00\"\n---\n# B\n")},
		"posts/c.md": {Data: []byte("---\ndate: 2026-01-02T08:00:00Z\n---\n# C\n")},
		"posts/d.md": {Data: []byte("---\ndate: 2026-01-02\n---\n# D\n")},
		"posts/e.md": {Data: []byte("---\ndate: 2026-01-02T12:00:00Z\n---\n# E\n")},
	}
	src := "<?catalog\nglob: \"posts/*.md\"\nsort: date\n" +
		"row: \"- {date} {filename}\"\n?>\n" +
		"- 2026-01-02 posts/d.md\n" +
		"- 2026-01-02T08:00:00Z posts/c.md\n" +
		"- 2026-01-02 09:00:00 posts/b.md\n" +
		"- 2026-01-02T10:00 posts/a.md\n" +
		"- 2026-01-02T12:00:00Z posts/e.md\n" +
		"<?/catalog?>\n"
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	expectDiags(t, r.Check(f), 0)
}
