---
id: 39
title: Extract generated-section archetype code
status: ✅
---
# Extract generated-section archetype code

## Goal

Extract the shared marker-based content generation
mechanism from `internal/rules/catalog/` into a reusable
archetype package `internal/archetype/gensection/`, so
future rules (e.g., `toc`) can reuse marker parsing,
YAML extraction, rendering, and fix logic.

## Tasks

### A. Define archetype interface

1. Create `internal/archetype/gensection/` package with:

   ```go
   // Directive defines a generated-section rule that
   // produces content from markers.
   type Directive interface {
       // Name returns the directive/rule name used in
       // markers (e.g., "catalog").
       Name() string

       // Validate checks directive-specific parameters.
       // Returns diagnostics for invalid params.
       Validate(params map[string]string,
           columns map[string]ColumnConfig,
       ) []lint.Diagnostic

       // Generate produces the expected content between
       // markers. Returns content and any diagnostics.
       Generate(f *lint.File, params map[string]string,
           columns map[string]ColumnConfig,
       ) (string, []lint.Diagnostic)
   }
   ```

2. Create `ColumnConfig` type (moved from catalog):

   ```go
   type ColumnConfig struct {
       MaxWidth int
       Wrap     string // "truncate" or "br"
   }
   ```

### B. Move shared logic

3. Move from catalog to `gensection/`:

  - `parse.go`: marker pair scanning, YAML body
     parsing, string param validation, column config
     parsing
  - Marker format detection (both old and new formats,
     depends on Plan 38)
  - `markerPair` and `markerScanState` types
  - `collectIgnoredLines`, `addBlockLineRange`,
     `addHTMLBlockLines`

4. Create `Engine` type that orchestrates Check/Fix
   using a registered `Directive`:

   ```go
   type Engine struct {
       directive Directive
   }

   func NewEngine(d Directive) *Engine

   func (e *Engine) Check(f *lint.File) []lint.Diagnostic
   func (e *Engine) Fix(f *lint.File) []byte
   ```

5. Move shared rendering utilities:

  - `ensureTrailingNewline`
  - `extractContent`
  - `replaceContent`
  - `splitLines`

### C. Refactor catalog rule

6. Reduce `internal/rules/catalog/rule.go` to a thin
   wrapper:

  - Embed or hold a `gensection.Engine`
  - Implement `gensection.Directive` interface
  - Delegate `Check()` and `Fix()` to the engine
  - Keep catalog-specific logic: glob resolution, entry
     building, sorting, rendering (minimal/template
     mode), column constraint application

7. Move `wrap.go` (column constraints) to stay in
   `internal/rules/catalog/` since it is
   catalog-specific.

### D. Tests

8. Add unit tests for `gensection.Engine` with a mock
   directive (simple test directive that returns
   static content).

9. Verify existing catalog tests still pass with the
   refactored code.

10. Run `go test ./...` and
    `go tool golangci-lint run`.

## Acceptance Criteria

- [ ] `internal/archetype/gensection/` package exists
      with `Directive` interface and `Engine`
- [ ] Marker parsing, YAML extraction, fix logic are
      in the archetype package
- [ ] `internal/rules/catalog/` implements `Directive`
      and delegates to `Engine`
- [ ] A new generated-section rule can be created by
      implementing `Directive` and calling `NewEngine`
- [ ] No behavioral changes
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues

## Dependencies

- Depends on Plan 37 (rename to catalog) being complete
- Depends on Plan 38 (simplified markers) for marker
  format abstraction
