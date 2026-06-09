package release

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// VerifyWebsiteLinks runs a fixed set of probes against the
// rendered HTML produced by `hugo --minify ...`. Each probe
// targets a behavior the render-link hook is responsible for:
// `.md` → permalink resolution, `index.md` → section URL,
// no `README.md` hrefs in rule pages, javascript:/data:
// hrefs neutralized by html/template, and the baseURL prefix
// appearing on site-absolute hrefs when one was supplied at
// build time. None of these are visible to the synced-tree
// mdsmith check (it walks the markdown filesystem
// pre-render), so without these probes a regression in the
// render-link hook ships silently.
//
// htmlDir is the Hugo output root (`public/` under
// website/). baseURL is the URL Hugo was built with; the
// path portion (e.g. `/mdsmith` for project-pages, empty
// for root deploys) is treated as the expected path prefix
// on every site-absolute href. Each probe's regex accepts
// both `href="value"` (Hugo's default) and `href=value`
// (--minify), so the function is robust whether or not the
// caller passed `--minify`.
//
// Probes fail closed with a single returned error naming
// the probe and the file that failed; subsequent probes
// are not run.
func VerifyWebsiteLinks(htmlDir, baseURL string) error {
	prefix, err := pathPrefixFromBaseURL(baseURL)
	if err != nil {
		return fmt.Errorf("parse base url: %w", err)
	}
	for _, p := range websiteLinkProbes(prefix) {
		if err := runWebsiteLinkProbe(htmlDir, p); err != nil {
			return err
		}
	}
	if err := verifyHomeHrefs(htmlDir, prefix); err != nil {
		return err
	}
	return verifyBreadcrumbs(htmlDir, prefix)
}

// homeLinkRe matches one anchor on the homepage, capturing its
// href whether quoted (Hugo's default) or unquoted (--minify).
// Other attributes may precede href (class on buttons and cards),
// so the prefix is lazy.
var homeLinkRe = regexp.MustCompile(`<a\s[^>]*?href="?([^"\s>]+)"?`)

// verifyHomeHrefs resolves every site-internal <a href> on the
// rendered homepage to a page on disk. The homepage is assembled
// almost entirely from templates (hero, nav, feature cards,
// footer), so its hardcoded hrefs bypass both the markdown-side
// link rules and the render-link hook that docs pages get — a
// renamed or pruned guide would otherwise rot a homepage link
// silently. External and protocol-relative links are skipped;
// each remaining site-absolute href resolves under the same rules
// breadcrumb crumbs use.
func verifyHomeHrefs(htmlDir, prefix string) error {
	data, err := readHTMLFile(filepath.Join(htmlDir, "index.html"))
	if err != nil {
		return fmt.Errorf("verify homepage hrefs: %w", err)
	}
	for _, m := range homeLinkRe.FindAllSubmatch(data, -1) {
		href := string(m[1])
		if !strings.HasPrefix(href, "/") || strings.HasPrefix(href, "//") {
			continue
		}
		if err := resolveCrumbHref(htmlDir, prefix, href); err != nil {
			return fmt.Errorf("verify homepage hrefs: %w", err)
		}
	}
	return nil
}

// breadcrumbNavRe captures the inner HTML of the docs-breadcrumb
// nav (partials/breadcrumb.html). The class attribute's quotes are
// optional so the probe matches both Hugo's default output and
// `hugo --minify`, which drops quotes around values that need none.
var breadcrumbNavRe = regexp.MustCompile(`(?s)<nav class="?docs-breadcrumb"?>(.*?)</nav>`)

// breadcrumbLinkRe matches one linked crumb `<a href=…>label</a>`,
// capturing the href (group 1) and the visible label (group 2).
// Labels are template-escaped page titles, so they never contain a
// literal `<`.
var breadcrumbLinkRe = regexp.MustCompile(`<a href="?([^"\s>]+)"?>([^<]*)</a>`)

