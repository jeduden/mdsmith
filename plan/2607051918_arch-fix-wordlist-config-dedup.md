---
id: 2607051918
title: >-
  Export wordlist.Dedup and remove the identical
  dedupStrings copy in internal/config
status: "🔲"
model: sonnet
summary: >-
  internal/wordlist.dedup and
  internal/config/wordlist_files.go's dedupStrings are
  byte-for-byte identical, and config already imports
  wordlist. Export one copy and delete the other.
  Flagged by the 2026-07-05 audit.
---
# Dedup wordlist/config helper duplication

## Goal

- Remove the duplicated `dedup`/`dedupStrings`
  helper between `internal/wordlist` and
  `internal/config`.
- Export one copy from `internal/wordlist` and
  delete the other.
- `internal/config` already imports
  `internal/wordlist`.

## Background

The 2026-07-05 audit (range: 0ededb3..528ce4c) found two
identical helpers:

- `internal/wordlist/wordlist.go`'s `dedup(ss []string)
  []string`
- `internal/config/wordlist_files.go`'s
  `dedupStrings(ss []string) []string`

Both bodies are byte-for-byte identical. Each
de-duplicates, keeping first-occurrence order, and
returns `nil` for empty input. `internal/config` already
imports `internal/wordlist` (for `wordlist.Lookup`/
`Resolve`). There is no layering reason for the second
copy — it is plain copy-paste.

While in this area, also fold in a second, lower-priority
duplicate pair the same audit flagged:

- `internal/config/wordlist_files.go`'s
  `stringsToAny(ss []string) []any`
- `internal/convention/nollmtells.go`'s
  `toAnySlice(ss []string) []any`

These have identical bodies too, but no import
relationship exists yet between
`internal/convention` and
`internal/config`/`internal/wordlist`.

Export `wordlist.ToAnySlice` alongside
`wordlist.Dedup` and point both call sites at it.
Do this instead of adding a new cross-package
import just for `internal/config`'s copy.

## Tasks

1. In `internal/wordlist/wordlist.go`, export `dedup` as
   `Dedup` and `stringsToAny`-equivalent as `ToAnySlice`
   (add `ToAnySlice` fresh — it does not exist in
   `internal/wordlist` yet).
2. Update `internal/wordlist`'s internal callers
   (`Resolve`) to use `Dedup`.
3. Add or update tests in
   `internal/wordlist/wordlist_test.go` for `Dedup` and
   `ToAnySlice`.
4. In `internal/config/wordlist_files.go`, delete
   `dedupStrings` and `stringsToAny`; replace call sites
   with `wordlist.Dedup` and `wordlist.ToAnySlice`.
5. In `internal/convention/nollmtells.go`, delete
   `toAnySlice`; replace call sites with
   `wordlist.ToAnySlice` (add the `internal/wordlist`
   import — confirm this does not create an import cycle;
   `internal/wordlist` must not import
   `internal/convention`).
6. `go build ./...` passes.
7. `go test ./internal/wordlist/...
   ./internal/config/... ./internal/convention/...`
   passes.

## Acceptance Criteria

- [ ] `internal/wordlist` exports `Dedup` and
      `ToAnySlice`, each with a dedicated test.
- [ ] `internal/config/wordlist_files.go` has no private
      `dedupStrings` or `stringsToAny`.
- [ ] `internal/convention/nollmtells.go` has no private
      `toAnySlice`.
- [ ] `go test ./...` is green.
- [ ] `mdsmith check .` is green.
