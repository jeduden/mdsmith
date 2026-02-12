---
id: 33
title: golangci-lint File Length Limits
status: ✅
---
# golangci-lint File Length Limits

## Goal

Enable the `lll` (long lines) linter in golangci-lint for Go
source files and fix any existing violations, complementing
the `funlen` and `gocognit` checks added in Plan 31.

## Tasks

### A. Enable lll

1. Add `lll` to the `linters.enable` list in
   `.golangci.yml`.

2. Configure the threshold:

   ```yaml
   lll:
     line-length: 120
     tab-width: 4
   ```

3. Run `go tool golangci-lint run` to identify violations.

### B. Fix violations

4. For each violation, choose the appropriate fix:

  - Long string literals: break across lines or use
     constants
  - Long function signatures: wrap parameters
  - Long struct literals: one field per line
  - Long comments: rewrap to 120 columns

5. Ensure no behavioral changes.

### C. Verify

6. Run `go tool golangci-lint run` and confirm 0 issues.

7. Run `go test ./...` and confirm all tests pass.

## Acceptance Criteria

- [ ] `lll` enabled with line-length 120
- [ ] All Go source lines are 120 characters or fewer
- [ ] No behavioral changes
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
