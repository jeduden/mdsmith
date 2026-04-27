---
id: 101
title: build directive and MDS039 lint rule
status: "🔲"
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

Built-in recipes (`screenshot`, `vhs`) are defined
here with their param schemas. Their execution
implementations live in plan 102.

## Design

### Directive syntax

```text
<?build
recipe: screenshot
url: /inbox
output: docs/inbox.png
?>
![screenshot output: docs/inbox.png](docs/inbox.png)
<?/build?>
```

```text
<?build
recipe: vhs
input: demo.tape
output: demo.gif
?>
![vhs output: demo.gif](demo.gif)
<?/build?>
```

The directive uses the same block form as
`<?catalog?>` and `<?include?>`. Inline form is
not supported.

Common parameters (all recipes):

| Name     | Required | Description                                                     |
|----------|----------|-----------------------------------------------------------------|
| `recipe` | yes      | Built-in or user-declared recipe name                           |
| `output` | yes      | Artifact path relative to the Markdown file; no `..` components |

`output` accepts any file extension; MDS039 applies
no extension filter.

### Built-in recipe param schemas

#### `screenshot`

| Param      | Required | Default    |
|------------|----------|------------|
| `url`      | yes      | —          |
| `selector` | no       | full page  |
| `viewport` | no       | `1280x800` |
| `wait`     | no       | `0` ms     |
| `click`    | no       | —          |
| `hide`     | no       | `[]`       |

#### `vhs`

| Param   | Required |
|---------|----------|
| `input` | yes      |

Built-in recipe params are hard-coded in MDS039.
User-declared recipes read their schemas from
`build.recipes` in `.mdsmith.yml` (plan 100).

### Generated body

Each recipe has a `body_template` rendered by
`mdsmith fix`. Built-in defaults:

| Recipe       | Default `body_template` |
|--------------|-------------------------|
| `screenshot` | `![{alt}]({output})`    |
| `vhs`        | `![{alt}]({output})`    |
| custom       | `[{output}]({output})`  |

`{alt}` defaults to `"{recipe} output: {output}"`.
`{output}` is the directive's `output` param value.
The rendered body always satisfies MDS032
(non-empty alt text for images).

User-declared recipes may override `body_template`
in `build.recipes.NAME.body_template` (plan 100).

### Rule: MDS039 (build)

- ID: `MDS039`
- Name: `build`
- Category: `meta`
- Default: enabled
- Fixable: yes (body only)

Validation:

1. **`recipe` resolves** — the recipe name must be a
   built-in (`screenshot`, `vhs`) or declared in
   `build.recipes`.
2. **`output` is safe** — relative path, no `..`
   components, inside the project root.
3. **Required params present** — built-in required
   params (e.g. `url` for `screenshot`) and
   user-declared `params.required` entries must all
   be supplied by the directive.
4. **No unknown params** — params not in the recipe's
   `required` or `optional` lists produce a warning.
5. **Body in sync** — the section body must equal the
   rendered `body_template`. MDS039 reports
   `stale-section` when it diverges; `Fix` rewrites
   the body using `gensection.Engine`.

A Markdown file can reference only recipes declared
in `.mdsmith.yml` or built-in recipes. It cannot
introduce a new recipe.

### Interaction with existing rules

- **MDS032**: `{alt}` in the default `body_template`
  ensures alt text is always non-empty.
- **MDS027**: a missing artifact file fires MDS027
  independently; MDS039 does not duplicate it.
- **merge-driver**: regenerates the `<?build?>` body
  on conflict; artifact bytes are not regenerated.

## Tasks

1. Implement the `<?build?>` directive in
   `internal/rules/build/` using `gensection.Engine`.
   Register as MDS039, category `meta`. `Generate`
   renders `body_template` only; it never calls a
   builder or touches the filesystem.
2. Add built-in recipe schemas for `screenshot` and
   `vhs` (param names, required/optional lists,
   default `body_template`).
3. Implement MDS039 validation (recipe resolution,
   `output` path safety, required params, unknown
   params, stale-section body check).
4. Add `good/`, `bad/`, and `fixed/` fixtures for
   MDS039 under `internal/rules/MDS039-build/`.
5. Wire MDS039 into `cmd/mdsmith/main.go`.
6. Document MDS039 in
   `internal/rules/MDS039-build/README.md`.
7. Add user guide at
   `docs/guides/directives/build.md` covering the
   directive syntax, built-in recipes, and how
   `mdsmith fix` keeps the body in sync.

## Acceptance Criteria

- [ ] `<?build recipe:screenshot url:... output:...?>`
      body is regenerated on `mdsmith fix`
- [ ] `<?build recipe:vhs input:demo.tape output:demo.gif?>`
      body is regenerated on `mdsmith fix`
- [ ] MDS039 reports `stale-section` when the body
      diverges from the rendered `body_template`
- [ ] `mdsmith check` does **not** run any external
      tool for `<?build?>` blocks
- [ ] MDS039 rejects an unknown recipe name
- [ ] MDS039 rejects a missing `output` param
- [ ] MDS039 rejects an `output` value that contains
      `..` components
- [ ] MDS039 rejects a `<?build recipe:screenshot?>`
      that omits the required `url` param
- [ ] MDS039 warns on a param not in the recipe's
      `required` or `optional` lists
- [ ] A Markdown file cannot introduce a new recipe;
      it can only reference recipes in `.mdsmith.yml`
      or built-in recipes
- [ ] `output` accepts any file extension; no
      extension filter is applied
- [ ] The rendered `body_template` uses `{alt}`
      defaulting to `"{recipe} output: {output}"`,
      satisfying MDS032
- [ ] A user-declared recipe's `body_template` from
      `build.recipes` is used instead of the default
- [ ] Merge driver regenerates `<?build?>` bodies on
      conflict (via `gensection.Engine`)
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
