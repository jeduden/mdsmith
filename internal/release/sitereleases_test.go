package release

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
		{"fence opened in a list item kept",
			"- ```sh\n  # install\n  ```\n# after", "- ```sh\n  # install\n  ```\n### after"},
		{"fence opened in a block quote kept",
			"> ```sh\n> # install\n> ```\n# after", "> ```sh\n> # install\n> ```\n### after"},
		{"fence in a nested list item closes at its content indent",
			"  - ```sh\n    # install\n    ```\n# after", "  - ```sh\n    # install\n    ```\n### after"},
		{"fence in a wide ordered item closes at its content indent",
			"10. ```sh\n    # install\n    ```\n# after", "10. ```sh\n    # install\n    ```\n### after"},
		{"fence ends with its list item",
			"- ```\n  # code\n- next\n# after", "- ```\n  # code\n- next\n### after"},
		{"fence ends with its block quote",
			"> ```\n> # code\n\n# after", "> ```\n> # code\n\n### after"},
		{"quoted closer does not close a top-level fence",
			"```md\n> ```\n# code\n> ```\n```\n# after", "```md\n> ```\n# code\n> ```\n```\n### after"},
		{"backticks in the info string make no fence",
			"```go``` is inline code\n# after", "```go``` is inline code\n### after"},
		{"inline backticks behind a list marker make no fence",
			"* ```x``` fix\n# after", "* ```x``` fix\n### after"},
		{"lazy line after an indented item continuation is no setext",
			"- item\n    more\nText\n---", "- item\n    more\nText\n---"},
		{"star break is not absorbed into a setext heading",
			"***\nText\n---", "***\n#### Text\n"},
		{"setext after a spaced break becomes atx", "* * *\nText\n---", "* * *\n#### Text\n"},
		{"setext in a list item keeps its indent",
			"- item\n\n  Sub\n  ---", "- item\n\n  #### Sub\n"},
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
		{"explicit id is scoped", "## A {#own}", "#### A {#v1-own}"},
		{"explicit page id cannot collide", "## S {#stable}", "#### S {#v1-stable}"},
		{"no slug falls back to heading", "## !!!", "#### !!! {#v1-heading}"},
		{"emoji heading gets an id", "## 🎉\n## 🎉", "#### 🎉 {#v1-heading}\n#### 🎉 {#v1-heading-1}"},
		{"empty heading", "##", "####"},
		{"counter skips a taken id", "## A\n## A\n## A 1", "#### A {#v1-a}\n#### A {#v1-a-1}\n#### A 1 {#v1-a-1-1}"},
		{"brace text is not an attribute", "## Fix {x}", "#### Fix {x} {#v1-fix-x}"},
		{"class attribute gains a scoped id", "## A {.c}", "#### A {#v1-a .c}"},
		{"key-value attribute gains a scoped id", "## A {k=v}", "#### A {#v1-a k=v}"},
		{"explicit id joins the uniqueness set", "## A {#a}\n## A", "#### A {#v1-a}\n#### A {#v1-a-1}"},
		{"unmatched brace is heading text", "## Support for {", "#### Support for { {#v1-support-for}"},
		{"empty braces are heading text", "## Support for {}", "#### Support for {} {#v1-support-for}"},
		{"comparison braces are heading text", "## What {a == b}", "#### What {a == b} {#v1-what-a-b}"},
		{"heading in a block quote", "> ## Quote", "> #### Quote {#v1-quote}"},
		{"heading in a nested block quote", "> > # Deep", "> > ### Deep {#v1-deep}"},
		{"heading in a bullet item", "- ## Item", "- #### Item {#v1-item}"},
		{"heading in an ordered item", "1. # One", "1. ### One {#v1-one}"},
		{"heading in a quoted list item", "> * ## Both", "> * #### Both {#v1-both}"},
		{"thematic break is not a heading", "* * *", "* * *"},
		{"quoted text is not a heading", "> #1 fixed", "> #1 fixed"},
		{"setext with its own id", "Notes {#n}\n---", "#### Notes {#v1-n}\n"},
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

func TestBuildSiteReleasesDefusesHugoShortcodes(t *testing.T) {
	got := BuildSiteReleases([]GitHubRelease{{
		TagName: "v1.0.0",
		Body:    "* quote {{< callout >}} and `{{% note %}}`",
	}})
	require.Len(t, got.Stable, 1)
	assert.Equal(t, "* quote {\u200b{< callout >}} and `{\u200b{% note %}}`",
		got.Stable[0].Body, "RenderString would expand a shortcode and fail on an unknown one")
}

