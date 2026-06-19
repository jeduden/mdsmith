---
date: "2026-06-19"
scope: "full repo — all seven threat-model surfaces"
method: "audit"
title: "mdsmith full-repo security audit — 2026-06-19"
summary: "Full-repo audit at 22f59f7. Two high findings: <?include?> and <?catalog?> follow within-workspace symlinks to files outside the root via os.DirFS (fix: replace with os.OpenRoot). One medium: CLI engine-runner goroutines lack recover(), crashing the process on adversarial panic. Two low and two informational. All 2026-06-12 findings confirmed fixed."
---
# mdsmith Security Review

- **Target:** jeduden/mdsmith @ `22f59f731b252e4429ceb1da89b58acf4da68e0f`
- **Mode:** audit
- **Scope:** full repo — all seven threat-model surfaces
- **Date:** 2026-06-19

## Summary

Critical: 0 | High: 2 | Medium: 1 | Low: 2 | Info: 2

| ID   | Sev    | Conf      | Title                                                                                              | Surface   | Location                                 |
| ---- | ------ | --------- | -------------------------------------------------------------------------------------------------- | --------- | ---------------------------------------- |
| S001 | high   | confirmed | <?include?> directive follows within-workspace symlinks to files outside the workspace root        | directive | `internal/rules/include/rule.go:253-267` |
| S002 | high   | confirmed | <?catalog glob:?> follows within-workspace symlinks to files outside the workspace root            | directive | `internal/rules/catalog/rule.go:893-906` |
| S003 | medium | confirmed | CLI engine-runner goroutines lack per-goroutine recover() — rule panic crashes the process         | cli       | `internal/engine/runner.go:353-370`      |
| S004 | low    | confirmed | <?catalog glob:?> has no per-directive file-count cap — large repos can cause OOM                  | directive | `internal/rules/catalog/rule.go:888-920` |
| S005 | low    | confirmed | hasSymlinkAncestor skips ancestor scan when cwd is unresolvable and no .git root exists            | cli       | `internal/lint/files.go:244-246`         |
| S006 | info   | confirmed | <?include?> path validation does not explicitly reject URL schemes — incidentally safe but fragile | directive | `internal/rules/include/rule.go:130-170` |
| S007 | info   | confirmed | githooksync rule reads hook and gitattributes files with unbounded os.ReadFile                     | git       | `internal/rules/githooksync/rule.go:180` |

## Findings

### S001 · <?include?> directive follows within-workspace symlinks to files outside the workspace root

**Severity:** high · **Confidence:** confirmed · **Surface:** directive · **CWE-73**

**Location:** `internal/rules/include/rule.go:253-267`

- related: `internal/lint/file.go:286`
- related: `pkg/mdsmith/workspace.go:100`

**What.** resolveIncludePath (rule.go:253-267) normalizes the path with path.Clean and checks
strings.HasPrefix(resolvedFile, "..") to block dot-dot escapes. It then opens the file
through f.RootFS, set to os.DirFS(rootDir) at lint/file.go:286 and workspace.go:100.
os.DirFS in Go 1.25 follows symlinks — confirmed empirically: os.DirFS("/root").Open of
a symlink-to-outside successfully reads the external target (15 bytes returned).
There is no filepath.EvalSymlinks containment check. A within-workspace symlink whose
target is outside the root passes the dot-dot check and is opened by os.DirFS.
During mdsmith fix the content is embedded in the generated section;
during mdsmith check the rule reads the target to compare the body.

**Impact.** A hostile git repo contains a symlink (docs/sensitive.md -> /home/user/.ssh/id_rsa)
and a Markdown file with <?include file: docs/sensitive.md ?>.
When a victim clones and runs mdsmith fix, the SSH private key is read from outside
the workspace and embedded in the generated section. A subsequent push leaks the key.
mdsmith check also reads the target (to compare content) without writing it.

**Repro (sketch).** Create a git repo with: (1) docs/secret.md -> /etc/passwd (symlink),
(2) README.md containing <?include file: docs/secret.md ?>...<?/include?>.
Clone it. Run mdsmith fix README.md. The /etc/passwd content is embedded in README.md.

**Fix.** Replace os.DirFS(rootDir) with os.OpenRoot(rootDir) (Go 1.24+, available in Go 1.25).
os.OpenRoot enforces RESOLVE_BENEATH — confirmed to refuse symlinks escaping the root
(test: root.Open returns 'path escapes from parent'). Wire the *os.Root via root.FS()
as RootFS in internal/lint/file.go:286 and pkg/mdsmith/workspace.go:100-107.
Add an e2e test confirming <?include?> on a within-workspace symlink to an outside
target is refused.

### S002 · <?catalog glob:?> follows within-workspace symlinks to files outside the workspace root

**Severity:** high · **Confidence:** confirmed · **Surface:** directive · **CWE-73**

