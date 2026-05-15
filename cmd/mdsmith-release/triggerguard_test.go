package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCheckReleaseTriggerWritesGitHubOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"draft":true}`))
	}))
	t.Cleanup(srv.Close)

	out := filepath.Join(t.TempDir(), "github-output.txt")
	t.Setenv("EVENT_NAME", "create")
	t.Setenv("CREATE_REF_TYPE", "tag")
	t.Setenv("RELEASE_TAG", "v1.2.3")
	t.Setenv("GITHUB_REPOSITORY", "jeduden/mdsmith")
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_API_URL", srv.URL)
	t.Setenv("GITHUB_OUTPUT", out)

	assert.Equal(t, 0, run([]string{"check-release-trigger"}))

	body, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "should_run=true\ncreate_release_is_draft=true\n", string(body))
}