// breadcrumbCurrentRe matches the trailing current-page crumb — the
// only attribute-less `<span>` in the nav (the separators all carry
// `class="sep"`).
var breadcrumbCurrentRe = regexp.MustCompile(`<span>([^<]*)</span>`)

// verifyBreadcrumbs walks every rendered index.html under htmlDir
// and validates the docs-breadcrumb trail on each page that has one.
// The partial builds the trail from the page's URL segments, so
// three invariants must hold:
//
//   - No two consecutive crumb labels are equal. A section titled
//     the same as a child directory renders the same label twice in
//     a row (the doubled "Concepts" report).
//   - The crumb count below the "mdsmith" home crumb equals the
//     page's URL depth, so no directory tier is skipped (the
//     reference/cli/ command pages dropped the "CLI Reference" tier
//     under the former .Ancestors trail).
//   - Every linked crumb resolves to a rendered page on disk.
//
// Pages without a breadcrumb (the homepage, 404) are skipped. prefix
// is the baseURL path component, trimmed from each href before it is
// resolved against htmlDir.
func verifyBreadcrumbs(htmlDir, prefix string) error {
	return filepath.WalkDir(htmlDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("verify breadcrumbs: walk %s: %w", path, walkErr)
		}
		if d.IsDir() || filepath.Base(path) != "index.html" {
			return nil
		}
		data, err := readHTMLFile(path)
		if err != nil {
			return fmt.Errorf("verify breadcrumbs: %w", err)
		}
		return checkPageBreadcrumb(htmlDir, prefix, path, data)
	})
}

// checkPageBreadcrumb validates the single page at file. It returns
// nil when the page carries no breadcrumb nav.
func checkPageBreadcrumb(htmlDir, prefix, file string, data []byte) error {
	nav := breadcrumbNavRe.FindSubmatch(data)
	if nav == nil {
		return nil
	}
	links := breadcrumbLinkRe.FindAllSubmatch(nav[1], -1)
	labels := make([]string, 0, len(links)+1)
	for _, m := range links {
		labels = append(labels, string(m[2]))
	}
	if cur := breadcrumbCurrentRe.FindSubmatch(nav[1]); cur != nil {
		labels = append(labels, string(cur[1]))
	}
	if len(labels) == 0 {
		return fmt.Errorf("verify breadcrumbs: empty breadcrumb in %s", file)
	}

	for i := 1; i < len(labels); i++ {
		if labels[i] == labels[i-1] {
			return fmt.Errorf(
				"verify breadcrumbs: duplicate consecutive crumb %q in %s",
				labels[i], file)
		}
	}

	depth, err := breadcrumbURLDepth(htmlDir, file)
	if err != nil {
		return err
	}
	if got := len(labels) - 1; got != depth {
		return fmt.Errorf(
			"verify breadcrumbs: %s has %d crumbs below home but URL depth is %d "+
				"(a directory tier is missing or doubled)", file, got, depth)
	}

	for _, m := range links {
		if err := resolveCrumbHref(htmlDir, prefix, string(m[1])); err != nil {
			return fmt.Errorf("verify breadcrumbs: %s: %w", file, err)
		}
	}
	return nil
}

// breadcrumbURLDepth returns the number of URL path segments for the
// page rendered at file. Hugo emits each page as
// `<segment>/.../index.html`, so the depth is the count of path
// components between htmlDir and the index.html. The site root's
// index.html has depth 0.
func breadcrumbURLDepth(htmlDir, file string) (int, error) {
	rel, err := filepath.Rel(htmlDir, file)
	if err != nil {
		return 0, fmt.Errorf("verify breadcrumbs: relpath %s: %w", file, err)
	}
	rel = strings.Trim(strings.TrimSuffix(filepath.ToSlash(rel), "index.html"), "/")
	if rel == "" {
		return 0, nil
	}
	return len(strings.Split(rel, "/")), nil
}

