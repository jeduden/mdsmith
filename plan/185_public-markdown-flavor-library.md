---
id: 185
title: 'Expose extended-syntax parsers and the flavor model in pkg/markdown'
status: "🔳"
summary: >-
  Promote every custom goldmark parser
  (the five extensions) and the flavor
  support model into a public
  pkg/markdown/flavor sub-package, take
  detection off internal/lint, and retire
  the last two hand-rolled goldmark
  configs.
model: ""
depends-on: [163]
---
# Expose extended-syntax parsers and the flavor model in pkg/markdown

## Goal

Give external Go callers one public
surface for mdsmith's extended-syntax
parsing. Move every custom parser and
the feature detection into it. Stop
duplicating the goldmark config and the
flavor-support table outside
[pkg/markdown](../pkg/markdown).

## Context

Plan [163](163_public-markdown-library.md)
scoped [pkg/markdown](../pkg/markdown) to
CommonMark plus the `<?…?>` block. It
left extended syntax out. The
[library doc](../docs/development/markdown-library.md)
still says "CommonMark only". This plan
revises that scope. The doc edits are in
scope here, not a follow-up.

Three goldmark configs exist today. The
[architecture hub](../docs/development/architecture/index.md)
rejects a local `goldmark.New()` outside
[pkg/markdown](../pkg/markdown). That is
the drift 163 removed from
[internal/release](../internal/release):

- [pkg/markdown](../pkg/markdown)
  `NewParser()`: the canonical
  CommonMark and PI config.
- [internal/rules/markdownflavor](../internal/rules/markdownflavor)
  `parser.go`: a `goldmark.New()` with
  ten extensions and the PI block.
  Detection only.
- [internal/schema](../internal/schema)
  `validate_content.go`: a hand-rolled
  `goldmark.New()` with the table
  extension and the PI block.

An exhaustive scan finds every custom
goldmark parser. The markers are
`ast.NewNodeKind`, an `Extend` method,
and an `ASTTransformer`. Only two owners
match. The PI block parser already lives
in [pkg/markdown](../pkg/markdown); 163
moved it. The five extensions under
[pkg/markdown/flavor/ext](../pkg/markdown/flavor/ext)
are the rest, and no others exist.
Moving those five empties the tree of
custom parsers outside the public
package.

Boundary facts shape the design:

- The five extensions under
  [pkg/markdown/flavor/ext](../pkg/markdown/flavor/ext)
  import only goldmark. They have no
  `internal/` dependency.
- The `Feature` enum and the
  flavor-to-feature `support` table are
  pure data. They key on
  `convention.Flavor`.
- `detect.go` takes `*lint.File`. That
  couples detection to the linter core.
- [internal/convention](../internal/convention)
  owns the `Flavor` identity. It is a
  leaf data package.
- [pkg/markdown](../pkg/markdown) imports
  no `internal/` package. So the
  `Flavor` and `Feature` types must move
  down. The rule and
  [internal/convention](../internal/convention)
  then depend inward. The dependency
  never points back.

Behavior must not change. MDS034
diagnostics and `--fix` output stay
byte-identical.
[internal/schema](../internal/schema)
validation stays byte-identical too.
That is the guarantee 163 held for
sync-docs.

## Tasks

1. [x] Decide the package shape. Add a
   sub-package `pkg/markdown/flavor`. It
   holds the extensions, the `Feature`
   model, the support table, and
   `Detect`. It depends on
   [pkg/markdown](../pkg/markdown). Keep
   it out of the byte-stable core: the
   core answers "parse and produce"; the
   new package answers "which features
   does this document use, and which
   flavors accept them". Record the
   rejected flat layout.
