---
summary: >-
  First-party hyperfine benchmark of mdsmith against mado,
  rumdl, panache, and markdownlint-cli2 over two corpora, with
  the exact commands, environment, and an honest reading of why
  mdsmith trails the per-file Rust linters.
---
# Markdown linter benchmark

Our own benchmark run, not a re-quote of each project's README.
Reproduce it with [`run.sh`](run.sh).

## Method

- Driver: `hyperfine` 1.20.0, `--warmup 3 --runs 10 -N`
  (markdownlint-cli2: `--warmup 2 --runs 6`).
- Each tool runs its default check/lint over a directory of
  `.md` files. No tool-specific config is supplied, so every
  engine runs on its built-in defaults.
- Caches disabled for the tools that have one (`rumdl
  --no-cache`, `panache --no-cache`) so every run is
  worst-case cold. mdsmith, mado, and markdownlint-cli2 keep
  no on-disk cache.
- This is a wall-clock throughput test of each tool's default
  rule set. It is not a like-for-like rule comparison: the
  tools do different amounts of work per file (see
  [Reading the result](#reading-the-result)).

### Corpora

| Corpus  | Files | Source                                            |
|---------|-------|---------------------------------------------------|
| repo    | 523   | mdsmith's own tracked Markdown (fixtures dropped) |
| neutral | 234   | Rust Book + Rust Reference `src/` (third-party)   |

### Environment

- Linux 6.18.5 x86_64, 4 vCPU, Intel Xeon @ 2.10 GHz
- mdsmith (Go 1.25.8 build), mado 0.3.0, rumdl 0.1.93,
  panache 2.46.0, markdownlint-cli2 0.22.1 (markdownlint
  0.40.0)
- Date: 2026-05-17

## Results

Median wall time (lower is better); `min` is the fastest of
10 runs; `vs mado` is the median ratio to the fastest tool.

### Corpus: repo (523 files)

| Tool              | Median  | Min     | vs mado |
|-------------------|---------|---------|---------|
| mado              | 45 ms   | 44 ms   | 1.0x    |
| rumdl             | 164 ms  | 153 ms  | 3.6x    |
| panache           | 226 ms  | 216 ms  | 5.0x    |
| mdsmith           | 1004 ms | 988 ms  | 22x     |
| markdownlint-cli2 | 3342 ms | 3304 ms | 74x     |

### Corpus: neutral (234 files)

| Tool              | Median  | Min     | vs mado |
|-------------------|---------|---------|---------|
| mado              | 46 ms   | 46 ms   | 1.0x    |
| rumdl             | 147 ms  | 142 ms  | 3.2x    |
| panache           | 315 ms  | 311 ms  | 6.8x    |
| mdsmith           | 1597 ms | 1563 ms | 34x     |
| markdownlint-cli2 | 3333 ms | 3079 ms | 72x     |

## Reading the result

Two facts stand out, and both are honest.

**Every native binary crushes the Node baseline.** mdsmith
checks the 523-file repo corpus in ~1.0 s; markdownlint-cli2
takes ~3.3 s. mado, rumdl, and panache are faster still. If
the alternative is a Node markdownlint, any of these is a
large speed win.

**mdsmith is the slowest native tool here, by design.** mado
is a check-only port of ~41 markdownlint rules. rumdl and
panache are per-file linters too. mdsmith does strictly more
on every run: it resolves the cross-file link and anchor
graph across the whole workspace, scores paragraph
readability and structure, estimates token budgets, and
validates generated sections. That work does not get cheaper
by adding files, which is why mdsmith is slower on the
234-file neutral corpus (long Rust Book prose) than on the
523-file repo corpus (short doc files).

So the earlier "same order of magnitude as the Rust tools"
framing was too kind. The accurate statement: mdsmith is
~3x faster than the Node markdownlint reference and ~20-34x
slower than a minimal Rust markdownlint clone, because it is
not a markdownlint clone — it ships a cross-file and
generated-content layer the others do not have. Pick mado or
rumdl for raw markdownlint-rule throughput; pick mdsmith when
the cross-file graph, readability budgets, and self-
maintaining sections are the point.

### A note on the "<300 ms" self-check claim

The performance feature page cites a sub-300 ms full check
"of this repository — 70-plus Markdown files". That figure
is a narrow scope. The repository now tracks ~720 Markdown
files; `mdsmith check .` on the whole tree measures ~1.4 s
here, and an 18-file `docs/features` subset is ~50 ms. The
"<300 ms" number is real only for a small subset, not a
full-repo check. That page should be re-scoped to match.
