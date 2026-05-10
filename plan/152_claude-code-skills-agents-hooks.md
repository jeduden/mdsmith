---
id: 152
title: Claude Code plugin extensions — skills, agents, hooks
status: "🔳"
model: sonnet
summary: >-
  Extend the Claude Code marketplace from plan 132
  with a second plugin (`mdsmith-tools`) that bundles
  three slash-command skills (`fix`, `kinds`,
  `check`), a `markdown-reviewer` subagent, and a
  post-edit lint hook so Claude Code users get a
  richer Markdown workflow beyond the LSP-only plugin.
---
# Claude Code plugin extensions — skills, agents, hooks

## Goal

Ship a second plugin in the mdsmith Claude Code
marketplace from plan 132. The new plugin adds
the three component types the LSP plugin omits:
skills, subagents, and hooks. Skills run mdsmith
workflows by name. The subagent delegates
Markdown review. The hook lints files
automatically after every edit. None of these
work from the bare LSP server.

## Background

Plan 132 ships `mdsmith-lsp`. That plugin is LSP
only. The marketplace lives at the repo root.
The Claude Code [plugin reference][plug-ref]
defines five component types:

[plug-ref]: https://code.claude.com/docs/en/plugins-reference

| Component   | Plan 132 | This plan |
|-------------|----------|-----------|
| LSP servers | ✅       | —         |
| Skills      | —        | ✅        |
| Agents      | —        | ✅        |
| Hooks       | —        | ✅        |
| MCP servers | —        | (skip)    |

LSP and skills/agents/hooks separate cleanly.
A user who wants only diagnostics installs
`mdsmith-lsp`. A user who wants the full
workflow installs both. The split mirrors the
official-marketplace convention. `gopls-lsp`
ships LSP only; richer Go tooling lives in
separate plugins.

## Non-Goals

- An MCP server. The LSP already exposes
  everything an editor or agent needs.
- Bundling binaries. Plan 130 covers that.
- Output styles or themes. No clear use case.
- A combined single plugin. Splitting LSP from
  the rest lets users install only what they
  want.

## Design

### Repository layout

```text
.claude-plugin/
  marketplace.json          # adds mdsmith-tools entry
editors/
  claude-code/              # mdsmith-lsp (plan 132)
  claude-code-tools/        # mdsmith-tools (this plan)
    .claude-plugin/
      plugin.json
    skills/
      fix/SKILL.md
      kinds/SKILL.md
      check/SKILL.md
    agents/
      markdown-reviewer.md
    hooks/
      hooks.json
    README.md
```

The `editors/claude-code-tools/` directory mirrors
the layout of `editors/claude-code/`. The
marketplace.json from plan 132 grows a second
entry pointing at it.

### Marketplace update

The full `marketplace.json` (per plan 132) keeps
its `name`, `owner`, `description`, and
`$schema` fields. Only the `plugins` array
grows. The block below is a fragment, not a
complete file:

```json
{
  "plugins": [
    {
      "name": "mdsmith-lsp",
      "source": "./editors/claude-code",
      "description": "Inline mdsmith diagnostics and Markdown navigation"
    },
    {
      "name": "mdsmith-tools",
      "source": "./editors/claude-code-tools",
      "description": "Markdown skills, reviewer agent, and post-edit lint hook"
    }
  ]
}
```

### Skills

Each skill is a directory under `skills/` with a
`SKILL.md` file. Skills are namespaced by plugin:
`/mdsmith-tools:fix`, `/mdsmith-tools:kinds`,
`/mdsmith-tools:check`.

| Skill   | Body                                                  | Wraps                                             |
|---------|-------------------------------------------------------|---------------------------------------------------|
| `fix`   | Run `mdsmith fix .` on the workspace, summarize stats | [`mdsmith fix`](../docs/reference/cli/fix.md)     |
| `kinds` | Show kind assignments and resolved rule config        | [`mdsmith kinds`](../docs/reference/cli/kinds.md) |
| `check` | Run `mdsmith check .` and show diagnostics            | [`mdsmith check`](../docs/reference/cli/check.md) |

`fix` is the highest-leverage skill: when an agent
edits Markdown across many files, one command
applies all the auto-fixes. `kinds` answers "what
rules apply to this file?". `check` is a deliberate
duplicate of LSP diagnostics for users who installed
`mdsmith-tools` without `mdsmith-lsp`.

Each `SKILL.md` follows the schema documented in
the Claude Code [Skills][skills-ref] reference.
Front matter sets `name`, `description`, and an
optional `tools` list. The body explains when to
invoke and what to run.

[skills-ref]: https://code.claude.com/docs/en/skills

### Agent: `markdown-reviewer`

A specialised subagent for reviewing Markdown
pull requests and document drafts. Tools: Read,
Grep, Bash (`mdsmith check`), GitHub MCP for PR
comments. Description steers Claude to delegate
when a task involves "review this docs PR" or
"check this Markdown for style issues".

The agent shells out to `mdsmith check --json`
and `mdsmith metrics` to surface readability,
length, and structural issues, then writes a
structured review summary. It does not auto-fix
— users delegate that to the `fix` skill or to
the LSP `source.fixAll.mdsmith` action.

