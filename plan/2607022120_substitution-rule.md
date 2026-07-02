---
id: 2607022120
title: Substitution rule — deterministic word-choice swaps with auto-fix
status: "🔲"
summary: >-
  Add a `substitution` rule that maps banned terms to preferred
  replacements and rewrites them under `mdsmith fix`. Vale's
  substitution check only flags; the auto-fix is mdsmith's
  edge. Swaps preserve case and word boundaries and skip code
  spans, code blocks, and URLs.
model: opus
depends-on: []
---
# Substitution rule — deterministic word-choice swaps with auto-fix

## Goal

Let a project declare "write Y, not X" pairs. `mdsmith fix`
then performs the swap. A house word-choice guide becomes a
mechanical, one-command rewrite.

## Context

Vale's
[`substitution`](https://vale.sh/docs/checks/substitution)
check maps a bad term to a preferred one. It reports the
suggestion. It does not apply it — Vale has no fix engine.

mdsmith does. `mdsmith fix` already runs a multi-pass loop
that rewrites whitespace, headings, fences, and tables in
place. A deterministic X-to-Y word swap fits that loop exactly.
This is the clearest "learn from Vale, then go one better"
feature: the same rule Vale ships, plus the auto-fix Vale
lacks.

It rounds out the prose-tell family. `forbidden-text` (MDS056)
bans a term outright. This rule bans a term *and* supplies its
replacement, so the author never has to pick one.

## Design

A new rule, `substitution` (next free ID, e.g. MDS072), holds
an ordered map of swaps and registers a fixer.

Settings:

- `swaps`: a map from a banned term to its replacement, e.g.
  `{"utilise": "use", "in order to": "to"}`. Order is
  preserved so a longer phrase wins before a substring of it.
- `case-sensitive`: default false. When false, a match folds
  case and the replacement mirrors the source case — `Utilise`
  becomes `Use`, `UTILISE` becomes `USE`, `utilise` becomes
  `use`.
- `whole-word`: default true. Word boundaries only, so
  `use` does not rewrite the middle of `causes`.

Matching walks `f.AST` and skips inline-code spans, fenced and
indented code blocks, autolinks, and link destinations, so a
command or a URL is never rewritten. Each swap compiles to a
package-scope pattern once.

The rule reports one diagnostic per hit (`use "X" instead of
"Y"`) in check mode. Under `mdsmith fix` it emits the byte
edits its fixer applies. The multi-pass loop already re-runs
rules until edits stabilize, so a swap that exposes a second
swap is handled with no extra machinery.

Two guardrails keep the swap safe:

- A replacement that itself contains a banned term is a config
  error at load, caught before any file is touched. Otherwise
  the fix loop would oscillate and hit its pass cap.
- Case mirroring only applies to all-lower, all-upper, and
  capitalized forms. A mixed-case source (`uTiLiSe`) is
  reported but left for the author, never guessed.

The banned side of a swap may also be sourced from a named
list. `WordlistTarget()` returns `swaps-from`; a `lists:` entry
supplies banned terms that map to a single configured default
replacement, for the "ban this whole list, replace with X"
case. Explicit `swaps` pairs always win over a list default.

## Tasks

1. Add package `internal/rules/substitution`: `Rule`,
   `ApplySettings` (swaps, case-sensitive, whole-word), and
   `Check`. Red/green per setting.
2. Add the AST-scope skip (code spans, code blocks, URLs) and
   the case-mirroring replacement helper, each unit-tested.
3. Register the fixer so `mdsmith fix` applies the swaps in the
   multi-pass loop; test that a swap exposing a second swap
   converges.
4. Add the two guardrails: reject a replacement containing a
   banned term at config load; leave mixed-case sources
   unfixed. Drive each red/green.
5. Implement `WordlistTarget() string { return "swaps-from" }`
   and the `rule.WordlistConsumer` assertion; a `lists:` set
   maps to a configured default replacement.
6. Register the rule (e.g. MDS072), add
   `internal/rules/MDS072-substitution/` with README and
   `good/`/`bad/` fixtures including a fix-round-trip case and a
   `lists:`-driven case; confirm alloc-budget coverage.
7. Update the rules reference, the Vale coexistence guide, and
   the migrate-from guides; regenerate catalogs with
   `mdsmith fix`.

## Acceptance Criteria

- [ ] `mdsmith fix` rewrites `utilise` to `use` and `Utilise`
      to `Use`, preserving case, and leaves `causes` untouched.
- [ ] A swap inside an inline-code span, a fenced block, or a
      URL is never rewritten.
- [ ] A swap whose replacement contains a banned term fails at
      config load with a clear message, before any file edit.
- [ ] A `lists:`-sourced banned set maps to the configured
      default replacement, proven by a fixture whose list lives
      in `.mdsmith/wordlists/`.
- [ ] The fix loop converges when one swap exposes another.
- [ ] The rule's `Check` stays within the alloc budget.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
