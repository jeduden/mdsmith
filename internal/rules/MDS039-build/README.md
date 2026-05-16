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
---
# MDS039: build

Validate `<?build?>` directive parameters and keep the section body
in sync with the recipe's rendered `body-template`.

## What it detects

MDS039 validates each `<?build?>` directive in a Markdown file:

1. **`recipe` resolves** — the recipe name must be declared in
   `build.recipes` in `.mdsmith.yml`.
2. **`output` is present and safe** — `output` is required, must be a
   relative path, and must not contain `..` path components.
3. **Required params present** — params listed as required by the
   recipe schema must all be supplied.
4. **No unknown params** (warning) — params not in the recipe's
   `required` or `optional` lists produce a warning.
5. **Body in sync** — the section body must equal the rendered
   `body-template`; MDS039 reports `generated section is out of date`
   when it diverges.

`mdsmith fix` rewrites the body using the rendered `body-template`.
No external tool is executed.

## Directive syntax

```text
<?build
recipe: RECIPE-NAME
output: path/to/artifact.ext
[recipe-specific params]
?>
RENDERED BODY
<?/build?>
```

### Common parameters

| Name     | Required | Description                                                     |
|----------|----------|-----------------------------------------------------------------|
| `recipe` | yes      | Recipe name declared in `build.recipes`                         |
| `output` | yes      | Artifact path relative to the Markdown file; no `..` components |

`output` accepts any file extension; MDS039 applies no extension
filter.

## Generated body

Each recipe has a `body-template` rendered by `mdsmith fix`. Two
placeholders are available:

| Placeholder | Value                                   |
|-------------|-----------------------------------------|
| `{output}`  | The `output` param value                |
| `{alt}`     | `"{recipe} output: {output}"` (default) |

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
output: docs/diagram.png
?>
![render output: docs/diagram.png](docs/diagram.png)
<?/build?>
```

### Bad — stale body

```markdown
<?build
recipe: render
source: diagram.svg
output: docs/diagram.png
?>
outdated content
<?/build?>
```

MDS039 reports: `generated section is out of date`

### Bad — unknown recipe

```markdown
<?build
recipe: nonexistent
output: out.png
?>
content
<?/build?>
```

MDS039 reports: `build directive references unknown recipe "nonexistent"`

### Bad — missing required param

```markdown
<?build
recipe: render
output: out.png
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
output: demo.gif
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