func TestDefuseShortcodes(t *testing.T) {
	cases := map[string]string{
		"{{< x >}}":         "{\u200b{< x >}}",
		"{{% x %}}":         "{\u200b{% x %}}",
		"{{</* x */>}}":     "{\u200b{</* x */>}}",
		"a {{< x >}} {{<y":  "a {\u200b{< x >}} {\u200b{<y",
		"{{ .Title }} {<{":  "{{ .Title }} {<{",
		"no shortcode here": "no shortcode here",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, defuseShortcodes(in))
		})
	}
}

func TestBuildSiteReleasesKeepsOnlyCandidatesAfterLatestStable(t *testing.T) {
	got := BuildSiteReleases([]GitHubRelease{
		{TagName: "v0.55.0-rc.1", Prerelease: true, PublishedAt: mustTime(t, "2026-08-01T00:00:00Z")},
		{TagName: "v0.55.0", PublishedAt: mustTime(t, "2026-08-02T00:00:00Z")},
		{TagName: "v0.56.0-rc.1", Prerelease: true, PublishedAt: mustTime(t, "2026-08-03T00:00:00Z")},
	})
	require.Len(t, got.Candidates, 1, "a candidate the latest stable release already shipped is dropped")
	assert.Equal(t, "v0.56.0-rc.1", got.Candidates[0].Tag)
}

func TestBuildSiteReleasesKeepsAllCandidatesWithoutStable(t *testing.T) {
	got := BuildSiteReleases([]GitHubRelease{
		{TagName: "v0.1.0-rc.1", Prerelease: true, PublishedAt: mustTime(t, "2026-01-01T00:00:00Z")},
		{TagName: "v0.1.0-rc.2", Prerelease: true, PublishedAt: mustTime(t, "2026-01-02T00:00:00Z")},
	})
	assert.Len(t, got.Candidates, 2)
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
		"A {#id}":        true,
		"A {.cls}":       true,
		"A {k=v}":        true,
		"A { #id }":      true,
		"Fix {x}":        false,
		"Plain":          false,
		"Ends in }":      false,
		"{#id} then":     false,
		"What {a == b}":  false,
		"A {#id .c k=v}": true,
		"A {k=}":         false,
		"A {=v}":         false,
		"A {#}":          false,
		"A {}":           false,
		"A {# x}":        false,
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
	assert.Equal(t, "v1-heading", ids.next("!!!"))
	assert.Empty(t, ids.next(""), "an empty heading gets no id")

	none := headingIDs{seen: map[string]bool{}}
	assert.Empty(t, none.next("A"))
}

func TestHeadingIDsClaim(t *testing.T) {
	ids := headingIDs{prefix: "v1", seen: map[string]bool{}}
	assert.Equal(t, "v1-a", ids.claim("v1-a"))
	assert.Equal(t, "v1-a-1", ids.claim("v1-a"))
	assert.Equal(t, "v1-a-2", ids.claim("v1-a"))
}

func TestHeadingIDsScoped(t *testing.T) {
	ids := headingIDs{prefix: "v1", seen: map[string]bool{}}
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"A", "A {#v1-a}", true},
		{"B {#own}", "B {#v1-own}", true},
		{"C {.c k=v}", "C {#v1-c .c k=v}", true},
		{"D {#!!}", "D {#v1-d}", true},
		{"!!! {.c}", "!!! {#v1-heading .c}", true},
		{"!!!", "!!! {#v1-heading-1}", true},
		{"", "", false},
		{"{.c}", "{.c}", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := ids.scoped(tc.in)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.ok, ok)
		})
	}
	none := headingIDs{seen: map[string]bool{}}
	got, ok := none.scoped("A {#own}")
	assert.Equal(t, "A {#own}", got)
	assert.False(t, ok)
}

func TestContainerPrefix(t *testing.T) {
	cases := map[string]int{
		"> ## Q":        2,
		">## Q":         1,
		"> > # Q":       4,
		"- ## I":        2,
		"+\t# I":        2,
		"1. # O":        3,
		"12) # O":       4,
		"> * ## B":      4,
		"* * *":         4,
		"-x":            0,
		"1.x":           0,
		"1234567890. x": 0,
		"plain":         0,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, containerPrefix(in))
		})
	}
}

