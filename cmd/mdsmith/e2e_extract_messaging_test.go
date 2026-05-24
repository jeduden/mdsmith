package main_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// messagingKindCfg mirrors the `messaging` kind in the real
// .mdsmith.yml. Duplicating it keeps the test hermetic — the
// real source file's conformance is enforced by
// `mdsmith check .` in CI, while this test pins the schema
// shape and the extract JSON projection.
const messagingKindCfg = `kinds:
  messaging:
    path-pattern: "docs/brand/messaging.md"
    schema:
      frontmatter:
        title: nonEmpty
        summary: nonEmpty
        eyebrow: nonEmpty
        headline-pre: nonEmpty
        headline-em: nonEmpty
        headline-post: nonEmpty
        lead: nonEmpty
        tagline: nonEmpty
        vscode-description: nonEmpty
        claude-code-lsp-description: nonEmpty
        claude-code-skills-description: nonEmpty
        claude-code-audit-description: nonEmpty
      closed: false
      sections:
        - heading: null
        - heading:
            regex: '.+'
            repeat: { min: 0 }
kind-assignment:
  - glob: ["docs/brand/messaging.md"]
    kinds: [messaging]
`

const messagingFixture = `---
title: mdsmith product messaging
summary: Canonical product messaging.
eyebrow: Eyebrow text.
headline-pre: Mark
headline-em: down
headline-post: ", smithed."
lead: Lead text.
tagline: Tagline text.
vscode-description: VS Code description.
claude-code-lsp-description: Claude Code LSP description.
claude-code-skills-description: Claude Code skills description.
claude-code-audit-description: Claude Code audit description.
---
# mdsmith product messaging

Body prose.
`

// expectedMessagingFields lists every frontmatter key the
// messaging kind must project. The sync command in
// internal/release will consume the same set, so adding a
// field requires updating .mdsmith.yml, the real source file,
// this constant, and the sync registry in one change.
var expectedMessagingFields = []string{
	"title",
	"summary",
	"eyebrow",
	"headline-pre",
	"headline-em",
	"headline-post",
	"lead",
	"tagline",
	"vscode-description",
	"claude-code-lsp-description",
	"claude-code-skills-description",
	"claude-code-audit-description",
}

// TestE2E_Extract_Messaging projects a conformant messaging
// file and asserts every documented field lands in the JSON
// under the `frontmatter` object. Catches schema regressions
// and silent field-name drift between the .mdsmith.yml schema
// and the future sync command's consumer list.
func TestE2E_Extract_Messaging(t *testing.T) {
	dir := kindsTestDir(t, messagingKindCfg, map[string]string{
		"docs/brand/messaging.md": messagingFixture,
	})
	stdout, stderr, code := runBinaryInDir(t, dir, "",
		"extract", "messaging", "docs/brand/messaging.md",
		"--format", "json")
	require.Equal(t, 0, code, "stderr=%s", stderr)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))

	fm, ok := got["frontmatter"].(map[string]any)
	require.True(t, ok, "missing frontmatter object in: %v", got)

	for _, key := range expectedMessagingFields {
		v, present := fm[key]
		assert.True(t, present, "missing frontmatter field %q", key)
		s, isString := v.(string)
		assert.True(t, isString, "field %q is not a string: %T", key, v)
		assert.NotEmpty(t, s, "field %q is empty", key)
	}
}
