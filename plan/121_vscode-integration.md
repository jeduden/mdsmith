---
id: 121
title: Expose mdsmith to VS Code via Language Server Protocol
status: "✅"
model: opus
summary: >-
  Ship an `mdsmith lsp` subcommand that speaks the
  Language Server Protocol over stdio plus a thin VS
  Code extension that spawns it, so Markdown
  diagnostics appear inline as the user types and
  `mdsmith fix` is exposed as code actions.
---
# Expose mdsmith to VS Code via Language Server Protocol

## Goal

Surface mdsmith diagnostics and auto-fixes inside VS
Code as the user edits a Markdown file, without
requiring the user to run `mdsmith check` from a
terminal. The same server speaks LSP, so any
LSP-aware editor (Neovim, Helix, JetBrains via the
LSP plugin) gets the same experience for free.

## Background

[docs/background/markdown-linters.md](../docs/background/markdown-linters.md)
flags VS Code support as a gap. Every peer linter
ships an extension. mdsmith does not. The
[CLI JSON schema](../docs/reference/cli.md) is
already labelled "stable for LSP consumption". Plan
95 designed `kinds resolve --json` and `check
--explain` for the same audience.

Two surfaces are needed: a server that speaks LSP
over stdio and reuses the lint pipeline, and a VS
Code extension that knows how to find the binary
and start it. The Node runtime never touches the
lint engine.

## Non-Goals

- A standalone Visual Studio (Windows IDE)
  extension — follow-up plan if demand surfaces.
- New rules or new fix logic. The plan wires the
  existing pipeline through LSP unchanged.
- Marketplace publication — gated on release
  planning (see Open Questions).

## Design

### Server: `mdsmith lsp`

A new subcommand `mdsmith lsp` runs an LSP server on
stdio. Implementation lives in `internal/lsp/` and
hand-rolls the JSON-RPC 2.0 framing plus the subset
of LSP message types the VS Code extension needs.
The server has no third-party dependencies beyond
the Go standard library; the `go.lsp.dev/*` packages
were considered and rejected once the surface area
turned out to be small enough that pulling in two
modules was net-negative.

LSP capabilities the server advertises:

| Capability                        | Behavior                                         |
|-----------------------------------|--------------------------------------------------|
| `textDocumentSync = Full`         | Re-lint on every change; debounced               |
| `publishDiagnostics`              | One push after each lint                         |
| `codeActionProvider`              | Per-diagnostic quick fixes (see below)           |
| `workspace.configuration`         | Pull `mdsmith.config`, `mdsmith.run`, trace      |
| `workspace.didChangeWatchedFiles` | Re-lint open buffers when `.mdsmith.yml` changes |

The server maps mdsmith JSON diagnostics to LSP
`Diagnostic`:

| mdsmith field    | LSP field                                         |
|------------------|---------------------------------------------------|
| `rule` + `name`  | `code` = rule (e.g. `MDS001`); `source = mdsmith` |
| `message`        | `message`                                         |
| `severity`       | `severity` (error → 1, warning → 2)               |
| `line`, `column` | `range.start`; end column derived per-rule        |
| `explanation`    | `data` (preserved for code-action handlers)       |

The server reuses
[`internal/engine`](../internal/engine) and
[`internal/lint`](../internal/lint) by feeding the
in-memory document text through the same pipeline
`check` uses, with a `Source` other than the file
on disk. This avoids forking the lint logic.

### Code actions

Two action kinds:

1. **Quick fix per diagnostic** —
   `quickfix`: each diagnostic that came from a
   fixable rule produces a `WorkspaceEdit` that
   applies just that rule's fix to the affected
   range. Runs the existing
   [`internal/fix`](../internal/fix) pass scoped to
   one diagnostic.
2. **Whole-file fix** —
   `source.fixAll.mdsmith`: runs `mdsmith fix` on
   the buffer and returns the diff as a
   `WorkspaceEdit`. This matches the contract VS
   Code's "Fix all" command expects, and lets users
   bind `editor.codeActionsOnSave` to it.

