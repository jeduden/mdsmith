package release

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// goodSite materializes a minimal Hugo output tree matching
// every assertion VerifyWebsiteLinks runs. Each test below
// starts from this corpus and mutates one file to break a
// single probe, so each failing case is isolated to the
// regex it targets. The synced docs tree is mounted at the
// site root, so pages render at `/<section>/...` with no
// `/docs` segment.
func goodSite(t *testing.T, prefix string) string {
	t.Helper()
	root := t.TempDir()
	// A .md content link rendered to a clean doc permalink
	// (reference/index.md links its sibling cli.md, served at
	// /reference/cli/) — satisfies the sibling-.md probe.
	writeFile(t, filepath.Join(root, "reference", "index.html"),
		`<a href="`+prefix+`/reference/cli/">cli</a>`)
	writeFile(t, filepath.Join(root, "reference", "schema-types", "index.html"),
		`<a href="`+prefix+`/rules/mds020-required-structure/">rule</a>`)
	writeFile(t, filepath.Join(root, "rules", "mds001", "index.html"),
		`<a href="`+prefix+`/rules/mds021/">sibling rule</a>`)
	writeFile(t, filepath.Join(root, "index.html"), `<p>home</p>`)
	return root
}

func TestVerifyWebsiteLinks_RootDeployPasses(t *testing.T) {
	root := goodSite(t, "")
	require.NoError(t, VerifyWebsiteLinks(root, ""))
}

func TestVerifyWebsiteLinks_SubpathDeployPasses(t *testing.T) {
	root := goodSite(t, "/mdsmith")
	require.NoError(t, VerifyWebsiteLinks(root, "https://example.com/mdsmith/"))
}

// TestVerifyWebsiteLinks_AcceptsUnquotedHref pins the bug
// fix from PR #309 review: `hugo --minify` drops the
// double-quote around href values when the URL contains no
// characters that require quoting. The probe regexes must
// match `href=value` as well as `href="value"`.
func TestVerifyWebsiteLinks_AcceptsUnquotedHref(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "reference", "index.html"),
		`<a href=/reference/cli/>cli</a>`)
	writeFile(t, filepath.Join(root, "reference", "schema-types", "index.html"),
		`<a href=/rules/mds020-required-structure/>rule</a>`)
	writeFile(t, filepath.Join(root, "rules", "mds001", "index.html"),
		`<a href=/rules/mds021/>sibling</a>`)
	writeFile(t, filepath.Join(root, "index.html"), `<p>home</p>`)
	require.NoError(t, VerifyWebsiteLinks(root, ""))
}

