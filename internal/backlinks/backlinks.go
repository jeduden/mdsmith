// Package backlinks resolves which workspace files link to a given
// target path (optionally scoped to an anchor). It owns the
// link/wikilink target-matching algorithm behind `mdsmith list
// backlinks`; cmd/mdsmith only parses flags and formats output.
package backlinks

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/globpath"
	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
)

// Record is one incoming link to the queried target.
//
// Kind is set to "wikilink" only when the record came from an
// Obsidian-style `[[Page]]` link; for ordinary Markdown links Kind
// stays empty and is omitted from JSON, so the historical
// {source, line, text, target} shape every existing consumer reads
// is preserved unchanged.
//
// Alias and Embed are only meaningful when Kind == "wikilink":
//   - Alias is the `|alias` half of `[[Page|alias]]`, empty when
//     the source had no alias.
//   - Embed is true when the source used `![[...]]` rather than
//     `[[...]]`.
//
// All three of Kind, Alias, and Embed are omitempty so a JSON
// document of standard-link records is byte-for-byte the same as
// before the wikilink fields were added.
type Record struct {
	Source string `json:"source"`
	Line   int    `json:"line"`
	Text   string `json:"text"`
	Target string `json:"target"`
	Kind   string `json:"kind,omitempty"`
	Alias  string `json:"alias,omitempty"`
	Embed  bool   `json:"embed,omitempty"`
}

// Collect walks every source file in files, extracts its outgoing
// links via linkgraph, and returns one record per link whose resolved
// workspace-relative target equals wantTarget. When wantAnchor is
// non-empty, the link's anchor must also match (after slugifying).
// includePatterns, when non-empty, filters source paths.
// ignorePatterns are config `ignore:` globs; matched sources are
// skipped so backlinks output respects the same scope as check/fix.
//
// stripFrontMatter must mirror the config's frontMatter setting so
// reported line numbers match MDS027 / the engine for the same file.
//
// Per-file read or parse failures do not abort the walk; instead they
// are collected and returned so the caller can surface them on stderr
// alongside whatever results were produced.
func Collect(
	files []string,
	rootDir, wantTarget, wantAnchor string,
	includePatterns, ignorePatterns []string,
	maxBytes int64,
	stripFrontMatter bool,
) ([]Record, []error) {
	wantAnchorSlug := ""
	if wantAnchor != "" {
		wantAnchorSlug = linkgraph.NormalizeAnchor(wantAnchor)
	}

	// Build the workspace wikilink index once per Collect call so a
	// corpus with N source files x M distinct wikilink targets does
	// one fs.WalkDir instead of N x M. Both this command and MDS027
	// route through linkgraph.WikilinkIndexFor so the build/cache
	// semantics live in one place.
	//
	// When rootDir is empty (e.g. unit-test calls with files in the
	// current directory and no resolved workspace root), wikilink
	// resolution is skipped entirely: extractBacklinksFromSource
	// leaves f.RootFS unset, and appendWikilinkBacklinks returns
	// early without producing any wikilink rows. Standard Markdown
	// links still surface.
	var index *linkgraph.WikilinkIndex
	if rootDir != "" {
		index = linkgraph.WikilinkIndexFor(nil, "", lint.OpenRootFS(rootDir))
	}

	var records []Record
	var errs []error
	for _, src := range files {
		srcRel := relPath(src, rootDir)
		// Self-links count: the contract is "every workspace file
		// that links to <target>", and a file that links to itself
		// satisfies that as literally as any other source.
		if !sourceMatches(srcRel, includePatterns) ||
			config.IsIgnored(ignorePatterns, srcRel) {
			continue
		}
		rs, err := extractBacklinksFromSource(
			src, srcRel, rootDir, wantTarget, wantAnchorSlug,
			maxBytes, stripFrontMatter, index,
		)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		records = append(records, rs...)
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Source != records[j].Source {
			return records[i].Source < records[j].Source
		}
		return records[i].Line < records[j].Line
	})
	return records, errs
}

