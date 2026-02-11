# Enable Stricter golangci-lint Checks

## Goal

Enable `funlen` and `gocognit` linters in golangci-lint to
enforce function length limits and cognitive complexity bounds,
then fix all existing violations so the codebase passes cleanly.

## Current State

The project enables 7 linters (errcheck, govet, staticcheck,
unused, ineffassign, misspell, revive) plus 2 formatters (gofmt,
goimports). Enabling `funlen` (max 60 lines) and `gocognit`
(max 30) today produces 22 violations:

- **funlen** (15 issues): long functions in rule
  implementations, tests, config, and generated-section
- **gocognit** (7 issues): high cognitive complexity in Fix
  methods, file resolution, integration tests, and a heading
  style helper

## Tasks

### A. Enable linters in config

1. Add `funlen` and `gocognit` to the `linters.enable` list
   in `.golangci.yml`.

2. Configure thresholds in `linters.settings`:

   ```yaml
   funlen:
     lines: 60
     statements: 40
   gocognit:
     min-complexity: 30
   ```

### B. Fix funlen violations in production code (8)

3. `internal/rules/blanklinearoundlists/rule.go` Check
   (61 > 60): extract helper for adjacent-line blank check.

4. `internal/rules/firstlineheading/rule.go` Check
   (69 > 60): extract frontmatter-skip and heading-search
   into helpers.

5. `internal/rules/generatedsection/parse.go`
   findMarkerPairs (43 stmts > 40): extract inner-loop body
   into a helper.

6. `internal/rules/generatedsection/parse.go`
   parseDirective (75 > 60): split YAML parsing and
   validation into separate functions.

7. `internal/rules/generatedsection/rule.go`
   validateDirective (129 > 60): break into per-directive-type
   validation helpers (catalog, include, etc.).

8. `internal/rules/generatedsection/rule.go`
   resolveCatalogWithDiags (62 > 60): extract glob-resolve
   or template-render step.

9. `internal/rules/headingstyle/rule.go` Fix (62 > 60):
   extract line-replacement logic into a helper.

10. `internal/rules/listindent/rule.go` Fix (65 > 60):
    extract indent-rewrite loop body.

### C. Fix gocognit violations in production code (5)

11. `internal/fix/fix.go` Fixer.Fix (complexity 36 > 30):
    extract per-file fix loop into a method.

12. `internal/lint/files.go` ResolveFilesWithOpts
    (complexity 43 > 30): extract directory-walk and
    gitignore logic into helpers.

13. `internal/rules/blanklinearoundfencedcode/rule.go` Fix
    (complexity 33 > 30): extract insert-blank-line decision
    into a helper.

14. `internal/rules/blanklinearoundheadings/rule.go` Fix
    (complexity 32 > 30): same pattern as fenced-code fix.

15. `internal/rules/blanklinearoundlists/rule.go` Fix
    (complexity 38 > 30): extract blank-line insertion logic.

16. `internal/rules/headingstyle/rule.go` isATXHeading
    (complexity 35 > 30): simplify or split conditionals.

### D. Fix funlen violations in test code (7)

17. `internal/config/config_test.go` TestParseValidYAML
    (70 > 60): use subtests or table-driven pattern.

18. `internal/engine/runner_test.go`
    TestRunner_DiagnosticsSortedByFileLineColumn (63 > 60):
    extract setup into a helper.

19. `internal/output/json_test.go`
    TestJSONFormatter_MultipleDiagnostics (66 > 60):
    extract assertion block.

20. `internal/rules/generatedsection/rule_test.go`
    TestCheck_DiagnosticScenarios (276 > 60): split into
    multiple focused subtests.

21. `internal/rules/generatedsection/rule_test.go`
    TestCheck_ContentGeneration (72 > 60): extract setup
    or split cases.

22. `internal/rules/generatedsection/rule_test.go`
    TestSort_Behavior (171 > 60): split into focused
    subtests.

23. `internal/rules/generatedsection/rule_test.go`
    TestSpec_DiagnosticLineNumbers (82 > 60): extract
    test helper.

### E. Fix gocognit violations in test code (2)

24. `internal/integration/rules_test.go` TestRuleFixtures
    (complexity 93 > 30): extract fixture-discovery,
    assertion, and fix-comparison into helper functions.

### F. Verify

25. Run `go tool golangci-lint run` and confirm 0 issues.

26. Run `go test ./...` and confirm all tests pass.

## Acceptance Criteria

- [ ] `funlen` enabled with lines: 60, statements: 40
- [ ] `gocognit` enabled with min-complexity: 30
- [ ] All 15 funlen violations fixed by extracting helpers
      or splitting functions
- [ ] All 7 gocognit violations fixed by reducing cognitive
      complexity
- [ ] No behavioral changes -- refactoring only
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
