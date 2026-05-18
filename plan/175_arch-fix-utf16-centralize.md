---
id: 175
title: Centralize UTF-16 column helpers in internal/mdtext
status: 🔲
summary: >-
  Lift the triplicated nonNegativeUTF16RuneLen /
  utf16FromByteOffset helpers out of internal/lsp,
  internal/rename, and cmd/mdsmith into
  internal/mdtext, removing the three private
  copies.
model: ''
depends-on: []
---
# Centralize UTF-16 column helpers in internal/mdtext

## Goal

Export three UTF-16 helpers from
`internal/mdtext`:

- `NonNegativeUTF16RuneLen`
- `UTF16FromByteOffset`
- `UTF16ToByteOffset`

Remove the three private copies in
`internal/lsp`, `internal/rename`, and
`cmd/mdsmith`.

## Tasks

1. Add exported functions to
   `internal/mdtext`:

  - `NonNegativeUTF16RuneLen(r rune) int`
  - `UTF16FromByteOffset(line []byte, byteOff int) int`
  - `UTF16ToByteOffset(line []byte, target int) int`

   Add unit tests for each in
   `internal/mdtext/utf16_test.go`.

2. Replace `nonNegativeUTF16RuneLen`
   and `utf16FromByteOffset` in
   `internal/lsp/diagnostics.go` with
   calls to the new `mdtext` functions.
   Update
   `internal/lsp/server_test.go` to
   remove its now-stale private test
   of the helper.
3. Replace `nonNegativeUTF16RuneLen`
   and `utf16FromByteOffset` in
   `internal/rename/rename.go` with
   calls to the new `mdtext` functions.
   Remove the private copy and its
   test in
   `internal/rename/helpers_test.go`.
4. Replace `nonNegativeUTF16RuneLen`
   and `utf16ToByteOffset` in
   `cmd/mdsmith/rename.go` with calls
   to `mdtext.NonNegativeUTF16RuneLen`
   and `mdtext.UTF16ToByteOffset`.
   Update `rename_unit_test.go` to
   remove the now-stale private test.
5. Run `go build ./...` and
   `go test ./...` to confirm no
   breakage.
6. Run `go tool golangci-lint run` to
   confirm no lint regressions.

## Acceptance Criteria

- [ ] `internal/mdtext` exports
  `NonNegativeUTF16RuneLen`,
  `UTF16FromByteOffset`, and
  `UTF16ToByteOffset`, each with a
  dedicated unit test.
- [ ] No private copy of
  `nonNegativeUTF16RuneLen`,
  `utf16FromByteOffset`, or
  `utf16ToByteOffset` remains in
  `internal/lsp/`, `internal/rename/`,
  or `cmd/mdsmith/`.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run`
  reports no issues.
