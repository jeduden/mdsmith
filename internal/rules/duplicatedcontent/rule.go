// Package duplicatedcontent implements MDS037, which flags substantial
// paragraphs that also appear verbatim in another Markdown file in the
// project root after whitespace and case normalization.
package duplicatedcontent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	gopath "path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/globpath"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdpath"
	"github.com/jeduden/mdsmith/internal/piparser"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// defaultMinChars is the minimum normalized paragraph length (in runes)
// that makes a paragraph large enough to be worth flagging as a duplicate.
// Below this threshold paragraphs like "See [foo](bar)." accumulate too
// many coincidental matches across a documentation corpus.
const defaultMinChars = 200

func init() {
	rule.Register(&Rule{})
}

// Rule detects paragraphs duplicated across Markdown files in the corpus.
type Rule struct {
	Include  []string
	Exclude  []string
	MinChars int
}

// EnabledByDefault implements rule.Defaultable. MDS037 is opt-in: in a
// project that intentionally shares prose across agent files (AGENTS.md,
// CLAUDE.md, .github/copilot-instructions.md, include-expanded docs) the
// default behavior would fire on every boilerplate paragraph. Projects
// that want duplication checks enable it explicitly in `.mdsmith.yml`.
func (r *Rule) EnabledByDefault() bool { return false }

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS037" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "duplicated-content" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "prose" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f.AST == nil {
		return nil
	}
	// Stdin / in-memory source has no filesystem context; a cross-
	// file rule cannot meaningfully run against it. Match MDS021/
	// MDS027 and short-circuit instead of walking RootFS behind the
	// user's back when they piped content through `-`.
	if f.FS == nil {
		return nil
	}

	// Validate config first so bad globs surface even on files that
	// contain no qualifying paragraphs.
	if configErr := r.validateFilters(); configErr != nil {
		return []lint.Diagnostic{configDiag(f, r, configErr)}
	}

	minChars := r.MinChars
	if minChars <= 0 {
		minChars = defaultMinChars
	}

	self := extractParagraphs(f, minChars)
	if len(self) == 0 {
		return nil
	}

	// resolveCorpus is guaranteed non-nil here: the f.FS == nil
	// guard above short-circuits, and resolveCorpus falls back to
	// f.FS when RootFS is missing or rootRelative fails.
	corpus, selfName, rootDir := resolveCorpus(f)

	// index covers every corpus file, self included: it is shared and
	// cached (via cfg.runCache.CorpusIndex) across every host file that
	// scans this same corpus signature, so it cannot exclude any one
	// host file's own entries at build time — the exclusion has to
	// happen per host file, below, using this call's own selfName.
	index := buildCorpusIndex(corpusScanConfig{
		runCache:         f.RunCache,
		rootDir:          rootDir,
		corpus:           corpus,
		maxBytes:         f.MaxInputBytes,
		minChars:         minChars,
		keySuffix:        "\x00" + strconv.Itoa(minChars),
		stripFrontMatter: f.StripFrontMatter,
		include:          r.Include,
		exclude:          r.Exclude,
	})

	var diags []lint.Diagnostic
	for _, p := range self {
		matches, ok := index[p.fingerprint]
		if !ok {
			continue
		}
		for _, m := range matches {
			if m.path == selfName {
				continue
			}
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     p.line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message: fmt.Sprintf(
					"paragraph duplicated in %s:%d",
					m.path, m.line,
				),
			})
		}
	}
	return diags
}

// paragraph is a fingerprinted paragraph in a single file.
type paragraph struct {
	fingerprint string
	line        int
}

// externalMatch is a paragraph match found in another file. The line is
// already adjusted for the other file's front-matter offset.
type externalMatch struct {
	path string
	line int
}

