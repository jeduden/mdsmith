---
id: 96
title: Adopt kinds in mdsmith repo and ship the docs
status: "🔲"
summary: >-
  Declare the kinds this repo needs, drop the four
  `proto.md` ignore entries, and ship the user-facing
  docs (file-kinds guide, Hugo terminology note).
---
# Adopt kinds in mdsmith repo and ship the docs

## Goal

Prove the kinds and placeholder-grammar machinery
work end-to-end on this repo. Drop the four `proto.md`
entries from `.mdsmith.yml` `ignore:` and confirm
`mdsmith check .` stays green. Ship the user-facing
docs at the same time so the new shape is
discoverable.

## Background

`.mdsmith.yml` currently has these four ignore
entries, all schema/template files:

- [.claude/skills/proto.md](../.claude/skills/proto.md)
- [plan/proto.md](../plan/proto.md)
- [internal/rules/proto.md](../internal/rules/proto.md)
- [docs/security/proto.md](../docs/security/proto.md)

Plus the `cross-file-reference-integrity` exclude on
the placeholder link in `internal/rules/proto.md`
([line 47 of .mdsmith.yml](../.mdsmith.yml)).

Once kinds + placeholder grammar are in place, all
four files can be linted under their own kinds with
appropriate `placeholders:` settings.

## Design

### Kinds this repo needs

Project-chosen names; a representative starter set:

```yaml
kinds:
  proto:
    rules:
      first-line-heading:
        placeholders: [var-token, heading-question,
                       placeholder-section]
      cross-file-reference-integrity:
        placeholders: [var-token]
      paragraph-readability: false
      paragraph-structure: false
      no-emphasis-as-heading: false
      front-matter: false
  plan:
    rules:
      required-structure:
        schema: plan/proto.md
  rule-readme:
    rules:
      required-structure:
        schema: internal/rules/proto.md
  skill:
    rules:
      required-structure:
        schema: .claude/skills/proto.md
  security-note:
    rules:
      required-structure:
        schema: docs/security/proto.md
```

Glob assignment binds files to kinds:

```yaml
kind-assignment:
  - files: ["**/proto.md"]
    kinds: [proto]
  - files: ["plan/[0-9]*_*.md"]    # excludes proto
    kinds: [plan]
  - files: ["internal/rules/MDS*/README.md"]
    kinds: [rule-readme]
  - files: [".claude/skills/*/SKILL.md"]
    kinds: [skill]
  - files: ["docs/security/[0-9]*.md"]   # date-named
    kinds: [security-note]
```

### Docs

Three new pieces:

1. `docs/guides/file-kinds.md` — user guide for kinds:
   declaration, assignment, merge order, conflict
   resolution, `mdsmith config show` workflow.
2. `docs/background/archetypes/placeholder-grammar/`
   — already produced by plan 93; this plan only links
   to it from rule READMEs.
3. Hugo terminology note added to the Hugo migration
   guide and the archetypes README, located at
   `docs/guides/directives/hugo-migration.md` and
   `docs/background/archetypes/README.md`: mdsmith
   *kind* ≈ Hugo *type*; mdsmith *archetype* is a
   rule-mechanics pattern, not Hugo's scaffold.

## Tasks

1. Add the kind declarations and `kind-assignment:`
   entries shown above to `.mdsmith.yml`.
2. Drop the four `proto.md` entries from `ignore:`
   and the placeholder-link entry from
   `cross-file-reference-integrity.exclude:`.
3. Confirm `mdsmith check .` stays green; iterate on
   kind bodies if any rule still flags a placeholder
   that's part of a real schema/template file.
4. Write `docs/guides/file-kinds.md` covering kind
   declaration, assignment (front matter + globs),
   merge order, and conflict resolution. Walk through
   `mdsmith config show <file>` as the troubleshooting
   path.
5. Add the Hugo terminology note to
   `docs/guides/directives/hugo-migration.md` and
   `docs/background/archetypes/README.md`.
6. Update each rule README that gained a
   `placeholders:` setting to link to the
   placeholder-grammar archetype page.

## Acceptance Criteria

- [ ] `mdsmith check .` passes with the four
      `proto.md` entries removed from `.mdsmith.yml`
      `ignore:` and the placeholder-link
      `cross-file-reference-integrity.exclude:`
      entry removed.
- [ ] Each `proto.md` file is linted under its
      project kind, with placeholder-aware rules
      passing on its placeholder-rich body.
- [ ] Adding a new schema file requires no
      `ignore:` change — assigning it to an existing
      kind via `kind-assignment:` is enough.
- [ ] `docs/guides/file-kinds.md` exists, describes
      declaration / assignment / merge / conflict
      resolution, and references `mdsmith config
      show` as the troubleshooting path.
- [ ] Hugo terminology note is present in
      `hugo-migration.md` and `archetypes/README.md`.
- [ ] Each rule that gained a `placeholders:`
      setting in plan 93 has a README link to the
      placeholder-grammar archetype page.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
