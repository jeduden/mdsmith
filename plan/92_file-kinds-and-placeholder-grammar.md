---
id: 92
title: File kinds and placeholder grammar
status: "🔲"
summary: >-
  Replace per-file ignore entries for schema and template
  Markdown with a composable model: user-declarable file
  kinds that reuse the existing rule-config syntax, plus
  a placeholder grammar exposed as per-rule settings.
---
# File kinds and placeholder grammar

## Goal

Remove the need to put schema, template, fragment, and
other "input-to-other-Markdown" files in `ignore:`.
Every Markdown file should be lintable under a declared
**kind**. Kinds are configured with the same syntax as
the existing rule configuration. No new bespoke schema.

The term `kind` aligns with Hugo's `type` (a content
category declared in front matter); it is *not* a
Hugo *archetype*. See the Hugo terminology task below.

## Background

Today `.mdsmith.yml` ignores four `proto.md` files
(`.claude/skills/`, `plan/`, `internal/rules/`,
`docs/security/`). Their front matter holds CUE schema
patterns and their bodies hold placeholder text like
`# ?`, `## ...`, and `{var}`.

The `required-structure` rule knows these files are
schemas, but other rules do not, so the only escape
hatch is `ignore:`. The same pressure recurs for any
file that seeds or validates other Markdown — templates,
includes, demo sources, future schemas.

The fix is two pieces, both built on existing plumbing:

1. **User-declared kinds.** A kind is a named block
   with the same shape as an `overrides:` entry minus
   `files:`. mdsmith ships **no built-in kinds**;
   each project declares its own vocabulary. Examples
   below use fictional names (`recipe`, `tip`,
   `worksheet`) — the names belong to the project,
   not to mdsmith. Implicit assignment is itself
   user-configured (see `implicit-kinds:`).
2. **Placeholder support as rule settings.** Each rule
   that wants to recognize template tokens (`# ?`,
   `## ...`, `{var}`, CUE front-matter values) exposes
   a `placeholders:` setting through the existing
   `Configurable` interface. Kinds enable it the same
   way any other rule setting is enabled.

## Design

### Kind declaration

A kind is a named block under `kinds:`. Its body has
the **same shape as an override entry** (`rules:`,
`front-matter:`, etc.) — minus `files:`, since files
are bound to kinds separately. The kind name (`recipe`
below) is whatever the project picks:

```yaml
kinds:
  recipe:                      # project-chosen name
    front-matter: false
    rules:
      first-line-heading:
        placeholders: [var-token, heading-question]
      cross-file-reference-integrity:
        placeholders: [var-token]
      paragraph-readability: false
      paragraph-structure: false
      no-emphasis-as-heading: false
```

Kinds merge with the same rules as overrides — later
wins, settings deep-merge.

### Kind assignment

A file's effective kind list is built from three
sources, concatenated in this order:

1. front-matter `kinds:` field (a YAML list; a
   single-kind file still uses a one-item list);
