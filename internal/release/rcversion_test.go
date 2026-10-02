package release

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLatestStableTag(t *testing.T) {
	cases := []struct {
		name   string
		tags   []string
		want   string
		wantOK bool
	}{
		{"none", nil, "", false},
		{"only prereleases", []string{"v1.0.0-rc.1", "v0.49.0-marketplace"}, "", false},
		{"numeric not lexical order", []string{"v0.9.0", "v0.10.0", "v0.2.5"}, "v0.10.0", true},
		{"ignores rc and suffixed tags", []string{"v0.55.1", "v0.56.0-rc.3", "v0.49.0-marketplace"}, "v0.55.1", true},
		{"ignores non-version tags", []string{"latest", "v1", "v1.2", "1.2.3", "v0.1.0"}, "v0.1.0", true},
		{"patch beats minor-equal", []string{"v1.2.3", "v1.2.10", "v1.2.9"}, "v1.2.10", true},
		{"major wins", []string{"v1.0.0", "v0.99.99"}, "v1.0.0", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := LatestStableTag(tc.tags)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestNextRCVersion(t *testing.T) {
	cases := []struct {
		name string
		tags []string
		want string
	}{
		{"no tags starts at v0.1.0", nil, "v0.1.0-rc.1"},
		{"bumps minor of latest stable", []string{"v0.55.0", "v0.55.1"}, "v0.56.0-rc.1"},
		{"continues the rc series", []string{"v0.55.1", "v0.56.0-rc.1", "v0.56.0-rc.2"}, "v0.56.0-rc.3"},
		{"rc numbers compare numerically", []string{"v0.55.1", "v0.56.0-rc.9", "v0.56.0-rc.10"}, "v0.56.0-rc.11"},
		{"fills from max, not count", []string{"v0.55.1", "v0.56.0-rc.4"}, "v0.56.0-rc.5"},
		{"ignores rcs of other bases", []string{"v0.55.1", "v0.55.0-rc.7", "v0.57.0-rc.2"}, "v0.56.0-rc.1"},
		{"resets after the stable ships", []string{"v0.56.0-rc.3", "v0.56.0"}, "v0.57.0-rc.1"},
		{"ignores other suffixes", []string{"v0.55.1", "v0.56.0-beta.4", "v0.56.0-rc.x"}, "v0.56.0-rc.1"},
		{"patch reset on minor bump", []string{"v1.4.7"}, "v1.5.0-rc.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NextRCVersion(tc.tags))
		})
	}
}

func TestPreviousStableTag(t *testing.T) {
	tags := []string{"v0.54.0", "v0.55.0", "v0.55.1", "v0.56.0-rc.1", "v0.56.0-rc.2", "v0.49.0-marketplace"}
	cases := []struct {
		name    string
		tags    []string
		version string
		want    string
		wantOK  bool
	}{
		{"stable skips rc tags", tags, "v0.56.0", "v0.55.1", true},
		{"rc compares against stable only", tags, "v0.56.0-rc.3", "v0.55.1", true},
		{"excludes the version itself", tags, "v0.55.1", "v0.55.0", true},
		{"stable after its own rcs", append(tags, "v0.56.0"), "v0.57.0-rc.1", "v0.56.0", true},
		{"rerun of a tagged version", append(tags, "v0.56.0"), "v0.56.0", "v0.55.1", true},
		{"no earlier stable", tags, "v0.1.0", "", false},
		{"build metadata ignored", tags, "v0.56.0+build.7", "v0.55.1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := PreviousStableTag(tc.tags, tc.version)
			require.NoError(t, err)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPreviousStableTagRejectsMalformedVersion(t *testing.T) {
	for _, v := range []string{"", "0.56.0", "v0.56", "latest"} {
		_, _, err := PreviousStableTag([]string{"v0.1.0"}, v)
		assert.Error(t, err, v)
	}
}

// tagsServer serves /repos/jeduden/mdsmith/tags across two pages so
// the Link-header pagination path runs.
func tagsServer(t *testing.T, extra http.HandlerFunc) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/repos/jeduden/mdsmith/tags" {
			assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
			if r.URL.Query().Get("page") == "2" {
				_, _ = fmt.Fprint(w, `[{"name":"v0.56.0-rc.2"},{"name":"v0.49.0-marketplace"}]`)
				return
			}
			next := srv.URL + "/repos/jeduden/mdsmith/tags?per_page=100&page=2"
			w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, next))
			_, _ = fmt.Fprint(w, `[{"name":"v0.55.1"},{"name":"v0.56.0-rc.1"},{"name":"v0.55.0"}]`)
			return
		}
		if extra != nil {
			extra(w, r)
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestListTagsFollowsPagination(t *testing.T) {
	srv := tagsServer(t, nil)
	tags, err := ListTags(GitHubRepoOptions{
		Repository: "jeduden/mdsmith",
		Token:      "test-token",
		APIBaseURL: srv.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"v0.55.1", "v0.56.0-rc.1", "v0.55.0", "v0.56.0-rc.2", "v0.49.0-marketplace"}, tags)
}

func TestListTagsRequiresRepositoryAndToken(t *testing.T) {
	_, err := ListTags(GitHubRepoOptions{Token: "x"})
	assert.ErrorContains(t, err, "repository")
	_, err = ListTags(GitHubRepoOptions{Repository: "jeduden/mdsmith"})
	assert.ErrorContains(t, err, "token")
}

func TestListTagsReportsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	_, err := ListTags(GitHubRepoOptions{Repository: "jeduden/mdsmith", Token: "t", APIBaseURL: srv.URL})
	assert.ErrorContains(t, err, "403")
}

func TestListTagsReportsBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{not json`)
	}))
	t.Cleanup(srv.Close)
	_, err := ListTags(GitHubRepoOptions{Repository: "jeduden/mdsmith", Token: "t", APIBaseURL: srv.URL})
	assert.ErrorContains(t, err, "parse")
}

func TestListTagsReportsTransportError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial refused")
	})}
	_, err := ListTags(GitHubRepoOptions{Repository: "jeduden/mdsmith", Token: "t", Client: client})
	assert.ErrorContains(t, err, "dial refused")
}

func TestListTagsRejectsBadBaseURL(t *testing.T) {
	_, err := ListTags(GitHubRepoOptions{Repository: "jeduden/mdsmith", Token: "t", APIBaseURL: "http://bad host"})
	assert.Error(t, err)
}

func TestResolveRCVersion(t *testing.T) {
	srv := tagsServer(t, nil)
	v, err := ResolveRCVersion(GitHubRepoOptions{
		Repository: "jeduden/mdsmith", Token: "test-token", APIBaseURL: srv.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, "v0.56.0-rc.3", v)
}

func TestResolveRCVersionPropagatesListError(t *testing.T) {
	_, err := ResolveRCVersion(GitHubRepoOptions{})
	assert.Error(t, err)
}

func TestGenerateReleaseNotesPinsPreviousStableTag(t *testing.T) {
	var got map[string]string
	srv := tagsServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/repos/jeduden/mdsmith/releases/generate-notes", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		_, _ = fmt.Fprint(w, `{"name":"v0.56.0","body":"## What's Changed\n* thing"}`)
	})
	body, err := GenerateReleaseNotes(NotesOptions{
		GitHubRepoOptions: GitHubRepoOptions{Repository: "jeduden/mdsmith", Token: "test-token", APIBaseURL: srv.URL},
		Tag:               "v0.56.0",
		Target:            "abc123",
	})
	require.NoError(t, err)
	assert.Equal(t, "## What's Changed\n* thing", body)
	assert.Equal(t, map[string]string{
		"tag_name":          "v0.56.0",
		"target_commitish":  "abc123",
		"previous_tag_name": "v0.55.1",
	}, got)
}

func TestGenerateReleaseNotesOmitsPreviousTagWhenNoStable(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/jeduden/mdsmith/tags" {
			_, _ = fmt.Fprint(w, `[]`)
			return
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		_, _ = fmt.Fprint(w, `{"body":"first"}`)
	}))
	t.Cleanup(srv.Close)
	body, err := GenerateReleaseNotes(NotesOptions{
		GitHubRepoOptions: GitHubRepoOptions{Repository: "jeduden/mdsmith", Token: "t", APIBaseURL: srv.URL},
		Tag:               "v0.1.0-rc.1",
		Target:            "abc123",
	})
	require.NoError(t, err)
	assert.Equal(t, "first", body)
	assert.Equal(t, map[string]string{"tag_name": "v0.1.0-rc.1", "target_commitish": "abc123"}, got)
}

func TestGenerateReleaseNotesValidatesInputs(t *testing.T) {
	repo := GitHubRepoOptions{Repository: "jeduden/mdsmith", Token: "t"}
	_, err := GenerateReleaseNotes(NotesOptions{GitHubRepoOptions: repo, Target: "abc"})
	assert.ErrorContains(t, err, "tag")
	_, err = GenerateReleaseNotes(NotesOptions{GitHubRepoOptions: repo, Tag: "v1.0.0"})
	assert.ErrorContains(t, err, "target")
	_, err = GenerateReleaseNotes(NotesOptions{GitHubRepoOptions: repo, Tag: "nope", Target: "abc"})
	assert.ErrorContains(t, err, "nope")
}

func TestGenerateReleaseNotesPropagatesListError(t *testing.T) {
	_, err := GenerateReleaseNotes(NotesOptions{Tag: "v1.0.0", Target: "abc"})
	assert.ErrorContains(t, err, "repository")
}

func TestGenerateReleaseNotesReportsHTTPError(t *testing.T) {
	srv := tagsServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "validation failed", http.StatusUnprocessableEntity)
	})
	_, err := GenerateReleaseNotes(NotesOptions{
		GitHubRepoOptions: GitHubRepoOptions{Repository: "jeduden/mdsmith", Token: "test-token", APIBaseURL: srv.URL},
		Tag:               "v0.56.0",
		Target:            "abc123",
	})
	assert.ErrorContains(t, err, "422")
}

func TestGenerateReleaseNotesReportsBadJSON(t *testing.T) {
	srv := tagsServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{not json`)
	})
	_, err := GenerateReleaseNotes(NotesOptions{
		GitHubRepoOptions: GitHubRepoOptions{Repository: "jeduden/mdsmith", Token: "test-token", APIBaseURL: srv.URL},
		Tag:               "v0.56.0",
		Target:            "abc123",
	})
	assert.ErrorContains(t, err, "parse")
}

func TestGenerateReleaseNotesReportsTransportError(t *testing.T) {
	srv := tagsServer(t, nil)
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == http.MethodPost {
			return nil, fmt.Errorf("connection reset")
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	_, err := GenerateReleaseNotes(NotesOptions{
		GitHubRepoOptions: GitHubRepoOptions{
			Repository: "jeduden/mdsmith", Token: "test-token", APIBaseURL: srv.URL, Client: client,
		},
		Tag:    "v0.56.0",
		Target: "abc123",
	})
	assert.ErrorContains(t, err, "connection reset")
	assert.Positive(t, calls)
}
