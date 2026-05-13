---
title: Detection patterns
summary: >-
  Detection heuristics, false positives, and fix
  recipes for each of the seven checks the
  `markdown-audit` skill runs.
---
# Detection patterns

Each section covers one numbered check from the
sibling `SKILL.md` workflow. Per check: the
signal, the heuristic, false positives to skip,
the severity bucket, and the fix recipe.

## Check 1 — No `.mdsmith.yml`

Signal: `.mdsmith.yml` does not exist at the repo
root (`test -f .mdsmith.yml`).

False positives: a repo with one or two top-level
Markdown files and nothing else. Skip with a
one-line note.

Severity: blocker when the repo has five or more
`.md` files. Nice-to-have otherwise.

Fix: run `mdsmith init` to create a default
config. Then walk checks 2 through 7 to populate
kinds, overrides, and assignments.

## Check 2 — Hand-maintained indexes

Signal: a `.md` file lists links to sibling files
that follow a pattern. Examples include an index of
every plan in `plan/`, every rule in
`internal/rules/`, every guide in `docs/guides/`.

Heuristic: walk every `.md` file
(`git ls-files '*.md'`). In each file, scan for a
contiguous block of three or more bullet-list lines
whose only content is a Markdown link
(`grep -n '^[[:space:]]*- \[' <file>`). The block
is a candidate when the linked paths share a
directory prefix, the block is not already inside
`<?catalog ... ?>` markers, and the linked files
exist on disk.

False positives:

- Lists of two items. Too small to count as an
  index.
- Lists mixing repo paths and external links. A
  catalog would lose the external links.
- "See also" lists with hand-picked entries.
  Sometimes the selection is the point.

Severity: tax.

Fix: use a `<?catalog?>` directive on the shared
directory. Read the canonical before/after pair
from the rule's own pattern folders:

- before (hand-maintained list): `internal/rules/MDS019-catalog/pattern/bad/`
- after (catalog directive): `internal/rules/MDS019-catalog/pattern/good/`

These folders are the single source of truth. The
rule's README `## Pattern` section pulls from
them. A directive-rule integration test fails
when they go missing.

Run `mdsmith fix <file>` and confirm the same
items reappear in the regenerated body.

## Check 3 — Similar files, no kind

Signal: a directory holds three or more `.md`
files with a similar front matter shape. The
files share the same required keys. No
`kind-assignment:` entry covers the directory.

Heuristic: list directories with three or more
`.md` files
(`git ls-files '*.md' | xargs -n1 dirname | sort | uniq -c`).
For each candidate, sample every file's front
matter. The directory is a kind candidate when 70%
of files share the same key set, and no glob in
`.mdsmith.yml` already covers it.

False positives:

- A directory of one-off Markdown without shared
  front matter (research spikes, ad-hoc notes).
- A directory already covered by a glob in
  `kind-assignment:`, even when the kind body is
  empty. The assignment alone counts.
- Bad fixtures inside `internal/rules/*/bad/`,
  intentionally malformed and under `ignore:`.

Severity: tax. Drop to nice-to-have when the
shared shape is small.

Fix: add an entry to `kinds:` plus a matching
`kind-assignment:` glob.

```yaml
kinds:
  runbook:
    rules:
      max-file-length:
        max: 400

kind-assignment:
  - glob: ["runbooks/*.md"]
    kinds: [runbook]
```

Start with the settings the files actually share
right now. Do not over-engineer. After applying,
run `mdsmith kinds resolve runbooks/example.md` to
confirm the file picks up the new kind.

## Check 4 — Duplicated content

Signal: two or more files contain near-identical
sections. One common case is a "Development"
block copied between `README.md` and
`docs/development/index.md`. Another is an
"Install" block copied across plugin READMEs.

Heuristic: hash every second-level section
(`##`-bounded) in every `.md` file. A matching
hash across files is a strong duplicate signal. A
weaker signal is an identical first non-blank
paragraph under a same-named heading.

False positives:

- Generated sections. A `<?catalog?>` body may
  look identical across files by design.
- Boilerplate (license footer, security contact)
  too small to warrant an include.

Severity: tax.

Fix: extract the shared body into one canonical
snippet. Replace each copy with `<?include?>`.
Read the canonical before/after pair from the
rule's own pattern folders:

