---
id: 2606260614
title: >-
  arch-fix: add dedicated unit tests for
  lineclass_scan.go HTML-scanning helpers
status: "🔲"
summary: >-
  Nine unexported sub-functions of the
  type-7 HTML block scanning path in
  internal/lint/lineclass_scan.go lack
  dedicated unit tests. This plan adds
  TestScanHTMLTag, TestScanClosingTag,
  TestScanOpenTag, TestScanTagName,
  TestScanAttribute, TestScanAttrValue,
  TestSkipHTMLWS, TestIsUnquotedStop, and
  TestEqualFoldASCII.
model: sonnet
---
# arch-fix: lineclass_scan HTML-scanning helper tests

## Goal

Nine unexported HTML-scanning helpers in
`internal/lint/lineclass_scan.go` lack
dedicated unit tests. Add a dedicated
`TestFoo` for each one. Closes the 2026-06-26
audit finding.

## Context

Audit 2026-06-26 (range: 3d35b77..fe7141b)
flagged nine unexported helpers. All nine
are sub-functions of `htmlType7Start`:

- `scanHTMLTag(s []byte) (int, bool)`
- `scanClosingTag(s []byte, i int) (int, bool)`
- `scanOpenTag(s []byte, i int) (int, bool)`
- `scanTagName(s []byte, i int) (int, bool)`
- `scanAttribute(s []byte, i int) (int, bool)`
- `scanAttrValue(s []byte, i int) (int, bool)`
- `skipHTMLWS(s []byte, i int) int`
- `isUnquotedStop(b byte) bool`
- `equalFoldASCII(a, b []byte) bool`

The existing `TestHTMLType7Start` exercises
these indirectly. The architecture tests doc
§"every function by name" requires a
dedicated test per function.

## Tasks

1. [ ] Add `TestScanHTMLTag` to
   `internal/lint/lineclass_scan_test.go`.
   Cover the open-tag path, the closing-tag
   path, and a non-`<` byte that returns
   `(0, false)`.
2. [ ] Add `TestScanClosingTag`. Cover
   `</tagname>` success, missing `>`, and
   a name starting with a digit.
3. [ ] Add `TestScanOpenTag`. Cover a
   minimal `<img>` success, `<img />`, one
   attribute, and failure cases
   (`<img src=>`, unclosed tag).
4. [ ] Add `TestScanTagName`. Cover a valid
   name, a name starting with a digit (fail),
   an empty input (fail), and a name
   terminated by `>`.
5. [ ] Add `TestScanAttribute`. Cover a
   plain valueless attribute, a `key=value`
   pair, a `key="value"` pair, and `=` with
   no value.
6. [ ] Add `TestScanAttrValue`. Cover bare
   unquoted, single-quoted, and
   double-quoted values; an unclosed quote;
   and an empty unquoted value.
7. [ ] Add `TestSkipHTMLWS`. Cover spaces,
   tabs, a mix, and a non-whitespace byte
   at position 0.
8. [ ] Add `TestIsUnquotedStop`. Cover bytes
   that stop (`"`, `'`, `=`, `>`, `` ` ``,
   control bytes) and bytes that do not.
9. [ ] Add `TestEqualFoldASCII`. Cover
   same-case match, mixed-case match,
   different bytes (no match), and
   different lengths (no match).

## Acceptance Criteria

- [ ] Each of the nine functions has a
  dedicated top-level `TestFoo` in
  `internal/lint/lineclass_scan_test.go`.
- [ ] `go test ./internal/lint/...` green.
- [ ] `go vet ./...` clean.
- [ ] No production code changed; tests only.
