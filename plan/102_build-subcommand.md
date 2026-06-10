---
id: 102
title: Multi-output `<?build?>` directive
status: "✅"
summary: >-
  Replace `output:` (singular) with `outputs:`
  (list) on the `<?build?>` directive. Add
  `inputs:` (list, validation only — staleness
  lands in plan 103). Render `body-template`
  once per output. Add `{outputs}` and
  `{inputs}` argv placeholders to recipe
  `command`. No backwards compatibility.
model: opus
depends-on: [100, 101]
---
# Multi-output `<?build?>` directive

## Goal

The `<?build?>` directive declares a list of
artifact paths in `outputs:` (was a single
string `output:` in plan 101) and a list of
input paths or globs in `inputs:`. The body
template renders once per output. Recipe
`command` strings reference `{outputs}` and
`{inputs}` to pass the lists as argv.

## Context

Builds on plan 101 (the `<?build?>` directive
and MDS039) and plan 100 (`build:` config
and MDS040). Plan 2606101546 wires targets through
a `Builder` into `mdsmith fix`; plan 103
layers staleness.

`output:` is gone — a clean break, no
deprecation, no migration. A directive with
only `output:` fails because `outputs:` is
required (see "MDS039 update"); the stray
`output:` draws the unknown-param warning.

## Design

### New directive shape

```text
<?build
recipe: pandoc
inputs:
  - chapters/intro.md
  - chapters/01-prologue.md
outputs:
  - book.html
  - book.epub
?>
- [book.html](book.html)
- [book.epub](book.epub)
<?/build?>
```

`outputs:` requires at least one entry. An
empty list is a diagnostic.

`inputs:` may be empty when the recipe is a
pure function of its config. Plan 103's
ActionID still covers recipe spec and
output paths.

Empty `inputs:` is wrong for remote-state
recipes: the cache never invalidates. Use
a synthetic input file the author touches,
or `--build-force` on a schedule.

Each entry in `outputs:` is a literal relative
path. No globs. Every output must be a path the
recipe will write. This keeps post-build
verification deterministic: every declared
output must exist after the recipe returns.

Each `inputs:` entry is a literal path or
a glob. Globs resolve at build time (plan
2606101546); plan 102 validates shape only.

### Body template — rendered once per output

The recipe's `body-template` is rendered once
per `outputs` entry, in declared order. The
rendered lines are joined with newlines and
stored as the section body.

| Placeholder | Value per render iteration                   |
| ----------- | -------------------------------------------- |
| `{output}`  | The current output path                      |
| `{alt}`     | `"{recipe} output: {output}"` for that entry |

Any change to `outputs:` makes the rendered
body diverge, and MDS039 reports `generated
section is out of date` — same guarantee as
plan 101.

### MDS039 update

MDS039 (plan 101) is changed to:

1. Reject `output:` — no longer a known
   param; the unknown-param diagnostic applies.
2. Require `outputs:` (list of strings,
   non-empty). Each entry is validated per
   "Path-shape rules" below.
3. Accept optional `inputs:` (list, may be
   empty): relative paths or globs. Shape
   only; resolution is plan 2606101546's job.
4. Render `body-template` once per `outputs`
   entry as described above.

### Path-shape rules

Every entry in `outputs:` and `inputs:`
(globs included) is validated against this
allowlist before MDS039 accepts it:

- Non-empty after trim. Empty or whitespace-
  only entries are a diagnostic.
- No NUL byte, no newline, no carriage
  return, no leading or trailing ASCII
  whitespace.