**Location:** `internal/rules/catalog/rule.go:893-906`

- related: `internal/lint/file.go:286`
- related: `pkg/mdsmith/workspace.go:100`

**What.** resolveGlobMatchesFrom (catalog/rule.go:873) calls doublestar.GlobWalk against
res.fs = f.RootFS = os.DirFS(rootDir). For symlink DirEntries the walk callback
calls fs.Stat(res.fs, m) at line 901 to get the target type, and includes the
path when the target is a regular file. Since res.fs is os.DirFS, fs.Stat follows
the symlink through the OS — same os.DirFS symlink-following confirmed for S001.
An in-workspace symlink matching the glob pattern causes readFrontMatter
to read the external target file.

**Impact.** A hostile repo with docs/leak.md -> /home/user/.aws/credentials and a catalog
directive matching docs/*.md causes the rule to read the external credentials file.
For non-Markdown files with no YAML front matter the result is empty (low impact);
for YAML-format config files (e.g. ~/.kube/config), fields could appear in the
generated catalog section.

**Repro (sketch).** Create a git repo with docs/leaked.md -> ~/.kube/config (symlink) and a Markdown
file containing <?catalog glob: docs/*.md ?>...<?/catalog?>.
Run mdsmith fix. The catalog reads docs/leaked.md through the symlink and attempts
to extract YAML front matter from ~/.kube/config.

**Fix.** Same fix as S001: replace os.DirFS with os.OpenRoot in all RootFS construction
sites. RESOLVE_BENEATH enforcement on os.OpenRoot.FS() also contains GlobWalk's
fs.Stat calls on symlinks, returning an error for targets outside the root.

### S003 · CLI engine-runner goroutines lack per-goroutine recover() — rule panic crashes the process

**Severity:** medium · **Confidence:** confirmed · **Surface:** cli · **CWE-390**

**Location:** `internal/engine/runner.go:353-370`

**What.** The parallel worker goroutines in runner.go:353-370 have no defer recover().
If a rule's Check or Fix method panics on attacker-controlled Markdown content,
the panic propagates to the Go runtime and crashes the entire mdsmith process.
The LSP server correctly contains panics via defer s.recoverPanic
(server_diagnostics.go:246), which covers RunSource calls that stay on the LSP's
calling goroutine. The CLI path with N>1 workers spawns goroutines outside any
recovery boundary; a panic on file i terminates the process immediately,
printing no diagnostics for the remaining files.

**Impact.** A hostile Markdown file that triggers a rule panic causes mdsmith check / fix
to crash with no output when run against a directory containing that file.
An attacker can place a panic-triggering file in a workspace to make linting
unreliable. This is a DoS of the CLI tool, not code execution.

**Repro (sketch).** Identify or craft Markdown that causes a rule's Check goroutine to panic
(e.g. a rule accessing AST nodes without nil guards on a crafted input).
Run mdsmith check . in a directory containing that file alongside many others.
The process crashes rather than reporting all diagnostics.

**Fix.** Wrap each worker goroutine body with a deferred recover that captures panics,
logs them, and sets the outcome for that file to an InternalError diagnostic
rather than terminating the process. Mirror the pattern in
internal/lsp/server.go recoverPanic.

### S004 · <?catalog glob:?> has no per-directive file-count cap — large repos can cause OOM

**Severity:** low · **Confidence:** confirmed · **Surface:** directive · **CWE-400**

**Location:** `internal/rules/catalog/rule.go:888-920`

**What.** resolveGlobMatchesFrom accumulates matched file paths with no upper-bound check.
A glob: "**/*.md" in a 100k-file repo causes the catalog rule to read front matter
from every matched file. Each file is bounded at 2 MB, but the count is uncapped.
The include rule has maxIncludeDepth = 10; no analogous maxCatalogMatches exists.
At 10 KB average size, 100k files = 1 GB of data plus per-file parse overhead.

**Impact.** On a very large workspace with a wildcard catalog directive, mdsmith fix/check
can run out of memory and be killed by the OS. This is a DoS on the linting pass.

**Repro (sketch).** Create a repo with 100,000 small .md files and a Markdown file containing `<?catalog glob: **/*.md ?>`. Run mdsmith check. Memory grows proportionally.

**Fix.** Add a maxCatalogMatches constant (e.g. 10,000) and return a diagnostic when
the match count exceeds it, analogous to maxIncludeDepth for the include rule.
Document the limit in docs/features/self-maintaining-sections.md.

### S005 · hasSymlinkAncestor skips ancestor scan when cwd is unresolvable and no .git root exists

**Severity:** low · **Confidence:** confirmed · **Surface:** cli · **CWE-61**

**Location:** `internal/lint/files.go:244-246`