// resolveCrumbHref confirms a breadcrumb href points at a rendered
// page under htmlDir. The baseURL path prefix is trimmed first, then
// the site-absolute path maps to `<htmlDir>/<path>/index.html` (the
// root "/" maps to `<htmlDir>/index.html`). Fragment and query tails
// are dropped. An external href (a scheme or a protocol-relative
// `//host`) is accepted without a filesystem check — breadcrumbs are
// internal, but the guard keeps a stray external crumb from being
// reported as a missing local page.
func resolveCrumbHref(htmlDir, prefix, href string) error {
	if strings.Contains(href, "://") || strings.HasPrefix(href, "//") {
		return nil
	}
	if i := strings.IndexAny(href, "#?"); i >= 0 {
		href = href[:i]
	}
	if prefix != "" {
		href = strings.TrimPrefix(href, prefix)
	}
	rel := strings.Trim(href, "/")
	target := filepath.Join(htmlDir, filepath.FromSlash(rel), "index.html")
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("crumb href %q resolves to missing page %s", href, target)
	}
	return nil
}

// linkProbe describes one rendered-HTML assertion. Exactly
// one of wantMatch, wantNoMatch, or wantAnyMatch is set:
//
//   - wantMatch (non-recursive): the single file at path
//     must contain a regex match.
//   - wantNoMatch (recursive): no file under path may
//     match — used for absence checks (no leaked README.md
//     hrefs, no javascript: schemes, …).
//   - wantAnyMatch (recursive): at least one file under
//     path must match — used to assert a render-link
//     behavior is reachable in the rendered output without
//     tying the probe to one specific docs page that a
//     legitimate edit could remove.
type linkProbe struct {
	name         string
	path         string
	wantMatch    *regexp.Regexp
	wantNoMatch  *regexp.Regexp
	wantAnyMatch *regexp.Regexp
	recursive    bool
}

// websiteLinkProbes returns the probes that VerifyWebsiteLinks
// runs against the rendered output. Built fresh per call so
// the captured prefix lands in each regex; prefix is the
// site-path component of the build's baseURL (`/mdsmith`
// for a project-pages deploy, "" for a root deploy).
func websiteLinkProbes(prefix string) []linkProbe {
	q := regexp.QuoteMeta
	hrefEq := `href="?` // allow both quoted (default) and unquoted (minified) emission
	return []linkProbe{
		{
			// A `.md` content link must render to the target
			// page's clean permalink (trailing slash, no `.md`).
			// reference/index.md links its sibling `cli.md`, which
			// Hugo serves at /reference/cli/. Pinned to a stable
			// user-facing page; the previous form pointed at a
			// docs/development/ page that the maintainer-doc prune
			// removed from the published site.
			name: "sibling .md resolves to target permalink",
			path: "reference/index.html",
			wantMatch: regexp.MustCompile(
				hrefEq + q(prefix) + `/reference/cli/`),
		},
		{
			// The rewriter emits site-absolute `/rules/<id>/`
			// targets for every cross-rule and docs-to-rule link
			// (the synced docs tree is mounted at the site root,
			// so there is no `/docs` segment). The render-link
			// hook manually prefixes those with
			// site.Home.RelPermalink so the rendered href carries
			// the baseURL's path component (empty on root
			// deploys, `/<repo>` on project-pages). The id is
			// lowercased to match Hugo's case-folded page URL
			// (the source dir is MDS…; the served page is mds…),
			// so the probe asserts the lowercased form — an
			// uppercase regression would fail it. A recursive
			// wantAnyMatch keeps the probe robust to legitimate
			// docs edits — any rendered page that carries one
			// such href satisfies the assertion, so removing a
			// single content reference does not block the deploy.
			name: "site-absolute /rules/ href carries baseURL prefix",
			path: ".",
			wantAnyMatch: regexp.MustCompile(
				hrefEq + q(prefix) + `/rules/mds[0-9a-z._-]+/`),
			recursive: true,
		},
		{
			name: "no README.md hrefs leaked into rule pages",
			path: "rules",
			wantNoMatch: regexp.MustCompile(
				`href=(?:"[^"]*README\.md|[^ ">]*README\.md)`),
			recursive: true,
		},
		{
			// URL schemes are case-insensitive per RFC 3986 — a
			// rendered `href="JavaScript:..."` is just as
			// dangerous as the lowercase form. `(?i)` makes the
			// regex case-fold so the probe catches both.
			name:        "no javascript: hrefs reached rendered HTML",
			path:        ".",
			wantNoMatch: regexp.MustCompile(`(?i)href=(?:"javascript:|javascript:)`),
			recursive:   true,
		},
		{
			name:        "no data: hrefs reached rendered HTML",
			path:        ".",
			wantNoMatch: regexp.MustCompile(`(?i)href=(?:"data:|data:)`),
			recursive:   true,
		},
	}
}

