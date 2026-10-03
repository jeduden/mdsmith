package release

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	require.NoError(t, err)
	return ts
}

func TestRewriteHeadingsDemotes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"h2 becomes h4", "## What's Changed\n* a", "#### What's Changed\n* a"},
		{"capped at h6", "##### Deep\n###### Deeper", "###### Deep\n###### Deeper"},
		{"empty heading", "##\nx", "####\nx"},
		{"indented heading", "   # Top", "   ### Top"},
		{"issue ref is not a heading", "#123 fixed", "#123 fixed"},
		{"seven hashes is not a heading", "####### x", "####### x"},
		{"four-space indent is code", "    # code", "    # code"},
		{"backtick fence kept", "```\n# comment\n```\n# after", "```\n# comment\n```\n### after"},
		{"tilde fence kept", "~~~sh\n# c\n~~~\n# d", "~~~sh\n# c\n~~~\n### d"},
		{"shorter fence does not close", "````\n```\n# in\n````\n# out", "````\n```\n# in\n````\n### out"},
		{"other fence char does not close", "```\n~~~\n# in\n```", "```\n~~~\n# in\n```"},
		{"tab after hashes", "#\tTab", "###\tTab"},
		{"fence closer with trailing spaces", "```\n# c\n```  \n# after", "```\n# c\n```  \n### after"},
		{"setext h2 becomes atx", "Highlights\n---\n\nx", "#### Highlights\n\n\nx"},
		{"setext h1 becomes atx", "Title\n===", "### Title\n"},
		{"multi-line setext joins its lines", "Two\nlines\n---", "#### Two lines\n\n"},
		{"thematic break after blank stays", "a\n\n---\nb", "a\n\n---\nb"},
		{"dash under a list item is a break", "- item\n---", "- item\n---"},
		{"underline under a fence is not setext", "```\nx\n```\n---", "```\nx\n```\n---"},
		{"underline under a heading is not setext", "# H\n---", "### H\n---"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, rewriteHeadings(tc.in, 2, ""))
		})
	}
}

func TestRewriteHeadingsAddsScopedIDs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"prefixed slug", "## What's Changed", "#### What's Changed {#v1-whats-changed}"},
		{"closing hashes dropped", "## New Contributors ##", "#### New Contributors {#v1-new-contributors}"},
		{"repeat gets a counter", "## A\n## A", "#### A {#v1-a}\n#### A {#v1-a-1}"},
		{"explicit attribute kept", "## A {#own}", "#### A {#own}"},
		{"no slug, no id", "## !!!", "#### !!!"},
		{"empty heading", "##", "####"},
		{"counter skips a taken id", "## A\n## A\n## A 1", "#### A {#v1-a}\n#### A {#v1-a-1}\n#### A 1 {#v1-a-1-1}"},
		{"brace text is not an attribute", "## Fix {x}", "#### Fix {x} {#v1-fix-x}"},
		{"class attribute kept", "## A {.c}", "#### A {.c}"},
		{"key-value attribute kept", "## A {k=v}", "#### A {k=v}"},
		{"setext heading gets an id", "Notes\n---", "#### Notes {#v1-notes}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, rewriteHeadings(tc.in, 2, "v1"))
		})
	}
}

func TestHeadingSlug(t *testing.T) {
	cases := map[string]string{
		"What's Changed":   "whats-changed",
		"v0.56.0-rc.2":     "v0-56-0-rc-2",
		"  Spaced  Out  ":  "spaced-out",
		"snake_case Title": "snake-case-title",
		"Ünïcode Wörds":    "ünïcode-wörds",
		"!!!":              "",
		"trailing dash -":  "trailing-dash",
		"tab\tseparated":   "tab-separated",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, headingSlug(in))
		})
	}
}

