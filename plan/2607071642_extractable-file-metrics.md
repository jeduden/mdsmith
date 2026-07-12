---
id: 2607071642
title: Single-file metric extraction via `mdsmith metrics get` (readability first)
status: "🔲"
summary: >-
  Add a `mdsmith metrics get <file>` command that emits one file's
  metrics as a single JSON or YAML object, add a readability metric
  (Automated Readability Index over the file's plain text) plus
  sentence-count and words-per-sentence metrics, and move the ARI
  formula into `internal/mdtext` so the metrics package never
  imports a rule package.
model: sonnet
depends-on: []
---
# Single-file metric extraction via `mdsmith metrics get`

## Goal

Let a caller read one file's readability score as JSON or YAML
from a command built for a single file. Make the next metric cheap
to add: one registry entry plus a README.

## Context

Today the readability number lives inside a lint rule,
[MDS023 paragraph-readability][mds023]. That rule scores each
paragraph with the Automated Readability Index (ARI). It emits a
pass or fail against `max-index`. The raw score is never returned
as data.

The data-shaped subsystem is [`internal/metrics`][metrics]. It is
a registry of file-scope metrics. The current set is MET001 bytes,
MET002 lines, MET003 words, MET004 headings, MET005
token-estimate, and MET006 conciseness.

The only command that emits them is
[`mdsmith metrics rank`][rankdoc]. Rank is a cross-file ordering.
It sorts many files by a metric and prints a JSON array. Asking it
for one file is a category error: ranking a single document is
meaningless, and the output is a one-element array a caller must
unwrap. There is no command that reads one file's metrics.

Two more gaps block the request. No metric reports readability.
And no metrics command prints `yaml`, though
[`mdsmith extract`][extractdoc] already offers both json and yaml.

MET007 is reserved by plan [2607022119][wordfreq]. So this plan
takes MET008 onward.

The ARI formula sits in
[`readability.go`][ari]. It depends only on `internal/mdtext`
counters. A metric must not import a rule package. Metrics is a
lower layer. So the formula moves down into `mdtext`. Its word,
sentence, and character counters already live there.

## Design

Four parts: move the formula, add metrics, add the command, add
the format.

**Move ARI into `mdtext`.** Add `mdtext.ARI(text string) float64`
holding the current formula and returning zero on zero words. The
rule's `ARI` becomes a thin delegate, so `IndexFunc` still works
and every MDS023 test still passes. This is the clean-architecture
step: afterward `internal/metrics` depends on `mdtext`, not on a
rule.

**Add the metrics.** Three registry entries. Each gets a
`MET###-<name>/README.md` that mirrors MET006. Each reads the
Document's cached `PlainText()`:

- MET008 `readability` — `mdtext.ARI(text)`. Float, precision 1,
  default order `desc`. Higher means harder to read. It is an
  approximate US grade level.
- MET009 `sentences` — `mdtext.CountSentences(text)`. Integer,
  `desc`.
- MET010 `avg-words-per-sentence` — words over sentences, or zero
  with no sentences. Float, precision 1, `desc`.

The three ship `Default: false` so the default `rank` table keeps
its six columns. That flag governs `rank` only. `metrics get` and
`metrics list` show every registered metric.

**Add `metrics get`.** A new subcommand for one file:
`mdsmith metrics get [flags] <file>`. It computes metrics for that
file and emits a single object, not an array — the shape a
single-document read wants. It reuses `metrics.Collect` with a
one-path slice and the existing `JSONValue` conversion.

- By default it emits every registered file-scope metric, since a
  single-file read has no column-bloat concern. So `readability`
  appears with no flag.
- `--metrics a,b` narrows to a subset.
- `-f json|yaml|text` picks the format. Text is an aligned
  `name  value` table.
- Exactly one file argument; zero or many is a usage error.

**Add the `yaml` format.** `get`, `rank`, and `list` all accept
`yaml`. Encode the same map the json path builds, through the
shared YAML encoder that [`mdsmith extract`][extractdoc] uses, so
json and yaml stay structurally identical. Each `unknown format`
message reads `text, json, yaml`.

## Example output

Reading one file's readability score as JSON:

```console
$ mdsmith metrics get --metrics readability -f json README.md
{
  "path": "README.md",
  "readability": 11.2
}
```

The same query as YAML:

```console
$ mdsmith metrics get --metrics readability -f yaml README.md
path: README.md
readability: 11.2
```

With no `--metrics`, every registered metric is emitted:

```console
$ mdsmith metrics get -f yaml docs/guides/install.md
path: docs/guides/install.md
bytes: 5120
lines: 132
words: 812
headings: 14
token-estimate: 609
conciseness: 71.5
readability: 9.4
sentences: 148
avg-words-per-sentence: 14.6
```

The text format is an aligned table:

```console
$ mdsmith metrics get --metrics readability,sentences README.md
NAME         VALUE
readability  11.2
sentences    96
```

An unavailable value (an unreadable or empty file) prints `null`
in json and yaml and `-` in text, matching the current metrics
behavior.

## Tasks

1. Add `mdtext.ARI` with a unit test. Rewrite
   `paragraphreadability.ARI` to delegate. Confirm the MDS023
   tests and the alloc budget still pass (red/green).
2. Add MET008 `readability` to the registry. Add its
   `internal/metrics/MET008-readability/README.md`. A registry
   test covers the value on a known fixture.
3. Add MET009 `sentences` and MET010 `avg-words-per-sentence`
   with their READMEs and registry tests.
4. Add the `metrics get` subcommand: flag parsing (one file,
   `--metrics`, `-f`), single-object json/yaml/text writers, and
   unit tests. Route it from `runMetrics` beside `list` and
   `rank`.
5. Add the `yaml` format to `get`, `rank`, and `list`. Extend the
   format switches, the `unknown format` errors, and the unit
   tests ([`metrics_unit_test.go`][mtest]). Assert json and yaml
   agree structurally.
6. Add [`docs/reference/cli/metrics.md`][rankdoc] coverage for
   `get` (flags, an example) and the new metrics. Update
   [`docs/guides/metrics-tradeoffs.md`][tradeoffs]. Regenerate the
   `CLAUDE.md` and `PLAN.md` catalogs with `mdsmith fix`.

## Acceptance Criteria

- [ ] `mdsmith metrics get --metrics readability -f json FILE`
      prints a single JSON object with the file's ARI score under
      a `readability` key — an object, not an array.
- [ ] `mdsmith metrics get -f yaml FILE` prints every registered
      metric as a YAML mapping; a test pins json and yaml
      together.
- [ ] `mdsmith metrics get` with zero or two file arguments is a
      usage error.
- [ ] `mdsmith metrics list` shows `readability`, `sentences`,
      and `avg-words-per-sentence`; none appears in a
      no-`--metrics` `rank` run.
- [ ] `internal/metrics` imports `mdtext` for ARI and no rule
      package (a test or the import audit proves it); MDS023
      behavior is unchanged.
- [ ] An unknown `-f` value reports `text, json, yaml`.
- [ ] All Markdown passes: `mdsmith check .`
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run` reports
      no issues.

[mds023]: ../internal/rules/MDS023-paragraph-readability/README.md
[metrics]: ../internal/metrics/registry.go
[ari]: ../internal/rules/paragraphreadability/readability.go
[mtest]: ../cmd/mdsmith/metrics_unit_test.go
[rankdoc]: ../docs/reference/cli/metrics.md
[extractdoc]: ../docs/reference/cli/extract.md
[tradeoffs]: ../docs/guides/metrics-tradeoffs.md
[wordfreq]: 2607022119_word-frequency-metric-rule.md
