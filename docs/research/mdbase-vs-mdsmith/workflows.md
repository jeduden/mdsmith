---
summary: >-
  Way-of-working comparison — daily authoring, repo bootstrap, schema
  evolution, file rename, CI pipeline, editor session, Obsidian-vault
  use, LLM/agent workflows, contributor onboarding.
---
# Way-of-working comparison

Features only matter when they meet a workflow. This
document walks the same tasks under each tool. The
question is not "what can each do" but "what does it
feel like to do this work."

## 1. Bootstrap a new project

**mdsmith.** Init writes a default config; the
first lint runs straight away.

```bash
cd my-docs
mdsmith init                # writes default .mdsmith.yml
mdsmith check .             # passes immediately on most repos
```

`mdsmith init` writes a config matching the
out-of-the-box rule defaults. The repo is now linted
on every `check`. Adding kinds, schemas, and
overrides is opt-in — projects with no special needs
never edit `.mdsmith.yml` after init.

**mdbase.** Init creates the collection skeleton;
the first type comes next.

```bash
cd my-collection
mdbase init                 # writes mdbase.yaml + _types/ folder
$EDITOR _types/note.md      # author the first type
mdbase validate             # validates current files
```

`mdbase init` creates the minimum collection: an
`mdbase.yaml` with `spec_version: "0.2.1"` and an
empty `_types/` folder. The user must author types
to get value. Untyped collections are valid (spec
permits zero types) but trivial — the tool acts as
a structured-YAML reader at that point.

Adoption path that the spec explicitly recommends
(Appendix D):

1. Start untyped (raw YAML front matter)
2. Run `mdbase infer-types` to bootstrap types from
   existing front matter
3. Tighten validation level from `off` → `warn` →
   `error` over time
4. Add path globs and field-presence rules to
   auto-match files

| Bootstrap step      | mdsmith              | mdbase                  |
|---------------------|----------------------|-------------------------|
| Init                | one command          | one command             |
| Useful immediately  | yes (rule defaults)  | only after typing files |
| Schema authoring    | optional             | central activity        |
| Run-cost first time | seconds (lints repo) | seconds (validates)     |

mdsmith is "lint everything by default; opt out
where wrong." mdbase is "describe types first; type
coverage grows over time."

## 2. Author a new file

**mdsmith.**
The user creates the file in any editor. mdsmith
runs on save (via LSP) or on commit (via pre-merge
hook or CI). Diagnostics surface where the file
violates rules; the user fixes them or lets
`mdsmith fix` rewrite the body.

mdsmith does not author files. The closest things
to a template are MDS020's schema files (a
`proto.md` whose body carries a heading skeleton
that real files must match) and the `<?include?>`
/ `<?catalog?>` directives (which fill in body
content).

**mdbase.**
The user can either:

- Edit a `.md` file by hand and save it
- Run `mdbase create task` to scaffold a typed file
  from defaults and `generated:` strategies (ULID,
  `now_on_write`, etc.)

`mdbase create` writes the front matter according
to the type. Required fields without defaults are
left blank for the user to fill in (or rejected if
strict mode is on).