- before: `internal/rules/MDS021-include/pattern/bad/`
  (section duplicated across two files)
- after: `internal/rules/MDS021-include/pattern/good/`
  (one snippet plus include directives)

Use `heading-level: "absolute"` when the host file
needs the included headings to nest under an
existing heading. Run `mdsmith fix` and confirm
both files regenerate the same body.

## Check 5 — Kind without `path-pattern`

Signal: a declared kind whose files share an
obvious naming convention, but the kind body has
no `path-pattern:`.

Heuristic: read `kinds:` and `kind-assignment:`
from `.mdsmith.yml`. For each kind, walk the files
that resolve to it. The kind is a candidate when
80% of those files share a regex-shaped filename.
Examples: `\d+_[a-z-]+\.md`, `RFC-\d{4}\.md`.

False positives: a kind with intentionally
heterogeneous membership — a "doc" kind covering
README, index, and topic pages qualifies.

Severity: nice-to-have.

Fix: add `path-pattern:` to the kind body.

```yaml
kinds:
  plan:
    path-pattern: "plan/[0-9][0-9]*_*.md"
```

Glob syntax has no "exactly digits" class, so the
pattern is an approximation. For tighter
constraints, combine `path-pattern:` with a
`<?require filename:?>` directive on the kind's
schema. Re-run `mdsmith check .` to confirm no
existing file fails the new pattern.

## Check 6 — Kind without a schema

Signal: a kind whose files share a front matter
schema or top-level heading sequence, but the
kind has no `required-structure.schema:`.

Heuristic: walk the files of the kind. The kind is
a candidate when 70% share the same required
front matter keys. It is a strong candidate when
70% also share the same top-level heading
sequence.

False positives: a kind with fewer than three
files. Wait until there is enough shape to lock
in.

Severity: nice-to-have.

Fix: pick the schema flavor that fits the kind,
then wire it in `.mdsmith.yml`.

Inline schema vs `proto.md` — when to use which:

- Inline schema in `.mdsmith.yml` when the shape
  is small (one or two required sections, no
  nested headings) or the kind exists only in
  this repo. One source file holds everything.
- A `proto.md` file when the schema runs more
  than a screen, the required sections nest, the
  schema is shared across kinds via
  `<?include?>`, or the prose around each
  required field is itself useful documentation.
  The proto file is itself a lintable Markdown
  document.

Both flavors satisfy `required-structure`. Do not
mix the two for a single kind — pick one.

The rule's example folder holds canonical samples
of each flavor under
`internal/rules/MDS020-required-structure/good/`.
Read them before authoring:

- `inline-flat.md` — inline schema with flat
  required sections.
- `inline-runbook.md` — inline schema with nested
  sections and heading aliases.
- `default.md` plus `data/tmpl.md` — proto file
  referenced via the `schema:` setting.
- `schema-compose.md` — proto file composed from
  shared fragments via `<?include?>`.

Wire-up for inline schema:

```yaml
kinds:
  plan:
    schema:
      sections:
        - heading: "Goal"
          required: true
        - heading: "Tasks"
          required: true
```

Wire-up for proto.md:

```yaml
kinds:
  plan:
    rules:
      required-structure:
        schema: plan/proto.md
```

After wiring, run `mdsmith check .` and confirm
every existing file passes. When a file fails,
relax the schema — do not break the files.

## Check 7 — File placement violation

Signal: a `.md` file lives in a directory the
repo's file-placement guide rejects.

Heuristic: when the repo has
`docs/development/file-placement.md`, read its
decision list — that is the source of truth.
Otherwise fall back to the mdsmith default
layout: repo root well-known files, `plan/`,
`docs/`, `internal/`, `.claude/skills/`,
`.github/`, and `editors/`.

False positives:

- A monorepo subpackage with its own `README.md`
  or `CHANGELOG.md`.
- A file the user listed under `ignore:` on
  purpose.

Severity: tax when the misplaced file hurts
navigation. Nice-to-have otherwise.

Fix: move the file to the directory the guide
points to. Cross-references update automatically
when `mdsmith fix .` runs — the
`cross-file-reference-integrity` rule rewrites
links to the moved file. After moving, run
`mdsmith kinds resolve <new-path>` to confirm the
file picks up the right kind at its new home.