// runWebsiteLinkProbe evaluates one probe. Recursive probes
// walk the subtree at p.path looking for either a forbidden
// match (wantNoMatch) or at least one allowed match
// (wantAnyMatch). Non-recursive probes read the single file
// at p.path and require it to match p.wantMatch. Splitting
// the modes keeps each branch reachable from at least one
// test.
func runWebsiteLinkProbe(root string, p linkProbe) error {
	target := filepath.Join(root, p.path)
	if p.recursive {
		if p.wantAnyMatch != nil {
			return walkAndRequireAny(target, p)
		}
		return walkAndReject(target, p)
	}
	data, err := readHTMLFile(target)
	if err != nil {
		return fmt.Errorf("verify %q: %w", p.name, err)
	}
	if !p.wantMatch.Match(data) {
		return fmt.Errorf("verify %q: no match for %s in %s",
			p.name, p.wantMatch, target)
	}
	return nil
}

// walkAndReject walks every .html file under target and
// returns an error on the first match of p.wantNoMatch. The
// WalkDir-supplied err is propagated so a broken symlink or
// a missing target root surfaces with the same wrapping
// readHTMLFile would produce on a single-file probe.
func walkAndReject(target string, p linkProbe) error {
	return filepath.WalkDir(target, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("verify %q: walk %s: %w", p.name, path, walkErr)
		}
		if d.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}
		data, err := readHTMLFile(path)
		if err != nil {
			return fmt.Errorf("verify %q: %w", p.name, err)
		}
		if p.wantNoMatch.Match(data) {
			return fmt.Errorf("verify %q: unwanted match for %s in %s",
				p.name, p.wantNoMatch, path)
		}
		return nil
	})
}

// walkAndRequireAny walks every .html file under target and
// returns nil as soon as one matches p.wantAnyMatch. If no
// file matches, returns a single error naming the regex and
// the searched root. Walk errors propagate with the same
// wrapping walkAndReject uses.
func walkAndRequireAny(target string, p linkProbe) error {
	var matched bool
	walkErr := filepath.WalkDir(target, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("verify %q: walk %s: %w", p.name, path, err)
		}
		if matched || d.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}
		data, readErr := readHTMLFile(path)
		if readErr != nil {
			return fmt.Errorf("verify %q: %w", p.name, readErr)
		}
		if p.wantAnyMatch.Match(data) {
			matched = true
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	if !matched {
		return fmt.Errorf("verify %q: no file under %s matched %s",
			p.name, target, p.wantAnyMatch)
	}
	return nil
}

// readHTMLFile reads an HTML file and wraps a missing-file
// error with a clearer message so the probe failure points
// at the rendered tree rather than a generic open error.
// Reads through os.ReadFile directly — VerifyWebsiteLinks
// runs only against a real Hugo output tree on disk, so
// there is no Toolkit fs seam to thread through here.
func readHTMLFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("rendered HTML not found: %s", path)
	}
	return data, err
}

// pathPrefixFromBaseURL returns the URL path component of
// baseURL, with a trailing slash trimmed so the caller can
// concatenate `/rules/...` without producing `//rules/...`.
// An empty baseURL or a root-deploy baseURL
// (`https://example.com/`) yields "". A project-pages
// baseURL (`https://example.com/repo/`) yields `/repo`.
func pathPrefixFromBaseURL(baseURL string) (string, error) {
	if baseURL == "" {
		return "", nil
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(u.Path, "/"), nil
}
