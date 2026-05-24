package release

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fullMessagingJSON = `{
  "frontmatter": {
    "title": "mdsmith product messaging",
    "summary": "Canonical product messaging.",
    "eyebrow": "Markdown as a single source of truth",
    "headline-pre": "Mark",
    "headline-em": "down",
    "headline-post": ", smithed.",
    "lead": "Lead text.",
    "tagline": "Tagline text.",
    "vscode-description": "VS Code description.",
    "claude-code-lsp-description": "LSP description.",
    "claude-code-skills-description": "Skills description.",
    "claude-code-audit-description": "Audit description."
  }
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
