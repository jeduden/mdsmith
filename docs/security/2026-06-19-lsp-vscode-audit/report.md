---
date: "2026-06-19"
scope: "LSP server and VS Code extension"
method: "audit"
title: "LSP server and VS Code extension security audit"
summary: "Audit of the LSP server and VS Code extension surfaces. No critical, high, or medium findings. Two low-severity gaps in the VS Code Workspace Trust gating for read-only informational commands; five confirmed baseline defenses."
---
# mdsmith Security Review

- **Target:** jeduden/mdsmith @ `22f59f731b252e4429ceb1da89b58acf4da68e0f`
- **Mode:** audit
- **Scope:** LSP server (internal/lsp, cmd/mdsmith/lsp.go) and VS Code extension (editors/vscode)
- **Date:** 2026-06-19

## Summary

Critical: 0 | High: 0 | Medium: 0 | Low: 2 | Info: 5

| ID   | Sev  | Conf      | Title                                                                                                                                        | Surface | Location                                      |
| ---- | ---- | --------- | -------------------------------------------------------------------------------------------------------------------------------------------- | ------- | --------------------------------------------- |
| S006 | low  | confirmed | VS Code extension: mdsmith.kinds.resolve and mdsmith.kinds.why run in untrusted workspaces without a trust gate                              | vscode  | `editors/vscode/package.json:125-130`         |
| S007 | low  | confirmed | VS Code extension: mdsmith-rule: virtual document content provider runs without trust gate                                                   | vscode  | `editors/vscode/src/wiring.ts:906-910`        |
| S001 | info | confirmed | LSP: initialize/didOpen/didChangeWatchedFiles are diagnostics-only — no file writes or recipe execution                                      | lsp     | `internal/lsp/server_lifecycle.go:14-104`     |
| S002 | info | confirmed | LSP: fix-on-save (source.fixAll.mdsmith) returns WorkspaceEdit for open document only — no writes outside the buffer, no recipe execution    | lsp     | `internal/lsp/server_codeaction.go:183-215`   |
| S003 | info | confirmed | LSP: per-document panic is contained — does not crash the server                                                                             | lsp     | `internal/lsp/server.go:343-358`              |
| S004 | info | confirmed | VS Code extension: Workspace Trust baseline holds — untrustedWorkspaces:limited, mdsmith.path/config restricted, all mutating commands gated | vscode  | `editors/vscode/package.json:63-70`           |
| S005 | info | confirmed | VS Code extension: spawn uses argv array, no shell interpolation                                                                             | vscode  | `editors/vscode/src/commands/runner.ts:21-35` |

## Findings

### S006 · VS Code extension: mdsmith.kinds.resolve and mdsmith.kinds.why run in untrusted workspaces without a trust gate

**Severity:** low · **Confidence:** confirmed · **Surface:** vscode · **CWE-284**

**Location:** `editors/vscode/package.json:125-130`

- related: `editors/vscode/src/wiring.ts:829`
- related: `editors/vscode/src/wiring.ts:838`
- related: `editors/vscode/src/commands/virtual-doc.ts:72`

**What.** The commandPalette 'when' conditions for mdsmith.kinds.resolve and mdsmith.kinds.why omit
isWorkspaceTrusted (package.json:125-130). When invoked in an untrusted workspace, wiring.ts:829/838
calls runKindsResolve/runKindsWhy with no isTrusted() guard, spawning 'mdsmith kinds resolve/why
--json -- file' (virtual-doc.ts:72). The binary is the bundled binary (mdsmith.path is restricted
to default). The subcommands are read-only: they output JSON and exit; no files are written, no
recipes are executed. The binary reads .mdsmith.yml to derive kind assignments, so a hostile config
can inject attacker-controlled text into the virtual document pane but cannot execute code.

**Impact.** Low. In untrusted workspaces a hostile .mdsmith.yml causes mdsmith to output attacker-controlled text into the virtual document pane. No file writes or code execution. The virtual document is read-only in the editor. The binary cannot be redirected (mdsmith.path restricted).

**Repro (sketch).** Clone a hostile repo with a malformed .mdsmith.yml. Open a .md file. Run 'mdsmith: Explain Rule on This File' from the palette. The virtual document displays content derived from the hostile config.

**Fix.** Add 'isWorkspaceTrusted' to the commandPalette 'when' conditions for mdsmith.kinds.why and mdsmith.kinds.resolve, and add an isTrusted() guard at the top of runKindsResolve/runKindsWhy — matching the pattern used by fix-workspace, init, and merge-driver. These are informational commands so the restriction is minor friction.

