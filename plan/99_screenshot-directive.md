---
id: 99
title: Build directive — declared artifact system
status: "🔲"
summary: >-
  A `<?build recipe:...?>` directive turns any
  tool-generated file into a declared artifact
  of its source. `mdsmith fix` keeps the body
  in sync; `mdsmith build` runs the recipe to
  regenerate the output. Built-in recipes cover
  `screenshot` and `vhs`; users add their own
  with a command template and param schema.
---
# Build directive — declared artifact system

## Goal

Give documentation build artifacts the same first-class
status that `<?catalog?>` and `<?include?>` give to
generated text. Authors declare the recipe, inputs, and
output path. `mdsmith build` runs the recipe when sources
change; `mdsmith fix` keeps the embedded link in sync.

## Context

Browser screenshots and `demo.gif` (from VHS) are both
manual today. `mdsmith build` gives them the same
declare-once model as `<?catalog?>` and `<?include?>`.

Lint and build stay separate. MDS039 validates params
and rewrites the body link; no external tools run.
`mdsmith build` dispatches to the recipe and is never
a side effect of `check`/`fix`.

## Design

### Directive syntax

```text
<?build
recipe: screenshot
url: /inbox
output: docs/inbox.png
?>
![Inbox screenshot](docs/inbox.png)
<?/build?>
```

```text
<?build
recipe: vhs
input: demo.tape
output: demo.gif
?>
![Terminal demo](demo.gif)
<?/build?>
```

Common parameters (all recipes):

| Name     | Required | Description                                      |
|----------|----------|--------------------------------------------------|
| `recipe` | yes      | Registered recipe name                           |
| `output` | yes      | Artifact path relative to Markdown file; no `..` |

`output` accepts any extension (PNG, GIF, SVG, …).
Recipe-specific parameters are defined per recipe.

### Generated body (lint layer)

Each recipe declares a `body_template` rendered by
`mdsmith fix`. Built-in defaults:

| Recipe       | Default `body_template` |
|--------------|-------------------------|
| `screenshot` | `![{alt}]({output})`    |
| `vhs`        | `![{alt}]({output})`    |
| custom       | `[{output}]({output})`  |

`alt` defaults to `"{recipe} output: {output}"` so
the body always passes MDS032. MDS039 reports
stale-section when the body diverges; `Fix` rewrites
it using `gensection.Engine`.

### Built-in recipes

**`screenshot`** — captures a URL via
[chromedp][chromedp]:

| Param      | Required | Default    |
|------------|----------|------------|
| `url`      | yes      | —          |
| `selector` | no       | full page  |
| `viewport` | no       | `1280x800` |
| `wait`     | no       | `0` ms     |
| `click`    | no       | —          |
| `hide`     | no       | `[]`       |

**`vhs`** — renders a `.tape` file:

| Param   | Required |
|---------|----------|
| `input` | yes      |

[chromedp]: https://github.com/chromedp/chromedp
[vhs]: https://github.com/charmbracelet/vhs

### User-defined recipes

Recipes are declared in `.mdsmith.yml`. Any
output type is allowed:

```yaml
build:
  recipes:
    mermaid:
      command: "mmdc -i {input} -o {output}"
      body_template: "![{alt}]({output})"
      params:
        required: [input]
        optional: [theme]
    api-spec:
      command: "redocly bundle {input} -o {output}"
      body_template: "[API spec]({output})"
      params:
        required: [input]
```

`{param}` tokens expand to directive parameter
values at build time. MDS039 validates that
`required` params are present.

### Security

Custom recipe commands are **never** passed to a shell.
At config load `command` is split on whitespace into
an argv list; `{param}` tokens become individual
arguments passed to `exec.Cmd` directly — no `sh -c`,
no metacharacter expansion. A value of `foo; rm -rf /`
is passed literally to the binary, not interpreted.

MDS040 (see below) enforces this at lint time by
rejecting `command` strings that would require a shell.
Path params (`output`, `input`) are validated by MDS039
as relative paths with no `..` components.

### Rule: MDS039 (build)

- ID: `MDS039`
- Name: `build`
- Category: `meta`
- Default: enabled
- Fixable: yes (lint layer only)

Validation:

- `recipe` is present and resolves to a known
  recipe (built-in or declared in config)
- `output` is a relative path inside the
  project root, no `..`
- Recipe-specific required params are present

### Rule: MDS040 (recipe-safety)

- ID: `MDS040`
- Name: `recipe-safety`
- Category: `meta`
- Default: enabled
- Fixable: no
- Target: `.mdsmith.yml` (config file)

Validates each user-defined recipe `command`:

- Non-empty
- First token is not a shell interpreter
  (`sh`, `bash`, `zsh`, `ksh`, `fish`,
  `/bin/sh`, `/bin/bash`, …)