func TestHeadingIDsATX(t *testing.T) {
	ids := headingIDs{prefix: "v1", seen: map[string]bool{}}
	assert.Equal(t, "### A {#v1-a}", ids.atx("###", " A ##"))
	assert.Equal(t, "### !!! {#v1-heading}", ids.atx("###", "\t!!!"))
	assert.Equal(t, "###  ", ids.atx("###", "  "), "an empty heading keeps its line")
}

func TestFenceContainers(t *testing.T) {
	quote := fenceContainer{quote: true}
	cases := []struct {
		name    string
		lead    int
		markers string
		pad     int
		want    []fenceContainer
	}{
		{"bullet", 0, "- ", 0, []fenceContainer{{width: 2}}},
		{"indented bullet", 2, "- ", 0, []fenceContainer{{width: 4}}},
		{"padded bullet", 0, "- ", 2, []fenceContainer{{width: 4}}},
		{"wide ordered item", 0, "10. ", 0, []fenceContainer{{width: 4}}},
		{"nested bullets", 0, "- - ", 0, []fenceContainer{{width: 2}, {width: 2}}},
		{"quote", 1, "> ", 1, []fenceContainer{quote}},
		{"quote without space", 0, ">", 0, []fenceContainer{quote}},
		{"quoted bullet", 1, "> * ", 1, []fenceContainer{quote, {width: 3}}},
		{"quote in a bullet", 0, "- > ", 0, []fenceContainer{{width: 2}, quote}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, fenceContainers(tc.lead, tc.markers, tc.pad))
		})
	}
}

func TestStripContainers(t *testing.T) {
	quote := fenceContainer{quote: true}
	item := fenceContainer{width: 4}
	cases := []struct {
		name   string
		line   string
		cs     []fenceContainer
		want   string
		inside bool
	}{
		{"top level keeps the line", "  ```", nil, "  ```", true},
		{"quote marker and space", "> ```", []fenceContainer{quote}, "```", true},
		{"quote marker alone", ">", []fenceContainer{quote}, "", true},
		{"indented quote marker", "   >```", []fenceContainer{quote}, "```", true},
		{"nested quotes", "  > >  ```", []fenceContainer{quote, quote}, " ```", true},
		{"missing quote marker", "```", []fenceContainer{quote}, "", false},
		{"blank line ends a quote", "", []fenceContainer{quote}, "", false},
		{"four-space quote marker", "    > ```", []fenceContainer{quote}, "", false},
		{"item content indent", "    ```", []fenceContainer{item}, "```", true},
		{"deeper than the item", "      x", []fenceContainer{item}, "  x", true},
		{"blank line stays in an item", "  \t", []fenceContainer{item}, "", true},
		{"blank line ends a quote inside an item", "", []fenceContainer{item, quote}, "", false},
		{"shallower line ends an item", "  ```", []fenceContainer{item}, "", false},
		{"quoted item", ">     x", []fenceContainer{quote, item}, "x", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, inside := stripContainers(tc.line, tc.cs)
			assert.Equal(t, tc.inside, inside)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestOpeningFence(t *testing.T) {
	cases := []struct {
		in   string
		char byte
		n    int
	}{
		{"```", '`', 3},
		{"````go", '`', 4},
		{"~~~ `x`", '~', 3},
		{"```go``` inline", 0, 0},
		{"``x", 0, 0},
		{"text", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			c, n := openingFence(tc.in)
			assert.Equal(t, tc.char, c)
			assert.Equal(t, tc.n, n)
		})
	}
}

func TestIsThematicBreak(t *testing.T) {
	cases := map[string]bool{
		"***":     true,
		"___":     true,
		"- - -":   true,
		"*\t*\t*": true,
		"**":      false,
		"*-*":     false,
		"__init":  false,
		"text":    false,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, isThematicBreak(in))
		})
	}
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

	w = headingRewriter{
		lines:     []string{"- ```", "  # code", "# after"},
		shift:     2,
		ids:       headingIDs{seen: map[string]bool{}},
		paraStart: -1,
		canStart:  true,
	}
	for i := range w.lines {
		w.visit(i)
	}
	assert.Equal(t, []string{"- ```", "  # code", "### after"}, w.lines,
		"a line outside the item ends the fence and is handled as ordinary")
	assert.Zero(t, w.fenceChar)
	assert.Nil(t, w.fenceIn)
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

	lines = []string{"  Sub", "  ---"}
	rewriteSetext(lines, 0, 1, "####", &noID)
	assert.Equal(t, []string{"  #### Sub", ""}, lines, "the first line's indent is kept")
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