- Forward-slash separators only. Backslash,
  Windows drive letters (`C:`), UNC prefixes
  (`\\?\`), NTFS alternate data streams
  (`foo:bar`), and reserved device names
  (`CON`, `PRN`, `AUX`, `NUL`, `COM1`-`COM9`,
  `LPT1`-`LPT9`) are rejected on every
  platform.
- Relative path; absolute paths and `~`
  prefixes are rejected.
- After `path.Clean`, the result must not
  start with `..` and must not contain `..`
  segments.
- Entries under `.mdsmith/` are rejected —
  that directory is mdsmith state.
- For `outputs:`: no glob characters
  (`*`, `?`, `[`). Outputs are literal paths.
- For `inputs:`: full doublestar syntax per
  `docs/reference/globs.md`, including
  leading `**/`. Matches are bounded to the
  project root (entries are relative).

At build time (plan 2606101546) the resolved path
is re-checked against the project root.
Inputs (which must exist) use
`filepath.EvalSymlinks` to resolve the full
chain. Outputs may not exist yet: the check
walks the longest existing prefix with
`EvalSymlinks` and joins the remaining
segments with `filepath.Join`. The check
uses OS-native separators internally and
normalises back to forward slashes
(`filepath.ToSlash`) before comparison and
diagnostics, keeping the slash-only
invariant on Windows. A symlinked output
or input outside the project is a build
error.

### Glob match cap

An `inputs:` glob matching more than
10 000 files is a build error. The cap is
per entry, so an author who needs more
declares multiple narrower patterns.

### Recipe `command` placeholders

Recipe `command` strings (plan 100) keep
the `{param}` rules; two collective
placeholders are added:

| Placeholder | Expansion                               |
| ----------- | --------------------------------------- |
| `{outputs}` | One argv per directive `outputs:` entry |
| `{inputs}`  | One argv per resolved `inputs:` entry   |

`outputs` and `inputs` become reserved param
names. They may not appear in
`params.required` or `params.optional`. MDS040
checks this.

`{outputs}` and `{inputs}` must appear as
standalone argv tokens after
`strings.Fields` tokenization. Embedded use
(e.g. `-o{outputs}`) is a `command`
validation error reported by MDS040, since
list-expanding a fragment of a token has no
well-defined semantics.

`{outputs}` is the only token that receives
output paths: plan 2606101546 substitutes
the staging paths for it at exec time.
Named params are opaque strings, never
rewritten, and must not carry output
paths. The staging contract (plan
2606101548) covers only writes to the
substituted `{outputs}` paths. One
declared output expands to one argv
(`command: "tool -o {outputs}"`). Several
expand to one argv each (`command:
"magick convert in.svg {outputs}"`,
`outputs: [a.png, b.png]`).

The actual argv expansion happens in plan
2606101546. Plan 102's MDS040 update only
validates that the reserved names are not
declared as params.

### Dependency-graph edges

The link graph's build edge keys on a
`source:` param MDS039 never knew
(`internal/linkgraph/directives.go`). It
moves to one edge per `inputs:` entry,
globs handled like `<?catalog?>` globs, so
`mdsmith deps`, `--incoming`, and the LSP
call-hierarchy gain real build edges.

## Tasks

1. Update MDS039 in `internal/rules/build/`:

  - Drop `output` from the known-param set.
  - Add `outputs` as required (list of
    strings, non-empty). Each entry validated
    by the path-shape rules above.
  - Add `inputs` as optional (list of strings).
    Each entry validated by the path-shape
    rules; full doublestar globs accepted.
  - Enforce the 10 000-match cap on each
    `inputs:` glob during resolution (plan
    2606101546 calls into the same validator).

2. Update body rendering in
   `internal/rules/build/`: render
   `body-template` once per `outputs` entry
   and join with newlines. `{output}` refers
   to the current entry in each iteration.
3. Update MDS040 in
   `internal/rules/recipesafety/`: add
   `inputs` and `outputs` to the
   reserved-param list. A recipe that
   declares either as a `params.required`
   or `params.optional` entry is a config
   error.
4. Rewrite MDS039 fixtures (`good/`, `bad/`,
   `fixed/`) for the new directive shape.
5. Update `internal/rules/build/rule_test.go`:
   replace every `output:` use with
   `outputs:`; add cases for multi-output
   body rendering and empty `outputs:`.
6. Update the user guide
   `docs/guides/directives/build.md`:
   document `outputs:` (list), `inputs:`
   (list), the once-per-output body render,
   and the `{outputs}` / `{inputs}`
   placeholders. Delete singular-form prose.
7. Update the build edge in
   `internal/linkgraph/directives.go`: one
   edge per `inputs:` entry (globs like
   catalog globs), replacing `source:` and
   its stale comment. Cover `deps` and
   `--incoming` in tests.

## Acceptance Criteria

- [x] `<?build?>` requires `outputs:` (list,
      non-empty); `output:` is rejected as an
      unknown param
- [x] `<?build?>` accepts optional `inputs:`
      (list of paths or globs)
- [x] Each `outputs:` and `inputs:` entry
      passes the path-shape rules: no NUL,
      no newline, no leading/trailing
      whitespace, no Windows drive letters,
      no UNC prefix, no NTFS ADS, no reserved
      device names, no `..` after `Clean`,
      nothing under `.mdsmith/`
- [x] An empty `outputs:` list, or any empty
      or whitespace-only entry inside either
      list, is a diagnostic
- [x] `outputs:` entries reject glob
      characters (`*`, `?`, `[`); `inputs:`
      accepts full doublestar globs
      (including leading `**/`)
- [x] An `inputs:` glob that resolves to
      more than 10 000 files is a build error
- [x] A symlinked output or input that
      escapes the project root is a build
      error
- [x] `body-template` renders once per
      `outputs` entry, joined with newlines,
      in declared order
- [x] `{output}` in `body-template` refers to
      the current output in each render
      iteration
- [x] MDS040 rejects a recipe declaring
      `inputs` or `outputs` in
      `params.required` or `params.optional`
- [x] All MDS039 fixtures use the new
      directive shape
- [x] `mdsmith deps` lists one build edge
      per `inputs:` entry; `--incoming` on
      an input names the directive's file
- [x] `docs/guides/directives/build.md`
      describes `outputs:` and `inputs:`; no
      singular-form prose remains
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no
      issues
