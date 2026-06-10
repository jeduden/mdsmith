---
id: 2606022122
title: Review and centralize YAML handling
status: ✅
summary: Audit all YAML parsing/marshaling, unify under a central package, ensure consistent security checks
model: sonnet
---
# Review and centralize YAML handling

<!-- Plan conventions:
  - Work test-driven: write a failing test, make it
    pass, commit.
  - Plan files must pass `mdsmith check plan/`.
  - Use Markdown links for real repo paths in prose.
    Bare backticked paths are allowed in commands,
    code blocks, and placeholders.

  Status values:
  - 🔲 not started
  - 🔳 in progress
  - ✅ completed
  - ⛔ superseded (replaced by another plan)

-->

## Goal

Centralize YAML operations under one package. Reduce duplication.
Ensure consistent security checks prevent DoS attacks.

## Context

### Current State

YAML handling is scattered across 9 packages with 13+ unmarshal
sites. Each site must manually call `RejectYAMLAliases()` before
`yaml.Unmarshal()` to prevent billion-laughs attacks. This
pattern is duplicated at every call site.

### Security Layer

File: `internal/lint/yamlsafe.go`

- Defines `RejectYAMLAliases()` for DoS protection

### Config Loading

Files: `internal/config/{load,convention,config}.go`

- Load config files, validate conventions
- Define `RuleCfg` custom marshal/unmarshal

### Front-matter

File: `internal/lint/frontmatter.go`

- Parse front-matter `kinds:` field

### Directive Parsing

Files in `internal/archetype/gensection/`, `internal/rules/`:

- Parse YAML parameters for multiple directive types
- Files: `parse.go`, `catalog/rule.go`,
  `requiredstructure/rule.go`

### Output

Files: `internal/kindsout/kindsout.go`, `cmd/mdsmith/main.go`

- Marshal kind bodies and config

### Other

File: `internal/corpus/config.go`

- Load corpus config

## Tasks

1. Create `internal/yamlutil` package with:

  - `UnmarshalSafe(data []byte, v any) error` — combines
     alias rejection + unmarshal
  - `UnmarshalNodeSafe(data []byte) (yaml.Node, error)` —
     for node-based parsing
  - `Marshal(v any) ([]byte, error)` — thin wrapper for
     consistency
  - Move `RejectYAMLAliases` from `internal/lint` to
     `internal/yamlutil`

2. Update all 13+ `yaml.Unmarshal` call sites to use
   `yamlutil.UnmarshalSafe`:

  - `internal/config/load.go` (2 sites: load, topLevelKeySet;
     validateConventionScalar uses plain yaml.Unmarshal since
     node parsing does not expand aliases)
  - `internal/lint/frontmatter.go` (1 site)
  - `internal/archetype/gensection/parse.go` (1 site)
  - `internal/rules/catalog/rule.go` (1 site)
  - `internal/rules/requiredstructure/rule.go` (3 sites via
     UnmarshalSafe: require, include, schema; front-matter
     read uses explicit RejectYAMLAliases + yaml.Unmarshal
     to preserve distinct error messages per error type)
  - `internal/corpus/config.go` (2 sites)
  - `cmd/mdsmith/main.go` (check if front-matter parsing
     needs update)

3. Update all `yaml.Marshal` call sites to use
   `yamlutil.Marshal`:

  - `internal/kindsout/kindsout.go`
  - `cmd/mdsmith/main.go`
  - `internal/config/config.go` (RuleCfg.MarshalYAML —
     keep custom logic, but consider if any standardization
     helps)

4. Update import statements across all affected files
5. Add godoc to `internal/yamlutil` package explaining:

  - Why alias rejection is mandatory for user content
  - When to use `UnmarshalSafe` vs direct `yaml` package
  - Link to adversarial-markdown security doc

6. Update tests:

  - Add `internal/yamlutil/yamlutil_test.go` with tests for
    new wrapper functions and alias rejection
  - Repurpose `internal/lint/yamlsafe_test.go` to verify
    alias rejection through the front-matter parsing path
  - Ensure coverage of error paths

7. Run full test suite: `go test ./...`
8. Run linter: `go tool golangci-lint run`

## Acceptance Criteria

- [x] New `internal/yamlutil` package exists with documented
      safe-unmarshal wrappers
- [x] `RejectYAMLAliases` moved from `internal/lint` to
      `internal/yamlutil`
- [x] All user-content unmarshal sites use
      `yamlutil.UnmarshalSafe` (no direct `yaml.Unmarshal`
      on user data)
- [x] All marshal sites use `yamlutil.Marshal` or keep
      well-documented custom logic in place
- [x] `internal/yamlutil` has comprehensive godoc
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
- [x] No direct calls to `lint.RejectYAMLAliases` outside
      `yamlutil` package
