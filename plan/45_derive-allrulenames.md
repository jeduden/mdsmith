---
id: 45
title: Derive allRuleNames from Rule Registry
status: 🔲
---
# Derive allRuleNames from Rule Registry

## Goal

Replace the hardcoded `allRuleNames` slice in `internal/config/load.go`
with a dynamic list derived from `rule.All()`, eliminating a duplicate
source of truth and fixing three rules missing from defaults.

## Tasks

1. In `internal/config/load.go`, modify `Defaults()` to build the rules
   map by iterating `rule.All()` instead of reading `allRuleNames`.
2. Remove the `allRuleNames` variable from `internal/config/load.go`.
3. Update `TestDefaultsAllRulesEnabled` in `internal/config/config_test.go`
   to derive expected rules from `rule.All()` instead of a hardcoded slice.
4. Run `go test ./...` and `go tool golangci-lint run`.

## Acceptance Criteria

- [ ] `Defaults()` derives rule list from `rule.All()`
- [ ] `allRuleNames` variable is removed from `internal/config/load.go`
- [ ] `TestDefaultsAllRulesEnabled` uses `rule.All()` for expectations
- [ ] All 22 registered rules appear in defaults (including
      `max-file-length`, `paragraph-readability`, `paragraph-structure`)
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
