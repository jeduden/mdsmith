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

// linkTitleTail optionally matches a Markdown link title — a
// double-quoted string preceded by whitespace — that sits between
// an inline link's URL and its closing `)`, or trails a reference
// definition. Every inline rewrite regex used to stop the path
// capture at the first whitespace (`\S+`/`[^)\s]+`) and then
// demand a literal `)`; a title made that `)` unreachable, so the
// whole titled link shipped verbatim as a dead repo-relative path
// (audit gap G6). Appending this group lets each rewrite consume
// the title and re-emit it after the rewritten path. It is a
// single capturing group, so it only ever adds one trailing
// submatch to whatever regex includes it.
const linkTitleTail = `(\s+"[^"]*")?`

// ruleReadmeLink matches a Markdown link whose target is a rule
// README relative path and captures the rule directory (with an
// optional `../` prefix) separately from the `README.md` filename
// and an optional `#anchor` fragment. It covers both link forms
// in play: the bare `MDS001-line-length/README.md` used by the
// rule index, and the sibling `../MDS021-include/README.md` used
// between per-rule READMEs. Rewriting drops `README.md` and
// keeps the captured anchor so the link resolves to the
// published page directory (`MDS001-line-length/`,
// `../MDS021-include/#section`) rather than an unpublished
// `README.md`.
//
// The anchor capture excludes whitespace (`#[^)\s]*`, not
// `#[^)]*`) so it stops at the space before a link title rather
// than swallowing it; the trailing linkTitleTail (group 3) then
// cleanly captures the title so `[x](MDS…/README.md "t")` (or
// `…/README.md#sec "t"`) is rewritten with its title kept rather
// than shipped as a dead path (audit gap G6).
var ruleReadmeLink = regexp.MustCompile(
	`\]\(((?:\.\./)?MDS[0-9A-Za-z._-]+)/README\.md(#[^)\s]*)?` +
		linkTitleTail + `\)`)

// ruleRefDefLink matches a Markdown reference-style link definition
// whose target is a rule README path (with an optional `../`
// prefix and an optional `#anchor` fragment). The multiline flag
// makes `^` match at each line start.
// Example: [mds027]: ../MDS027-cross-file-reference-integrity/README.md#index
// Rewriting drops `/README.md`, adds a trailing slash, and keeps
// the anchor so the reference resolves to the published rule
// page directory.
var ruleRefDefLink = regexp.MustCompile(
	`(?m)^(\[[^\]]+\]: (?:\.\./)?(MDS[0-9A-Za-z._-]+))/README\.md(#\S+)?`)

// repoDocsLink matches an inline link from a rule README into the
// docs/ tree (`../../../docs/path/file.md`). The docs tree IS
// published on the site (the synced content/docs/ tree is mounted
// at the site root), but Hugo serves each page at `/path/file/`
// (no `docs` segment, no `.md`), so the raw relative path
// resolves to a 404. Group 1 captures the path without `.md`;
// group 2 captures an optional `#anchor` fragment.
var repoDocsLink = regexp.MustCompile(`\]\(\.\./\.\./\.\./docs/([^)#]*)\.md([^)]*)\)`)

// repoPlanLink matches an inline link from a rule README into the
// plan/ tree (`../../../plan/file.md`). Plan files are not published
// on the site and must be rewritten as absolute GitHub blob URLs.
var repoPlanLink = regexp.MustCompile(`\]\(\.\./\.\./\.\./plan/([^)]+)\)`)

// repoPlanRefDef matches a reference-style link definition whose
// target is a plan/ file. The multiline flag makes `^` line-anchored.
// Example: [plan107]: ../../../plan/107_no-reference-style.md
var repoPlanRefDef = regexp.MustCompile(`(?m)^(\[[^\]]+\]: )\.\./\.\./\.\./plan/(\S+)`)

// repoRuleLink matches an inline link from any docs/ page into
// the internal/rules/ tree (`../…/internal/rules/MDS001-line-length/`
// or `../…/internal/rules/MDS001-line-length/README.md`). Unlike
// ruleReadmeLink (which only matches the bare or single-`../`
// prefix used inside rule pages themselves), this regex accepts
// any depth of `../` segments so plain docs deep in the tree
// (e.g. docs/background/concepts/generated-section.md, 4 levels
// up) get their rule links rewritten too. Group 1 captures the
// rule directory name. The trailing `README.md` is optional —
// docs link to both forms. Group 2 captures an optional
// `#anchor` so a deep-link into a rule README's heading
// (e.g. `MDS020-required-structure/README.md#index-side-output`)
// preserves the fragment after the slash. The anchor capture
// excludes whitespace (`#[^)\s]*`) so a title is left for the
// trailing linkTitleTail (group 3) to capture, rather than the
// anchor swallowing `#sec "t"` as one blob (gap G6).
var repoRuleLink = regexp.MustCompile(
	`\]\((?:\.\./)+internal/rules/(MDS[0-9A-Za-z._-]+)/` +
		`(?:README\.md)?(#[^)\s]*)?` + linkTitleTail + `\)`)

