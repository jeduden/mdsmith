---
id: 160
title: Claude Code plugin extensions — skills, agents, hooks
status: "🔳"
model: sonnet
depends-on: [161]
summary: >-
  Add three plugins to the Claude Code marketplace
  alongside the existing `mdsmith-lsp`,
  `mdsmith-audit`, and `mdsmith-dev-lsp`:
  `mdsmith-skills` (slash-commands `/mdsmith-fix`,
  `/mdsmith-kinds`, `/mdsmith-check`),
  `mdsmith-reviewer`
  (`markdown-reviewer` subagent), and
  `mdsmith-autofix` (post-edit lint hook). Follows
  the one-plugin-per-component-kind precedent set
  by `mdsmith-audit`.
---
# Claude Code plugin extensions — skills, agents, hooks

## Goal

Ship three plugins covering skills, subagents,
hooks.

## Background

The [marketplace][mp] already lists three plugins:

- `mdsmith-lsp` — LSP server (plan 132).
- `mdsmith-audit` — `markdown-audit` skill.
- `mdsmith-dev-lsp` — gopls, TypeScript, mdsmith
  LSPs for contributors.

`mdsmith-audit` set the convention: one plugin
per component bundle. Three new plugins below
cover the three component types missing so far
(skills, agents, hooks). MCP is out of scope.

[mp]: ../.claude-plugin/marketplace.json
[cc-audit]: ../editors/claude-code-audit
[audit-skill]: ../editors/claude-code-audit/skills/markdown-audit/SKILL.md
[audit-manifest]: ../editors/claude-code-audit/.claude-plugin/plugin.json
[audit-readme]: ../editors/claude-code-audit/README.md
[install-guide]: ../docs/guides/install.md
[settings]: ../.claude/settings.json
[mdsmith-yml]: ../.mdsmith.yml
[claude-md]: ../CLAUDE.md
[proto]: ../.claude/skills/proto.md
[skills-ref]: https://code.claude.com/docs/en/skills

## Non-Goals

- MCP server (LSP covers what agents need),
  bundling binaries (plan 130), a monolithic
  `mdsmith-tools` plugin, or publishing
  `.claude/skills/` contributor skills.

## Design

### Repository layout

Each new directory mirrors [audit][cc-audit]:

```text
editors/
  claude-code-skills/
    .claude-plugin/plugin.json
    skills/mdsmith-fix/SKILL.md
    skills/mdsmith-kinds/SKILL.md
    skills/mdsmith-check/SKILL.md
    README.md
  claude-code-reviewer/
    .claude-plugin/plugin.json
    agents/markdown-reviewer.md
    patterns.md
    README.md
  claude-code-autofix/
    .claude-plugin/plugin.json
    hooks/hooks.json
    README.md
```

### Marketplace update

Fragment of `plugins`:

```json
{
  "plugins": [
    {
      "name": "mdsmith-skills",
      "source": "./editors/claude-code-skills",
      "description": "Slash-command skills for mdsmith fix, kinds, and check"
    },
    {
      "name": "mdsmith-reviewer",
      "source": "./editors/claude-code-reviewer",
      "description": "Subagent that reviews Markdown PRs and drafts"
    },
    {
      "name": "mdsmith-autofix",
      "source": "./editors/claude-code-autofix",
      "description": "Post-edit hook that runs mdsmith fix on .md files"
    }
  ]
}
```

### `mdsmith-skills`

Three slash-commands under `skills/`. Skills
are invoked by `name`, with no plugin prefix
(per [the Skills docs][skills-ref]; audit uses
`/markdown-audit`). SKILL names carry an
`mdsmith-` prefix to stay distinctive:

| Slash command    | Wraps                                             |
|------------------|---------------------------------------------------|
| `/mdsmith-fix`   | [`mdsmith fix`](../docs/reference/cli/fix.md)     |
| `/mdsmith-kinds` | [`mdsmith kinds`](../docs/reference/cli/kinds.md) |
| `/mdsmith-check` | [`mdsmith check`](../docs/reference/cli/check.md) |

`/mdsmith-fix` is the highest-leverage.
`/mdsmith-check` deliberately duplicates LSP
diagnostics for users without `mdsmith-lsp`.

Each `SKILL.md` follows the [Skills][skills-ref]
schema and [proto.md][proto]. Front matter sets
`name`, `description`, `user-invocable: true`,
`argument-hint`, and an `allowed-tools` allowlist
scoped to `Bash(mdsmith:*)`. Front-matter and
structure follow [the audit SKILL.md][audit-skill]
pattern; bodies are command-specific (no
`<?include?>`). No local copy in
`.claude/skills/`.

### `mdsmith-reviewer`

One subagent in `agents/markdown-reviewer.md`
plus a sibling `patterns.md` at the plugin
root, outside `agents/` so the loader treats
it as data. `patterns.md` ships the three
config-level checks (no `.mdsmith.yml`,
similar files without a kind, kind without
`path-pattern`); installed users don't need
the source-tree audit skill. Rule-backed
patterns load via `mdsmith help patterns -f
json` or LSP `mdsmith/rulePatterns` (see
[plan 161][p161]). The agent proposes the
rule, directive, or kind config to **adopt**
so the pattern stops drifting; content nits
stay with `mdsmith check`. Tools: Read, Grep,
Bash (`mdsmith help`, `mdsmith check -f
json`, `mdsmith kinds resolve`), GitHub MCP.
No auto-fix.

[p161]: ./161_rule-pattern-metadata.md

### `mdsmith-autofix`

One `PostToolUse` hook in `hooks/hooks.json`.
Matches `Edit`, `Write`, and `MultiEdit` on
`.md` files. Runs `mdsmith fix` on the edited
file.