2. [x] Move every custom goldmark parser
   into `pkg/markdown/flavor`. That is
   all five extensions: superscript,
   subscript, math block, inline math,
   and abbreviation. The abbreviation
   extension also registers an
   `ASTTransformer`; move it whole. Move
   their unit tests from
   [pkg/markdown/flavor/ext](../pkg/markdown/flavor/ext)
   too. They import only goldmark, so no
   dependency direction changes. The PI
   parser already sits in
   [pkg/markdown](../pkg/markdown) from
   163, so this leaves no custom parser
   outside the public package.
3. [x] Move the `Flavor` identity out of
   [internal/convention](../internal/convention).
   Move the `Feature` and `support`
   model out of the rule. Put both in
   `pkg/markdown/flavor`. Make
   [internal/convention](../internal/convention)
   alias them so
   [internal/config](../internal/config)
   is unchanged. The dependency points
   convention to flavor only.
4. [x] Build the public detector
   `Detect(doc *markdown.Document, accept
   func(Feature) bool) []Finding`. Define
   the public `Finding` and
   `HeadingIDExtra` shapes. Take a parsed
   document, not `*lint.File`. This keeps
   [pkg/markdown](../pkg/markdown) free of
   any [internal/lint](../internal/lint)
   import.
5. [x] Reduce
   [internal/rules/markdownflavor](../internal/rules/markdownflavor)
   to a rule adapter. It maps config and
   convention in. It maps `flavor.Finding`
   to diagnostics and fixes out. The
   `rule.Rule` and `rule.FixableRule`
   contract does not change.
6. [x] Migrate
   [internal/schema](../internal/schema)
   `validate_content.go` and the rule's
   `parser.go` onto the public
   constructor. Leave no `goldmark.New(`
   under `internal/`.
7. [x] Add a contract test that locks the
   `pkg/markdown/flavor` API shape. A new
   external surface ships with a contract
   test.
8. [x] Update the docs this scope
   contradicts. Fix the
   [library doc](../docs/development/markdown-library.md)
   "CommonMark only" text and its
   stable-surface list. Fix the
   [cross-system](../docs/development/architecture/cross-system.md)
   versioning bullet. Fix the
   [architecture hub](../docs/development/architecture/index.md)
   note. Run `mdsmith fix` to refresh
   catalogs.

## Acceptance Criteria

- [x] `pkg/markdown/flavor` exposes the
  five extension constructors, the
  `Flavor` and `Feature` model, and
  `Detect`. The compatibility policy
  documents it. Verifies ISP and OCP.
- [x] No package under
  [pkg/markdown](../pkg/markdown) imports
  `internal/`. The check
  `grep -r mdsmith/internal pkg/markdown`
  is empty. Verifies DIP.
- [x] One goldmark config remains, under
  [pkg/markdown](../pkg/markdown). No
  `goldmark.New(` exists under
  `internal/`.
- [x] No `ast.NewNodeKind`, goldmark
  `Extend` method, or custom
  `parser.BlockParser`,
  `parser.InlineParser`, or
  `ASTTransformer` exists under
  `internal/` or `cmd/`. Every custom
  parser lives in
  [pkg/markdown](../pkg/markdown).
  Verifies SRP and DIP.
- [x] [internal/convention](../internal/convention)
  and the rule define no own `Flavor` or
  `Feature` type. Both depend inward.
  Verifies DIP and SRP.
- [x] MDS034 diagnostics, `--fix`
  output, and
  [internal/schema](../internal/schema)
  validation are byte-identical before
  and after. Table tests pin this.
  Verifies Liskov.
- [x] Every moved or new function ships
  its dedicated unit test. A contract
  test locks the `pkg/markdown/flavor`
  shape. Verifies the test pyramid.
- [x] The
  [library doc](../docs/development/markdown-library.md),
  [cross-system](../docs/development/architecture/cross-system.md),
  and
  [architecture hub](../docs/development/architecture/index.md)
  no longer say "CommonMark only". The
  boundary and versioning entries cover
  the flavor surface.
- [x] All tests pass: `go test ./...`.
- [x] `go tool golangci-lint run`
  reports no issues.
- [x] `mdsmith check .` passes. The
  coverage gate holds.
