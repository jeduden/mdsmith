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
