---
id: 32
title: Max File Length Rule
status: ✅
---
# Max File Length Rule

## Goal

Add MDS022 `max-file-length` that warns when a markdown
file has too many lines. The limit is set in the config.
This nudges authors to split long files.

## Tasks

1. Create `internal/rules/maxfilelength/rule.go` implementing
   `rule.Rule` and `rule.Configurable`. Settings:

   | Setting | Type | Default | Description           |
   |---------|------|---------|-----------------------|
   | `max`   | int  | 300     | Maximum lines allowed |

2. Check logic: count `len(f.Lines)`. If it exceeds `max`,
   emit a single diagnostic on line 1:

  - `file too long (350 > 300)`

3. Register rule in `init()`, add blank import in `main.go`
   and integration test file.

4. Add `Category() string` returning `"meta"`.

5. Write `rules/MDS022-max-file-length/README.md` with
   settings table, config example, and bad/good examples.

6. Create test fixtures: `bad.md` (a file exceeding 300
   lines) and `good.md` (a file under the limit).

7. Unit tests:

  - File at exactly `max` lines: no diagnostic
  - File at `max + 1` lines: diagnostic
  - Custom `max` setting respected
  - Empty file: no diagnostic

8. Add default config entry in `.mdsmith.yml`.

9. Run `go test ./...` and `go tool golangci-lint run`.

## Acceptance Criteria

- [x] MDS022 reports when file exceeds max lines
- [x] Single diagnostic on line 1 with count details
- [x] `max` setting configurable, default 300
- [x] Rule README with examples
- [x] All tests pass: `go test ./...`
- [x] `golangci-lint run` reports no issues
