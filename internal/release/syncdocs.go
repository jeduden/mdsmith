package release

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// hugoShortcodeAngle matches Hugo's angle-bracket shortcode form
// `{{< name args >}}` so the rewritten output uses Hugo's documented
// escape syntax `{{</* name args */>}}`, which renders the text
// verbatim instead of resolving a shortcode that does not exist.
// escapeHugoShortcodes skips matches whose body is already wrapped
// in the `/* … */` escape markers so a second pass over an
// already-escaped doc is a no-op.
var hugoShortcodeAngle = regexp.MustCompile(`\{\{<([^}]*)>\}\}`)

// hugoShortcodePercent matches the percent shortcode form
// `{{% name args %}}` for the same escape treatment via the
// `{{%/* ... */%}}` form.
var hugoShortcodePercent = regexp.MustCompile(`\{\{%([^}]*)%\}\}`)

// syncableExt is the allow-list of file extensions copied into the
// Hugo content tree. Files outside this set (Go embed.go helpers,
// build artifacts, etc.) live in docs/ as repo plumbing but have
// no place in the rendered site.
var syncableExt = map[string]struct{}{
	".md":   {},
	".svg":  {},
	".png":  {},
	".jpg":  {},
	".jpeg": {},
	".gif":  {},
	".webp": {},
}

// SyncDocs snapshots srcDir into dstDir for a Hugo build. The
// transforms reconcile two mismatches between mdsmith's docs/ tree
// and Hugo's expectations:
//
//   - proto.md files are schema templates — their front matter
//     holds CUE constraint strings, not real values — and would
//     blow up Hugo's metadata parser. Dropped.
//   - index.md is the docs/-tree convention for a directory
//     overview; Hugo's matching convention is _index.md (the
//     former turns the directory into a leaf bundle whose
//     siblings become resources rather than pages). Renamed.
//   - Non-markdown, non-image files (Go embeds, scripts) ride
//     along in docs/ for repo reasons but never render. Skipped.
//   - Literal {{< ... >}} and {{% ... %}} patterns inside
//     documentation about Hugo would otherwise resolve as
//     shortcodes during the build. Escaped to {{</* ... */>}}
//     and {{%/* ... */%}}.
//
// dstDir is removed before the copy, so SyncDocs is idempotent.
//
// Returns an error without touching the filesystem if srcDir and
// dstDir overlap (equal, dstDir under srcDir, or srcDir under
// dstDir). The destination is wiped before the copy, so an
// overlap would irrevocably delete the source tree before reading
// it. The check compares absolute, cleaned paths so a caller
// passing relative paths or trailing slashes still gets the
// guard.
func (t *Toolkit) SyncDocs(srcDir, dstDir string) error {
	if err := checkSyncDocsPaths(srcDir, dstDir); err != nil {
		return err
	}
	if _, err := t.fs.Stat(srcDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("source not found: %s", srcDir)
		}
		return fmt.Errorf("stat %s: %w", srcDir, err)
	}
	if err := t.fs.RemoveAll(dstDir); err != nil {
		return fmt.Errorf("wipe dst %s: %w", dstDir, err)
	}
	if err := t.fs.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("mkdir dst %s: %w", dstDir, err)
	}
	if _, err := t.syncDocsDir(srcDir, dstDir); err != nil {
		return fmt.Errorf("sync %s -> %s: %w", srcDir, dstDir, err)
	}
	return nil
}

// checkSyncDocsPaths refuses src/dst combinations that would
// wipe the source tree on the initial RemoveAll. Both paths are
// resolved to absolute, cleaned form (via the absPath seam, so
// tests can drive the unreachable Abs error branches) before the
// comparison, so relative inputs and trailing slashes still trip
// the guard.
func checkSyncDocsPaths(srcDir, dstDir string) error {
	src, err := absPath(srcDir)
	if err != nil {
		return fmt.Errorf("resolve src: %w", err)
	}
	dst, err := absPath(dstDir)
	if err != nil {
		return fmt.Errorf("resolve dst: %w", err)
	}
	if src == dst {
		return fmt.Errorf("src and dst point at the same path: %s", src)
	}
	if isUnder(dst, src) {
		return fmt.Errorf("dst %s is inside src %s", dst, src)
	}
	if isUnder(src, dst) {
		return fmt.Errorf("src %s is inside dst %s", src, dst)
	}
	return nil
}

