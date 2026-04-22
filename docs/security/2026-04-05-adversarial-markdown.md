---
date: "2026-04-05"
scope: >-
  Adversarial markdown input causing unintended
  side effects on the host machine
method: >-
  5 parallel blind-review agents, each targeting
  a different attack surface
title: Adversarial Markdown Input
summary: >-
  10 findings (1 high, 1 medium-high, 8 medium) covering
  OOM, YAML bomb, ANSI injection, symlinks, path traversal,
  content injection, CUE injection, schema path, TOCTOU,
  and include size. Cross-linter comparison with top 5 tools.
---
# Security Review: Adversarial Markdown Input

---

## Critical Findings Summary

| #   | Finding                                          | Severity    | Attack Vector                               | File(s)                                                                                                          |
|-----|--------------------------------------------------|-------------|---------------------------------------------|------------------------------------------------------------------------------------------------------------------|
| 1   | No file-size limit — OOM via large input         | High        | Any `.md` arg or stdin                      | `internal/engine/runner.go:52`, `internal/fix/fix.go:84`, `cmd/mdsmith/main.go:577`                              |
| 2   | YAML billion-laughs via anchor expansion         | Medium-High | Front matter in any scanned `.md`           | `internal/config/load.go:22`, `internal/archetype/gensection/parse.go:189`, `internal/rules/catalog/rule.go:415` |
| 3   | ANSI escape injection in terminal output         | Medium      | Heading/source content in any `.md`         | `internal/output/text.go:23,78`                                                                                  |
| 4   | Symlinks followed by default (read+write)        | Medium      | Symlink in repo, `fix` overwrites target    | `internal/lint/files.go:190`, `internal/fix/fix.go:84-122`                                                       |
| 5   | Path traversal in cross-file-reference-integrity | Medium      | `[link](../../../etc/secret.md#h)` in `.md` | `internal/rules/crossfilereferenceintegrity/rule.go:258`                                                         |
| 6   | Catalog front-matter Markdown injection          | Medium      | Front matter values with `](`...`)`         | `internal/fieldinterp/fieldinterp.go`, `internal/rules/catalog/rule.go`                                          |
| 7   | CUE expression injection via schema values       | Medium      | Schema `.md` front matter strings           | `internal/rules/requiredstructure/rule.go:285-290`                                                               |
| 8   | Unvalidated schema path (arbitrary read)         | Medium      | `.mdsmith.yml` `schema:` setting            | `internal/rules/requiredstructure/rule.go:82`                                                                    |
| 9   | Non-atomic write in fix mode (TOCTOU)            | Low-Medium  | Symlink swap between read and write         | `internal/fix/fix.go:84-122`                                                                                     |
| 10  | No size limit on included files                  | Medium      | `<?include file: "huge.md"?>`               | `internal/rules/include/rule.go:194`                                                                             |

---

## Detailed Findings

### 1. HIGH — No File-Size Limit (OOM)

**Locations:**

- `internal/engine/runner.go:52` — `os.ReadFile(path)` with no size check
- `cmd/mdsmith/main.go:577` — `io.ReadAll(os.Stdin)` with no limit
- `internal/fix/fix.go:84` — `os.ReadFile(path)` in fix path

The entire file is loaded into memory before any rule fires. The `max-file-length` rule (MDS022) only emits a diagnostic *after* loading — it does not prevent the load. `lint.NewFileFromSource` then calls `bytes.Split`, duplicating the allocation.

**Adversarial file:** A multi-GB `.md` file (or one piped via stdin) triggers OOM. In CI, this kills the linting job or exhausts container memory.

**Recommendation:** Add an `io.LimitReader` / `os.Stat` size guard before `os.ReadFile`. A sensible default (e.g., 10 MB) with a `--max-input-size` override would suffice.

---

### 2. MEDIUM-HIGH — YAML Billion-Laughs via Anchor Expansion

**Library:** `gopkg.in/yaml.v3 v3.0.1` — no alias-expansion depth or size limit.

**Locations:** Every `yaml.Unmarshal` call:

- `internal/config/load.go:22` — config file
- `internal/archetype/gensection/parse.go:189` — directive YAML body
- `internal/rules/catalog/rule.go:415` — per-file front matter
- `internal/rules/requiredstructure/rule.go:220,233` — schema front matter
- `cmd/mdsmith/main.go:352` — `query` subcommand