Claude Code passes tool input as JSON on stdin
(see [`.claude/settings.json`][settings] for
pattern). The file path is at
`.tool_input.file_path`. The wrapping `hooks`
array allows multiple per-matcher commands.

```jsonc
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write|MultiEdit",
        "hooks": [
          {
            "type": "command",
            "command": "FILE=$(jq -re '.tool_input.file_path // empty') || { echo 'hook: jq missing or no file_path' >&2; exit 1; }; case \"$FILE\" in *.md|*.markdown) mdsmith fix -- \"${FILE#\"$PWD/\"}\" || true;; esac"
          }
        ]
      }
    ]
  }
}
```

`${FILE#"$PWD/"}` strips the workspace prefix
so relative globs match; the quotes force a
literal prefix (unquoted `$PWD` would
glob-interpret `[`, `*`, `?`). `-re` plus
`// empty` and the `||` guard make jq /
missing-`file_path` failures exit 1 with a
stderr warning. `|| true` covers only
`mdsmith fix`; `--` blocks flag parsing;
`case` skips non-Markdown. `mdsmith` and `jq`
must be on the PATH Claude Code sees.

The hook is opt-in. Users who prefer manual
`fix` just do not install `mdsmith-autofix`.

### `plugin.json` manifests

Same shape as [the audit
manifest][audit-manifest]. Vary `name`,
`description`, and `keywords` per plugin:
`["markdown", "skills", "mdsmith"]`,
`["markdown", "review", "agent"]`,
`["markdown", "hook", "autofix"]`. No inline
component declarations — components load from
the standard `skills/`, `agents/`,
`hooks/hooks.json` paths.

### Kind-assignment for plugin SKILL.md

`.mdsmith.yml` names the audit path four
times. Look in the `skill` kind, in
`kind-assignment`, and in two SKILL
`overrides:`. Swap each
`editors/claude-code-audit/skills/*/SKILL.md`
glob for `editors/claude-code-*/skills/*/SKILL.md`.
Surface the diff per [CLAUDE.md][claude-md].
Existing plugins are unchanged; new ones
appear in `/plugin marketplace update`
without auto-installing.

## Tasks

1. Create `editors/claude-code-skills/`,
   `editors/claude-code-reviewer/`, and
   `editors/claude-code-autofix/` with the
   layouts above.
2. Add a `plugin.json` to each, matching the
   shapes above.
3. Write the three SKILL.md files
   (`mdsmith-fix`, `mdsmith-kinds`,
   `mdsmith-check`) under
   `editors/claude-code-skills/skills/`. Each
   shells out to the matching `mdsmith`
   subcommand; surface stderr on non-zero exit.
4. Write
   `editors/claude-code-reviewer/agents/markdown-reviewer.md`.
   Subagent front matter uses `tools` (not the
   skill-style `allowed-tools`); body describes
   capabilities and triggers.
5. Add
   `editors/claude-code-autofix/hooks/hooks.json`
   using the inner-`hooks`-array format above.
6. Extend [`marketplace.json`][mp] with the
   three new entries and broaden the top-level
   `description` past "via LSP" to cover skills,
   the reviewer, and the autofix hook.
7. Generalize the four audit-specific
   `editors/claude-code-audit/skills/*/SKILL.md`
   entries in [`.mdsmith.yml`][mdsmith-yml]
   (path-pattern, kind-assignment, two overrides)
   to `editors/claude-code-*/skills/*/SKILL.md`.
   Surface the diff per [CLAUDE.md][claude-md].
8. Add a `README.md` per plugin (see [audit
   README][audit-readme]): install commands,
   contents, PATH prereq (`mdsmith`; plus `jq`
   for autofix), and the autofix opt-out.
9. Document the three new plugins in the
   [install guide][install-guide] under the
   Claude Code section.
10. Smoke-test: install each plugin, run
    `/mdsmith-fix`, `/mdsmith-kinds`, and
    `/mdsmith-check`, invoke `markdown-reviewer`
    on a sample file, and verify the hook fires
    on Edit, Write, and MultiEdit.

## Acceptance Criteria

- [ ] `/plugin install mdsmith-skills@mdsmith`,
      `mdsmith-reviewer@mdsmith`, and
      `mdsmith-autofix@mdsmith` each succeed
      after `/reload-plugins`.
- [ ] `/mdsmith-fix`, `/mdsmith-kinds`, and
      `/mdsmith-check` each run their matching
      `mdsmith` subcommand and surface output.
- [ ] `markdown-reviewer` produces a structured
      review summary on a sample Markdown PR;
      it pulls rule-backed patterns via
      `mdsmith help patterns` (no hard-coded
      list — verified by adding a new rule
      and seeing it picked up unchanged) and
      surfaces the three config-level checks
      from sibling `patterns.md` (fixture:
      missing `.mdsmith.yml`, similar files
      without a kind, kind without
      `path-pattern`).
- [ ] After Edit/Write/MultiEdit on a `.md`
      file, `mdsmith fix` runs on it. Verified
      with an absolute path under a workspace
      containing `[`, `*`, or `?` — `mdsmith`
      must receive the workspace-relative path.
- [ ] `claude plugin validate` passes on each
      new manifest; `mdsmith kinds resolve`
      reports `skill` for audit and
      `mdsmith-skills` SKILL.md files.
- [x] The [install guide][install-guide]
      documents all six plugins.
- [x] `go test ./...` and `mdsmith check .`
      pass.

## Open Questions

- **Bundle vs split.** Revisit if install count
  becomes painful.
- **Agent vs. skill.** A `/mdsmith-review`
  skill might cover most cases more cheaply.

## ...

<?allow-empty-section?>
