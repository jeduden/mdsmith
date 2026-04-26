---
id: 92
title: File roles and placeholder grammar
status: "🔲"
summary: >-
  Replace per-file ignore entries for schema and template
  Markdown with a composable model: user-declarable file
  roles that reuse the existing rule-config syntax, plus
  a placeholder grammar exposed as per-rule settings.
---
# File roles and placeholder grammar

## Goal

Remove the need to put schema, template, fragment, and
other "input-to-other-Markdown" files in `ignore:`.
Every Markdown file should be lintable under a declared
**role**. Roles are configured with the same syntax as
the existing rule configuration. No new bespoke schema.

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

1. **User-declarable roles.** A role is a named block
   with the same shape as an `overrides:` entry minus
   `files:`. The five names mdsmith ships with
   (`document`, `schema`, `template`, `fragment`,
   `fixture`) are defaults — projects can declare
   their own (e.g. `agent-prompt`, `runbook`) without
   touching rule code.
2. **Placeholder support as rule settings.** Each rule
   that wants to recognize template tokens (`# ?`,
   `## ...`, `{var}`, CUE front-matter values) exposes
   a `placeholders:` setting through the existing
   `Configurable` interface. Roles enable it the same
   way any other rule setting is enabled.

## Design

### Role declaration

A role is a named block under `roles:`. Its body has
the **same shape as an override entry** (`rules:`,
`front-matter:`, etc.) — minus `files:`, since files
are bound to roles separately.

```yaml
roles:
  schema:
    front-matter: false
    rules:
      first-line-heading:
        placeholders: [var-token, heading-question]
      cross-file-reference-integrity:
        placeholders: [var-token]
      paragraph-readability: false
      paragraph-structure: false
      no-emphasis-as-heading: false
      required-structure:
        mode: schema
```

Same merge semantics as overrides apply: later wins,
settings deep-merge. Five roles ship with mdsmith as
defaults; users can add or override them.

### Role assignment

A file's role set is the union of:

- explicit `role:` field in front matter;
- glob-based assignment in `.mdsmith.yml`
  (`role-assignment:` map, separate from role
  declaration);
- implicit roles inferred from config references
  (any path used as a `required-structure: schema:`
  target gains role `schema`; any path referenced by
  `<?include?>` gains `fragment`).

Implicit assignment is what removes the four current
ignore entries without any user action.

### Composability

1. **Role merge follows override merge.** Roles apply
   in declared order; the file's own glob overrides
   apply last. Resolution is deterministic and
   inspectable.
2. **Lint-once.** Content pulled in via `<?include?>`
   or `<?catalog?>` is linted in its source file
   under its source role; the host file does not
   re-diagnose embedded bytes.
3. **Schema vs subject separation.** A schema file's
   front matter is validated as a *schema* (CUE
   patterns OK); referenced subject files are
   validated as *data*.
4. **No role names in rule code.** Rules consult
   their own settings; they never branch on
   `if role == "schema"`. New roles cannot regress
   existing rule behavior because rules have no
   knowledge of role names.

### Placeholder support

Placeholder grammars are named tokens (`var-token`
for `{identifier}`, `heading-question` for `# ?`,
`cue-frontmatter`, …). Each rule that opts in declares
a `placeholders:` setting via `Configurable` — same
machinery as any other rule setting. Roles enable the
right tokens per rule:

```yaml
roles:
  schema:
    rules:
      cross-file-reference-integrity:
        placeholders: [var-token]
```

Adding a new placeholder grammar is a code change in
one helper plus opt-ins per rule; it is not coupled
to any role.

### Migration

Migration removes ignore entries one role at a time
and keeps the linter green at every step. Behavior on
files not yet tagged with a role is unchanged.

## Examples

### Explicit role in front matter

```markdown
---
role: schema
id: '=~"^MDS[0-9]{3}$"'
status: '"ready" | "not-ready"'
---
# {id}: {name}
```

### Glob-based assignment

Role assignment lives next to role declarations:

