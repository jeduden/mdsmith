---
date: "2026-07-03"
scope: "diff since the 2026-06-19 full-repo audit — directive engine, CLI/engine core, LSP, VS Code extension, CI/supply chain"
method: "audit"
title: "mdsmith post-audit diff review — 2026-07-03"
summary: "Diff review of ~250 commits merged since the 2026-06-19 full-repo audit. The os.OpenRoot symlink-containment fix and the new panic-recovery hardening both hold; no Critical/High/Medium findings. Two low findings, both fixed in plan/2607031500: Workspace.ReadFile bypassed the containment its sibling FS() view enforces (F001), and an unbounded block-quote recursion in the Layer-0 scanner could have stack-overflowed the process, though only behind an unwired internal spike flag (F002)."
---
# mdsmith Security Review

- **Target:** jeduden/mdsmith @ `bfa7803d4f1dc68cd4cdfe375e93b12748417ae6`
- **Mode:** audit
- **Scope:** Diff review of everything merged since the 2026-06-19 full-repo audit (base 22f59f731b252e4429ceb1da89b58acf4da68e0f), across the directive engine, CLI/engine core, LSP server, VS Code extension, and CI/supply chain
- **Date:** 2026-07-03

## Summary

Critical: 0 | High: 0 | Medium: 0 | Low: 2 | Info: 0

| ID   | Sev | Conf      | Title                                                                                                                                                                             | Surface   | Location                          |
| ---- | --- | --------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------- | --------------------------------- |
| F001 | low | confirmed | Workspace.ReadFile (OSWorkspace/OverlayWorkspace) reads through a caller-supplied path with no symlink containment, unlike the sibling FS() view                                  | directive | `pkg/mdsmith/workspace.go:72-74`  |
| F002 | low | confirmed | Unbounded block-quote recursion in the Layer-0 parse-skip scanner can trigger a fatal (unrecoverable) stack overflow, but the path is gated behind an unwired internal spike flag | cli       | `internal/lint/layer0.go:224-296` |

## Findings

### F001 · Workspace.ReadFile (OSWorkspace/OverlayWorkspace) reads through a caller-supplied path with no symlink containment, unlike the sibling FS() view

**Severity:** low · **Confidence:** confirmed · **Surface:** directive · **CWE-61**

**Location:** `pkg/mdsmith/workspace.go:72-74`

- related: `pkg/mdsmith/overlay.go:57-66`
- related: `pkg/mdsmith/session.go:430-436`
- related: `pkg/mdsmith/overlay.go:96-111`

**What.** OverlayWorkspace.FS() and OSWorkspace's FS() view were rewired in this window to build their fs.FS through lint.OpenRootFS (os.OpenRoot, RESOLVE_BENEATH semantics).
overlay_test.go:61-72 confirms a within-workspace symlink to an outside target is refused through FS().
But the sibling ReadFile methods (workspace.go:72-74, overlay.go:57-66) still call plain os.ReadFile against Join(Root, path), justified only by a `//nolint:gosec // path is caller-controlled` comment.
No OpenRoot, no containment check — a symlink is followed.
The two seams are documented as mirroring each other but only one enforces containment.

**Impact.** Session.frontMatterFor (session.go:431), used by the public Session.Kinds API, calls Workspace.ReadFile(uri) directly, bypassing FS().
If a hostile repo contains a within-workspace symlink (e.g. notes/evil.md -> /etc/passwd) and the CLI or an editor resolves kinds/front-matter for that exact path, the external file's bytes are read and parsed as front matter with no containment check.
This is narrower than the historical include/catalog escape: it needs the victim to act on the specific symlinked path, and the workspace walk's symlink-default-deny may keep it from being auto-discovered.
No exec or write is reachable — only a front-matter read/parse.

**Repro (sketch).** In a test repo: `ln -s /etc/passwd notes/evil.md`, add notes/evil.md nowhere else. Call `Session.Kinds("notes/evil.md")` (or run `mdsmith kinds resolve notes/evil.md` if the CLI accepts an explicit symlinked path argument) and observe that ReadFile follows the symlink and attempts to parse /etc/passwd as front matter, rather than being refused the way `fs.ReadFile(ws.FS(), "evil.md")` already is per overlay_test.go's containment test.

