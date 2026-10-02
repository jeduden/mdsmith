package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeTagsAPI(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/jeduden/mdsmith/tags":
			_, _ = fmt.Fprint(w, `[{"name":"v0.55.1"},{"name":"v0.56.0-rc.1"}]`)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/jeduden/mdsmith/releases/generate-notes":
			_, _ = fmt.Fprint(w, `{"body":"## What's Changed"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusTeapot)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GITHUB_REPOSITORY", "jeduden/mdsmith")
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_API_URL", srv.URL)
	return srv
}

func TestRunRCVersionPrintsNextCandidate(t *testing.T) {
	fakeTagsAPI(t)
	var code int
	out := captureStdout(t, func() int {
		code = run([]string{"rc-version"})
		return code
	})
	assert.Equal(t, 0, code)
	assert.Equal(t, "v0.56.0-rc.2\n", out)
}

func TestRunRCVersionReportsError(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "jeduden/mdsmith")
	t.Setenv("GITHUB_TOKEN", "")
	assert.Equal(t, 1, run([]string{"rc-version"}))
}

func TestRunRCVersionFlagParseError(t *testing.T) {
	assert.Equal(t, 2, run([]string{"rc-version", "--bogus"}))
}

func TestRunRCVersionRejectsPositionalArgs(t *testing.T) {
	assert.Equal(t, 2, run([]string{"rc-version", "extra"}))
}

func TestRunReleaseNotesWritesBody(t *testing.T) {
	fakeTagsAPI(t)
	t.Setenv("RELEASE_TAG", "v0.56.0")
	t.Setenv("GITHUB_SHA", "abc123")
	out := filepath.Join(t.TempDir(), "notes.md")

	assert.Equal(t, 0, run([]string{"release-notes", out}))
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "## What's Changed\n", string(got))
}

func TestRunReleaseNotesReportsMissingTag(t *testing.T) {
	fakeTagsAPI(t)
	t.Setenv("RELEASE_TAG", "")
	t.Setenv("GITHUB_SHA", "abc123")
	assert.Equal(t, 1, run([]string{"release-notes", filepath.Join(t.TempDir(), "n.md")}))
}

func TestRunReleaseNotesReportsWriteError(t *testing.T) {
	fakeTagsAPI(t)
	t.Setenv("RELEASE_TAG", "v0.56.0")
	t.Setenv("GITHUB_SHA", "abc123")
	missing := filepath.Join(t.TempDir(), "no-such-dir", "n.md")
	assert.Equal(t, 1, run([]string{"release-notes", missing}))
}

func TestRunReleaseNotesFlagParseError(t *testing.T) {
	assert.Equal(t, 2, run([]string{"release-notes", "--bogus"}))
}

func TestRunReleaseNotesRequiresOutPath(t *testing.T) {
	assert.Equal(t, 2, run([]string{"release-notes"}))
}

func fakeRCCheckAPI(t *testing.T, tags string) *[]string {
	t.Helper()
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/jeduden/mdsmith/tags":
			_, _ = fmt.Fprint(w, tags)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/jeduden/mdsmith/releases":
			_, _ = fmt.Fprint(w, `[{"id":42,"draft":true,"tag_name":"v0.56.0-rc.2"}]`)
		case r.Method == http.MethodDelete:
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusTeapot)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GITHUB_REPOSITORY", "jeduden/mdsmith")
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_API_URL", srv.URL)
	t.Setenv("RELEASE_TAG", "v0.56.0-rc.2")
	return &deleted
}

func outputFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "github_output")
	t.Setenv("GITHUB_OUTPUT", p)
	return p
}

func TestRunCheckRCCurrent(t *testing.T) {
	deleted := fakeRCCheckAPI(t, `[{"name":"v0.55.1"},{"name":"v0.56.0-rc.1"}]`)
	out := outputFile(t)

	assert.Equal(t, 0, run([]string{"check-rc", "--discard-draft"}))
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "current=true\n", string(got))
	assert.Empty(t, *deleted)
}

func TestRunCheckRCStaleDiscardsDraft(t *testing.T) {
	deleted := fakeRCCheckAPI(t, `[{"name":"v0.56.0"}]`)
	out := outputFile(t)

	assert.Equal(t, 0, run([]string{"check-rc", "--discard-draft"}))
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "current=false\n", string(got))
	assert.Equal(t, []string{"/repos/jeduden/mdsmith/releases/42"}, *deleted)
}

func TestRunCheckRCWithoutGitHubOutput(t *testing.T) {
	fakeRCCheckAPI(t, `[{"name":"v0.55.1"},{"name":"v0.56.0-rc.1"}]`)
	t.Setenv("GITHUB_OUTPUT", "")
	var code int
	out := captureStdout(t, func() int {
		code = run([]string{"check-rc"})
		return code
	})
	assert.Equal(t, 0, code)
	assert.Equal(t, "current=true\n", out)
}

func TestRunCheckRCReportsAPIError(t *testing.T) {
	fakeRCCheckAPI(t, `[]`)
	t.Setenv("GITHUB_TOKEN", "")
	assert.Equal(t, 1, run([]string{"check-rc"}))
}

func TestRunCheckRCReportsOutputWriteError(t *testing.T) {
	fakeRCCheckAPI(t, `[{"name":"v0.55.1"},{"name":"v0.56.0-rc.1"}]`)
	t.Setenv("GITHUB_OUTPUT", t.TempDir()) // a directory cannot be opened for append
	assert.Equal(t, 1, run([]string{"check-rc"}))
}

func TestRunCheckRCFlagParseError(t *testing.T) {
	assert.Equal(t, 2, run([]string{"check-rc", "--bogus"}))
}

func TestRunCheckRCRejectsPositionalArgs(t *testing.T) {
	assert.Equal(t, 2, run([]string{"check-rc", "extra"}))
}