// repoRuleRefDef matches a reference-style link definition whose
// target is a repo-relative internal/rules/ path. Multiline flag
// so `^` anchors at each line start. Mirrors repoRuleLink — any
// `../` depth, optional README.md suffix, optional anchor.
// Example: [mds020]: ../../internal/rules/MDS020-required-structure/README.md
// Group 4 is the optional linkTitleTail, captured before the `$`
// so a titled definition keeps its title (gap G6) — without it the
// `$` could never match past the title and the def shipped raw.
var repoRuleRefDef = regexp.MustCompile(
	`(?m)^(\[[^\]]+\]: )(?:\.\./)+internal/rules/` +
		`(MDS[0-9A-Za-z._-]+)/(?:README\.md)?(#\S+)?` +
		linkTitleTail + `$`)

// rulePageURLBase is the site-absolute URL prefix every rule page
// lives under. Hugo serves website/content/docs/rules/<dir>/index.md
// at /rules/<dir>/ (content/docs/ is mounted at the site root, so
// there is no `docs` URL segment), so repo-relative
// `internal/rules/<dir>/` links from any docs page must rewrite to
// this prefix to resolve.
const rulePageURLBase = "/rules/"

// repoNonPublishedLink matches an inline link whose target is a
// repo-relative path that the website does NOT publish — every
// tree under the repo that lives outside docs/ and outside
// internal/rules/MDS*/ falls here. On the source tree the link
// resolves to a real file (so mdsmith fix and humans reading the
// source see it fine); on the synced Hugo tree there is no such
// path, so the link 404s. Rewriting to an absolute GitHub blob
// URL keeps the reference clickable on the live site while still
// pointing at the canonical source on github.com.
//
// Group 1 captures the repo-relative path (everything past the
// last `../`). The alternative order matters: every `internal/…`
// prefix that is NOT `internal/rules/MDS…/` falls here, but
// `internal/rules/MDS…/` itself is already rewritten by
// rewriteRuleLinks (run first) and never reaches this regex.
// The `internal/rules/<non-MDS>/` case (e.g.
// `internal/rules/markdownflavor/`) is intentionally caught
// here because only MDS-prefixed directories carry a published
// README.
//
// Root-level files (PLAN.md, README.md, LICENSE, …) are listed
// explicitly. They live at the repo root with no enclosing
// directory, so a `(?:\.\./)+` prefix is the only signal that
// the link target is repo-relative rather than sibling.
//
// Each path uses `\S+` rather than `[^)]+` so the target stops
// at the whitespace before an optional Markdown link title; the
// trailing linkTitleTail (group 2) then consumes that title and
// rewriteNonPublishedInline re-emits it after the GitHub URL,
// instead of the whole titled link shipping unrewritten (G6).
var repoNonPublishedLink = regexp.MustCompile(
	`\]\((?:\.\./)+(` +
		`plan/\S+|` +
		`cmd/\S+|` +
		`editors/\S+|` +
		`cue/\S+|` +
		`npm/\S+|` +
		`python/\S+|` +
		`website/\S+|` +
		`\.claude/\S+|` +
		`\.github/\S+|` +
		`internal/\S+|` +
		`PLAN\.md|README\.md|LICENSE|SECURITY\.md|CLAUDE\.md|AGENTS\.md` +
		`)` + linkTitleTail + `\)`)

// repoNonPublishedRefDef is the reference-style sibling of
// repoNonPublishedLink for definitions like
// `[plan107]: ../../../plan/107.md`. Multiline anchor; `\S+`
// captures the target so trailing whitespace or comments after
// the URL are not eaten.
var repoNonPublishedRefDef = regexp.MustCompile(
	`(?m)^(\[[^\]]+\]: )(?:\.\./)+(` +
		`plan/\S+|` +
		`cmd/\S+|` +
		`editors/\S+|` +
		`cue/\S+|` +
		`npm/\S+|` +
		`python/\S+|` +
		`website/\S+|` +
		`\.claude/\S+|` +
		`\.github/\S+|` +
		`internal/\S+|` +
		`PLAN\.md|README\.md|LICENSE|SECURITY\.md|CLAUDE\.md|AGENTS\.md` +
		`)`)

// repoPrunedDocLink matches an inline link from a published doc
// into a docs/ subtree SyncDocs prunes from the site
// (development, research, security, brand — see
// nonPublishedDocDirs). Those four dirs are direct children of
// docs/, so a `(?:\.\./)+<dir>/…` reference always resolves to
// `docs/<dir>/…` whatever the source page's depth; the captured
// suffix (group 1) is therefore the docs-relative path, and
// rewritePrunedDocInline prepends `docs/` to build the GitHub
// source URL. Group 2 is the optional link title, re-emitted so a
// titled link keeps it. Mirrors repoNonPublishedLink, which cannot
// be reused because its captures are already repo-root-relative.
var repoPrunedDocLink = regexp.MustCompile(
	`\]\((?:\.\./)+((?:development|research|security|brand)/\S+)` +
		linkTitleTail + `\)`)

// repoPrunedDocRefDef is the reference-style sibling of
// repoPrunedDocLink for `[label]: ../../development/foo.md` forms.
// Group 1 is the `[label]: ` prefix; group 2 the docs-relative
// suffix.
var repoPrunedDocRefDef = regexp.MustCompile(
	`(?m)^(\[[^\]]+\]: )(?:\.\./)+` +
		`((?:development|research|security|brand)/\S+)`)