**Adversarial file:** Any `.md` with exponentially-nested YAML anchors in its front matter:

```yaml
---
a: &a ["x","x","x","x","x","x","x","x","x","x"]
b: &b [*a,*a,*a,*a,*a,*a,*a,*a,*a,*a]
c: &c [*b,*b,*b,*b,*b,*b,*b,*b,*b,*b]
d: &d [*c,*c,*c,*c,*c,*c,*c,*c,*c,*c]
---
```

This expands to 10^4 = 10,000 elements (4 levels). With 8 levels: 10^8 = 100 million strings. OOM or CPU exhaustion.

**Recommendation:** Use `yaml.NewDecoder` with a size-limited reader, or pre-check front-matter byte length before unmarshalling.

---

### 3. MEDIUM — ANSI Escape Injection in Terminal Output

**Location:** `internal/output/text.go:23-26, 78-80`

Diagnostic messages and source-line snippets embed raw user-controlled content via `%s` with no sanitization:

```go
fmt.Fprintf(w, "...%s\n", d.Message)   // line 23-26
fmt.Fprintf(w, format, ..., line)       // line 78 (source snippet)
```

`d.Message` often includes verbatim heading text. `line` is a raw source line from the file.

**Adversarial file:**

```markdown
## Title\033[2J\033]0;pwned\007
```

Escape sequences pass through to the terminal: screen clearing (`\033[2J`), window-title hijacking (`\033]0;...`), OSC hyperlink injection. Even with `--no-color`, source snippets are still unescaped.

**Recommendation:** Strip or replace `\x1b` bytes in all user-controlled strings before writing to the terminal.

---

### 4. MEDIUM — Symlinks Followed by Default (Read + Write)

**Locations (after plan 84):**

- `internal/lint/files.go` — `resolveArg`, `resolveGlob`, and
  `walkDir` Lstat each path entry and skip symbolic links unless
  `FollowSymlinks` opts in. `hasSymlinkAncestor` also rejects any
  path whose relative ancestors include a symlinked directory, so
  `linked/dirty.md` and `linked/*.md` cannot reach external targets.
- `internal/discovery/discovery.go` — the discovery walker applies
  the same Lstat-based skip during directory traversal.
- `internal/fix/fix.go` — atomic write via `os.Rename(tmp, path)`
  replaces the symlink entry itself rather than following it to the
  target (plan 83 write-side protection).

**Original finding:** symlink following was the default; the `--no-follow-symlinks` flag and config key were opt-in.

**Status (plan 84, resolved):** the default is inverted. Symlinks are skipped by default across directory walks, glob expansion, and explicit non-glob path arguments. Symlinked directories are always skipped — including when a path or glob traverses through one — regardless of `FollowSymlinks`. Users opt in (for file symlinks) with `--follow-symlinks` or `follow-symlinks: true`. The old `--no-follow-symlinks` flag has been removed; the legacy `no-follow-symlinks:` config key still parses and emits a deprecation warning.

**Adversarial file:**

```bash
# Attacker places in repo:
ln -s /etc/cron.d/jobs evil.md
# CI runs: mdsmith fix .
# Result: /etc/cron.d/jobs is overwritten with "fixed" markdown content
```

**Recommendation:** Default to `O_NOFOLLOW` semantics for write operations, or at minimum `os.Lstat` before write to detect symlinks.

---

### 5. MEDIUM — Path Traversal in cross-file-reference-integrity

**Location:** `internal/rules/crossfilereferenceintegrity/rule.go:258-269`

```go
func resolveTargetOSPath(sourcePath, linkPath string) (string, bool) {
    return filepath.Clean(filepath.Join(filepath.Dir(sourcePath), linkPath)), true
}
```

No project-root boundary check. A link with `../../../` resolves to an absolute path and `os.ReadFile` is called on it.

**Adversarial file:**

```markdown
[notes](../../../../home/user/private-notes.md#secret-heading)
```

Causes `os.ReadFile("/home/user/private-notes.md")`. Content is parsed for heading anchors — existence of specific headings leaks as lint diagnostics.

By default only `.md`/`.markdown` targets are followed; with `strict: true` all targets get `os.Stat`.