// isUnder reports whether child sits strictly below parent in
// the cleaned absolute-path tree (so isUnder(p, p) is false).
// filepath.Rel handles filesystem roots correctly: a naive
// `HasPrefix(child, parent+sep)` test breaks when parent is "/"
// (or a Windows volume root) because parent+sep becomes "//",
// which would let SyncDocs RemoveAll a root destination. Rel
// also rejects sibling-prefix false positives (`/tmp/foobar`
// is not under `/tmp/foo`) because the relative path then
// starts with "..".
func isUnder(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." || rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// SyncDocs delegates to a default-OS Toolkit (see Stamp).
func SyncDocs(srcDir, dstDir string) error {
	return New().SyncDocs(srcDir, dstDir)
}

// syncDocsDir copies one directory level and returns true if any
// file ended up under dst. An empty dst is removed so the rendered
// tree doesn't expose hollow directories from upstream pruning
// (a docs/ subdir containing only a proto.md, say).
func (t *Toolkit) syncDocsDir(src, dst string) (bool, error) {
	entries, err := t.fs.ReadDir(src)
	if err != nil {
		return false, fmt.Errorf("read dir %s: %w", src, err)
	}
	wrote := false
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		if e.Type()&fs.ModeSymlink != 0 {
			// Skip symlinks (including symlinked dirs, whose
			// DirEntry.Type reports ModeSymlink and IsDir
			// false). Following one would let a link inside
			// docs/ copy arbitrary runner files into the
			// published site.
			continue
		}
		if e.IsDir() {
			childDst := filepath.Join(dst, e.Name())
			if err := t.fs.MkdirAll(childDst, 0o755); err != nil {
				return wrote, fmt.Errorf("mkdir %s: %w", childDst, err)
			}
			childWrote, err := t.syncDocsDir(srcPath, childDst)
			if err != nil {
				return wrote, err
			}
			if childWrote {
				wrote = true
			}
			continue
		}
		name := e.Name()
		if name == "proto.md" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if _, ok := syncableExt[ext]; !ok {
			continue
		}
		dstName := name
		if name == "index.md" {
			dstName = "_index.md"
		}
		data, err := t.fs.ReadFile(srcPath)
		if err != nil {
			return wrote, fmt.Errorf("read %s: %w", srcPath, err)
		}
		if ext == ".md" {
			data = transformMarkdown(data)
		}
		dstPath := filepath.Join(dst, dstName)
		if err := t.fs.WriteFile(dstPath, data, 0o644); err != nil {
			return wrote, fmt.Errorf("write %s: %w", dstPath, err)
		}
		wrote = true
	}
	if !wrote {
		if err := t.fs.RemoveAll(dst); err != nil {
			return wrote, fmt.Errorf("prune empty dst %s: %w", dst, err)
		}
	}
	return wrote, nil
}

// transformMarkdown applies the docs/-tree → Hugo content
// reconciliations to one markdown file: lift the body H1
// into a front-matter title, then escape literal shortcode
// patterns so documentation about Hugo renders verbatim.
func transformMarkdown(b []byte) []byte {
	return escapeHugoShortcodes(liftDocTitle(b))
}

// headingText flattens a heading node's inline children to
// plain text. Code spans contribute their code without the
// backticks, and emphasis/links/strong recurse, so a heading
// like "# The `mdsmith` CLI" yields a clean front-matter
// title for <title>/breadcrumb/sidebar use.
func headingText(n ast.Node, src []byte) string {
	var sb strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if tn, ok := c.(*ast.Text); ok {
			sb.Write(tn.Segment.Value(src))
			continue
		}
		// Emphasis/strong/link/code-span recurse; their leaf
		// Text nodes carry the visible characters (a code span's
		// child Text is the code without backticks).
		sb.WriteString(headingText(c, src))
	}
	return sb.String()
}

// liftDocTitle reconciles the two title conventions at the
// sync boundary. mdsmith docs carry the page title as the
// first body H1 (the first-line-heading rule enforces it)
// and keep only `summary` in front matter; Hugo themes
// expect the title in front-matter `title:` with no body
// H1. Left unreconciled, every synced page renders two H1s
// (the template's {{ .Title }} plus the body's) and pages
// without an explicit `title:` show Hugo's filename guess
// in the breadcrumb, <title>, and sidebar.
//
// The body is parsed with goldmark so the "first block is a
// level-1 heading" test is a real CommonMark decision: a '#'
// inside a fenced code block, an HTML comment ahead of the
// heading, or a setext underline are all classified
// correctly instead of by line-prefix guessing. When the
// first block is an H1 its text is promoted to a
// front-matter `title:` and the heading's source span (plus
// one trailing blank line) is spliced out of the body. An
// existing `title:` is left untouched; a file with no
// front-matter block gets one synthesized (research/ scratch
// notes carry a body H1 but no front matter, and would
// otherwise render an empty template `{{ .Title }}` H1 above
// the body's). Anything whose first block is not an H1 is
// returned unchanged.
func liftDocTitle(b []byte) []byte {
	s := string(b)
	fmBlock, body, hasFM := "", s, false
	if strings.HasPrefix(s, "---\n") {
		after := s[len("---\n"):]
		idx := strings.Index(after, "\n---\n")
		if idx < 0 {
			return b
		}
		fmBlock, body, hasFM = after[:idx], after[idx+len("\n---\n"):], true
	}

	src := []byte(body)
	h, ok := goldmark.New().Parser().Parse(text.NewReader(src)).FirstChild().(*ast.Heading)
	if !ok || h.Level != 1 || h.Lines().Len() == 0 {
		return b
	}
	title := strings.TrimSpace(headingText(h, src))
	if title == "" {
		return b
	}

	delStart, delEnd := headingSpan(src, h)
	newBody := string(src[:delStart]) + string(src[delEnd:])
	return []byte("---\n" + mergeFMTitle(fmBlock, hasFM, title) + "\n---\n" + newBody)
}