// indexMdLink matches an inline link whose target ends in
// `<path>/index.md` (the docs/-tree convention for a directory
// overview). The rewrite drops the `index.md` filename so the
// target becomes `<path>/`. Hugo serves a section's _index.md
// at the directory URL with no filename; the synced filesystem
// still has `_index.md` at that directory thanks to SyncDocs'
// rename, so MDS027 stats the directory target as a regular
// existing path. The render-link hook
// (website/layouts/_default/_markup/render-link.html) then
// resolves the directory target to the section's absolute
// permalink at HTML-render time, which is what fixes the
// source-vs-rendered-URL depth mismatch that a bare relative
// directory link would otherwise have on leaf pages.
//
// Group 1 captures the path prefix (may be empty for a bare
// `index.md` sibling reference like `docs/development/foo.md`
// linking back to `docs/development/index.md`); group 2 captures
// an optional `#anchor` (whitespace excluded so it stops before
// a title); group 3 is the optional linkTitleTail so a titled
// directory link keeps its title — even when it also carries a
// `#fragment` (gap G6). When group 1 is empty
// rewriteIndexMdLink substitutes `./` so the rewritten target
// stays a directory reference (a bare `#anchor` would silently
// flip semantics to a same-file local-anchor link).
var indexMdLink = regexp.MustCompile(
	`\]\(((?:[^)/]+/)*)index\.md((?:#[^)\s]*)?)` + linkTitleTail + `\)`)

// ruleFixtureLink matches an inline link in a per-rule README
// whose target is a fixture path under the rule's own directory:
// `[good/default.md](good/default.md)`, `[bad/x.md](bad/x.md)`,
// or the `pattern/bad/` / `pattern/good/` directives-rule case.
// Fixtures live in the source tree but are intentionally not
// republished on the site (no Hugo page for raw test data), so a
// repo-relative link 404s. Rewrite to the rule's GitHub source
// tree URL so the example file is still reachable.
//
// `\S*` (not `[^)]*`) stops the target at the whitespace before
// an optional Markdown link title; the trailing linkTitleTail
// (group 2) then consumes that title and rewriteRuleFixtures
// re-emits it after the GitHub URL (gap G6).
var ruleFixtureLink = regexp.MustCompile(
	`\]\(((?:bad|good|pattern)/\S*)` + linkTitleTail + `\)`)

// ruleRelativeNonMDSLink matches an inline link in a per-rule
// README whose target is a `(?:\.\./)+<non-MDS>` reference into
// a repo location outside the rule's own directory. The path's
// first character (the one after the final `../`) is restricted
// to lowercase or a dot so MDS-prefixed cross-rule references
// (`../MDS021-include/…`) — which ruleReadmeLink already routes
// to the site URL — are excluded.
//
// Two depths are common in the source:
//   - depth 1 (`../<x>`): a sibling Go package (../linelength/rule.go),
//     the shared proto.md (../proto.md), or a sibling test file
//     (../paragraphstructure/rule_test.go).
//   - depth 2 (`../../<x>`): a cousin internal/ subdirectory
//     (../../mdtext/sentence_equivalence_test.go).
//
// Depth-3+ references are repo-rooted (../../../docs/,
// ../../../plan/, ../../../internal/, …) and have already been
// rewritten by rewriteRuleLinks (repoDocsLink, repoPlanLink,
// repoNonPublishedLink) by the time transformRulePage applies
// this regex; rewriteRuleRelativeInline declines depths outside
// 1–2 so it cannot fight those rewrites.
//
// The path uses `\S*` so it stops at the whitespace before an
// optional Markdown link title; the trailing linkTitleTail (the
// final captured group) then consumes that title and
// rewriteRuleRelativeInline re-emits it after the GitHub URL.
var ruleRelativeNonMDSLink = regexp.MustCompile(
	`\]\(((?:\.\./)+)([a-z._]\S*)` + linkTitleTail + `\)`)

// ruleRelativeNonMDSRefDef is the reference-style sibling of
// ruleRelativeNonMDSLink. Multiline flag so `^` and `$` anchor at
// each line start/end. The leading capture is the `[label]: `
// prefix, then the `(?:\.\./)+` depth, the non-MDS path, and the
// optional linkTitleTail — re-emitted by rewriteRuleRelativeRefDef
// so a titled definition keeps its title.
var ruleRelativeNonMDSRefDef = regexp.MustCompile(
	`(?m)^(\[[^\]]+\]: )((?:\.\./)+)([a-z._]\S*)` + linkTitleTail + `$`)

// ruleSourceTreeBase is the GitHub directory (tree) route for a
// rule's source. Per-rule READMEs carry an
// `Implementation: [source](./)` link that points at the rule's
// own directory; on the published site `./` would self-link the
// generated page, so syncRulePages rewrites it to the rule's
// GitHub tree URL. `/tree/` (not `/blob/`) is the directory route.
const ruleSourceTreeBase = "https://github.com/jeduden/mdsmith/tree/main/internal/rules/"

