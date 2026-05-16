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

// linkProbe describes one rendered-HTML assertion. Either
// wantMatch or wantNoMatch is set, never both; whichever is
// non-nil drives the check at that path. recursive=true
// scans every .html file under path; recursive=false reads
// the single file at path.
type linkProbe struct {
	name        string
	path        string
	wantMatch   *regexp.Regexp
	wantNoMatch *regexp.Regexp
	recursive   bool
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
			path: "docs/development/merge-queue/index.html",
			wantMatch: regexp.MustCompile(
				hrefEq + q(prefix) + `/docs/development/pr-fixup-workflow/`),
		},
		{
			name: "index.md drop resolves to section URL on leaf page",
			path: "docs/development/architecture-audit/index.html",
			wantMatch: regexp.MustCompile(
				hrefEq + q(prefix) + `/docs/development/architecture/`),
		},
		{
			// The rewriter emits site-absolute `/docs/rules/<id>/`
			// targets for every cross-rule and docs-to-rule link.
			// The render-link hook routes those through relURL so
			// the rendered href carries the baseURL's path prefix
			// (empty for root deploys). Without this probe a
			// regression in relURL would slip past the two probes
			// above, which exercise only the GetPage branch.
			name: "site-absolute /docs/rules/ href carries baseURL prefix",
			path: "docs/reference/schema-types/index.html",
			wantMatch: regexp.MustCompile(
				hrefEq + q(prefix) + `/docs/rules/MDS020-required-structure/`),
		},
		{
			name: "no README.md hrefs leaked into rule pages",
			path: "docs/rules",
			wantNoMatch: regexp.MustCompile(
				`href=(?:"[^"]*README\.md|[^ ">]*README\.md)`),
			recursive: true,
		},
		{
			name:        "no javascript: hrefs reached rendered HTML",
			path:        ".",
			wantNoMatch: regexp.MustCompile(`href=(?:"javascript:|javascript:)`),
			recursive:   true,
		},
		{
			name:        "no data: hrefs reached rendered HTML",
			path:        ".",
			wantNoMatch: regexp.MustCompile(`href=(?:"data:|data:)`),
			recursive:   true,
		},
	}
}

// runWebsiteLinkProbe evaluates one probe. Recursive probes
// walk the subtree at p.path looking for any file that
// matches p.wantNoMatch (every recursive probe today is an
// absence check). Non-recursive probes read the single file
// at p.path and require it to match p.wantMatch. Splitting
// the two modes keeps each branch reachable from at least
// one test.
func runWebsiteLinkProbe(root string, p linkProbe) error {
	target := filepath.Join(root, p.path)
	if p.recursive {
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
// concatenate `/docs/...` without producing `//docs/...`.
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