**Fix.** Route OSWorkspace.ReadFile and OverlayWorkspace.ReadFile through the same lint.OpenRootFS-backed containment as their FS() methods (e.g. implement ReadFile in terms of fs.ReadFile(w.FS(), relPath) when Root is set, or share a single resolved-and-contained open helper), so the two seams can no longer diverge. Add a test mirroring overlay_test.go's FS()-containment test but calling ReadFile directly, for both OSWorkspace and OverlayWorkspace.

### F002 · Unbounded block-quote recursion in the Layer-0 parse-skip scanner can trigger a fatal (unrecoverable) stack overflow, but the path is gated behind an unwired internal spike flag

**Severity:** low · **Confidence:** confirmed · **Surface:** cli · **CWE-674**

**Location:** `internal/lint/layer0.go:224-296`

- related: `internal/lint/layer0_para.go:204-209`
- related: `internal/engine/runner_layer0.go:45`
- related: `pkg/mdsmith/batch.go:134-140`

**What.** tryBlockquote (layer0.go:224) recurses into scanLayer0(body) once per block-quote nesting level whenever codeCapable is true (layer0.go:295-296).
codeCapable is set by lineHasNonFenceCode (layer0_para.go:204-209), which returns true for *any* nested block-quote marker line, not just actual code content.
A single line of N nested `>` markers drives N recursive calls with no depth cap.
Go cannot recover() a stack overflow — a fatal runtime error, not a panic — so this would bypass the panic-recovery hardening added elsewhere in this window (runner.go:397, checker.go:480).
scanLayer0/tryBlockquote only runs when f.AST is nil, which requires the MDSMITH_LAYER0_SKIP or MDSMITH_SPIKE_FLAT_L0 env vars (runner_layer0.go:45, batch.go:134-140) — both internal, undocumented spike flags with no CLI flag, config key, or LSP setting wired anywhere (confirmed by grep).
Default check/fix/lsp never sets these, so the path is unreachable today.

**Impact.** None under default configuration.
If either spike flag is ever promoted to a supported option (the code's own comments describe this as the planned direction), a single crafted line of deeply nested `>` markers would crash the whole mdsmith process or LSP server with a fatal, unrecoverable stack overflow — a session-wide DoS from just opening a hostile file.

**Repro (sketch).** With MDSMITH_LAYER0_SKIP=1 set, lint a file containing one line of a few hundred thousand `>` characters followed by text; scanLayer0 recurses once per level with no cap, eventually exhausting the goroutine stack.

**Fix.** Add a depth cap to tryBlockquote's recursion (mirroring the existing maxIncludeDepth pattern used for <?include?>) before either MDSMITH_LAYER0_SKIP or MDSMITH_SPIKE_FLAT_L0 is exposed as a supported, non-experimental path. Not urgent while both remain internal-only and unwired.

## Coverage

~250 commits since the last full audit were mapped to the seven threat-model surfaces by changed file, then each surface reviewed against references/threat-model.md.
Directive engine (include, catalog, rootfs.go, overlay.go, workspace*.go): the os.OpenRoot symlink containment introduced this window (closing prior S001/S002/S004/S006) holds for <?include?>, <?catalog glob:?>, and both workspaces' FS() views; one asymmetry found in the direct Workspace.ReadFile seam (F001, low).
CLI core / engine runner (runner*.go, layer0*.go, inline_scan.go, inline_blocks.go): per-file and per-goroutine panic recovery is newly added and test-covered; the new byte-level inline scanner is forward-only with no unbounded recursion; one unbounded block-quote recursion found, gated behind an unwired internal env-var spike flag (F002, low, unreachable via any public surface).
LSP server and VS Code extension: the server.go split and the Workspace Trust gating on kinds/rule-doc commands (S006/S007) both hold; no new child_process/exec sinks; no session-isolation regression from the unimplemented per-client-singleton plan.
CI / supply chain: merge-queue.yml's new label gate narrows rather than widens when the privileged token runs; dependency bumps (Go 1.25.11, x/net 0.56.0, x/tools, x/sys, wazero) are clean upstream CVE fixes with no replace directives.
Obsidian plugin and the Git merge-driver/pre-merge-commit hook had no changed files this window and were not re-reviewed.
