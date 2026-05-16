---
id: 161
title: Expose rule maintainability patterns via CLI help and LSP
status: "🔳"
model: sonnet
depends-on: []
summary: >-
  Each rule README under `internal/rules/` declares
  a maintainability pattern — the structural shape
  where adopting the rule (or its directive/config)
  prevents drift. Schema-validate the block,
  surface it through `mdsmith help rule <name>` and
  a new `mdsmith help patterns` topic, and serve
  the same metadata through the LSP so the
  mdsmith-reviewer agent (plan 160) and any editor
  client can query patterns without hard-coding
  them in skill bodies.
---
# Expose rule maintainability patterns via CLI help and LSP

## Goal

Move pattern knowledge out of skill bodies and
into the rules. Each rule README declares the
structural shape that adopting the rule would
keep clean. The CLI and LSP expose that
documentation so agents and editors can query
it.

## Background

Plan 160 introduces the `mdsmith-reviewer`
agent. The agent walks changed files and
proposes which rule, directive, or kind config
to **adopt** so a pattern stops drifting.
Rules like `catalog` and `include` only
validate declared directives; they don't
detect a hand-maintained index. The reviewer
surfaces the opportunity before adoption.
Patterns live where the agent queries them at
runtime, not in the skill body.

The natural home is the existing rule README at
[`internal/rules/<id>-<name>/README.md`](../internal/rules/).
The `markdown-audit` skill's
[`patterns.md`](../.claude/skills/markdown-audit/patterns.md)
captures seven checks today.

Five rules carry maintainability blocks. The
duplicated-content audit check binds to two
rules. Detection and the recommended fix live
in different places:

- catalog — hand-maintained indexes; adopt
  `<?catalog?>`.
- duplicated-content (MDS037) — repeated
  paragraphs; extract via `<?include?>` or
  refactor.
- include (MDS021) — near-duplicate sections
  worth deduping; adopt `<?include?>`.
- required-structure — kind without a schema;
  declare it inline at `kinds.<name>.schema`
  or via proto file at
  `kinds.<name>.rules.required-structure.schema`.
- directory-structure — file-placement
  violations; move the file to an allowed
  directory, or extend
  `directory-structure.allowed` if the new
  location is correct.

MDS037 and MDS033 fire on the pattern
directly. Their `fix` matches the diagnostic
remedy. The other three (catalog, include,
required-structure) only validate declared
structures. Those blocks frame adoption
opportunities the reviewer surfaces before
the rule fires. Three audit checks are
config-level and stay in `patterns.md`: no
`.mdsmith.yml`, similar files without a kind,
kind without `path-pattern`. (Non-goal: no
new rules.)

## Non-Goals

- Adding new rules. This plan only documents
  and exposes existing rules' patterns.
- Auto-fixing the patterns. Fixes stay with
  existing `mdsmith fix` paths and the
  user-invoked [`/mdsmith-fix`](160_claude-code-skills-agents-hooks.md).
- Generalising the metadata beyond
  maintainability patterns; rule docs may grow
  other structured sections later, but the
  first cut targets the reviewer's needs.

## Design

### Rule README schema

Make `signal` and `fix` structured. The
contract cannot drift from prose. Add a
`maintainability` block to the rule README's
front matter. The [rule-readme proto][proto]
validates it:

[proto]: ../internal/rules/proto.md
[dproto]: ../internal/rules/directive-proto.md

```yaml
---
maintainability:
  signal: "a list of links to sibling files in the same directory"
  fix: "adopt a `<?catalog?>` directive so the list stays in sync"
  for-diagnostic: false
---
```

Schema goes in [proto.md][proto] and
[directive-proto.md][dproto] as a quoted CUE
expression:

```yaml
maintainability: '{signal: string & != "", fix: string & != "", "for-diagnostic"?: bool | *false} | null'
```

The field is required (no `?` suffix on
`maintainability` itself). Both `signal` and
`fix` are present and non-empty, or the whole
block is the literal `null` for rules with no
maintainability pattern. `mdsmith check`
rejects absence and partial blocks at lint
time.

`for-diagnostic: true` opts the entry into
hover enrichment. The default is `false` —
catalog, include, and required-structure fire
only after adoption. An "adopt X" sketch on
their diagnostics misleads. The two rules
that fire on the pattern directly set
`for-diagnostic: true`. Those are
duplicated-content and directory-structure.

Rules with no pattern set
`maintainability: null` and are omitted from
CLI/LSP payloads below. README bodies stay
free-form; tooling reads front matter only.

### CLI exposure

`mdsmith help rule <name>` strips front matter
today
([`internal/rules/ruledocs.go`](../internal/rules/ruledocs.go)).
Extend the renderer to append a
"Maintainability pattern" section built from
the `maintainability` block. The README body
stays free-form prose.

For bulk-load by agents, add a new
`mdsmith help patterns` topic (sibling of
`rule`, `metrics`, `kinds`). It emits every
rule's pattern in one shot.

Default output is text. `-f json` produces
records of `{id, name, signal, fix,
for-diagnostic}`. `id` is the stable
diagnostic code (e.g. `MDS001`; matches
`Diagnostic.code`). `name`
is the human-readable rule name (e.g.
`line-length`).

### LSP exposure

The LSP gets one extension request and one
existing-method enrichment:

- `mdsmith/rulePatterns` — server returns the
  same payload as `mdsmith help patterns -f
  json`. Agent clients can call this once per
  session.
- `textDocument/hover` on a diagnostic from a
  rule whose `maintainability.fix` describes a
  diagnostic-level remediation appends that
  fix. Adoption-style fixes (e.g. catalog,
  include — recommending a directive *before*
  it exists) are skipped here so an
  already-firing diagnostic does not get a
  "adopt `<?include?>`" suggestion. The
  implementer adds a `for-diagnostic: bool`
  flag (or equivalent) on the `maintainability`
  block to mark which entries hover may
  surface.

### Reviewer agent integration

The mdsmith-reviewer agent (plan 160) calls one
of:

- `mdsmith help patterns -f json` when running
  outside an editor.
- `mdsmith/rulePatterns` when an LSP server is
  attached.

Either way, the agent stays free of
hard-coded patterns and picks up new rules
automatically.

## Tasks

1. Add the `maintainability` front-matter block
   (with the CUE constraint above) to both
   [`internal/rules/proto.md`][proto] (used by
   the `rule-readme` kind) and
   [`internal/rules/directive-proto.md`][dproto]
   (used by the `directive-rule-readme` kind
   for MDS019/021/038/039). Both kinds
   reference their respective proto, so the
   constraint propagates automatically once
   added.
2. Populate `maintainability` on every existing
   rule README. Reuse content from the audit
   skill's [patterns.md][audit-patterns] where
   it overlaps. Rules with no maintainability
   pattern set `maintainability: null`.
3. Extend `mdsmith help rule <name>` to render
   the `maintainability` block as a
   "Maintainability pattern" section appended
   to the existing body output. Add the new
   `patterns` help topic to
   [`cmd/mdsmith`](../cmd/mdsmith) emitting
   `{id, name, signal, fix, for-diagnostic}`
   records (omitting `maintainability: null`
   rules); honour `-f text|json`.
   Write failing tests first per CLAUDE.md.
4. Add the `mdsmith/rulePatterns` LSP method
   and the `textDocument/hover` enrichment.
   Failing tests first.
5. Document both surfaces in
   [`docs/reference/cli/help.md`](../docs/reference/cli/help.md)
   and
   [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md).
6. Trim [`patterns.md`][audit-patterns] to
   just the three config-level checks (no
   `.mdsmith.yml`, similar files without a
   kind, kind without `path-pattern`). The
   five rule-backed checks now live in their
   rule READMEs. Update the audit skill body
   to load rule-backed patterns from
   `mdsmith help patterns` and keep reading
   the trimmed `patterns.md` for the non-rule
   checks. The marketplace plugin at
   `editors/claude-code-audit/` ships only the
   SKILL.md (no sibling `patterns.md`); either
   inline the trimmed content into the plugin
   SKILL.md via `<?include?>` so installed
   users still get the non-rule heuristics, or
   add a sibling `patterns.md` under
   `editors/claude-code-audit/skills/markdown-audit/`.

[audit-patterns]: ../.claude/skills/markdown-audit/patterns.md

## Acceptance Criteria

- [x] Every rule README declares a
      `maintainability` block (either
      `{signal, fix}` or `null`);
      `mdsmith check internal/rules/` passes
      via the `rule-readme` and
      `directive-rule-readme` kinds.
- [x] Absence of the field, or a partial block
      (e.g. `signal` without `fix`), fails
      `mdsmith check` with a schema error.
- [x] For a rule with a non-null
      `maintainability` block,
      `mdsmith help rule <name>` renders it as a
      "Maintainability pattern" section.
- [x] For a rule with `maintainability: null`,
      `mdsmith help rule <name>` does not render
      a "Maintainability pattern" section
      (neither empty nor literal `null`).
- [x] `mdsmith help patterns` (default text)
      lists every rule's pattern in a readable
      form; covered by a new test asserting the
      output includes each rule's `signal` and
      `fix` lines for non-null rules.
- [x] `mdsmith help patterns -f json` emits a
      JSON array of
      `{id, name, signal, fix, for-diagnostic}`
      entries (with `id` matching diagnostic
      codes like `MDS001`), omitting
      rules with `maintainability: null`.
      Covered by a new unit test.
- [x] `mdsmith/rulePatterns` returns the same
      payload over LSP. Covered by a new LSP
      end-to-end test.
- [x] `textDocument/hover` on a diagnostic
      from a `for-diagnostic: true` rule
      (duplicated-content, directory-structure)
      appends the fix sketch. Hover on the
      three adoption-only rules (catalog,
      include, required-structure) and on
      `maintainability: null` rules is
      unchanged. Covered by a new hover test.
- [x] [`docs/reference/cli/help.md`](../docs/reference/cli/help.md)
      and
      [`docs/reference/cli/lsp.md`](../docs/reference/cli/lsp.md)
      document the new topic and LSP method.
- [ ] Both the local `markdown-audit` skill
      and the installed `mdsmith-audit` plugin
      surface all five rule-backed patterns
      (from `mdsmith help patterns`) plus the
      three trimmed config-level checks. Deferred
      to a follow-up; this PR ships the data
      source the skill consumes.
- [x] `go test ./...` passes.
- [x] `mdsmith check .` passes.

## ...

<?allow-empty-section?>
