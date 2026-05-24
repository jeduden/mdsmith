package release

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoRoot resolves the project root from this test file's
// location (two parents up from internal/release/). Used by the
// integration test that shells out to `go run ./cmd/mdsmith`.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

const fullMessagingJSON = `{
  "frontmatter": {
    "title": "mdsmith product messaging",
    "summary": "Canonical product messaging.",
    "headline-pre": "Mark",
    "headline-em": "down",
    "headline-post": ", smithed."
  },
  "eyebrow": { "text": "Markdown as a single source of truth" },
  "lead": { "text": "Lead text." },
  "tagline": { "text": "Tagline text." },
  "vscode-description": { "text": "VS Code description." },
  "claude-code-lsp-description": { "text": "LSP description." },
  "claude-code-skills-description": { "text": "Skills description." },
  "claude-code-audit-description": { "text": "Audit description." }
}`

func stubMessagingExtractor(t *testing.T, body []byte, err error) {
	t.Helper()
	prev := messagingExtractor
	t.Cleanup(func() { messagingExtractor = prev })
	messagingExtractor = func(string) ([]byte, error) { return body, err }
}

func TestLoadMessaging_DecodesAllFields(t *testing.T) {
	stubMessagingExtractor(t, []byte(fullMessagingJSON), nil)
	m, err := LoadMessaging("ignored")
	require.NoError(t, err)
	assert.Equal(t, "mdsmith product messaging", m.Title)
	assert.Equal(t, "Markdown as a single source of truth", m.Eyebrow)
	assert.Equal(t, "Mark", m.HeadlinePre)
	assert.Equal(t, "down", m.HeadlineEm)
	assert.Equal(t, ", smithed.", m.HeadlinePost)
	assert.Equal(t, "Lead text.", m.Lead)
	assert.Equal(t, "Tagline text.", m.Tagline)
	assert.Equal(t, "VS Code description.", m.VSCodeDescription)
	assert.Equal(t, "LSP description.", m.ClaudeCodeLSPDescription)
	assert.Equal(t, "Skills description.", m.ClaudeCodeSkillsDescription)
	assert.Equal(t, "Audit description.", m.ClaudeCodeAuditDescription)
}

func TestLoadMessaging_EmptyFieldFails(t *testing.T) {
	partial := `{"frontmatter": {"title": "ok", "summary": "ok"}}`
	stubMessagingExtractor(t, []byte(partial), nil)
	_, err := LoadMessaging("ignored")
	require.Error(t, err)
	msg := err.Error()
	for _, want := range []string{
		"eyebrow", "headline-pre", "headline-em", "headline-post",
		"lead", "tagline", "vscode-description",
		"claude-code-lsp-description", "claude-code-skills-description",
		"claude-code-audit-description",
	} {
		assert.Contains(t, msg, want)
	}
}

func TestLoadMessaging_WhitespaceOnlyFieldFails(t *testing.T) {
	// All required fields present but `lead` is just whitespace.
	bad := `{"frontmatter": {
		"title": "t", "summary": "s", "eyebrow": "e",
		"headline-pre": "p", "headline-em": "m", "headline-post": "x",
		"lead": "   \t\n", "tagline": "tg",
		"vscode-description": "v",
		"claude-code-lsp-description": "l",
		"claude-code-skills-description": "sk",
		"claude-code-audit-description": "a"
	}}`
	stubMessagingExtractor(t, []byte(bad), nil)
	_, err := LoadMessaging("ignored")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lead")
}

func TestLoadMessaging_BadJSON(t *testing.T) {
	stubMessagingExtractor(t, []byte("not json"), nil)
	_, err := LoadMessaging("ignored")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode messaging json")
}

func TestLoadMessaging_ExtractorError(t *testing.T) {
	stubMessagingExtractor(t, nil, errors.New("boom"))
	_, err := LoadMessaging("ignored")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

// TestRunMdsmithExtract_AgainstRepoRoot exercises the real
// shell-out path (the variable `messagingExtractor` defaults to
// `runMdsmithExtract`). It runs `go run ./cmd/mdsmith extract
// messaging docs/brand/messaging.md --format json` against the
// actual repository, decoding the result to make sure the
// command-line wiring still produces the JSON envelope that
// LoadMessaging expects. The other LoadMessaging tests use a
// stub; this one covers the production code path.
func TestRunMdsmithExtract_AgainstRepoRoot(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles cmd/mdsmith; skipped under -short")
	}
	root := repoRoot(t)
	out, err := runMdsmithExtract(root)
	require.NoError(t, err)
	// Frontmatter holds metadata; prose lives in top-level
	// body-section objects, each with a `text` field
	// (paragraph projection).
	var envelope struct {
		Frontmatter map[string]any `json:"frontmatter"`
		Tagline     struct {
			Text string `json:"text"`
		} `json:"tagline"`
		Lead struct {
			Text string `json:"text"`
		} `json:"lead"`
	}
	require.NoError(t, json.Unmarshal(out, &envelope))
	assert.NotEmpty(t, envelope.Frontmatter["title"])
	assert.NotEmpty(t, envelope.Tagline.Text)
	assert.NotEmpty(t, envelope.Lead.Text)
}

// TestRunMdsmithExtract_NonRepoCwd hits the ExitError branch:
// pointing the shell-out at a tempdir without a go.mod makes
// `go run` exit with status 1, so runMdsmithExtract formats and
// returns the ExitError-wrapped failure.
func TestRunMdsmithExtract_NonRepoCwd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles cmd/mdsmith; skipped under -short")
	}
	_, err := runMdsmithExtract(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mdsmith extract messaging")
}

// TestRunMdsmithExtract_BadExecutable hits the non-ExitError
// fallback: with PATH cleared, exec.LookPath can't find `go`,
// so cmd.Output returns an exec.ErrNotFound (NOT an ExitError),
// and the fallback fmt.Errorf branch runs.
func TestRunMdsmithExtract_BadExecutable(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := runMdsmithExtract(repoRoot(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mdsmith extract messaging")
}
