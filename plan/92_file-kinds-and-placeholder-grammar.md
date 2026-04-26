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
`kind` aligns with Hugo's `type` (front-matter content
category), not Hugo's `archetype` (see Hugo task).

## Background

Today `.mdsmith.yml` ignores four `proto.md` files:
`.claude/skills/proto.md`, `plan/proto.md`,
`internal/rules/proto.md`, `docs/security/proto.md`.

Their front matter holds CUE schema patterns. Their
bodies hold placeholder text like `# ?`, `## ...`,
and `{var}`.

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
   not to mdsmith.
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
are bound to kinds separately. The kind name (`plan`
below) is whatever the project picks. A kind owns its
schema by setting `rules.required-structure.schema:`
in its body, no special top-level field needed:

```yaml
kinds:
  plan:                          # project-chosen name
    rules:
      required-structure:
        schema: plan/proto.md    # kind 'plan' owns
                                 # this schema
      first-line-heading:
        placeholders: [var-token, heading-question]
      paragraph-readability: false
      no-emphasis-as-heading: false
  proto:                         # for the proto.md
    rules:                       # files themselves
      cross-file-reference-integrity:
        placeholders: [var-token]
      paragraph-readability: false
      front-matter: false
```

Kinds merge with the same rules as overrides — later
wins, settings deep-merge.

### Kind assignment

A file's effective kind list is built from two sources,
concatenated in this order:

1. front-matter `kinds:` field (a YAML list; a
   single-kind file still uses a one-item list);
2. matching entries in `kind-assignment:` (config
   order; each entry's kinds in the order listed).

Duplicate names are dropped after their first
occurrence. Referencing an undeclared kind is a config
error.

### Composability

1. **Kind merge follows override merge.** Kinds apply
   in *effective-list* order: front-matter first,
   then `kind-assignment:` matches in config order;
   duplicates dropped after first occurrence. The
   file's own glob overrides apply last. Order is
   list-driven, so it is stable across runs.
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
`cue-frontmatter`, …). Each opt-in rule exposes a
`placeholders:` setting via `Configurable` (see the
kind declaration above). Adding a grammar is one
helper plus per-rule opt-ins; it is not coupled to
any kind. Migration removes `ignore:` entries one
kind at a time; untagged files behave as today.

## Examples

> Kind names below (`plan`, `proto`, `tip`,
> `worksheet`) are fictional — projects pick their own.

### Explicit kinds in front matter

```markdown
---
kinds: [plan]
id: 92
status: 🔲
---
# File kinds and placeholder grammar
```

A file with multiple kinds uses a multi-element list:
`kinds: [tip, worksheet]`. Merge order matches list
order.

### Glob-based assignment

`kind-assignment:` is a **list** of entries (same
shape as `overrides:`); negation globs (`!path`)
exclude paths a broader glob would catch.

```yaml
kind-assignment:
  - files: ["plan/*.md", "!plan/proto.md"]
    kinds: [plan]
  - files: ["**/proto.md"]
    kinds: [proto]
  - files: [".github/PULL_REQUEST_TEMPLATE.md"]
    kinds: [worksheet]
  - files:
      - "docs/_partials/**"
      - "!docs/_partials/legacy/**"  # excluded
    kinds: [tip]
```

### Schema linkage

A kind that defines a `rules.required-structure
.schema:` value applies that CUE schema to every
file of that kind. The proto.md file itself is
typically a separate kind (e.g. `proto` above) so
its placeholder-rich body lints cleanly.

## Tasks

1. Inventory current `ignore:` and per-file
   `overrides:` entries; classify each by intended
   kind.
2. Add the `kinds:` and `kind-assignment:` config
   keys; reuse the existing override merge to apply a
   kind's body. The linter's core must reference no
   kind names.
3. Add the front-matter `kinds:` list field; resolve
   each file's effective kind list per Kind
   assignment above.
4. Add a placeholder helper (`var-token`,
   `heading-question`, `cue-frontmatter`, …) and a
   `placeholders:` setting on opt-in rules
   (`first-line-heading`, `heading-increment`,
   `no-emphasis-as-heading`,
   `cross-file-reference-integrity`,
   `paragraph-readability`, `paragraph-structure`,
   `required-structure` — which performs front-
   matter schema checks). Wire the same vocabulary
   into `catalog` front-matter interpolation and
   engine front-matter parsing under the
   `front-matter:` config key.
5. Implement lint-once for `<?include?>` and
   `<?catalog?>` host files.
6. Update this repo's `.mdsmith.yml` to declare the
   kinds it needs and drop the four `proto.md`
   entries from `ignore:`; confirm `mdsmith check .`
   stays green.
7. New guide at `docs/guides/file-kinds.md`
   describing kind declaration, assignment, and
   merge order. Link from each affected rule README.
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
- [ ] No kind name is referenced by mdsmith's core
      or shipped default config (enforced by grep
      test).
- [ ] Files of a kind that sets
      `rules.required-structure.schema:` are
      validated against that schema; the schema file
      itself lints cleanly under its own kind with
      placeholder-aware rules.
- [ ] Content embedded into a host file via
      `<?include?>` or `<?catalog?>` produces
      diagnostics only against the source file, not
      the host (covered by test).
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
