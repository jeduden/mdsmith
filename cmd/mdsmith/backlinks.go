package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/globpath"
	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
)

// backlinkRecord is one incoming link to the queried target.
//
// Kind distinguishes a standard Markdown link ("link") from an
// Obsidian-style wikilink ("wikilink"). JSON consumers can filter
// on this field; the text formatter renders both shapes inline.
type backlinkRecord struct {
	Source string `json:"source"`
	Line   int    `json:"line"`
	Text   string `json:"text"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
}

// backlinksOptions bundles the parsed CLI flags for `backlinks`.
type backlinksOptions struct {
	configPath      string
	format          string
	maxInputSize    string
	includePatterns []string
	limit           int
	walk            walkCLI
}

// parseBacklinksFlags parses the flags for `mdsmith list backlinks` and
// returns the options plus the remaining positional arguments.
func parseBacklinksFlags(args []string) (backlinksOptions, []string, error) {
	fs := flag.NewFlagSet("backlinks", flag.ContinueOnError)
	var (
		opts                        backlinksOptions
		noGitignore, followSymlinks bool
	)
	fs.StringVarP(&opts.configPath, "config", "c", "", "Override config file path")
	fs.StringVarP(&opts.format, "format", "f", "text", "Output format: text, json")
	fs.StringArrayVar(&opts.includePatterns, "include", nil,
		"Restrict sources to paths matching glob (repeatable)")
	fs.IntVar(&opts.limit, "limit", 0, "Cap output at N rows (0 = no cap)")
	fs.BoolVar(&noGitignore, "no-gitignore", false, "Disable .gitignore filtering when walking directories")
	fs.BoolVar(&followSymlinks, "follow-symlinks", false,
		"Follow symlinks; omitted defers to follow-symlinks config (default skip); "+
			"=false forces skip over any config opt-in")
	fs.StringVar(&opts.maxInputSize, "max-input-size", "",
		"Maximum file size to process (e.g. 2MB, 500KB, 0=unlimited)")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: mdsmith list backlinks [flags] <target>\n\n"+
			"List every workspace link that points at <target>. Optionally\n"+
			"scope by anchor with `path#anchor`.\n\n"+
			"Exit codes: 0 found, 1 none, 2 error\n\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return opts, nil, err
	}
	opts.walk = walkCLI{
		noGitignore:    noGitignore,
		followSymlinks: followSymlinksOverride(fs, followSymlinks),
	}
	return opts, fs.Args(), nil
}

// runBacklinks implements the "backlinks" subcommand: list every
// workspace link that points at <target> (optionally scoped to an
// anchor on the target).
func runBacklinks(args []string) int {
	opts, posArgs, err := parseBacklinksFlags(args)
	if err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: list backlinks"); code >= 0 {
			return code
		}
	}

	targetPath, targetAnchor, code := validateBacklinksArgs(opts, posArgs)
	if code >= 0 {
		return code
	}

	cfg, cfgPath, _, files, code := discoverFiles(opts.configPath, false, opts.walk)
	if code >= 0 {
		// 0 means "config + discovery returned no files"; for backlinks
		// that is just "no results", not an error.
		if code == 0 {
			return emitBacklinks(os.Stdout, nil, opts.format, opts.limit)
		}
		return code
	}

	maxBytes, err := resolveMaxInputBytes(cfg, opts.maxInputSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	rootDir := rootDirFromConfig(cfgPath)
	wantTarget := normalizeWorkspacePath(targetPath)

	records, errs := collectBacklinks(
		files, rootDir, wantTarget, targetAnchor,
		opts.includePatterns, cfg.Ignore, maxBytes, frontMatterEnabled(cfg),
	)
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", e)
	}

	code = emitBacklinks(os.Stdout, records, opts.format, opts.limit)
	// Exit-code precedence: records found wins (0), then runtime
	// errors (2), then clean empty result (1). This matches `check`,
	// where diagnostics outrank per-file read errors but a fully
	// failed run reports the error.
	if code == 1 && len(errs) > 0 {
		return 2
	}
	return code
}

