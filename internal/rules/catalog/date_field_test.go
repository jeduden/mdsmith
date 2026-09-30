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
