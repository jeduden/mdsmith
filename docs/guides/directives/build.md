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
every `<?build?>` directive across the files it processed. It then
decides which targets are stale (see
[Staleness and the build cache](#staleness-and-the-build-cache)) and
rebuilds only those. It prints one `OK`, `SKIP`, or `FAIL` line per
target, named by the target's first declared output:

```text
OK book.html
SKIP guide.html
```

`mdsmith fix` exits non-zero if any recipe fails. A failing recipe
leaves no partial output: each target stages its outputs in a
random-suffixed per-recipe directory under `.mdsmith/build-staging/`,
and mdsmith renames the staged files into place only after the recipe
succeeds and the [output post-conditions](#output-post-conditions)
pass. A pre-existing output survives a failed rebuild untouched.

The build pass treats `.mdsmith.yml` and every `<?build?>` directive
as untrusted. Before it runs any recipe it consults the
[trust gate](#the-trust-gate); each recipe then runs under a
[hermetic environment](#hermetic-execution-environment) in its own
process group, and its writes are checked against the declared
outputs. See [Build safety](#build-safety) for the full model.

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

| Flag                            | Behavior                                                          |
| ------------------------------- | ----------------------------------------------------------------- |
| (none)                          | Lint-fix pass, then build only stale targets                      |
| `--no-build`                    | Lint-fix pass only; skips the build pass, including hooks         |
| `--build-only`                  | Build pass only                                                   |
| `--build-recipe NAME`           | Build only directives whose `recipe:` is `NAME`; hooks still run  |
| `--build-dry-run`               | Print each target's `STALE` or `FRESH` verdict; run no recipe     |
| `--build-force`                 | Rebuild every target; refresh all cache entries                   |
| `--build-check-stale`           | Print stale targets, exit non-zero if any stale; run no recipe    |
| `--build-no-cache`              | Treat all targets as stale; do not read or write the cache        |
| `--build-timeout DUR`           | Per-recipe timeout (default `30s`); fires a process-group kill    |
| `--build-no-hooks`              | Run the build pass but skip both `before` and `after` hook lists  |
| `--build-skip-hooks-when-fresh` | Skip both hook lists when no target is stale; run them otherwise  |
| `--build-stream`                | Live-forward each recipe's stdout/stderr, prefixed by target name |
| `--build-explain TARGET`        | Print `TARGET`'s ActionID inputs and cache verdict; run no recipe |
| `--build-verify`                | Run each recipe twice and warn when the two outputs differ        |
| `--build-jobs N`                | Run up to `N` recipes concurrently (default `1`)                  |

`--no-build` and `--build-only` are mutually exclusive. `--build-force`
excludes `--build-check-stale` and `--build-no-cache`. `--build-explain`
and `--build-verify` conflict with each other and with `--build-dry-run`
or `--build-check-stale`. `--build-jobs N` requires N ≥ 1.

`--build-check-stale` makes artifact freshness a CI signal: it runs no
recipe and exits non-zero when any declared output is out of date, so a
build step can fail a pull request that forgot to regenerate.

Each recipe's stdout and stderr are captured to
`.mdsmith/build-logs/<action-id>.log`. A failure prints source, argv,
exit, duration, log, and the last 20 stderr lines; orphan logs are
removed at the next fix pass. `--build-stream` forwards output to the
terminal line by line. `--build-explain TARGET` prints a target's
ActionID inputs and cache verdict without running the recipe; no match
exits non-zero. `--build-verify` runs each recipe twice and warns on
output mismatch, marking the cache entry `unstable`. `--build-jobs N`
runs up to `N` recipes concurrently (default `1`); disjoint outputs
make all pairs safe to parallelize.

## Build lifecycle hooks

`build.hooks.before` and `build.hooks.after` declare commands to run
once per `mdsmith fix` build pass — `before` hooks run before any
recipe, `after` hooks run after the recipe pass. Use them to start a
dev server before screenshot recipes and stop it after.

### Configuration

```yaml
build:
  hooks:
    before:
      - command: "make dev-server-start"
        name: "start dev server"
      - command: "scripts/wait-for-port {port}"
        params:
          port: "3000"
    after:
      - command: "make dev-server-stop"
        name: "stop dev server"
  recipes:
    screenshot:
      command: "capture-tool {url} {outputs}"
      params:
        required: [url]
```

Each hook entry has three fields:

| Field     | Required | Description                                                      |
| --------- | -------- | ---------------------------------------------------------------- |
| `command` | yes      | Argv template — same `{param}` rules as recipes                  |
| `params`  | no       | Map of param name to literal string value                        |
| `name`    | no       | Display label for `OK`/`FAIL` output; defaults to the executable |

Hooks have no directive surface. They are config-level and run once
per `mdsmith fix` build pass, not once per directive.

### Execution order

```text
1. Lint-fix pass (existing behavior)
2. before[0], before[1], … (in declaration order)
3. Recipe pass
4. after[0], after[1], … (in declaration order)
```

### Failure semantics

| Failing step  | Result                                                                |
| ------------- | --------------------------------------------------------------------- |
| `before` hook | Abort with the hook's exit code; recipes and `after` hooks do not run |
| Recipe        | Finish the recipe pass, then run `after` hooks; exit non-zero         |
| `after` hook  | Report and continue remaining `after` hooks; exit non-zero            |

The exit code priority: lint-fix errors → `before`-fail → recipe-fail →
`after`-fail → 0. A failing `before` hook means setup is incomplete and
recipes would produce garbage; a failing `after` hook means teardown is
broken but artifacts are already written.

### Hook argv rules

Hook commands follow the same no-shell rules as recipes (MDS040): no
shell interpreter first, no shell operators, no fused `{param}`
placeholders, no `..` in the executable, and `{inputs}`/`{outputs}`
are forbidden since hooks have no directive context. Hook `params` must
not contain NUL, newline, CR, or leading/trailing whitespace (max 4 KB).

### When to use `--build-skip-hooks-when-fresh`

By default hooks run even when every target is fresh. Pass
`--build-skip-hooks-when-fresh` to skip both lists when nothing rebuilds.
`--build-no-hooks` skips hooks entirely; `--no-build` skips the build
pass and hooks together.

## Build safety

The build pass is the only part of mdsmith that runs an external
process. It treats `.mdsmith.yml` and the `<?build?>` directives as
untrusted input, so a freshly cloned repository cannot silently run a
recipe. Four layers cooperate: a trust gate gates execution, a
hermetic environment bounds what a recipe can read and how it is
killed, atomic-write hardening protects the project tree from a hostile
staging path, and output post-conditions reject any write outside the
declared `outputs:`.

These layers raise the cost of an accidental or hostile recipe; they
are **not** a sandbox. PATH allowlisting and the staging working
directory are for build determinism, not filesystem confinement: a
recipe can still write through an absolute path or `../`, and a write
into a directory that is not a declared output's parent is not
detected. Real confinement needs an OS sandbox (a container, `bwrap`,
or similar). Only run recipes you would run by hand.

### The trust gate

`mdsmith fix` always runs the lint-fix pass. The build pass — the part
that executes recipes — runs only when the config is trusted on this
clone:

- The gate is satisfied when a marker named after the loaded config
  (`.mdsmith.yml.trust` for the default `.mdsmith.yml`, or
  `<name>.trust` under `mdsmith fix -c <name>`) exists and its bytes
  are identical to that config.
- Any drift (an edit to the config after it was trusted), or a missing
  marker, makes the build pass abort with a `build not trusted` message
  and exit code 2. The lint-fix pass has already run, so formatting
  still happens.
- `mdsmith fix --no-build` skips the gate and the build pass together.
- `--build-dry-run` and `--build-check-stale` never consult the gate:
  they enumerate targets without running anything.

The marker is per-clone, not per-repository: list it in `.gitignore`
alongside the build cache and staging dir.

Run `mdsmith trust` to review and trust a config. It prints a unified
diff between the stored marker and the current `.mdsmith.yml`, prompts
for confirmation, and rewrites the marker (mode `0600`) on accept. Pass
`--yes` to skip the prompt. Re-run it whenever you change `build:`
settings.

#### CI guidance

CI runners are presumed sandboxed and opt in with an environment
variable instead of a committed marker:

```sh
MDSMITH_TRUST_BUILD=1 mdsmith fix .
```

When `MDSMITH_TRUST_BUILD` is set to an affirmative value (anything other
than `0`, `false`, `no`, or `off`) the gate is satisfied without a marker
file; setting it to a disabling value leaves the gate in force. The
variable is consumed by the gate only; it is **not** passed through to
recipes (see the hermetic environment below). Set it only on a runner you
control.

### Hermetic execution environment

Each recipe runs with a minimal, explicit environment rather than
inheriting the parent process's:

- `PATH` is `build.exec.path` (default `/usr/bin:/bin`). Nothing else
  is on the path unless you add its directory.
- Only the environment variables named in `build.exec.env-pass-through`
  are forwarded, each with its current value. The default list is
  `[HOME, LANG, LC_ALL]`. A name that is unset in the parent
  contributes no entry.
- The working directory (`Cmd.Dir`) is the per-recipe staging dir.

```yaml
build:
  exec:
    path: "/usr/bin:/bin:/opt/pandoc/bin"
    env-pass-through: [HOME, LANG, LC_ALL, SOURCE_DATE_EPOCH]
```

`env-pass-through` *replaces* the default list — it does not append.
Re-list the defaults you still want; the example above keeps all three
and adds `SOURCE_DATE_EPOCH` for reproducible builds. MDS040 rejects an
empty pass-through name or a name containing `=` (which would smuggle in
a value rather than forward a variable).

The default `build.exec.path` deliberately omits mdsmith's own install
directory. A recipe that invokes `mdsmith` (for example to run
`mdsmith extract`) must add the directory holding the binary to
`build.exec.path`.

Each recipe runs in its own process group (a new session via `Setpgid`
on Unix; `CREATE_NEW_PROCESS_GROUP` plus a Job Object on Windows). When
`--build-timeout` expires mdsmith signals the whole group — `SIGTERM`
on Unix, `CTRL_BREAK_EVENT` on Windows — waits five seconds, then force
-kills the group (`SIGKILL` / Job Object termination). A recipe that
spawns a background daemon cannot leave an orphan behind.

### Atomic-write hardening

The staging machinery refuses an unsafe staging root and writes each
output atomically:

1. `.mdsmith/build-staging/` is created `0700` if absent. If it exists
   it must be a real directory — a symlink or a non-directory is
   refused — and on Unix it must not be group- or world-writable, so a
   hostile co-tenant cannot plant a symlink at a predictable name.
2. The per-recipe staging dir is created with `os.MkdirTemp`, so its
   name carries a random suffix.
3. Each declared output maps to a file inside that staging dir.
4. Before each output is committed, mdsmith `Lstat`s the destination
   and refuses to replace an existing symlink. The replace itself is an
   atomic `rename(2)` (`MoveFileEx` on Windows) for same-volume paths.

A declared output under `.mdsmith/` is refused by the build pass
itself, not only by the MDS039 lint rule, so a recipe can never
overwrite mdsmith's own state (the build cache, the trust marker, or
the checked-in `kinds/`, `schemas/`, and `conventions/`) even if MDS039
is disabled in config. A recipe that stages its declared output as a
symlink or a directory rather than a regular file is also rejected
before the commit.

Multi-output commit is **not** transactional: if the N+1th rename fails
after N succeeded, mdsmith reports the partial state, removes the
staging dir, and exits with `FAIL`. Because no cache entry was written,
the next `mdsmith fix` reruns the whole recipe.

### Output post-conditions

After a recipe exits 0, two checks run before any file is committed:

- **Every declared output exists** in the staging dir. A recipe that
  exits 0 without producing a declared output is a build failure
  (`recipe exited 0 but did not produce X`).
- **No undeclared write** landed in the project tree. mdsmith snapshots
  the parent directories of the declared outputs before the recipe
  (file list, size, mtime, mode, and a sha256 of each file's content)
  and diffs them after. Any added, removed, or modified file outside
  the declared `outputs:` is a build failure that names the file.
  Content hashing catches an edit that preserves size and mtime; mode
  catches a `chmod`.

Two limits apply. The check covers only the *parent directories* of
declared outputs, so a write into an unrelated subtree is not seen.
Symlinks are snapshotted via `Lstat` metadata plus `os.Readlink` and
never followed. A snapshot scope above 2000 directory entries is a
build error naming the oversized directory — point the outputs at a
narrower directory.

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
.mdsmith.yml.trust
```

The `.mdsmith.yml.trust` marker is the per-clone build
[trust gate](#the-trust-gate); it must stay untracked so trust is a
decision each checkout makes for itself.

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