// githubBlobBase / githubTreeBase are GitHub's file vs directory
// URL prefixes. A repo-relative link whose target ends with `/`
// must go to /tree/ (GitHub renders a directory listing there);
// any other target — a `.md`, `.go`, an extension-less script —
// must go to /blob/, which is the file route. Using the wrong
// one is not just a stylistic difference: /tree/ on a file 404s
// and /blob/ on a directory 404s, both silently.
const (
	githubBlobBase = "https://github.com/jeduden/mdsmith/blob/main/"
	githubTreeBase = "https://github.com/jeduden/mdsmith/tree/main/"
)

// githubURLForPath returns the GitHub URL for a repo-relative
// path, selecting /tree/ for directory targets (trailing slash)
// and /blob/ for file targets. Centralized so every rewrite that
// emits a GitHub URL picks the right route.
func githubURLForPath(path []byte) string {
	if bytes.HasSuffix(path, []byte("/")) {
		return githubTreeBase + string(path)
	}
	return githubBlobBase + string(path)
}

// ruleDirName matches the MDS-prefixed directory names used for
// per-rule subdirectories under internal/rules/. The prefix guard
// stops syncRulePages from copying non-rule directories (fixtures,
// helper files, …) into the Hugo content tree.
var ruleDirName = regexp.MustCompile(`^MDS[0-9]`)

// rewriteRuleLinks rewrites every repo-relative link in a synced
// markdown body so it resolves on the published site. The three
// classes are applied in order: (1) links into internal/rules/MDS…/
// become /rules/<dir>/<#anchor> site URLs, with <dir> lowercased
// because Hugo case-folds path-derived URLs by default
// (disablePathToLower is off), so the synced
// content/docs/rules/MDS001-line-length/ page is served at
// /rules/mds001-line-length/. The repo-relative source dir keeps
// its MDS… case (MDS027 stats it on disk; absolute targets are
// short-circuited so the lowercased URL is not filesystem-checked);
// (2) links into any
// other non-published repo path — plan/, cmd/, editors/, website/,
// .claude/, internal/ (other than the rule pages already handled
// in step 1), and root-level files — become absolute GitHub URLs,
// /blob/ for file targets and /tree/ for directory targets;
// (3) sibling links to `<path>/index.md` drop the `index.md`
// filename so the target becomes the directory itself (`<path>/`).
// Hugo serves `_index.md` (the rename SyncDocs applies) at that
// directory URL, and MDS027 stats the directory as an existing
// path, so the rendered link works and the lint still resolves.
//
// The whole pass runs under applyOutsideCode so a Markdown
// example inside a fenced code block OR an inline code span
// (an MDS021 README demoing `[rules](../internal/rules/)`, or
// a catalog-output snippet showing `[index.md](development/index.md)`
// in a fenced block) is left verbatim — those examples are
// documentation, not real link targets.
//
// Rule rewrites must precede the non-published rewrite: both
// match `internal/rules/MDS…`, and only the first leaves the
// link on-site rather than routing it to GitHub.
//
// Idempotent: already-rewritten paths (a leading `/rules/`,
// `https://`, or `_index.md`) do not match any regex, so a
// second pass is a no-op.
func rewriteRuleLinks(b []byte) []byte {
	return applyOutsideCode(b, func(seg []byte) []byte {
		seg = repoRuleLink.ReplaceAllFunc(seg, rewriteRepoRuleInline)
		seg = repoRuleRefDef.ReplaceAllFunc(seg, rewriteRepoRuleRefDef)
		seg = repoNonPublishedLink.ReplaceAllFunc(seg, rewriteNonPublishedInline)
		seg = repoNonPublishedRefDef.ReplaceAllFunc(seg, rewriteNonPublishedRefDef)
		// Pruned maintainer-doc links route to GitHub. Must
		// precede indexMdLink so a `../development/x/index.md`
		// link becomes a GitHub URL instead of being stripped to
		// a (now-unpublished) `../development/x/` directory link.
		seg = repoPrunedDocLink.ReplaceAllFunc(seg, rewritePrunedDocInline)
		seg = repoPrunedDocRefDef.ReplaceAllFunc(seg, rewritePrunedDocRefDef)
		// Drop the `index.md` filename — Hugo serves the
		// directory's _index.md at `/<path>/`, not at
		// `/<path>/_index.md` or `/<path>/index.md`. Keeping
		// either filename in the markdown produces a 404 on
		// the live site.
		seg = indexMdLink.ReplaceAllFunc(seg, rewriteIndexMdLink)
		return seg
	})
}

// rewriteIndexMdLink emits the `index.md`-stripped target.
// indexMdLink's group 1 is the path prefix; group 2 is the
// optional `#anchor`; group 3 is the optional linkTitleTail.
// A non-empty prefix passes through (so `dir/index.md` → `dir/`);
// an empty prefix is replaced with `./` so a bare `index.md`
// link rewrites to `./` instead of collapsing to a bare
// `#anchor` (which would be a same-file local-anchor reference,
// a semantic change MDS027 would silently accept against the
// wrong file).
func rewriteIndexMdLink(match []byte) []byte {
	m := indexMdLink.FindSubmatch(match)
	prefix := m[1]
	if len(prefix) == 0 {
		prefix = []byte("./")
	}
	return []byte("](" + string(prefix) + string(m[2]) + string(m[3]) + ")")
}

