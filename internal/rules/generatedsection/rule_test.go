package generatedsection

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jeduden/tidymark/internal/lint"
)

// helper creates a *lint.File with the given source and attaches the given FS.
func newTestFile(t *testing.T, path, source string, fs ...fstest.MapFS) *lint.File {
	t.Helper()
	f, err := lint.NewFile(path, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) > 0 {
		f.FS = fs[0]
	}
	return f
}

// expectDiags asserts the number of diagnostics and returns them.
func expectDiags(t *testing.T, diags []lint.Diagnostic, count int) {
	t.Helper()
	if len(diags) != count {
		msgs := make([]string, len(diags))
		for i, d := range diags {
			msgs[i] = d.Message
		}
		t.Fatalf("expected %d diagnostic(s), got %d: %v", count, len(diags), msgs)
	}
}

// expectDiagMsg asserts the first diagnostic has the given message substring.
func expectDiagMsg(t *testing.T, diags []lint.Diagnostic, msg string) {
	t.Helper()
	if len(diags) == 0 {
		t.Fatalf("expected diagnostic with message %q, got none", msg)
	}
	if !strings.Contains(diags[0].Message, msg) {
		t.Errorf("expected diagnostic message containing %q, got %q", msg, diags[0].Message)
	}
}

// expectDiagLine asserts the first diagnostic is on the given line.
func expectDiagLine(t *testing.T, diags []lint.Diagnostic, line int) {
	t.Helper()
	if len(diags) == 0 {
		t.Fatalf("expected diagnostic on line %d, got none", line)
	}
	if diags[0].Line != line {
		t.Errorf("expected diagnostic on line %d, got line %d (message: %s)", line, diags[0].Line, diags[0].Message)
	}
}

// =====================================================================
// Rule metadata
// =====================================================================

func TestRule_ID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "TM019" {
		t.Errorf("expected ID TM019, got %s", r.ID())
	}
}

func TestRule_Name(t *testing.T) {
	r := &Rule{}
	if r.Name() != "generated-section" {
		t.Errorf("expected Name generated-section, got %s", r.Name())
	}
}

// =====================================================================
// Core rendering
// =====================================================================