**Recommendation:** Compare resolved path against a project-root boundary and reject traversals above it.

---

### 6. MEDIUM — Catalog Front-Matter Markdown Injection

**Locations:**

- `internal/fieldinterp/fieldinterp.go` — `Interpolate()` inserts values as plain strings
- `internal/rules/catalog/rule.go` — row template rendering

Front-matter values are interpolated into Markdown templates without escaping.

**Adversarial file:** A `.md` with crafted front matter:

```yaml
---
summary: "](evil.com) [click me"
---
```

Template `- [{summary}]({filename})` produces:

```markdown
- [](evil.com) [click me](path.md)
```

This injects arbitrary links into the generated catalog section. Newlines in values can escape the row and inject additional Markdown structure.

**Recommendation:** Escape `[`, `]`, `(`, `)`, and newlines in interpolated field values.

---

### 7. MEDIUM — CUE Expression Injection via Schema Front Matter

**Location:** `internal/rules/requiredstructure/rule.go:285-290`

Schema YAML string values are embedded verbatim into CUE source that is compiled and evaluated:

```go
case string:
    expr := strings.TrimSpace(x)
    return expr, nil  // raw CUE expression — no sanitization
```

This flows to `ctx.CompileString(schema)`. An attacker controlling the schema file can inject closing braces to escape `close({...})` and inject arbitrary CUE constructs — potentially bypassing all validation or causing DoS via expensive CUE evaluation.

**Recommendation:** Quote string values as CUE string literals rather than embedding them as raw expressions, or validate that values parse as a single CUE expression.

---

### 8. MEDIUM — Unvalidated Schema Path (Arbitrary File Read)

**Location:** `internal/rules/requiredstructure/rule.go:82`

```go
schData, err := os.ReadFile(r.Schema)
```

The `schema` setting from `.mdsmith.yml` is passed directly to `os.ReadFile` — absolute paths and `../` traversals are accepted. Content is parsed as Markdown; heading text appears in diagnostic messages.

**Adversarial config:** `schema: /etc/passwd` — mdsmith reads the file and leaks partial content in error messages.

**Recommendation:** Reject absolute paths and `..` segments in the schema setting, or resolve relative to config-file directory with boundary check.

---

### 9. LOW-MEDIUM — Non-Atomic Write in Fix Mode (TOCTOU)

**Location:** `internal/fix/fix.go:84-122`

The fix pipeline: `ReadFile` → process → `WriteFile` to the same path. No locking, no atomic rename. A symlink can be swapped between read and write.

**Recommendation:** Write to a temp file in the same directory, then `os.Rename` into place.

---

### 10. MEDIUM — No Size Limit on Included Files

**Location:** `internal/rules/include/rule.go:194`

`fs.ReadFile(readFS, readPath)` reads the full included file with no size guard. The include depth limit is 10, so 10 large files can be loaded.

**Recommendation:** Apply the same size limit as recommended for primary input files.

---

## Non-Findings (Defenses That Work)

| Area                               | Status                                                                                   |
|------------------------------------|------------------------------------------------------------------------------------------|
| **Command injection**              | No `os/exec` calls use directive parameters. All exec uses hardcoded subcommands.        |
| **Go template injection**          | No `text/template` or `html/template` usage. Custom `{field}` interpolation only.        |
| **ReDoS**                          | Go's `regexp` uses RE2 (linear time). Not exploitable.                                   |
| **Include path traversal**         | Absolute paths blocked, `..` segments rejected, `os.DirFS` boundary enforced.            |
| **Catalog glob traversal**         | Absolute paths and `..` rejected; `doublestar.Glob` operates within `os.DirFS`.          |
| **Circular includes**              | Visited-set cycle detection + `maxIncludeDepth = 10`.                                    |
| **Infinite loops**                 | All loops terminate on finite input; PI parser returns `Close` at EOF.                   |
| **Environment variable expansion** | Not present anywhere in config or directives.                                            |
| **Supply chain**                   | No known-vulnerable runtime dependencies. Large indirect dep set is from dev-only tools. |

---

## Threat Model: CI Linting Untrusted PRs

The highest-risk scenario is a CI pipeline running `mdsmith check .` or `mdsmith fix .` on untrusted pull requests. An attacker contributing a PR can craft `.md` files that:

1. **OOM the CI runner** (findings 1, 2, 10)
2. **Read files outside the repo** via cross-file-reference links (finding 5)
3. **Inject terminal escapes** into CI logs (finding 3)
4. **Inject malicious links** into catalog-generated sections if `fix` is run (finding 6)
5. **Overwrite files via symlinks** if `fix` is run (finding 4)

The attacker does NOT need to control `.mdsmith.yml` for findings 1-5 — only the `.md` files in the PR.

---

## Revised Mitigation Recommendations

Each proposed mitigation was blind-reviewed by an independent agent for
correctness, completeness, hidden tradeoffs, and whether a better
alternative exists. Below are the final recommendations.

### M1. File-Size Limit (Finding 1) — REVISED

**Original proposal:** `os.Stat` size guard before `os.ReadFile` at 3 call sites.

**Problems identified:**

- `os.Stat` introduces TOCTOU — file can grow between stat and read.
- Only 3 of ~15 production `os.ReadFile`/`io.ReadAll`/`fs.ReadFile`
  sites were covered. Missed sites include `metrics/rank.go`,
  `mergedriver.go`, `crossfilereferenceintegrity/rule.go`,
  `requiredstructure/rule.go`, and `catalog/rule.go`.
- 10 MB default is generous; 1–2 MB is equally safe for Markdown.

**Revised recommendation:** Create a shared `readFileLimited(path string,
max int64) ([]byte, error)` helper using `os.Open` + `io.LimitReader(f,
max+1)` + post-read length assertion. Apply uniformly across all ~15
production call sites. Default limit 2 MB, overridable via
`--max-input-size`. ~30 lines of new code for complete coverage.

**Verdict:** Fix is correct in concept but under-scoped. The shared
helper approach is strictly better.

---

### M2. YAML Billion-Laughs (Finding 2) — REJECTED, REPLACED

**Original proposal:** Cap front-matter byte length at 64 KB before
`yaml.Unmarshal`.

**Problems identified:**

- **Byte-length limiting does not prevent the attack.** A 1 KB YAML with
  8 levels of nested aliases expands to 10^8 strings. The attack uses
  small input that expands exponentially — capping input bytes misses the
  point entirely.
- `gopkg.in/yaml.v3` has no alias-depth or expansion-size controls.
- Only 2 of 13 `yaml.Unmarshal` call sites were covered.

**Revised recommendation (pick one):**

1. **Pre-scan for anchors/aliases** (simplest): Before any
   `yaml.Unmarshal` on user-supplied content, reject input containing
   YAML anchor (`&`) or alias (`*`) syntax. Legitimate Markdown front
   matter virtually never uses YAML anchors. One-line check, zero false
   positives for this tool's use case.
2. **Switch to `github.com/goccy/go-yaml`**: Provides
   `yaml.WithMaxAliasesNum()` and `yaml.WithMaxLiteralStringSize()`,
   eliminating the problem structurally.

Apply to all 13 `yaml.Unmarshal` sites that process user-supplied `.md`
content, not just 2.

**Verdict:** Original fix is ineffective. Pre-scan for `&`/`*` is the
minimum viable fix.

---

### M3. ANSI Escape Injection (Finding 3) — REVISED

**Original proposal:** Replace `\x1b` with `\u241b` in
`sanitizeTerminal()`.

**Problems identified:**

- `\x1b` is not the only injection vector. `\x9b` (C1 CSI) is a
  single-byte CSI recognized by most terminals. The full C1 range
  (0x80–0x9F) includes `\x9d` (OSC), `\x9c` (ST). Also `\x07` (BEL)
  and `\x08` (backspace) can produce misleading output.
- `\u241b` may display as `?` on non-Unicode terminals.
- Output-layer sanitization (in `text.go`) is the correct layer — it
  preserves raw data for JSON consumers.