// rewriteRepoRuleInline rewrites an inline link into
// internal/rules/MDS…/ to its site-absolute rule-page URL. The
// rule-directory capture (m[1]) is lowercased — Hugo case-folds
// path-derived URLs, so the served page is /rules/mds…/ — while
// the optional `#anchor` capture (m[2]) and the optional link
// title (m[3]) pass through unchanged. ReplaceAllFunc (not a
// ReplaceAll template) is required because $1 cannot be
// lowercased inside a replacement template.
func rewriteRepoRuleInline(match []byte) []byte {
	m := repoRuleLink.FindSubmatch(match)
	return []byte("](" + rulePageURLBase +
		strings.ToLower(string(m[1])) + "/" +
		string(m[2]) + string(m[3]) + ")")
}

// rewriteRepoRuleRefDef is the reference-style sibling of
// rewriteRepoRuleInline. m[1] is the `[label]: ` prefix, m[2] the
// rule directory (lowercased to match Hugo's served URL), m[3] an
// optional `#anchor` left as authored, m[4] the optional link
// title re-emitted so the definition keeps it.
func rewriteRepoRuleRefDef(match []byte) []byte {
	m := repoRuleRefDef.FindSubmatch(match)
	return []byte(string(m[1]) + rulePageURLBase +
		strings.ToLower(string(m[2])) + "/" +
		string(m[3]) + string(m[4]))
}

// rewriteNonPublishedInline applies repoNonPublishedLink's
// match, routing the captured path to /tree/ or /blob/ per
// githubURLForPath. ReplaceAllFunc is used (rather than
// ReplaceAll with a template) because the URL prefix depends on
// the captured path's trailing slash — directory links must use
// /tree/ on GitHub and file links must use /blob/, and a
// template cannot branch on the capture. m[2] is the optional
// link title, re-emitted after the URL so a titled link keeps it.
func rewriteNonPublishedInline(match []byte) []byte {
	m := repoNonPublishedLink.FindSubmatch(match)
	return []byte("](" + githubURLForPath(m[1]) + string(m[2]) + ")")
}

// rewriteNonPublishedRefDef is the reference-style sibling of
// rewriteNonPublishedInline. The captured label prefix (m[1])
// is preserved so the definition keeps its `[label]: ` form.
func rewriteNonPublishedRefDef(match []byte) []byte {
	m := repoNonPublishedRefDef.FindSubmatch(match)
	return []byte(string(m[1]) + githubURLForPath(m[2]))
}

// rewritePrunedDocInline routes a link into a pruned maintainer-doc
// tree to its GitHub source. m[1] is the docs-relative suffix
// (e.g. `development/coverage.md`); prepending `docs/` reconstructs
// the repo path githubURLForPath expects. m[2] is the optional link
// title, re-emitted after the URL.
func rewritePrunedDocInline(match []byte) []byte {
	m := repoPrunedDocLink.FindSubmatch(match)
	return []byte("](" + githubURLForPath([]byte("docs/"+string(m[1]))) +
		string(m[2]) + ")")
}

// rewritePrunedDocRefDef is the reference-style sibling of
// rewritePrunedDocInline. m[1] is the `[label]: ` prefix; m[2] the
// docs-relative suffix.
func rewritePrunedDocRefDef(match []byte) []byte {
	m := repoPrunedDocRefDef.FindSubmatch(match)
	return []byte(string(m[1]) + githubURLForPath([]byte("docs/"+string(m[2]))))
}

// applyOutsideCode calls fn on each maximal substring of src
// that lies OUTSIDE Markdown code regions and passes code
// regions through unchanged. Two constructs are skipped:
//
//   - Fenced code blocks: a line beginning (after up to three
//     leading spaces, per CommonMark) with three or more
//     backticks or tildes opens a fence, and the matching run
//     on its own line closes it. The opener, body, and closer
//     all pass through verbatim.
//   - Inline code spans: a backtick-delimited region on a
//     single line (`code`). Multi-backtick spans (the doubled
//     or tripled forms) and multi-line spans fall through as
//     plain text — they are rare in mdsmith's authored docs.
//
// Without these guards the link rewrites would corrupt
// documentation examples that show what a directive PRODUCES:
// MDS021 demoing `[rules](../internal/rules/)` inside an inline
// code span; the generating-content guide demoing
// `[i](development/index.md)` inside a fenced markdown block.
// Indented code blocks and raw HTML blocks are not detected
// because no observed corruption hits them; switching to a
// goldmark-AST walk would handle those at the cost of more
// complexity than the current bug surface justifies.
func applyOutsideCode(src []byte, fn func([]byte) []byte) []byte {
	return applyOutsideFences(src, func(seg []byte) []byte {
		return applyOutsideInlineCode(seg, fn)
	})
}

