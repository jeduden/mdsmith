---
id: 2607071642
title: Single-file metric extraction via `mdsmith metrics get` (readability first)
status: "✅"
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

- It emits every registered file-scope metric. There is no metric
  selector: a single-file read has no column-bloat concern, and a
  reader narrows downstream with `jq` or `yq` — exactly as
  [`mdsmith extract`][extractdoc] emits its whole projection. So
  `readability` appears with no flag, and the surface stays as
  small as `extract`'s.
- `-f json|yaml|text` picks the format, the same short and long
  flag `extract` uses. Text is an aligned `name  value` table.
- Exactly one positional file, like `extract` / `export` / `deps`.
  Zero or many is a usage error.

**Add the `yaml` format.** `get`, `rank`, and `list` all accept
`yaml`. Encode the same map the json path builds, through the
shared YAML encoder that [`mdsmith extract`][extractdoc] uses, so
json and yaml stay structurally identical. Each `unknown format`
message reads `text, json, yaml`.

## CLI design

Every choice above is derived from an existing command, not
invented, so the surface stays predictable. The rules:

- **Match the grammar.** `mdsmith`'s single-file emitters —
  `extract`, `export`, `deps` — take a positional `<file>` and,
  for data, `-f/--format`. `get` copies that exactly.
- **Least surprise.** `get` adds no flag that a neighbour command
  does not already have. It has no metric selector because
  `extract` has no field selector; filtering is a downstream
  `jq`/`yq` job. `rank` keeps `--metrics` because it alone needs
  it (columns and the `--by` sort key across many files).
- **One direction per stream.** Data to stdout, errors to stderr,
  exit `0` on success and `2` on a usage or runtime error, as the
  other emitters do.
- **Discoverable.** `mdsmith help metrics` and the usage text name
  `get`, its one argument, and its formats.
- **Proven by running it.** Acceptance runs `metrics get` on a
  real repo file and checks the printed object, not just the
  tests.

## Example output

One file's metrics as a single YAML object:

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

The same file as JSON:

```console
$ mdsmith metrics get -f json README.md
{
  "path": "README.md",
  "bytes": 4096,
  ...
  "readability": 11.2
}
```

Just the readability score is a downstream filter, as with
`extract`:

```console
$ mdsmith metrics get -f json README.md | jq .readability
11.2
```

The text format is an aligned table:

```console
$ mdsmith metrics get README.md
NAME                    VALUE
bytes                   4096
...
readability             11.2
sentences               96
avg-words-per-sentence  14.2
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
4. Add the `metrics get` subcommand: parse exactly one positional
   file and `-f`, emit single-object json/yaml/text writers, and
   unit tests including the zero-and-many-argument usage errors.
   Route it from `runMetrics` beside `list` and `rank`, and name
   it in the `metrics` usage text and `mdsmith help metrics`.
5. Add the `yaml` format to `get`, `rank`, and `list`. Extend the
   format switches, the `unknown format` errors, and the unit
   tests ([`metrics_unit_test.go`][mtest]). Assert json and yaml
   agree structurally.
6. Add [`docs/reference/cli/metrics.md`][rankdoc] coverage for
   `get` (its one argument, its formats, an example) and the new
   metrics. Update [`docs/guides/metrics-tradeoffs.md`][tradeoffs].
   Regenerate the `CLAUDE.md` and `PLAN.md` catalogs with
   `mdsmith fix`.

## Acceptance Criteria

- [x] `mdsmith metrics get -f json FILE` prints a single JSON
      object (not an array) carrying every registered metric,
      including the file's ARI score under a `readability` key.
- [x] `mdsmith metrics get -f yaml FILE` prints the same values as
      a YAML mapping; a test pins json and yaml together.
- [x] `mdsmith metrics get` with zero or two file arguments is a
      usage error; the command takes no metric-selector flag.
- [x] Running `mdsmith metrics get` on a real repo file (dogfood,
      not only unit tests) prints the expected object.
- [x] `mdsmith metrics list` shows `readability`, `sentences`,
      and `avg-words-per-sentence`; none appears in a
      no-`--metrics` `rank` run.
- [x] `internal/metrics` imports `mdtext` for ARI and no rule
      package (a test or the import audit proves it); MDS023
      behavior is unchanged.
- [x] An unknown `-f` value reports `text, json, yaml`.
- [x] All Markdown passes: `mdsmith check .`
- [x] All tests pass: `go test ./...`
- [x] `go tool -modfile=tools/go.mod golangci-lint run` reports
      no issues.

[mds023]: ../internal/rules/MDS023-paragraph-readability/README.md
[metrics]: ../internal/metrics/registry.go
[ari]: ../internal/rules/paragraphreadability/readability.go
[mtest]: ../cmd/mdsmith/metrics_unit_test.go
[rankdoc]: ../docs/reference/cli/metrics.md
[extractdoc]: ../docs/reference/cli/extract.md
[tradeoffs]: ../docs/guides/metrics-tradeoffs.md
[wordfreq]: 2607022119_word-frequency-metric-rule.md