// extractBacklinksFromSource reads one source file, parses it, and
// returns the backlink records (and any read/parse error) for links
// that resolve to wantTarget. wantAnchorSlug is the already-slugified
// anchor query, or "" to match any anchor.
func extractBacklinksFromSource(
	src, srcRel, rootDir, wantTarget, wantAnchorSlug string,
	maxBytes int64, stripFrontMatter bool,
	index *linkgraph.WikilinkIndex,
) ([]Record, error) {
	data, err := bytelimit.ReadFileLimited(src, maxBytes)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", srcRel, err)
	}
	// Reuse the lint pipeline's front-matter handling so line numbers
	// in records match what users see in editors. Mirror the config's
	// frontMatter setting so backlinks stays aligned with MDS027 when
	// stripping is turned off.
	//
	// NewFileFromSource only errors when NewFile errors, and NewFile
	// never errors — goldmark always returns an AST. The discard keeps
	// the linter happy without preserving an unreachable branch.
	f, _ := lint.NewFileFromSource(src, data, stripFrontMatter) //nolint:errcheck
	// Wikilink resolution needs the workspace root: ResolveWikiLink
	// walks the fs.FS to find candidates. Standard Markdown link
	// resolution operates on the source-relative path and never reads
	// f.RootFS, so this is a wikilink-only requirement.
	if rootDir != "" {
		f.SetRootDir(rootDir)
	}
	var out []Record
	for _, link := range linkgraph.ExtractLinks(f) {
		t := link.Target
		// Skip same-file anchor refs: backlinks only surfaces cross-
		// file edges. linkgraph guarantees a non-empty Path whenever
		// LocalAnchor is false.
		if t.LocalAnchor {
			continue
		}
		resolved := resolveLinkTarget(srcRel, t.Path)
		if resolved == "" || resolved != wantTarget {
			continue
		}
		if wantAnchorSlug != "" && linkgraph.NormalizeAnchor(t.Anchor) != wantAnchorSlug {
			continue
		}
		out = append(out, Record{
			Source: srcRel,
			// ExtractLinks returns body-relative lines (rules need
			// that for the engine's offset-adjustment); the CLI shows
			// file-relative line numbers, so add f.LineOffset back in.
			Line:   link.Line + f.LineOffset,
			Text:   link.Text(f.Source),
			Target: t.Raw,
		})
	}
	out = appendWikilinkBacklinks(out, f, srcRel, wantTarget, wantAnchorSlug, index)
	return out, nil
}

// appendWikilinkBacklinks scans f for Obsidian-style wikilinks whose
// resolved workspace-relative target matches wantTarget. Resolution
// uses the same shortest-path algorithm MDS027 applies, sandboxed to
// the file's root. Wikilinks are extracted unconditionally — the
// scan is cheap and the user querying backlinks already opted in by
// running the command.
//
// When a prebuilt index is supplied, every wikilink resolves via
// O(1) map lookups; otherwise resolution falls back to per-call
// fs.WalkDir, memoized per target within this file.
func appendWikilinkBacklinks(
	out []Record,
	f *lint.File,
	srcRel, wantTarget, wantAnchorSlug string,
	index *linkgraph.WikilinkIndex,
) []Record {
	wikilinks := linkgraph.ExtractWikiLinks(f)
	if len(wikilinks) == 0 {
		return out
	}
	root := f.RootFS
	if root == nil && index == nil {
		return out
	}
	type resolveResult struct {
		path string
		ok   bool
	}
	cache := map[string]resolveResult{}
	for _, wl := range wikilinks {
		r, cached := cache[wl.Target]
		if !cached {
			if index != nil {
				r.path, r.ok = index.Resolve(wl.Target)
			} else {
				r.path, r.ok = linkgraph.ResolveWikiLink(root, srcRel, wl.Target)
			}
			cache[wl.Target] = r
		}
		if !r.ok || r.path != wantTarget {
			continue
		}
		if wantAnchorSlug != "" && linkgraph.NormalizeAnchor(wl.Anchor) != wantAnchorSlug {
			continue
		}
		// Visible text mirrors what a reader would see: the alias
		// when one is present; otherwise the target-as-written
		// (target + "#anchor" if the source carried a fragment).
		// Using wl.Target alone would drop the anchor portion from
		// the JSON `text` field for `[[page#Section]]` and break
		// downstream round-tripping.
		text := wl.Alias
		if text == "" {
			text = wikilinkTargetString(wl)
		}
		out = append(out, Record{
			Source: srcRel,
			Line:   wl.Line + f.LineOffset,
			Text:   text,
			Target: wikilinkTargetString(wl),
			Kind:   "wikilink",
			Alias:  wl.Alias,
			Embed:  wl.Embed,
		})
	}
	return out
}

