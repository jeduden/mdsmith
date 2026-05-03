---
id: 101
title: build directive and MDS039 lint rule
status: "✅"
summary: >-
  Add the `<?build?>` directive and MDS039, which
  validates directive params against the recipe's
  declared schema and keeps the body in sync with
  the rendered `body_template` on `mdsmith fix`.
  No external tool runs at lint time.
model: sonnet
---
# build directive and MDS039 lint rule

## Goal

Let authors declare a build artifact with a
`<?build?>` directive. `mdsmith fix` renders the
body from the recipe's `body_template`. MDS039
validates params and reports a stale-section
diagnostic when the rendered body diverges from
the actual body — without running any external tool.

## Context

Depends on plan 100 (`build:` config block and
MDS040). Plan 102 adds the actual build execution.
After this plan, `mdsmith check` and `mdsmith fix`
work end-to-end for `<?build?>` blocks; artifact
files are not yet produced.

All recipes must be declared in `build.recipes` in
`.mdsmith.yml`. There are no built-in recipes.

## Design

### Directive syntax

```text
<?build
recipe: render
source: diagram.svg
output: docs/diagram.png
?>
![render output: docs/diagram.png](docs/diagram.png)
<?/build?>
```

The directive uses the same block form as
`<?catalog?>` and `<?include?>`. Inline form is
not supported.

Common parameters (all recipes):

| Name     | Required | Description                                                     |
|----------|----------|-----------------------------------------------------------------|
| `recipe` | yes      | Recipe name declared in `build.recipes`                         |
| `output` | yes      | Artifact path relative to the Markdown file; no `..` components |

`output` accepts any file extension; MDS039 applies
no extension filter.

### Generated body

Each recipe has a `body_template` rendered by
`mdsmith fix`:

| Placeholder | Value                                   |
|-------------|-----------------------------------------|
| `{output}`  | The `output` param value                |
| `{alt}`     | `"{recipe} output: {output}"` (default) |

When `body-template` is omitted from the recipe
declaration, the default `[{output}]({output})`
is used.

User-declared recipes may set `body_template`
in `build.recipes.NAME.body-template` (plan 100).

### Rule: MDS039 (build)

- ID: `MDS039`
- Name: `build`
- Category: `meta`
- Default: enabled
- Fixable: yes (body only)

Validation:

1. **`recipe` resolves** — the recipe name must be
   declared in `build.recipes`.
2. **`output` is safe** — relative path, no `..`
   components, inside the project root.
3. **Required params present** — `params.required`
   entries from the recipe schema must all be
   supplied by the directive.
4. **No unknown params** — params not in the recipe's
   `required` or `optional` lists produce a warning.
5. **Body in sync** — the section body must equal the
   rendered `body_template`. MDS039 reports
   `stale-section` when it diverges; `Fix` rewrites
   the body using `gensection.Engine`.

A Markdown file can only reference recipes declared
in `.mdsmith.yml`. It cannot introduce a new recipe.

### Interaction with existing rules

- **MDS027**: a missing artifact file fires MDS027
  independently; MDS039 does not duplicate it.
- **MDS040**: validates `build.recipes` command
  safety; MDS039 validates `<?build?>` usage in
  Markdown files.
- **merge-driver**: regenerates the `<?build?>` body
  on conflict; artifact bytes are not regenerated.

## Tasks

1. [x] Implement the `<?build?>` directive in
   `internal/rules/build/` using `gensection.Engine`.
   Register as MDS039, category `meta`. `Generate`
   renders `body_template` only; it never calls a
   builder or touches the filesystem.
2. [x] Implement MDS039 validation (recipe resolution,
   `output` path safety, required params, unknown
   params, stale-section body check).
3. [x] Add `good/`, `bad/`, and `fixed/` fixtures for
   MDS039 under `internal/rules/MDS039-build/`.
4. [x] Wire MDS039 into `cmd/mdsmith/main.go`.
5. [x] Document MDS039 in
   `internal/rules/MDS039-build/README.md`.
6. [x] Add user guide at
   `docs/guides/directives/build.md` covering the
   directive syntax and how `mdsmith fix` keeps
   the body in sync.

## Acceptance Criteria

- [x] `<?build?>` body is regenerated on `mdsmith fix`
      using the recipe's `body_template`
- [x] MDS039 reports `stale-section` when the body
      diverges from the rendered `body_template`
- [x] `mdsmith check` does **not** run any external
      tool for `<?build?>` blocks
- [x] MDS039 rejects an unknown recipe name
- [x] MDS039 rejects a missing `output` param
- [x] MDS039 rejects an `output` value that contains
      `..` components
- [x] MDS039 rejects a directive that omits a
      required param declared by the recipe
- [x] MDS039 warns on a param not in the recipe's
      `required` or `optional` lists
- [x] A Markdown file cannot introduce a new recipe;
      it can only reference recipes in `.mdsmith.yml`
- [x] `output` accepts any file extension; no
      extension filter is applied
- [x] The rendered `body_template` uses `{alt}`
      defaulting to `"{recipe} output: {output}"`
- [x] A user-declared recipe's `body_template` from
      `build.recipes` is used instead of the default
- [x] Merge driver regenerates `<?build?>` bodies on
      conflict (via `gensection.Engine`)
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
