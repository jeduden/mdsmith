---
date: "2026-06-09"
scope: "full repo — all surfaces"
method: "audit"
title: "mdsmith security audit — 2026-06-09"
summary: "Full-repo audit at 646f04a. One medium finding (LSP panic DoS, no recover in lint goroutines) and two informational findings. All §0 baseline defenses confirmed."
---
# mdsmith Security Review

- **Target:** jeduden/mdsmith @ `646f04a6fd4630aab79649577f24b5a49555e005`
- **Mode:** audit
- **Scope:** full repo — all surfaces
- **Date:** 2026-06-09

## Summary

Critical: 0 | High: 0 | Medium: 1 | Low: 0 | Info: 2

| ID   | Sev    | Conf      | Title                                                                                   | Surface   | Location                                     |
| ---- | ------ | --------- | --------------------------------------------------------------------------------------- | --------- | -------------------------------------------- |
| S001 | medium | confirmed | LSP lint goroutines have no panic recovery — hostile Markdown crashes the server        | lsp       | `internal/lsp/server_diagnostics.go:131-156` |
| S002 | info   | confirmed | cuetemplate.buildCUESource panics on json.Marshal failure instead of returning an error | directive | `internal/cuetemplate/cuetemplate.go:174`    |
| S003 | info   | confirmed | convention.go calls yaml.Unmarshal directly instead of yamlutil.UnmarshalNodeSafe       | directive | `internal/config/convention.go:206`          |

## Findings

### S001 · LSP lint goroutines have no panic recovery — hostile Markdown crashes the server

**Severity:** medium · **Confidence:** confirmed · **Surface:** lsp · **CWE-248**

**Location:** `internal/lsp/server_diagnostics.go:131-156`

- related: `internal/lsp/server_diagnostics.go:82`
- related: `internal/lsp/server_diagnostics.go:240`

**What.** The AfterFunc-based lint goroutine
(`runLintIfCurrent` → `runLint` → `sess.CheckVersion`)
carries no `recover()` wrapper anywhere in its call chain.
In Go, a panic in a goroutine without a recover kills the
entire process. The LSP server (`mdsmith lsp`) is a
long-lived process — one per editor session. If a rule,
the parser, or include/catalog resolution panics while
processing an attacker-controlled Markdown file (nil
dereference, index out of bounds, type assertion, etc.),
the server exits, all squiggles disappear, and VS Code
shows a language-server-crashed banner. The same gap
applies to `dispatchRaw` and `dispatch` — a panic in an
LSP message handler also crashes the main loop.

**Impact.** Denial of service. An attacker who can get
the victim to open a crafted Markdown file in VS Code
crashes the LSP server. If the file stays open, VS Code's
auto-restart loop makes the DoS persistent. No code
execution, no data exfiltration.

**Repro (sketch).** Author a Markdown file that triggers an
unguarded panic in any lint rule — for example, a rule that
nil-dereferences an AST node on an unusual parse tree.
Open the file in VS Code with the extension active.
The LSP server crashes and the Output Channel shows the
panic stack trace.

**Fix.** Wrap the body of `runLintIfCurrent` (or `runLint`)
in a deferred recover:

```go
defer func() {
    if r := recover(); r != nil {
        log.Printf("lint panic: %v", r)
    }
}()
```

Apply the same wrapper to `dispatchRaw`. This converts an
unrecoverable crash into a logged error. The server stays
up; that document's diagnostics are absent for that cycle.
Fix the root-cause panic separately.

## Hardening / Informational

### S002 · cuetemplate.buildCUESource panics on json.Marshal failure instead of returning an error

**Severity:** info · **Confidence:** confirmed · **Surface:** directive · **CWE-248**

**Location:** `internal/cuetemplate/cuetemplate.go:174`

**What.** The `buildCUESource` helper calls `json.Marshal(emit)`
and panics on error. It is called from
`cuetemplate.Template.Render` when evaluating a catalog
`row-expr:` directive. The `emit` map comes from the
Markdown file's front matter — attacker-controlled content.
`json.Marshal` failure on go-yaml v3 output is unlikely today
(all scalar types are JSON-marshallable), but any future type
that cannot be marshalled would make this reachable.
In the LSP context, this panic is not recovered (see S001)
and kills the server.

**Impact.** If triggered in the LSP: same DoS as S001.
If triggered via `mdsmith fix`: the CLI exits non-zero
and aborts the fix pass. No data is lost.

**Repro (sketch).** Not reachable with standard go-yaml v3 output.
Would become reachable if a front-matter field holds a type that
`json.Marshal` cannot serialise.

**Fix.** Replace the panic with an error return. Propagate the
error up through `buildCUESource` → `Render`. The call site
in catalog's `renderTemplate` already handles error returns —
so only the function signature changes.

### S003 · convention.go calls yaml.Unmarshal directly instead of yamlutil.UnmarshalNodeSafe

**Severity:** info · **Confidence:** confirmed · **Surface:** directive · **CWE-400**

**Location:** `internal/config/convention.go:206`

**What.** `parseConventionFileBody` calls `yaml.Unmarshal(data, &node)`
directly into a `yaml.Node` rather than routing through
`yamlutil.UnmarshalNodeSafe`. The in-code comment documents this
as intentional: go-yaml v3 does not expand aliases when
deserialising into a `yaml.Node`, so billion-laughs is not a
risk here. The concern is consistency. Every other user-YAML
entry point uses the safe wrappers. A future refactor that adds
a `.Decode()` call on the resulting node without re-checking
would reintroduce the alias-expansion risk.

**Impact.** No current exploitability. The inconsistency with the
project's safety convention creates a latent risk if the call
site evolves.

**Repro (sketch).** No repro needed; this is a hardening gap.

**Fix.** Replace `yaml.Unmarshal(data, &node)` with
`yamlutil.UnmarshalNodeSafe(data)`. The function already
imports `yamlutil` for the pre-check at line 205 — this is a
one-line change.

## Coverage

All seven threat-model surfaces were reviewed:
directive engine (include/catalog/build), CLI core and
workspace walk, LSP server, VS Code extension, Obsidian
plugin, distribution wrappers (npm, GitHub Actions), and
Git integration (merge-driver, pre-merge-commit).

The Obsidian plugin TypeScript source was not present in the
tree at this ref — that surface is inconclusive.

All §0 baseline defenses were confirmed to still hold.
Three findings were recorded: one confirmed Medium
(LSP panic DoS) and two informational (a cuetemplate
panic-vs-error and a YAML-safety inconsistency in
convention.go).