// applyOutsideFences is the fenced-block half of applyOutsideCode.
// Exposed for the corner case where a caller has already
// stripped inline code spans and only needs the fence guard.
func applyOutsideFences(src []byte, fn func([]byte) []byte) []byte {
	var out bytes.Buffer
	var nonCode bytes.Buffer
	flush := func() {
		if nonCode.Len() == 0 {
			return
		}
		out.Write(fn(nonCode.Bytes()))
		nonCode.Reset()
	}

	inFence := false
	var fenceChar byte
	var fenceLen int

	start := 0
	for i := 0; i <= len(src); i++ {
		if i < len(src) && src[i] != '\n' {
			continue
		}
		line := src[start:i]
		thisChar, thisLen := fenceMarker(line)
		var transition bool
		if !inFence {
			if thisLen >= 3 {
				inFence = true
				fenceChar = thisChar
				fenceLen = thisLen
				transition = true
			}
		} else if thisChar == fenceChar && thisLen >= fenceLen && fenceLineEmptyAfter(line, thisLen) {
			inFence = false
			transition = true
		}

		dst := &nonCode
		if inFence || transition {
			flush()
			dst = &out
		}
		dst.Write(line)
		if i < len(src) {
			dst.WriteByte('\n')
		}
		start = i + 1
	}

	flush()
	return out.Bytes()
}

// inlineCodeSpan matches a single-backtick code span on one
// line: `text`. The body uses a negated character class that
// excludes the backtick and newline, so the match cannot cross
// either — that is what keeps the regex from spanning adjacent
// code spans on the same line or running into a following line.
// Multi-backtick spans (the doubled or tripled opener forms)
// and multi-line spans are not matched.
var inlineCodeSpan = regexp.MustCompile("`[^`\n]+`")

// applyOutsideInlineCode calls fn on every maximal substring of
// b that lies outside an inline code span. The spans themselves
// pass through verbatim. Used inside applyOutsideCode to gate a
// regex rewrite from corrupting a code-spanned example like
// MDS021's `[rules](../internal/rules/)` that documents the
// include directive's output.
func applyOutsideInlineCode(b []byte, fn func([]byte) []byte) []byte {
	matches := inlineCodeSpan.FindAllIndex(b, -1)
	if len(matches) == 0 {
		return fn(b)
	}
	var out bytes.Buffer
	last := 0
	for _, m := range matches {
		out.Write(fn(b[last:m[0]]))
		out.Write(b[m[0]:m[1]])
		last = m[1]
	}
	out.Write(fn(b[last:]))
	return out.Bytes()
}

// fenceMarker reports the fence char (backtick or tilde) and
// run length at the start of line after up to three leading
// spaces, or (0, 0) if the line is not a fence candidate — i.e.
// the first non-space char is not a backtick or tilde, or the
// run length is less than three.
func fenceMarker(line []byte) (byte, int) {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) {
		return 0, 0
	}
	c := line[i]
	if c != '`' && c != '~' {
		return 0, 0
	}
	count := 0
	for i+count < len(line) && line[i+count] == c {
		count++
	}
	if count < 3 {
		return 0, 0
	}
	return c, count
}

// fenceLineEmptyAfter reports whether the part of line that
// follows the fence run (skipping up to three leading spaces
// plus runLen marker chars) is blank. CommonMark allows an info
// string after an opener but requires a closer to be followed
// only by spaces — this guard keeps a code line that happens
// to start with a backtick run inside a fence from prematurely
// closing it.
func fenceLineEmptyAfter(line []byte, runLen int) bool {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	return len(bytes.TrimSpace(line[i+runLen:])) == 0
}

// BuildWebsite prepares the Hugo content tree: optionally runs
// `mdsmith fix` against srcDir so every <?catalog?>/<?include?>
// body is current, snapshots srcDir into dstDir via SyncDocs,
// then publishes the rule directory (internal/rules/index.md) as
// the browsable Rules section under dstDir and copies each
// per-rule README as its own website page. It is the canonical
// implementation behind the website sync — both local dev and the
// pages-deploy workflow call this rather than carrying the
// fix+sync sequence as inline shell (see
// docs/development/release-tooling.md: every workflow that needs
// runtime logic goes through this binary).
//
// The fix step shells out to `go run ./cmd/mdsmith` through the
// Runner seam, mirroring how BuildWheels invokes python; it
// expects the caller's working directory to be the repo root
// (true in CI and for the documented local invocation).
func (t *Toolkit) BuildWebsite(srcDir, dstDir string, runFix bool) error {
	if runFix {
		if err := t.runner.RunCommand("", "go", "run", "./cmd/mdsmith", "fix", srcDir); err != nil {
			return fmt.Errorf("mdsmith fix %s: %w", srcDir, err)
		}
	}
	// SyncDocs already contextualizes every failure path
	// (same-path guard, source not found, wipe/mkdir dst,
	// sync src -> dst), so it is returned unwrapped to keep
	// messages from doubling up.
	if err := t.SyncDocs(srcDir, dstDir); err != nil {
		return err
	}
	// The rule directory lives under internal/rules/, a sibling
	// of the docs/ source tree, not inside it — so SyncDocs never
	// sees it. Publish it as its own section after the docs
	// snapshot (SyncDocs wiped dstDir, so this must run last).
	rulesDir := filepath.Join(filepath.Dir(filepath.Clean(srcDir)), "internal", "rules")
	if err := t.syncRuleIndex(rulesDir, dstDir); err != nil {
		return err
	}
	return t.syncRulePages(rulesDir, dstDir)
}