// generatedRanges returns the [start, stop) byte ranges that cover the
// body of generated sections (<?include?> and <?catalog?>). Only
// top-level, well-formed open/close pairs produce a range; malformed or
// unmatched markers are silently skipped, which is safe because the
// generated-section rule (MDS031/MDS032) handles those errors separately.
//
// Nested same-name pairs (an inner <?include?> inside an outer
// <?include?> body) are handled with a depth counter so the outer range
// does not close prematurely on the inner end marker.
func generatedRanges(f *lint.File) [][2]int {
	if f.AST == nil {
		return nil
	}
	var ranges [][2]int
	var openPI *piparser.ProcessingInstruction
	depth := 0
	for n := f.AST.FirstChild(); n != nil; n = n.NextSibling() {
		pi, ok := n.(*piparser.ProcessingInstruction)
		if !ok {
			continue
		}
		if openPI == nil {
			if (pi.Name == "include" || pi.Name == "catalog") && pi.HasClosure() {
				openPI = pi
				depth = 0
			}
		} else if pi.Name == openPI.Name && pi.HasClosure() {
			depth++
		} else if pi.Name == "/"+openPI.Name && pi.HasClosure() && pi.Lines().Len() > 0 {
			if depth > 0 {
				depth--
			} else {
				start := openPI.ClosureLine.Stop
				stop := pi.Lines().At(0).Start
				if stop > start {
					ranges = append(ranges, [2]int{start, stop})
				}
				openPI = nil
			}
		}
	}
	return ranges
}

// inGeneratedRange reports whether offset falls within any of the given
// [start, stop) byte ranges.
func inGeneratedRange(offset int, ranges [][2]int) bool {
	for _, r := range ranges {
		if offset >= r[0] && offset < r[1] {
			return true
		}
	}
	return false
}

// extractParagraphs walks f.AST and returns fingerprints for every
// paragraph whose normalized text is at least minChars runes long.
// Paragraphs are read via Node.Lines so raw markdown text — not rendered
// inline output — feeds the fingerprint. Paragraphs with no source lines
// (a shape goldmark never produces today, but cheap to guard), ones
// shorter than the threshold, and paragraphs inside generated sections
// (<?include?> or <?catalog?> bodies) are skipped.
//
// raw and norm are scratch []byte buffers reused across every paragraph
// in the file (reset via buf[:0], never reallocated down). A prior
// version rebuilt a fresh strings.Builder per paragraph for line-
// gathering, another fresh one inside normalize for whitespace/case
// folding, and a []byte(normalized) copy before hashing — three
// allocations on every paragraph, none of them reusable because
// strings.Builder.Reset drops its backing array (immutability: a
// string already handed to a caller via String() must not be
// invalidated by a later Write). Working end to end in []byte and
// hashing sha256.Sum256(norm) directly (no string round-trip) lets the
// same two buffers serve the whole file: their capacity stabilizes at
// the largest paragraph seen, so later paragraphs allocate nothing.
// slices.Grow before the first append into each buffer keeps the very
// first (or largest-yet) paragraph in a file from paying incremental
// regrowth the way a bare append from nil would — the segment-length
// sum needed for it is pure arithmetic over already-fetched start/stop
// offsets, not a second pass over the source bytes. See
// docs/development/high-performance-go.md "Reuse loop-local buffers"
// and "Pre-size slices".
func extractParagraphs(f *lint.File, minChars int) []paragraph {
	genRanges := generatedRanges(f)
	var out []paragraph
	var raw, norm []byte
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n.Kind() != ast.KindParagraph {
			return ast.WalkContinue, nil
		}
		lines := n.Lines()
		if lines.Len() == 0 {
			return ast.WalkSkipChildren, nil
		}
		if inGeneratedRange(lines.At(0).Start, genRanges) {
			return ast.WalkSkipChildren, nil
		}
		size := 0
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			size += seg.Stop - seg.Start
		}
		raw = slices.Grow(raw[:0], size)
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			raw = append(raw, seg.Value(f.Source)...)
		}
		// Whitespace collapse only shrinks, and lowercasing a rune
		// essentially never grows its UTF-8 encoding, so len(raw) is a
		// good capacity estimate — not a hard guarantee, so append
		// still grows norm on its own in the rare case this undersizes
		// it, exactly as it would without the hint.
		norm = appendNormalized(slices.Grow(norm[:0], len(raw)), raw)
		if utf8.RuneCount(norm) < minChars {
			return ast.WalkSkipChildren, nil
		}
		sum := sha256.Sum256(norm)
		out = append(out, paragraph{
			fingerprint: hex.EncodeToString(sum[:]),
			line:        f.LineOfOffset(lines.At(0).Start),
		})
		return ast.WalkSkipChildren, nil
	})
	return out
}

