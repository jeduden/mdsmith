---
title: Architecture audit log
summary: >-
  Running log of SOLID and clean-architecture
  findings on origin/main. The
  solid-architecture skill (audit mode)
  appends here; blockers are also filed as
  plans.
audit-from: 91c15ca9c3eca3f7277ac476efe59e8b8c681a84
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

### resolved by plan/154, plan/155, plan/201

[plan/154](../../plan/154_arch-fix-rule-helper-extraction.md):
Four rules imported `fencedcodestyle`. One
imported `tableformat`. Helpers lifted to
`internal/rules/fencepos` and
`internal/rules/tablefmt`.
`TestRulesDoNotImportEachOther` guards the
boundary.

[plan/155](../../plan/155_arch-fix-convention-config-ownership.md):
`internal/config` imported `markdownflavor`.
Convention types hoisted to
`internal/convention`.
`TestConfigDoesNotImportRules` guards.

[plan/201](../../plan/201_arch-fix-testutil-rename.md):
`internal/testutil` → `internal/testsymlink`.
Five test callers updated.

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

The `markdownflavor/ext` package is
used only within the parent rule
(`fix.go`, `parser.go`, `detect.go`).
Fine as an internal split, but worth
a package comment explaining why it is
separate. Resolved by
[plan/185](../../plan/185_public-markdown-flavor-library.md):
moved to `pkg/markdown/flavor/ext`.

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

## Audit 2026-05-24 (range: e8b3d04..91c15ca)

SHA `41e61a5e` (nominal audit-from) was
not in this local clone; checkout
started at `e8b3d04`. The effective
sweep covered 58 commits,
`e8b3d04` through `91c15ca`. Of the Go
and TypeScript files touched, 3 plans
from the 2026-05-19 audit closed (200,
201, 202); 4 remain open (203, 204,
205, 206).

### resolved since 2026-05-19

- **plan/200** ✅ — `docs/guides/directives`
  embed moved to `internal/directives`.
- **plan/201** ✅ — `internal/testutil`
  renamed to `internal/testsymlink`.
- **plan/202** ✅ — `cmd/mdsmith/main.go`
  split into per-subcommand files; now
  669 lines (was 1 306).

### open (carrying from 2026-05-19)

- **plan/203** (🔲) — server.go and
  symbols.go still unsplit. server.go
  now 1 867 lines (was 1 536 when
  first flagged); symbols.go 1 394.
- **plan/204** (🔲) —
  `internal/fix → internal/engine`
  inversion unresolved.
- **plan/205** (🔲) — `extension.ts`
  SRP refactor not started.
- **plan/206** (🔲) — `cue/types`
  missing from layering map.

### tax (2026-05-24)

`internal/lint` SRP never scheduled.

The 2026-05-13 audit flagged
`internal/lint` as mixing
`File`/`Diagnostic` value types with
code-block AST helpers, gitignore
matching, byte-limit guards, PI
parsing, and YAML safety. YAML safety
migrated to `internal/yamlutil` since
then. Three concerns remain: `gitignore.go`
(278 lines), `limits.go`
(`ReadFileLimited` is imported by 17+
files outside `internal/lint`), and
`pi.go` + `pi_parser.go`. No plan was
ever filed — [plan/221][221] schedules
the split.

[221]: ../../plan/221_arch-fix-lint-srp.md

### nice-to-have (2026-05-24)

`internal/punkt` not in layering map.

Commit `a1aa6c5` vendored a Punkt
sentence-tokenizer fork into
`internal/punkt`. Only
`internal/mdtext` imports it. The
package answers "segment text into
sentences". The SRP bullet list in
go.md lists the core packages but omits
this helper. [plan/222][222] adds it.

[222]: ../../plan/222_arch-fix-punkt-layering.md
