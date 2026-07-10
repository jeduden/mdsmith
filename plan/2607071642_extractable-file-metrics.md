---
id: 2607071642
title: Extractable per-file metrics (readability first) with YAML output
status: "🔲"
summary: >-
  Make per-file Markdown measurements extractable as structured
  data: add a readability metric (Automated Readability Index over
  the file's plain text) plus sentence-count and
  words-per-sentence metrics, teach `mdsmith metrics rank` and
  `metrics list` a `yaml` output format alongside `json`, and move
  the ARI formula into `internal/mdtext` so the metrics package
  never imports a rule package.
model: sonnet
depends-on: []
---
# Extractable per-file metrics (readability first) with YAML output

## Goal

Let a caller read one file's readability score as JSON or YAML
from a single command. Make the next metric cheap to add: one
registry entry plus a README.

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
[`mdsmith metrics rank`][rankdoc] computes them and prints them.

Two gaps block the request. First, no metric reports readability.
Second, `metrics rank` and `metrics list` print only `text` and
`json`. There is no `yaml`. Yet [`mdsmith extract`][extractdoc]
already offers both json and yaml.

MET007 is reserved by plan [2607022119][wordfreq]. So this plan
takes MET008 onward.

The ARI formula sits in
[`readability.go`][ari]. It depends only on `internal/mdtext`
counters. A metric must not import a rule package. Metrics is a
lower layer. So the formula moves down into `mdtext`. Its word,
sentence, and character counters already live there.

## Design

Three parts: move the formula, add metrics, add the format.

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

All three ship `Default: false`. So the default `metrics rank`
table keeps its six columns. A caller opts in with
`--metrics readability`. They still show in `metrics list`, which
lists every registered metric.

**Add the `yaml` format.** Extend `writeRankOutput` and the
`metrics list` switch to accept `yaml`. Encode the same map the
json path builds. Use the shared YAML encoder that
[`mdsmith extract`][extractdoc] uses. So json and yaml stay
structurally identical. Update the two `unknown format` messages
to read `text, json, yaml`.

**Single-file usage.** `mdsmith metrics rank <file> -f yaml`
already targets one file, and its output is a one-element list, so
a reader takes `.[0].readability`. A single-file object view is
out of scope here; the list shape stays consistent and pipeable,
and it is noted as a possible follow-up.

## Example output

Reading one file's readability score as JSON:

```console
$ mdsmith metrics rank --metrics readability -f json README.md
[
  {
    "path": "README.md",
    "readability": 11.2
  }
]
```

The same query as YAML:

```console
$ mdsmith metrics rank --metrics readability -f yaml README.md
- path: README.md
  readability: 11.2
```

Several new metrics at once, ranked by readability:

```console
$ mdsmith metrics rank \
    --metrics readability,sentences,avg-words-per-sentence \
    --by readability -f json docs/guides/install.md
[
  {
    "path": "docs/guides/install.md",
    "readability": 9.4,
    "sentences": 148,
    "avg-words-per-sentence": 14.6
  }
]
```

`mdsmith metrics list` gains the three rows (opt-in, so
`DEFAULT` is `false`):

```console
$ mdsmith metrics list
ID      NAME                    SCOPE  ORDER  DEFAULT  DESCRIPTION
...
MET008  readability             file   desc   false    Automated Readability Index ...
MET009  sentences               file   desc   false    Sentence count ...
MET010  avg-words-per-sentence  file   desc   false    Mean words per sentence ...
```

A single unavailable value (an unreadable or empty file) prints
`null` in json and yaml and `-` in text, matching the current
metrics behavior.

## Tasks

1. Add `mdtext.ARI` with a unit test. Rewrite
   `paragraphreadability.ARI` to delegate. Confirm the MDS023
   tests and the alloc budget still pass (red/green).
2. Add MET008 `readability` to the registry. Add its
   `internal/metrics/MET008-readability/README.md`. A registry
   test covers the value on a known fixture.
3. Add MET009 `sentences` and MET010 `avg-words-per-sentence`
   with their READMEs and registry tests.
4. Add the `yaml` format to `metrics rank` and `metrics list`.
   Extend the format switches, the `unknown format` errors, and
   the unit tests ([`metrics_unit_test.go`][mtest]). Assert json
   and yaml agree structurally.
5. Update [`docs/reference/cli/metrics.md`][rankdoc]: the format
   flags, the metric list, a readability example. Update
   [`docs/guides/metrics-tradeoffs.md`][tradeoffs]. Regenerate the
   `CLAUDE.md` and `PLAN.md` catalogs with `mdsmith fix`.

## Acceptance Criteria

- [ ] `mdsmith metrics rank --metrics readability -f json FILE`
      prints the file's ARI score under a `readability` key.
- [ ] `mdsmith metrics rank --metrics readability -f yaml FILE`
      prints the same value as YAML; a test pins json and yaml
      together.
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