// appendNormalized appends the normalized form of src onto dst: runs of
// whitespace collapse to a single space, letters lowercase, and neither
// a leading nor a trailing space survives. The goal is to treat
// paragraphs that differ only by reflow or case as duplicates.
func appendNormalized(dst, src []byte) []byte {
	start := len(dst)
	inSpace := false
	for i := 0; i < len(src); {
		r, size := utf8.DecodeRune(src[i:])
		i += size
		if unicode.IsSpace(r) {
			if !inSpace && len(dst) > start {
				dst = append(dst, ' ')
			}
			inSpace = true
			continue
		}
		dst = utf8.AppendRune(dst, unicode.ToLower(r))
		inSpace = false
	}
	if n := len(dst); n > start && dst[n-1] == ' ' {
		dst = dst[:n-1]
	}
	return dst
}

// resolveCorpus picks the filesystem to scan and the path of the current
// file within it. RootFS (the project root) is preferred; otherwise the
// file's own directory is used. The returned selfName is forward-slash,
// fs.FS-style so it can be compared to fs.WalkDir's path argument. The
// returned rootDir is f.RootDir when RootFS is in play, or "" for the
// FS-only fallback — buildCorpusIndex uses it to form the absolute
// on-disk path that keys the per-file RunCache memo; an empty rootDir
// signals "no stable absolute path, skip the RunCache".
//
// f.Path may be absolute (CLI runs with a discovered root) or relative
// to the project root (ResolveFiles returns things like "./docs/a.md").
// Absolute paths go through filepath.Rel; relative paths are cleaned
// and slashed in place. Either way, a self-path that escapes RootDir
// falls through to the FS scope rather than walking the whole project
// root behind the user's back. Callers guarantee f.FS != nil before
// invoking this.
func resolveCorpus(f *lint.File) (corpus fs.FS, selfName string, rootDir string) {
	if f.RootFS != nil && f.RootDir != "" {
		if selfName, ok := rootRelative(f.RootDir, f.Path); ok {
			return f.RootFS, selfName, f.RootDir
		}
	}
	return f.FS, filepath.Base(f.Path), ""
}

// rootRelative returns path expressed relative to rootDir using forward
// slashes, or ok=false when path escapes rootDir.
//
// The Runner passes file paths through verbatim from the command line,
// so a relative path may be CWD-relative rather than root-relative
// (e.g. running `mdsmith check a.md` from the `docs/` subdirectory
// gives `f.Path = "a.md"` even though the file lives at `docs/a.md`
// under RootDir). To handle that uniformly, convert to an absolute
// path first and then compute the relative against RootDir; that way
// both absolute inputs and any flavor of relative input resolve to
// the same root-relative string.
func rootRelative(rootDir, path string) (string, bool) {
	absPath := path
	if !filepath.IsAbs(path) {
		var err error
		absPath, err = filepath.Abs(path)
		if err != nil {
			return "", false
		}
	}
	rel, err := filepath.Rel(rootDir, absPath)
	if err != nil {
		return "", false
	}
	slash := filepath.ToSlash(rel)
	slash = strings.TrimPrefix(slash, "./")
	if slash == ".." || strings.HasPrefix(slash, "../") {
		return "", false
	}
	return slash, true
}

// corpusScanConfig bundles the settings buildCorpusIndex and
// candidateParagraphs all need to walk one corpus. Building it once
// in Check instead of re-threading each setting as its own
// positional parameter keeps a future setting addition (RunCache and
// keySuffix were both added this way) from growing an already-long
// parameter list further.
//
// It deliberately carries no host-file-specific field (no selfName):
// buildCorpusIndex's result is shared and cached across every host
// file scanning the same corpus signature, so it must not depend on
// which file is asking. Excluding the asking file's own entries from
// the corpus is Check's job, applied to the shared result.
type corpusScanConfig struct {
	runCache *lint.RunCache
	corpus   fs.FS
	// rootDir is the absolute on-disk directory corpus is rooted at,
	// or "" for the FS-only fallback (no stable absolute path) —
	// candidateParagraphs skips the RunCache in that case.
	rootDir  string
	maxBytes int64
	minChars int
	// keySuffix is "\x00"+minChars, built once per Check call since
	// minChars is fixed for the whole corpus walk.
	keySuffix        string
	stripFrontMatter bool
	include, exclude []string
}