func TestBuildSiteReleases(t *testing.T) {
	rels := []GitHubRelease{
		{
			TagName: "v0.55.0", Name: "v0.55.0", HTMLURL: "u550",
			PublishedAt: mustTime(t, "2026-08-29T11:37:42Z"), Body: "## What's Changed\r\n* a\r\n",
		},
		{
			TagName: "v0.56.0-rc.1", HTMLURL: "urc1", Prerelease: true, Body: "rc1",
			PublishedAt: mustTime(t, "2026-10-02T23:43:12Z"),
		},
		{TagName: "v0.56.0-rc.3", Name: "draft", Draft: true, Prerelease: true},
		{
			TagName: "v0.55.1", Name: "v0.55.1", HTMLURL: "u551", Body: "fix",
			PublishedAt: mustTime(t, "2026-08-29T22:32:06Z"),
		},
		{
			TagName: "v0.56.0-rc.2", Name: "v0.56.0-rc.2", HTMLURL: "urc2", Prerelease: true, Body: "rc2",
			PublishedAt: mustTime(t, "2026-10-03T00:05:12Z"),
		},
	}
	got := BuildSiteReleases(rels)

	require.Len(t, got.Stable, 2)
	assert.Equal(t, "v0.55.1", got.Stable[0].Tag, "newest stable first")
	assert.Equal(t, "v0.55.0", got.Stable[1].Tag)
	assert.Equal(t, "#### What's Changed {#v0-55-0-whats-changed}\n* a", got.Stable[1].Body,
		"CRLF normalized, headings demoted and tag-scoped, trailing space trimmed")
	assert.Equal(t, "u551", got.Stable[0].URL)

	require.Len(t, got.Candidates, 2, "draft dropped")
	assert.Equal(t, "v0.56.0-rc.2", got.Candidates[0].Tag, "newest candidate first")
	assert.Equal(t, "v0.56.0-rc.1", got.Candidates[1].Name, "empty name falls back to the tag")
}

func TestBuildSiteReleasesTieBreaksOnTag(t *testing.T) {
	ts := mustTime(t, "2026-01-01T00:00:00Z")
	got := BuildSiteReleases([]GitHubRelease{
		{TagName: "v1.0.0", PublishedAt: ts},
		{TagName: "v1.0.1", PublishedAt: ts},
	})
	require.Len(t, got.Stable, 2)
	assert.Equal(t, "v1.0.1", got.Stable[0].Tag)
}

func TestBuildSiteReleasesEmptyListsMarshalAsArrays(t *testing.T) {
	data, err := json.Marshal(BuildSiteReleases(nil))
	require.NoError(t, err)
	assert.JSONEq(t, `{"stable":[],"candidates":[]}`, string(data))
}