Rules whose fix touches multiple non-contiguous
ranges (e.g. `catalog`, `toc`) only surface as
whole-file actions; the per-diagnostic action is
omitted to avoid partial regenerations.

### Client: VS Code extension

Lives in a new top-level directory `editors/vscode/`
to keep the Go module clean. TypeScript, built and
tested with [Bun](https://bun.sh) (`bun run build.ts`
to bundle, `bun test` for the unit tests),
packaged with `vsce`. The entry point uses
Microsoft's `vscode-languageclient` package to speak
to `mdsmith lsp` over stdio.

Settings the extension contributes:

| Setting                | Default     | Purpose                               |
|------------------------|-------------|---------------------------------------|
| `mdsmith.path`         | `"mdsmith"` | Binary path; resolved against `$PATH` |
| `mdsmith.config`       | `""`        | Override `-c` config path             |
| `mdsmith.run`          | `"onSave"`  | `onType`, `onSave`, or `off`          |
| `mdsmith.fixOnSave`    | `false`     | Wires `source.fixAll.mdsmith` on save |
| `mdsmith.trace.server` | `"off"`     | LSP trace verbosity                   |

`mdsmith.path` is client-only. The extension reads
it to spawn the server. The server pulls only
`mdsmith.config`, `mdsmith.run`, and
`mdsmith.trace.server`. The `mdsmith.run` default
is `onSave`. Live linting per keystroke is opt-in.

