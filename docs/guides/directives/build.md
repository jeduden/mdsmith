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
every `<?build?>` directive across the files it processed, decides which
targets are stale (see [Staleness and the build cache](#staleness-and-the-build-cache)),
rebuilds only those, and prints one `OK`, `SKIP`, or `FAIL` line per
target:

```text
build docs/architecture.md:12 (render): OK
build docs/guide.md:8 (render): SKIP
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

Two directives may not declare overlapping outputs. An exact path
collision (`a.txt` and `a.txt`) or a directory-prefix collision (`book/`
and `book/index.html`) is a build error that names both source locations
and runs neither recipe, so a build never races two writers to the same
path.

### `mdsmith fix` build flags

| Flag                  | Behavior                                                           |
| --------------------- | ------------------------------------------------------------------ |
| (none)                | Lint-fix pass, then build only stale targets                       |
| `--no-build`          | Lint-fix pass only                                                 |
| `--build-only`        | Build pass only                                                    |
| `--build-recipe NAME` | Build only directives whose `recipe:` is `NAME`                    |
| `--build-dry-run`     | Print each target's `STALE` or `FRESH` verdict; run no recipe      |
| `--build-force`       | Rebuild every target; refresh all cache entries                    |
| `--build-check-stale` | Print stale targets, exit non-zero if any are stale; run no recipe |
| `--build-no-cache`    | Treat all targets as stale; do not read or write the cache         |
| `--build-timeout DUR` | Per-recipe timeout (default `30s`)                                 |

`--no-build` and `--build-only` are mutually exclusive.
`--build-force` cannot be combined with `--build-check-stale` or
`--build-no-cache`.

`--build-check-stale` makes artifact freshness a CI signal: it runs no
recipe and exits non-zero when any declared output is out of date, so a
build step can fail a pull request that forgot to regenerate.

## Staleness and the build cache

The build pass is incremental. By default `mdsmith fix` rebuilds only the
targets whose recipe spec, inputs, or outputs changed; a fresh target
prints `SKIP` and its recipe never runs. Three states appear in the
per-target summary:

| State  | Meaning                                          |
| ------ | ------------------------------------------------ |
| `OK`   | The target was stale and its recipe rebuilt it   |
| `SKIP` | The target was fresh; its recipe was skipped     |
| `FAIL` | The recipe failed, or an input could not resolve |

### How freshness is decided

For each target mdsmith computes one ActionID: a sha256 over the recipe
`command`, the directive's params, the sorted relative input paths, the
sha256 of each input's content, the sorted relative output paths, and the
cache schema version. Every field is length-framed, so an input path
containing a NUL byte or a sentinel character can never collide with a
different input set.

A target is **fresh** only when all of the following hold:

1. Every declared `inputs:` entry resolves (a missing non-glob input is a
   build error; a glob matching zero files is a build error).
2. Every declared output exists on disk.
3. The cached ActionID for the target's output set equals the freshly
   computed ActionID.
4. Each output's content hash equals the hash recorded in the cache —
   so hand-editing or externally regenerating an artifact triggers a
   rebuild on the next `fix`.

Otherwise the target is stale and its recipe runs. Content hashing, not
mtime, decides freshness: a `git checkout` rarely preserves mtimes, but
file contents are stable.

### The cache file

mdsmith stores build state at `.mdsmith/build-cache.json`. It carries a
schema `version` and one entry per target:

```json
{
  "version": 1,
  "entries": [
    {
      "outputs": [{"path": "assets/diagram.png", "hash": "sha256-…"}],
      "inputs": ["diagram.svg"],
      "action-id": "sha256-…",
      "recipe": "render",
      "built-at": "2026-06-11T12:00:00Z"
    }
  ]
}
```

All paths are stored relative to the project root, so the cache is stable
across clone locations. Cache writes are atomic — a temp file plus a
rename — so a mid-build crash leaves the previous cache readable. A
target is keyed by its sorted set of output paths; the `action-id`,
`recipe`, and `built-at` fields are advisory metadata.

The build cache and the build working directories are machine-local
state. Ignore them in Git, but never ignore the whole `.mdsmith/` folder
— its `kinds/`, `schemas/`, and `conventions/` subfolders are
checked-in config:

```text
.mdsmith/build-cache.json
.mdsmith/build-logs/
.mdsmith/build-staging/
```

### Recipe default inputs

A recipe may declare implicit inputs in `default-inputs`. Each entry is a
literal relative path or a `{param}` token naming one of the recipe's
declared params:

```yaml
build:
  recipes:
    vhs:
      command: "vhs {tape}"
      params:
        required: [tape]
      default-inputs: ["{tape}"]
```

A directive supplying `tape: demo.tape` then has its effective input set
computed as `{ demo.tape } ∪` the directive's own `inputs:`, so authors
never restate the recipe's own source file. The token expands to the
root-joined absolute path at exec time, but the value folded into the
ActionID is always the relative path the param supplies (`demo.tape`).

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