### S007 · VS Code extension: mdsmith-rule: virtual document content provider runs without trust gate

**Severity:** low · **Confidence:** confirmed · **Surface:** vscode · **CWE-284**

**Location:** `editors/vscode/src/wiring.ts:906-910`

- related: `editors/vscode/src/commands/rule-doc.ts:160`
- related: `editors/vscode/src/commands/rule-doc.ts:35`

**What.** The mdsmith-rule: TextDocumentContentProvider (wiring.ts:906-910) runs 'mdsmith help rule
\<id>' to render an embedded rule README. It has no trust gate. The id is validated against
/^MDS\d+$/i (rule-doc.ts:35) before being passed as a CLI argument, so arbitrary arg injection is
prevented. The binary is the bundled binary (mdsmith.path restricted). 'mdsmith help rule' reads
READMEs embedded in the binary — no network call, no file writes. The `OPEN_RULE_DOC_COMMAND`
constant is triggered by rewritten hover links in MarkdownString blocks with isTrusted set only to
`{enabledCommands: ['OPEN_RULE_DOC_COMMAND']}` (rule-doc.ts:115), preventing command injection.
Risk is minimal but the pattern is inconsistent with the trust model.

**Impact.** Very low. The worst case is spawning the bundled mdsmith binary with a validated rule ID argument. The output is embedded README text displayed in a read-only virtual document.

**Repro (sketch).** Hover over a diagnostic in an untrusted workspace. Click the rewritten rule-docs link. 'mdsmith help rule MDS0XX' runs.

**Fix.** Low priority. If desired, add a trust check before spawning in provideRuleDocContent / fetchRuleDocContent. Alternatively, embed the rule READMEs directly in the extension's JS bundle to eliminate the spawn entirely for this read-only use case.

## Hardening / Informational

### S001 · LSP: initialize/didOpen/didChangeWatchedFiles are diagnostics-only — no file writes or recipe execution

**Severity:** info · **Confidence:** confirmed · **Surface:** lsp

**Location:** `internal/lsp/server_lifecycle.go:14-104`

- related: `internal/lsp/server_documents.go:16`
- related: `internal/lsp/server_documents.go:129`

**What.** handleInitialize (server_lifecycle.go:14) stores rootDir and capabilities only.
handleInitialized (server_lifecycle.go:77) calls reloadConfig() (reads config, never writes) and
optionally fetches client settings on a goroutine. handleDidOpen (server_documents.go:16) stores
the document and schedules a lint pass. handleDidChangeWatchedFiles (server_documents.go:129)
reloads config or invalidates caches. None writes files or executes recipes. The 2026-06-12 audit
confirmed exec.Command paths are CLI-only. Fix-on-save calls sess.Fix in-process and returns
WorkspaceEdit; the client applies the edit.

**Impact.** None — positive confirmation that the trust-on-open boundary holds.

**Repro (sketch).** N/A — not a defect.

**Fix.** No action required. The LSP fix-on-save path (S002 below) deserves a note: it returns WorkspaceEdit only for the open document, not for workspace-wide file writes.

### S002 · LSP: fix-on-save (source.fixAll.mdsmith) returns WorkspaceEdit for open document only — no writes outside the buffer, no recipe execution

**Severity:** info · **Confidence:** confirmed · **Surface:** lsp

**Location:** `internal/lsp/server_codeaction.go:183-215`

- related: `internal/lsp/server.go:548`

**What.** appendFixAllAction (server_codeaction.go:183) calls sess.Fix(relPath, doc.text) and wraps the result in a WorkspaceEdit covering only the URI of the open document. workspace/executeCommand is absent from the dispatch table (server.go:548-559); unknown methods return method-not-found per LSP spec. There is no server-side file write — the client applies the WorkspaceEdit. The prior audit (S004) confirmed sess.Fix does not import internal/build, so no recipe execution occurs. quickFixBytesFor (server_codeaction.go:232) similarly calls sess.FixRule for per-rule fixes.

**Impact.** None — positive confirmation.

**Repro (sketch).** N/A — not a defect.

**Fix.** No action required.

### S003 · LSP: per-document panic is contained — does not crash the server

**Severity:** info · **Confidence:** confirmed · **Surface:** lsp

**Location:** `internal/lsp/server.go:343-358`

