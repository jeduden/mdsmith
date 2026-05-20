---
summary: How mdsmith compares to other Markdown linters.
---
# Markdown Linters Comparison

This document compares mdsmith with popular Markdown
linting and formatting tools. It also covers the emerging
use of LLMs as linters.

## Tool Overview

### mdsmith

Go binary with zero runtime deps. 30 rule IDs
([MDS001][mds001]-[MDS030][mds030]) covering structural
linting, readability metrics, and generated-content
management.

[MDS029][mds029] (conciseness-scoring) is experimental and
disabled by default. The other 29 rules are stable.

Key differentiators:

- Token-budget rule ([MDS028][mds028]) for LLM context
  windows
- Paragraph readability (ARI grade) and structure limits
- Regenerable sections: catalog, include, toc,
  required-structure
- Git merge driver for auto-resolving generated sections
- Metrics subsystem (bytes, lines, words, headings,
  tokens)
- Offline rule docs compiled into the binary

### [markdownlint][] ([markdownlint-cli2][])

Node.js. ~60 built-in rules
([MD001][md001]-[MD058][md058]), 31 auto-fixable. The most
widely adopted Markdown linter. GitHub uses it internally
via [markdownlint-github][].

- Config: JSONC, YAML, or JS
- Official [VS Code extension][mdl-vscode] and
  [GitHub Action][mdl-action]
- Custom rules via npm packages
- Mature ecosystem with Prettier compatibility presets
- ~5.9k GitHub stars (library)

### [remark-lint][]

Node.js. ~70 rules distributed as individual npm packages.
Part of the [unified][]/[remark][] AST pipeline ecosystem.

- Architecture: parse to mdast AST, lint, serialize
- Three maintained presets (consistent, recommended, style)
- "Fix" by round-tripping through AST (reformat entire
  file)
- Powers MDX, Gatsby, and Next.js documentation
- Deep composability with unified plugins
- ~8.8k stars (remark parent project)

### [rumdl][]

Rust binary, zero runtime deps. ~1.1k stars. Positions itself
as "a modern Markdown linter and formatter, built for speed
with Rust" — an explicit, ruff-inspired drop-in replacement
for markdownlint.

- 71 lint rules, each mapped to a markdownlint ID
  ([MD001][md001]-style); also reads existing markdownlint
  JSON/YAML config, so a repo can switch with no rule rewrite
- Autofix via `rumdl check --fix` plus a `rumdl fmt` formatter
- Config: TOML (`.rumdl.toml`, or `[tool.rumdl]` in
  `pyproject.toml`); parent-dir discovery like ESLint/Git
- LSP server plus VS Code/Cursor/Windsurf and JetBrains
  extensions
- Flavor switches for GFM, MkDocs, MDX, and Quarto
- Install: cargo, pip, npm, Homebrew, Nix, mise, winget,
  binary download