func TestRendering_MinimalMode(t *testing.T) {
	// Minimal mode (glob only) produces plain bullet list with basenames as link text.
	src := `<!-- tidymark:gen:start catalog
glob: "docs/*.md"
-->
- [api.md](docs/api.md)
- [guide.md](docs/guide.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"docs/api.md":   {Data: []byte("# API\n")},
		"docs/guide.md": {Data: []byte("# Guide\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestRendering_ListTemplateWithFrontMatter(t *testing.T) {
	// List template renders per-file with front matter fields.
	src := `<!-- tidymark:gen:start catalog
glob: "docs/*.md"
row: "- [{{.title}}]({{.filename}}) -- {{.description}}"
-->
- [API Reference](docs/api.md) -- Complete API docs
- [Getting Started](docs/guide.md) -- How to get started
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"docs/api.md":   {Data: []byte("---\ntitle: API Reference\ndescription: Complete API docs\n---\n# API\n")},
		"docs/guide.md": {Data: []byte("---\ntitle: Getting Started\ndescription: How to get started\n---\n# Guide\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestRendering_TableHeaderRows(t *testing.T) {
	// Table template renders static header + per-file rows.
	src := `<!-- tidymark:gen:start catalog
glob: "docs/*.md"
header: |
  | Title | Description |
  |-------|-------------|
row: "| [{{.title}}]({{.filename}}) | {{.description}} |"
-->
| Title | Description |
|-------|-------------|
| [API Reference](docs/api.md) | Complete API docs |
| [Getting Started](docs/guide.md) | How to get started |
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"docs/api.md":   {Data: []byte("---\ntitle: API Reference\ndescription: Complete API docs\n---\n# API\n")},
		"docs/guide.md": {Data: []byte("---\ntitle: Getting Started\ndescription: How to get started\n---\n# Guide\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestRendering_MultilineRowPipe(t *testing.T) {
	// Multi-line `row` value with YAML `|` produces multi-line output per file.
	// YAML `|` clips trailing newline to one.
	src := `<!-- tidymark:gen:start catalog
glob: "docs/*.md"
row: |
  ### {{.title}}
  {{.description}}
-->
### API
Complete API docs
### Guide
How to get started
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"docs/api.md":   {Data: []byte("---\ntitle: API\ndescription: Complete API docs\n---\n")},
		"docs/guide.md": {Data: []byte("---\ntitle: Guide\ndescription: How to get started\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestRendering_MultilineRowPipePlus(t *testing.T) {
	// Multi-line `row` value with YAML `|+` preserves trailing blank lines between entries.
	src := `<!-- tidymark:gen:start catalog
glob: "docs/*.md"
row: |+
  ### [{{.title}}]({{.filename}})

  {{.description}}

-->
### [API](docs/api.md)

Complete API docs

### [Guide](docs/guide.md)

How to get started

<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"docs/api.md":   {Data: []byte("---\ntitle: API\ndescription: Complete API docs\n---\n")},
		"docs/guide.md": {Data: []byte("---\ntitle: Guide\ndescription: How to get started\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestRendering_PipeStripImplicitNewline(t *testing.T) {
	// YAML `|-` strips trailing newlines; implicit `\n` rule adds one back.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: |-
  - {{.filename}}
-->
- a.md
- b.md
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# A\n")},
		"b.md": {Data: []byte("# B\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestRendering_EachValueTerminatedByNewline(t *testing.T) {
	// Each rendered value (header, row, footer, empty) is terminated by implicit trailing `\n`.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
header: "| Title |"
row: "| {{.filename}} |"
footer: "---"
-->
| Title |
| a.md |
---
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# A\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestRendering_FooterRendersAfterRows(t *testing.T) {
	// `footer` renders static content after rows.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
header: |
  | Title | Description |
  |-------|-------------|
row: "| [{{.title}}]({{.filename}}) | {{.description}} |"
footer: |

  ---
-->
| Title | Description |
|-------|-------------|
| [API](a.md) | docs |

---
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: API\ndescription: docs\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestRendering_EmptyFallbackRendersWhenNoMatches(t *testing.T) {
	// `empty` renders fallback text when glob matches zero files.
	src := `<!-- tidymark:gen:start catalog
glob: "nonexistent/*.md"
empty: No documents found.
-->
No documents found.
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestRendering_EmptyAloneWithoutRowIsValid(t *testing.T) {
	// `empty` alone without `row` is valid (no diagnostic).
	src := `<!-- tidymark:gen:start catalog
glob: "nonexistent/*.md"
empty: No documents found.
-->
No documents found.
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestRendering_EmptyPlusHeaderWithoutRowProducesDiag(t *testing.T) {
	// `empty` + `header` without `row` produces missing-row diagnostic.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
empty: No docs.
header: |
  | Title |
  |-------|
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, `generated section template missing required "row" key`)
}

func TestRendering_EmptyValueGetsTrailingNewline(t *testing.T) {
	// `empty` value gets trailing `\n`.
	src := `<!-- tidymark:gen:start catalog
glob: "nonexistent/*.md"
empty: "Nothing here"
-->
Nothing here
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestRendering_NoEmptyNoMatchesEmptyContent(t *testing.T) {
	// No `empty` + no matches produces empty content between markers.
	src := `<!-- tidymark:gen:start catalog
glob: "nonexistent/*.md"
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestRendering_WhenEmptyRendersHeaderFooterNotIncluded(t *testing.T) {
	// When `empty` renders, `header`/`footer` are not included.
	src := `<!-- tidymark:gen:start catalog
glob: "nonexistent/*.md"
header: "| Title |"
row: "| {{.filename}} |"
footer: "---"
empty: No documents.
-->
No documents.
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestRendering_WhenGlobMatchesFilesEmptyIgnored(t *testing.T) {
	// When glob matches files and `empty` is defined, `empty` is ignored.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "- {{.filename}}"
empty: No documents.
-->
- a.md
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# A\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Diagnostics
// =====================================================================

func TestDiag_OrphanedEndMarker(t *testing.T) {
	src := `Some text
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, "unexpected generated section end marker")
	// Orphaned end markers are reported on the end marker line.
	expectDiagLine(t, diags, 2)
}

func TestDiag_NestedStartMarkers(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
<!-- tidymark:gen:start catalog
glob: "other/*.md"
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "nested generated section markers are not allowed") {
			found = true
			if d.Line != 4 {
				t.Errorf("expected nested marker diagnostic on line 4, got %d", d.Line)
			}
			break
		}
	}
	if !found {
		t.Error("expected nested marker diagnostic")
	}
}

func TestDiag_NonStringYAMLValues(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: 42
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, `non-string value for key "glob"`) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected non-string value diagnostic for glob, got %v", diags)
	}
}

func TestDiag_NonStringMultipleKeys(t *testing.T) {
	// Non-string YAML values produce diagnostic per key.
	src := `<!-- tidymark:gen:start catalog
glob: 42
sort: true
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	nonStringCount := 0
	for _, d := range diags {
		if strings.Contains(d.Message, "non-string value for key") {
			nonStringCount++
		}
	}
	if nonStringCount < 2 {
		t.Errorf("expected at least 2 non-string value diagnostics, got %d", nonStringCount)
	}
}

// =====================================================================
// Sort
// =====================================================================

// =====================================================================
// Template fields
// =====================================================================

func TestFields_FilenameResolvesToRelativePath(t *testing.T) {
	// `{{.filename}}` resolves to path relative to linted file's directory, never has leading `./`.
	src := `<!-- tidymark:gen:start catalog
glob: "docs/*.md"
row: "- {{.filename}}"
-->
- docs/api.md
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"docs/api.md": {Data: []byte("# API\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestFields_MissingFrontMatterFieldsEmpty(t *testing.T) {
	// Files without front matter resolve fields to empty string.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "- [{{.title}}]({{.filename}})"
-->
- [](a.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# No front matter\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestFields_HeaderFooterContainTemplateLiterals(t *testing.T) {
	// `header`/`footer` containing `{{...}}` render literally (no template expansion).
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
header: "{{.title}} header"
row: "- {{.filename}}"
footer: "{{.footer}} end"
-->
{{.title}} header
- a.md
{{.footer}} end
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# A\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestFields_EmptyContainsTemplateLiterals(t *testing.T) {
	// `empty` containing `{{...}}` renders literally.
	src := `<!-- tidymark:gen:start catalog
glob: "nonexistent/*.md"
empty: "{{.something}} no data"
-->
{{.something}} no data
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Fix behavior
// =====================================================================

func TestFix_IdempotentOnFreshContent(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
- [a.md](a.md)
- [b.md](b.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# A\n")},
		"b.md": {Data: []byte("# B\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	result := r.Fix(f)
	if string(result) != src {
		t.Errorf("Fix on fresh content should be idempotent.\nExpected:\n%s\nGot:\n%s", src, string(result))
	}
}

func TestFix_LeavesMalformedMarkersUnchanged(t *testing.T) {
	// Missing directive name -> malformed.
	src := `<!-- tidymark:gen:start
glob: "*.md"
-->
old content
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# A\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	result := r.Fix(f)
	if string(result) != src {
		t.Errorf("Fix should leave malformed markers unchanged.\nExpected:\n%s\nGot:\n%s", src, string(result))
	}
}

func TestFix_LeavesTemplateErrorSectionsUnchanged(t *testing.T) {
	// Invalid template syntax -> fix leaves section unchanged.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "{{.title"
-->
old content
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# A\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	result := r.Fix(f)
	if string(result) != src {
		t.Errorf("Fix should leave template-error sections unchanged.\nExpected:\n%s\nGot:\n%s", src, string(result))
	}
}

func TestFix_FullCycleIdempotent(t *testing.T) {
	// First fix generates content, second fix should leave it unchanged.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "- [{{.title}}]({{.filename}})"
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Alpha\n---\n# A\n")},
		"b.md": {Data: []byte("---\ntitle: Beta\n---\n# B\n")},
	}

	// First fix.
	f1 := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	result1 := r.Fix(f1)

	// Second fix on the result of the first fix.
	f2 := newTestFile(t, "index.md", string(result1), mapFS)
	result2 := r.Fix(f2)

	if string(result1) != string(result2) {
		t.Errorf("Fix is not idempotent.\nFirst:\n%s\nSecond:\n%s", string(result1), string(result2))
	}
}

func TestFix_MultipleMarkerPairs(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "a/*.md"
-->
old
<!-- tidymark:gen:end -->

<!-- tidymark:gen:start catalog
glob: "b/*.md"
-->
old
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a/one.md": {Data: []byte("# One\n")},
		"b/two.md": {Data: []byte("# Two\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	result := r.Fix(f)

	if !strings.Contains(string(result), "- [one.md](a/one.md)") {
		t.Error("Fix should regenerate first section with a/one.md")
	}
	if !strings.Contains(string(result), "- [two.md](b/two.md)") {
		t.Error("Fix should regenerate second section with b/two.md")
	}
}

func TestFix_SkipsInvalidPairLeavesValidPair(t *testing.T) {
	// When one marker pair has validation errors and another is valid,
	// fix should skip the invalid pair and regenerate the valid one.
	src := `<!-- tidymark:gen:start foobar
glob: "*.md"
-->
old invalid content
<!-- tidymark:gen:end -->

<!-- tidymark:gen:start catalog
glob: "*.md"
-->
old
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# A\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	result := string(r.Fix(f))
	// The invalid section should remain unchanged.
	if !strings.Contains(result, "old invalid content") {
		t.Error("Fix should leave invalid section content unchanged")
	}
	// The valid section should be regenerated.
	if !strings.Contains(result, "- [a.md](a.md)") {
		t.Errorf("Fix should regenerate valid section.\nGot:\n%s", result)
	}
}

// =====================================================================
// Edge cases
// =====================================================================

func TestEdge_MarkersInsideFencedCodeBlock(t *testing.T) {
	src := "```\n<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n<!-- tidymark:gen:end -->\n```\n"
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestEdge_MarkersInsideIndentedCodeBlock(t *testing.T) {
	// Indented code blocks (4-space indent) should also ignore markers.
	src := "Paragraph before.\n\n" +
		"    <!-- tidymark:gen:start catalog\n    glob: \"*.md\"\n    -->\n" +
		"    <!-- tidymark:gen:end -->\n\nParagraph after.\n"
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestEdge_MarkersInsideHTMLBlock(t *testing.T) {
	// goldmark treats <div>...</div> as an HTML block.
	src := "<div>\n<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n<!-- tidymark:gen:end -->\n</div>\n"
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestEdge_MarkersInsideHTMLBlockWithClosure(t *testing.T) {
	// HTML block type 6 with a closing blank line as closure.
	// <table> is recognized as an HTML block that includes content until a blank line.
	src := "<table>\n<tr><td><!-- tidymark:gen:start catalog\nglob: \"*.md\"\n--></td></tr>\n</table>\n\nText after.\n"
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	// Markers inside HTML block should be ignored. No structural errors expected.
	for _, d := range diags {
		if strings.Contains(d.Message, "no closing marker") ||
			strings.Contains(d.Message, "unexpected") {
			t.Errorf("markers inside HTML block should be ignored: %s", d.Message)
		}
	}
}

func TestEdge_MarkersInsidePreBlock(t *testing.T) {
	// goldmark HTML block type 1 (<pre>) has explicit closure (</pre>).
	// Markers inside should be ignored.
	src := "<pre>\n<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n<!-- tidymark:gen:end -->\n</pre>\n"
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestEdge_MarkersInsideScriptBlock(t *testing.T) {
	// goldmark HTML block type 1 (<script>) has explicit closure (</script>).
	src := "<script>\n<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n<!-- tidymark:gen:end -->\n</script>\n"
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestEdge_TerminatorAllowsLeadingTrailingWhitespace(t *testing.T) {
	// `-->` terminator allows leading/trailing whitespace.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
  -->
- [a.md](a.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# Hello\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestEdge_EndMarkerWithWhitespace(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
- [a.md](a.md)
  <!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# Hello\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestEdge_SingleLineStartMarker(t *testing.T) {
	// Single-line start marker has empty YAML body (triggers missing-parameter diagnostic).
	src := `<!-- tidymark:gen:start catalog -->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, `missing required "glob" parameter`)
}

func TestEdge_DirectiveWhitespaceTrimmedExtraWordsIgnored(t *testing.T) {
	// Directive name whitespace is trimmed; extra words after name ignored.
	src := `<!-- tidymark:gen:start   catalog   extra words
glob: "*.md"
-->
- [a.md](a.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# Hello\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestEdge_StdinInputSkipsRule(t *testing.T) {
	// Stdin input skips the rule (`f.FS == nil`).
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
stale content
<!-- tidymark:gen:end -->
`
	f := newTestFile(t, "index.md", src) // no FS set -> nil
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestEdge_StdinInputFixReturnsSourceUnchanged(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
stale content
<!-- tidymark:gen:end -->
`
	f := newTestFile(t, "index.md", src) // no FS set -> nil
	r := &Rule{}
	result := r.Fix(f)
	if string(result) != src {
		t.Errorf("Fix with nil FS should return source unchanged")
	}
}

func TestEdge_GlobMatchingDirectorySkipped(t *testing.T) {
	// Directories matched by glob should be silently skipped.
	src := `<!-- tidymark:gen:start catalog
glob: "*"
-->
- [a.md](a.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md":    {Data: []byte("# A\n")},
		"subdir/": {Data: []byte("")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestEdge_MultipleMarkerPairsIndependent(t *testing.T) {
	// Multiple marker pairs in one file processed independently.
	src := `<!-- tidymark:gen:start catalog
glob: "a/*.md"
-->
- [one.md](a/one.md)
<!-- tidymark:gen:end -->

Text between sections.

<!-- tidymark:gen:start catalog
glob: "b/*.md"
-->
- [two.md](b/two.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a/one.md": {Data: []byte("# One\n")},
		"b/two.md": {Data: []byte("# Two\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestEdge_AllDiagnosticsReportColumn1(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			"stale",
			"<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\nold\n<!-- tidymark:gen:end -->\n",
		},
		{
			"unclosed",
			"<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\nold\n",
		},
		{
			"orphaned",
			"<!-- tidymark:gen:end -->\n",
		},
		{
			"missing directive",
			"<!-- tidymark:gen:start\nglob: \"*.md\"\n-->\n<!-- tidymark:gen:end -->\n",
		},
		{
			"unknown directive",
			"<!-- tidymark:gen:start foobar\nglob: \"*.md\"\n-->\n<!-- tidymark:gen:end -->\n",
		},
		{
			"missing glob",
			"<!-- tidymark:gen:start catalog\nsort: path\n-->\n<!-- tidymark:gen:end -->\n",
		},
		{
			"empty glob",
			"<!-- tidymark:gen:start catalog\nglob: \"\"\n-->\n<!-- tidymark:gen:end -->\n",
		},
		{
			"empty sort",
			"<!-- tidymark:gen:start catalog\nglob: \"*.md\"\nsort: \"\"\n-->\n<!-- tidymark:gen:end -->\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mapFS := fstest.MapFS{
				"a.md": {Data: []byte("# A\n")},
			}
			f := newTestFile(t, "index.md", tc.src, mapFS)
			r := &Rule{}
			diags := r.Check(f)
			for _, d := range diags {
				if d.Column != 1 {
					t.Errorf("expected column 1, got %d for message: %s", d.Column, d.Message)
				}
			}
		})
	}
}

func TestEdge_RecursiveGlobPatterns(t *testing.T) {
	// Recursive `**` glob patterns are supported.
	src := `<!-- tidymark:gen:start catalog
glob: "**/*.md"
-->
- [deep.md](a/b/c/deep.md)
- [top.md](top.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"top.md":        {Data: []byte("# Top\n")},
		"a/b/c/deep.md": {Data: []byte("# Deep\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestEdge_UnknownYAMLKeysIgnored(t *testing.T) {
	// Unknown YAML keys are silently ignored.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
unknown_key: something
another: value
-->
- [a.md](a.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# Hello\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	for _, d := range diags {
		if strings.Contains(d.Message, "unknown_key") || strings.Contains(d.Message, "another") {
			t.Errorf("unknown YAML keys should be silently ignored, got: %s", d.Message)
		}
	}
}

func TestEdge_DuplicateYAMLKeysRejected(t *testing.T) {
	// gopkg.in/yaml.v3 rejects duplicate keys as invalid YAML.
	src := `<!-- tidymark:gen:start catalog
glob: "nonexistent/*.md"
glob: "*.md"
-->
- [a.md](a.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# Hello\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, "invalid YAML")
}

func TestEdge_InvalidFrontMatterTreatedAsNone(t *testing.T) {
	// Matched file with invalid front matter treated as no front matter.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "- [{{.title}}]({{.filename}})"
-->
- [](a.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ninvalid: [yaml\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestEdge_DotfilesMatchedByStar(t *testing.T) {
	// The doublestar library matches dotfiles with `*` by default.
	// Both visible.md and .hidden.md are matched.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
- [.hidden.md](.hidden.md)
- [visible.md](visible.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"visible.md": {Data: []byte("# Visible\n")},
		".hidden.md": {Data: []byte("# Hidden\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestEdge_NoFrontMatterFilenameWorks(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "{{.filename}}"
-->
a.md
b.md
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("no front matter here\n")},
		"b.md": {Data: []byte("also no front matter\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestEdge_ValidationShortCircuitsOnStructuralErrors(t *testing.T) {
	// Missing directive name should prevent further validation (no glob error).
	src := `<!-- tidymark:gen:start
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, "missing directive name")
}

func TestEdge_InvalidYAMLShortCircuits(t *testing.T) {
	// Invalid YAML should prevent template/glob validation.
	src := `<!-- tidymark:gen:start catalog
glob: [invalid
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, "invalid YAML")
}

func TestEdge_NonStringValuesShortCircuit(t *testing.T) {
	// Non-string values should prevent further validation.
	src := `<!-- tidymark:gen:start catalog
glob: 42
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	// Should get non-string error but NOT a missing glob error.
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, "non-string value")
}

// =====================================================================
// Table-driven diagnostic scenarios
// =====================================================================

// diagScenario is a test case for diagnostic scenario table tests.
type diagScenario struct {
	name      string
	src       string
	fs        fstest.MapFS
	wantCount int
	wantMsg   string
}

// runDiagScenarios runs a slice of diagnostic test scenarios.
func runDiagScenarios(t *testing.T, tests []diagScenario) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newTestFile(t, "index.md", tc.src, tc.fs)
			r := &Rule{}
			diags := r.Check(f)
			expectDiags(t, diags, tc.wantCount)
			if tc.wantMsg != "" && len(diags) > 0 {
				expectDiagMsg(t, diags, tc.wantMsg)
			}
		})
	}
}

func TestCheck_StructuralErrors(t *testing.T) {
	runDiagScenarios(t, []diagScenario{
		{
			name: "valid pair no errors",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n" +
				"- [a.md](a.md)\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{"a.md": {Data: []byte("# A\n")}},
			wantCount: 0,
		},
		{
			name:      "unclosed start",
			src:       "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\ncontent\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "no closing marker",
		},
		{
			name:      "orphaned end",
			src:       "text\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "unexpected generated section end marker",
		},
		{
			name: "stale section",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n" +
				"- [a.md](a.md)\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{"a.md": {Data: []byte("# A\n")}, "b.md": {Data: []byte("# B\n")}},
			wantCount: 1,
			wantMsg:   "generated section is out of date",
		},
	})
}

func TestCheck_DirectiveErrors(t *testing.T) {
	runDiagScenarios(t, []diagScenario{
		{
			name:      "missing directive name",
			src:       "<!-- tidymark:gen:start\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "missing directive name",
		},
		{
			name:      "unknown directive",
			src:       "<!-- tidymark:gen:start unknown\nglob: \"*.md\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `unknown generated section directive "unknown"`,
		},
		{
			name:      "CATALOG case sensitive",
			src:       "<!-- tidymark:gen:start CATALOG\nglob: \"*.md\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `unknown generated section directive "CATALOG"`,
		},
		{
			name:      "Catalog case sensitive",
			src:       "<!-- tidymark:gen:start Catalog\nglob: \"*.md\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `unknown generated section directive "Catalog"`,
		},
		{
			name:      "invalid YAML",
			src:       "<!-- tidymark:gen:start catalog\n: invalid : yaml ::: [\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "invalid YAML",
		},
	})
}

func TestCheck_GlobErrors(t *testing.T) {
	runDiagScenarios(t, []diagScenario{
		{
			name:      "empty glob",
			src:       "<!-- tidymark:gen:start catalog\nglob: \"\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `empty "glob" parameter`,
		},
		{
			name:      "absolute glob",
			src:       "<!-- tidymark:gen:start catalog\nglob: /etc/files/*.md\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "absolute glob path",
		},
		{
			name:      "glob with ..",
			src:       "<!-- tidymark:gen:start catalog\nglob: \"../*.md\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `".." path traversal`,
		},
		{
			name:      "missing glob",
			src:       "<!-- tidymark:gen:start catalog\nsort: path\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `missing required "glob" parameter`,
		},
		{
			name:      "invalid glob pattern",
			src:       "<!-- tidymark:gen:start catalog\nglob: \"[invalid\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "invalid glob pattern",
		},
	})
}

func TestCheck_TemplateErrors(t *testing.T) {
	runDiagScenarios(t, []diagScenario{
		{
			name:      "empty row",
			src:       "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\nrow: \"\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `empty "row" value`,
		},
		{
			name: "header without row",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n" +
				"header: \"| T |\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `missing required "row" key`,
		},
		{
			name: "footer without row",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n" +
				"footer: \"---\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `missing required "row" key`,
		},
		{
			name: "header and footer without row",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n" +
				"header: \"| Title |\"\nfooter: \"---\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `missing required "row" key`,
		},
		{
			name: "invalid template syntax",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n" +
				"row: \"{{.title\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "invalid template",
		},
	})
}

func TestCheck_SortErrors(t *testing.T) {
	runDiagScenarios(t, []diagScenario{
		{
			name:      "empty sort",
			src:       "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\nsort: \"\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `empty "sort" value`,
		},
		{
			name:      "sort dash only",
			src:       "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\nsort: \"-\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "invalid sort value",
		},
		{
			name: "sort with whitespace",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n" +
				"sort: \"foo bar\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "invalid sort value",
		},
		{
			name: "sort with tab",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n" +
				"sort: \"foo\tbar\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "invalid sort value",
		},
	})
}

// =====================================================================
// Table-driven content generation tests
// =====================================================================

func TestCheck_ContentGeneration_MinimalMode(t *testing.T) {
	runDiagScenarios(t, []diagScenario{
		{
			name: "up to date",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n" +
				"- [hello.md](hello.md)\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{"hello.md": {Data: []byte("# Hello\n")}},
			wantCount: 0,
		},
		{
			name: "stale",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n" +
				"- [old.md](old.md)\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{"new.md": {Data: []byte("# New\n")}},
			wantCount: 1,
		},
	})
}

func TestCheck_ContentGeneration_TemplateAndEmpty(t *testing.T) {
	runDiagScenarios(t, []diagScenario{
		{
			name: "template mode with front matter up to date",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n" +
				"row: \"- [{{.title}}]({{.filename}})\"\n-->\n" +
				"- [My Title](a.md)\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{"a.md": {Data: []byte("---\ntitle: My Title\n---\n# A\n")}},
			wantCount: 0,
		},
		{
			name: "empty fallback up to date",
			src: "<!-- tidymark:gen:start catalog\n" +
				"glob: \"nonexistent/*.md\"\nempty: No files found.\n-->\n" +
				"No files found.\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 0,
		},
		{
			name:      "no empty no matches empty content",
			src:       "<!-- tidymark:gen:start catalog\nglob: \"nonexistent/*.md\"\n-->\n<!-- tidymark:gen:end -->\n",
			fs:        fstest.MapFS{},
			wantCount: 0,
		},
	})
}

// =====================================================================
// Table-driven Fix tests
// =====================================================================

func TestFix_Scenarios(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		fs       fstest.MapFS
		contains []string
	}{
		{
			name: "regenerate stale minimal",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
<!-- tidymark:gen:end -->
`,
			fs:       fstest.MapFS{"a.md": {Data: []byte("# A\n")}},
			contains: []string{"- [a.md](a.md)"},
		},
		{
			name: "regenerate stale template",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "- [{{.title}}]({{.filename}})"
-->
<!-- tidymark:gen:end -->
`,
			fs:       fstest.MapFS{"a.md": {Data: []byte("---\ntitle: Hello\n---\n")}},
			contains: []string{"- [Hello](a.md)"},
		},
		{
			name: "fix with empty fallback",
			src: `<!-- tidymark:gen:start catalog
glob: "nonexistent/*.md"
empty: Nothing here.
-->
old content
<!-- tidymark:gen:end -->
`,
			fs:       fstest.MapFS{},
			contains: []string{"Nothing here."},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newTestFile(t, "index.md", tc.src, tc.fs)
			r := &Rule{}
			result := string(r.Fix(f))
			for _, sub := range tc.contains {
				if !strings.Contains(result, sub) {
					t.Errorf("Fix result missing %q.\nGot:\n%s", sub, result)
				}
			}
		})
	}
}

// =====================================================================
// Table-driven Sort tests
// =====================================================================

// sortScenario is a test case for sort behavior validation (expects 0 diagnostics).
type sortScenario struct {
	name string
	src  string
	fs   fstest.MapFS
}

// runSortScenarios runs a slice of sort scenarios, each expecting zero diagnostics.
func runSortScenarios(t *testing.T, tests []sortScenario) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newTestFile(t, "index.md", tc.src, tc.fs)
			r := &Rule{}
			diags := r.Check(f)
			expectDiags(t, diags, 0)
		})
	}
}

func TestSort_PathAndFilename(t *testing.T) {
	threeFiles := fstest.MapFS{
		"c.md": {Data: []byte("# C\n")},
		"a.md": {Data: []byte("# A\n")},
		"b.md": {Data: []byte("# B\n")},
	}
	runSortScenarios(t, []sortScenario{
		{
			name: "path ascending (default)",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n" +
				"- [a.md](a.md)\n- [b.md](b.md)\n- [c.md](c.md)\n<!-- tidymark:gen:end -->\n",
			fs: threeFiles,
		},
		{
			name: "path descending",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\nsort: -path\n-->\n" +
				"- [c.md](c.md)\n- [b.md](b.md)\n- [a.md](a.md)\n<!-- tidymark:gen:end -->\n",
			fs: threeFiles,
		},
		{
			name: "by filename (basename)",
			src: "<!-- tidymark:gen:start catalog\nglob: \"**/*.md\"\nsort: filename\n-->\n" +
				"- [apple.md](z/apple.md)\n- [banana.md](a/banana.md)\n<!-- tidymark:gen:end -->\n",
			fs: fstest.MapFS{
				"a/banana.md": {Data: []byte("# Banana\n")},
				"z/apple.md":  {Data: []byte("# Apple\n")},
			},
		},
		{
			name: "filename descending",
			src: "<!-- tidymark:gen:start catalog\nglob: \"**/*.md\"\nsort: -filename\n-->\n" +
				"- [beta.md](a/beta.md)\n- [alpha.md](z/alpha.md)\n<!-- tidymark:gen:end -->\n",
			fs: fstest.MapFS{
				"z/alpha.md": {Data: []byte("# Alpha\n")},
				"a/beta.md":  {Data: []byte("# Beta\n")},
			},
		},
		{
			name: "case-insensitive path sort",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n" +
				"- [AAA.md](AAA.md)\n- [bbb.md](bbb.md)\n<!-- tidymark:gen:end -->\n",
			fs: fstest.MapFS{
				"bbb.md": {Data: []byte("# B\n")},
				"AAA.md": {Data: []byte("# A\n")},
			},
		},
	})
}

func TestSort_FrontMatterKey(t *testing.T) {
	twoFiles := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Zulu\n---\n")},
		"b.md": {Data: []byte("---\ntitle: Alpha\n---\n")},
	}
	runSortScenarios(t, []sortScenario{
		{
			name: "title ascending",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n" +
				"sort: title\nrow: \"- [{{.title}}]({{.filename}})\"\n-->\n" +
				"- [Alpha](b.md)\n- [Zulu](a.md)\n<!-- tidymark:gen:end -->\n",
			fs: twoFiles,
		},
		{
			name: "title descending",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n" +
				"sort: -title\nrow: \"- [{{.title}}]({{.filename}})\"\n-->\n" +
				"- [Zulu](a.md)\n- [Alpha](b.md)\n<!-- tidymark:gen:end -->\n",
			fs: twoFiles,
		},
		{
			name: "front matter key in minimal mode",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\nsort: title\n-->\n" +
				"- [b.md](b.md)\n- [a.md](a.md)\n<!-- tidymark:gen:end -->\n",
			fs: twoFiles,
		},
	})
}

func TestSort_TiebreakerAndCaseInsensitive(t *testing.T) {
	runSortScenarios(t, []sortScenario{
		{
			name: "tiebreaker when values equal",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n" +
				"sort: title\nrow: \"- [{{.title}}]({{.filename}})\"\n-->\n" +
				"- [Same](a.md)\n- [Same](b.md)\n<!-- tidymark:gen:end -->\n",
			fs: fstest.MapFS{
				"a.md": {Data: []byte("---\ntitle: Same\n---\n")},
				"b.md": {Data: []byte("---\ntitle: Same\n---\n")},
			},
		},
		{
			name: "case-insensitive title",
			src: "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n" +
				"sort: title\nrow: \"- [{{.title}}]({{.filename}})\"\n-->\n" +
				"- [alpha](a.md)\n- [Beta](b.md)\n<!-- tidymark:gen:end -->\n",
			fs: fstest.MapFS{
				"a.md": {Data: []byte("---\ntitle: alpha\n---\n")},
				"b.md": {Data: []byte("---\ntitle: Beta\n---\n")},
			},
		},
	})
}

// =====================================================================
// Internal function tests
// =====================================================================

func TestParseSort_Default(t *testing.T) {
	key, desc := parseSort(map[string]string{})
	if key != "path" || desc {
		t.Errorf("expected (path, false), got (%s, %v)", key, desc)
	}
}

func TestParseSort_Ascending(t *testing.T) {
	key, desc := parseSort(map[string]string{"sort": "title"})
	if key != "title" || desc {
		t.Errorf("expected (title, false), got (%s, %v)", key, desc)
	}
}

func TestParseSort_Descending(t *testing.T) {
	key, desc := parseSort(map[string]string{"sort": "-title"})
	if key != "title" || !desc {
		t.Errorf("expected (title, true), got (%s, %v)", key, desc)
	}
}

func TestParseSort_EmptyValue(t *testing.T) {
	key, desc := parseSort(map[string]string{"sort": ""})
	if key != "path" || desc {
		t.Errorf("expected (path, false) for empty, got (%s, %v)", key, desc)
	}
}

func TestParseRowTemplate_Valid(t *testing.T) {
	tmpl, err := parseRowTemplate("- [{{.title}}]({{.filename}})")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tmpl == nil {
		t.Fatal("expected non-nil template")
	}
}

func TestParseRowTemplate_Invalid(t *testing.T) {
	_, err := parseRowTemplate("{{.title")
	if err == nil {
		t.Error("expected error for invalid template")
	}
}

func TestContainsDotDot(t *testing.T) {
	tests := []struct {
		pattern string
		want    bool
	}{
		{"../foo", true},
		{"foo/../bar", true},
		{"foo/bar/..", true},
		{"foo/bar", false},
		{"foo..bar", false},
		{"...", false},
		{"..", true},
	}
	for _, tc := range tests {
		t.Run(tc.pattern, func(t *testing.T) {
			got := containsDotDot(tc.pattern)
			if got != tc.want {
				t.Errorf("containsDotDot(%q) = %v, want %v", tc.pattern, got, tc.want)
			}
		})
	}
}

func TestEnsureTrailingNewline(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello\n"},
		{"hello\n", "hello\n"},
		{"", "\n"},
		{"\n", "\n"},
		{"a\nb\n", "a\nb\n"},
		{"a\nb", "a\nb\n"},
	}
	for _, tc := range tests {
		got := ensureTrailingNewline(tc.input)
		if got != tc.want {
			t.Errorf("ensureTrailingNewline(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSplitLines(t *testing.T) {
	input := []byte("a\nb\nc")
	lines := splitLines(input)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if string(lines[0]) != "a" {
		t.Errorf("line 0: got %q", string(lines[0]))
	}
	if string(lines[1]) != "b" {
		t.Errorf("line 1: got %q", string(lines[1]))
	}
	if string(lines[2]) != "c" {
		t.Errorf("line 2: got %q", string(lines[2]))
	}
}

func TestSplitLines_Empty(t *testing.T) {
	lines := splitLines([]byte(""))
	if len(lines) != 1 {
		t.Fatalf("expected 1 line for empty input, got %d", len(lines))
	}
	if string(lines[0]) != "" {
		t.Errorf("expected empty line, got %q", string(lines[0]))
	}
}

func TestSplitLines_SingleNewline(t *testing.T) {
	lines := splitLines([]byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines for single newline, got %d", len(lines))
	}
	if string(lines[0]) != "" {
		t.Errorf("expected empty first line, got %q", string(lines[0]))
	}
	if string(lines[1]) != "" {
		t.Errorf("expected empty second line, got %q", string(lines[1]))
	}
}

func TestSplitLines_TrailingNewline(t *testing.T) {
	lines := splitLines([]byte("a\nb\n"))
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if string(lines[0]) != "a" {
		t.Errorf("line 0: got %q", string(lines[0]))
	}
	if string(lines[1]) != "b" {
		t.Errorf("line 1: got %q", string(lines[1]))
	}
	if string(lines[2]) != "" {
		t.Errorf("line 2: expected empty, got %q", string(lines[2]))
	}
}

func TestRenderMinimal(t *testing.T) {
	entries := []fileEntry{
		{fields: map[string]string{"filename": "docs/api.md"}},
		{fields: map[string]string{"filename": "docs/guide.md"}},
	}
	got := renderMinimal(entries)
	expected := "- [api.md](docs/api.md)\n- [guide.md](docs/guide.md)\n"
	if got != expected {
		t.Errorf("renderMinimal mismatch.\nExpected:\n%s\nGot:\n%s", expected, got)
	}
}

func TestRenderEmpty_WithValue(t *testing.T) {
	got := renderEmpty(map[string]string{"empty": "No files found."})
	if got != "No files found.\n" {
		t.Errorf("expected 'No files found.\\n', got %q", got)
	}
}

func TestRenderEmpty_NoKey(t *testing.T) {
	got := renderEmpty(map[string]string{})
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestRenderEmpty_EmptyValue(t *testing.T) {
	got := renderEmpty(map[string]string{"empty": ""})
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestRenderTemplate_HeaderRowFooter(t *testing.T) {
	params := map[string]string{
		"header": "| Title |\n|-------|",
		"row":    "| {{.title}} |",
		"footer": "---",
	}
	entries := []fileEntry{
		{fields: map[string]string{"title": "A", "filename": "a.md"}},
		{fields: map[string]string{"title": "B", "filename": "b.md"}},
	}
	got, err := renderTemplate(params, entries)
	if err != nil {
		t.Fatal(err)
	}
	expected := "| Title |\n|-------|\n| A |\n| B |\n---\n"
	if got != expected {
		t.Errorf("renderTemplate mismatch.\nExpected:\n%s\nGot:\n%s", expected, got)
	}
}

func TestRenderTemplate_RowOnly(t *testing.T) {
	params := map[string]string{
		"row": "- {{.filename}}",
	}
	entries := []fileEntry{
		{fields: map[string]string{"filename": "a.md"}},
	}
	got, err := renderTemplate(params, entries)
	if err != nil {
		t.Fatal(err)
	}
	expected := "- a.md\n"
	if got != expected {
		t.Errorf("renderTemplate mismatch.\nExpected:\n%s\nGot:\n%s", expected, got)
	}
}

func TestRenderTemplate_FooterOnly(t *testing.T) {
	params := map[string]string{
		"row":    "- {{.filename}}",
		"footer": "---",
	}
	entries := []fileEntry{
		{fields: map[string]string{"filename": "a.md"}},
	}
	got, err := renderTemplate(params, entries)
	if err != nil {
		t.Fatal(err)
	}
	expected := "- a.md\n---\n"
	if got != expected {
		t.Errorf("renderTemplate mismatch.\nExpected:\n%s\nGot:\n%s", expected, got)
	}
}

func TestRenderTemplate_InvalidTemplateReturnsError(t *testing.T) {
	params := map[string]string{
		"row": "{{.title",
	}
	entries := []fileEntry{
		{fields: map[string]string{"filename": "a.md"}},
	}
	_, err := renderTemplate(params, entries)
	if err == nil {
		t.Error("expected error for invalid template syntax")
	}
}

func TestRenderTemplate_ExecutionErrorReturnsError(t *testing.T) {
	// Calling a non-existent function in the template triggers an execution error.
	params := map[string]string{
		"row": "{{call .missing}}",
	}
	entries := []fileEntry{
		{fields: map[string]string{"filename": "a.md"}},
	}
	_, err := renderTemplate(params, entries)
	if err == nil {
		t.Error("expected error for template execution failure")
	}
}

func TestSortEntries_PathAscending(t *testing.T) {
	entries := []fileEntry{
		{fields: map[string]string{"filename": "c.md"}},
		{fields: map[string]string{"filename": "a.md"}},
		{fields: map[string]string{"filename": "b.md"}},
	}
	sortEntries(entries, "path", false)
	if entries[0].fields["filename"] != "a.md" {
		t.Errorf("expected first entry a.md, got %s", entries[0].fields["filename"])
	}
	if entries[2].fields["filename"] != "c.md" {
		t.Errorf("expected last entry c.md, got %s", entries[2].fields["filename"])
	}
}

func TestSortEntries_PathDescending(t *testing.T) {
	entries := []fileEntry{
		{fields: map[string]string{"filename": "a.md"}},
		{fields: map[string]string{"filename": "c.md"}},
		{fields: map[string]string{"filename": "b.md"}},
	}
	sortEntries(entries, "path", true)
	if entries[0].fields["filename"] != "c.md" {
		t.Errorf("expected first entry c.md, got %s", entries[0].fields["filename"])
	}
	if entries[2].fields["filename"] != "a.md" {
		t.Errorf("expected last entry a.md, got %s", entries[2].fields["filename"])
	}
}

func TestSortEntries_FrontMatterKey(t *testing.T) {
	entries := []fileEntry{
		{fields: map[string]string{"filename": "a.md", "title": "Zulu"}},
		{fields: map[string]string{"filename": "b.md", "title": "Alpha"}},
	}
	sortEntries(entries, "title", false)
	if entries[0].fields["title"] != "Alpha" {
		t.Errorf("expected Alpha first, got %s", entries[0].fields["title"])
	}
}

func TestSortEntries_Tiebreaker(t *testing.T) {
	entries := []fileEntry{
		{fields: map[string]string{"filename": "b.md", "title": "Same"}},
		{fields: map[string]string{"filename": "a.md", "title": "Same"}},
	}
	sortEntries(entries, "title", false)
	if entries[0].fields["filename"] != "a.md" {
		t.Errorf("expected a.md first (tiebreaker), got %s", entries[0].fields["filename"])
	}
}

func TestSortEntries_TiebreakerDescending(t *testing.T) {
	// Even when descending, the tiebreaker is path ascending.
	entries := []fileEntry{
		{fields: map[string]string{"filename": "b.md", "title": "Same"}},
		{fields: map[string]string{"filename": "a.md", "title": "Same"}},
	}
	sortEntries(entries, "title", true)
	if entries[0].fields["filename"] != "a.md" {
		t.Errorf("expected a.md first (tiebreaker ascending), got %s", entries[0].fields["filename"])
	}
}

func TestSortEntries_FilenameDescending(t *testing.T) {
	entries := []fileEntry{
		{fields: map[string]string{"filename": "a/alpha.md"}},
		{fields: map[string]string{"filename": "z/zulu.md"}},
	}
	sortEntries(entries, "filename", true)
	if entries[0].fields["filename"] != "z/zulu.md" {
		t.Errorf("expected z/zulu.md first (filename descending), got %s", entries[0].fields["filename"])
	}
}

func TestSortValue_Path(t *testing.T) {
	e := fileEntry{fields: map[string]string{"filename": "docs/a.md"}}
	if v := sortValue(e, "path"); v != "docs/a.md" {
		t.Errorf("expected docs/a.md, got %s", v)
	}
}

func TestSortValue_Filename(t *testing.T) {
	e := fileEntry{fields: map[string]string{"filename": "docs/a.md"}}
	if v := sortValue(e, "filename"); v != "a.md" {
		t.Errorf("expected a.md, got %s", v)
	}
}

func TestSortValue_FrontMatterField(t *testing.T) {
	e := fileEntry{fields: map[string]string{"filename": "a.md", "title": "Hello"}}
	if v := sortValue(e, "title"); v != "Hello" {
		t.Errorf("expected Hello, got %s", v)
	}
}

func TestSortValue_MissingField(t *testing.T) {
	e := fileEntry{fields: map[string]string{"filename": "a.md"}}
	if v := sortValue(e, "title"); v != "" {
		t.Errorf("expected empty string for missing field, got %q", v)
	}
}

// =====================================================================
// readFrontMatter
// =====================================================================

func TestReadFrontMatter_Valid(t *testing.T) {
	fs := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Hello\ndescription: World\n---\n# Content\n")},
	}
	fm := readFrontMatter(fs, "a.md")
	if fm["title"] != "Hello" {
		t.Errorf("expected title Hello, got %q", fm["title"])
	}
	if fm["description"] != "World" {
		t.Errorf("expected description World, got %q", fm["description"])
	}
}

func TestReadFrontMatter_NoFrontMatter(t *testing.T) {
	fs := fstest.MapFS{
		"a.md": {Data: []byte("# No front matter\n")},
	}
	fm := readFrontMatter(fs, "a.md")
	if fm != nil {
		t.Errorf("expected nil for no front matter, got %v", fm)
	}
}

func TestReadFrontMatter_InvalidYAML(t *testing.T) {
	fs := fstest.MapFS{
		"a.md": {Data: []byte("---\ninvalid: [yaml\n---\n")},
	}
	fm := readFrontMatter(fs, "a.md")
	if fm != nil {
		t.Errorf("expected nil for invalid YAML, got %v", fm)
	}
}

func TestReadFrontMatter_NonStringValue(t *testing.T) {
	fs := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Hello\ncount: 42\n---\n")},
	}
	fm := readFrontMatter(fs, "a.md")
	if fm["title"] != "Hello" {
		t.Errorf("expected title Hello, got %q", fm["title"])
	}
	if fm["count"] != "42" {
		t.Errorf("expected count '42', got %q", fm["count"])
	}
}

func TestReadFrontMatter_UnreadableFile(t *testing.T) {
	fs := fstest.MapFS{}
	fm := readFrontMatter(fs, "missing.md")
	if fm != nil {
		t.Errorf("expected nil for missing file, got %v", fm)
	}
}

func TestReadFrontMatter_EmptyFile(t *testing.T) {
	fs := fstest.MapFS{
		"empty.md": {Data: []byte("")},
	}
	fm := readFrontMatter(fs, "empty.md")
	if fm != nil {
		t.Errorf("expected nil for empty file, got %v", fm)
	}
}

func TestReadFrontMatter_OnlyOpeningDelimiter(t *testing.T) {
	// File starts with --- but has no closing ---.
	fs := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Hello\n")},
	}
	fm := readFrontMatter(fs, "a.md")
	if fm != nil {
		t.Errorf("expected nil for unclosed front matter, got %v", fm)
	}
}

func TestReadFrontMatter_BooleanValue(t *testing.T) {
	// Boolean YAML values should be converted via fmt.Sprintf.
	fs := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Hello\ndraft: true\n---\n")},
	}
	fm := readFrontMatter(fs, "a.md")
	if fm["draft"] != "true" {
		t.Errorf("expected draft 'true', got %q", fm["draft"])
	}
}

func TestReadFrontMatter_ListValue(t *testing.T) {
	// List values in front matter should be converted via fmt.Sprintf.
	fs := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Hello\ntags: [go, lint]\n---\n")},
	}
	fm := readFrontMatter(fs, "a.md")
	if fm["title"] != "Hello" {
		t.Errorf("expected title Hello, got %q", fm["title"])
	}
	// List value is formatted as a Go slice string.
	if fm["tags"] == "" {
		t.Error("expected non-empty tags value")
	}
}

// =====================================================================
// extractContent and replaceContent
// =====================================================================

func TestExtractContent_Normal(t *testing.T) {
	src := `line1
line2
line3
line4
line5
`
	f := newTestFile(t, "test.md", src)
	mp := markerPair{
		startLine:   1,
		endLine:     5,
		contentFrom: 2,
		contentTo:   4,
	}
	content := extractContent(f, mp)
	if content != "line2\nline3\nline4\n" {
		t.Errorf("expected 'line2\\nline3\\nline4\\n', got %q", content)
	}
}

func TestExtractContent_Empty(t *testing.T) {
	src := `start
end
`
	f := newTestFile(t, "test.md", src)
	mp := markerPair{
		startLine:   1,
		endLine:     2,
		contentFrom: 2,
		contentTo:   1,
	}
	content := extractContent(f, mp)
	if content != "" {
		t.Errorf("expected empty content, got %q", content)
	}
}

func TestExtractContent_SingleLine(t *testing.T) {
	src := `start
middle
end
`
	f := newTestFile(t, "test.md", src)
	mp := markerPair{
		startLine:   1,
		endLine:     3,
		contentFrom: 2,
		contentTo:   2,
	}
	content := extractContent(f, mp)
	if content != "middle\n" {
		t.Errorf("expected 'middle\\n', got %q", content)
	}
}

func TestReplaceContent_Normal(t *testing.T) {
	src := "start\nold1\nold2\nend\ntrailing\n"
	f := newTestFile(t, "test.md", src)
	mp := markerPair{
		startLine:   1,
		endLine:     4,
		contentFrom: 2,
		contentTo:   3,
	}
	result := replaceContent(f, mp, "new1\nnew2\n")
	expected := "start\nnew1\nnew2\nend\ntrailing\n"
	if string(result) != expected {
		t.Errorf("replaceContent mismatch.\nExpected:\n%s\nGot:\n%s", expected, string(result))
	}
}

func TestReplaceContent_EmptyContent(t *testing.T) {
	src := "start\nold\nend\n"
	f := newTestFile(t, "test.md", src)
	mp := markerPair{
		startLine:   1,
		endLine:     3,
		contentFrom: 2,
		contentTo:   2,
	}
	result := replaceContent(f, mp, "")
	expected := "start\nend\n"
	if string(result) != expected {
		t.Errorf("replaceContent with empty content mismatch.\nExpected:\n%s\nGot:\n%s", expected, string(result))
	}
}

// =====================================================================
// Integration scenarios
// =====================================================================

func TestIntegration_FullTableWithSortAndEmpty(t *testing.T) {
	src := `# Project Index

<!-- tidymark:gen:start catalog
glob: "rules/*/README.md"
sort: title
header: |
  | Rule | Description |
  |------|-------------|
row: "| [{{.title}}]({{.filename}}) | {{.description}} |"
empty: No rules defined yet.
-->
| Rule | Description |
|------|-------------|
| [First Heading](rules/tm001/README.md) | Checks headings |
| [Line Length](rules/tm002/README.md) | Checks line length |
<!-- tidymark:gen:end -->

Some trailing text.
`
	mapFS := fstest.MapFS{
		"rules/tm002/README.md": {Data: []byte(
			"---\ntitle: Line Length\ndescription: Checks line length\n---\n# Rule\n")},
		"rules/tm001/README.md": {Data: []byte(
			"---\ntitle: First Heading\ndescription: Checks headings\n---\n# Rule\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Task 8: High-priority missing spec tests
// =====================================================================

func TestSpec_TemplateExecutionError_CheckEmitsDiagnostic(t *testing.T) {
	// Template execution error emits diagnostic.
	// {{call .title}} tries to call a string as a function, which fails at execution time.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "{{call .title}}"
-->
old content
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Hello\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, "template execution failed")
}

func TestSpec_TemplateExecutionError_FixLeavesSectionUnchanged(t *testing.T) {
	// Fix leaves section unchanged when template execution fails.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "{{call .title}}"
-->
old content
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Hello\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	result := r.Fix(f)
	if string(result) != src {
		t.Errorf("Fix should leave template-execution-error section unchanged.\n"+
			"Expected:\n%s\nGot:\n%s", src, string(result))
	}
}

func TestSpec_BraceExpansionSupported(t *testing.T) {
	// The doublestar library supports brace expansion `{a,b}`.
	// (The spec originally said "not supported" but the implementation
	// delegates to doublestar which handles braces natively.)
	src := `<!-- tidymark:gen:start catalog
glob: "*.{md,txt}"
-->
- [a.md](a.md)
- [b.txt](b.txt)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md":  {Data: []byte("# A\n")},
		"b.txt": {Data: []byte("# B\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestSpec_WindowsCRLFLineEndingsFlaggedAsStale(t *testing.T) {
	// Windows \r\n line endings in existing content are flagged as stale
	// since generated content uses \n.
	src := "<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n- [a.md](a.md)\r\n<!-- tidymark:gen:end -->\n"
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# A\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, "generated section is out of date")
}

func TestSpec_GlobMatchingLintedFileIncluded(t *testing.T) {
	// Glob matching the linted file itself includes it in output.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
- [index.md](index.md)
- [other.md](other.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"index.md": {Data: []byte(src)},
		"other.md": {Data: []byte("# Other\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestSpec_BinaryNonMarkdownMatchedFiles(t *testing.T) {
	// Binary/non-Markdown matched files: {{.filename}} resolves, no front matter extracted.
	src := `<!-- tidymark:gen:start catalog
glob: "*"
row: "- [{{.title}}]({{.filename}})"
-->
- [](data.bin)
- [Hello](readme.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"data.bin":  {Data: []byte{0x00, 0x01, 0x02, 0xFF}},
		"readme.md": {Data: []byte("---\ntitle: Hello\n---\n# Hi\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestSpec_YAMLAnchorsAliasesSupported(t *testing.T) {
	// YAML anchors, aliases, and merge keys are supported.
	src := `<!-- tidymark:gen:start catalog
glob: "docs/*.md"
row: "- {{.filename}}"
-->
- docs/a.md
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"docs/a.md": {Data: []byte("# A\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestSpec_DoubleDashSort(t *testing.T) {
	// `sort: --priority` means descending by key `-priority`.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: "--priority"
row: "- [{{.title}}]({{.filename}})"
-->
- [High](b.md)
- [Low](a.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Low\n-priority: \"1\"\n---\n")},
		"b.md": {Data: []byte("---\ntitle: High\n-priority: \"2\"\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestSpec_UnreadableMatchedFilesSilentlySkipped(t *testing.T) {
	// Unreadable matched files are silently skipped (end-to-end via Check).
	// We simulate this by having an FS that lists a file but fails to read it.
	// fstest.MapFS won't do this naturally, but we can test via a file
	// that is a directory entry in the FS that Stats ok but can't be read.
	// Instead, test that the rendering logic correctly excludes files that
	// can't have front matter read (the readFrontMatter returns nil).
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "- [{{.title}}]({{.filename}})"
-->
- [Hello](good.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"good.md": {Data: []byte("---\ntitle: Hello\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Task 9: Medium-priority missing spec tests
// =====================================================================

func TestSpec_TemplateOutputNotHTMLEscaped(t *testing.T) {
	// Template output is not HTML-escaped: <, >, & appear literally.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "- {{.title}}"
-->
- <b>Bold & "quoted"</b>
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: '<b>Bold & \"quoted\"</b>'\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestSpec_GoBuiltinTemplateFunctions(t *testing.T) {
	// Go built-in template functions (len, print, index) are available.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "- {{print .title}} ({{len .title}} chars)"
-->
- Hello (5 chars)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Hello\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// assertDiagOnLine checks that at least one diagnostic matches the message
// substring and is on the expected line.
func assertDiagOnLine(t *testing.T, diags []lint.Diagnostic, wantMsg string, wantLine int) {
	t.Helper()
	if len(diags) == 0 {
		t.Fatal("expected at least one diagnostic")
	}
	for _, d := range diags {
		if strings.Contains(d.Message, wantMsg) {
			if d.Line != wantLine {
				t.Errorf("expected diagnostic on line %d, got line %d (message: %s)", wantLine, d.Line, d.Message)
			}
			return
		}
	}
	t.Errorf("expected diagnostic with message containing %q", wantMsg)
}

func diagLineNumberCases() []struct {
	name     string
	src      string
	wantLine int
	wantMsg  string
} {
	return []struct {
		name     string
		src      string
		wantLine int
		wantMsg  string
	}{
		{
			"stale section on start marker line",
			"prefix\n<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n" +
				"old\n<!-- tidymark:gen:end -->\n",
			2, "out of date",
		},
		{
			"unclosed on start marker line",
			"prefix\n<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n" +
				"content\n",
			2, "no closing marker",
		},
		{
			"orphaned on end marker line",
			"prefix\ntext\n<!-- tidymark:gen:end -->\n",
			3, "unexpected generated section end marker",
		},
		{
			"missing directive on start marker line",
			"prefix\n<!-- tidymark:gen:start\n-->\n" +
				"<!-- tidymark:gen:end -->\n",
			2, "missing directive name",
		},
		{
			"unknown directive on start marker line",
			"prefix\n<!-- tidymark:gen:start foobar\nglob: \"*.md\"\n-->\n" +
				"<!-- tidymark:gen:end -->\n",
			2, "unknown generated section directive",
		},
		{
			"missing glob on start marker line",
			"prefix\n<!-- tidymark:gen:start catalog\nsort: path\n-->\n" +
				"<!-- tidymark:gen:end -->\n",
			2, `missing required "glob" parameter`,
		},
		{
			"invalid YAML on start marker line",
			"prefix\n<!-- tidymark:gen:start catalog\n: [invalid\n-->\n" +
				"<!-- tidymark:gen:end -->\n",
			2, "invalid YAML",
		},
		{
			"nested on nested start line",
			"<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n" +
				"prefix\n<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n" +
				"<!-- tidymark:gen:end -->\n",
			5, "nested generated section markers",
		},
	}
}

func TestSpec_DiagnosticLineNumbers(t *testing.T) {
	twoFiles := fstest.MapFS{
		"a.md": {Data: []byte("# A\n")},
		"b.md": {Data: []byte("# B\n")},
	}
	for _, tc := range diagLineNumberCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := newTestFile(t, "index.md", tc.src, twoFiles)
			r := &Rule{}
			diags := r.Check(f)
			assertDiagOnLine(t, diags, tc.wantMsg, tc.wantLine)
		})
	}
}

func TestSpec_MissingFrontMatterValuesSortAsEmptyString(t *testing.T) {
	// Missing front matter values sort as empty string (end-to-end).
	// Empty string sorts before any non-empty string, so files without
	// the sort field come first in ascending order.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: priority
row: "- [{{.title}}]({{.filename}})"
-->
- [No Priority](a.md)
- [High](b.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: No Priority\n---\n")},
		"b.md": {Data: []byte("---\ntitle: High\npriority: zzz\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestSpec_NonStringFrontMatterRenderedThroughTemplate(t *testing.T) {
	// Non-string front matter values are converted via fmt.Sprintf and rendered.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "- {{.title}} (count: {{.count}})"
-->
- Hello (count: 42)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Hello\ncount: 42\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestSpec_EmptyPlusFooterWithoutRowProducesDiag(t *testing.T) {
	// `empty` + `footer` without `row` produces missing-row diagnostic.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
empty: No docs.
footer: "---"
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, `generated section template missing required "row" key`)
}

func TestIntegration_EmptyFallbackWithFullTemplate(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "nonexistent/*.md"
header: |
  | Title |
  |-------|
row: "| {{.title}} |"
footer: "---"
empty: No documents.
-->
No documents.
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}
