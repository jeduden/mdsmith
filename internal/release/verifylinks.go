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
			name: "sibling .md resolves to target permalink",
			path: "development/merge-queue/index.html",
			wantMatch: regexp.MustCompile(
				hrefEq + q(prefix) + `/development/pr-fixup-workflow/`),
		},
		{
			name: "index.md drop resolves to section URL on leaf page",
			path: "development/architecture-audit/index.html",
			wantMatch: regexp.MustCompile(
				hrefEq + q(prefix) + `/development/architecture/`),
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
