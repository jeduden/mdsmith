package linkgraph

import (
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/lint"
)

// WikiLink is one parsed Obsidian-style wikilink occurrence.
//
// Target is the destination filename or stem (without alias or
// anchor). Anchor and Alias are the optional fragment and display
// label. Embed reports whether the source used `![[...]]` rather
// than `[[...]]`.
//
// Line and Column are body-relative — same convention as Link.
type WikiLink struct {
	Target string
	Anchor string
	Alias  string
	Embed  bool
	Line   int
	Column int
}

// wikilinkRE matches Obsidian-style wikilinks.
// Group 1: leading "!" (embed marker)
// Group 2: target stem or filename (no anchor or alias)
// Group 3: optional anchor (text after "#")
// Group 4: optional alias (text after "|")
var wikilinkRE = regexp.MustCompile(
	`(!?)\[\[([^\[\]\n|#]+)(?:#([^\[\]\n|]+))?(?:\|([^\[\]\n]+))?\]\]`,
)

// ExtractWikiLinks scans f.Source for Obsidian-style wikilinks
// (`[[Page]]`, `[[Page#anchor]]`, `[[Page|alias]]`, `![[file.png]]`).
// Matches inside fenced/indented code blocks, code spans, and
// `<?...?>` processing-instruction blocks are skipped — the same
// guards MDS054 applies to its bracket scanner.
//
// Lines are body-relative (post front-matter strip).
func ExtractWikiLinks(f *lint.File) []WikiLink {
	if f == nil || len(f.Source) == 0 {
		return nil
	}
	codeLines := lint.CollectCodeBlockLines(f)
	piLines := lint.CollectPIBlockLines(f)
	codeSpans := collectCodeSpanRanges(f)

	source := f.Source
	var out []WikiLink
	for _, m := range wikilinkRE.FindAllSubmatchIndex(source, -1) {
		start := m[0]
		line := f.LineOfOffset(start)
		if codeLines[line] || piLines[line] {
			continue
		}
		if inCodeSpan(codeSpans, start) {
			continue
		}
		embed := m[3] > m[2]
		bracketStart := m[0]
		if embed {
			bracketStart++
		}
		col := f.ColumnOfOffset(bracketStart)
		wl := WikiLink{
			Target: strings.TrimSpace(string(source[m[4]:m[5]])),
			Embed:  embed,
			Line:   line,
			Column: col,
		}
		if m[6] >= 0 {
			wl.Anchor = strings.TrimSpace(string(source[m[6]:m[7]]))
		}
		if m[8] >= 0 {
			wl.Alias = strings.TrimSpace(string(source[m[8]:m[9]]))
		}
		out = append(out, wl)
	}
	return out
}

// byteRange is a half-open [start, end) byte range.
type byteRange struct{ start, end int }

func collectCodeSpanRanges(f *lint.File) []byteRange {
	var out []byteRange
	source := f.Source
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, ok := n.(*ast.CodeSpan); !ok {
			return ast.WalkContinue, nil
		}
		first, last := codeSpanTextBounds(n)
		if first < 0 {
			return ast.WalkContinue, nil
		}
		start := first
		for start > 0 && source[start-1] == '`' {
			start--
		}
		end := last
		for end < len(source) && source[end] == '`' {
			end++
		}
		out = append(out, byteRange{start, end})
		return ast.WalkContinue, nil
	})
	return out
}

func codeSpanTextBounds(n ast.Node) (first, last int) {
	first = -1
	last = -1
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok := c.(*ast.Text)
		if !ok {
			continue
		}
		if first < 0 {
			first = t.Segment.Start
		}
		last = t.Segment.Stop
	}
	return first, last
}

func inCodeSpan(spans []byteRange, offset int) bool {
	for _, r := range spans {
		if offset >= r.start && offset < r.end {
			return true
		}
	}
	return false
}

// ResolveWikiLink resolves an Obsidian-style wikilink target against
// root, returning the workspace-relative path of the resolved file.
//
// Resolution rules:
//
//   - When target has no extension or ends in `.md`/`.markdown`, the
//     search matches files whose stem (filename minus extension)
//     equals target, case-insensitive. The target itself is also
//     considered a stem when it lacks an extension.
//   - When target has any other extension (an embed like `image.png`),
//     the search matches files by exact filename, case-insensitive.
//   - Ties are broken by the shortest path (fewest separators); then
//     alphabetically. Two matches at the same depth never both win.
//   - The walk is sandboxed to root: paths that would escape via `..`
//     are rejected before the walk starts.
//
// from is the workspace-relative path of the source file. It is
// reserved for future per-directory resolution preference; today it
// only blocks empty targets the same way `ParseTarget` does for
// regular links.
func ResolveWikiLink(root fs.FS, from, target string) (string, bool) {
	if root == nil || target == "" {
		return "", false
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false
	}
	if strings.Contains(target, "..") {
		return "", false
	}
	cleaned := path.Clean(target)
	if cleaned == "." || strings.HasPrefix(cleaned, "/") {
		return "", false
	}

	wantName, wantStem, stemMode := wikilinkSearchKey(target)
	_ = from

	var matches []string
	walkErr := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		base := path.Base(p)
		if stemMode {
			stem := strings.TrimSuffix(base, path.Ext(base))
			if strings.EqualFold(stem, wantStem) && isMarkdownName(base) {
				matches = append(matches, p)
			}
			return nil
		}
		if strings.EqualFold(base, wantName) {
			matches = append(matches, p)
		}
		return nil
	})
	if walkErr != nil || len(matches) == 0 {
		return "", false
	}
	sort.Slice(matches, func(i, j int) bool {
		di := strings.Count(matches[i], "/")
		dj := strings.Count(matches[j], "/")
		if di != dj {
			return di < dj
		}
		return matches[i] < matches[j]
	})
	return matches[0], true
}

// wikilinkSearchKey splits target into the lookup parameters
// ResolveWikiLink walks the FS with.
//
// stemMode true means "match by filename stem against .md/.markdown
// files only" — the bare-page case (`[[Notes]]` or
// `[[Notes.md]]`). stemMode false means "match by exact filename"
// — the typed-extension case (`![[diagram.png]]`).
func wikilinkSearchKey(target string) (wantName, wantStem string, stemMode bool) {
	target = filepath.ToSlash(target)
	base := path.Base(target)
	ext := strings.ToLower(path.Ext(base))
	switch ext {
	case "", ".md", ".markdown":
		stem := strings.TrimSuffix(base, path.Ext(base))
		return "", stem, true
	default:
		return base, "", false
	}
}

func isMarkdownName(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	return ext == ".md" || ext == ".markdown"
}
