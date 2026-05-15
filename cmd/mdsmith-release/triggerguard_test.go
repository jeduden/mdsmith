package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/release"
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

// TestRunCheckReleaseTriggerReportsCheckError covers the
// reportError branch when CheckReleaseTrigger itself fails
// (here: create-event tag without a token).
func TestRunCheckReleaseTriggerReportsCheckError(t *testing.T) {
	t.Setenv("EVENT_NAME", "create")
	t.Setenv("CREATE_REF_TYPE", "tag")
	t.Setenv("RELEASE_TAG", "v1.2.3")
	t.Setenv("GITHUB_REPOSITORY", "jeduden/mdsmith")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GITHUB_OUTPUT", "")

	assert.Equal(t, 1, run([]string{"check-release-trigger"}))
}

// TestRunCheckReleaseTriggerReportsWriteError covers the
// reportError branch when writeReleaseTriggerGuardOutput fails
// (here: GITHUB_OUTPUT points at a directory).
func TestRunCheckReleaseTriggerReportsWriteError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EVENT_NAME", "push")
	t.Setenv("GITHUB_OUTPUT", dir)

	assert.Equal(t, 1, run([]string{"check-release-trigger"}))
}

// TestWriteReleaseTriggerGuardOutputStdout covers the path=="" branch
// (no GITHUB_OUTPUT set) which writes to the supplied stdout writer,
// asserting the exact formatted output.
func TestWriteReleaseTriggerGuardOutputStdout(t *testing.T) {
	var buf bytes.Buffer
	err := writeReleaseTriggerGuardOutput(&buf, "", release.TriggerGuardResult{
		ShouldRun:            true,
		CreateReleaseIsDraft: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "should_run=true\ncreate_release_is_draft=true\n", buf.String())
}

// TestWriteReleaseTriggerGuardOutputOpenError covers the
// os.OpenFile error branch.
func TestWriteReleaseTriggerGuardOutputOpenError(t *testing.T) {
	dir := t.TempDir()
	err := writeReleaseTriggerGuardOutput(io.Discard, dir, release.TriggerGuardResult{})
	require.Error(t, err)
}
