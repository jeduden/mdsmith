---
title: Progressive Disclosure for AI Agents
weight: 80
summary: >-
  Use `<?catalog?>` with a per-file `summary` front
  matter field to emit a one-line index of a directory,
  so AI coding agents read a few thousand tokens of
  metadata up front and only `Read` the files a task
  actually touches.
---
# Progressive Disclosure for AI Agents

AI coding agents (Claude Code, Codex, Copilot, Cursor)
pay context tokens for every file they pull into the
prompt. Loading every doc in a repository eagerly is
the easy default and the wrong one: 60 docs at 2 000
tokens each is 120 000 tokens spent before the agent
has done anything.

Progressive disclosure inverts that. The agent file
(`CLAUDE.md`, `AGENTS.md`, `.github/copilot-instructions.md`)
carries a short index — one line per doc, naming the
specific data each doc contains — and the agent
`Read`s the full file only when a task makes one
relevant.

mdsmith builds that index for you. This guide is the
recipe.

## Motivation

A three-tier loading model is the broader pattern:

1. **Metadata.** Filenames + one-line `summary`
   strings. Cheap; always in the prompt.
2. **Instructions.** A skill or guide body the agent
   pulls in when the metadata says it applies.
3. **Reference.** Full source files the agent reads
   on demand once it knows what it is looking for.