// buildCorpusIndex resolves cfg's corpus file list (via corpusFiles)
// and returns a map from paragraph fingerprint to every occurrence
// found across every corpus file. It does NOT exclude the asking host
// file's own entries — Check does that, after the call, using its own
// selfName. The result is shared (via cfg.runCache.CorpusIndex) across
// every host file that scans an identical corpus signature, and the
// corpus signature does not include which file is asking, so an
// exclusion baked in here would only be correct for whichever host
// file happened to trigger the first build; every other host file
// sharing the cache would see a stale exclusion (RunCache.CorpusIndex
// caught this on review; TestCheck_SelfExclusionSurvivesSharedRunCache
// pins it).
//
// Files that can't be read or parsed are silently skipped — this rule
// is advisory and should never fail a run because a sibling file is
// malformed or oversize.
//
// cfg.runCache and cfg.rootDir let candidateParagraphs memoize each
// sibling's fingerprinted paragraphs on the engine's RunCache: a
// workspace of N files enabling MDS037 otherwise re-reads and
// re-parses every sibling from scratch on every host file's Check, an
// O(N^2) cost across the run.
//
// The aggregation itself — allocating the map and populating it from
// every corpus file's (already-memoized) paragraphs, then sorting
// each fingerprint's matches — is memoized on cfg.runCache.CorpusIndex
// too: without it, that O(N)-sized rebuild still ran once per host
// file, an O(N^2) cost even though every leaf read was already
// cached. See docs/development/high-performance-go.md, "Memoize
// per-input computations".
func buildCorpusIndex(cfg corpusScanConfig) map[string][]externalMatch {
	build := func() map[string][]externalMatch {
		index := make(map[string][]externalMatch)
		for _, path := range corpusFiles(cfg) {
			indexFile(index, cfg, path)
		}

		// Sort each fingerprint's matches so diagnostics are
		// deterministic. slices.SortFunc sorts the concrete
		// externalMatch values directly, unlike sort.Slice, which
		// drives reflect.Swapper under the hood — see
		// docs/development/high-performance-go.md's "reflect in hot
		// paths" anti-pattern.
		for fp, matches := range index {
			slices.SortFunc(matches, func(a, b externalMatch) int {
				if a.path != b.path {
					return strings.Compare(a.path, b.path)
				}
				return a.line - b.line
			})
			index[fp] = matches
		}
		return index
	}
	if cfg.runCache == nil || cfg.rootDir == "" {
		return build()
	}
	v := cfg.runCache.CorpusIndex(corpusFilesKey(cfg)+cfg.keySuffix, func() any { return build() })
	index, _ := v.(map[string][]externalMatch)
	return index
}

// corpusFiles resolves the corpus's eligible Markdown paths (matching
// mdpath.IsMarkdownPath and cfg.include/cfg.exclude, honoring the same
// directory-skip rules as before), memoized on cfg.runCache.GlobMatches
// — the same tree-shape-only cache the catalog rule and the wikilink
// index use. The result depends only on which files exist plus the
// rule's include/exclude settings, never on file content, so a host
// file's own edit cannot stale it; only a create/delete/rename can,
// and the engine already calls RunCache.InvalidateGlobMatches on
// those. Without cfg.runCache or cfg.rootDir (in-memory FS, no stable
// absolute path) every call walks the tree directly.
func corpusFiles(cfg corpusScanConfig) []string {
	build := func() []string {
		var files []string
		_ = fs.WalkDir(cfg.corpus, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return walkDirDecision(path, cfg.exclude)
			}
			if !mdpath.IsMarkdownPath(path) || !matchesFilters(path, cfg.include, cfg.exclude) {
				return nil
			}
			files = append(files, path)
			return nil
		})
		return files
	}
	if cfg.runCache == nil || cfg.rootDir == "" {
		return build()
	}
	return cfg.runCache.GlobMatches(corpusFilesKey(cfg), build)
}