- Static tokens contain no shell operators:
  `&&` `||` `;` `|` `>` `<` `>>` `2>` `` ` `` `$(` `${`
- No adjacent fused `{param}` tokens (e.g. `{a}{b}`)
- Executable token contains no `..` components

Example diagnostics:

```text
.mdsmith.yml:5:5: recipe "mermaid": command uses shell
interpreter "bash" — use the direct binary (MDS040)

.mdsmith.yml:9:5: recipe "audit": command contains
shell operator "&&" — use a wrapper script (MDS040)
```

### `mdsmith build` subcommand

```text
mdsmith build [paths...] [flags]
```

Flags: `--recipe NAME`, `--base-url URL`,
`--dry-run`, `--timeout DURATION` (default `30s`).

Walks files, collects `<?build?>` blocks, dispatches
to recipe drivers in order, writes artifacts atomically.
Exits non-zero on failure with a per-file `OK | FAIL`
summary.

### Configuration

```yaml
build:
  base-url: ""    # joined to path-only URLs
  recipes: {}     # user-defined recipe declarations
```

Per-directive params override config defaults.

### Interaction with existing rules

- **MDS032**: derived `alt` is non-empty.
- **MDS027**: missing artifact fires MDS027.
- **`merge-driver`**: regenerates the body; artifact
  bytes are not regenerated.

## Tasks

1. Define `Builder` interface and recipe registry in
   `internal/build/`. Built-ins: `screenshot` (chromedp),
   `vhs` (exec). Custom-recipe impl tokenises `command`
   at config load into argv; runs via `os/exec` — no
   shell.
2. Add MDS040 (`recipe-safety`) in
   `internal/rules/MDS040-recipe-safety/`: validates
   each recipe `command` (no shell interpreter, no shell
   operators, no fused placeholders, no `..`). Not
   fixable. Add `good/` and `bad/` fixtures. Wire into
   `cmd/mdsmith/main.go`.
3. Create `<?build?>` directive in `internal/rules/build/`
   via `gensection.Engine`. Register as MDS039, category
   `meta`. `Generate` renders the body template only;
   never calls a builder.
4. Add MDS039 fixtures `good/`, `bad/`, `fixed/` under
   `internal/rules/MDS039-build/`.
5. Wire MDS039 into `cmd/mdsmith/main.go`.
6. Add `mdsmith build` subcommand with all flags.
7. Add `build:` config block in `internal/config/`.
   Surface recipe registry to MDS039 and the build
   command.
8. Integration tests: `screenshot` against
   `httptest.Server` writes a non-empty PNG; a
   `cp`-based custom recipe writes a file. Both skip
   when the required binary is absent.
9. Document MDS039 and MDS040 READMEs; user guide at
   `docs/guides/directives/build.md`.
10. Demo using a static HTML file (no dev server).

## Acceptance Criteria

- [ ] `<?build recipe:screenshot url:... output:...?>`
      regenerates its body on `mdsmith fix`
- [ ] `<?build recipe:vhs input:demo.tape output:demo.gif?>`
      regenerates its body on `mdsmith fix`
- [ ] MDS039 reports stale-section when the body
      diverges from the rendered `body_template`
- [ ] MDS039 rejects unknown recipe, missing
      `output`, path traversal in `output` or
      path params, and missing required params
- [ ] A user-defined recipe with a `cp` command
      produces its output file
- [ ] Custom recipe command is executed via
      `os/exec` with explicit argv — no shell
      interpreter involved
- [ ] MDS040 flags a recipe whose `command`
      starts with a shell interpreter
- [ ] MDS040 flags a recipe whose `command`
      contains a shell operator token (`&&`,
      `||`, `;`, `|`, `>`, etc.)
- [ ] MDS040 flags fused adjacent `{param}`
      placeholders (e.g. `{a}{b}`)
- [ ] MDS040 passes a recipe with a clean
      command like `mmdc -i {input} -o {output}`
- [ ] A Markdown file cannot introduce a new
      recipe command; it can only reference
      recipes declared in `.mdsmith.yml`
- [ ] `output` can be any file extension; no
      extension filter is applied by MDS039
- [ ] `mdsmith build` against `httptest.Server`
      writes a non-empty PNG
- [ ] `mdsmith build --dry-run` lists every
      target without running any tool
- [ ] `mdsmith build` exits non-zero on failure
      with a per-file `OK | FAIL` summary
- [ ] `mdsmith check` does **not** run any
      external tool for `<?build?>` blocks
- [ ] `build:` config defaults apply to
      directives and are overridden per-directive
- [ ] CI without chromium still passes; build
      tests are skipped, not failed
- [ ] Merge driver regenerates `<?build?>`
      bodies on conflict (gensection engine)
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports
      no issues