Tier 1 is the bottleneck. A vague index ("docs page
about catalog") forces the agent to load tier 2 to
decide whether tier 2 was needed at all — the
disclosure stops being progressive. A precise index
("How `<?catalog?>` projects a `summary` field into a
generated bullet list, with the `where` and `sort`
parameters") lets the agent skip the file without
opening it.

mdsmith's `<?catalog?>` directive automates tier 1.
Each tracked file declares a `summary` in YAML front
matter; the directive generates the index from those
summaries; `mdsmith check` flags drift; `mdsmith fix`
re-renders.

## How mdsmith does it

[`CLAUDE.md`](../../CLAUDE.md) is the live example.
A single `<?catalog?>` under its `## Docs` heading
matches every tracked doc under `docs/**` — four `!`
patterns exclude research notes, security reports,
brand copy, and `proto.md` templates — sorts by
path, and emits one bullet per match:

```markdown
<?catalog
glob:
  - "docs/**/*.md"
  - "!docs/research/**"
  - "!docs/security/**"
  - "!docs/brand/**"
  - "!**/proto.md"
sort: path
header: ""
row: "- [{summary}]({filename})"
?>
- [How generated sections work — markers, directives, and fix behavior.](docs/background/concepts/generated-section.md)
- [Build commands, project layout, code style, test fixtures, coverage gate, and merge conflicts.](docs/development/index.md)
...
<?/catalog?>
```

The 113-doc tree compresses to ~3 400 tokens of index.
The agent reads `CLAUDE.md` once at session start and
already knows which file to open for any task it gets.

`AGENTS.md` is a thin shim that `<?include?>`s
`CLAUDE.md`, so Codex and Copilot consume the same
index from the same source.

## Step-by-step

### 1. Add a `summary` to every file in the indexed tree

Front matter on each doc:

```yaml
---
title: Coverage Gate
summary: >-
  Codecov coverage gate and CI status checks.
---
```

The `summary` is what shows up in the agent's index.
Write it like a tier-1 cue: state the specific data
the file contains, in one sentence the agent can match
against a task description. Vague verbs ("describes",
"handles", "covers") force a tier-2 load to confirm.

### 2. Place a `<?catalog?>` in the agent file

In `CLAUDE.md` (or `AGENTS.md`, or any orient-the-
agent doc):

```markdown
## Docs

<?catalog
glob: "docs/**/*.md"
sort: path
row: "- [{summary}]({filename})"
?>
<?/catalog?>
```

`row` is the line shape; `{summary}` and `{filename}`
are pulled from the matched file's front matter.
Without `header` or `footer`, the body is just the
rows.

### 3. Run `mdsmith fix` to populate the body

```bash
mdsmith fix CLAUDE.md
```

The empty body between the markers fills in. From
this point, the index is read-only: edit the
underlying file's `summary` (not the body line) and
re-run `fix`.

### 4. Lock in freshness with `mdsmith check`

```bash
mdsmith check .
```

If anyone edits a file's `summary` and forgets to
re-render, `check` fails with an MDS019 diagnostic
naming the catalog whose body drifted. Run this in
CI; the index cannot rot.

### 5. (Optional) Filter and partition

A single 60-line index is fine for a small repo. For
larger trees, partition by category so each section
is short enough for the agent to skim:

```markdown
### Background

<?catalog
glob: "docs/background/**/*.md"
sort: path
row: "- [{summary}]({filename})"
?>
<?/catalog?>

### Reference

<?catalog
glob: "docs/reference/**/*.md"
sort: path
row: "- [{summary}]({filename})"
?>
<?/catalog?>
```

Or filter by front matter with `where`:

```markdown
<?catalog
glob: "plan/*.md"
where: 'status: "🔳"'
sort: numeric:id
row: "- [{id}: {summary}]({filename})"
?>
<?/catalog?>
```

`where` is a CUE expression evaluated against the
matched file's parsed front matter. The same grammar
[`mdsmith list query`](../reference/cli/query.md) uses
drops in unchanged. See
[Generating Content with Directives](directives/generating-content.md)
for the full parameter reference.

## Common issues

### A file is missing `summary`

`{summary}` resolves to an empty string and the row
becomes `- [](path/to/file.md)`. Fix with a schema
that requires the field. Declare a kind for the
indexed tree, point a `required-structure` schema at
it, and list `summary` as required front matter:

```markdown
---
title: nonEmpty
summary: nonEmpty
---
# ?
```

See [Schemas](schemas.md) for the kind +
`proto.md` wiring. Once the schema is in place,
`mdsmith check` blocks any file in the tree that
ships without a usable summary.

### Summaries are too vague to disclose anything

If a summary reads "Docs page about catalogs", an
agent has no way to skip it without loading the file.
The repo writing guideline applies: name the inputs
(front matter fields, glob patterns, heading levels)
and the condition checked against them, not the
mechanism. "How `<?catalog?>` projects a `summary`
field into a bullet list, with `where` and `sort`
parameters" is a tier-1 cue; "Catalog directive
guide" is not.

This is a content rule, not a lint rule.
[MDS024 paragraph-structure](../../internal/rules/MDS024-paragraph-structure/README.md)
and the surrounding readability rules catch the worst
offenders in body prose; summaries are short enough
that review is the gate.

### The index is too large to load eagerly

A 200-line catalog still beats loading 200 files,
but it crowds out room for the agent's actual work.
Two tactics:

- **Partition.** One catalog per top-level directory,
  each under its own heading. The agent reads the
  index, jumps to the section that matches the task.
- **Filter.** Pin the catalog to active work with
  `where: 'status: "🔳"'` (or `kind`, or a tag
  field). Archived material drops out of the index
  without leaving the repo.

The
[`token-budget`](../../internal/rules/MDS028-token-budget/README.md)
rule on `CLAUDE.md` is the canary: if the index
pushes the file over budget, the rule fires before
the agent runs out of context.

### A summary disagrees with the file's content

`mdsmith check` does not validate `summary` against
the body — it only validates the catalog body
against the front matter. Drift here is a content
problem, not a lint problem. Two mitigations:

- Keep `summary` short. A two-sentence summary is
  hard to falsify; a paragraph is not.
- Treat `summary` as part of the same change as the
  content. The review process catches mismatches the
  same way it catches stale docstrings.

### The field you want to project is a list

`row` placeholders project scalar values only. A
front matter field that is a list or a map renders
as an empty string — there is no list-comprehension
or join syntax in the placeholder grammar.

For now, project a scalar derived from the list (a
joined comma-separated field maintained alongside
the structured one), or generate the catalog body
from a Go helper that walks the structured field
itself.

### Generated index conflicts on merge

Two branches both ran `mdsmith fix` and both committed
the regenerated catalog body — git reports a conflict
inside the `<?catalog?>` markers. Install the
[merge driver](../reference/cli/merge-driver.md)
once per clone:

```bash
mdsmith merge-driver install
```

The driver re-runs `fix` and re-stages the result, so
generated-section conflicts resolve without manual
intervention.

## See also

- [Generating Content with Directives](directives/generating-content.md)
  — full reference for `<?catalog?>` and `<?include?>`.
- [Generated sections concept](../background/concepts/generated-section.md)
  — markers, directives, and fix behavior.
- [Schemas](schemas.md) — require a `summary` field
  per directory.
- [`mdsmith list query`](../reference/cli/query.md) —
  same CUE grammar `where:` uses.