**What.** hasSymlinkAncestor uses the nearest .git directory or cwd as its stop boundary
for the ancestor Lstat scan. When both os.Getwd() fails (empty) and
gitProjectRoot returns empty (no .git ancestor), the scan's boundary is empty
and the ancestor walk is skipped at lines 244-246.
In this environment, a path like symlinked-dir/file.md passes the ancestor scan;
the leaf Lstat check (line 148) still catches a symlinked leaf file, but a
symlinked intermediate directory component is not detected.
Triggering this requires a pathological environment (failed cwd, no .git) —
an extremely unlikely production scenario.

**Impact.** In the edge case (failed cwd + no .git), a symlinked directory component in
an explicit file path argument could slip through symlink default-deny.
The leaf Lstat still catches symlinked leaves. Combined with a crafted workspace,
this could allow linting a file through a symlinked directory.

**Repro (sketch).** Construct an environment where os.Getwd returns an error and there is no .git
ancestor directory. Pass mdsmith check symlinked-dir/real-file.md where
symlinked-dir is a symlink to an external directory. The file is processed.

**Fix.** When ancestorStopBoundary is empty, either refuse to scan (return an error)
or fall back to scanning up to the filesystem root.
The current silent skip is too permissive for a security boundary.

## Hardening / Informational

### S006 · <?include?> path validation does not explicitly reject URL schemes — incidentally safe but fragile

**Severity:** info · **Confidence:** confirmed · **Surface:** directive · **CWE-918**

**Location:** `internal/rules/include/rule.go:130-170`

**What.** validateIncludeDirective checks filepath.IsAbs(file) to block absolute paths.
On Linux, filepath.IsAbs("<http://example.com/foo">) returns false, so a URL-scheme
include path passes the absolute-path check. The path then reaches os.DirFS which
makes no network calls — so http:// paths resolve to a local path that fails to
open. The defense is incidental: os.DirFS's lack of network support, not an
explicit scheme check. If RootFS were replaced with a network-aware fs.FS
(e.g. a WASM host adapter that resolves URLs), the SSRF guard would be absent.

**Impact.** No impact in current code. Latent risk if the FS abstraction is extended
to support URL-scheme paths in WASM or plugin contexts.

**Repro (sketch).** N/A — currently safe due to os.DirFS not making network requests.

**Fix.** Add an explicit check for common URL scheme prefixes (<http://,> <https://,> file://)
in validateIncludeDirective, alongside the filepath.IsAbs check.

### S007 · githooksync rule reads hook and gitattributes files with unbounded os.ReadFile

**Severity:** info · **Confidence:** confirmed · **Surface:** git · **CWE-400**

**Location:** `internal/rules/githooksync/rule.go:180`

- related: `internal/rules/githooksync/rule.go:249`
- related: `internal/rules/githooksync/rule.go:295`
- related: `internal/rules/githooksync/rule.go:386`

**What.** The githooksync rule reads .git/hooks/pre-merge-commit, .git/hooks/post-merge,
and .gitattributes via os.ReadFile without a size cap. A hostile repository could
place a very large hook file to cause high memory allocation during check.
bytelimit.ReadFileLimited is used consistently elsewhere but not in this rule.

**Impact.** A .git/hooks/ file of e.g. 100 MB causes githooksync to allocate 100 MB per
lint pass. Combined with a large workspace, this could contribute to OOM.
Low practical severity because .git/ files are generally small.

**Repro (sketch).** Create a repo with a 100 MB .git/hooks/pre-merge-commit file.
Run mdsmith check. The githooksync rule reads the full file into memory.

**Fix.** Replace the four os.ReadFile calls with bytelimit.ReadFileLimited using a cap
(e.g. 1 MB, matching the config file cap). Return a diagnostic when exceeded.

## Coverage

All seven surfaces reviewed. Directive engine (include, catalog, build/MDS040, CUE, YAML):
§0 baselines hold for recipe execution, but include/catalog path resolution follows symlinks
via os.DirFS without EvalSymlinks containment check (S001, S002). CLI core and workspace
walk: symlink default-deny holds for walk and explicit-arg resolution; CLI engine-runner
goroutines lack per-goroutine recover() (S003); catalog glob file-count uncapped (S004).
LSP server: covered by today's separate 2026-06-19-lsp-vscode-audit; baselines confirmed.
VS Code extension: covered by 2026-06-19-lsp-vscode-audit. Obsidian plugin: WASM-based
engine, no binary spawn; configPath still routed to vault.adapter.read without ..
validation (carry-forward tentative finding, not newly confirmed). Distribution: all §0
supply-chain baselines hold — komac SHA256 fix confirmed, Actions pinned by SHA, no
pull_request_target with elevated privileges, no postinstall scripts, OIDC Trusted
Publishing, Sigstore/SLSA attestation present. Git integration: all §0 baselines hold;
guardFn and temp-then-rename applied to hook writes. All 2026-06-12 findings confirmed
fixed (komac checksum, MDS040 gate bypass, hook lstat, GOPATH fallback, rename symlink).
