---
id: 72
title: Fix skill formatting and add validation
status: "✅"
summary: Fix pr-fixup skill directory layout for Claude Code and add mdsmith validation for skill files.
---
# Fix skill formatting and add validation

## Goal

Fix the pr-fixup skill so Claude Code discovers it.
Claude Code requires `SKILL.md` inside a directory.
Add mdsmith validation for skill files using the
existing `required-structure` rule with a template.

## Context

Claude Code skills must live at
`.claude/skills/<name>/SKILL.md` (a directory per skill
with `SKILL.md` as entry point). The current pr-fixup
skill is a flat file at `.claude/skills/pr-fixup.md`,
so Claude cannot discover it.

Skills support optional YAML frontmatter with fields
like `name`, `description`, `user-invocable`,
`allowed-tools`, `disable-model-invocation`, `model`,
`context`, `agent`, `argument-hint`.

The repo has parallel agent instruction files for
Copilot and OpenAI Codex. These are plain markdown
without frontmatter. Their existing overrides in
`.mdsmith.yml` are sufficient.

## Tasks

1. Move `.claude/skills/pr-fixup.md` to
   `.claude/skills/pr-fixup/SKILL.md`
2. Add YAML frontmatter to the skill with `name` and
   `description` fields
3. Create a skill template at
   `.claude/skills/proto.md` that validates Claude
   skill frontmatter via the `required-structure` rule
   and CUE schema; keep it generic (no rule-specific
   fields — just the fields Claude Code expects:
   `name`, `description`, optional booleans/strings)
4. Add an override in `.mdsmith.yml` for
   `.claude/skills/*/SKILL.md` that applies the skill
   template via `required-structure`, and adjusts
   `max-file-length`, `first-line-heading`,
   `heading-increment` as needed (skills often start
   with `##` not `#`, and include generated content)
5. Update the existing `.mdsmith.yml` override that
   references `.claude/skills/pr-fixup.md` to point to
   the new path `.claude/skills/pr-fixup/SKILL.md`
6. Update `.claude/settings.json` if it references the
   old skill path
7. Run `mdsmith check .` to verify all files pass
8. Run `go test ./...` to verify no tests break

## Acceptance Criteria

- [ ] Claude Code discovers pr-fixup skill (file at
      `.claude/skills/pr-fixup/SKILL.md`)
- [ ] Skill has valid YAML frontmatter with `name` and
      `description`
- [ ] `required-structure` template validates skill
      frontmatter schema (CUE)
- [ ] `mdsmith check .` passes
- [ ] `go test ./...` passes