Document selector: `markdown` and `*.markdown` files.
Activation event: `onLanguage:markdown`. The
extension does not bundle the binary; it surfaces a
clear error and a "Download mdsmith" link when the
binary is missing. The link points to the
[GitHub releases page](https://github.com/jeduden/mdsmith/releases).

### Configuration discovery

The server resolves `.mdsmith.yml` the same way the
CLI does (walk up from the document URI). When
`mdsmith.config` is set, that path wins. A
`workspace/didChangeWatchedFiles` subscription on
`**/.mdsmith.yml` triggers a re-lint of every open
buffer so config edits take effect immediately.

### Lifecycle and performance

- Per-document lint runs on the document goroutine
  and is debounced 200 ms after the last
  `didChange`.
- The server is single-process, multi-document. One
  client equals one server. No daemon mode.
- Memory budget: same `GOMEMLIMIT` (512 MB) the CLI
  sets in [`cmd/mdsmith/main.go`](../cmd/mdsmith/main.go).
- **Latency budget**: p95 squiggle-update under
  150 ms on a 1 000-line file and under 500 ms on a
  5 000-line file, measured end-to-end (`didChange`
  to `publishDiagnostics`). The benchmark drives
  the perf task; missing it blocks the default flip
  to `onType`.

### Distribution

CI packages the extension as a `.vsix` on each
release. The artifact ships next to the Go
binaries. Marketplace publication waits for a
publisher token. Users install with `code
--install-extension mdsmith-<version>.vsix` until
then.

## Tasks

1. Add `internal/lsp` package: server skeleton,
   stdio transport, capability advertisement,
   `initialize`/`shutdown` handlers. Unit-test the
   handshake against an in-memory jsonrpc2 pair.
2. Wire `textDocument/didOpen`, `didChange`, and
   `didClose`: maintain an in-memory document store
   keyed by URI, run the lint pipeline against the
   buffer text, publish diagnostics. Reuse
   [`internal/engine`](../internal/engine) by
   exposing a `LintBuffer(uri, text) []Diagnostic`
   entry point on
   [`internal/lint`](../internal/lint).
3. Add diagnostic-to-LSP mapping with end-column
   computation; round-trip test against
   `internal/output/json.go`'s shape so the two
   surfaces stay in sync.
4. Add `textDocument/codeAction` returning
   per-diagnostic quick fixes for rules whose fix
   is range-local. Add the
   `source.fixAll.mdsmith` whole-file action.
5. Add `workspace/didChangeWatchedFiles` for
   `**/.mdsmith.yml`; invalidate the cached config
   and re-lint open documents.
6. Register the `lsp` subcommand in
   [`cmd/mdsmith/main.go`](../cmd/mdsmith/main.go)
   and add it to `usageText`.
7. Add an end-to-end test in `cmd/mdsmith/`
   exercising `mdsmith lsp` over a pipe: send
   `initialize`, open a buffer with a known
   violation, assert the published diagnostic shape.
8. Create `editors/vscode/`: `package.json`,
   `tsconfig.json`, a Bun `build.ts` bundler, the
   `LanguageClient` bootstrap, the settings
   contributions, and a README that documents
   install / config.
9. Add a CI job that runs `bun install
   --frozen-lockfile && bun test && bun run
   build.ts --production && bunx @vscode/vsce
   package` in `editors/vscode/` and uploads the
   `.vsix` as a release artifact.
10. Update the Commands table in
    [docs/reference/cli.md](../docs/reference/cli.md)
    to include `lsp`. Update the integration table
    in
    [the linter comparison](../docs/background/markdown-linters.md)
    to flip "VS Code: no" to "yes". Add a new
    `docs/guides/editors/vscode.md` covering
    install, settings, and troubleshooting.
11. Add a benchmark
    `internal/lsp/bench_test.go` that measures
    end-to-end `didChange` →
    `publishDiagnostics` latency on synthetic 1k
    and 5k line documents. The benchmark uses
    `*testing.B`; wire the budgets above as
    `b.Fatalf` thresholds. Add a step to
    [`.github/workflows/`](../.github/workflows)
    that runs the benchmark explicitly
    (`go test -run=^$ -bench=. ./internal/lsp/...`);
    `go test ./...` does not invoke benchmarks by
    default. Document the invocation in
    `docs/guides/editors/vscode.md`.

## Acceptance Criteria

- [x] `mdsmith lsp` runs an LSP server on stdio and
      survives a full
      `initialize` → `didOpen` → `didChange` →
      `shutdown` round trip in an integration test.
- [x] Opening a Markdown file with a `MDS001`
      violation in VS Code shows the squiggle
      inline within 500 ms of save (manual smoke
      test documented in the new guide).
- [x] CI runs the latency benchmark and reports
      p95 latency under the 150 ms / 500 ms budgets
      on the 1k / 5k-line fixtures. Invocation:
      `go test -run=^$ -bench=. ./internal/lsp/...`
- [x] Quick-fix code actions appear for fixable
      rules and apply only the corresponding range;
      the file's other diagnostics are unaffected.
- [x] `source.fixAll.mdsmith` produces the same
      output as `mdsmith fix` on the same buffer
      (integration test compares the two).
- [x] Editing `.mdsmith.yml` re-lints open
      documents without restarting the editor.
- [x] `mdsmith lsp --help` documents the
      subcommand; `usageText` lists it.
- [x] `editors/vscode/` builds with `bun run
      build.ts --production` and packages with
      `bunx @vscode/vsce package` in CI; the
      `.vsix` is attached to release artifacts.
- [x] `docs/reference/cli.md` Commands table
      includes `lsp`;
      `docs/background/markdown-linters.md` no
      longer reports "VS Code: no".
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no
      issues.
- [x] `mdsmith check .` passes including the new
      docs and the updated `PLAN.md` catalog.

## Open Questions

- **Markdown docs under `editors/vscode/`.**
  Resolved: `editors/**` was added to
  `directory-structure.allowed` and the extension
  README was authored to pass the repo's existing
  rules as-is. No subtree-specific overrides were
  needed.
- **Marketplace publication.** Publishing requires
  an Azure DevOps publisher account and `VSCE_PAT`.
  Decide as part of release planning; this plan
  ships the `.vsix` only.
- **Visual Studio (Windows) parity.** Visual
  Studio supports LSP via the
  `Microsoft.VisualStudio.LanguageServer` package
  but needs a separate `.vsix` host. Track in a
  follow-up plan if user demand surfaces.
- **Notebook Markdown.** VS Code Markdown cells in
  `.ipynb` notebooks reach the language server
  through a different document URI scheme. Out of
  scope for v1; revisit if requested.

## ...

<?allow-empty-section?>