// validateBacklinksArgs checks the positional arg shape and the
// numeric/glob flag invariants for backlinks. It returns the parsed
// target (path + anchor) plus code = -1 on success, or a non-negative
// exit code with a message already written to stderr.
func validateBacklinksArgs(opts backlinksOptions, posArgs []string) (targetPath, targetAnchor string, code int) {
	if len(posArgs) == 0 {
		fmt.Fprint(os.Stderr, "mdsmith: list backlinks requires a target file argument\n")
		return "", "", 2
	}
	if len(posArgs) > 1 {
		fmt.Fprintf(os.Stderr, "mdsmith: list backlinks takes one target argument, got %d\n", len(posArgs))
		return "", "", 2
	}
	// Route the target through the same parser the link walker uses
	// (linkgraph.ParseTarget) so the CLI accepts exactly the shapes
	// that can ever appear as a link destination. This rejects
	// malformed percent escapes, query-only inputs, schemed URLs, and
	// other forms with no corresponding link target — all of which
	// would otherwise pass and produce a silent empty result.
	parsed, ok := linkgraph.ParseTarget(posArgs[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "mdsmith: invalid target %q\n", posArgs[0])
		return "", "", 2
	}
	if parsed.LocalAnchor {
		fmt.Fprint(os.Stderr, "mdsmith: list backlinks target must include a file path\n")
		return "", "", 2
	}
	targetPath, targetAnchor = parsed.Path, parsed.Anchor
	// `<target>` is workspace-relative by contract. An absolute path
	// or a parent-traversal entry normalises to something outside the
	// workspace and would silently match nothing — which a caller
	// cannot distinguish from a genuine empty result. Reject upfront
	// so the failure is loud. ParseTarget already percent-decoded the
	// path, so `%2Fetc...` and `%2e%2e/...` are checked here in their
	// decoded form.
	if !isWorkspaceRelativeTarget(targetPath) {
		fmt.Fprintf(os.Stderr, "mdsmith: target %q must be workspace-relative\n", targetPath)
		return "", "", 2
	}
	if err := validateIncludePatterns(opts.includePatterns); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return "", "", 2
	}
	if opts.limit < 0 {
		fmt.Fprintf(os.Stderr, "mdsmith: --limit must be >= 0 (got %d)\n", opts.limit)
		return "", "", 2
	}
	return targetPath, targetAnchor, -1
}

// validateIncludePatterns rejects any --include glob that doublestar
// would silently reject (returning false from every Match call). A
// typo like `--include "["` would otherwise filter out every source
// and surface as "no backlinks found" instead of a clear error.
func validateIncludePatterns(patterns []string) error {
	for _, p := range patterns {
		if !doublestar.ValidatePattern(p) {
			return fmt.Errorf("invalid --include glob %q", p)
		}
	}
	return nil
}

// normalizeWorkspacePath returns the cleaned workspace-relative form
// of target. validateBacklinksArgs already rejects absolute paths
// and `..` traversals and routes the input through
// linkgraph.ParseTarget (which percent-decodes), so this helper only
// has to handle a relative, decoded path: strip a leading `./`,
// normalize separators, and clean the result.
func normalizeWorkspacePath(target string) string {
	t := filepath.ToSlash(target)
	t = strings.TrimPrefix(t, "./")
	return path.Clean(t)
}

// collectBacklinks walks every source file in files, extracts its
// outgoing links via linkgraph, and returns one record per link whose
// resolved workspace-relative target equals wantTarget. When anchor
// is non-empty, the link's anchor must also match (after slugifying).
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
func collectBacklinks(
	files []string,
	rootDir, wantTarget, wantAnchor string,
	includePatterns, ignorePatterns []string,
	maxBytes int64,
	stripFrontMatter bool,
) ([]backlinkRecord, []error) {
	wantAnchorSlug := ""
	if wantAnchor != "" {
		wantAnchorSlug = linkgraph.NormalizeAnchor(wantAnchor)
	}

	var records []backlinkRecord
	var errs []error
	for _, src := range files {
		srcRel := workspaceRelativePath(src, rootDir)
		// Self-links count: the contract is "every workspace file
		// that links to <target>", and a file that links to itself
		// satisfies that as literally as any other source.
		if !sourceMatches(srcRel, includePatterns) ||
			config.IsIgnored(ignorePatterns, srcRel) {
			continue
		}
		rs, err := extractBacklinksFromSource(
			src, srcRel, rootDir, wantTarget, wantAnchorSlug,
			maxBytes, stripFrontMatter,
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
) ([]backlinkRecord, error) {
	data, err := lint.ReadFileLimited(src, maxBytes)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", srcRel, err)
	}
	// Reuse the lint pipeline's front-matter handling so line numbers
	// in records match what users see in editors. Mirror the config's
	// frontMatter setting so backlinks stays aligned with MDS027 when
	// stripping is turned off.
	f, err := lint.NewFileFromSource(src, data, stripFrontMatter)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", srcRel, err)
	}
	// Wikilink resolution needs the workspace root: ResolveWikiLink
	// walks the fs.FS to find candidates. Standard Markdown link
	// resolution operates on the source-relative path and never reads
	// f.RootFS, so this is a wikilink-only requirement.
	if rootDir != "" {
		f.RootDir = rootDir
		f.RootFS = os.DirFS(rootDir)
	}
	var out []backlinkRecord
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
		out = append(out, backlinkRecord{
			Source: srcRel,
			// ExtractLinks returns body-relative lines (rules need
			// that for the engine's offset-adjustment); the CLI shows
			// file-relative line numbers, so add f.LineOffset back in.
			Line:   link.Line + f.LineOffset,
			Text:   link.Text,
			Target: t.Raw,
			Kind:   "link",
		})
	}
	out = appendWikilinkBacklinks(out, f, srcRel, wantTarget, wantAnchorSlug)
	return out, nil
}