### Hooks

One hook: `PostToolUse` on `Edit` and `Write` of
`*.md` files. The hook runs `mdsmith fix` on the
single file that was just edited.

```jsonc
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "F=$(jq -r '.tool_input.file_path // empty'); case \"$F\" in *.md|*.markdown) mdsmith fix -- \"$F\";; esac"
          }
        ]
      }
    ]
  }
}
```

The hook receives tool input as JSON on stdin.
`jq` extracts `tool_input.file_path`. The `case`
filters to Markdown extensions. The `--`
terminator stops the path from being parsed as
a flag.

The hook is opt-in via the plugin's enable/disable
toggle. Users who prefer to run `fix` manually
disable the plugin or fork it without the hook.

The hook does not fire on `MultiEdit` because the
LSP `source.fixAll.mdsmith` action covers the
multi-buffer case more cleanly. Document this in
the plugin README so users are not surprised.

### `plugin.json`

```json
{
  "$schema": "https://json.schemastore.org/claude-code-plugin-manifest.json",
  "name": "mdsmith-tools",
  "description": "Markdown skills, reviewer agent, and post-edit lint hook",
  "homepage": "https://github.com/jeduden/mdsmith",
  "repository": "https://github.com/jeduden/mdsmith",
  "license": "MIT",
  "keywords": ["markdown", "review", "lint"],
  "skills": "./skills/",
  "agents": ["./agents/markdown-reviewer.md"],
  "hooks": "./hooks/hooks.json"
}
```

No inline component definitions; each component
loads from its standard location so the manifest
stays small.

### Backwards compatibility

`mdsmith-lsp` (plan 132) is unchanged. The
marketplace.json gains one entry. Users who
already installed `mdsmith-lsp` see the new
plugin in `/plugin marketplace update` output
but it does not auto-install.

## Tasks

1. Create the `editors/claude-code-tools/`
   directory with the layout above.
2. Add the `plugin.json` manifest declaring
   `skills`, `agents`, and `hooks` paths.
3. Write the three SKILL.md files (`fix`,
   `kinds`, `check`). Each shells out to the
   matching CLI subcommand and surfaces stderr
   on non-zero exit.
4. Write `agents/markdown-reviewer.md` with the
   agent definition. Front matter plus a body
   describing capabilities and triggers.
5. Add `hooks/hooks.json` with the `PostToolUse`
   entry.
6. Extend the marketplace.json from plan 132 to
   include the new plugin entry.
7. Update [.mdsmith.yml](../.mdsmith.yml)'s
   `directory-structure.allowed` if needed to
   cover the new path. `editors/**` is already
   allowed (per plan 132's audit), so no change
   should be required — confirm during smoke
   test.
8. Extend [.mdsmith.yml](../.mdsmith.yml)'s
   `kind-assignment` block (and the matching
   `overrides:` entries that disable
   `first-line-heading` and `heading-increment`
   for SKILL.md files) to cover
   `editors/claude-code-tools/skills/*/SKILL.md`.
   The existing `skill` kind currently targets
   only `.claude/skills/*/SKILL.md`; without
   the extension, the new SKILL.md files would
   fail those rules. Surface the diff per
   [CLAUDE.md](../CLAUDE.md) before applying.
9. Add a `README.md` covering install, the three
   skills, the agent, the hook, and the binary
   prerequisite.
10. Document the new plugin in
    [docs/guides/install.md](../docs/guides/install.md)
    under the Claude Code section plan 132 added.
11. Smoke-test end-to-end: install `mdsmith-tools`
    in a scratch workspace, run each skill, invoke
    the agent on a sample Markdown file, edit a
    `.md` file and verify the post-edit hook
    runs `mdsmith fix`.

## Acceptance Criteria

- [ ] `/plugin install mdsmith-tools@mdsmith`
      succeeds after `/reload-plugins`.
- [ ] `/mdsmith-tools:fix` runs `mdsmith fix .`
      and reports the fix-of-total count.
- [ ] `/mdsmith-tools:kinds` shows kind
      assignments for the current file.
- [ ] `/mdsmith-tools:check` runs
      `mdsmith check .` and surfaces diagnostics
      to the agent.
- [ ] The `markdown-reviewer` agent appears in
      the agent picker and produces a structured
      review summary on a sample Markdown PR.
- [ ] After editing a `.md` file via Claude
      Code's Edit or Write tool, `mdsmith fix`
      runs automatically on that file.
- [ ] `claude plugin validate
      ./editors/claude-code-tools` reports no
      errors.
- [ ] [docs/guides/install.md](../docs/guides/install.md)
      documents both `mdsmith-lsp` and
      `mdsmith-tools` install paths.
- [ ] All tests pass: `go test ./...`.
- [ ] `mdsmith check .` passes including the new
      manifests and SKILL.md files.

## Open Questions

- **Hook scope.** Auto-`fix` on every Edit may
  surprise users who prefer manual fixes. The
  hook is opt-in via plugin enable; a per-file
  override may be needed later.
- **Agent vs. skill for review.** The
  `markdown-reviewer` agent is heavyweight; a
  `/mdsmith-tools:review` skill might cover
  most cases more cheaply.

## ...

<?allow-empty-section?>