2. matching entries in `kind-assignment:` (config
   order; each entry's kinds in the order listed);
3. matching entries in `implicit-kinds:` (config
   order).

Duplicate names are dropped after their first
occurrence. Referencing an undeclared kind is a
config error.

`implicit-kinds:` maps **reference sources** to a
kind name. A reference source is any place where one
Markdown file references another. The linter's core
has no hardcoded kind names; the project picks them:

```yaml
implicit-kinds:
  - source: required-structure.schema  # mdsmith field
    kind: recipe                       # project name
  - source: include
    kind: tip
  - source: catalog
    kind: tip
```

With this in place, declaring a `recipe` kind plus
`required-structure.schema: plan/proto.md` is enough
to drop `plan/proto.md` from `ignore:`.

### Composability

1. **Kind merge follows override merge.** Kinds
   apply in *effective-list* order: front-matter,
   then `kind-assignment:`, then `implicit-kinds:`,
   each in config order; duplicates dropped after
   first occurrence. The file's own glob overrides
   apply last. The result is deterministic across
   runs because order is driven by lists, not maps.
2. **Lint-once.** Content pulled in via `<?include?>`
   or `<?catalog?>` is linted in its source file
   under its source kind; the host file does not
   re-diagnose embedded bytes.
3. **Schema vs subject separation.** A schema file's
   front matter is validated as a *schema* (CUE
   patterns OK); referenced subject files are
   validated as *data*.
4. **No kind names in rule code.** Rules read their
   own settings; they never branch on a kind name.
   New kinds cannot regress existing behavior.

### Placeholder support

Placeholder grammars are named tokens (`var-token`
for `{identifier}`, `heading-question` for `# ?`,
`cue-frontmatter`, …). Each opt-in rule declares a
`placeholders:` setting via `Configurable` — same
machinery as any other rule setting (the kind
declaration above shows it on
`cross-file-reference-integrity`). Adding a new
grammar is a code change in one helper plus opt-ins
per rule; it is not coupled to any kind. Migration
removes `ignore:` entries one kind at a time, keeping
the linter green at each step; untagged files behave
as today.

## Examples

> Example kind names below (`recipe`, `tip`,
> `worksheet`) are deliberately fictional — they
> aren't mdsmith built-ins. Real projects pick
> names that fit their domain.

### Explicit kinds in front matter

```markdown
---
kinds: [recipe]
id: '=~"^MDS[0-9]{3}$"'
status: '"ready" | "not-ready"'
---
# {id}: {name}
```

A file with multiple kinds uses a multi-element list:
`kinds: [tip, worksheet]`. Merge order matches list
order.

### Glob-based assignment

```yaml
kind-assignment:
  recipe:
    - "**/proto.md"
  worksheet:
    - ".github/PULL_REQUEST_TEMPLATE.md"
  tip:
    - "docs/_partials/**"
```

### Implicit kind from a config reference

```yaml
overrides:
  - files: ["plan/*.md"]
    rules:
      required-structure:
        schema: plan/proto.md   # → kind 'recipe'
```

Combined with the `implicit-kinds:` block above,
`plan/proto.md` gains kind `recipe` automatically.
No `ignore:`, no front-matter tag, no glob needed.

### Composability and lint-once

`docs/_partials/intro.md` (kind `tip`) is included
by `docs/index.md` via `<?include?>`. The fragment
is linted once, under `tip`; the host's embedded
bytes are not re-checked. A file with multiple kinds
(e.g. `[tip, worksheet]`) gets a deep-merge of both
kind bodies in list order; file-glob overrides apply
last. Inspect with `mdsmith config show <path>`.

## Tasks

1. Inventory current `ignore:` and per-file
   `overrides:` entries; classify each by intended
   kind.
2. Add the `kinds:`, `kind-assignment:`, and
   `implicit-kinds:` config keys; reuse the existing
   override merge to apply a kind's body. The
   linter's core must reference no kind names.
3. Add the front-matter `kinds:` list field and wire
   the implicit-from-reference assignment driven by
   `implicit-kinds:` (supported sources:
   `required-structure.schema`, `include`,
   `catalog`).
4. Add a placeholder helper (`var-token`,
   `heading-question`, `cue-frontmatter`, …) and a
   `placeholders:` setting on each rule that opts in
   (`first-line-heading`, `heading-increment`,
   `no-emphasis-as-heading`,
   `cross-file-reference-integrity`,
   `paragraph-readability`, `paragraph-structure`,
   front-matter validation).
5. Implement lint-once for `<?include?>` and
   `<?catalog?>` host files.
6. Update this repo's `.mdsmith.yml` to declare the
   kinds it needs and drop the four `proto.md`
   entries from `ignore:`; confirm `mdsmith check .`
   stays green.
7. New guide at `docs/guides/file-kinds.md`
   describing kind declaration, assignment,
   `implicit-kinds:`, and merge order. Link from
   each affected rule README.
8. New archetype page at
   `docs/background/archetypes/placeholder-grammar/`
   describing the token vocabulary and the
   `placeholders:` rule-setting contract. Link from
   each opt-in rule README.
9. Hugo terminology note in
   `docs/guides/directives/hugo-migration.md` and
   `docs/background/archetypes/README.md`: mdsmith
   *kind* ≈ Hugo *type*; mdsmith *archetype* is a
   rule-mechanics pattern, not Hugo's scaffold.
10. Add CLI commands `mdsmith config kinds` (list
    declared kinds with their merged bodies) and
    `mdsmith config show <file>` (print resolved
    kind list and merged rule config for a file).
11. Emit a clear config error when kind assignment
    references an undeclared kind name.

## Acceptance Criteria

- [ ] `mdsmith check .` passes with the four
      `proto.md` entries removed from `.mdsmith.yml`
      `ignore:`.
- [ ] A kind declaration parses with the same syntax
      as an override entry (minus `files:`); override
      merge is reused for kind merge (verified by
      test).
- [ ] Two project-declared kinds compose correctly
      with each other and with file-glob overrides
      (covered by test).
- [ ] A file declaring multiple kinds via
      `kinds: [a, b]` in front matter merges them in
      list order (covered by test).
- [ ] Implicit kind assignment is configured by
      `implicit-kinds:`; no kind name is referenced
      by mdsmith's core or shipped default config
      (enforced by grep test).
- [ ] Schema files declared by the project are
      linted under their chosen kind: CUE-pattern
      front matter, `# ?` and `## ...` placeholders,
      and `{var}` tokens produce no diagnostics from
      rules whose `placeholders:` setting permits
      them.
- [ ] Content embedded into a host file via
      `<?include?>` or `<?catalog?>` produces
      diagnostics only against the source file, not
      the host (covered by test).
- [ ] Adding a new schema file under any directory
      requires no change to `ignore:` — the kind is
      assigned implicitly from its
      `required-structure.schema` reference.
- [ ] Rule code contains no `if kind == "..."`
      branches; rule behavior is selected through
      `Configurable` settings only (enforced by grep
      test).
- [ ] Referencing an undeclared kind name produces a
      clear config error.
- [ ] `mdsmith config kinds` lists declared kinds
      with their merged bodies.
- [ ] `mdsmith config show <file>` prints the
      resolved kind list and the merged rule config.
- [ ] Hugo terminology note is present in
      `hugo-migration.md` and `archetypes/README.md`.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
