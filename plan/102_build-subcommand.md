---
id: 102
title: Multi-output `<?build?>` directive
status: "🔲"
summary: >-
  Replace `output:` (singular) with `outputs:`
  (list) on the `<?build?>` directive. Add
  `inputs:` (list, validation only — staleness
  lands in plan 103). Render `body-template`
  once per output. Add `{outputs}` and
  `{inputs}` argv placeholders to recipe
  `command`. No backwards compatibility.
model: opus
---
# Multi-output `<?build?>` directive

## Goal

The `<?build?>` directive declares a list of
artifact paths in `outputs:` (was a single
string `output:` in plan 101) and a list of
input paths or globs in `inputs:`. The body
template renders once per output. Recipe
`command` strings can reference `{outputs}` and
`{inputs}` to pass the lists to the recipe
binary as separate argv arguments.

## Context

Builds on plan 101 (the `<?build?>` directive
and MDS039) and plan 100 (`build:` config and
MDS040). Plan 115 wires the resulting targets
through a `Builder` and into `mdsmith fix`.
Plan 103 layers staleness on top of `inputs:` /
`outputs:`.

`output:` and `outputs:` are not both accepted.
This is a clean break — there is no backwards
compatibility for the singular form, no
deprecation warning, no migration diagnostic.
MDS039 simply rejects unknown directive params,
so `output:` becomes an error like any typo.

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

`inputs:` may be empty (some recipes have no
file inputs — e.g. a recipe that scrapes a
remote URL). With no file inputs, plan 103's
ActionID covers the recipe spec and output
hashes only; the target stays fresh until
the recipe spec or outputs change.

Each entry in `outputs:` is a literal relative
path. No globs. Every output must be a path the
recipe will write. This keeps post-build
verification deterministic (every declared
output must exist on disk after the recipe
returns).

Each entry in `inputs:` is a literal path or a
glob. Globs are evaluated at build time (plan
115). Plan 102 only validates the path shape
(see "MDS039 update" below).

### Body template — rendered once per output

The recipe's `body-template` is rendered once
per `outputs` entry, in declared order. The
rendered lines are joined with newlines and
stored as the section body.

| Placeholder | Value per render iteration                   |
|-------------|----------------------------------------------|
| `{output}`  | The current output path                      |
| `{alt}`     | `"{recipe} output: {output}"` for that entry |

With `outputs: [foo.png]` the body is one line.
With `outputs: [a.png, b.png]` the body is two
lines, in declared order.

Any change to `outputs:` makes the rendered
body diverge, and MDS039 reports `generated
section is out of date` — same guarantee as
plan 101.

### MDS039 update

MDS039 (plan 101) is changed to:

1. Reject `output:` (singular) — it is no
   longer a known param. The standard "unknown
   param" diagnostic applies.
2. Require `outputs:` (list of strings,
   non-empty). Each entry is validated per
   "Path-shape rules" below.
3. Accept optional `inputs:` (list of strings,
   may be empty). Each entry is a relative
   path with no `..` or a glob. The glob shape
   is validated; resolution is plan 115's job.
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
- For `outputs:`: no glob characters
  (`*`, `?`, `[`). Outputs are literal paths.
- For `inputs:` globs: `**` is allowed only
  inside a path (not as the first segment) to
  bound expansion at the project root.

At build time (plan 115) the resolved path is
re-checked: `filepath.EvalSymlinks` must not
escape the project root. A symlinked output
or input pointing outside the project is a
build error.

### Glob match cap

A single `inputs:` glob that matches more
than 10 000 files is a build error. The cap
is per directive entry, not per directive,
so an author who needs more declares
multiple narrower patterns.

### Recipe `command` placeholders

Recipe `command` strings (plan 100) keep
existing `{param}` rules. Two new collective
placeholders are added:

| Placeholder | Expansion                               |
|-------------|-----------------------------------------|
| `{outputs}` | One argv per directive `outputs:` entry |
| `{inputs}`  | One argv per resolved `inputs:` entry   |

`outputs` and `inputs` become reserved param
names. They may not appear in
`params.required` or `params.optional`. MDS040
checks this.

A single-output recipe uses a named param:
`command: "tool -o {dest}"` with the directive
supplying `dest: foo.png` and `outputs:
[foo.png]`. The author keeps both fields in
sync; MDS039 does not auto-link them.

A multi-output recipe uses `{outputs}`:
`command: "magick convert in.svg {outputs}"`.
The directive supplies `outputs: [a.png,
b.png]`; argv expansion appends each as a
separate argument.

The actual argv expansion happens in plan 115.
Plan 102's MDS040 update only validates that
the reserved names are not declared as params.

### Fixture and doc updates

- `internal/rules/MDS039-build/good/`,
  `bad/`, and `fixed/` fixtures are rewritten
  for the new directive shape.
- `docs/guides/directives/build.md` documents
  `outputs:` and `inputs:` and the once-per-
  output body render. Sentences referring to
  the old singular form are deleted, not
  marked deprecated.

## Tasks

1. Update MDS039 in `internal/rules/build/`:

  - Drop `output` from the known-param set.
  - Add `outputs` as required (list of
    strings, non-empty). Each entry validated
    by the path-shape rules above.
  - Add `inputs` as optional (list of strings).
    Each entry validated by the path-shape
    rules; globs allowed except `**` at
    position 0.
  - Enforce the 10 000-match cap on each
    `inputs:` glob during resolution (plan
    115 calls into the same validator).

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
5. Update unit tests in
   `internal/rules/build/rule_test.go`:
   replace every `output:` use with
   `outputs:` (list). Add cases for
   multi-output body rendering and for empty
   `outputs:`.
6. Update the user guide
   `docs/guides/directives/build.md`:
   document `outputs:` (list), `inputs:`
   (list), the once-per-output body render,
   and the `{outputs}` / `{inputs}`
   placeholders. Delete singular-form prose.

## Acceptance Criteria

- [ ] `<?build?>` requires `outputs:` (list,
      non-empty); `output:` is rejected as an
      unknown param
- [ ] `<?build?>` accepts optional `inputs:`
      (list of paths or globs)
- [ ] Each `outputs:` and `inputs:` entry
      passes the path-shape rules: no NUL,
      no newline, no leading/trailing
      whitespace, no Windows drive letters,
      no UNC prefix, no NTFS ADS, no reserved
      device names, no `..` after `Clean`
- [ ] An empty `outputs:` list, or any empty
      or whitespace-only entry inside either
      list, is a diagnostic
- [ ] `outputs:` entries reject glob
      characters (`*`, `?`, `[`); `inputs:`
      globs reject `**` at position 0
- [ ] An `inputs:` glob that resolves to
      more than 10 000 files is a build error
- [ ] A symlinked output or input that
      escapes the project root is a build
      error
- [ ] `body-template` renders once per
      `outputs` entry, joined with newlines,
      in declared order
- [ ] `{output}` in `body-template` refers to
      the current output in each render
      iteration
- [ ] MDS040 rejects a recipe declaring
      `inputs` or `outputs` in
      `params.required` or `params.optional`
- [ ] All MDS039 fixtures use the new
      directive shape
- [ ] `docs/guides/directives/build.md`
      describes `outputs:` and `inputs:`; no
      singular-form prose remains
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
