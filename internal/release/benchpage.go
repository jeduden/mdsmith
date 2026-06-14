package release

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// benchReadmeRel is the repo-relative path of the benchmark research
// README whose rendered, link-rewritten copy release.yml publishes to
// the orphan assets branch (assets/benchmarks/pages/benchmark.md). It
// is the prose write-up the performance page links to: a copy carrying
// this release's freshly measured numbers, where main's committed
// snapshot lags between deliberate run.sh refreshes.
const benchReadmeRel = benchDirRel + "/README.md"

// linkScheme matches a leading URI scheme (https:, mailto:, …) so an
// already-absolute link target is left untouched by the rewrite.
var linkScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*:`)

// benchInlineLink matches an inline Markdown link's `](target)` tail,
// capturing the target (group 1, up to the first whitespace or `)`)
// and an optional double-quoted title (group 2). Image embeds share
// the same tail and are rewritten identically — both are real link
// targets once the page is lifted off the repo tree.
var benchInlineLink = regexp.MustCompile(`\]\(([^)\s]+)((?:\s+"[^"]*")?)\)`)

// benchRefDef matches a reference-style link definition line,
// capturing the `[label]: ` prefix (group 1), the target (group 2),
// and an optional title (group 3). Multiline so `^`/`$` anchor at each
// line within the non-code segments applyOutsideCode hands it.
var benchRefDef = regexp.MustCompile(
	`(?m)^(\[[^\]]+\]:[ \t]+)(\S+)((?:[ \t]+"[^"]*")?)[ \t]*$`)

// rewriteRelativeLinksToGitHub rewrites every repo-relative Markdown
// link in data — resolved against srcDirRel, a repo-root-relative
// directory — to an absolute GitHub URL on main, so a page lifted out
// of the repo tree has no link that 404s. The benchmark README is the
// caller: published to the orphan assets branch, none of its sibling
// files (run.sh, the coverage matrix, the rule READMEs it cites) exist
// there, so each relative link must point back at github.com/main.
//
// Targets that already resolve as-is are left untouched: anchor-only
// (`#sec`), site-absolute (`/x`), and scheme-qualified (`https://…`,
// `mailto:…`) links, plus anything inside a fenced block or inline
// code span — those are documentation examples, not real targets, and
// applyOutsideCode keeps the rewrite away from them.
func rewriteRelativeLinksToGitHub(data []byte, srcDirRel string) []byte {
	return applyOutsideCode(data, func(seg []byte) []byte {
		seg = benchInlineLink.ReplaceAllFunc(seg, func(m []byte) []byte {
			sub := benchInlineLink.FindSubmatch(m)
			url, ok := githubURLForRelativeTarget(string(sub[1]), srcDirRel)
			if !ok {
				return m
			}
			return []byte("](" + url + string(sub[2]) + ")")
		})
		return benchRefDef.ReplaceAllFunc(seg, func(m []byte) []byte {
			sub := benchRefDef.FindSubmatch(m)
			url, ok := githubURLForRelativeTarget(string(sub[2]), srcDirRel)
			if !ok {
				return m
			}
			return []byte(string(sub[1]) + url + string(sub[3]))
		})
	})
}

// githubURLForRelativeTarget resolves a single Markdown link target
// against srcDirRel and returns its absolute GitHub URL on main plus
// true, or ("", false) when the target must be left as-is (empty,
// anchor-only, site-absolute, or already scheme-qualified). A trailing
// `#fragment` is preserved, and a trailing slash routes to /tree/
// (GitHub's directory view) rather than /blob/ via githubURLForPath.
func githubURLForRelativeTarget(target, srcDirRel string) (string, bool) {
	if target == "" || target[0] == '#' || target[0] == '/' ||
		linkScheme.MatchString(target) {
		return "", false
	}
	rel, frag := target, ""
	if i := strings.IndexByte(target, '#'); i >= 0 {
		rel, frag = target[:i], target[i:]
	}
	// rel is always non-empty here: the guard above rejects a
	// leading '#', so any '#' found sits at index >= 1.
	resolved := path.Join(srcDirRel, rel)
	if strings.HasSuffix(rel, "/") {
		resolved += "/"
	}
	return githubURLForPath([]byte(resolved)) + frag, true
}

// RenderBenchPage reads the benchmark README (already `mdsmith fix`ed
// upstream so its <?include?> tables carry this release's freshly
// measured numbers), rewrites every repo-relative link to an absolute
// GitHub URL on main, and writes the result to outPath. release.yml's
// benchmark-publish job publishes that file to
// assets/benchmarks/pages/benchmark.md so the performance page links a
// rendered, fresh-numbers copy whose inner links all resolve.
func (t *Toolkit) RenderBenchPage(root, outPath string) error {
	src := filepath.Join(root, filepath.FromSlash(benchReadmeRel))
	data, err := t.fs.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read benchmark README %s: %w", src, err)
	}
	page := rewriteRelativeLinksToGitHub(data, benchDirRel)
	dir := filepath.Dir(outPath)
	if err := t.fs.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := t.fs.WriteFile(outPath, page, 0o644); err != nil {
		return fmt.Errorf("write benchmark page %s: %w", outPath, err)
	}
	return nil
}

// RenderBenchPage delegates to a default-OS Toolkit (see Stamp).
func RenderBenchPage(root, outPath string) error {
	return New().RenderBenchPage(root, outPath)
}
