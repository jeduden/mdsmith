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

	"github.com/jeduden/mdsmith/internal/globpath"
	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
)

// backlinkRecord is one incoming link to the queried target.
type backlinkRecord struct {
	Source string `json:"source"`
	Line   int    `json:"line"`
	Text   string `json:"text"`
	Target string `json:"target"`
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

// parseBacklinksFlags parses the flags for `mdsmith backlinks` and
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
		fmt.Fprint(os.Stderr, "Usage: mdsmith backlinks [flags] <target>\n\n"+
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
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: backlinks"); code >= 0 {
			return code
		}
	}

	if len(posArgs) == 0 {
		fmt.Fprint(os.Stderr, "mdsmith: backlinks requires a target file argument\n")
		return 2
	}
	if len(posArgs) > 1 {
		fmt.Fprintf(os.Stderr, "mdsmith: backlinks takes one target argument, got %d\n", len(posArgs))
		return 2
	}

	targetPath, targetAnchor := splitTarget(posArgs[0])
	if targetPath == "" {
		fmt.Fprint(os.Stderr, "mdsmith: backlinks target must include a file path\n")
		return 2
	}

	if err := validateIncludePatterns(opts.includePatterns); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
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
	wantTarget := normalizeWorkspacePath(targetPath, rootDir)

	records, errs := collectBacklinks(files, rootDir, wantTarget, targetAnchor, opts.includePatterns, maxBytes)
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", e)
	}

	return emitBacklinks(os.Stdout, records, opts.format, opts.limit)
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

// splitTarget separates `path#anchor` into (path, anchor). A bare
// path returns ("path", ""). A leading `#` (anchor-only) returns
// ("", "anchor") — that is rejected by the caller because backlinks
// always operate on a file target.
func splitTarget(arg string) (path, anchor string) {
	if i := strings.IndexByte(arg, '#'); i >= 0 {
		return arg[:i], arg[i+1:]
	}
	return arg, ""
}

// normalizeWorkspacePath returns a workspace-relative form of target,
// keyed by rootDir. Absolute paths are converted via filepath.Rel.
// Paths that escape rootDir return the cleaned absolute form so they
// match nothing — backlinks are workspace-internal only.
func normalizeWorkspacePath(target, rootDir string) string {
	t := filepath.ToSlash(target)
	t = strings.TrimPrefix(t, "./")
	if filepath.IsAbs(filepath.FromSlash(t)) && rootDir != "" {
		absRoot, _ := filepath.Abs(rootDir)
		rel, err := filepath.Rel(absRoot, filepath.FromSlash(t))
		if err == nil && !strings.HasPrefix(rel, "..") {
			t = filepath.ToSlash(rel)
		}
	}
	return path.Clean(t)
}

// collectBacklinks walks every source file in files, extracts its
// outgoing links via linkgraph, and returns one record per link whose
// resolved workspace-relative target equals wantTarget. When anchor
// is non-empty, the link's anchor must also match (after slugifying).
// includePatterns, when non-empty, filters source paths.
//
// Per-file read or parse failures do not abort the walk; instead they
// are collected and returned so the caller can surface them on stderr
// alongside whatever results were produced.
func collectBacklinks(
	files []string,
	rootDir, wantTarget, wantAnchor string,
	includePatterns []string,
	maxBytes int64,
) ([]backlinkRecord, []error) {
	wantAnchorSlug := ""
	if wantAnchor != "" {
		wantAnchorSlug = linkgraph.NormalizeAnchor(wantAnchor)
	}

	var records []backlinkRecord
	var errs []error
	for _, src := range files {
		srcRel := workspaceRelativePath(src, rootDir)
		if !sourceMatches(srcRel, includePatterns) {
			continue
		}
		// A file never lists itself in its own backlinks.
		if srcRel == wantTarget {
			continue
		}
		data, err := lint.ReadFileLimited(src, maxBytes)
		if err != nil {
			errs = append(errs, fmt.Errorf("reading %s: %w", srcRel, err))
			continue
		}
		// Reuse the lint pipeline's front-matter handling so line
		// numbers in records match what users see in editors.
		f, err := lint.NewFileFromSource(src, data, true)
		if err != nil {
			errs = append(errs, fmt.Errorf("parsing %s: %w", srcRel, err))
			continue
		}
		for _, link := range linkgraph.ExtractLinks(f) {
			t := link.Target
			if t.LocalAnchor || t.Path == "" {
				continue
			}
			resolved := resolveLinkTarget(srcRel, t.Path)
			if resolved == "" || resolved != wantTarget {
				continue
			}
			if wantAnchorSlug != "" {
				if linkgraph.NormalizeAnchor(t.Anchor) != wantAnchorSlug {
					continue
				}
			}
			records = append(records, backlinkRecord{
				Source: srcRel,
				// ExtractLinks returns body-relative lines (rules need
				// that for the engine's offset-adjustment); the CLI
				// shows file-relative line numbers, so add f.LineOffset
				// back in.
				Line:   link.Line + f.LineOffset,
				Text:   link.Text,
				Target: t.Raw,
			})
		}
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Source != records[j].Source {
			return records[i].Source < records[j].Source
		}
		return records[i].Line < records[j].Line
	})
	return records, errs
}

// workspaceRelativePath returns p relative to rootDir using forward
// slashes. When rootDir is empty or p cannot be made relative, p is
// returned as-is with separators normalized.
func workspaceRelativePath(p, rootDir string) string {
	cleaned := filepath.ToSlash(p)
	if rootDir == "" {
		return strings.TrimPrefix(cleaned, "./")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return strings.TrimPrefix(cleaned, "./")
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return strings.TrimPrefix(cleaned, "./")
	}
	rel, err := filepath.Rel(absRoot, abs)
	if err != nil {
		return strings.TrimPrefix(cleaned, "./")
	}
	return filepath.ToSlash(rel)
}

// resolveLinkTarget joins srcRel's directory with the link's path and
// returns the workspace-relative result. Both inputs use forward
// slashes. Absolute paths and ones that escape the workspace root
// return "" so callers treat them as "outside the graph".
func resolveLinkTarget(srcRel, linkPath string) string {
	srcRel = strings.ReplaceAll(srcRel, `\`, `/`)
	linkPath = strings.ReplaceAll(linkPath, `\`, `/`)
	if path.IsAbs(srcRel) || path.IsAbs(linkPath) {
		return ""
	}
	dir := path.Dir(srcRel)
	cleaned := path.Clean(path.Join(dir, linkPath))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	return cleaned
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
			if _, err := fmt.Fprintf(w, "%s:%d: [%s](%s)\n", r.Source, r.Line, r.Text, r.Target); err != nil {
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
