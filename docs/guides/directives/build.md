---
title: Build directive
summary: >-
  How to use the build directive to declare artifact outputs and
  source inputs, keep generated bodies in sync, and configure
  user-declared recipes.
---
# Build directive

The `<?build?>` directive declares one or more build artifacts —
files produced by a recipe configured in `build.recipes` — and the
source inputs they are built from. `mdsmith fix` renders the section
body from the recipe's `body-template` and keeps it up to date. No
external tool runs at lint time.

## Syntax

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

The directive uses the same block form as `<?catalog?>` and
`<?include?>`. Inline form is not supported.

### Common parameters

| Name      | Required | Description                                         |
| --------- | -------- | --------------------------------------------------- |
| `recipe`  | yes      | Recipe name declared in `build.recipes`             |
| `outputs` | yes      | Non-empty list of relative artifact paths; no globs |
| `inputs`  | no       | List of relative source paths or doublestar globs   |

`outputs` entries accept any file extension; the rule applies no
extension filter.

### Path-shape rules

Every `outputs:` and `inputs:` entry must be a relative path with
forward-slash separators. The rule rejects an entry that has a NUL
byte, a newline, leading or trailing whitespace, a backslash, a
Windows drive letter (`C:`), a UNC prefix (`\\?\`), an NTFS alternate
data stream (`foo:bar`), a reserved device name (`CON`, `PRN`, `NUL`,
`COM1`–`COM9`, `LPT1`–`LPT9`), an absolute path, a `~` prefix, a `..`
component after `path.Clean`, or a path under `.mdsmith/`. An empty
list, or an empty or whitespace-only entry inside either list, is a
diagnostic.

`outputs:` entries are literal paths — glob characters (`*`, `?`,
`[`) are rejected. `inputs:` entries accept the full doublestar glob
syntax documented in [Glob patterns](../../reference/globs.md),
including a leading `**/`. An `inputs:` glob that resolves to more
than 10 000 files is a build error; split it into narrower patterns.

## Declaring recipes

All recipes must be declared in `build.recipes` in `.mdsmith.yml`.
A `<?build?>` directive can only reference recipes declared there;
it cannot introduce a new recipe inline.

```yaml
build:
  recipes:
    render:
      command: "myrenderer {source} -o {outputs}"
      body-template: "![{alt}]({output})"
      params:
        required: [source]
        optional: [title]
    pandoc:
      command: "pandoc {inputs} -o {outputs}"
      body-template: "[{output}]({output})"
      params:
        required: [from]
```

Then in a Markdown file:

```text
<?build
recipe: render
source: diagram.svg
outputs:
  - docs/diagram.png
?>
![render output: docs/diagram.png](docs/diagram.png)
<?/build?>
```

### Recipe command placeholders

A recipe `command` references the directive's params with `{param}`
tokens. Two collective placeholders carry the directive's lists:

| Placeholder | Expansion                               |
| ----------- | --------------------------------------- |
| `{outputs}` | One argv per directive `outputs:` entry |
| `{inputs}`  | One argv per resolved `inputs:` entry   |

`outputs` and `inputs` are reserved param names: a recipe must not
declare either in `params.required` or `params.optional`, and MDS040
reports it if it does. Each placeholder must stand alone as its own
argv token after whitespace splitting — embedded use like
`-o{outputs}` is a `command` validation error, because expanding a
list inside a token fragment has no well-defined meaning.

## Generated body

`mdsmith fix` renders the section body from the recipe's
`body-template`, once per `outputs:` entry, in declared order, and
joins the rendered lines with newlines. Two placeholders are
available per render iteration:

| Placeholder | Value                                        |
| ----------- | -------------------------------------------- |
| `{output}`  | The current `outputs:` entry                 |
| `{alt}`     | `"{recipe} output: {output}"` for that entry |

When `body-template` is omitted from the recipe declaration, the
default `[{output}]({output})` is used. Any change to `outputs:`
makes the rendered body diverge, so MDS039 reports `generated
section is out of date` until you run `mdsmith fix`.

## Rule MDS039

MDS039 validates `<?build?>` directives and reports:

- **Error** when `recipe` is missing or not declared in `build.recipes`
- **Error** when `outputs:` is missing or empty, or any `outputs:` or
  `inputs:` entry fails the path-shape rules above
- **Error** when a required param for the recipe is absent
- **Warning** when a param is not in the recipe's `required` or
  `optional` lists — the removed singular `output:` draws this warning
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
