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

func TestDiag_UpToDateSectionZeroDiags(t *testing.T) {
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
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestDiag_StaleSectionOneDiag(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
- [a.md](a.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# A\n")},
		"b.md": {Data: []byte("# B\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, "generated section is out of date")
}

func TestDiag_UnclosedStartMarker(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
some content
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, "generated section has no closing marker")
}

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

func TestDiag_MissingDirectiveName(t *testing.T) {
	src := `<!-- tidymark:gen:start
glob: "*.md"
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, "generated section marker missing directive name")
}

func TestDiag_UnknownDirectiveName(t *testing.T) {
	src := `<!-- tidymark:gen:start foobar
glob: "*.md"
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, `unknown generated section directive "foobar"`)
}

func TestDiag_CaseSensitiveDirectiveName(t *testing.T) {
	// Case-sensitive: `Catalog` triggers "unknown directive".
	src := `<!-- tidymark:gen:start Catalog
glob: "*.md"
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, `unknown generated section directive "Catalog"`)
}

func TestDiag_CATALOGCaseSensitive(t *testing.T) {
	src := `<!-- tidymark:gen:start CATALOG
glob: "*.md"
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, `unknown generated section directive "CATALOG"`)
}

func TestDiag_MissingGlob(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
sort: path
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, `missing required "glob" parameter`)
}

func TestDiag_EmptyGlob(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: ""
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, `empty "glob" parameter`)
}

