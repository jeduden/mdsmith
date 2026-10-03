package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeReleasesAPI(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/jeduden/mdsmith/releases" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusTeapot)
			return
		}
		_, _ = fmt.Fprint(w, `[
		  {"tag_name":"v0.56.0-rc.1","prerelease":true,"published_at":"2026-10-02T00:00:00Z"},
		  {"tag_name":"v0.55.1","published_at":"2026-08-29T00:00:00Z"}
		]`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GITHUB_REPOSITORY", "jeduden/mdsmith")
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_API_URL", srv.URL)
}

func readSiteReleases(t *testing.T, path string) map[string][]map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var got map[string][]map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	return got
}

func TestRunSyncReleasesWritesOutFlag(t *testing.T) {
	fakeReleasesAPI(t)
	out := filepath.Join(t.TempDir(), "releases.json")

	assert.Equal(t, 0, run([]string{"sync-releases", "--out", out}))
	got := readSiteReleases(t, out)
	require.Len(t, got["stable"], 1)
	require.Len(t, got["candidates"], 1)
	assert.Equal(t, "v0.55.1", got["stable"][0]["tag"])
	assert.Equal(t, "v0.56.0-rc.1", got["candidates"][0]["tag"])
}

func TestRunSyncReleasesDefaultsToWebsiteData(t *testing.T) {
	fakeReleasesAPI(t)
	dir := t.TempDir()
	t.Chdir(dir)

	assert.Equal(t, 0, run([]string{"sync-releases"}))
	got := readSiteReleases(t, filepath.Join(dir, "website", "data", "releases.json"))
	assert.Len(t, got["stable"], 1)
}

func TestRunSyncReleasesReportsError(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "jeduden/mdsmith")
	t.Setenv("GITHUB_TOKEN", "")
	assert.Equal(t, 1, run([]string{"sync-releases", "--out", filepath.Join(t.TempDir(), "r.json")}))
}

func TestRunSyncReleasesFlagParseError(t *testing.T) {
	assert.Equal(t, 2, run([]string{"sync-releases", "--bogus"}))
}

func TestRunSyncReleasesRejectsPositionalArgs(t *testing.T) {
	assert.Equal(t, 2, run([]string{"sync-releases", "extra"}))
}