- related: `internal/lsp/server_diagnostics.go:245`
- related: `internal/lsp/server.go:798`
- related: `internal/lsp/panic_recovery_test.go:23`

**What.** Three layers of panic containment: (1) dispatchRaw (server.go:343) defers recover() per
JSON-RPC frame — a panic in any handler is caught, logged via window/logMessage, and an
InternalError response settles the client's pending call; the dispatch loop continues. (2) runLint
(server_diagnostics.go:245) defers recoverPanic('lint '+uri) — a rule panic on attacker-controlled
content leaves the server running and publishes no diagnostics for that document. (3)
fetchClientSettings (server.go:798) defers recoverPanic('fetch client settings'). All three are
exercised by tests in panic_recovery_test.go (TestRunLintPanicIsRecovered,
TestDispatchRawPanicAllowsNextRequest, TestFetchClientSettingsPanicIsRecovered).

**Impact.** None — positive confirmation. An attacker-controlled Markdown file that causes a rule to panic cannot crash the LSP server or hang the editor.

**Repro (sketch).** N/A — not a defect.

**Fix.** No action required.

### S004 · VS Code extension: Workspace Trust baseline holds — untrustedWorkspaces:limited, mdsmith.path/config restricted, all mutating commands gated

**Severity:** info · **Confidence:** confirmed · **Surface:** vscode

**Location:** `editors/vscode/package.json:63-70`

- related: `editors/vscode/src/commands/fix-workspace.ts:62`
- related: `editors/vscode/src/commands/init.ts:23`
- related: `editors/vscode/src/commands/merge-driver.ts:26`
- related: `editors/vscode/package.json:113`

**What.** package.json declares capabilities.untrustedWorkspaces.supported='limited' with
restrictedConfigurations=['mdsmith.path','mdsmith.config'] (package.json:63-70). All three mutating
commands gate on isTrusted() at handler entry (fix-workspace.ts:62, init.ts:23, merge-driver.ts:26)
in addition to the package.json commandPalette 'when: isWorkspaceTrusted' conditions
(package.json:113-122). Since mdsmith.path is restricted, a workspace-supplied
.vscode/settings.json cannot redirect the binary in restricted mode — VS Code returns the default
value. The binary resolution in wiring.ts:487-489 reads the setting at server start, but gets the
default 'mdsmith' (bundled binary) for untrusted workspaces.

**Impact.** None — positive confirmation that the baseline defenses hold.

**Repro (sketch).** N/A — not a defect.

**Fix.** No action required.

### S005 · VS Code extension: spawn uses argv array, no shell interpolation

**Severity:** info · **Confidence:** confirmed · **Surface:** vscode

**Location:** `editors/vscode/src/commands/runner.ts:21-35`

**What.** defaultSpawn (runner.ts:21) calls nodeSpawn(binary, args, {cwd, stdio: ['ignore','pipe','pipe']}) with no shell option — it defaults to false. All binary invocations across fix-workspace.ts, init.ts, merge-driver.ts, kinds.ts, virtual-doc.ts, and rule-doc.ts pass a fixed string[] args array with -- separating options from file paths (kinds commands) or rule IDs validated against /^MDS\d+$/i (rule-doc). No shell interpolation, no exec() variant used.

**Impact.** None — positive confirmation.

**Repro (sketch).** N/A — not a defect.

**Fix.** No action required.

## Coverage

Reviewed: cmd/mdsmith/lsp.go, internal/lsp/server.go, internal/lsp/server_lifecycle.go,
internal/lsp/server_documents.go, internal/lsp/server_codeaction.go,
internal/lsp/server_diagnostics.go, internal/lsp/panic_recovery_test.go,
editors/vscode/package.json, editors/vscode/src/extension.ts, editors/vscode/src/wiring.ts,
editors/vscode/src/binary.ts, editors/vscode/src/commands/runner.ts,
editors/vscode/src/commands/fix-workspace.ts, editors/vscode/src/commands/init.ts,
editors/vscode/src/commands/merge-driver.ts, editors/vscode/src/commands/kinds.ts,
editors/vscode/src/commands/virtual-doc.ts, editors/vscode/src/commands/rule-doc.ts.
Cross-referenced the 2026-06-12-git-lsp-audit for exec.Command and build-pass findings.
Not covered in this audit: Obsidian plugin, distribution wrappers, CUE/YAML evaluation,
workspace-walk symlink deny, rename write-safety, ReDoS in rules.
