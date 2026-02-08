# 10: Fix Mode (`--fix`)

## Goal

Auto-fix in place for all fixable rules. The `--fix` flag rewrites files on
disk with corrections applied.

## Fixable Rules

TM002, TM006, TM007, TM008, TM009, TM010, TM012, TM013, TM014, TM015, TM016

## Tasks

1. Fix coordinator (`internal/fix/fix.go`)
   - For each file: run fixable rules in deterministic order (by rule ID)
   - Each fixable rule returns corrected `[]byte` content
   - Re-parse AST between rule passes (source positions change after edits)
   - Write final result back to disk
2. Wire `--fix` flag in `main.go`

## Acceptance Criteria

- [ ] `tidymark --fix file.md` modifies the file in place
- [ ] After fixing, re-running `tidymark file.md` on the same file reports
      zero violations for the fixed rules
- [ ] Fixes are applied in rule-ID order (TM002 before TM006 before TM007, etc.)
- [ ] Multiple fixable violations in one file are all corrected in a single
      invocation
- [ ] Non-fixable rule violations are still reported after fixing (exit code 1
      if any remain)
- [ ] A file with no violations is not modified (mtime unchanged)
- [ ] `--fix` combined with `--format json` outputs remaining violations in JSON
- [ ] `--fix` with a read-only file exits 2 with an error message
- [ ] `--fix` on multiple files fixes each one independently
- [ ] Fixing a file does not introduce new violations from other rules
      (re-parse between passes ensures correctness)
- [ ] `--fix` without file arguments exits 0 (graceful empty invocation)
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
