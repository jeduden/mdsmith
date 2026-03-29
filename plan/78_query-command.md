---
id: 78
title: "Query subcommand for front-matter filtering"
status: "🔲"
summary: "Add mdsmith query to select files by CUE expression on front matter"
---
# Query subcommand for front-matter filtering

## Context

The `plan/` directory accumulates completed plans. Today
the only cleanup path is manual grep-and-rm. mdsmith
already parses front matter and evaluates CUE constraints
(in `requiredstructure`), so it can drive file selection
natively.

A general `query` subcommand prints matching file paths
to stdout. Callers compose it with Unix tools:

```bash
mdsmith query 'status: "✅"' "plan/[0-9]*.md" \
  | xargs git rm
```

CUE is the query language. mdsmith already depends on
`cuelang.org/go/cue` for front-matter validation. A CUE
expression unifies with parsed YAML front matter — the
same mechanism `validateFrontMatterCUE` uses.

## Goal

Add `mdsmith query` to print paths of Markdown files
whose front matter satisfies a CUE expression, one path
per line on stdout.

## Design

```bash
mdsmith query [flags] <cue-expr> [files...]
```

Flags:

- `-0` — NUL-delimit output (for `xargs -0`)
- `-v, --verbose` — print skipped files and reasons
  on stderr

The CUE expression is a struct literal body. It is
compiled and unified with each file's front matter. Files
whose front matter satisfies the expression are printed.

Examples:

```bash
# List completed plans
mdsmith query 'status: "✅"' "plan/[0-9]*.md"

# Delete them
mdsmith query 'status: "✅"' "plan/[0-9]*.md" \
  | xargs git rm

# NUL-delimited for paths with spaces
mdsmith query -0 'status: "✅"' plan/ | xargs -0 rm

# Compound condition
mdsmith query 'status: "✅", id: >50' plan/

# All plans not yet started
mdsmith query 'status: "🔲"' plan/
```

Output goes to stdout (one path per line). Status
messages and errors go to stderr.

Exit codes follow `grep` convention, not the linter
convention (`check`/`fix` use 1 for "issues found").
This is intentional — `query` is a filter, not a
linter:

- 0 — at least one file matched
- 1 — no files matched
- 2 — invalid CUE expression or runtime error

A malformed CUE expression prints the compile error to
stderr and exits 2, so typos are never silent.

Files without front matter or where unification fails
are silently skipped (shown with `--verbose`).

Proto/template files are naturally excluded because their
front-matter values are CUE schema strings (e.g.
`'"🔲" | "🔳" | "✅"'`), not concrete values — CUE
unification rejects non-concrete data.

## Implementation

The core logic mirrors `validateFrontMatterCUE` in
`internal/rules/requiredstructure/rule.go`:

```go
func matchFrontMatter(schema cue.Value,
    fm map[string]any) bool {
  data, _ := json.Marshal(fm)
  val := schema.Context().CompileBytes(data)
  merged := schema.Unify(val)
  return merged.Validate(cue.Concrete(true)) == nil
}
```

The caller compiles the CUE expression once and exits
2 on compile error. It then passes the compiled value
to `matchFrontMatter` for each file.

Reuses `lint.ResolveFilesWithOpts` for file discovery.
Reuses `lint.StripFrontMatter` plus YAML unmarshal
for front matter. Unmarshals into `map[string]any` to
keep numeric types so `id: >50` works.

## Tasks

1. Extract `readFrontMatterRaw(path) map[string]any`
   into a shared internal package (or keep a small local
   copy) so both catalog and query can use it
2. Add `internal/query/query.go` with
   `Match(expr string, fm map[string]any) bool`
   wrapping CUE compile-unify-validate
3. Add `runQuery` in `cmd/mdsmith/main.go` with flag
   parsing (`-0`, `--verbose`), file resolution, and
   stdout output loop
4. Register `query` in the subcommand dispatcher and
   update `usageText`
5. Add `query` to the Commands table in `README.md`
   and `docs/design/cli.md`
6. Write unit tests for `internal/query`: matching
   expression passes, non-matching fails, missing field
   fails, absent front matter fails, schema-string
   front matter (proto) fails, compound expressions
7. Write an e2e test: matches printed to stdout, no
   matches yields exit 1, `-0` uses NUL delimiter
8. Run `mdsmith check .` and `go test ./...`

## Acceptance Criteria

- [ ] `mdsmith query 'status: "✅"' plan/` prints paths
  of completed plans, one per line
- [ ] Piping into `xargs git rm` deletes them
- [ ] Exit 0 on match, exit 1 on no match
- [ ] `-0` outputs NUL-delimited paths
- [ ] Files without front matter are silently skipped
- [ ] `proto.md` is never matched (non-concrete CUE
  values fail validation)
- [ ] All tests pass: `go test ./...`
- [ ] `README.md` Commands table includes `query`
- [ ] `docs/design/cli.md` Commands table includes
  `query`
- [ ] Invalid CUE expression prints error to stderr
  and exits 2
- [ ] `go tool golangci-lint run` reports no issues