| Authoring task                      | mdsmith                                    | mdbase                            |
|-------------------------------------|--------------------------------------------|-----------------------------------|
| Create `.md` from template          | hand or `<?include?>` from a template file | `mdbase create <type>`            |
| Fill required fields                | manual                                     | manual + auto-defaults            |
| Generated values (ULID, timestamps) | no                                         | yes (`generated:` strategies)     |
| Validate on write                   | n/a (mdsmith doesn't write)                | yes (validates before persisting) |
| Lint after write                    | yes (LSP / CI)                             | n/a (lint is mdsmith's domain)    |

mdsmith is post-hoc: edit then verify. mdbase has a
write-time path: scaffold then verify before commit.

## 3. Daily editor session

### mdsmith via VS Code extension

Diagnostics appear inline as the user types. The
"Quick Fix" lightbulb offers per-diagnostic fixes
and a "Fix all in file" action. On save, the file
relints. `.mdsmith.yml` changes invalidate the
session cache automatically.

What is missing today: completion, hover, and any
form of navigation (definition, references, outline,
call hierarchy). The shipped LSP is
diagnostic-and-fix only.

What is planned: hover for rule and directive docs
([plan 122][plan122w]) and full symbol navigation
(plan 131, tracked in [PR #238][pr238w]; the plan
file lives on that branch and is not yet on
`main`). After plan 131 lands, the editor session
also gets:

- File outline (`documentSymbol`) following the
  heading tree, with directives and front-matter
  keys hung off appropriately
- Go-to-definition from anchor links, file links,
  and `[text][label]` references
- "Find all references" on a heading, link-ref
  label, file, or `kind:` value
- Workspace symbol search across headings, labels,
  kinds, and front-matter titles
- Call hierarchy: "who includes / catalogs / links
  to this file" (incoming) and "what does this
  file include / link out to" (outgoing)

### mdbase via mdbase-lsp (Rust)

Diagnostics for type errors, link errors,
constraint violations. Plus:

- Completion for field names, enum values, type names
- Hover with type definition and field constraints
- Go-to-definition on link targets and types

What is missing: autofix. mdbase-lsp does not
rewrite content the way mdsmith does — it surfaces
errors but expects the user to resolve them.

### Both servers running

Many editors can run multiple LSPs on the same
buffer. The user gets:

- mdsmith → diagnostics + autofix on prose,
  structure, catalog, TOC, include; outline +
  navigation + call hierarchy after plan 131
- mdbase-lsp → diagnostics + completion + hover +
  go-to-definition on types and links

The two diagnostic streams use distinct rule IDs
and error codes, so duplicates are rare. Both
servers respect per-file front matter and read the
same on-disk content. Navigation results are
complementary: mdsmith plan 131 gives the
include/catalog graph; mdbase-lsp gives the
typed-link graph.

| Editor experience        | mdsmith today | mdsmith planned   | mdbase-lsp |
|--------------------------|---------------|-------------------|------------|
| Inline diagnostics       | yes           | yes               | yes        |
| Quick fix per diagnostic | yes           | yes               | no         |
| Fix all in file          | yes           | yes               | no         |
| Field-name completion    | no            | no                | yes        |
| Rule / directive hover   | no            | yes (plan 122)    | n/a        |
| Type / kind hover        | no            | no                | yes        |
| Go-to type definition    | no            | partial (`kind:`) | yes        |
| Go-to anchor / heading   | no            | yes (plan 131)    | yes        |
| Workspace symbol search  | no            | yes (plan 131)    | unknown    |
| File outline             | no            | yes (plan 131)    | partial    |
| Find references          | no            | yes (plan 131)    | unknown    |
| Call hierarchy           | no            | yes (plan 131)    | no         |
| Rename refactor          | no            | no                | yes (L5)   |
| Backlinks panel          | no            | yes (plan 131)    | yes (L5)   |

## 4. Schema evolution

**mdsmith.**
Editing a CUE schema is editing a regular file under
version control. The next `mdsmith check` re-validates
all files against the updated schema. Files that no
longer comply produce MDS020 diagnostics.

There is no migration mechanism: if a schema change
is breaking, the user fixes the affected files
manually, possibly in the same PR.

Typical patterns:

- Add an optional field — no migration needed
- Add a required field — existing files break; fix
  them in the same commit
- Tighten a constraint — existing violators surface;
  fix them or relax the constraint

**mdbase.**
Spec §5 ("Schema Evolution") and L6 migrations
codify the lifecycle.

Permitted changes without migration:

- Adding optional fields
- Adding default values
- Loosening constraints

Breaking changes (require migration manifest at L6):

- Adding required fields
- Changing field types
- Removing fields (treated per `strict:` setting)
- Tightening enum values

A migration manifest is a YAML file in the
`<types>/_migrations/` folder. It declares the
schema version delta and a backfill expression. The
implementation runs the backfill on `mdbase migrate`.

Example manifest pattern (illustrative):

```yaml
from_version: 1
to_version: 2
backfill:
  - type: task
    field: priority
    expr: 3   # default for the new required field
```

| Schema-evolution step | mdsmith                      | mdbase                                  |
|-----------------------|------------------------------|-----------------------------------------|
| Add optional field    | edit CUE; no migration       | edit type; no migration                 |
| Add required field    | edit CUE; manually fix files | migration manifest with backfill        |
| Change field type     | edit CUE; manually fix files | migration manifest                      |
| Track schema versions | git history                  | git history + migration version numbers |
| Bulk update files     | external scripts             | `mdbase migrate` (L6)                   |

mdbase's migration story is the more mature one for
mature collections. mdsmith's story is "it's just
text — change it and fix the breakage in the same
commit."

## 5. Rename a file

**mdsmith.** Plain `mv` plus a re-lint surfaces broken
links; rewriting them is manual.

```bash
mv docs/old-name.md docs/new-name.md
mdsmith check .
# MDS027 reports broken links in every file that
# referenced docs/old-name.md
```

Then fix each broken link manually or via search-
and-replace. A separate refactoring tool can
automate this; mdsmith does not provide one.

### mdbase L5

```bash
mdbase rename docs/old-name.md docs/new-name.md
# moves the file; rewrites incoming wikilinks and
# Markdown links across the collection; preserves
# link styles; reports per-file failures
```

The rename is a single command. Reference updates
are best-effort and per-file: a permission error on
one file does not abort the rename. The user gets a
list of files that were not updated.

| Rename step            | mdsmith                  | mdbase L5                     |
|------------------------|--------------------------|-------------------------------|
| Move file              | `mv`                     | `mdbase rename`               |
| Detect broken links    | `mdsmith check` (MDS027) | n/a (already rewritten)       |
| Auto-rewrite incoming  | no                       | yes                           |
| Preserve wikilink form | n/a                      | yes (wikilink stays wikilink) |
| Concurrent-edit safety | n/a (no rewrite)         | optimistic mtime check        |

Rename is the most common task where mdsmith feels
the absence of a graph. Teams that rename often,
especially in vault-style projects, get more value
from mdbase here.

## 6. Find and select files

**mdsmith.** A CUE struct literal filters files by
front matter.

```bash
mdsmith query 'status: "✅"' plan/
# emits paths of plan files where status is "✅"
```

The output is a list of file paths. Pipe into other
tools as needed. There is no body search, no sort,
no pagination.

**mdbase.** A query document carries `types`,
`where`, sort and pagination.

```bash
mdbase query '
  types: [task]
  where: status == "open" && priority >= 3
  order_by: [{field: due, direction: asc}]
  limit: 20
'
# emits a structured result set
```

The output is a JSON array of file objects with
metadata (path, types, effective FM, mtime, size,
optionally body). Sortable, paginatable, with body
search via `file.body.contains(...)`.

| Selection task         | mdsmith             | mdbase                    |
|------------------------|---------------------|---------------------------|
| Filter by FM field     | yes                 | yes                       |
| Sort by FM field       | no (pipe to `sort`) | yes                       |
| Paginate               | no (head/tail)      | yes (`limit`/`offset`)    |
| Body full-text search  | no                  | yes (`file.body`)         |
| Cross-file traversal   | no                  | yes (`asFile().property`) |
| Aggregation / grouping | no                  | yes (`Query+`)            |
| Available in scripts   | yes                 | yes                       |

For ad-hoc CI checks ("filter to ready plans"),
mdsmith query suffices. For knowledge-base browsing
("show overdue tasks grouped by assignee"), mdbase
is built for it.

## 7. CI pipeline

### mdsmith-only repo

```yaml
# .github/workflows/lint.yml
- run: go install github.com/jeduden/mdsmith/cmd/mdsmith@vX.Y.Z
- run: mdsmith check .
```

One install step, one check command, one binary on
the PATH. Total CI time is dominated by Go install.

### mdbase-only repo

```yaml
- run: npm install -g mdbase-cli
- run: mdbase validate
- run: mdbase query 'where: status == "draft"' >/tmp/drafts.json
```

Node setup plus validation. Optional cache build
(`mdbase cache rebuild`) for repeat queries.

### Both

```yaml
- run: go install github.com/jeduden/mdsmith/cmd/mdsmith@vX.Y.Z
- run: npm install -g mdbase-cli
- run: mdsmith check .
- run: mdbase validate
```

Two phases, two reports. Unify into a single status
check or surface as separate checks. Diagnostics
do not overlap: mdsmith reports prose and structure
issues; mdbase reports schema and link issues.

| CI concern               | mdsmith only      | mdbase only       | both          |
|--------------------------|-------------------|-------------------|---------------|
| Single tool install      | go binary         | npm package       | go + npm      |
| Cold-cache lint time     | seconds           | seconds           | additive      |
| Lint failure exits CI    | yes               | when `error` mode | both          |
| Diff-only mode           | path-based filter | path-based filter | both per side |
| Output format for review | text or JSON      | JSON              | per-tool      |

## 8. Pre-commit / pre-merge

**mdsmith.** Two installers wire git hooks: the
pre-merge-commit runs `mdsmith fix` after a merge,
and the merge driver auto-resolves conflicts inside
generated sections.

```bash
mdsmith pre-merge-commit install
mdsmith merge-driver install '*.md'
```

The pre-merge-commit hook runs `mdsmith fix` after
a merge. The merge driver auto-resolves conflicts
inside generated sections (catalog, include, TOC,
build) by regenerating them. Both are mdsmith-specific.

**mdbase.**
The spec does not mandate hooks. An impl could ship
a `pre-commit` style hook to run `mdbase validate`,
but it is not part of the spec. Teams typically wire
this themselves via `lefthook`, `husky`, or
`pre-commit`.

| Hook concern                    | mdsmith                            | mdbase          |
|---------------------------------|------------------------------------|-----------------|
| Pre-commit lint                 | external runner                    | external runner |
| Pre-merge-commit fix            | `mdsmith pre-merge-commit install` | external        |
| Merge-conflict driver           | `mdsmith merge-driver`             | none            |
| Auto-resolve generated sections | yes                                | n/a             |

Generated-section conflicts are an mdsmith-specific
problem (mdbase does not generate body content).
The merge driver only matters for users with
catalog / TOC / include / build directives.

## 9. Obsidian vault use case

A team that maintains an Obsidian vault and wants
both linting and structured data:

- mdbase fits naturally — vault-style folders, type
  files in `_types/`, wikilinks first-class, Bases
  query syntax familiar to Dataview users
- mdsmith adds structural and prose linting that
  Obsidian itself does not provide

Vaults need:

- Wikilink validation → mdbase (mdsmith treats them
  as text)
- Heading and table consistency → mdsmith
- Front-matter type checks → either or both
- Backlink panel → mdbase via LSP
- Daily-note template generation → external (Obsidian
  Templater plugin) or mdsmith `<?include?>`

See [interop.md](interop.md) for how to lay out a
vault that runs both tools.

## 10. LLM / agent workflows

This is where the file-first design pays off for
both tools.

### Agent reads a file

The agent reads the raw `.md` file. Front matter is
the structured layer. Both tools see the same bytes.
No build step, no `public/` artifact tree, no
proprietary format. The agent can:

- Open the file with any tool
- Make edits
- Save the file
- Re-run `mdsmith check` or `mdbase validate` to
  verify

### Agent writes a file

mdsmith does not author files; the agent produces
the body and front matter. After writing, the agent
runs `mdsmith fix` to regenerate any directive
bodies (catalog, TOC, include).

mdbase has `mdbase create <type>`. The agent can
delegate scaffolding to mdbase, get a typed shell
with defaults filled in, and then fill in the body.

### Agent renames a file

mdsmith: `mv` then fix broken links by hand. The
agent has to find every link.

mdbase: `mdbase rename` does the work. The agent
issues one command.

### Agent runs queries

Both tools support querying. mdbase's richer query
language lets the agent ask "all files with status
== 'open' and priority >= 3" without iterating.
mdsmith's CUE query covers the simpler "filter by
field" case.

### Agent verifies its own work

Both tools are CI-friendly: deterministic, exit
code, JSON output. After an edit, the agent runs
the relevant tool, parses the JSON, and decides
what to fix next.

### Agent navigates over LSP

Some agents (Claude Code, for one) have a built-in
LSP tool that exposes nine standard LSP methods —
`textDocument/documentSymbol`, `definition`,
`implementation`, `hover`, `references`,
`workspace/symbol`, plus `prepareCallHierarchy` and
the two call-direction methods. Both tools target
this surface, but differently.

- **mdbase-lsp** today gives the agent the typed
  view: completion of field names, hover with
  type definitions, go-to-definition on a link's
  target. Symbol-level workspace navigation
  (find-references on a heading, outline,
  call hierarchy on the include graph) is not
  advertised in the project README at the time of
  writing.
- **mdsmith** today exposes only diagnostics and
  code actions over LSP. Plan 131 ([PR #238][pr238w];
  the plan file is on that branch, not yet on
  `main`) adds `documentSymbol`, `definition`,
  `implementation`, `references`, `workspace/symbol`,
  and call hierarchy. The plan explicitly maps the
  nine Claude-LSP methods onto the existing AST and
  link graph.

Once plan 131 lands, an agent can ask the mdsmith
LSP "what files include this runbook?" via
`incomingCalls`, "what does this overview embed?"
via `outgoingCalls`, "show me the outline" via
`documentSymbol`, and "find every link to this
heading" via `references` — all without leaving
the LSP protocol.

### Side-by-side agent surface

| Agent task                 | mdsmith today  | mdsmith planned | mdbase-lsp      |
|----------------------------|----------------|-----------------|-----------------|
| Read file                  | direct         | direct          | direct          |
| Write body                 | direct + fix   | direct + fix    | direct          |
| Scaffold typed file        | n/a            | n/a             | `mdbase create` |
| Validate FM shape          | `check` MDS020 | `check` MDS020  | `validate`      |
| Rename + update refs       | manual         | manual          | `mdbase rename` |
| Query collection           | `query` (CUE)  | `query` (CUE)   | `query` Bases   |
| Block on broken links      | `check` MDS027 | `check` MDS027  | `validate`      |
| File outline (LSP)         | no             | yes (plan 131)  | partial         |
| Go-to-def on anchor link   | no             | yes (plan 131)  | yes             |
| Find heading references    | no             | yes (plan 131)  | unknown         |
| Workspace symbol search    | no             | yes (plan 131)  | unknown         |
| Call hierarchy on includes | no             | yes (plan 131)  | no              |
| Hover rule docs            | no             | yes (plan 122)  | n/a             |
| Hover type / field         | no             | no              | yes             |

For agent-driven docs work the two tools are
complementary. mdsmith excels at "verify and fix
content" plus, after plan 131, "navigate the
document graph"; mdbase excels at "navigate and
edit typed records."

## 11. Onboarding a new contributor

### mdsmith repo

The contributor:

1. Clones the repo
2. Builds from the clone with
   `go install ./cmd/mdsmith`, or pins a release
   with
   `go install github.com/jeduden/mdsmith/cmd/mdsmith@vX.Y.Z`
3. Reads `.mdsmith.yml` (and CLAUDE.md if present)
4. Runs `mdsmith check .` and fixes any drift
5. Runs `mdsmith help <rule-id>` to read embedded
   docs offline

`mdsmith kinds` shows the resolved rule config per
file, helpful for understanding why a particular
file lints differently from neighbours.

### mdbase repo

The contributor:

1. Clones the repo
2. Installs `mdbase-cli` (npm) or `mdbase-lsp` (cargo)
3. Reads `mdbase.yaml` for config
4. Reads `_types/*.md` — every type file is a
   self-documenting Markdown file with the schema
   in its front matter and a description in its body
5. Runs `mdbase validate` to confirm the collection
   is healthy

mdbase has a discoverability advantage here: the
type files are visible in any file tree, and each
has its own documentation. mdsmith's kinds live
inside `.mdsmith.yml`, which is a single config file
the contributor must read top to bottom.

| Onboarding step          | mdsmith                                   | mdbase                            |
|--------------------------|-------------------------------------------|-----------------------------------|
| Install                  | one binary                                | npm + cargo                       |
| Config to read           | `.mdsmith.yml`                            | `mdbase.yaml` + `_types/`         |
| Per-type docs            | `internal/rules/<id>/README.md` (in repo) | `_types/<name>.md` body (in repo) |
| Inspect effective config | `mdsmith kinds`                           | `mdbase types --inspect`          |
| Offline rule docs        | `mdsmith help <id>`                       | spec doc on GitHub                |

## 12. Mental model summary

| Concept                         | mdsmith says              | mdbase says               |
|---------------------------------|---------------------------|---------------------------|
| What's a file?                  | Text to lint and fix      | A typed record            |
| What's metadata?                | YAML the rules read       | Effective FM + computed   |
| Who validates?                  | Per-rule severity         | Per-mode (off/warn/error) |
| Who creates?                    | Humans (mdsmith doesn't)  | Humans or `mdbase create` |
| Who renames?                    | `mv` + manual link fix    | `mdbase rename`           |
| Who indexes?                    | No one (re-read each run) | SQLite (L6)               |
| Who watches?                    | LSP per-session           | Watch mode (L6)           |
| Who lints prose?                | mdsmith                   | (out of scope)            |
| Who handles structure rules?    | mdsmith                   | (out of scope)            |
| Who generates content?          | mdsmith directives        | (out of scope)            |
| Who generates artifacts (HTML)? | external SSG              | external SSG              |

A mental shortcut:

- **mdsmith** treats Markdown as text that should
  follow rules.
- **mdbase** treats Markdown as records in a folder-
  database.

Both can be true of the same files at the same time.
The combined workflow gets both: typed, queryable,
rename-safe records that are also well-formed,
readable, and have synced generated sections.

[plan122w]: ../../../plan/122_vscode-hover-and-palette.md
[pr238w]: https://github.com/jeduden/mdsmith/pull/238
