# 01: Skeleton & Core Types

## Goal

Compilable binary that exits 0, plus foundational types that all later phases
build on. Add goldmark as the Markdown AST parser dependency.

## Tasks

1. Add goldmark dependency: `go get github.com/yuin/goldmark`
2. Define the `File` type (`internal/lint/file.go`)
   - `Path string`, `Source []byte`, `Lines [][]byte`, `AST ast.Node`
   - Constructor parses source with goldmark once; all rules share the result
3. Define the `Diagnostic` type (`internal/lint/diagnostic.go`)
   - Fields: `File`, `Line`, `Column`, `RuleID`, `RuleName`, `Severity`, `Message`
4. Define the `Rule` interface (`internal/rule/rule.go`)
   - `ID() string`, `Name() string`, `Check(f *lint.File) []lint.Diagnostic`
5. Define the `FixableRule` interface (extends `Rule`)
   - Adds `Fix(f *lint.File) []byte`
6. Rule registry (`internal/rule/registry.go`)
   - `Register(r Rule)`, `All() []Rule`, `ByID(id string) Rule`
7. Minimal `main.go` (`cmd/tidymark/main.go`)
   - Parse flags with stdlib `flag`
   - `--version` and `--help`; exit 0

## Acceptance Criteria

- [ ] `go build ./cmd/tidymark` produces a binary without errors
- [ ] Running the binary with no args exits with code 0
- [ ] `tidymark --version` prints a version string and exits 0
- [ ] `tidymark --help` prints usage text and exits 0
- [ ] `File` constructor accepts raw bytes, returns a struct with parsed AST
      (goldmark `ast.Document`) and `Lines` split correctly
- [ ] Parsing an empty file produces a valid (empty) AST without error
- [ ] Parsing a file with headings, code blocks, and lists produces a non-nil AST
- [ ] `Register` + `All` round-trips: registering N rules and calling `All()`
      returns all N in registration order
- [ ] `ByID("TM001")` returns the registered rule; unknown ID returns nil
- [ ] `Diagnostic` struct can be constructed and its fields read back correctly
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
