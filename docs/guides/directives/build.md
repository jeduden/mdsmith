---
title: Build directive
summary: >-
  How to use the build directive to declare artifact outputs, keep
  generated bodies in sync, and configure user-declared recipes.
---
# Build directive

The `<?build?>` directive declares a build artifact — a file
produced by a recipe configured in `build.recipes`. `mdsmith fix`
renders the section body from the recipe's `body-template` and
keeps it up to date. No external tool runs at lint time.

## Syntax

```text
<?build
recipe: RECIPE-NAME
output: path/to/artifact.ext
[recipe-specific params]
?>
RENDERED BODY
<?/build?>
```

The directive uses the same block form as `<?catalog?>` and
`<?include?>`. Inline form is not supported.

### Common parameters

| Name     | Required | Description                                                   |
| -------- | -------- | ------------------------------------------------------------- |
| `recipe` | yes      | Recipe name declared in `build.recipes`                       |
| `output` | yes      | Relative artifact path; no `..` components; no absolute paths |

`output` accepts any file extension; the rule applies no extension
filter.

## Declaring recipes

All recipes must be declared in `build.recipes` in `.mdsmith.yml`.
A `<?build?>` directive can only reference recipes declared there;
it cannot introduce a new recipe inline.

```yaml
build:
  recipes:
    render:
      command: "myrenderer {source} -o {output}"
      body-template: "![{alt}]({output})"
      params:
        required: [source]
        optional: [title, output]
    vhs:
      command: "vhs {input}"
      body-template: "![{alt}]({output})"
      params:
        required: [input]
```

Then in a Markdown file:

```text
<?build
recipe: render
source: diagram.svg
output: docs/diagram.png
?>
![render output: docs/diagram.png](docs/diagram.png)
<?/build?>
```

## Generated body

`mdsmith fix` renders the section body from the recipe's
`body-template`. Two placeholders are available:

| Placeholder | Value                                   |
| ----------- | --------------------------------------- |
| `{output}`  | The `output` param value                |
| `{alt}`     | `"{recipe} output: {output}"` (default) |

When `body-template` is omitted from the recipe declaration, the
default `[{output}]({output})` is used.

## Rule MDS039

MDS039 validates `<?build?>` directives and reports:

- **Error** when `recipe` is missing or not declared in `build.recipes`
- **Error** when `output` is missing, is an absolute path, or contains `..` components
- **Error** when a required param for the recipe is absent
- **Warning** when a param is not in the recipe's `required` or
  `optional` lists
- **Error** (`generated section is out of date`) when the body
  diverges from the rendered `body-template`

Run `mdsmith fix <file>` to regenerate stale bodies.

## Interaction with other rules

- **MDS027**: a missing artifact file fires MDS027 independently;
  MDS039 does not duplicate it.
- **MDS040**: validates `build.recipes` command safety at lint time;
  MDS039 validates `<?build?>` directive usage in Markdown files.
- **merge-driver**: regenerates `<?build?>` bodies on conflict
  via `gensection.Engine`; artifact bytes are not regenerated.