// corpusFilesKey builds corpusFiles' RunCache key from cfg.rootDir
// plus cfg.include/cfg.exclude — the settings corpusFiles' build
// closure actually reads. minChars is deliberately excluded: it
// affects which paragraphs within a file qualify (candidateParagraphs'
// own key already covers that), not which files the corpus walk
// visits. Each pattern is length-prefixed so a boundary can never be
// ambiguous between adjacent patterns, mirroring the catalog rule's
// globMatchesKey.
func corpusFilesKey(cfg corpusScanConfig) string {
	var key strings.Builder
	key.Grow(64)
	writeKeyPart := func(s string) {
		key.WriteString(strconv.Itoa(len(s)))
		key.WriteByte(':')
		key.WriteString(s)
	}
	key.WriteString("duplicatedcontent-corpus\x00")
	writeKeyPart(cfg.rootDir)
	key.WriteString(strconv.Itoa(len(cfg.include)))
	for _, p := range cfg.include {
		writeKeyPart(p)
	}
	key.WriteString(strconv.Itoa(len(cfg.exclude)))
	for _, p := range cfg.exclude {
		writeKeyPart(p)
	}
	return key.String()
}

// walkDirDecision returns the fs.WalkDirFunc verdict for a directory:
// descend normally, or SkipDir for known-heavy subtrees (`.git`,
// `node_modules`) and user-configured excludes.
func walkDirDecision(p string, exclude []string) error {
	if p == "." {
		return nil
	}
	// fs.WalkDir always yields forward-slash paths; gopath.Base
	// splits on '/' regardless of OS, while filepath.Base would
	// only split on '\\' on Windows and leave the whole path
	// intact.
	switch gopath.Base(p) {
	case ".git", "node_modules":
		return fs.SkipDir
	}
	if shouldSkipDir(p, exclude) {
		return fs.SkipDir
	}
	return nil
}

// indexFile appends every paragraph fingerprint a corpus file
// contains into index. corpusFiles has already applied
// IsMarkdownPath and include/exclude filtering, so every path this
// is called with belongs in the aggregate — including whichever
// file will later ask about it as the host (Check excludes the
// asking file's own entries after the call, not here; see
// buildCorpusIndex). Unreadable or unparseable files are silently
// dropped — this rule is advisory and must not fail a run because of
// a sibling.
func indexFile(index map[string][]externalMatch, cfg corpusScanConfig, path string) {
	for _, p := range candidateParagraphs(cfg, path) {
		index[p.fingerprint] = append(index[p.fingerprint], externalMatch{
			path: path,
			line: p.line,
		})
	}
}

// candidateParagraphs returns a corpus candidate's fingerprinted
// paragraphs, adjusted for its own front-matter line offset. When
// cfg.runCache and cfg.rootDir are both available, the result is
// memoized on the RunCache keyed by the candidate's absolute on-disk
// path plus cfg.keySuffix (built from minChars, the one rule setting
// that can vary per file kind and therefore change which paragraphs
// qualify) — RunCache.Invalidate evicts by the absPath prefix, so an
// LSP document edit still refreshes the right slot. Without a stable
// absolute path (in-memory FS, or no RunCache at all), every call
// reads and re-parses the file directly.
func candidateParagraphs(cfg corpusScanConfig, path string) []paragraph {
	build := func() []paragraph {
		data, err := bytelimit.ReadFSFileLimited(cfg.corpus, path, cfg.maxBytes)
		if err != nil {
			return nil
		}
		// NewFileFromSource cannot fail for in-memory bytes that came
		// out of ReadFSFileLimited successfully; goldmark's parser
		// does not error on any input. The error return is kept in
		// the signature for future-proofing but is dead here.
		other, _ := lint.NewFileFromSource(path, data, cfg.stripFrontMatter) //nolint:errcheck
		paragraphs := extractParagraphs(other, cfg.minChars)
		for i := range paragraphs {
			paragraphs[i].line += other.LineOffset
		}
		return paragraphs
	}
	if cfg.runCache == nil || cfg.rootDir == "" {
		return build()
	}
	absPath := filepath.Join(cfg.rootDir, filepath.FromSlash(path))
	v := cfg.runCache.DuplicateParagraphs(absPath+cfg.keySuffix, func() any { return build() })
	paragraphs, _ := v.([]paragraph)
	return paragraphs
}

