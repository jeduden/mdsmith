---
id: 81
title: 'OOM mitigation: configurable file-size limit'
status: "🔲"
summary: >-
  Guard every file-read path against OOM by enforcing
  a configurable byte-size cap (default 2 MB).
---
# OOM mitigation: configurable file-size limit

## Goal

Prevent out-of-memory crashes from large Markdown
files. Add a configurable byte-size limit enforced
before any file content is loaded into memory.

## Background

`os.ReadFile` and `io.ReadAll` load the entire input
into a byte slice with no size guard. A multi-GB `.md`
file (or stdin pipe) will OOM the process. The
`max-file-length` rule (MDS022) only emits a diagnostic
*after* the file is fully loaded — it does not prevent
the allocation.

Every top-5 Markdown linter (Prettier, markdownlint,
Vale, remark-lint, textlint) has the same gap. remark's
docs advise callers to "cap input at 500 KB" but
nothing is enforced.

## Design

### Shared helper

New file `internal/lint/limits.go`:

```go
const DefaultMaxInputBytes int64 = 2 * 1024 * 1024

func ReadFileLimited(path string, max int64) ([]byte, error)
func ReadFSFileLimited(fsys fs.FS, name string, max int64) ([]byte, error)
```

Both use `Open` + `io.LimitReader(f, max+1)` +
`io.ReadAll` + post-read `len(data) > max` check.
The `+1` sentinel distinguishes "exactly at limit"
from "truncated". When `max <= 0`, no limit is applied
(unlimited mode).

### Configuration

Top-level key in `.mdsmith.yml` (not a rule setting —
this is infrastructure, not a lint rule):

```yaml
max-input-size: 2MB
```

New `MaxInputSize` field on `config.Config`:

```go
MaxInputSize string `yaml:"max-input-size"`
```

### Size-string parser

New file `internal/config/size.go`:

```go
func ParseSize(s string) (int64, error)
```

Accepted formats: `2MB`, `500KB`, `1GB`, bare integer
(bytes), `0` (unlimited). Case-insensitive. Uses
binary units (1 MB = 1,048,576 bytes).

### CLI flag

```text
--max-input-size <size>
```

Added to both `check` and `fix` flag sets. CLI value
overrides the config value. Default: `"2MB"`.

### Threading the limit

Add `MaxInputBytes int64` field to `engine.Runner` and
`fix.Fixer`. Set from the parsed config + CLI override
in `cmd/mdsmith/main.go`.

### Read sites to guard

Primary entry points (replace `os.ReadFile` with
`lint.ReadFileLimited`):

1. `internal/engine/runner.go:52`
2. `internal/fix/fix.go:84`
3. `cmd/mdsmith/main.go:577` (stdin via
   `io.LimitReader`)

Secondary read sites (replace with
`lint.ReadFileLimited` or `lint.ReadFSFileLimited`):

4. `internal/rules/include/rule.go:194`
5. `internal/rules/catalog/rule.go:395,468`
6. `internal/rules/crossfilereferenceintegrity/rule.go:236,253`
7. `internal/rules/requiredstructure/rule.go:82,496`
8. `internal/metrics/rank.go:21`
9. `cmd/mdsmith/mergedriver.go:96,135`
10. `internal/config/load.go:16`

Rules that read files need the limit threaded via
`lint.File` or a new field on the rule struct (set
during `ApplySettings` or via the runner).

### Error behavior

- `check`/`fix`: Emit error, skip file, continue.
  Exit code 2.
- `stdin`: Print to stderr, exit 2.
- Message format:
  `reading "huge.md": file too large (15728640 bytes, max 2097152)`

## Tasks

1. Add `internal/lint/limits.go` with
   `ReadFileLimited` and `ReadFSFileLimited`
2. Add `internal/lint/limits_test.go` with tests for
   normal, at-limit, over-limit, zero (unlimited),
   and empty-file cases
3. Add `internal/config/size.go` with `ParseSize`
4. Add `internal/config/size_test.go` with tests for
   `2MB`, `500KB`, `0`, bare integer, invalid input
5. Add `MaxInputSize` field to `config.Config`
6. Add `MaxInputBytes` field to `engine.Runner` and
   `fix.Fixer`
7. Add `--max-input-size` flag to `check` and `fix`
   subcommands
8. Replace `os.ReadFile` with `ReadFileLimited` at all
   primary read sites (runner, fixer, stdin)
9. Replace `os.ReadFile` / `fs.ReadFile` at all
   secondary read sites (include, catalog,
   cross-file-ref, required-structure, metrics, merge
   driver, config)
10. Thread `MaxInputBytes` to rules that read files
    (via `lint.File` field or rule configuration)
11. Add integration test: file exceeding limit produces
    error diagnostic and exit code 2
12. Document `max-input-size` in `docs/reference/cli.md`

## Acceptance Criteria

- [ ] `ReadFileLimited` returns error for files
      exceeding the configured limit
- [ ] `ReadFileLimited` succeeds for files at or below
      the limit (no off-by-one)
- [ ] `ReadFileLimited` with `max <= 0` applies no
      limit (unlimited mode)
- [ ] `ParseSize` handles `2MB`, `500KB`, `1GB`, bare
      integers, and `0`
- [ ] `.mdsmith.yml` `max-input-size` key is respected
- [ ] `--max-input-size` CLI flag overrides config
- [ ] `--max-input-size 0` disables the limit
- [ ] All ~15 read sites use the limited helper
- [ ] Error message includes actual size and limit
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
