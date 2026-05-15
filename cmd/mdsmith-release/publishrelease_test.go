package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunPublishReleaseFlipsDraft(t *testing.T) {
	var patched bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			patched = true
			_, _ = w.Write([]byte(`{"id":42,"draft":false}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":42,"draft":true}`))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GITHUB_REPOSITORY", "jeduden/mdsmith")
	t.Setenv("RELEASE_TAG", "v1.2.3")
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_API_URL", srv.URL)

	assert.Equal(t, 0, run([]string{"publish-release"}))
	assert.True(t, patched)
}

// TestRunPublishReleaseReportsError covers the reportError branch
// when PublishRelease fails (here: no token in the environment).
func TestRunPublishReleaseReportsError(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "jeduden/mdsmith")
	t.Setenv("RELEASE_TAG", "v1.2.3")
	t.Setenv("GITHUB_TOKEN", "")

	assert.Equal(t, 1, run([]string{"publish-release"}))
}

// TestRunPublishReleaseFlagParseError covers the
// reportFlagParseErr branch for an unknown flag.
func TestRunPublishReleaseFlagParseError(t *testing.T) {
	assert.Equal(t, 2, run([]string{"publish-release", "--bogus"}))
}

// TestRunPublishReleaseRejectsPositionalArgs covers the
// fs.NArg() != 0 usage branch.
func TestRunPublishReleaseRejectsPositionalArgs(t *testing.T) {
	assert.Equal(t, 2, run([]string{"publish-release", "extra"}))
}