// appendWikilinkBacklinks scans f for Obsidian-style wikilinks whose
// resolved workspace-relative target matches wantTarget. Resolution
// uses the same shortest-path algorithm MDS027 applies, sandboxed to
// the file's root. Wikilinks are extracted unconditionally — the
// scan is cheap and the user querying backlinks already opted in by
// running the command.
func appendWikilinkBacklinks(
	out []backlinkRecord,
	f *lint.File,
	srcRel, wantTarget, wantAnchorSlug string,
) []backlinkRecord {
	wikilinks := linkgraph.ExtractWikiLinks(f)
	if len(wikilinks) == 0 {
		return out
	}
	root := f.RootFS
	if root == nil {
		return out
	}
	for _, wl := range wikilinks {
		resolved, ok := linkgraph.ResolveWikiLink(root, srcRel, wl.Target)
		if !ok || resolved != wantTarget {
			continue
		}
		if wantAnchorSlug != "" && linkgraph.NormalizeAnchor(wl.Anchor) != wantAnchorSlug {
			continue
		}
		text := wl.Alias
		if text == "" {
			text = wl.Target
		}
		out = append(out, backlinkRecord{
			Source: srcRel,
			Line:   wl.Line + f.LineOffset,
			Text:   text,
			Target: wikilinkTargetString(wl),
			Kind:   "wikilink",
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

// workspaceRelativePath returns p relative to rootDir using forward
// slashes. When rootDir is empty or filepath.Rel cannot relate the
// paths, p is returned as-is with separators normalized.
func workspaceRelativePath(p, rootDir string) string {
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

// isWorkspaceRelativeTarget reports whether target is a usable
// workspace-relative path. Absolute paths (POSIX / Windows / UNC)
// and parent-traversal entries are rejected so the caller can fail
// loudly instead of silently producing an empty result set.
func isWorkspaceRelativeTarget(target string) bool {
	t := filepath.ToSlash(target)
	if isAbsOrDriveOrUNC(t) {
		return false
	}
	cleaned := path.Clean(t)
	return cleaned != ".." && !strings.HasPrefix(cleaned, "../")
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

// formatBacklinkTextLine renders one record for the human text
// format. Standard links use the `[text](target)` shape; wikilinks
// use the source-form `[[target]]` (or `[[target|alias]]`,
// `![[target]]` for embeds) so a user scanning output can see at a
// glance which records came from each link kind.
func formatBacklinkTextLine(r backlinkRecord) string {
	if r.Kind == "wikilink" {
		alias := ""
		if r.Text != "" && r.Text != r.Target {
			alias = "|" + r.Text
		}
		return fmt.Sprintf("%s:%d: [[%s%s]]", r.Source, r.Line, r.Target, alias)
	}
	return fmt.Sprintf("%s:%d: [%s](%s)", r.Source, r.Line, r.Text, r.Target)
}

// emitBacklinks writes records to w using format. limit caps the
// output; 0 means no cap. Exit code: 0 when records were emitted,
// 1 when none were found, 2 on write error.
func emitBacklinks(w io.Writer, records []backlinkRecord, format string, limit int) int {
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	switch format {
	case "json":
		// Stable shape: empty results emit `[]`, not `null`, so
		// downstream tools can `length()` without a nil branch.
		out := records
		if out == nil {
			out = []backlinkRecord{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: writing json: %v\n", err)
			return 2
		}
	case "text", "":
		for _, r := range records {
			line := formatBacklinkTextLine(r)
			if _, err := fmt.Fprintln(w, line); err != nil {
				fmt.Fprintf(os.Stderr, "mdsmith: writing output: %v\n", err)
				return 2
			}
		}
	default:
		fmt.Fprintf(os.Stderr, "mdsmith: unknown --format %q (want text or json)\n", format)
		return 2
	}
	if len(records) == 0 {
		return 1
	}
	return 0
}