// shouldSkipDir reports whether a directory path matches one of the
// exclude patterns and should be pruned from the walk. Matching the
// slash path lets patterns like "vendor/**" hit at the directory
// boundary; matching basename lets ".git" or "node_modules" skip
// wherever they appear in the tree. Include patterns are intentionally
// not consulted here: excluding a subtree is safe, but a missing
// include match at the directory level would skip subtree entries
// that individual include patterns could still allow.
func shouldSkipDir(p string, exclude []string) bool {
	if len(exclude) == 0 {
		return false
	}
	// p is an fs.WalkDir path (forward slash on every OS), so
	// gopath.Base does the right thing cross-platform where
	// filepath.Base would not split on '/' on Windows.
	base := gopath.Base(p)
	// Try the directory path with a trailing slash too so that
	// subtree patterns like "vendor/**" or "docs/generated/**"
	// match at the directory boundary — fs.WalkDir yields
	// "docs/generated" (no trailing slash) even for directories,
	// so the raw glob expects "docs/generated/<rest>" and skips
	// the bare directory without this.
	slashed := p + "/"
	for _, pattern := range exclude {
		matched, err := doublestar.Match(pattern, p)
		if err == nil && matched {
			return true
		}
		matched, err = doublestar.Match(pattern, slashed)
		if err == nil && matched {
			return true
		}
		matched, err = doublestar.Match(pattern, base)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// matchesFilters reports whether path is allowed by include/exclude.
// Patterns are matched against both the full forward-slash path and
// the basename, so `"draft.md"` excludes a file regardless of which
// directory it sits in.
func matchesFilters(p string, include, exclude []string) bool {
	for _, pattern := range exclude {
		if globpath.Match(pattern, p) {
			return false
		}
	}
	if len(include) == 0 {
		return true
	}
	for _, pattern := range include {
		if globpath.Match(pattern, p) {
			return true
		}
	}
	return false
}

// validateFilters checks that include/exclude patterns are valid.
func (r *Rule) validateFilters() error {
	if err := validatePatterns(r.Include); err != nil {
		return fmt.Errorf("include: %w", err)
	}
	if err := validatePatterns(r.Exclude); err != nil {
		return fmt.Errorf("exclude: %w", err)
	}
	return nil
}

// validatePatterns checks that all patterns are valid doublestar patterns.
func validatePatterns(patterns []string) error {
	for _, pat := range patterns {
		if _, err := doublestar.Match(pat, ""); err != nil {
			return fmt.Errorf("invalid glob pattern %q: %w", pat, err)
		}
	}
	return nil
}

func configDiag(f *lint.File, r *Rule, err error) lint.Diagnostic {
	return lint.Diagnostic{
		File:     f.Path,
		Line:     1,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Error,
		Message:  "duplicated-content: " + err.Error(),
	}
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(cfg map[string]any) error {
	for k, v := range cfg {
		switch k {
		case "include":
			list, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"duplicated-content: include must be a list of strings, got %T",
					v,
				)
			}
			r.Include = list
		case "exclude":
			list, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"duplicated-content: exclude must be a list of strings, got %T",
					v,
				)
			}
			r.Exclude = list
		case "min-chars":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf(
					"duplicated-content: min-chars must be an integer, got %T",
					v,
				)
			}
			if n <= 0 {
				// Check treats a zero MinChars as "unset" and applies
				// defaultMinChars, so an explicit 0 in config would be
				// silently ignored; reject it at validation time to
				// keep ApplySettings and Check consistent.
				return fmt.Errorf(
					"duplicated-content: min-chars must be > 0, got %d",
					n,
				)
			}
			r.MinChars = n
		default:
			return fmt.Errorf("duplicated-content: unknown setting %q", k)
		}
	}

	if err := validatePatterns(r.Include); err != nil {
		return fmt.Errorf(
			"duplicated-content: include has invalid glob pattern: %w",
			err,
		)
	}
	if err := validatePatterns(r.Exclude); err != nil {
		return fmt.Errorf(
			"duplicated-content: exclude has invalid glob pattern: %w",
			err,
		)
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"include":   []string{},
		"exclude":   []string{},
		"min-chars": defaultMinChars,
	}
}

var _ rule.Configurable = (*Rule)(nil)