// wikilinkTargetString renders just the target+anchor of wl as it
// appeared in the source — the destination half a Markdown link's
// Target field would carry. Alias and embed prefix are dropped so
// the field reflects "where does this point", not "how does it look".
func wikilinkTargetString(wl linkgraph.WikiLink) string {
	if wl.Anchor == "" {
		return wl.Target
	}
	return wl.Target + "#" + wl.Anchor
}

// relPath returns p relative to rootDir using forward slashes. When
// rootDir is empty or filepath.Rel cannot relate the paths, p is
// returned as-is with separators normalized.
//
// This intentionally duplicates cmd/mdsmith's workspaceRelativePath
// rather than importing it: cmd/mdsmith depends on internal/backlinks,
// never the reverse, and this is a small, stable, independently
// tested pure function — cheaper to keep in both places than to widen
// this package's exports for a helper that four other CLI subcommands
// also use unrelated to backlinks.
func relPath(p, rootDir string) string {
	fallback := strings.TrimPrefix(filepath.ToSlash(p), "./")
	if rootDir == "" {
		return fallback
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return fallback
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return fallback
	}
	rel, err := filepath.Rel(absRoot, abs)
	if err != nil {
		return fallback
	}
	return filepath.ToSlash(rel)
}

// resolveLinkTarget joins srcRel's directory with the link's path and
// returns the workspace-relative result. Both inputs use forward
// slashes. Absolute paths (including Windows drive letters and UNC
// prefixes) and ones that escape the workspace root return "" so
// callers treat them as "outside the graph".
func resolveLinkTarget(srcRel, linkPath string) string {
	srcRel = strings.ReplaceAll(srcRel, `\`, `/`)
	linkPath = strings.ReplaceAll(linkPath, `\`, `/`)
	if isAbsOrDriveOrUNC(srcRel) || isAbsOrDriveOrUNC(linkPath) {
		return ""
	}
	dir := path.Dir(srcRel)
	cleaned := path.Clean(path.Join(dir, linkPath))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	return cleaned
}

// isAbsOrDriveOrUNC reports whether p is absolute under any of the
// schemes mdsmith targets: POSIX-style leading `/`, Windows drive
// letters like `C:/`, or UNC prefixes like `//host`. `path.IsAbs`
// alone misses the Windows forms because the path package is Unix-only.
func isAbsOrDriveOrUNC(p string) bool {
	if path.IsAbs(p) {
		return true
	}
	if len(p) >= 2 && p[1] == ':' {
		c := p[0]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			return true
		}
	}
	return strings.HasPrefix(p, "//")
}

// sourceMatches reports whether src should be considered, given the
// list of --include globs. An empty list lets every source through.
func sourceMatches(src string, includePatterns []string) bool {
	if len(includePatterns) == 0 {
		return true
	}
	for _, pat := range includePatterns {
		if globpath.Match(pat, src) {
			return true
		}
	}
	return false
}