func TestDiag_AbsoluteGlobPath(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: /etc/*.md
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, "absolute glob path")
}

func TestDiag_GlobWithDotDot(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "../*.md"
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, `".." path traversal`)
}

func TestDiag_InvalidGlobPattern(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "[invalid"
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, "invalid glob pattern")
}

func TestDiag_InvalidYAMLBody(t *testing.T) {
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
	expectDiagMsg(t, diags, "generated section has invalid YAML")
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

func TestDiag_EmptyRow(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: ""
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, `generated section directive has empty "row" value`)
}

func TestDiag_EmptySort(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: ""
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, `empty "sort" value`)
}

func TestDiag_HeaderFooterWithoutRow(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "header without row",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
header: |
  | Title |
  |-------|
-->
<!-- tidymark:gen:end -->
`,
		},
		{
			name: "footer without row",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
footer: "---"
-->
<!-- tidymark:gen:end -->
`,
		},
		{
			name: "header and footer without row",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
header: "| Title |"
footer: "---"
-->
<!-- tidymark:gen:end -->
`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mapFS := fstest.MapFS{}
			f := newTestFile(t, "index.md", tc.src, mapFS)
			r := &Rule{}
			diags := r.Check(f)
			expectDiags(t, diags, 1)
			expectDiagMsg(t, diags, `generated section template missing required "row" key`)
		})
	}
}

func TestDiag_InvalidTemplateSyntax(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "{{.title"
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "generated section has invalid template") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected invalid template diagnostic, got %v", diags)
	}
}

func TestDiag_SortDashOnly(t *testing.T) {
	// `sort: "-"` (dash only) produces diagnostic.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: "-"
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, "invalid sort value")
}

func TestDiag_SortWithWhitespace(t *testing.T) {
	// Sort value with whitespace produces diagnostic.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: "foo bar"
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	expectDiagMsg(t, diags, "invalid sort value")
}

// =====================================================================
// Sort
// =====================================================================

func TestSort_PathAscendingDefault(t *testing.T) {
	// `sort: path` orders case-insensitively by relative file path (default).
	src := `<!-- tidymark:gen:start catalog
glob: "**/*.md"
-->
- [a.md](a.md)
- [b.md](docs/b.md)
- [z.md](z.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"z.md":      {Data: []byte("# Z\n")},
		"a.md":      {Data: []byte("# A\n")},
		"docs/b.md": {Data: []byte("# B\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestSort_FilenameOrdersByBasename(t *testing.T) {
	// `sort: filename` orders by basename.
	src := `<!-- tidymark:gen:start catalog
glob: "**/*.md"
sort: filename
-->
- [alpha.md](z/alpha.md)
- [beta.md](a/beta.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a/beta.md":  {Data: []byte("# Beta\n")},
		"z/alpha.md": {Data: []byte("# Alpha\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestSort_TitleAscending(t *testing.T) {
	// `sort: title` orders by front matter `title` field.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: title
row: "- [{{.title}}]({{.filename}})"
-->
- [Alpha](b.md)
- [Beta](a.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Beta\n---\n")},
		"b.md": {Data: []byte("---\ntitle: Alpha\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestSort_TitleDescending(t *testing.T) {
	// `sort: -title` orders descending.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: -title
row: "- [{{.title}}]({{.filename}})"
-->
- [Beta](a.md)
- [Alpha](b.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Beta\n---\n")},
		"b.md": {Data: []byte("---\ntitle: Alpha\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestSort_PathTiebreakerWhenValuesEqual(t *testing.T) {
	// Sort uses path as tiebreaker when values are equal.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: title
row: "- [{{.title}}]({{.filename}})"
-->
- [Same](a.md)
- [Same](b.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Same\n---\n")},
		"b.md": {Data: []byte("---\ntitle: Same\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestSort_CaseInsensitive(t *testing.T) {
	// Sort comparison is case-insensitive.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: title
row: "- [{{.title}}]({{.filename}})"
-->
- [alpha](a.md)
- [Beta](b.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: alpha\n---\n")},
		"b.md": {Data: []byte("---\ntitle: Beta\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestSort_FrontMatterKeyMinimalMode(t *testing.T) {
	// Sort with front matter key in minimal mode reads front matter.
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: title
-->
- [b.md](b.md)
- [a.md](a.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Zulu\n---\n")},
		"b.md": {Data: []byte("---\ntitle: Alpha\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestSort_PathDescending(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: -path
-->
- [z.md](z.md)
- [b.md](b.md)
- [a.md](a.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# A\n")},
		"b.md": {Data: []byte("# B\n")},
		"z.md": {Data: []byte("# Z\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

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

func TestFix_RegeneratesStaleSection(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
- [a.md](a.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("# A\n")},
		"b.md": {Data: []byte("# B\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	result := r.Fix(f)
	expected := `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
- [a.md](a.md)
- [b.md](b.md)
<!-- tidymark:gen:end -->
`
	if string(result) != expected {
		t.Errorf("Fix result mismatch.\nExpected:\n%s\nGot:\n%s", expected, string(result))
	}
}

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

func TestFix_WithEmptyFallback(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "nonexistent/*.md"
empty: Nothing here.
-->
old content
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	result := string(r.Fix(f))
	if !strings.Contains(result, "Nothing here.") {
		t.Errorf("Fix result missing empty fallback.\nGot:\n%s", result)
	}
}

func TestFix_WithTemplate(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "- [{{.title}}]({{.filename}})"
-->
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"a.md": {Data: []byte("---\ntitle: Hello\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	result := string(r.Fix(f))
	if !strings.Contains(result, "- [Hello](a.md)") {
		t.Errorf("Fix result missing template output.\nGot:\n%s", result)
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

func TestEdge_MarkersInsideHTMLBlock(t *testing.T) {
	// goldmark treats <div>...</div> as an HTML block.
	src := "<div>\n<!-- tidymark:gen:start catalog\nglob: \"*.md\"\n-->\n<!-- tidymark:gen:end -->\n</div>\n"
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
	// Should parse successfully (no structural errors).
	for _, d := range diags {
		if strings.Contains(d.Message, "no closing marker") ||
			strings.Contains(d.Message, "invalid YAML") {
			t.Errorf("unexpected error with whitespace terminator: %s", d.Message)
		}
	}
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
	for _, d := range diags {
		if strings.Contains(d.Message, "no closing marker") {
			t.Errorf("end marker with whitespace not recognized: %s", d.Message)
		}
	}
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
	// No "unknown directive" error means "catalog" was correctly parsed.
	for _, d := range diags {
		if strings.Contains(d.Message, "unknown") {
			t.Errorf("unexpected unknown directive error: %s", d.Message)
		}
	}
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
	for _, d := range diags {
		if strings.Contains(d.Message, "subdir") {
			t.Errorf("directory should be silently skipped: %s", d.Message)
		}
	}
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

func TestEdge_CaseInsensitivePathSort(t *testing.T) {
	src := `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
- [AAA.md](AAA.md)
- [bbb.md](bbb.md)
<!-- tidymark:gen:end -->
`
	mapFS := fstest.MapFS{
		"bbb.md": {Data: []byte("# B\n")},
		"AAA.md": {Data: []byte("# A\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Table-driven diagnostic scenarios
// =====================================================================

func TestCheck_DiagnosticScenarios(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		fs        fstest.MapFS
		wantCount int
		wantMsg   string
	}{
		{
			name: "valid pair no errors",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
- [a.md](a.md)
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{"a.md": {Data: []byte("# A\n")}},
			wantCount: 0,
		},
		{
			name: "unclosed start",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
content
`,
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "no closing marker",
		},
		{
			name: "orphaned end",
			src: `text
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "unexpected generated section end marker",
		},
		{
			name: "missing directive name",
			src: `<!-- tidymark:gen:start
-->
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "missing directive name",
		},
		{
			name: "unknown directive",
			src: `<!-- tidymark:gen:start unknown
glob: "*.md"
-->
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `unknown generated section directive "unknown"`,
		},
		{
			name: "CATALOG case sensitive",
			src: `<!-- tidymark:gen:start CATALOG
glob: "*.md"
-->
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `unknown generated section directive "CATALOG"`,
		},
		{
			name: "invalid YAML",
			src: `<!-- tidymark:gen:start catalog
: invalid : yaml ::: [
-->
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "invalid YAML",
		},
		{
			name: "empty glob",
			src: `<!-- tidymark:gen:start catalog
glob: ""
-->
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `empty "glob" parameter`,
		},
		{
			name: "absolute glob",
			src: `<!-- tidymark:gen:start catalog
glob: /etc/files/*.md
-->
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "absolute glob path",
		},
		{
			name: "glob with ..",
			src: `<!-- tidymark:gen:start catalog
glob: "../*.md"
-->
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `".." path traversal`,
		},
		{
			name: "empty row",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
row: ""
-->
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `empty "row" value`,
		},
		{
			name: "header without row",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
header: "| T |"
-->
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `missing required "row" key`,
		},
		{
			name: "footer without row",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
footer: "---"
-->
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `missing required "row" key`,
		},
		{
			name: "empty sort",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: ""
-->
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   `empty "sort" value`,
		},
		{
			name: "sort dash only",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: "-"
-->
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "invalid sort value",
		},
		{
			name: "sort with whitespace",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: "foo bar"
-->
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 1,
			wantMsg:   "invalid sort value",
		},
	}

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

// =====================================================================
// Table-driven content generation tests
// =====================================================================

func TestCheck_ContentGeneration(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		fs        fstest.MapFS
		wantCount int
	}{
		{
			name: "minimal mode up to date",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
- [hello.md](hello.md)
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{"hello.md": {Data: []byte("# Hello\n")}},
			wantCount: 0,
		},
		{
			name: "minimal mode stale",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
- [old.md](old.md)
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{"new.md": {Data: []byte("# New\n")}},
			wantCount: 1,
		},
		{
			name: "template mode with front matter up to date",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
row: "- [{{.title}}]({{.filename}})"
-->
- [My Title](a.md)
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{"a.md": {Data: []byte("---\ntitle: My Title\n---\n# A\n")}},
			wantCount: 0,
		},
		{
			name: "empty fallback up to date",
			src: `<!-- tidymark:gen:start catalog
glob: "nonexistent/*.md"
empty: No files found.
-->
No files found.
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 0,
		},
		{
			name: "no empty no matches empty content",
			src: `<!-- tidymark:gen:start catalog
glob: "nonexistent/*.md"
-->
<!-- tidymark:gen:end -->
`,
			fs:        fstest.MapFS{},
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newTestFile(t, "index.md", tc.src, tc.fs)
			r := &Rule{}
			diags := r.Check(f)
			expectDiags(t, diags, tc.wantCount)
		})
	}
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

func TestSort_Behavior(t *testing.T) {
	tests := []struct {
		name string
		src  string
		fs   fstest.MapFS
	}{
		{
			name: "sort path ascending (default)",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
- [a.md](a.md)
- [b.md](b.md)
- [c.md](c.md)
<!-- tidymark:gen:end -->
`,
			fs: fstest.MapFS{
				"c.md": {Data: []byte("# C\n")},
				"a.md": {Data: []byte("# A\n")},
				"b.md": {Data: []byte("# B\n")},
			},
		},
		{
			name: "sort path descending",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: -path
-->
- [c.md](c.md)
- [b.md](b.md)
- [a.md](a.md)
<!-- tidymark:gen:end -->
`,
			fs: fstest.MapFS{
				"c.md": {Data: []byte("# C\n")},
				"a.md": {Data: []byte("# A\n")},
				"b.md": {Data: []byte("# B\n")},
			},
		},
		{
			name: "sort by filename (basename)",
			src: `<!-- tidymark:gen:start catalog
glob: "**/*.md"
sort: filename
-->
- [apple.md](z/apple.md)
- [banana.md](a/banana.md)
<!-- tidymark:gen:end -->
`,
			fs: fstest.MapFS{
				"a/banana.md": {Data: []byte("# Banana\n")},
				"z/apple.md":  {Data: []byte("# Apple\n")},
			},
		},
		{
			name: "sort by title ascending",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: title
row: "- [{{.title}}]({{.filename}})"
-->
- [Alpha](b.md)
- [Zulu](a.md)
<!-- tidymark:gen:end -->
`,
			fs: fstest.MapFS{
				"a.md": {Data: []byte("---\ntitle: Zulu\n---\n")},
				"b.md": {Data: []byte("---\ntitle: Alpha\n---\n")},
			},
		},
		{
			name: "sort by title descending",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
sort: -title
row: "- [{{.title}}]({{.filename}})"
-->
- [Zulu](a.md)
- [Alpha](b.md)
<!-- tidymark:gen:end -->
`,
			fs: fstest.MapFS{
				"a.md": {Data: []byte("---\ntitle: Zulu\n---\n")},
				"b.md": {Data: []byte("---\ntitle: Alpha\n---\n")},
			},
		},
		{
			name: "case-insensitive path sort",
			src: `<!-- tidymark:gen:start catalog
glob: "*.md"
-->
- [AAA.md](AAA.md)
- [bbb.md](bbb.md)
<!-- tidymark:gen:end -->
`,
			fs: fstest.MapFS{
				"bbb.md": {Data: []byte("# B\n")},
				"AAA.md": {Data: []byte("# A\n")},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newTestFile(t, "index.md", tc.src, tc.fs)
			r := &Rule{}
			diags := r.Check(f)
			expectDiags(t, diags, 0)
		})
	}
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
		"rules/tm002/README.md": {Data: []byte("---\ntitle: Line Length\ndescription: Checks line length\n---\n# Rule\n")},
		"rules/tm001/README.md": {Data: []byte("---\ntitle: First Heading\ndescription: Checks headings\n---\n# Rule\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
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