func TestHeadingRewriterContainedATX(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"quoted heading", "> # A", "> ### A {#v1-a}"},
		{"indented after marker", " - # A", " - ### A {#v1-a}"},
		{"no container marker", "| # A |", "| # A |"},
		{"four spaces after marker is code", ">     # A", ">     # A"},
		{"quoted text", "> text", "> text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := headingRewriter{
				lines: []string{tc.in}, shift: 2,
				ids: headingIDs{prefix: "v1", seen: map[string]bool{}},
			}
			trimmed := strings.TrimLeft(tc.in, " ")
			w.containedATX(0, tc.in[:len(tc.in)-len(trimmed)], trimmed)
			assert.Equal(t, tc.want, w.lines[0])
		})
	}
}

func TestHeadingRewriterIsCloser(t *testing.T) {
	w := headingRewriter{fenceChar: '`', fenceLen: 3}
	assert.True(t, w.isCloser("```"))
	assert.True(t, w.isCloser("````  "))
	assert.False(t, w.isCloser("``"), "shorter run")
	assert.False(t, w.isCloser("~~~"), "other fence char")
	assert.False(t, w.isCloser("``` x"), "text after the run")
	assert.False(t, w.isCloser("> ```"), "markers are stripContainers' job")
}

func TestBuildSiteReleasesBackportKeepsNextLineCandidates(t *testing.T) {
	got := BuildSiteReleases([]GitHubRelease{
		{TagName: "v0.56.0", PublishedAt: mustTime(t, "2026-08-01T00:00:00Z")},
		{TagName: "v0.57.0-rc.1", Prerelease: true, PublishedAt: mustTime(t, "2026-08-02T00:00:00Z")},
		{TagName: "v0.55.2", PublishedAt: mustTime(t, "2026-08-03T00:00:00Z")},
		{TagName: "v0.57.0-rc.2", Prerelease: true, PublishedAt: mustTime(t, "2026-08-04T00:00:00Z")},
	})
	tags := make([]string, 0, len(got.Candidates))
	for _, c := range got.Candidates {
		tags = append(tags, c.Tag)
	}
	assert.Equal(t, []string{"v0.57.0-rc.2", "v0.57.0-rc.1"}, tags,
		"a later backport of an older line must not hide the next line's candidates")
}

func TestCandidatesAfterStable(t *testing.T) {
	at := func(s string) time.Time { return mustTime(t, s) }
	stable := []SiteRelease{
		{Tag: "v0.55.2", Published: at("2026-08-03T00:00:00Z")},
		{Tag: "v0.56.0", Published: at("2026-08-01T00:00:00Z")},
	}
	cands := []SiteRelease{
		{Tag: "v0.57.0-rc.1", Published: at("2026-08-02T00:00:00Z")},
		{Tag: "v0.56.0-rc.4", Published: at("2026-07-30T00:00:00Z")},
		{Tag: "nightly-late", Published: at("2026-08-05T00:00:00Z")},
		{Tag: "nightly-early", Published: at("2026-08-02T00:00:00Z")},
	}
	got := candidatesAfterStable(cands, stable)
	tags := make([]string, 0, len(got))
	for _, c := range got {
		tags = append(tags, c.Tag)
	}
	assert.Equal(t, []string{"v0.57.0-rc.1", "nightly-late"}, tags,
		"semver candidates compare by version; others by date against the newest stable")

	noSemver := []SiteRelease{{Tag: "v0.49.0-marketplace", Published: at("2026-08-01T00:00:00Z")}}
	got = candidatesAfterStable([]SiteRelease{
		{Tag: "v0.1.0-rc.1", Published: at("2026-08-02T00:00:00Z")},
		{Tag: "v0.0.9-rc.1", Published: at("2026-07-01T00:00:00Z")},
	}, noSemver)
	require.Len(t, got, 1, "without a plain vX.Y.Z stable the cut falls back to dates")
	assert.Equal(t, "v0.1.0-rc.1", got[0].Tag)

	all := []SiteRelease{{Tag: "v0.1.0-rc.1"}}
	assert.Equal(t, all, candidatesAfterStable(all, nil), "no stable release keeps every candidate")
}

func TestIsAttributeToken(t *testing.T) {
	cases := map[string]bool{
		"#id":      true,
		".cls":     true,
		".NET":     true,
		"k=v":      true,
		"data-x=1": true,
		"#":        false,
		".":        false,
		"k=":       false,
		"=v":       false,
		"a":        false,
		"==":       false,
		"1k=v":     false,
		"#a b":     false,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, isAttributeToken(in))
		})
	}
}