// headingSpan returns the byte range in src covering the
// heading's physical line(s) — including a setext underline
// and one trailing blank line — so the splice removes the
// whole heading, not just goldmark's text segment.
func headingSpan(src []byte, h *ast.Heading) (int, int) {
	segStart := h.Lines().At(0).Start
	segStop := h.Lines().At(h.Lines().Len() - 1).Stop
	start := bytes.LastIndexByte(src[:segStart], '\n') + 1
	end := len(src)
	if nl := bytes.IndexByte(src[segStop:], '\n'); nl >= 0 {
		end = segStop + nl + 1
	}
	// goldmark emits the same Heading node for ATX and setext
	// H1s. ATX permits up to 3 leading spaces before the '#'
	// (CommonMark), so trim those before testing for the
	// marker — otherwise an indented ATX heading is misread
	// as setext and the following content line is deleted. A
	// setext H1 has no '#' and an "====" underline on the
	// next line that must also be removed.
	i := start
	for i < end && i-start < 3 && src[i] == ' ' {
		i++
	}
	if (i >= end || src[i] != '#') && end < len(src) {
		if nl := bytes.IndexByte(src[end:], '\n'); nl >= 0 {
			end += nl + 1
		} else {
			end = len(src)
		}
	}
	// Drop one blank line left behind so the body does not
	// start with whitespace.
	if rest := src[end:]; len(rest) > 0 {
		if nl := bytes.IndexByte(rest, '\n'); nl >= 0 && strings.TrimSpace(string(rest[:nl])) == "" {
			end += nl + 1
		}
	}
	return start, end
}

// mergeFMTitle returns the front-matter inner text with a
// `title:` ensured. An existing title is kept untouched;
// otherwise the promoted title is appended, or becomes the
// sole key when the source carried no front matter.
func mergeFMTitle(fmBlock string, hasFM bool, title string) string {
	if hasFM {
		for _, ln := range strings.Split(fmBlock, "\n") {
			if strings.HasPrefix(ln, "title:") {
				return fmBlock
			}
		}
	}
	esc := strings.ReplaceAll(title, `\`, `\\`)
	esc = strings.ReplaceAll(esc, `"`, `\"`)
	titleLine := "title: \"" + esc + "\""
	if strings.TrimSpace(fmBlock) == "" {
		return titleLine
	}
	return strings.TrimRight(fmBlock, "\n") + "\n" + titleLine
}

// escapeHugoShortcodes rewrites every shortcode-shaped pattern in
// b to its Hugo escape form. Already-escaped patterns
// (`{{</* … */>}}`, `{{%/* … */%}}`) are left untouched so a
// second pass over the same content does not double-escape into
// `{{</*/* … *//*>}}`.
func escapeHugoShortcodes(b []byte) []byte {
	b = hugoShortcodeAngle.ReplaceAllFunc(b, func(m []byte) []byte {
		if bytes.HasPrefix(m, []byte("{{</*")) && bytes.HasSuffix(m, []byte("*/>}}")) {
			return m
		}
		sub := hugoShortcodeAngle.FindSubmatch(m)
		return append(append([]byte("{{</*"), sub[1]...), []byte("*/>}}")...)
	})
	b = hugoShortcodePercent.ReplaceAllFunc(b, func(m []byte) []byte {
		if bytes.HasPrefix(m, []byte("{{%/*")) && bytes.HasSuffix(m, []byte("*/%}}")) {
			return m
		}
		sub := hugoShortcodePercent.FindSubmatch(m)
		return append(append([]byte("{{%/*"), sub[1]...), []byte("*/%}}")...)
	})
	return b
}
