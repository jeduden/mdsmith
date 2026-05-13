---
title: Architecture audit log
summary: >-
  Running log of SOLID and clean-architecture
  findings on origin/main. The
  solid-architecture skill (audit mode)
  appends here; blockers are also filed as
  plans.
audit-from: 7464d27310f722d9bfe203dc66b11bde714d4dd8
---
# Architecture audit log

This file is maintained by the
solid-architecture skill in audit mode.

## Audit 2026-05-13 (range: 6af677fb..7464d273)

Starting SHA
`6af677fb57e78e39415d42c6c31d9d3f2127e200`
was the oldest reachable commit on
`origin/main`. The repo history did not
extend a full month back from 2026-05-13.
The touched set covered 1107 files. Of
those, 425 were Go or TypeScript sources
outside fixture and generated paths.

### blockers

Rule packages import other rule packages.

Four rules import
`internal/rules/fencedcodestyle` for
fence-position helpers (`FenceCharAt`,
`FenceOpenLine`, `FenceOpenLineRange`,
`FenceCloseLine`,
`FenceCloseLineRange`):

- `internal/rules/fencedcodelanguage`
- `internal/rules/orderedlistnumbering`
- `internal/rules/unclosedcodeblock`
- `internal/rules/blanklinearoundfencedcode`

A fifth rule (`internal/rules/catalog`)
imports `internal/rules/tableformat` for
`FormatString`.

This violates DIP. The
[architecture hub](architecture/index.md)
states that a rule MUST NOT import
another rule package.

Severity: blocker.

Fix by lifting the helpers into sibling
helper packages (e.g.
`internal/rules/fencepos`,
`internal/rules/tablefmt`) shared by
donor and consumer rules. Scheduled by
[plan/154](../../plan/154_arch-fix-rule-helper-extraction.md).

Config imports a rule package.

[`internal/config/convention.go`](../../internal/config/convention.go)
imports `internal/rules/markdownflavor`.

The imported symbols are:

- `Convention`
- `RulePreset`
- `ParseFlavor`
- `Lookup`
- `ConventionNames`

This violates dependency direction. The
[layering map](architecture/index.md)
puts rules at the lowest layer. Config
is a mid-layer.

Severity: blocker.

Fix by hoisting the convention and flavor
data shapes into a layer config can own.
A new `internal/convention` package
works. The markdownflavor rule then
consumes those types instead of defining
them. Scheduled by
[plan/155](../../plan/155_arch-fix-convention-config-ownership.md).

### tax

`editors/vscode/src/extension.ts` is too
fat.

The file is 509 lines wide. Concerns it
owns today:

- LSP client lifecycle.
- A custom `ErrorHandler`.
- A config-file watcher.
- Fix-on-save wiring.
- `registerPaletteCommands`.
- The `mdsmith-kinds:` virtual-doc
  provider.

The
[TypeScript architecture doc](architecture/typescript.md)
calls out this gap. Target is "thin
entry; delegates to `wiring.ts`". This
violates SRP.

Severity: tax.

Fix by moving the LSP client lifecycle,
the watcher, the error handler, and the
command registrations into `wiring.ts`.
Dedicated modules under `commands/`
also work.

`internal/lsp/hover.go` imports from
`docs/`.

The
[hover.go file](../../internal/lsp/hover.go)
imports `docs/guides/directives`. That
is a Go package living inside the docs
tree.

The import is used as an `embed.FS` for
directive documentation served via
hover. This violates DIP and the
dependency direction rule. The layering
map has no `docs/` layer. A Go package
under `docs/` blurs the source vs.
documentation boundary.

Severity: tax.

Fix by moving the embed package to
`internal/directives`. Co-locating with
`internal/concepts` also works. That is
the established pattern for embedded
doc content.

`internal/fix` imports `internal/engine`.

The
[fix package](../../internal/fix/fix.go)
imports `internal/engine` for
`CheckRules`, `ConfigureRule`, and
`DedupeDiagnostics`. The
[layering map](architecture/index.md)
shows engine above fix. The actual
import graph is the reverse. Either the
doc layering needs to flip fix above
engine, or those three functions belong
in a lower shared package consumable by
both engine and fix. Severity: tax.

`internal/lint` answers too many
questions.

The
[lint package](../../internal/lint/)
mixes `File`/`Diagnostic` value types,
code block AST helpers, gitignore
matching, byte-limit guards,
processing-instruction parsing, YAML
safety, and front-matter extraction.
This violates SRP. Severity: tax. Fix
by splitting along question lines.
Keep `File`/`Diagnostic` in `lint`.
Move gitignore, limits, PI, and
yamlsafe into sibling packages each
named for their question.

`cmd/mdsmith/main.go` is too long.

The
[main.go entry](../../cmd/mdsmith/main.go)
is 1202 lines across 39 functions. Six
handlers exceed 50 lines (`runHelp`
81, `runFix` 71, `fixDiscovered` 68,
`runCheck` 62, `checkStdin` 61, `run`
57). The
[Go architecture doc](architecture/go.md)
states that a handler in `cmd/` longer
than ~50 lines is a smell. Severity:
tax. Fix by splitting the over-long
handlers into per-subcommand files.
The pattern is already used for
`kinds.go`, `metrics.go`,
`backlinks.go`, and `mergedriver.go`.

`internal/testutil` uses an
anti-pattern name.

The
[testutil package](../../internal/testutil/)
comment reads "small helpers shared
across test binaries". That is the
canonical `util` / `helpers`
anti-pattern. The architecture hub
flags it on sight. The current
contents are a single focused helper
(`symlink.go`). Severity: tax. Fix by
renaming to the question it answers
(e.g. `testsymlink`).

### nice-to-have

Spike binaries reach into rule
sub-packages.

Two files import
`internal/rules/concisenessscoring`
sub-packages directly:

- [`spike_gonative_classifier.go`][go-spike]
- [`spike_wasm_classifier.go`][wasm-spike]

Both are build-tag-gated. This is not a
production hazard. Fold into a shared
scoring port if and when these spikes
graduate.

[go-spike]: ../../cmd/mdsmith/spike_gonative_classifier.go
[wasm-spike]: ../../cmd/mdsmith/spike_wasm_classifier.go

Sub-package of a rule.

The
[`markdownflavor/ext` package](../../internal/rules/markdownflavor/ext/)
is used only within the parent rule
(`fix.go`, `parser.go`, `detect.go`).
This is fine as an internal split.
Worth a one-sentence package comment
explaining why it is separate so
future readers do not read it as a
separate rule package.
