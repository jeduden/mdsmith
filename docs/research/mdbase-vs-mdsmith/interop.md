---
summary: >-
  Running mdsmith and mdbase on the same files — coexistence model, the
  dual-schema problem, recommended layouts, suggested integration
  patterns, and a sketch of a future schema bridge.
---
# Interop and combined use

This document covers what happens when both tools
read the same files. The short answer: they coexist
cleanly on disk because their write surfaces do not
collide, but the dual-schema problem makes typed
front matter more work than it should be.

## 1. Coexistence on disk

Both tools read YAML front matter and Markdown body
content. Neither tool requires a proprietary
serialization. Files written by one are valid for
the other.

Write surfaces:

| Tool    | What it writes                                                                                                                                                              |
|---------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| mdsmith | Body fixes (`mdsmith fix`); generated section bodies (catalog, TOC, include, build); `.gitattributes` + git hooks via merge-driver / pre-merge-commit installers            |
| mdbase  | Front-matter via CRUD ops (`create`, `update`); incoming-link rewrites on `rename`; SQLite cache at `.mdbase/index.sqlite`; migration manifests under `_types/_migrations/` |

These do not overlap. Specifically:

- mdsmith does not edit front matter (rules read it;
  fixes target body content)
- mdbase does not edit body content (CRUD targets
  fields, not paragraphs; rename rewrites links but
  not surrounding prose)

So a file edited by mdbase remains lint-clean if it
was clean before, and a file fixed by mdsmith
remains valid against its mdbase type if it was
valid before. The exception is the front-matter
*shape*, which both tools have opinions about — that
is the dual-schema problem.

## 2. The dual-schema problem

Both tools want to validate front matter, and
neither can read the other's schema language.

| Concern                            | mdsmith                                                                    | mdbase                               |
|------------------------------------|----------------------------------------------------------------------------|--------------------------------------|
| Schema file format                 | Markdown `proto.md` (CUE in front matter, heading template in body)        | Markdown in `_types/<name>.md` (DSL) |
| Schema reference                   | `kinds: { <name>: { rules: { required-structure: { schema: <path> } } } }` | folder convention `_types/`          |
| Schema location                    | anywhere referenced by a kind, by convention next to the files             | fixed `_types/` folder               |
| Schema discoverable from file tree | yes — `proto.md` is a normal Markdown file                                 | yes — `_types/*.md` are normal files |
| Reads the other's schema           | no                                                                         | no                                   |

The pain points:

1. **Define-once is impossible.** A team that wants
   typed FM under both tools maintains two
   definitions of every type. Field renames must be
   applied twice. Constraint changes must be
   applied twice.
2. **Drift is silent.** If only one schema is
   updated, the other tool keeps validating against
   the old shape. A file might pass mdsmith and fail
   mdbase, or vice versa.
3. **Tooling cannot cross-reference.** A schema
   editor for mdsmith does not see mdbase types; a
   schema editor for mdbase does not see CUE
   constraints.

There is no automated bridge today. Strategies that
work in practice are below.

## 3. Strategies for combined use

### Strategy A: Use mdbase as the source of truth

Pick mdbase types as the authoritative schema. Use
mdsmith for prose, structure, generated content,
and link integrity. Decide explicitly what to do
with MDS020.

Two configurations work, with different
trade-offs:

**A.1: Disable MDS020 entirely.** Simplest. The
mdbase types validate front matter; mdsmith does
not touch FM shape:

```yaml
# .mdsmith.yml
rules:
  required-structure: false  # mdbase owns all FM + structure checks
```

Note this also disables MDS020's heading-template
enforcement (the body-structure half of the rule),
since `false` toggles the whole rule. If the
project wants mdsmith to keep enforcing the
heading template while letting mdbase own FM
typing, use A.2 instead.

**A.2: Keep mdsmith's heading-template
enforcement (caveat).** The intent is "mdsmith
validates body structure, mdbase validates FM,
neither overlaps". mdsmith does not currently
ship a clean way to express this: MDS020's
schema is one document with FM constraints AND
a body template, and the rule applies both.
Three partial routes are available, each with
trade-offs:

- **Schema with empty front-matter block.**
  Author a `proto.md` whose YAML front matter is
  empty (still has the `---` fences but no
  keys), so the CUE schema is empty and matches
  any FM. The body template still validates
  headings. Verify on your repo: behavior on
  unknown FM keys depends on the rule's
  `default_strict` and any per-kind override.
- **Schema that mirrors mdbase's types.** Write
  the same FM constraints in mdsmith CUE that
  mdbase has in DSL. Both tools then validate FM
  redundantly. Tedious to keep in sync; bundles
  the dual-schema problem this whole strategy
  was trying to avoid.
- **Disable required-structure entirely (back
  to A.1).** Simplest. Accepts the loss of
  body-structure enforcement.

