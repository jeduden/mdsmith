---
title: Architecture audit log
summary: >-
  Running log of SOLID and clean-architecture
  findings on origin/main. The
  solid-architecture skill (audit mode)
  appends here; blockers are also filed as
  plans.
audit-from: 41e61a5e08279bb37230cdb2e7ae33119b26077a
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

### resolved by plan/154

Rule packages imported other rule
packages.

Four rules imported
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
imported `internal/rules/tableformat`
for `FormatString`.

[plan/154](../../plan/154_arch-fix-rule-helper-extraction.md)
lifted the helpers into two sibling
packages:

- `internal/rules/fencepos` exports
  `CharAt`, `OpenLine`,
  `OpenLineRange`, `CloseLine`, and
  `CloseLineRange`.
- `internal/rules/tablefmt` exports
  `FormatString`. The donor also
  needs `Violations` and
  `FormatLines`; both are exported.

Both donors (`fencedcodestyle`,
`tableformat`) and the four consumers
now depend on these helpers. No rule
imports another rule.

`TestRulesDoNotImportEachOther` in
`internal/integration/` guards the new
boundary. It parses every non-test
`.go` file under `internal/rules/`. It
fails if a file imports another
`internal/rules/<...>` package other
than the documented helpers
(`astutil`, `settings`, `fencepos`,
`tablefmt`). A sub-package of the
file's own rule is also allowed. The
blank-import barrel package
`internal/rules/all/` is exempt by
design.

### resolved by plan/155

Config imports a rule package.

[`internal/config/convention.go`](../../internal/config/convention.go)
imported `internal/rules/markdownflavor`
to use `Convention`, `RulePreset`,
`ParseFlavor`, `Lookup`, and
`ConventionNames`.

[plan/155](../../plan/155_arch-fix-convention-config-ownership.md)
hoisted those shapes into a new
[internal/convention package](../../internal/convention/convention.go).
The markdownflavor rule now imports
`internal/convention` for the `Flavor`
type. The config package depends on
`internal/convention`, not on a rule.

`TestConfigDoesNotImportRules` guards
the new direction. It parses every
non-test file under `internal/config/`.
It fails if any import path contains
`internal/rules/`.

### resolved by plan/201

`internal/testutil` used an
anti-pattern name. The package
comment read "small helpers shared
across test binaries" — the canonical
`util` / `helpers` smell — but held a
single helper, `symlink.go`.

[plan/201](../../plan/201_arch-fix-testutil-rename.md)
renamed the package to
[internal/testsymlink](../../internal/testsymlink/).
The new name states the question the
one file answers. Five test files now
import the new path.

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

## Audit 2026-05-17 (range: 7464d273..b5a6d72)

Covered `internal/rename`, `internal/index`,
`mdsmith deps`, `mdsmith export`. Tax:
`nonNegativeUTF16RuneLen` copied privately in
three packages; export from `internal/mdtext` —
[plan/186](../../plan/186_arch-fix-utf16-centralize.md).

## Decision 2026-05-17 (plan/174)

### plan/153 non-goal superseded

Plan 153 kept the workspace symbol
index at `internal/lsp/index`. Its
stated non-goal: "only link/edge
extraction is in scope." Plan 174
supersedes that. The package is now
`internal/index`, a peer support
package.

The move is a pure `git mv`; no logic
changed. Two forces drove it.
`internal/schema` already imported the
index from outside `internal/lsp`. The
new `mdsmith rename` and `mdsmith deps`
surfaces need it too, and the layering
map forbids `cmd/mdsmith` →
`internal/lsp`. A peer package removes
the conflict. `internal/index` must
never import `internal/lsp`.

## Audit 2026-05-19 (range: 7464d273..41e61a5)

131 Go files. Plans 154, 155, 174 green.

### tax (2026-05-19)

- `server.go` (1 536) and `symbols.go` (1 385)
  exceed 1 000 lines — [plan/203][203].
- Five items from 2026-05-13 now scheduled:
  [hover][200], [testutil][201], [main.go][202],
  [fix→engine][204], [extension.ts][205].

[200]: ../../plan/200_arch-fix-hover-embed.md
[201]: ../../plan/201_arch-fix-testutil-rename.md
[202]: ../../plan/202_arch-fix-main-split.md
[203]: ../../plan/203_arch-fix-lsp-server-split.md
[204]: ../../plan/204_arch-fix-fix-engine-inversion.md
[205]: ../../plan/205_arch-fix-extension-ts-srp.md
[206]: ../../plan/206_arch-fix-cue-types-docs.md

### nice-to-have (2026-05-19)

`cue/types` not in layering map — [plan/206][206].