// TestVerifyWebsiteLinks_FailsOnMissingSiblingMD overwrites the
// pinned reference/index.html with a stale `.md` ref, so the
// non-recursive sibling-.md probe reads that one file and finds no
// permalink match.
func TestVerifyWebsiteLinks_FailsOnMissingSiblingMD(t *testing.T) {
	root := goodSite(t, "")
	writeFile(t, filepath.Join(root, "reference", "index.html"),
		`<a href="cli.md">stale .md ref</a>`)
	err := VerifyWebsiteLinks(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sibling .md")
}

func TestVerifyWebsiteLinks_FailsOnLeakedREADMEHref(t *testing.T) {
	root := goodSite(t, "")
	writeFile(t, filepath.Join(root, "rules", "mds999", "index.html"),
		`<a href="../MDS021-include/README.md">leaked</a>`)
	err := VerifyWebsiteLinks(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "README.md")
}

func TestVerifyWebsiteLinks_FailsOnQuotedREADMEHref(t *testing.T) {
	// The quoted form must be caught too — the original
	// inline-shell regex (`href=[^"]*README\.md`) could not
	// cross the opening quote.
	root := goodSite(t, "")
	writeFile(t, filepath.Join(root, "rules", "mds999", "index.html"),
		`<a href="../README.md">quoted leak</a>`)
	err := VerifyWebsiteLinks(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "README.md")
}

func TestVerifyWebsiteLinks_FailsOnJavascriptScheme(t *testing.T) {
	root := goodSite(t, "")
	writeFile(t, filepath.Join(root, "evil", "index.html"),
		`<a href="javascript:alert(1)">click</a>`)
	err := VerifyWebsiteLinks(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "javascript:")
}

// TestVerifyWebsiteLinks_FailsOnMixedCaseJavascript pins
// the case-insensitive scheme guard: URL schemes are
// case-insensitive per RFC 3986, so `JavaScript:` is the
// same dangerous scheme as `javascript:` and the probe
// must reject it too.
func TestVerifyWebsiteLinks_FailsOnMixedCaseJavascript(t *testing.T) {
	root := goodSite(t, "")
	writeFile(t, filepath.Join(root, "evil", "index.html"),
		`<a href="JavaScript:alert(1)">click</a>`)
	err := VerifyWebsiteLinks(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "javascript:")
}

// TestVerifyWebsiteLinks_FailsOnMissingSiteAbsolute exercises
// walkAndRequireAny's no-match path: the good fixture has the
// /rules/mdsxxx/ link, but stripping the prefix expectation
// means nothing matches under subpath baseURL.
func TestVerifyWebsiteLinks_FailsOnMissingSiteAbsolute(t *testing.T) {
	// Build a tree that has every required href except the
	// site-absolute /rules/mdsxxx/ form.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "reference", "index.html"),
		`<a href="/mdsmith/reference/cli/">x</a>`)
	// No MDS-rule href under any subpath.
	err := VerifyWebsiteLinks(root, "https://example.com/mdsmith/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/mdsmith/rules/")
}

