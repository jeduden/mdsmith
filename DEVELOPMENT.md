---
title: Development
summary: Build commands, project layout, code style, test fixtures, and merge conflicts.
---

Build and test reference for mdsmith contributors.

## Build & Test Commands

Requires Go 1.24+.

- `go build ./...` — build all packages
- `go test ./...` — run all tests
- `go test -run TestName ./...` — run a specific test
- `go run ./cmd/mdsmith check .` — lint markdown
- `go run ./cmd/mdsmith fix .` — auto-fix markdown
- `go tool golangci-lint run` — run linter
- `go vet ./...` — run go vet

## Project Layout

Follow the [standard Go project
layout](https://go.dev/doc/modules/layout):

- `cmd/mdsmith/` — main application entry point
- `internal/` — private packages not importable by
  other modules
- `internal/rules/` — rule documentation
  (`internal/rules/<id>-<name>/README.md`)
- `testdata/` — test fixtures (markdown files for
  testing rules)

## Code Style

- Follow standard Go conventions (gofmt, goimports)
- Use golangci-lint for linting
- Keep functions small and focused
- Error messages should be lowercase, no trailing
  punctuation
- Prefer returning errors over panicking

## Test Fixtures

Rule test fixtures live in
`internal/rules/<id>-<name>/`. Each rule has `good/` and
`bad/` examples (or `good.md` / `bad.md`).

Good fixtures must pass **all** rules, not just their
own. When a good fixture uses non-default settings
(e.g. setext headings, tilde fences), add a matching
override in `.mdsmith.yml` so that `mdsmith check .`
also passes.

Bad fixtures are excluded via the `ignore:` section in
`.mdsmith.yml`.

## Test Coverage

- `go test -coverprofile=cover.out ./...` — generate
  coverage profile
- `go tool cover -html=cover.out` — view in browser
- `go tool cover -func=cover.out` — print per-function
  summary

Use coverage to confirm Red/Green TDD cycles exercise
all code paths.

## Generated Sections

Content between `<?directive ... ?>` and
`<?/directive?>` markers is auto-generated. Do not
edit the body by hand — edit the directive parameters
or the source file it references, then run
`mdsmith fix <file>` to regenerate.

After merge conflicts in generated sections, run
`mdsmith fix <file>` to regenerate. The
`merge-driver` command automates this:

```bash
mdsmith merge-driver install [files...]
```

Run `mdsmith merge-driver install` once per clone.
