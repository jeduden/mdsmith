---
id: 2607022119
title: Word-frequency metric and over-repetition rule
status: "🔲"
summary: >-
  Add a MET007 word-frequency metric that ranks a file's
  content words by count (feeding `mdsmith metrics rank`),
  plus an opt-in rule that flags any content word repeated
  past a threshold within a scope. A stopword `lists:` set is
  subtracted first so common words never trip it.
model: sonnet
depends-on: [2606251522]
---
# Word-frequency metric and over-repetition rule

## Goal

Rank the content words in a file by how often they appear.
Fail when one non-stopword word repeats past a threshold in a
scope. This catches accidental over-repetition that no
banned-word list anticipates.

## Context

The `occurrence` rule (plan 2607022118) bounds a *known* token
you name in config. This plan handles the *unknown* case: the
word you did not think to list but repeated six times in one
section. Vale's
[`repetition`](https://vale.sh/docs/checks/repetition) check
only catches immediately adjacent duplicates ("the the"); this
is broader — frequency density across a scope.

mdsmith already ships a metrics subsystem: bytes, lines,
words, headings, token estimate, and conciseness (MET001
through MET006). All rank via `mdsmith metrics rank`. Word
frequency is the natural MET007. It is a per-file distribution,
not a single scalar.

The rule reuses the named word-list mechanism for its stopword
set (`lists:`, plan 2606251522 / PR #694). A project points at
a shared stopword list. It need not restate "the, a, of, and"
in every rule.

## Design

Two artifacts, one shared tokenizer.

**MET007 word-frequency (metric).** A new metric under
`internal/metrics/` that emits the top-N words and their counts
for a file. It folds case, strips inline-code and code blocks,
and splits on Unicode word boundaries. It applies `min-length`
(default 4 runes), which already drops the shortest function
words. No stopword list ships compiled in, matching the
no-built-in-lists direction. It reports a small structured
distribution so
`mdsmith metrics rank --metric word-frequency` can rank files
by their single most-repeated content word (the "most
repetitive file" query a release gate wants).

**over-repetition (rule, opt-in, off by default).** A rule that
runs the same tokenizer per scope and fails when any surviving
word's count in that scope exceeds `max` (default `scope:
section`). Settings:

- `scope`: `file` | `section` | `paragraph`.
- `max`: the per-word ceiling (e.g. 4 per section).
- `min-length`: ignore words shorter than N runes (default 4),
  so short connectors never dominate.
- `stopwords`: the `WordlistTarget()` key. A `lists:` set
  unions a shared stopword list into it, subtracted before
  counting. With no list, only `min-length` filters.
  `mdsmith init --wordlists` can scaffold a starter stopword
  list.

Both share one unexported tokenizer so the metric and the rule
never disagree on what a "word" is. The rule flags; it does not
rewrite (choosing a synonym is a semantic act, not a mechanical
one). Case folding and stopword subtraction happen once per
scope with reused buffers, keeping `Check` within the
≤10-alloc budget.

## Tasks

1. Add the shared tokenizer (case-fold, strip code, split on
   word boundaries, apply `min-length`) as an unexported helper
   reused by both artifacts. Stopword subtraction is the rule's
   job, from its `lists:` set; the tokenizer ships no compiled
   stopword list.
2. Add MET007 under `internal/metrics/` with its
   `MET007-word-frequency/` README and registry wiring; make it
   rankable through `mdsmith metrics rank`.
3. Add package `internal/rules/overrepetition`: `Rule`,
   `ApplySettings` (scope, max, min-length, stopwords), and
   `Check`. Red/green per setting. Off by default.
4. Implement `WordlistTarget() string { return "stopwords" }`
   and the `rule.WordlistConsumer` assertion so `lists:` feeds
   the stopword set.
5. Register the rule (next free ID, e.g. MDS071), add
   `internal/rules/MDS071-over-repetition/` with README and
   `good/`/`bad/` fixtures including a `lists:`-driven stopword
   case; confirm alloc-budget coverage.
6. Document both in the metrics and rules references and in
   `docs/guides/metrics-tradeoffs.md`; regenerate catalogs with
   `mdsmith fix`.

## Acceptance Criteria

- [ ] `mdsmith metrics rank --metric word-frequency` ranks a
      corpus by each file's most-repeated content word.
- [ ] A section repeating one content word five times fails
      over-repetition at `max: 4`; the same word inside a code
      block does not count.
- [ ] A word on the stopword `lists:` set is never flagged,
      proven by a fixture whose list lives in
      `.mdsmith/wordlists/`.
- [ ] The metric and the rule agree on token boundaries
      because they share one tokenizer (a test asserts it).
- [ ] The rule's `Check` stays within the alloc budget.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues.
