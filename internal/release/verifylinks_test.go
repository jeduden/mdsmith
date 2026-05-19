package release

import (
	"path/filepath"
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
	writeFile(t, filepath.Join(root, "development", "merge-queue", "index.html"),
		`<a href="`+prefix+`/development/pr-fixup-workflow/">pr fixup</a>`)
	writeFile(t, filepath.Join(root, "development", "architecture-audit", "index.html"),
		`<a href="`+prefix+`/development/architecture/">arch</a>`)
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
	writeFile(t, filepath.Join(root, "development", "merge-queue", "index.html"),
		`<a href=/development/pr-fixup-workflow/>pr fixup</a>`)
	writeFile(t, filepath.Join(root, "development", "architecture-audit", "index.html"),
		`<a href=/development/architecture/>arch</a>`)
	writeFile(t, filepath.Join(root, "reference", "schema-types", "index.html"),
		`<a href=/rules/mds020-required-structure/>rule</a>`)
	writeFile(t, filepath.Join(root, "rules", "mds001", "index.html"),
		`<a href=/rules/mds021/>sibling</a>`)
	require.NoError(t, VerifyWebsiteLinks(root, ""))
}

func TestVerifyWebsiteLinks_FailsOnMissingSiblingMD(t *testing.T) {
	root := goodSite(t, "")
	writeFile(t, filepath.Join(root, "development", "merge-queue", "index.html"),
		`<a href="pr-fixup-workflow.md">stale .md ref</a>`)
	err := VerifyWebsiteLinks(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sibling .md")
}

func TestVerifyWebsiteLinks_FailsOnIndexMDMisresolved(t *testing.T) {
	root := goodSite(t, "")
	// Simulate the bug PR #309 fixed: relative target stayed
	// relative, browser resolves below the leaf page.
	writeFile(t, filepath.Join(root, "development", "architecture-audit", "index.html"),
		`<a href="architecture/">stale relative</a>`)
	err := VerifyWebsiteLinks(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "index.md drop")
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
	writeFile(t, filepath.Join(root, "development", "merge-queue", "index.html"),
		`<a href="/mdsmith/development/pr-fixup-workflow/">x</a>`)
	writeFile(t, filepath.Join(root, "development", "architecture-audit", "index.html"),
		`<a href="/mdsmith/development/architecture/">x</a>`)
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
	// Materialize only the non-recursive probe targets plus a
	// page carrying the site-absolute rule href, so we reach
	// the recursive `no README.md leak` probe.
	writeFile(t, filepath.Join(root, "development", "merge-queue", "index.html"),
		`<a href="/development/pr-fixup-workflow/">x</a>`)
	writeFile(t, filepath.Join(root, "development", "architecture-audit", "index.html"),
		`<a href="/development/architecture/">x</a>`)
	writeFile(t, filepath.Join(root, "reference", "schema-types", "index.html"),
		`<a href="/rules/mds020-required-structure/">x</a>`)
	// rules/ is absent.
	err := VerifyWebsiteLinks(root, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "walk")
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
