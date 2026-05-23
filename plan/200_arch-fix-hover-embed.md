---
id: 200
title: Move docs/ embed out of internal/lsp/hover.go
status: "✅"
summary: >-
  Fix the DIP violation where internal/lsp/hover.go
  imports a Go package from docs/guides/directives.
  Move the embed to internal/directives so the
  docs tree contains only documentation.
model: ""
depends-on: []
---
# Move docs/ embed out of internal/lsp/hover.go

## Goal

[internal/lsp/hover.go](../internal/lsp/hover.go) imports
[docs/guides/directives](../docs/guides/directives) as an `embed.FS`.
A Go package inside `docs/` blurs source
vs. documentation. It also violates the
layering map: no `docs/` layer sits
between helpers and [internal/lsp](../internal/lsp).
Moving the embed to `internal/directives`
follows the `internal/concepts` pattern.

The full user guides in
[docs/guides/directives/](../docs/guides/directives/)
stay put. They are indexed by the
`docs/guides/index.md` catalog and linked
from rule READMEs.

The new
[internal/directives/](../internal/directives/)
holds short, hover-sized stubs. This is the
`internal/concepts/placeholder-grammar.md`
pattern: same filename as the guide, but
separate Go-private content. Each stub links
out to the matching full guide.

## Tasks

1. Add `internal/directives/**` to
   `directory-structure.allowed` in
   `.mdsmith.yml` so the new package's
   markdown files lint cleanly.
2. Create `internal/directives/` with a
   `directives.go` file that embeds
   `*.md` plus short hover stubs for
   `build.md`, `enforcing-structure.md`,
   and `generating-content.md`. Each stub
   summarises the directive(s) and links
   out to the matching
   `docs/guides/directives/` guide.
3. Replace the import in
   `internal/lsp/hover.go` to
   `internal/directives`.
4. Remove `embed.go` from
   `docs/guides/directives/` (keep the
   Markdown guides there).
5. Add `TestDirectivesSource` in
   `internal/directives/directives_test.go`.
6. Run `go build ./...` and
   `go test ./...`.
7. Run `go run ./cmd/mdsmith check .`.

## Acceptance Criteria

- [x] `grep -r 'docs/guides/directives' internal/`
  returns no Go imports.
- [x] `internal/directives/` has the
  embed and a passing unit test.
- [x] `go build ./...` clean.
- [x] `go test ./...` passes.
- [x] `go tool golangci-lint run` clean.