- Benchmarked on the Rust Book repo (478 files, Oct 2025);
  see [Benchmarks](#benchmarks)

### [mado][]

Rust binary. Tagline: "A fast Markdown linter written in
Rust. Compatible with CommonMark and GitHub Flavored Markdown
(GFM)." Speed-first: the README leads with a benchmark, not a
feature list.

- ~41 rules mapped to markdownlint IDs (MD001-MD047 with
  gaps); each rule is tagged stable, unstable, or unsupported
- Check-only: no autofix, no `fix` mode, no LSP as of 2026-05
- Config: TOML (`mado.toml` / `.mado.toml`) with a published
  JSON Schema; per-platform global config path
- Install: Homebrew, Nix, pacman, Scoop, WinGet, prebuilt
  binaries
- Ships a GitHub Actions integration
- Headline claim: "≈49-60x faster than existing linters";
  numbers in [Benchmarks](#benchmarks)

### [panache][]

Rust binary. Not a markdownlint clone — its own rule IDs and
a different target. Tagline: "A language server, formatter,
and linter for Markdown, Quarto, and R Markdown, built in
Rust with a lossless CST parser and support for external
formatters and linters on code blocks."

- Three tools in one: `panache lint`, `panache format`, and
  a full LSP (diagnostics, code actions, symbols, folding)
- Lossless CST parser keeps Pandoc/Quarto syntax (fenced
  divs, attribute spans) instead of flattening it — the
  stated edge over Prettier and mdformat on `.qmd`/`.Rmd`
- Delegates embedded code blocks to external formatters and
  linters rather than reimplementing them
- Config: TOML (`panache.toml` / `.panache.toml`)
- Install: cargo, prebuilt binaries, AUR, Nix, PyPI
  (uv/pipx), npm, VS Code extension
- Ships reproducible hyperfine benchmarks against Prettier,
  Pandoc, rumdl, mdformat, mado, and markdownlint; see
  [Benchmarks](#benchmarks)

### [Prettier][]

Node.js. Opinionated formatter, not a linter. ~51.7k
stars.

- Reformats Markdown with zero config
- Formats embedded code blocks (JS, TS, CSS, JSON, etc.)
- Key option: [`proseWrap`][prosewrap] (preserve, always,
  never) — controls whether Prettier reflows paragraph
  line breaks to fit the print width, unwraps paragraphs
  onto single lines, or leaves existing breaks untouched
- No diagnostics, no structural checks, no rule toggles
- Best paired with a linter for structural validation

### [Vale][]

Go binary. Prose and style linter, not a structural
linter. Checks writing quality against style guides
(Microsoft, Google, AP, custom). ~5.3k stars.

- 11 [extension points][vale-checks] (existence,
  substitution, metric, etc.)
- Styles are YAML rule files; no code required
- Config: `.vale.ini` (INI format)
- Markup-aware: Markdown, HTML, RST, AsciiDoc, DITA, XML
- Used by Grafana, GitLab, DigitalOcean for docs
- Official [GitHub Action][vale-action] and
  [VS Code extension][vale-vscode]

### [textlint][]

Node.js. Zero built-in rules, fully pluggable architecture
modeled after ESLint. ~3.1k stars.

- 100+ community rules available via npm
- Parser plugins for Markdown, HTML, RST, AsciiDoc, Typst
- Autofix via `--fix` for rules implementing the fixer API
- [MCP server][textlint-mcp] support (v14.8+, enhanced in
  v15.2) for AI assistant integration
- Strong Japanese language support

### [Hugo][]

Go binary. Static site generator, not a linter. ~78k
stars. Included here because Hugo's templating overlaps
with mdsmith's directive system, and teams often weigh
the two when deciding where docs automation should live.

- Reads Markdown plus YAML/TOML/JSON front matter and
  renders to HTML via Go templates
- [Shortcodes][hugo-shortcodes] (`{{< ... >}}`) inject
  generated content (TOC, file inclusion, catalog-like
  lists) at build time
- No linting or diagnostics — invalid Markdown either
  renders silently or fails the build
- Output lives in `public/` and is typically gitignored;
  the rendered HTML is the deliverable
- Front matter is the canonical metadata source for
  taxonomies, list pages, and template variables

Hugo and mdsmith differ on **where** generated content
lives:

| Aspect              | Hugo                           | mdsmith                          |
|---------------------|--------------------------------|----------------------------------|
| Generated output    | Separate `public/` HTML tree   | In-place inside the source `.md` |
| Source readability  | Templates obscure final body   | Source always renders as-is      |
| Validation          | None (build succeeds or fails) | Diagnostics + autofix            |
| TOC / list-of-files | Shortcodes / list templates    | `<?toc?>` / `<?catalog?>`        |
| File inclusion      | `{{< readfile >}}` shortcode   | `<?include?>` directive          |
| Variable syntax     | `{{ .Title }}` (Go template)   | `{title}` (front matter field)   |
| Merge conflicts     | Re-render at build time        | `merge-driver install` resolves  |
| Agent friendliness  | Indirect (must run build)      | Direct (file is the source)      |

To ease migration, mdsmith maps common Hugo template
fields to placeholders. See the
[Hugo migration guide][hugo-migration] for that mapping.

### [Obsidian][]

Electron app. Markdown note-taking tool with local-first
storage. ~60k stars. Included here because teams that
write docs in Obsidian often want structural linting on
the same `.md` files.

- Uses its own [Obsidian Flavored Markdown][obsidian-fm]
  (OFM): wikilinks (`[[Page]]`), callouts (blockquote
  with `[!type]` prefix), embed syntax (`![[file.png]]`),
  and inline metadata (`key:: value`)
- No built-in linter — community plugins (e.g. [Linter
  plugin][obsidian-linter]) add YAML front matter fixes,
  heading normalization, and whitespace rules
- Files are plain `.md` on disk and are committed to Git
  like any other source; CI can run mdsmith over the vault

| Aspect             | Obsidian                          | mdsmith                              |
|--------------------|-----------------------------------|--------------------------------------|
| Purpose            | Note-taking editor                | Linter / fixer                       |
| Linting            | Community plugin only             | Built-in, CI-ready                   |
| Wikilinks          | Native (`[[Page]]`)               | Validated by [MDS027][mds027]        |
| Callouts           | Native (`> [!note]`)              | Validated by [MDS067][mds067]        |
| Front matter       | YAML or Dataview inline (`key::`) | YAML only (inline not recognized)    |
| Agent friendliness | Editor-centric, manual saves      | Direct file access, no editor needed |

Pin `convention: obsidian` (see the
[conventions reference][conventions]) to enable both
checks with one config line.

The wikilink check resolves `[[Page]]` against every
workspace file by stem. Matching is case-insensitive
with a shortest-path tie-break.

The callout check accepts the 12 base Obsidian types
and their aliases out of the box. Dataview inline
fields (`key:: value`) are still not front matter.
The `require`/`schema` directives do not read them.

### [mdbase][]

Specification for treating folders of Markdown files
as typed, queryable data collections. Reference impl
in TypeScript, with a Node CLI and a Rust LSP. MIT,
version 0.2.1 (early release as of 2026-05). The same
files-on-disk philosophy as mdsmith, but scoped to the
data layer: types, queries, and rename refactoring
rather than prose linting.

A small example shows the overlap and the split.
Both tools read this `.md` file as-is:

```markdown
---
title: Migrate auth to OIDC
status: in-progress
priority: 3
due: 2026-06-01
---
# Migrate auth to OIDC

The current SAML flow has two open issues. We will
swap to OIDC over the next sprint.

See the [migration log](./auth-migration-log.md).
```

What each tool **does** with the same bytes:

| Layer                                     | mdsmith                                                 | mdbase                                                    |
|-------------------------------------------|---------------------------------------------------------|-----------------------------------------------------------|
| YAML front matter                         | reads it; can validate shape via CUE schema             | reads it; validates against `_types/task.md`              |
| Body content (prose, headings)            | lints line length, headings, prose, links               | not in scope                                              |
| Cross-file link                           | flags broken `auth-migration-log.md` (MDS027)           | flags broken link (L4) and rewrites it on rename (L5)     |
| `status: in-progress`                     | available to `mdsmith list query`                       | filterable in Bases queries; appears in backlink graphs   |
| `due: 2026-06-01`                         | available to query                                      | filterable with date arithmetic (`due <= today() + "7d"`) |
| `mdsmith fix` runs                        | reformats tables, regenerates TOC/catalog               | n/a                                                       |
| `mdbase rename` runs                      | n/a                                                     | moves the file and rewrites every incoming link           |
| Body readability, structure, token budget | yes (MDS023 ARI, MDS024 sentences, MDS028 token budget) | no                                                        |

The **shared** layer is the YAML front matter.
Both tools read `status`, `priority`, `due` as
structured fields. mdbase enforces field types
out of the box via `_types/`. mdsmith does the
same when a CUE schema is wired up via MDS020;
without one, it treats them as plain YAML.

The **current surface difference** sits in the
body and the link graph. mdsmith ships prose,
structure, and generated-content rules today.
mdbase ships rename refactoring, the link graph,
and richer queries today. Either surface is a
snapshot, not a charter — see the deep-dive for
evolutionary candidates either way.

See the [deep-dive comparison][mdbase-deep-dive].
It covers types, queries, validation, links, the fix
engine, workflows, and how to run both tools together.

Using language models (GPT-4, Claude, etc.) directly to
check prose quality, conciseness, and style. This is
emerging through dedicated CLI tools and AI review bots.

How it works:

- Send Markdown to an LLM with a style prompt
- LLM returns diagnostics: verbose paragraphs, unclear
  phrasing, jargon, redundancy
- Tools wrap this in CLI or CI workflows

Dedicated tools:

- **[VectorLint][]**: CLI AI prose linter. Rules defined
  in natural language in a `VECTORLINT.md` file. Uses
  error-density scoring and 1-4 rubrics. Supports
  OpenAI, Anthropic, and Google providers.
- **[GPTLint][]**: two-pass LLM linter. A cheap model
  finds candidates, a strong model filters false
  positives. Uses [GritQL][] to pre-filter files.
  Cost: ~$0.83 for 351 API calls on its own codebase
  ([source][gptlint-cost]).

AI review services:

- [GitHub Copilot code review][copilot-review] (reviews
  Markdown in PRs)
- [CodeRabbit][] (combines 40+ linters with LLM review)
- [Claude Code Action][claude-action] (Anthropic's PR
  review action)

Hybrid systems:

- Grammarly: rule-based grammar + ML models. Their
  [CoEdIT][] model (770M-11B params) outperforms GPT-3
  at text editing while being ~60x smaller
  ([paper][coedit-paper]).

Strengths:

- Excels at subjective quality: conciseness, clarity
- Catches semantic issues no rule-based tool can detect
- A single natural-language instruction replaces many
  regex patterns (e.g. "flag hedging language")
- Understands context and intent, not just patterns

Weaknesses:

- Non-deterministic: same input may yield different
  output, even at temperature=0
- Costly: API calls per file per run add up
- Slow: seconds per file vs milliseconds for rules
- Requires network (local LLMs work but lower quality)
- Hard to use as a blocking CI gate reliably

## Feature Comparison

<?allow-empty-section?>

### Structural Linting

All three cover the core structural rules; markdownlint has
the broadest set. The full rule-by-rule mapping lives in the
[markdownlint coverage matrix][mdcov]: every markdownlint
`MDxxx`, the mdsmith rule that covers it or the plan that
schedules it, and the mdsmith-only rules. As of 2026-05
mdsmith implements 46 of 52 active markdownlint rules (44
fully, 2 partial); the other 6 are scheduled in plans 172
and 181-182.

### Rust Markdown linters (rumdl, mado, panache)

Three Rust tools sit next to mdsmith. [rumdl][] and [mado][]
are markdownlint-compatible: they adopt markdownlint's
`MDxxx` rule IDs as a drop-in surface. [panache][] is not —
it keeps its own IDs and targets Pandoc/Quarto/R Markdown.
mdsmith also keeps its own `MDSxxx` IDs and adds a
cross-file, generated-content, and readability layer none
of the three carry.

In the [coverage matrix][mdcov] the markdownlint column
doubles as the rumdl/mado column: both reuse the same
`MDxxx` semantics. rumdl implements ~71 of those IDs; mado
implements ~41. Neither adds rules outside the markdownlint
set. panache does not map to that matrix — its checks
target Quarto and
R Markdown constructs the others flatten away.

| Aspect                  | mdsmith      | rumdl                | mado                 | panache      |
|-------------------------|--------------|----------------------|----------------------|--------------|
| Language                | Go           | Rust                 | Rust                 | Rust         |
| Rule IDs                | own `MDSxxx` | markdownlint `MDxxx` | markdownlint `MDxxx` | own          |
| Rule count              | 30+          | 71                   | ~41                  | unenumerated |
| Autofix / format        | `fix`        | `--fix`, `fmt`       | no                   | `format`     |
| LSP / editor            | yes (LSP)    | yes (LSP)            | no                   | yes (LSP)    |
| Config format           | YAML         | TOML                 | TOML                 | TOML         |
| Reuse markdownlint cfg  | no           | yes                  | no                   | no           |
| Cross-file integrity    | yes          | no                   | no                   | no           |
| Generated sections      | yes          | no                   | no                   | no           |
| Readability/token rules | yes          | no                   | no                   | no           |
| Front-matter schema     | yes          | no                   | no                   | no           |
| Quarto / R Markdown     | no           | Quarto flavor        | no                   | yes (CST)    |

**Presentation notes (what to learn).** All three READMEs
win on focus. mado opens with one sentence and a benchmark
table — no feature wall before the proof. rumdl leads with
a single positioning line ("built for speed with Rust"),
names its inspiration (ruff), and states the drop-in promise
up front. panache leads with one precise sentence that names
its three jobs and its one technical edge (the lossless CST).
mdsmith's own README front matter applies the same lesson:
one crisp line, one verifiable number (sub-300 ms
self-check), and an explicit "not just a markdownlint clone"
framing before the feature list.

### Prose and Readability

| Capability        | mdsmith                         | Vale                                  | LLM |
|-------------------|---------------------------------|---------------------------------------|-----|
| Readability grade | [MDS023][mds023] (ARI)          | [metric][vale-metric] ext             | yes |
| Sentence limits   | [MDS024][mds024]                | [occurrence][vale-occurrence] ext     | yes |
| Word choice       | no                              | [substitution][vale-substitution] ext | yes |
| Passive voice     | no                              | [existence][vale-existence] ext       | yes |
| Jargon detection  | no                              | [existence][vale-existence] ext       | yes |
| Conciseness       | [MDS029][mds029] (experimental) | no                                    | yes |
| Tone enforcement  | no                              | custom styles                         | yes |
| Token budget      | [MDS028][mds028]                | no                                    | no  |
| Deterministic     | yes                             | yes                                   | no  |

mdsmith focuses on measurable readability metrics (ARI
grade, sentence count, token budget). Vale excels at style
guide enforcement. LLMs handle subjective quality best but
lack determinism.

### Formatting and Fixing

| Capability         | mdsmith               | Prettier                 | markdownlint |
|--------------------|-----------------------|--------------------------|--------------|
| Autofix CLI        | `fix`                 | `--write`                | `--fix`      |
| Table alignment    | [MDS025][mds025]      | yes                      | no           |
| Prose wrapping     | no                    | [`proseWrap`][prosewrap] | no           |
| Embedded code fmt  | no                    | JS/TS/CSS/JSON           | no           |
| Multi-pass fix     | yes                   | single pass              | single pass  |
| Generated sections | catalog, include, toc | no                       | no           |

Prose wrapping controls whether a tool reflows paragraph
line breaks. Prettier's [`proseWrap`][prosewrap] option
has three modes: `always` (wrap to print width), `never`
(unwrap to one line per paragraph), and `preserve` (leave
as-is, the default). Neither mdsmith nor markdownlint
reflow prose; they only diagnose long lines.

Prettier is the strongest pure formatter. mdsmith has
unique autofix for generated content (catalog, include, toc).
markdownlint fixes structural violations.

### Cross-File and Project Features

| Capability           | mdsmith              | markdownlint | remark-lint                     |
|----------------------|----------------------|--------------|---------------------------------|
| Link integrity       | [MDS027][mds027]     | no           | [remark-validate-links][rl-vl]  |
| Include sections     | [MDS021][mds021]     | no           | no                              |
| Catalog generation   | [MDS019][mds019]     | no           | no                              |
| Required structure   | [MDS020][mds020]     | no           | no                              |
| Front-matter query   | `mdsmith list query` | no           | no                              |
| Git merge driver     | yes                  | no           | no                              |
| Metrics ranking      | yes                  | no           | no                              |
| Gitignore aware      | yes                  | yes          | no                              |
| Front matter support | yes                  | via plugin   | via [remark-frontmatter][rl-fm] |

mdsmith has the strongest cross-file and project-level
features. The merge driver and regenerable sections are
unique to mdsmith. The `list query` subcommand
([plan 78][plan78]) selects files by a CUE expression
over front matter (e.g.
`mdsmith list query 'status: "✅"' plan/`), which no other
tool in this comparison offers natively.

### Renderer Portability

Several Markdown renderers expand non-standard
tokens into tables of contents. Common
variants are `[TOC]` (Python-Markdown),
`[[_TOC_]]` (GitLab, Azure DevOps), `[[toc]]`
(markdown-it, VitePress), and `${toc}` (some
VitePress configs). CommonMark and goldmark —
the engine mdsmith uses — expand none of
them. They render as literal text.

[MDS035][mds035] (toc-directive, opt-in) flags
each of the four tokens on its own line. For
`[TOC]`, the rule suppresses the diagnostic
when a matching link reference definition
makes it a legitimate link. No other linter
in this comparison detects these tokens.

`mdsmith fix` replaces each token with a
`<?toc?>...<?/toc?>` block ([MDS038][mds038]).
A second fix pass populates the block with a
nested heading list.

### Runtime and Integration

| Property       | mdsmith    | markdownlint   | remark-lint  | Prettier     | Vale       | textlint     | LLM         |
|----------------|------------|----------------|--------------|--------------|------------|--------------|-------------|
| Language       | Go         | Node.js        | Node.js      | Node.js      | Go         | Node.js      | API         |
| Runtime deps   | none       | Node 20+       | Node 16+     | Node 16+     | none       | Node 20+     | network     |
| Install        | binary     | npm            | npm          | npm          | binary     | npm          | varies      |
| Config format  | YAML       | JSONC/YAML/JS  | JSON/YAML/JS | JSON/YAML/JS | INI+YAML   | JSON/YAML/JS | prompt      |
| Output formats | text, JSON | text, JSON     | text         | none         | text, JSON | text, JSON   | text        |
| VS Code        | yes (LSP)  | yes            | yes          | yes          | yes        | yes          | varies      |
| GitHub Action  | no         | yes            | via npm      | via npm      | yes        | via npm      | custom      |
| Pre-commit     | lefthook   | husky/lefthook | husky        | husky        | hooks      | husky        | impractical |
| Offline        | yes        | yes            | yes          | yes          | yes        | yes          | no          |
| Deterministic  | yes        | yes            | yes          | yes          | yes        | yes          | no          |

Go-based tools (mdsmith, Vale) have zero runtime
dependencies. Node.js tools require a runtime but benefit
from the npm ecosystem. LLM-based linting requires network
access and is non-deterministic.

## Benchmarks

We ran our own benchmark, not re-quoted READMEs. It uses
hyperfine over two corpora; the full method is in the
[benchmark doc][bench]. Numbers come from its output:

<?include
file: ../research/benchmarks/results.fragment.md
?>
<!-- Generated by docs/research/benchmarks/gen_fragments.py from
docs/research/benchmarks/data/*.json — do not edit by hand. Re-run
the harness (run.sh) and `mdsmith fix` to refresh. -->

`mdsmith` is the default rule set; `mdsmith-parity` disables the
mdsmith-only rules so the work class matches the markdownlint
tools (see `bench-parity.mdsmith.yml`).

**Repo corpus — 523 Markdown files** (median wall time, lower is
better; `vs mado` is the median ratio to the fastest tool):

| Tool              | Median  | Min     | vs mado |
|-------------------|---------|---------|---------|
| mdsmith-parity    | 38 ms   | 35 ms   | 1.0x    |
| mado              | 40 ms   | 38 ms   | 1.0x    |
| panache           | 78 ms   | 73 ms   | 2.0x    |
| rumdl             | 79 ms   | 75 ms   | 2.1x    |
| mdsmith           | 195 ms  | 185 ms  | 5.1x    |
| markdownlint-cli2 | 2127 ms | 2100 ms | 56x     |

**Neutral corpus — 234 files** (Rust Book + Rust Reference,
longer third-party prose):

| Tool              | Median  | Min     | vs mado |
|-------------------|---------|---------|---------|
| mado              | 38 ms   | 35 ms   | 1.0x    |
| mdsmith-parity    | 73 ms   | 71 ms   | 1.9x    |
| rumdl             | 73 ms   | 68 ms   | 1.9x    |
| mdsmith           | 128 ms  | 124 ms  | 3.4x    |
| panache           | 158 ms  | 153 ms  | 4.2x    |
| markdownlint-cli2 | 1764 ms | 1669 ms | 46x     |
<?/include?>

Every native binary beats the Node baseline — default
mdsmith is about 4x faster than markdownlint-cli2. It is the
slowest native tool here because it also walks the
cross-file graph, scores readability, and validates
generated sections. With those mdsmith-only rules off (the
`mdsmith-parity` row) it is about 2.5x faster, within ~2x of
rumdl; closing that last gap is active work. So the read is
fit today: pick mado or rumdl for raw markdownlint
throughput, panache for Quarto or R Markdown, mdsmith for
the cross-file and self-maintaining-section layer.

## When to Use What

**mdsmith** fits best when you need readability limits,
token budgets, or generated content sections. Its single
binary makes CI setup simple.

**markdownlint** is the safe default for teams already in the
Node.js ecosystem. Widest community adoption, most editor
integrations, battle-tested rule set.

**remark-lint** suits projects deep in the unified/remark
ecosystem (MDX, Gatsby, Next.js). Its AST pipeline enables
custom transformations beyond linting.

**Prettier** is a formatter, not a linter. Use it alongside a
linter. Pair with markdownlint (using the Prettier compat
preset) or remark-lint for structural checks.

**Vale** is the right choice for enforcing prose style guides
(Microsoft, Google, AP). It complements structural linters
rather than replacing them.

**textlint** works well for polyglot text linting (especially
Japanese) and teams wanting ESLint-style modularity.

**rumdl** is the pick when you want markdownlint's exact
rules and config. You get them as one fast Rust binary,
with autofix and an LSP. It is a drop-in speed upgrade for
a Node markdownlint setup.

**mado** fits a check-only CI gate. It just needs
markdownlint rules run as fast as it can. There is no
autofix, LSP, or front-matter support, and the gate does
not need them.

**panache** is the right choice for Quarto and R Markdown.
Its lossless CST keeps the Pandoc syntax that Prettier and
mdformat flatten. It bundles the formatter, linter, and LSP
for those formats.

**LLM as linter** is best for subjective quality checks:
conciseness, clarity, tone. Use it in PR review workflows
where latency and cost are acceptable. Pair with a
deterministic linter for structural rules.

## Combining Tools

Most teams benefit from layering tools. Common pairings:

- **Structure + format**: markdownlint + Prettier
- **Structure + prose**: mdsmith + Vale
- **Structure + AI review**: mdsmith + LLM review in CI
- **Full stack**: mdsmith (structure + readability) + Vale
  (style) + Prettier (formatting)

mdsmith's conciseness-scoring rule ([MDS029][mds029]) is a
heuristic prototype aiming to bring LLM-grade quality
checks into an offline tool. Classifier-backed scoring is
a future step to bridge static rules and LLM review.

## Front Matter and Document Templates

Front matter (YAML between `---` delimiters) is a key
integration point. Tools handle it differently.

**mdsmith** uses front matter in three rules:

- **catalog ([MDS019][mds019])**: reads front matter
  fields from matched files to build summary tables.
  Fields become template variables (`{title}`,
  `{status}`).
- **required-structure ([MDS020][mds020])**: validates
  document headings and front matter against a template.
  Supports CUE schemas for field types and constraints.
- **include ([MDS021][mds021])**: strips front matter from
  included files by default (`strip-frontmatter: true`).

mdsmith also provides **proto files** as templates for
rule and metric docs. The proto defines required front
matter fields (id, name, status, description) with CUE
validation patterns, required heading structure, and
content guidelines. Every rule README is validated against
its proto via the [required-structure][mds020] rule.

[MDS020][mds020] validates front matter fields against CUE
schemas embedded in templates. There is no standalone rule
that validates front matter without also checking heading
structure.

**markdownlint** has no built-in front matter awareness.
It strips front matter to avoid false positives but does
not inspect its content. Custom rules can access it.

**remark-lint** supports front matter via the
[remark-frontmatter][rl-fm] plugin. Rules can then inspect
the parsed YAML. No built-in validation rules exist.

**Prettier** preserves front matter blocks but does not
format or validate their content.

**Vale** is front-matter-aware: it skips YAML blocks to
avoid false positives on metadata fields.

## Progressive Disclosure

mdsmith's catalog rule ([MDS019][mds019]) implements
progressive disclosure for documentation sets. A summary
table gives readers the overview; each row links to the
full document for details. Running `mdsmith fix` keeps the
table in sync with source front matter.

This pattern is useful for large repos where readers need
to find the right document without reading everything.
No other linter in this comparison generates or maintains
navigational tables from document metadata.

## Markdown Include / Preprocessor Tools

Several tools provide file inclusion for Markdown. All are
preprocessors: they transform source files at build time,
producing a separate output file.

| Tool                  | Language | Include syntax          | Stars |
|-----------------------|----------|-------------------------|-------|
| [markdown-include][]  | Python   | `{!filename!}`          | ~100  |
| [MarkdownPP][]        | Python   | `!INCLUDE "file.md"`    | ~350  |
| [Markedpp][]          | Node.js  | `!include(file.md)`     | ~50   |
| [MyST Markdown][myst] | Python   | `{include} directive`   | ~400  |
| [Gitdown][]           | Node.js  | `<<< file.md`           | ~460  |
| [mdpre][]             | Python   | preprocessor directives | ~20   |

Key differences from mdsmith's include rule
([MDS021][mds021]):

- **Build step required.** Preprocessors read source files
  and write transformed output. The source and output are
  different files. mdsmith regenerates included content
  in place — the source file is always valid Markdown.
- **No validation.** Preprocessors replace directives with
  file contents but do not lint the result. mdsmith's
  include rule validates that included sections stay in
  sync and auto-fixes drift via `mdsmith fix`.
- **Not agent-friendly.** Agents read and write the same
  file. A preprocessor build step adds friction: the
  agent must know to run the preprocessor after editing,
  and the included content is invisible in the source.
  With mdsmith, the included content lives in the source
  file and is always readable.

## Slidev and Presentation Markdown

Slidev uses Markdown files as slide decks. Slides are
split by `---` lines, with YAML front matter for config.
No tool in this comparison has Slidev support:

- Linters treat `---` separators as horizontal rules,
  which may trigger false positives
- Front matter blocks between slides may confuse parsers
  that expect a single front matter block at file start
- Slidev-specific directives (layout, clicks, transitions)
  appear as YAML or HTML comments that linters ignore

Teams using Slidev alongside standard Markdown docs
should use separate config overrides (e.g. ignore or
relaxed rules) for presentation files.

## Security Posture

A Markdown linter parses untrusted input from any
contributor. It also walks the repo file tree.
Adversarial input has caused OOMs, YAML
billion-laughs expansion, ANSI escape injection,
symlink escapes, and path traversal in the wider
linter ecosystem.

mdsmith ran a
[10-finding adversarial review][mdsmith-sec]. It
covered the parser, front-matter loader, terminal
output, file walker, include directive, and CUE
schemas. Findings were addressed in
[plan 83][plan83] (security hardening batch) and
[plan 84][plan84] (symlink default-deny). The
current posture:

| Hardening                          | mdsmith                | markdownlint    | remark-lint      | Prettier         | Vale      |
|------------------------------------|------------------------|-----------------|------------------|------------------|-----------|
| File-size cap on input             | yes                    | no              | no               | no               | no        |
| YAML billion-laughs guard          | yes (alias rejection)  | n/a (no FM)     | parser-dependent | parser-dependent | n/a       |
| ANSI escape sanitization           | yes                    | no              | no               | n/a              | no        |
| Symlinks denied by default         | yes                    | follows         | follows          | follows          | follows   |
| Cross-file links sandboxed to repo | yes ([MDS027][mds027]) | n/a             | plugin-dependent | n/a              | n/a       |
| Include size cap                   | yes                    | n/a             | n/a              | n/a              | n/a       |
| Schema/CUE path validation         | yes ([MDS020][mds020]) | n/a             | n/a              | n/a              | n/a       |
| Atomic writes in fix mode          | yes                    | n/a             | n/a              | yes              | n/a       |
| Network access at runtime          | none                   | none            | none             | none             | none      |
| Dependency surface                 | Go stdlib + goldmark   | Node + npm tree | Node + npm tree  | Node + npm tree  | Go stdlib |

Two structural properties reduce the attack surface
relative to Node.js linters:

- **Single static binary.** No runtime package
  resolution, no `node_modules` to audit.
  Supply-chain risk is the Go module graph in
  `go.mod`, reviewed at upgrade time.
- **No network calls.** Rule docs, schemas, and
  tokenizers ship inside the binary. CI does not need
  outbound network for linting, and adversarial
  Markdown cannot exfiltrate via SSRF.

Teams handling untrusted Markdown (PRs from external
contributors, user-submitted content) should treat the
linter as a parser of untrusted input. mdsmith aims to
fail safely on adversarial input rather than crash or
escape the repo root.

## Future Plans

Open work is tracked in [PLAN.md](../../PLAN.md). The
items most relevant to this comparison are:

- **Build subsystem** (plans
  [101][plan101], [102][plan102], [103][plan103],
  [104][plan104]) — a `mdsmith build` subcommand with
  a `<?build?>` directive, staleness tracking, and
  lifecycle hooks. This will close part of the gap
  with Hugo: deriving artifacts from Markdown sources
  without leaving the linter.
- **Closing rule gaps with markdownlint** — 6 rules remain
  unimplemented: [plan 172](../../plan/172_link-style-rule-and-config.md)
  covers MD054, and plans 181-182 schedule the remaining 5
  (table structure; code-block style). MDS062
  (`link-validity`) now covers markdownlint MD011 and MD042;
  MDS064 (`atx-heading-whitespace`) covers MD018-MD021 and
  MD023; MDS063 (`descriptive-link-text`) covers MD059.
  The [coverage matrix][mdcov] tracks each.
- **User-defined Markdown conventions**
  ([plan 113][plan113]) — let teams package their own
  rule presets the way the built-in conventions
  ([reference][conventions]) do today.
- **Glob unification** ([plan 120][plan120]) — one
  glob matcher across config, directives, and CLI
  arguments.
- **Recipe-safety rule MDS040 and a build-config
  block** ([plan 100][plan100]) — guard rails on the
  forthcoming build directive so it cannot run
  arbitrary commands without explicit opt-in.

Pin a version (`go install github.com/jeduden/mdsmith/cmd/mdsmith@vX.Y.Z`) if
you need a stable rule set while these land.

<!-- mdsmith rule links -->
[mds001]: ../../internal/rules/MDS001-line-length/README.md
[mds019]: ../../internal/rules/MDS019-catalog/README.md
[mds020]: ../../internal/rules/MDS020-required-structure/README.md
[mds021]: ../../internal/rules/MDS021-include/README.md
[mds023]: ../../internal/rules/MDS023-paragraph-readability/README.md
[mds024]: ../../internal/rules/MDS024-paragraph-structure/README.md
[mds025]: ../../internal/rules/MDS025-table-format/README.md
[mds027]: ../../internal/rules/MDS027-cross-file-reference-integrity/README.md
[mds067]: ../../internal/rules/MDS067-callout-type/README.md
[mds028]: ../../internal/rules/MDS028-token-budget/README.md
[mds029]: ../../internal/rules/MDS029-conciseness-scoring/README.md
[mds030]: ../../internal/rules/MDS030-empty-section-body/README.md
[mds035]: ../../internal/rules/MDS035-toc-directive/README.md
[mds038]: ../../internal/rules/MDS038-toc/README.md
<!-- markdownlint links -->
[markdownlint]: https://github.com/DavidAnson/markdownlint
[markdownlint-cli2]: https://github.com/DavidAnson/markdownlint-cli2
[markdownlint-github]: https://github.com/github/markdownlint-github
[mdl-vscode]: https://marketplace.visualstudio.com/items?itemName=DavidAnson.vscode-markdownlint
[mdl-action]: https://github.com/DavidAnson/markdownlint-cli2-action
[md001]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md001.md
[md058]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md058.md
<!-- remark-lint links -->
[remark-lint]: https://github.com/remarkjs/remark-lint
[remark]: https://github.com/remarkjs/remark
[unified]: https://github.com/unifiedjs/unified
[rl-vl]: https://github.com/remarkjs/remark-validate-links
[rl-fm]: https://github.com/remarkjs/remark-frontmatter
<!-- include / preprocessor tool links -->
[markdown-include]: https://github.com/cmacmackin/markdown-include
[MarkdownPP]: https://github.com/amyreese/markdown-pp
[Markedpp]: https://github.com/commenthol/markedpp
[myst]: https://mystmd.org/guide/embed
[Gitdown]: https://github.com/gajus/gitdown
[mdpre]: https://github.com/MartinPacker/mdpre
<!-- prettier, vale, textlint, llm tool links -->
[Prettier]: https://prettier.io/
[prosewrap]: https://prettier.io/docs/options#prose-wrap
[Vale]: https://github.com/errata-ai/vale
[vale-checks]: https://vale.sh/docs/checks/existence
[vale-existence]: https://vale.sh/docs/checks/existence
[vale-substitution]: https://vale.sh/docs/checks/substitution
[vale-occurrence]: https://vale.sh/docs/checks/repetition
[vale-metric]: https://vale.sh/docs/checks/metric
[vale-action]: https://github.com/errata-ai/vale-action
[vale-vscode]: https://marketplace.visualstudio.com/items?itemName=errata-ai.vale-server
[textlint]: https://github.com/textlint/textlint
[textlint-mcp]: https://textlint.org/docs/mcp/
[VectorLint]: https://github.com/TRocket-Labs/vectorlint
[GPTLint]: https://github.com/gptlint/gptlint
[GritQL]: https://docs.grit.io/language/overview
[gptlint-cost]: https://gptlint.dev/project/cost
[copilot-review]: https://docs.github.com/copilot/using-github-copilot/code-review/using-copilot-code-review
[CodeRabbit]: https://www.coderabbit.ai/
[claude-action]: https://github.com/anthropics/claude-code-action
[CoEdIT]: https://github.com/vipulraheja/coedit
[coedit-paper]: https://arxiv.org/abs/2305.09857
<!-- hugo links -->
[Hugo]: https://gohugo.io/
[hugo-shortcodes]: https://gohugo.io/content-management/shortcodes/
[hugo-migration]: ../guides/directives/hugo-migration.md
<!-- obsidian links -->
[Obsidian]: https://obsidian.md/
[obsidian-fm]: https://help.obsidian.md/Editing+and+formatting/Obsidian+Flavored+Markdown
[obsidian-linter]: https://github.com/platers/obsidian-linter
<!-- mdbase links -->
[mdbase]: https://mdbase.dev/
[mdbase-deep-dive]: ../research/mdbase-vs-mdsmith/README.md
<!-- rust markdown linter links -->
[rumdl]: https://github.com/rvben/rumdl
[mado]: https://github.com/akiomik/mado
[panache]: https://panache.bz/
<!-- mdsmith plan + security + reference links -->
[mdsmith-sec]: ../security/2026-04-05-adversarial-markdown.md
[conventions]: ../reference/conventions.md
[bench]: ../research/benchmarks/README.md
[mdcov]: ../research/markdownlint-coverage/README.md
[plan78]: ../../plan/78_query-command.md
[plan83]: ../../plan/83_security-hardening-batch.md
[plan84]: ../../plan/84_symlink-default-deny.md
[plan100]: ../../plan/100_build-config-and-mds040.md
[plan101]: ../../plan/101_build-directive-mds039.md
[plan102]: ../../plan/102_build-subcommand.md
[plan103]: ../../plan/103_build-staleness-and-deps.md
[plan104]: ../../plan/104_build-lifecycle-hooks.md
[plan113]: ../../plan/113_user-defined-profiles.md
[plan120]: ../../plan/120_glob-unification.md
