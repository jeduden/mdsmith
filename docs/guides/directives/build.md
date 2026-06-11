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
body from the recipe's `body-template` and keeps it up to date, then
runs a build pass that executes each recipe and writes its declared
outputs. `mdsmith check` is read-only: it validates the directive and
the body but never runs a recipe.

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

## Running the build

`mdsmith fix` runs a build pass after the lint-fix pass. It collects
every `<?build?>` directive across the files it processed, dispatches
each to its recipe, and prints one `OK` or `FAIL` line per target:

```text
build docs/architecture.md:12 (render): OK
build docs/overview.md:30 (pandoc): FAIL: recipe "pandoc" failed: exit status 1
```

`mdsmith fix` exits non-zero if any recipe fails. A failing recipe
leaves no partial output: each target stages its outputs in a
per-target temp directory, and mdsmith renames the staged files into
place only after the recipe succeeds. A pre-existing output survives
a failed rebuild untouched.

The build pass runs *after* the lint-fix pass, so a freshly-edited
`outputs:` list is built with its new value. The pass runs only from
the `mdsmith fix` CLI: it is not part of the public engine API, the
WebAssembly bindings, the LSP fix-on-save path, or the Git
merge-driver, none of which ever execute a process.

### Recipe dispatch

A recipe `command` is dispatched via `os/exec` with an explicit argv.
No shell is invoked, so a `;`, `|`, or `$(…)` inside a param value is
passed through as one literal argument and never interpreted. The
command string is tokenized once with whitespace splitting; `{param}`,
`{inputs}`, and `{outputs}` are substituted *after* tokenization, so a
param value containing whitespace stays a single argv entry.

`inputs:` globs resolve against the project root with the doublestar
matcher. A resolved input that escapes the project root (for example
through a symlink), or one glob that matches more than 10 000 files,
is a build error.

### `mdsmith fix` build flags

| Flag                  | Behavior                                        |
| --------------------- | ----------------------------------------------- |
| (none)                | Lint-fix pass, then build pass                  |
| `--no-build`          | Lint-fix pass only                              |
| `--build-only`        | Build pass only                                 |
| `--build-recipe NAME` | Build only directives whose `recipe:` is `NAME` |
| `--build-dry-run`     | Enumerate targets; run no recipe                |
| `--build-timeout DUR` | Per-recipe timeout (default `30s`)              |

`--no-build` and `--build-only` are mutually exclusive.

### Markdown as data

A recipe can pipe a Markdown file's structure into a downstream tool.
This recipe runs `mdsmith extract` on an input file and feeds the
JSON into a chart generator:

```yaml
build:
  recipes:
    chart:
      command: chart-tool --from {inputs} --out {outputs}
```

```text
<?build
recipe: chart
inputs:
  - data/metrics.md
outputs:
  - assets/metrics.svg
?>
![chart output: assets/metrics.svg](assets/metrics.svg)
<?/build?>
```

Here `chart-tool` is your own program; supply one that reads the
extracted data and writes the chart. mdsmith only dispatches the
recipe and writes its declared output.

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