- `JSONFormatter` is safe (Go's `encoding/json` escapes control chars).

**Revised recommendation:** Use `strings.Map` to strip all control
bytes: 0x00–0x08, 0x0B–0x0C, 0x0E–0x1F, 0x7F, and 0x80–0x9F. Preserve
only `\t` (0x09), `\n` (0x0A), `\r` (0x0D). Apply in `TextFormatter`
only (not `JSONFormatter`). No external dependency needed.

```go
func sanitizeTerminal(s string) string {
    return strings.Map(func(r rune) rune {
        if r == '\t' || r == '\n' || r == '\r' {
            return r
        }
        if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
            return -1 // strip
        }
        return r
    }, s)
}
```

**Verdict:** Right approach, incomplete character set. The
`strings.Map` version is more robust with similar complexity.

---

### M4. Symlink Default (Finding 4) — REVISED

**Original proposal:** (a) Invert default to reject symlinks in
`walkDir`, (b) `os.Lstat` before write in `fix.go`.

**Problems identified:**

- `os.Lstat` before write has its own TOCTOU — symlink created between
  Lstat and WriteFile. Atomic rename (M9) is the proper write-side fix.
- Default-deny should apply to `check` too, not just `fix` — reading
  `/etc/passwd` via symlink in CI is also a risk.
- 5 other read paths follow symlinks and are unguarded: include
  directive, catalog glob, cross-file-reference-integrity,
  `resolveGlob`, all via `os.DirFS` which follows symlinks.
- `filepath.WalkDir` (Go 1.16+) with `d.Type()` is more efficient than
  `filepath.Walk` + extra `Lstat`.

**Revised recommendation:**

1. Default-deny symlinks in `walkDir` for both `check` and `fix`
   (breaking change — document in release notes). Add `--follow-symlinks`
   opt-in flag.
2. Drop the `Lstat`-before-write approach entirely; rely on atomic
   rename from M9 instead (`os.Rename` replaces the symlink itself, not
   the target).
3. Consider migrating to `filepath.WalkDir` for a cleaner symlink check
   without extra syscalls.

**Verdict:** Write-side fix should be atomic rename (M9), not Lstat.
Read-side default-deny is the correct primary fix.

---

### M5. Path Traversal Boundary (Finding 5) — REVISED

**Original proposal:** `strings.HasPrefix(absResolved, absRoot+sep)` in
`resolveTargetOSPath`.

**Problems identified:**

- `strings.HasPrefix` with `absRoot+sep` is fragile: trailing slashes
  on root (Windows `C:\`), case sensitivity (macOS HFS+), and symlink
  resolution of root itself all cause bypasses.
- `lint.File` carries `FS`/`RootFS` (`fs.FS` values) but not a
  `RootDir` string — plumbing is needed.
- Boundary check must happen before `os.Stat`, not just before
  `os.ReadFile`, to prevent existence-leak in `strict: true` mode.

**Revised recommendation:**

1. Add `RootDir string` field to `lint.File`, populated in `runner.Run`
   alongside `RootFS`.
2. Use `filepath.Rel(absRoot, absResolved)` + check for leading `..` —
   idiomatic Go, handles all separator edge cases.
3. Move boundary enforcement into `resolveTargetFile` (before `os.Stat`)
   not just `resolveTargetOSPath`.
4. When `RootDir` is empty (no config), skip the OS-path branch and fall
   through to the `fs.FS`-based path (already root-bounded).

**Verdict:** `filepath.Rel` is strictly better than `HasPrefix`. ~10
lines of correct code.

---

### M6. Catalog Markdown Injection (Finding 6) — REJECTED, REPLACED

**Original proposal:** `escapeMarkdownInline()` in
`fieldinterp.Interpolate` escaping `[`, `]`, `(`, `)`.

**Problems identified:**

- **Wrong layer.** `fieldinterp.Interpolate` is a general-purpose
  utility used by include, schema, and other non-Markdown contexts.
  Escaping there would corrupt those paths.
- **Breaks existing usage.** The project's own `CLAUDE.md` catalog uses
  `{summary}` values containing Markdown-formatted text with brackets.
  Silent escaping would produce `\[Build commands\]` — visibly broken.
- Incomplete character set: misses `` ` ``, `<`, `>`, `!`, `*`, `_`.
- Newline → space is wrong for YAML block scalars with meaningful
  structure.

**Revised recommendation:** Emit a **lint diagnostic** (not silent
escaping) when a front-matter value interpolated into a catalog `row`
template contains suspicious characters: embedded newlines, or unbalanced
`[`/`]` patterns that would break link syntax. Apply in the catalog rule's
`renderTemplate` path specifically, not in `fieldinterp.Interpolate`.
This surfaces the problem at authoring time without corrupting legitimate
content.

**Verdict:** Silent escaping is harmful. A targeted lint warning is the
proportionate fix.

---

### M7. CUE Expression Injection (Finding 7) — DOWNGRADED

**Original proposal:** Pre-validate each string by compiling in isolation
with `cuecontext.New().CompileString(expr)`.

**Problems identified:**

- **Pre-compile doesn't prevent injection.** `string | _` is valid CUE
  in isolation and in the wrapper — but it weakens the constraint to
  accept any type. Isolation compilation cannot detect context-dependent
  injection.
- Strings are **intentionally** CUE expressions (constraints like
  `=~ "pattern"`, `>= 0`). Quoting them would break all legitimate use.
- **CUE has no I/O, no exec, no network.** The blast radius is limited
  to schema bypass or DoS — no RCE possible.
- **Threat requires `.mdsmith.yml` write access** — whoever controls the
  config already controls linting behavior.

**Revised recommendation:** The CUE API (`CompileString`, `Unify`,
`Validate`) is not context-aware, so `context.WithTimeout` cannot
interrupt long-running evaluation. Use `GOMEMLIMIT` at process level
to cap memory. Accept the schema-bypass risk as inherent to the
design — it's within the trust boundary of config-file authors.

**Verdict:** Low priority. The proposed fix is ineffective and the threat
requires high privilege. Memory limiting is the realistic bound.

---

### M8. Schema Path Validation (Finding 8) — REVISED

**Original proposal:** Reject `..` and absolute paths in
`ApplySettings`, resolve relative to config dir.

**Problems identified:**

- **Symlinks bypass `..` check.** `schemas/evil` → `/etc/passwd` passes
  all string checks.
- Config dir is not available at `ApplySettings` time — would require an
  interface signature change across all rule implementors.
- Switching from CWD-relative to config-dir-relative resolution is a
  breaking change.

**Revised recommendation:** Read the schema via `f.RootFS` (the
project-root-scoped `fs.FS` already on `lint.File`) instead of
`os.ReadFile(r.Schema)`. This:

- Handles symlinks structurally (`os.DirFS` boundary)
- Requires no interface changes
- Is consistent with how the `include` directive reads files
- Avoids the CWD vs. config-dir ambiguity

**Verdict:** `fs.FS`-scoped read is architecturally cleaner and more
complete than string-based path validation.

---

### M9. Atomic Write (Finding 9) — APPROVED

**Original proposal:** Temp-file-then-rename in `fix.go`.

**Review verdict: Sound as proposed.** Key validations:

- `os.Rename` is atomic on Linux/macOS within the same filesystem.
  Same-directory temp placement ensures this.
- `os.Rename(tmp, path)` replaces the symlink itself (not the target),
  correctly closing the TOCTOU symlink-swap vector on write.
- Minor gaps are acceptable: `EXDEV` cross-device fails cleanly (error,
  not corruption); ACL/xattr loss is irrelevant for Markdown files.
- Merge driver's writes to `pathname` (lines 140, 157) are a worthwhile
  follow-on target.

**Verdict:** Implement as proposed. This is the correct write-side fix
that also subsumes M4's write protection.

---

### M10. Include Size Limit (Finding 10) — REVISED

**Original proposal:** `fs.ReadFile` + post-read size check, or
`fs.Open` + `io.LimitReader`.

**Problems identified:**

- For `os.DirFS`-backed reads, `fs.ReadFile` calls `os.ReadFile`
  internally which pre-allocates based on `stat.Size()` — the entire
  file is already in memory before the check runs. `io.LimitReader`
  provides no benefit over a post-read check for this code path.
- Off-by-one: must read `limit+1` bytes to distinguish "exact limit"
  from "truncated".
- `catalog/rule.go` has parallel unguarded `fs.ReadFile` calls
  (`readFrontMatter` at :395, `scanIncludesForTarget` at :468) — not
  covered by this fix.
- No cumulative limit across the include chain (10 files × 2 MB = 20 MB).

**Revised recommendation:** Use the shared `readFileLimited` helper from
M1 (post-read `len(data) > limit` check). Apply to:

- `include/rule.go:194`
- `catalog/rule.go:395` and `:468`

The simpler post-read check is equally effective as `io.LimitReader` for
`os.DirFS` and avoids unnecessary complexity. Consider reducing
`maxIncludeDepth` from 10 to 5 (real nesting never exceeds 3–4).

**Verdict:** Post-read check via shared helper is sufficient. Extend
coverage to catalog reads.

---

## Implementation Priority (Revised)

| Priority | Mitigation                                          | Complexity               | Impact                                |
|----------|-----------------------------------------------------|--------------------------|---------------------------------------|
| 1        | **M1** — `readFileLimited` helper across all sites  | Low (~30 LOC)            | Eliminates OOM from large files       |
| 2        | **M2** — Pre-scan for YAML `&`/`*` before unmarshal | Low (~10 LOC per site)   | Eliminates billion-laughs             |
| 3        | **M9** — Atomic write in fix mode                   | Low (~20 LOC)            | Eliminates TOCTOU + partial writes    |
| 4        | **M3** — `strings.Map` control-char stripping       | Low (~15 LOC)            | Eliminates terminal injection         |
| 5        | **M4** — Default-deny symlinks in walkDir           | Medium (breaking)        | Eliminates symlink read+write attacks |
| 6        | **M5** — `filepath.Rel` boundary in cross-file-ref  | Low (~10 LOC + plumbing) | Eliminates out-of-project reads       |
| 7        | **M8** — Schema read via `f.RootFS`                 | Low (~5 LOC)             | Eliminates arbitrary file read        |
| 8        | **M10** — Size limit on include + catalog reads     | Low (reuse M1 helper)    | Bounds include-chain memory           |
| 9        | **M6** — Lint warning on suspicious catalog values  | Low (~20 LOC)            | Surfaces injection at author time     |
| 10       | **M7** — CUE evaluation timeout                     | Low (~5 LOC)             | Bounds DoS from complex expressions   |

---

## Cross-Linter Comparison: How the Top 5 Handle These Exploits

Comparison of how Prettier (51K stars), markdownlint (6K), Vale (5.3K),
remark-lint (1K + remark 8.8K), and textlint (3.1K) handle the same 10
security concerns found in mdsmith.

### Legend

- **Vuln** = Same vulnerability exists, no mitigation
- **Mitigated** = Vulnerability exists but is actively mitigated
- **By design** = Intentional behavior, documented as expected
- **N/A** = Feature doesn't exist, so attack surface absent
- **Fixed** = Was vulnerable, now patched

### Comparison Matrix

| #   | Exploit                                | mdsmith    | Prettier                  | markdownlint              | Vale                         | remark-lint                                 | textlint                      |
|-----|----------------------------------------|------------|---------------------------|---------------------------|------------------------------|---------------------------------------------|-------------------------------|
| 1   | **OOM (no file-size limit)**           | Vuln       | Vuln                      | Vuln                      | Vuln                         | Vuln (documented advisory)                  | Vuln                          |
| 2   | **YAML billion-laughs**                | Vuln       | Mitigated (`eemeli/yaml`) | N/A (no YAML parse)       | Mitigated (`yaml.v2` v2.4.0) | Mitigated (`eemeli/yaml` maxAliasCount:100) | Vuln (`js-yaml` v4, no limit) |
| 3   | **ANSI escape injection**              | Vuln       | Vuln                      | Vuln                      | Vuln                         | Vuln                                        | Vuln                          |
| 4   | **Symlinks followed**                  | Vuln       | Fixed (v3.0)              | Vuln                      | Vuln (explicit)              | Vuln (partial)                              | Mitigated (glob default)      |
| 5   | **Path traversal (link check)**        | Vuln       | N/A                       | Vuln (3rd-party rule)     | N/A                          | Vuln (documented risk)                      | Vuln (3rd-party rule)         |
| 6   | **Content injection via front matter** | Vuln       | N/A                       | N/A                       | N/A                          | N/A                                         | N/A                           |
| 7   | **Config expression/code injection**   | Vuln (CUE) | By design (JS plugins)    | By design (`.cjs` config) | By design (Tengo scripts)    | By design (`.remarkrc.js`)                  | By design (`--rulesdir`)      |
| 8   | **Unvalidated config paths**           | Vuln       | Low risk                  | Vuln (`extends`)          | Vuln (packages/zip)          | Vuln (config search)                        | Vuln (`--config`)             |
| 9   | **Non-atomic writes**                  | Vuln       | Vuln                      | Vuln                      | N/A (read-only)              | Vuln                                        | Vuln                          |
| 10  | **No include size limit**              | Vuln       | N/A                       | N/A                       | N/A                          | Vuln (3rd-party plugins)                    | N/A                           |

### Key Observations

**1. OOM from large files is an industry-wide gap.**
Every tool examined loads entire files into memory with no size guard.
remark's docs advise callers to "cap input at 500 KB" but don't enforce
it. mdsmith implementing M1 (`readFileLimited`) would make it the first
in this group to have a built-in defense.

**2. YAML billion-laughs is a library choice.**

- `eemeli/yaml` (used by Prettier, remark) defaults to `maxAliasCount:
  100` — effective mitigation with zero application code.
- `gopkg.in/yaml.v2` v2.4.0 (used by Vale) back-ported alias-depth
  fixes — also effective.
- `gopkg.in/yaml.v3` (used by mdsmith) and `js-yaml` v4 (used by
  textlint) have **no** alias limits — both are vulnerable.
- markdownlint avoids the issue entirely by not parsing YAML.
- **Recommendation for mdsmith:** M2's pre-scan for `&`/`*` tokens
  remains the simplest fix. Alternatively, switching to `goccy/go-yaml`
  (which has `MaxAliasesNum`) would match the `eemeli/yaml` approach.

**3. ANSI escape injection is universal and unmitigated.**
All 6 tools (including mdsmith) emit user-controlled content to the
terminal without stripping control characters. No tool has shipped a
fix. mdsmith implementing M3 (`strings.Map` sanitizer) would be
first-in-class.

**4. Symlinks: only Prettier has fixed this.**
Prettier v3.0 rejects symlinks during glob expansion. All others follow
symlinks by default. Vale explicitly follows symlinks in a custom
`Walk()` function. mdsmith's M4 (default-deny) would match Prettier's
stance.

**5. Config-as-code is "by design" everywhere in the Node.js ecosystem.**
markdownlint, remark-lint, textlint, and Prettier all load `.js`/`.cjs`
config files via `require()`/`import()` with no sandboxing. This is
documented and intentional — the security boundary is "don't run linters
on untrusted repos." Vale partially sandboxes Tengo scripts (no `os`
module). mdsmith's CUE injection (finding 7) is less severe since CUE
has no I/O primitives.

**6. Non-atomic writes are universal among tools with fix/write modes.**
Prettier, markdownlint, remark-lint, and textlint all use bare
`fs.writeFile` with no temp-file-then-rename. Vale avoids the issue by
being read-only. mdsmith implementing M9 (atomic rename) would be
first-in-class.

**7. Path traversal in link checking is acknowledged but unfixed.**
remark-validate-links documents the risk ("this may be dangerous") but
ships no enforcement. markdownlint-rule-relative-links and
textlint-rule-no-dead-link have similar gaps. mdsmith's M5
(`filepath.Rel` boundary) would be a concrete improvement.

**8. mdsmith's unique attack surfaces.**
Findings 6 (catalog content injection) and 10 (include size limit)
are specific to mdsmith's directive system. No other tool in this
comparison has equivalent features, so there is no industry precedent
to compare against. These are novel attack surfaces that require
mdsmith-specific mitigations.

### mdsmith's Competitive Position After Mitigations

If all 10 revised mitigations (M1–M10) are implemented, mdsmith would
have the strongest security posture among the tools compared:

| Defense                 | mdsmith (post-fix)    | Best current peer       |
|-------------------------|-----------------------|-------------------------|
| File-size limit         | M1: `readFileLimited` | remark: advisory only   |
| YAML alias bomb         | M2: pre-scan `&`/`*`  | remark: `maxAliasCount` |
| Terminal sanitization   | M3: `strings.Map`     | None                    |
| Symlink default-deny    | M4: opt-in only       | Prettier v3: reject     |
| Path traversal boundary | M5: `filepath.Rel`    | None                    |
| Atomic writes           | M9: temp+rename       | None                    |
