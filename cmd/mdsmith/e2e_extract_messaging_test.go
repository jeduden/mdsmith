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
//
// Per the extract-markdown-as-data guide, prose fields
// (eyebrow, lead, tagline, per-surface descriptions) live in
// H2 body sections; only the metadata that other tools read
// as structured (title, summary, the website hero's headline
// triple) stays in frontmatter.
const messagingKindCfg = `kinds:
  messaging:
    schema:
      frontmatter:
        title: nonEmpty
        summary: nonEmpty
        headline-pre: nonEmpty
        headline-em: nonEmpty
        headline-post: nonEmpty
      closed: false
      sections:
        - heading: null
        - heading: { regex: '^Eyebrow$' }
          content:
            - { kind: paragraph, required: true }
        - heading: { regex: '^Lead$' }
          content:
            - { kind: paragraph, required: true }
        - heading: { regex: '^Tagline$' }
          content:
            - { kind: paragraph, required: true }
        - heading: { regex: '^VS Code$' }
          bind: vscode-description
          content:
            - { kind: paragraph, required: true }
        - heading: { regex: '^Claude Code LSP$' }
          bind: claude-code-lsp-description
          content:
            - { kind: paragraph, required: true }
        - heading: { regex: '^Claude Code skills$' }
          bind: claude-code-skills-description
          content:
            - { kind: paragraph, required: true }
        - heading: { regex: '^Claude Code audit$' }
          bind: claude-code-audit-description
          content:
            - { kind: paragraph, required: true }
kind-assignment:
  - glob: ["docs/brand/messaging.md"]
    kinds: [messaging]
`

const messagingFixture = `---
title: mdsmith product messaging
summary: Canonical product messaging.
headline-pre: Mark
headline-em: down
headline-post: ", smithed."
---
# mdsmith product messaging

## Eyebrow

Eyebrow text.

## Lead

Lead text.

## Tagline

Tagline text.

## VS Code

VS Code description.

## Claude Code LSP

Claude Code LSP description.

## Claude Code skills

Claude Code skills description.

## Claude Code audit

Claude Code audit description.
`

// expectedMessagingFrontmatter lists every key the messaging
// kind must project under the JSON root's `frontmatter` object.
var expectedMessagingFrontmatter = []string{
	"title",
	"summary",
	"headline-pre",
	"headline-em",
	"headline-post",
}

// expectedMessagingSections lists every top-level body-section
// key the messaging kind must project (each carries a `text`
// field — the paragraph under the H2). Adding a field requires
// updating .mdsmith.yml, the real source file, this constant,
// and the sync registry in one change.
var expectedMessagingSections = []string{
	"eyebrow",
	"lead",
	"tagline",
	"vscode-description",
	"claude-code-lsp-description",
	"claude-code-skills-description",
	"claude-code-audit-description",
}

// TestE2E_Extract_Messaging projects a conformant messaging
// file and asserts every documented field lands in the JSON
// at its expected location (frontmatter scalars vs.
// section-text projections). Catches schema regressions and
// silent field-name drift between the .mdsmith.yml schema and
// the sync command's consumer list.
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
	for _, key := range expectedMessagingFrontmatter {
		v, present := fm[key]
		assert.True(t, present, "missing frontmatter field %q", key)
		s, isString := v.(string)
		assert.True(t, isString, "field %q is not a string: %T", key, v)
		assert.NotEmpty(t, s, "field %q is empty", key)
	}
	for _, key := range expectedMessagingSections {
		sec, present := got[key].(map[string]any)
		assert.True(t, present, "missing section %q at root", key)
		text, isString := sec["text"].(string)
		assert.True(t, isString, "section %q text not a string: %T", key, sec["text"])
		assert.NotEmpty(t, text, "section %q text is empty", key)
	}
}