func TestListReleasesFollowsPagination(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		assert.Equal(t, "/repos/o/r/releases", r.URL.Path)
		if r.URL.Query().Get("page") == "2" {
			_, _ = fmt.Fprint(w, `[{"tag_name":"v0.1.0","published_at":"2026-01-01T00:00:00Z"}]`)
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/releases?per_page=100&page=2>; rel="next"`, srv.URL))
		_, _ = fmt.Fprint(w, `[{"tag_name":"v0.2.0","name":"two","html_url":"h","body":"b",`+
			`"prerelease":true,"draft":false,"published_at":"2026-02-01T00:00:00Z"}]`)
	}))
	defer srv.Close()

	got, err := ListReleases(GitHubRepoOptions{Repository: "o/r", Token: "tok", APIBaseURL: srv.URL})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, GitHubRelease{
		TagName: "v0.2.0", Name: "two", HTMLURL: "h", Body: "b", Prerelease: true,
		PublishedAt: mustTime(t, "2026-02-01T00:00:00Z"),
	}, got[0])
	assert.Equal(t, "v0.1.0", got[1].TagName)
}

func TestListReleasesErrors(t *testing.T) {
	t.Run("missing repository", func(t *testing.T) {
		_, err := ListReleases(GitHubRepoOptions{Token: "tok"})
		assert.ErrorContains(t, err, "repository is required")
	})
	t.Run("bad status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusForbidden)
		}))
		defer srv.Close()
		_, err := ListReleases(GitHubRepoOptions{Repository: "o/r", Token: "tok", APIBaseURL: srv.URL})
		assert.ErrorContains(t, err, "403")
	})
	t.Run("bad json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, `{`)
		}))
		defer srv.Close()
		_, err := ListReleases(GitHubRepoOptions{Repository: "o/r", Token: "tok", APIBaseURL: srv.URL})
		assert.ErrorContains(t, err, "parse")
	})
	t.Run("bad url", func(t *testing.T) {
		_, err := ListReleases(GitHubRepoOptions{Repository: "o/r", Token: "tok", APIBaseURL: "http://[::1"})
		assert.Error(t, err)
	})
	t.Run("transport error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		srv.Close()
		_, err := ListReleases(GitHubRepoOptions{Repository: "o/r", Token: "tok", APIBaseURL: srv.URL})
		assert.Error(t, err)
	})
}

func TestSyncReleasesWritesJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `[
		  {"tag_name":"v0.56.0-rc.1","html_url":"rc","body":"## What's Changed","prerelease":true,
		   "published_at":"2026-10-02T00:00:00Z"},
		  {"tag_name":"v0.55.1","name":"v0.55.1","html_url":"st","body":"fix","published_at":"2026-08-29T00:00:00Z"}
		]`)
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "data", "releases.json")
	require.NoError(t, SyncReleases(GitHubRepoOptions{Repository: "o/r", Token: "tok", APIBaseURL: srv.URL}, out))

	raw, err := os.ReadFile(out)
	require.NoError(t, err)
	var got SiteReleases
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Len(t, got.Stable, 1)
	require.Len(t, got.Candidates, 1)
	assert.Equal(t, "v0.55.1", got.Stable[0].Tag)
	assert.Equal(t, "#### What's Changed {#v0-56-0-rc-1-whats-changed}", got.Candidates[0].Body)
	assert.Equal(t, byte('\n'), raw[len(raw)-1], "file ends with a newline")
}

func TestSyncReleasesErrors(t *testing.T) {
	t.Run("list fails", func(t *testing.T) {
		err := SyncReleases(GitHubRepoOptions{}, filepath.Join(t.TempDir(), "x.json"))
		assert.ErrorContains(t, err, "repository is required")
	})
	t.Run("out dir not creatable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, `[]`)
		}))
		defer srv.Close()
		blocker := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(blocker, nil, 0o644))
		err := SyncReleases(GitHubRepoOptions{Repository: "o/r", Token: "tok", APIBaseURL: srv.URL},
			filepath.Join(blocker, "sub", "releases.json"))
		assert.Error(t, err)
	})
}

func TestSetextLevel(t *testing.T) {
	cases := map[string]int{
		"===":   1,
		"=":     1,
		"---":   2,
		"-  \t": 2,
		"- - -": 0,
		"==-":   0,
		"":      0,
		"  ":    0,
		"text":  0,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, setextLevel(in))
		})
	}
}

func TestOpensNonParagraphBlock(t *testing.T) {
	cases := map[string]bool{
		"> quote":         true,
		"<details>":       true,
		"| a | b |":       true,
		"- item":          true,
		"*\titem":         true,
		"+":               true,
		"1. item":         true,
		"12) item":        true,
		"3.":              true,
		"**Full** change": false,
		"-dash":           false,
		"1.5 release":     false,
		"2026":            false,
		"plain":           false,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, opensNonParagraphBlock(in))
		})
	}
}

func TestHasAttributeBlock(t *testing.T) {
	cases := map[string]bool{
		"A {#id}":    true,
		"A {.cls}":   true,
		"A {k=v}":    true,
		"A { #id }":  true,
		"Fix {x}":    false,
		"Plain":      false,
		"Ends in }":  false,
		"{#id} then": false,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, hasAttributeBlock(in))
		})
	}
}

func TestHeadingIDsNext(t *testing.T) {
	ids := headingIDs{prefix: "v1", seen: map[string]bool{}}
	assert.Equal(t, "v1-a-1", ids.next("A 1"))
	assert.Equal(t, "v1-a", ids.next("A"))
	assert.Equal(t, "v1-a-2", ids.next("A"))
	assert.Empty(t, ids.next("!!!"))
	assert.Empty(t, ids.next("A {#own}"))

	none := headingIDs{seen: map[string]bool{}}
	assert.Empty(t, none.next("A"))
}

func TestHeadingIDsATX(t *testing.T) {
	ids := headingIDs{prefix: "v1", seen: map[string]bool{}}
	assert.Equal(t, "### A {#v1-a}", ids.atx("###", " A ##"))
	assert.Equal(t, "###\t!!!", ids.atx("###", "\t!!!"))
}

func TestHeadingRewriterClosesFence(t *testing.T) {
	w := headingRewriter{fenceChar: '`', fenceLen: 3}
	assert.True(t, w.closesFence("```"))
	assert.True(t, w.closesFence("  ````  "))
	assert.False(t, w.closesFence("``"))
	assert.False(t, w.closesFence("```go"))
	assert.False(t, w.closesFence("~~~"))
	assert.False(t, w.closesFence("    ```"))
}

func TestHeadingRewriterHashes(t *testing.T) {
	w := headingRewriter{shift: 2}
	assert.Equal(t, "###", w.hashes(1))
	assert.Equal(t, "######", w.hashes(5))
}

func TestHeadingRewriterVisit(t *testing.T) {
	w := headingRewriter{
		lines:     []string{"Para", "    indented", "---"},
		shift:     2,
		ids:       headingIDs{seen: map[string]bool{}},
		paraStart: -1,
		canStart:  true,
	}
	for i := range w.lines {
		w.visit(i)
	}
	// The indented line lazily continues the paragraph, so the
	// underline turns both lines into one heading.
	assert.Equal(t, []string{"#### Para indented", "", ""}, w.lines)
}

func TestHeadingRewriterBlock(t *testing.T) {
	w := headingRewriter{
		lines:     []string{"- item", "lazy", "---"},
		ids:       headingIDs{seen: map[string]bool{}},
		paraStart: -1,
		canStart:  true,
	}
	w.block(0, "", w.lines[0])
	assert.False(t, w.canStart)
	w.block(1, "", w.lines[1])
	assert.Equal(t, -1, w.paraStart, "a lazy list continuation opens no paragraph")
	w.block(2, "", w.lines[2])
	assert.Equal(t, []string{"- item", "lazy", "---"}, w.lines)
}

func TestRewriteSetext(t *testing.T) {
	ids := headingIDs{prefix: "v1", seen: map[string]bool{}}
	lines := []string{"intro", "", "Two ", "  lines", "---", "after"}
	rewriteSetext(lines, 2, 4, "####", &ids)
	assert.Equal(t, []string{"intro", "", "#### Two lines {#v1-two-lines}", "", "", "after"}, lines)

	noID := headingIDs{seen: map[string]bool{}}
	lines = []string{"Title", "==="}
	rewriteSetext(lines, 0, 1, "###", &noID)
	assert.Equal(t, []string{"### Title", ""}, lines)
}

func TestATXLevel(t *testing.T) {
	cases := map[string]int{
		"# a":       1,
		"###### a":  6,
		"#######":   0,
		"##":        2,
		"#\ta":      1,
		"#a":        0,
		"text":      0,
		"":          0,
		"### # a #": 3,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, atxLevel(in))
		})
	}
}

func TestSortNewestFirst(t *testing.T) {
	early := mustTime(t, "2026-01-01T00:00:00Z")
	late := mustTime(t, "2026-02-01T00:00:00Z")
	rs := []SiteRelease{
		{Tag: "v1.0.0", Published: early},
		{Tag: "v1.1.0", Published: late},
		{Tag: "v1.0.1", Published: early},
	}
	sortNewestFirst(rs)
	tags := []string{rs[0].Tag, rs[1].Tag, rs[2].Tag}
	assert.Equal(t, []string{"v1.1.0", "v1.0.1", "v1.0.0"}, tags)
}

func TestATXHeadingText(t *testing.T) {
	cases := map[string]string{
		" Title":          "Title",
		" Title ##":       "Title",
		" Title\t#":       "Title",
		" C#":             "C#",
		" ##":             "",
		"":                "",
		" Title {#id}":    "Title {#id}",
		" Title ## extra": "Title ## extra",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, atxHeadingText(in))
		})
	}
}