A future mdsmith config option that toggles
"validate body structure only, skip FM" within
MDS020 would close the gap cleanly. Today
A.1 is the practical pick unless body
heading-template enforcement is critical.

Pros:

- One source of FM truth (`_types/`)
- mdsmith stays focused on linting and structure
- The schema is self-documenting (each type file
  has a body explaining the type)

Cons:

- mdsmith cannot block PRs on FM-shape errors
  under A.1 (rely on `mdbase validate` in CI)
- The CUE constraint surface (cross-field, regex,
  bounds) is not available under A.1
- A.2's three routes each carry the caveats
  above; none is fully clean today

This is the recommended pattern when you want the
mdbase typed-vault experience plus mdsmith's
linting.

### Strategy B: Use mdsmith CUE as the source of truth

The opposite: keep CUE as the authoritative schema.
Use mdbase as a query and rename tool only, with
its validation set to `off`:

```yaml
# mdbase.yaml
spec_version: "0.2.1"
default_validation: off
```

Pros:

- One schema layer (CUE) to maintain
- CUE's full constraint power
- Seamless on existing mdsmith repos

Cons:

- mdbase types still exist as data-layer hints
  (otherwise queries can't filter by type)
- Can't use mdbase create/update because they
  validate-on-write — workaround: use raw FM editing
  and rely on mdsmith for shape

This pattern fits teams already invested in mdsmith
who want only mdbase's rename-and-query features.

### Strategy C: Maintain both, by hand

Keep CUE schemas and mdbase types in sync manually.
Convention: a single PR updates both. A pre-commit
check could compare the two but no such tool exists
today.

Pros:

- Both tools enforce on PRs; defense in depth
- Schema authoring stays close to either tool's
  natural surface

Cons:

- Drift is the default state
- Diff review must check that schemas match

Use only if both validation surfaces matter and the
drift cost is acceptable.

### Strategy D: Generate one from the other

Write a script that:

- Reads `_types/*.md` (mdbase types)
- Emits matching mdsmith `proto.md` schema files
  next to the matching kind, with the type DSL
  translated into CUE in the front matter and the
  documentation body preserved or replaced with a
  heading template

Or the reverse direction. Run the script in CI; fail
if the generated output drifts from committed
content.

Pros:

- Single source of truth, low drift risk
- Can be incremental (only types that need both)

Cons:

- The script is real engineering effort
- Some constraints don't translate (CUE regex →
  mdbase `pattern:` works; CUE cross-field
  constraints → mdbase has no equivalent)

This is the "right answer" but requires building a
bridge nobody has built yet. See section 7 for a
sketch.

## 4. Recommended folder layout

Below is one layout that runs both tools cleanly.

```text
my-vault/
├── mdbase.yaml              # mdbase config
├── .mdsmith.yml             # mdsmith config (kinds + rules)
├── _types/                  # mdbase type defs (Strategy A)
│   ├── note.md
│   ├── task.md
│   └── person.md
├── notes/                   # actual content
│   ├── proto.md             # mdsmith schema (Strategy B/C; CUE in FM)
│   ├── 2026-05-05-meeting.md
│   └── …
├── tasks/
│   ├── proto.md             # mdsmith schema for tasks
│   └── …
├── _types/_migrations/      # mdbase migrations (L6)
└── .mdbase/                 # gitignored cache
    └── index.sqlite
```

`.gitignore` entries:

```text
.mdbase/
```

`.mdsmith.yml` settings — pick the granularity
deliberately. Two copy/paste-valid options:

**Option 1: full exclusion** (no mdsmith lint at
all on `_types/`).

```yaml
# .mdsmith.yml
ignore:
  - "_types/**"      # mdbase owns these end-to-end
  - ".mdbase/**"     # gitignored cache, never lint
```

**Option 2: lint prose/whitespace in `_types/`,
skip schema-validation conflicts only.** Disable
just `required-structure` on `_types/` via an
override; everything else still runs.

```yaml
# .mdsmith.yml
overrides:
  - glob: ["_types/**"]
    rules:
      required-structure: false  # mdbase owns FM shape
ignore:
  - ".mdbase/**"
```

`ignore:` skips **all** mdsmith rules — including
prose readability, line length, table formatting,
and link integrity — for the matched files. That
matches the "mdbase owns these end-to-end"
intent in Option 1. If the team wants type
files to be linted for prose and whitespace
consistency (they are normal Markdown after all)
but doesn't want MDS020 fighting mdbase, the
`overrides:` form in Option 2 is the better fit.

`mdbase.yaml` `exclude:` entries:

```yaml
exclude:
  - .git
  - node_modules
  - .mdbase
  - .mdsmith.yml   # not a content file
```

This keeps each tool out of the other's territory.
mdsmith does not lint type files (mdbase owns them).
mdbase does not consider hidden config files as part
of the collection.

## 5. Rules to disable when using both

