package release

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
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
		return err
	}
	if err := t.fs.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	_, err := t.syncDocsDir(srcDir, dstDir)
	return err
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
// Both inputs must already come from absPath, so neither has a
// trailing separator — appending one before the prefix test
// keeps `/tmp/foobar` from being treated as nested under
// `/tmp/foo`.
func isUnder(child, parent string) bool {
	return strings.HasPrefix(child, parent+string(filepath.Separator))
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
		return false, err
	}
	wrote := false
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		if e.IsDir() {
			childDst := filepath.Join(dst, e.Name())
			if err := t.fs.MkdirAll(childDst, 0o755); err != nil {
				return wrote, err
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
			return wrote, err
		}
		if ext == ".md" {
			data = escapeHugoShortcodes(data)
		}
		if err := t.fs.WriteFile(filepath.Join(dst, dstName), data, 0o644); err != nil {
			return wrote, err
		}
		wrote = true
	}
	if !wrote {
		if err := t.fs.RemoveAll(dst); err != nil {
			return wrote, err
		}
	}
	return wrote, nil
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
