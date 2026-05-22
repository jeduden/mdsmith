---
id: 200
title: Move docs/ embed out of internal/lsp/hover.go
status: "🔲"
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

## Tasks

1. Create `internal/directives/` with a
   `directives.go` file that embeds the
   content from `docs/guides/directives/`.
2. Replace the import in
   `internal/lsp/hover.go` to
   `internal/directives`.
3. Remove the Go package files from
   `docs/guides/directives/` (keep the
   Markdown docs there).
4. Add `TestDirectivesSource` in
   `internal/directives/directives_test.go`.
5. Run `go build ./...` and
   `go test ./...`.
6. Run `go run ./cmd/mdsmith check .`.

## Acceptance Criteria

- [ ] `grep -r 'docs/guides/directives' internal/`
  returns no Go imports.
- [ ] `internal/directives/` has the
  embed and a passing unit test.
- [ ] `go build ./...` clean.
- [ ] `go test ./...` passes.
- [ ] `go tool golangci-lint run` clean.