mdsmith rules to consider disabling under
**Strategy A (mdbase owns FM)**:

| Rule             | Reason to disable                                |
|------------------|--------------------------------------------------|
| MDS020           | mdbase validates FM shape                        |
| MDS027           | optional: mdbase L4 also checks links — pick one |
| MDS019 (catalog) | mdbase queries can produce equivalent listings   |

Keep these:

- All prose / structure / readability rules — mdbase
  has nothing equivalent
- `<?include?>` — useful regardless
- `<?toc?>` — useful regardless
- Merge driver — useful regardless

Conversely under **Strategy B (mdsmith owns FM)**,
disable mdbase validation:

```yaml
# mdbase.yaml
default_validation: off
```

…and rely on `mdsmith check` for FM shape. mdbase
becomes a query/rename tool only.

## 6. Sample CI pipeline

Combined CI for a vault running both tools:

```yaml
# .github/workflows/lint.yml
name: lint
on: [push, pull_request]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'   # match mdsmith's go.mod
      - uses: actions/setup-node@v4
        with:
          node-version: '22'
      # Pin a specific mdsmith release to keep CI reproducible.
      - run: go install github.com/jeduden/mdsmith/cmd/mdsmith@vX.Y.Z
      - run: npm install -g mdbase-cli
      - run: mdsmith check .
      - run: mdbase validate
```

Both run on every PR. They produce independent
reports; either failing fails the job. Order does
not matter — the tools do not write during validation.
The Go version should track mdsmith's `go.mod`
directive; pinning the install version (rather than
`@latest`) keeps CI reproducible across mdsmith
releases.

If you also want fix-mode in CI (as a regression
guard against drift in generated sections):

```yaml
      - run: mdsmith fix .
      - run: |
          if ! git diff --quiet; then
            echo "mdsmith fix produced changes; commit them"
            git --no-pager diff
            exit 1
          fi
```

This catches a contributor who edited a generated
section by hand.

## 7. Future: a schema bridge

There is room for a small tool that translates
between the two schema surfaces. Sketch:

```text
                    mdbase _types/*.md
                        │
                        ▼
              ┌─────────────────────┐
              │  schema-bridge tool │
              │   (read both,       │
              │   diff, generate)   │
              └─────────────────────┘
                        │
                        ▼
              mdsmith proto.md schemas
              (CUE in front matter,
               heading template in body)
```

A first cut would handle the common subset:

- `string` with `min_length` / `max_length` / `pattern`
  → CUE `=~ "regex" & strings.MinRunes(N)`
- `integer` with `min`/`max` → CUE `int & >=N & <=M`
- `enum` with `values` → CUE disjunction
- `list` with `items` → CUE list type
- `object` with `fields` → CUE struct
- `required` → CUE non-optional
- `default` → CUE default value `*"x" | string`

Things that don't translate cleanly:

- mdbase `link` with `target` — needs cross-file
  resolution that CUE alone cannot express
- mdbase `computed:` — CUE has expressions but
  evaluation semantics differ
- mdbase `generated:` (ULID, timestamps on write) —
  CUE is purely structural; no write-time hooks
- CUE cross-field constraints (`if x then y >= 0`) —
  mdbase has no equivalent

The bridge tool would round-trip the common cases
and emit warnings where one side has constraints
the other cannot represent. This is a clear
opportunity for a small contribution to either
ecosystem.

## 8. When to skip one tool

A team running both does not always benefit. Cases
where one tool alone is enough:

| Project shape                                          | Use only            |
|--------------------------------------------------------|---------------------|
| Open-source repo with `docs/` and a README             | mdsmith             |
| Static site Markdown (Hugo / Jekyll / Astro)           | mdsmith + SSG       |
| Personal Obsidian vault, no CI, no schemas             | mdbase (or neither) |
| Knowledge graph with backlinks and rename-heavy work   | mdbase              |
| Plan / RFC tracker with status fields and prose review | mdsmith             |
| Mixed vault + docs site for a product                  | both                |

The core trigger to add the second tool:

- Add **mdsmith** when you start caring about prose
  consistency, structural rules, generated TOCs,
  or Markdown across many contributors.
- Add **mdbase** when you start caring about typed
  records, rename safety, backlinks, or queries
  beyond `grep`.

## 9. Open questions

A few things this comparison doesn't answer:

1. How does mdbase's SQLite cache interact with
   editor saves on Windows where `.mdbase/index.sqlite`
   is a binary file with active locks?
2. Does mdbase-lsp expose code actions for adding a
   missing required field, the way mdsmith does for
   formatting fixes?
3. Can mdsmith ship a `link-rename` subcommand that
   approximates mdbase L5 rename for projects that
   don't want a second tool?
4. Is there appetite in either project to specify a
   shared FM-schema interchange format?

These are tractable questions, but answering them
needs experimentation against real vaults. The
research note ends here; the experiments are
follow-up work.