// syncRuleIndex snapshots the rule directory index
// (rulesDir/index.md) into dstDir/rules/_index.md so the full MDS
// catalog is a browsable page on the site. The same docs→Hugo
// transforms SyncDocs applies (title lift, directive-marker strip)
// are applied here, then each rule-README relative link is
// rewritten to the corresponding local site page URL (e.g.
// `MDS001-line-length/README.md` → `MDS001-line-length/`) so the
// links stay on-site rather than bouncing to GitHub.
//
// A cascade front-matter block is injected so Hugo applies the
// `rule` layout type to all child pages, enabling category and
// status metadata to be displayed in the per-rule template.
//
// A missing rule index is not an error: callers that point
// BuildWebsite at a docs tree with no sibling internal/rules/
// (every BuildWebsite unit test, say) simply get no Rules section.
//
// index.md is located via ReadDir and skipped if it is a symlink,
// mirroring SyncDocs' symlink handling: following a link planted
// at internal/rules/index.md would let build-website publish an
// arbitrary runner file into the Hugo tree. A symlinked (or
// absent) index.md is treated the same as no rule index.
func (t *Toolkit) syncRuleIndex(rulesDir, dstDir string) error {
	entries, err := t.fs.ReadDir(rulesDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read rule dir %s: %w", rulesDir, err)
	}
	var found bool
	for _, e := range entries {
		if e.Name() != "index.md" {
			continue
		}
		// Skip a symlinked index.md (DirEntry.Type reports
		// ModeSymlink for a link regardless of its target).
		if e.Type()&fs.ModeSymlink == 0 {
			found = true
		}
		break
	}
	if !found {
		return nil
	}
	// ReadDir already confirmed a non-symlink index.md, so a
	// ReadFile failure here is a real error (or a TOCTOU
	// disappearance) — wrap it rather than special-casing
	// not-exist.
	src := filepath.Join(rulesDir, "index.md")
	data, err := t.fs.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read rule index %s: %w", src, err)
	}
	data = transformMarkdown(data)
	// Rewrite `MDS001-line-length/README.md` → `MDS001-line-length/`
	// so links stay on-site rather than pointing at GitHub.
	// Skip code regions so a documented `MDS…/README.md` example
	// stays verbatim.
	data = applyOutsideCode(data, func(seg []byte) []byte {
		return ruleReadmeLink.ReplaceAll(seg, []byte("]($1/${2}${3})"))
	})
	// Cascade the `rule` layout type to all child pages so Hugo
	// picks up the per-rule template for category/status display.
	data = injectFMField(data, "cascade:\n  type: rule")
	dstRulesDir := filepath.Join(dstDir, "rules")
	if err := t.fs.MkdirAll(dstRulesDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dstRulesDir, err)
	}
	dst := filepath.Join(dstRulesDir, "_index.md")
	if err := t.fs.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write rule index %s: %w", dst, err)
	}
	return nil
}

// syncRulePages copies each per-rule README
// (rulesDir/MDS###-name/README.md) into
// dstDir/rules/MDS###-name/index.md as a standalone Hugo page.
// The same docs→Hugo transforms applied by SyncDocs (title lift,
// directive-marker strip, Hugo shortcode escaping) are applied to
// each README. A `github_source` front-matter field recording the
// repo-relative path to the rule's source directory is injected so
// the rule layout template can link back to GitHub.
//
// Symlinked rule directories are skipped (same rationale as
// SyncDocs). Directories whose name does not start with "MDS" are
// skipped (guard against helper or fixture directories). A rule
// directory with no README.md is silently skipped rather than
// failing, so a partially-authored rule does not block the build.
//
// A missing rules directory is not an error — same as
// syncRuleIndex.
func (t *Toolkit) syncRulePages(rulesDir, dstDir string) error {
	entries, err := t.fs.ReadDir(rulesDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read rule dir %s: %w", rulesDir, err)
	}
	dstRulesDir := filepath.Join(dstDir, "rules")
	for _, e := range entries {
		if e.Type()&fs.ModeSymlink != 0 || !e.IsDir() {
			continue
		}
		if !ruleDirName.MatchString(e.Name()) {
			continue
		}
		readmeSrc := filepath.Join(rulesDir, e.Name(), "README.md")
		data, err := t.fs.ReadFile(readmeSrc)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return fmt.Errorf("read rule README %s: %w", readmeSrc, err)
		}
		ruleName := e.Name()
		data = transformRulePage(transformMarkdown(data), ruleName)
		ruleDst := filepath.Join(dstRulesDir, ruleName)
		if err := t.fs.MkdirAll(ruleDst, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", ruleDst, err)
		}
		dst := filepath.Join(ruleDst, "index.md")
		if err := t.fs.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("write rule page %s: %w", dst, err)
		}
	}
	return nil
}

