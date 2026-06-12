---
id: MDS039
name: build
status: ready
description: >-
  Validate `<?build?>` directive parameters and keep the section body
  in sync with the recipe's rendered `body-template`.
category: directive
nature: directive
maintainability: null
markdownlint: []
rumdl: []
mado: []
panache: []
obsidian-linter: []
gomarklint: []
---
# MDS039: build

Validate `<?build?>` directive parameters and keep the section body
in sync with the recipe's rendered `body-template`.

## What it detects

MDS039 validates each `<?build?>` directive in a Markdown file:

1. **`recipe` resolves** — the recipe name must be declared in
   `build.recipes` in `.mdsmith.yml`.
2. **`outputs` is present and safe** — `outputs` is a required,
   non-empty list. Each entry must be a relative path with
   forward-slash separators, no `..` components after `path.Clean`, no
   Windows drive letter or UNC prefix, no reserved device name, and no
   glob characters. Entries under `.mdsmith/` are rejected.
3. **`inputs` is well-formed** — `inputs` is an optional list of paths
   or doublestar globs. Each entry passes the same path-shape rules as
   `outputs`, except glob characters are allowed.
4. **Required params present** — params listed as required by the
   recipe schema must all be supplied.
5. **No unknown params** (warning) — params not in the recipe's
   `required` or `optional` lists produce a warning. The old singular
   `output:` is no longer known and draws this warning.
6. **Body in sync** — the section body must equal the rendered
   `body-template`; MDS039 reports `generated section is out of date`
   when it diverges.

`mdsmith fix` rewrites the body using the rendered `body-template`.
No external tool is executed.

## Directive syntax

```text
<?build
recipe: RECIPE-NAME
inputs:
  - path/to/source.md
outputs:
  - path/to/artifact.ext
[recipe-specific params]
?>
RENDERED BODY
<?/build?>
```

### Common parameters

| Name      | Required | Description                                         |
| --------- | -------- | --------------------------------------------------- |
| `recipe`  | yes      | Recipe name declared in `build.recipes`             |
| `outputs` | yes      | Non-empty list of relative artifact paths; no globs |
| `inputs`  | no       | List of relative source paths or doublestar globs   |

`outputs` entries accept any file extension; MDS039 applies no
extension filter.

## Generated body

Each recipe has a `body-template`. `mdsmith fix` renders it once per
`outputs` entry, in declared order, and joins the lines with newlines.
Two placeholders are available per render iteration:

| Placeholder | Value                                        |
| ----------- | -------------------------------------------- |
| `{output}`  | The current `outputs` entry                  |
| `{alt}`     | `"{recipe} output: {output}"` for that entry |

When `body-template` is omitted from the recipe declaration, the
default `[{output}]({output})` is used.

## Config

```yaml
build:
  recipes:
    render:
      body-template: "![{alt}]({output})"
      params:
        required: [source]
        optional: [title]
```

To disable MDS039:

```yaml
rules:
  build: false
```

## Examples

### Good

```markdown
<?build
recipe: render
source: diagram.svg
outputs:
  - docs/diagram.png
?>
![render output: docs/diagram.png](docs/diagram.png)
<?/build?>
```

### Bad — stale body

```markdown
<?build
recipe: render
source: diagram.svg
outputs:
  - docs/diagram.png
?>
outdated content
<?/build?>
```

MDS039 reports: `generated section is out of date`

### Bad — unknown recipe

```markdown
<?build
recipe: nonexistent
outputs:
  - out.png
?>
content
<?/build?>
```

MDS039 reports: `build directive references unknown recipe "nonexistent"`

### Bad — missing required param

```markdown
<?build
recipe: render
outputs:
  - out.png
?>
content
<?/build?>
```

MDS039 reports:
`build directive recipe "render": missing required parameter "source"`

## Pattern

The bad pattern is a hand-maintained snippet
describing where a generated artifact lives. The
good pattern is the same content produced by a
`<?build?>` directive. The canonical source
files live in [pattern/bad/](pattern/bad/) and
[pattern/good/](pattern/good/); the snippets
below mirror those files for quick reference.
The markdown-audit skill reads the folders
directly.

### Without the directive

````markdown
# Demo

The recorded demo lives at `demo.gif`. Re-record
the GIF with:

```sh
vhs demo.tape
```

Embedded inline:

![demo](demo.gif)
````

### With the directive

```markdown
# Demo

<?build
recipe: vhs
source: demo.tape
outputs:
  - demo.gif
?>
![demo](demo.gif)
<?/build?>
```

## Meta-Information

- **ID**: MDS039
- **Name**: `build`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes (body only)
- **Implementation**:
  [source](./)
- **Category**: directive