func TestVerifyWebsiteLinks_FailsOnDataScheme(t *testing.T) {
	root := goodSite(t, "")
	writeFile(t, filepath.Join(root, "evil", "index.html"),
		`<a href="data:text/html,<script>1</script>">click</a>`)
	err := VerifyWebsiteLinks(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data:")
}

func TestVerifyWebsiteLinks_FailsOnMissingPrefix(t *testing.T) {
	root := goodSite(t, "") // built without prefix
	err := VerifyWebsiteLinks(root, "https://example.com/mdsmith/")
	require.Error(t, err)
	// The probe should mention the prefix it expected.
	assert.Contains(t, err.Error(), "/mdsmith/")
}

func TestVerifyWebsiteLinks_MissingTargetFileWraps(t *testing.T) {
	root := t.TempDir() // no merge-queue file
	err := VerifyWebsiteLinks(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rendered HTML not found")
}

// TestVerifyWebsiteLinks_InvalidBaseURLWraps drives the
// pathPrefixFromBaseURL error path. A URL with an unparsable
// scheme returns an error before the probes run.
func TestVerifyWebsiteLinks_InvalidBaseURLWraps(t *testing.T) {
	err := VerifyWebsiteLinks(t.TempDir(), "://invalid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse base url")
}

// TestVerifyWebsiteLinks_MissingRecursiveRootWraps drives
// the WalkDir-callback error branch in walkAndReject: the
// recursive probe root (rules) does not exist, so WalkDir
// calls the callback with a stat error.
func TestVerifyWebsiteLinks_MissingRecursiveRootWraps(t *testing.T) {
	root := t.TempDir()
	// Materialize only the non-recursive probe target plus a
	// page carrying the site-absolute rule href, so we reach
	// the recursive `no README.md leak` probe.
	writeFile(t, filepath.Join(root, "reference", "index.html"),
		`<a href="/reference/cli/">x</a>`)
	writeFile(t, filepath.Join(root, "reference", "schema-types", "index.html"),
		`<a href="/rules/mds020-required-structure/">x</a>`)
	// rules/ is absent.
	err := VerifyWebsiteLinks(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "walk")
}

// --- homepage href probes ---

// TestVerifyWebsiteLinks_HomeInternalHrefResolves accepts a
// homepage whose internal links point at rendered pages; external
// links and unquoted (--minify) hrefs are handled too.
func TestVerifyWebsiteLinks_HomeInternalHrefResolves(t *testing.T) {
	root := goodSite(t, "")
	writeFile(t, filepath.Join(root, "guides", "migrate-from-markdownlint", "index.html"),
		`<p>guide</p>`)
	writeFile(t, filepath.Join(root, "guides", "install", "index.html"),
		`<p>install</p>`)
	writeFile(t, filepath.Join(root, "index.html"),
		`<a class="hero-switch-link" href="/guides/migrate-from-markdownlint/">guide</a>`+
			`<a href=/guides/install/>install</a>`+
			`<a href="https://github.com/jeduden/mdsmith" rel="noopener">gh</a>`+
			`<a href="//cdn.example.com/x/">protocol-relative</a>`)
	require.NoError(t, VerifyWebsiteLinks(root, ""))
}

// TestVerifyWebsiteLinks_FailsOnMissingHomepage pins the probe's
// fail-closed behavior: a rendered tree without a homepage cannot
// pass, mirroring the single-file probes.
func TestVerifyWebsiteLinks_FailsOnMissingHomepage(t *testing.T) {
	root := goodSite(t, "")
	require.NoError(t, os.Remove(filepath.Join(root, "index.html")))
	err := VerifyWebsiteLinks(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify homepage hrefs")
}

// TestVerifyWebsiteLinks_FailsOnHomeHrefMissingTarget pins the
// link-rot guard: the homepage is assembled from templates (hero,
// nav, cards, footer), so its hardcoded hrefs bypass the markdown
// link rules — a renamed guide must fail the probe.
func TestVerifyWebsiteLinks_FailsOnHomeHrefMissingTarget(t *testing.T) {
	root := goodSite(t, "")
	writeFile(t, filepath.Join(root, "index.html"),
		`<a href="/guides/migrate-from-markdownlint/">moved guide</a>`)
	err := VerifyWebsiteLinks(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migrate-from-markdownlint")
}

// TestVerifyWebsiteLinks_HomeHrefSubpathResolves trims the
// project-pages baseURL prefix before resolving, mirroring the
// breadcrumb resolution rules.
func TestVerifyWebsiteLinks_HomeHrefSubpathResolves(t *testing.T) {
	root := goodSite(t, "/mdsmith")
	writeFile(t, filepath.Join(root, "guides", "install", "index.html"),
		`<p>install</p>`)
	writeFile(t, filepath.Join(root, "index.html"),
		`<a href="/mdsmith/guides/install/">install</a>`)
	require.NoError(t, VerifyWebsiteLinks(root, "https://example.com/mdsmith/"))
}

// --- breadcrumb probes ---

// crumb renders one breadcrumb entry; an empty href marks the
// trailing current-page crumb (a bare <span>).
func crumb(label, href string) string {
	if href == "" {
		return "<span>" + label + "</span>"
	}
	return `<a href="` + href + `">` + label + "</a>"
}

// crumbNav joins entries with separator spans into a docs-breadcrumb
// nav, matching the markup partials/breadcrumb.html emits.
func crumbNav(entries ...string) string {
	return `<nav class="docs-breadcrumb">` +
		strings.Join(entries, `<span class="sep">/</span>`) + "</nav>"
}

// goodCrumbSite materializes a rendered tree whose breadcrumbs
// satisfy every verifyBreadcrumbs invariant: a home page (no
// breadcrumb), a section, a section-overview page, and a leaf one
// tier deeper. prefix is the baseURL path component carried on each
// href, mirroring Hugo's relURL output.
func goodCrumbSite(t *testing.T, prefix string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "index.html"), "<p>home</p>")
	writeFile(t, filepath.Join(root, "reference", "index.html"),
		crumbNav(crumb("mdsmith", prefix+"/"), crumb("Reference", "")))
	writeFile(t, filepath.Join(root, "reference", "cli", "index.html"),
		crumbNav(crumb("mdsmith", prefix+"/"),
			crumb("Reference", prefix+"/reference/"),
			crumb("CLI Reference", "")))
	writeFile(t, filepath.Join(root, "reference", "cli", "check", "index.html"),
		crumbNav(crumb("mdsmith", prefix+"/"),
			crumb("Reference", prefix+"/reference/"),
			crumb("CLI Reference", prefix+"/reference/cli/"),
			crumb("mdsmith check", "")))
	return root
}

func TestVerifyBreadcrumbs_GoodPasses(t *testing.T) {
	require.NoError(t, verifyBreadcrumbs(goodCrumbSite(t, ""), ""))
}

func TestVerifyBreadcrumbs_GoodPassesSubpath(t *testing.T) {
	require.NoError(t,
		verifyBreadcrumbs(goodCrumbSite(t, "/mdsmith"), "/mdsmith"))
}

// TestVerifyBreadcrumbs_FailsOnDuplicate pins the reported bug: a
// section titled the same as its child directory renders the same
// label twice in a row ("mdsmith / Concepts / Concepts / …").
func TestVerifyBreadcrumbs_FailsOnDuplicate(t *testing.T) {
	root := goodCrumbSite(t, "")
	writeFile(t, filepath.Join(root, "background", "concepts", "index.html"),
		crumbNav(crumb("mdsmith", "/"),
			crumb("Concepts", "/background/"),
			crumb("Concepts", "")))
	// Parent so the /background/ crumb href resolves.
	writeFile(t, filepath.Join(root, "background", "index.html"),
		crumbNav(crumb("mdsmith", "/"), crumb("Concepts", "")))
	err := verifyBreadcrumbs(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate consecutive crumb")
	assert.Contains(t, err.Error(), "Concepts")
}

// TestVerifyBreadcrumbs_FailsOnDepthMismatch pins the reference/cli/
// regression: a page three URL segments deep whose trail skips a
// directory tier has fewer crumbs than its depth.
func TestVerifyBreadcrumbs_FailsOnDepthMismatch(t *testing.T) {
	root := goodCrumbSite(t, "")
	// reference/cli/check is depth 3 but this trail omits the CLI tier.
	writeFile(t, filepath.Join(root, "reference", "cli", "check", "index.html"),
		crumbNav(crumb("mdsmith", "/"),
			crumb("Reference", "/reference/"),
			crumb("mdsmith check", "")))
	err := verifyBreadcrumbs(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "URL depth")
}

func TestVerifyBreadcrumbs_FailsOnBrokenLink(t *testing.T) {
	root := goodCrumbSite(t, "")
	writeFile(t, filepath.Join(root, "guides", "editors", "vscode", "index.html"),
		crumbNav(crumb("mdsmith", "/"),
			crumb("Guides", "/guides/"),
			crumb("Editors", "/guides/editors/"), // no such page rendered
			crumb("mdsmith for VS Code", "")))
	writeFile(t, filepath.Join(root, "guides", "index.html"),
		crumbNav(crumb("mdsmith", "/"), crumb("Guides", "")))
	err := verifyBreadcrumbs(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing page")
}

// TestVerifyBreadcrumbs_SkipsPagesWithoutBreadcrumb confirms a page
// with no docs-breadcrumb nav (homepage, 404) is ignored.
func TestVerifyBreadcrumbs_SkipsPagesWithoutBreadcrumb(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "index.html"), "<p>home, no nav</p>")
	writeFile(t, filepath.Join(root, "404.html"), "<p>not found</p>")
	require.NoError(t, verifyBreadcrumbs(root, ""))
}

// TestVerifyBreadcrumbs_AcceptsMinified pins the unquoted-attribute
// form `hugo --minify` emits, and confirms a duplicate is still
// caught in that form.
func TestVerifyBreadcrumbs_AcceptsMinified(t *testing.T) {
	good := t.TempDir()
	writeFile(t, filepath.Join(good, "index.html"), "<p>home</p>")
	writeFile(t, filepath.Join(good, "reference", "index.html"),
		`<nav class=docs-breadcrumb><a href=/>mdsmith</a>`+
			`<span class=sep>/</span><span>Reference</span></nav>`)
	require.NoError(t, verifyBreadcrumbs(good, ""))

	bad := t.TempDir()
	writeFile(t, filepath.Join(bad, "index.html"), "<p>home</p>")
	writeFile(t, filepath.Join(bad, "background", "index.html"),
		`<nav class=docs-breadcrumb><a href=/>mdsmith</a>`+
			`<span class=sep>/</span><span>x</span></nav>`)
	writeFile(t, filepath.Join(bad, "background", "concepts", "index.html"),
		`<nav class=docs-breadcrumb><a href=/>mdsmith</a>`+
			`<span class=sep>/</span><a href=/background/>Concepts</a>`+
			`<span class=sep>/</span><span>Concepts</span></nav>`)
	err := verifyBreadcrumbs(bad, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate consecutive crumb")
}

// TestVerifyBreadcrumbs_MissingRootWraps drives the WalkDir-error
// branch: a non-existent root surfaces a wrapped walk error.
func TestVerifyBreadcrumbs_MissingRootWraps(t *testing.T) {
	err := verifyBreadcrumbs(filepath.Join(t.TempDir(), "nope"), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "walk")
}

// TestVerifyBreadcrumbs_UnreadableMemberWraps drives the
// readHTMLFile-error branch: a dangling index.html symlink is a
// non-dir entry the walk descends to but cannot read.
func TestVerifyBreadcrumbs_UnreadableMemberWraps(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "page")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	link := filepath.Join(dir, "index.html")
	if err := os.Symlink(filepath.Join(dir, "no-such-target"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	err := verifyBreadcrumbs(filepath.Dir(dir), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rendered HTML not found")
}

// TestVerifyWebsiteLinks_RunsBreadcrumbCheck confirms the breadcrumb
// pass is wired into VerifyWebsiteLinks: a tree that satisfies every
// link probe but carries a doubled crumb still fails.
func TestVerifyWebsiteLinks_RunsBreadcrumbCheck(t *testing.T) {
	root := goodSite(t, "")
	writeFile(t, filepath.Join(root, "background", "index.html"),
		crumbNav(crumb("mdsmith", "/"), crumb("Concepts", "")))
	writeFile(t, filepath.Join(root, "background", "concepts", "index.html"),
		crumbNav(crumb("mdsmith", "/"),
			crumb("Concepts", "/background/"),
			crumb("Concepts", "")))
	err := VerifyWebsiteLinks(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate consecutive crumb")
}

func TestResolveCrumbHref(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "index.html"), "home")
	writeFile(t, filepath.Join(root, "reference", "index.html"), "ref")

	cases := []struct {
		name, prefix, href string
		wantErr            bool
	}{
		{"root", "", "/", false},
		{"section", "", "/reference/", false},
		{"missing", "", "/nope/", true},
		{"external", "", "https://example.com/x/", false},
		{"protocol relative", "", "//cdn/x/", false},
		{"fragment trimmed", "", "/reference/#output", false},
		{"subpath stripped", "/mdsmith", "/mdsmith/reference/", false},
		{"subpath missing", "/mdsmith", "/mdsmith/nope/", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := resolveCrumbHref(root, tc.prefix, tc.href)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "missing page")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCheckPageBreadcrumb_EmptyNavFails drives the empty-breadcrumb
// branch: a nav element with no crumbs at all.
func TestCheckPageBreadcrumb_EmptyNavFails(t *testing.T) {
	err := checkPageBreadcrumb(t.TempDir(), "", "x/index.html",
		[]byte(`<nav class="docs-breadcrumb"></nav>`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty breadcrumb")
}

// TestCheckPageBreadcrumb_DepthErrorWraps drives the
// depth-computation error branch: a relative htmlDir against an
// absolute file path makes filepath.Rel fail.
func TestCheckPageBreadcrumb_DepthErrorWraps(t *testing.T) {
	err := checkPageBreadcrumb("relative", "", "/abs/page/index.html",
		[]byte(crumbNav(crumb("mdsmith", "/"), crumb("X", ""))))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "relpath")
}

func TestBreadcrumbURLDepth(t *testing.T) {
	cases := []struct {
		name, root, file string
		want             int
		wantErr          bool
	}{
		{"root", "/site", "/site/index.html", 0, false},
		{"section", "/site", "/site/reference/index.html", 1, false},
		{"deep", "/site", "/site/reference/cli/check/index.html", 3, false},
		{"rel error", "relative", "/abs/x/index.html", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := breadcrumbURLDepth(tc.root, tc.file)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPathPrefixFromBaseURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"root no slash", "https://example.com", ""},
		{"root with slash", "https://example.com/", ""},
		{"project pages", "https://example.com/mdsmith/", "/mdsmith"},
		{"project pages no slash", "https://example.com/mdsmith", "/mdsmith"},
		{"nested", "https://example.com/foo/bar/", "/foo/bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pathPrefixFromBaseURL(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPathPrefixFromBaseURL_InvalidURL(t *testing.T) {
	_, err := pathPrefixFromBaseURL("://invalid")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "missing protocol") ||
		strings.Contains(err.Error(), "parse"),
		"err = %v", err)
}

// --- walkAndReject / walkAndRequireAny error branches ---

// TestWalkAndReject_MissingRootSurfacesWalkError pins the
// WalkDir-error branch in walkAndReject: pointing the probe at a
// path that does not exist must produce a wrapped error naming
// the probe and the failed path. The end-to-end Verify* tests
// only ever pass a real Hugo tree, so this branch was uncovered.
func TestWalkAndReject_MissingRootSurfacesWalkError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	err := walkAndReject(missing, linkProbe{name: "probe-x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "probe-x")
	assert.Contains(t, err.Error(), "walk")
}

// TestWalkAndRequireAny_MissingRootSurfacesWalkError mirrors the
// same WalkDir-error branch for the require-any variant.
func TestWalkAndRequireAny_MissingRootSurfacesWalkError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	err := walkAndRequireAny(missing, linkProbe{name: "probe-y"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "probe-y")
	assert.Contains(t, err.Error(), "walk")
}

// danglingHTML plants a symlink with a .html extension that points
// at a nonexistent target inside dir. WalkDir's lstat sees a
// non-directory .html entry, so the probe tries to read it and
// readHTMLFile fails — the unreadable-member branch that the
// missing-root tests above do not reach (those fail in WalkDir
// itself, before any read). Skipped where symlinks are unavailable.
func danglingHTML(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	link := filepath.Join(dir, "broken.html")
	if err := os.Symlink(filepath.Join(dir, "no-such-target"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

// TestWalkAndReject_UnreadableMemberSurfacesReadError drives the
// readHTMLFile-error branch inside walkAndReject: a dangling .html
// symlink is a non-dir entry the walk descends to but cannot read.
func TestWalkAndReject_UnreadableMemberSurfacesReadError(t *testing.T) {
	dir := t.TempDir()
	danglingHTML(t, dir)
	err := walkAndReject(dir, linkProbe{
		name:        "probe-reject",
		wantNoMatch: regexp.MustCompile(`never-matches`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "probe-reject")
	assert.Contains(t, err.Error(), "rendered HTML not found")
}

// TestWalkAndRequireAny_UnreadableMemberSurfacesReadError drives
// the same readHTMLFile-error branch for the require-any variant.
func TestWalkAndRequireAny_UnreadableMemberSurfacesReadError(t *testing.T) {
	dir := t.TempDir()
	danglingHTML(t, dir)
	err := walkAndRequireAny(dir, linkProbe{
		name:         "probe-any",
		wantAnyMatch: regexp.MustCompile(`anything`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "probe-any")
	assert.Contains(t, err.Error(), "rendered HTML not found")
}