// transformRulePage applies all rule-page link rewrites to data,
// which is a single rule README's content (already through
// transformMarkdown). The pass runs under applyOutsideCode so
// code regions in rule READMEs — MDS021 demoing
// `[rules](../internal/rules/)` in an inline code span, MDS027
// demoing `[guide](good/guide.md)` in a fenced block, etc. —
// pass through as documentation text rather than being
// rewritten alongside real link targets. It also rewrites the
// `[source](./)` self-link and injects the `github_source`
// front-matter field.
func transformRulePage(data []byte, ruleName string) []byte {
	ruleSourceFiles := githubBlobBase + "internal/rules/" + ruleName + "/"
	ruleSourceDir := ruleSourceTreeBase + ruleName + "/"
	data = applyOutsideCode(data, func(seg []byte) []byte {
		seg = ruleReadmeLink.ReplaceAll(seg, []byte("]($1/${2}${3})"))
		seg = ruleRefDefLink.ReplaceAll(seg, []byte("$1/$3"))
		seg = repoDocsLink.ReplaceAll(seg, []byte("](/$1/$2)"))
		seg = repoPlanLink.ReplaceAll(seg, []byte("]("+githubBlobBase+"plan/$1)"))
		seg = repoPlanRefDef.ReplaceAll(seg, []byte("${1}"+githubBlobBase+"plan/$2"))
		seg = rewriteRuleFixtures(seg, ruleSourceFiles, ruleSourceDir)
		seg = ruleRelativeNonMDSLink.ReplaceAllFunc(seg, rewriteRuleRelativeInline)
		seg = ruleRelativeNonMDSRefDef.ReplaceAllFunc(seg, rewriteRuleRelativeRefDef)
		return seg
	})
	data = bytes.ReplaceAll(data,
		[]byte("[source](./)"),
		[]byte("[source]("+ruleSourceDir+")"))
	return injectFMField(data, "github_source: internal/rules/"+ruleName+"/")
}

// rewriteRuleFixtures rewrites fixture references
// (good/default.md, bad/x.md, pattern/good/) to the rule's
// GitHub URL: /blob/ for file targets, /tree/ for directory
// targets. Fixtures are not republished on the site, so a
// repo-relative link 404s.
func rewriteRuleFixtures(seg []byte, fileBase, dirBase string) []byte {
	return ruleFixtureLink.ReplaceAllFunc(seg, func(match []byte) []byte {
		m := ruleFixtureLink.FindSubmatch(match)
		base := fileBase
		if bytes.HasSuffix(m[1], []byte("/")) {
			base = dirBase
		}
		return []byte("](" + base + string(m[1]) + string(m[2]) + ")")
	})
}

// rewriteRuleRelativeInline rewrites a `(?:\.\./)+<non-MDS>`
// inline link from a rule README to its GitHub URL. Depth maps
// to the repo-rooted prefix that the `../` walk lands on from
// internal/rules/<rule>/ (1 → internal/rules/, 2 → internal/).
// Higher depths are declined — those are repo-rooted links that
// the earlier rewriteRuleLinks pass already handled, and matching
// them here would either double-rewrite a URL the earlier pass
// emitted or wrongly map a `../../../<x>` repo-root reference
// into an internal/-rooted GitHub URL. The trailing linkTitleTail
// (m[3]) is re-emitted so a titled link keeps its title.
func rewriteRuleRelativeInline(match []byte) []byte {
	m := ruleRelativeNonMDSLink.FindSubmatch(match)
	prefix, ok := repoPrefixForRuleRelativeDepth(m[1])
	if !ok {
		return match
	}
	return []byte("](" +
		githubURLForPath([]byte(prefix+string(m[2]))) +
		string(m[3]) + ")")
}

// rewriteRuleRelativeRefDef is the reference-style sibling of
// rewriteRuleRelativeInline. m[1] is the `[label]: ` prefix, m[2]
// the `(?:\.\./)+` depth capture, m[3] the non-MDS path, and m[4]
// the optional title re-emitted so the definition keeps it.
func rewriteRuleRelativeRefDef(match []byte) []byte {
	m := ruleRelativeNonMDSRefDef.FindSubmatch(match)
	prefix, ok := repoPrefixForRuleRelativeDepth(m[2])
	if !ok {
		return match
	}
	return []byte(string(m[1]) +
		githubURLForPath([]byte(prefix+string(m[3]))) +
		string(m[4]))
}

// repoPrefixForRuleRelativeDepth maps a `(?:\.\./)+` capture to
// the repo-rooted directory it lands on when walked up from
// internal/rules/<rule>/. The two-level structure makes only
// depths 1 and 2 well-defined within this rewriter; depth 3+
// reaches the repo root and is the territory of the earlier
// rewriters (repoDocsLink, repoPlanLink, repoNonPublishedLink).
// Returns ok=false outside that range so the caller can leave the
// match untouched.
func repoPrefixForRuleRelativeDepth(dotdot []byte) (string, bool) {
	switch bytes.Count(dotdot, []byte("../")) {
	case 1:
		return "internal/rules/", true
	case 2:
		return "internal/", true
	default:
		return "", false
	}
}

// injectFMField inserts a YAML field block into a document's front
// matter. If the document has no front matter, one is synthesized.
// If the front matter is malformed (opening delimiter with no
// close), the document is returned byte-for-byte unchanged.
// field must be valid YAML without a trailing newline.
func injectFMField(data []byte, field string) []byte {
	fm, body, hasFM, ok := splitDocFrontMatter(string(data))
	if !ok {
		return data
	}
	if hasFM {
		return []byte("---\n" + strings.TrimRight(fm, "\n") + "\n" + field + "\n---\n" + body)
	}
	return []byte("---\n" + field + "\n---\n" + body)
}

// BuildWebsite delegates to a default-OS Toolkit (see Stamp).
func BuildWebsite(srcDir, dstDir string, runFix bool) error {
	return New().BuildWebsite(srcDir, dstDir, runFix)
}