```yaml
role-assignment:
  schema:
    - "**/proto.md"
  template:
    - ".github/PULL_REQUEST_TEMPLATE.md"
  fragment:
    - "docs/_partials/**"
```

### Implicit role from a config reference

```yaml
overrides:
  - files: ["plan/*.md"]
    rules:
      required-structure:
        schema: plan/proto.md   # plan/proto.md → schema
```

The path `plan/proto.md` gains role `schema`
implicitly. No `ignore:`, no front-matter tag, no
glob needed.

### User-declared role (no rule code changes)

```yaml
roles:
  agent-prompt:
    rules:
      paragraph-readability: false
      first-line-heading:
        placeholders: [var-token]

role-assignment:
  agent-prompt:
    - ".claude/prompts/**"
```

### Composability — merged role config

`docs/_partials/setup-snippet.md` carries both
`fragment` and `template`. The effective config is a
deep-merge of both role bodies in declared order.
File-glob overrides apply last. Inspect the result
with `mdsmith config show <path>`.

### Lint-once for embeds

```text
docs/index.md           role: document
└── <?include file: _partials/intro.md ?>

docs/_partials/intro.md role: fragment
```

`docs/_partials/intro.md` is linted under `fragment`.
The embedded bytes inside `docs/index.md` are not
re-checked.

### Schema vs subject

| File                                            | Role     | Front matter `id` validated as           |
|-------------------------------------------------|----------|------------------------------------------|
| `plan/proto.md`                                 | schema   | CUE pattern (`int & >=1`)                |
| `plan/92_file-roles-and-placeholder-grammar.md` | document | data; matched against `proto.md` pattern |

## Tasks

1. Inventory current `ignore:` and per-file
   `overrides:` entries; classify each by intended
   role.
2. Add the `roles:` and `role-assignment:` config
   keys; reuse the existing override merge to apply a
   role's body.
3. Add the front-matter `role:` field and the
   implicit-from-reference assignment path
   (`required-structure: schema:` and `<?include?>`).
4. Add a placeholder helper (`var-token`,
   `heading-question`, `cue-frontmatter`, …) and a
   `placeholders:` setting on each rule that opts in
   (`first-line-heading`, `heading-increment`,
   `no-emphasis-as-heading`,
   `cross-file-reference-integrity`,
   `paragraph-readability`, `paragraph-structure`,
   front-matter validation).
5. Ship the five built-in roles (`document`,
   `schema`, `template`, `fragment`, `fixture`) as
   default config that users can override.
6. Implement lint-once for `<?include?>` and
   `<?catalog?>` host files.
7. Drop the four `proto.md` entries from `ignore:`
   and confirm `mdsmith check .` stays green.
8. Document the model under
   `docs/background/archetypes/` and link from each
   affected rule README.
9. Add a `mdsmith config show <file>` debug command
   that prints the resolved role set and merged rule
   config for a file.

## Acceptance Criteria

- [ ] `mdsmith check .` passes with the four
      `proto.md` entries removed from `.mdsmith.yml`
      `ignore:`.
- [ ] A role declaration parses with the same syntax
      as an override entry (minus `files:`); override
      merge is reused for role merge (verified by
      test).
- [ ] A user-declared role (not in the built-in five)
      composes correctly with built-ins and with
      file-glob overrides (covered by test).
- [ ] Schema files are linted under role `schema`:
      CUE-pattern front matter, `# ?` and `## ...`
      placeholders, and `{var}` tokens produce no
      diagnostics from rules whose `placeholders:`
      setting permits them.
- [ ] Content embedded into a host file via
      `<?include?>` or `<?catalog?>` produces
      diagnostics only against the source file, not
      the host (covered by test).
- [ ] Adding a new schema file under any directory
      requires no change to `ignore:` — the role is
      assigned implicitly from its
      `required-structure: schema:` reference.
- [ ] Rule code contains no `if role == "..."`
      branches; rule behavior is selected through
      `Configurable` settings only (enforced by grep
      test).
- [ ] `mdsmith config show <file>` prints the
      resolved role set and the merged rule config.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
