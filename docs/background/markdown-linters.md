---
summary: Comparison of mdsmith with other Markdown linters and formatters.
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
- Regenerable sections: catalog, include,
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

### LLM as Linter

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

| Capability          | mdsmith                        | markdownlint                             | remark-lint                     |
|---------------------|--------------------------------|------------------------------------------|---------------------------------|
| Heading hierarchy   | [MDS003][mds003]               | [MD001][md001]                           | [heading-increment][rl-hi]      |
| First-line heading  | [MDS004][mds004]               | [MD041][md041]                           | [first-heading-level][rl-fhl]   |
| Duplicate headings  | [MDS005][mds005]               | [MD024][md024]                           | [no-duplicate-headings][rl-ndh] |
| Blank line spacing  | [MDS013][mds013]-[015][mds015] | [MD022][md022],[025][md025],[031][md031] | plugins                         |
| List indentation    | [MDS016][mds016]               | [MD007][md007]                           | [list-item-indent][rl-lii]      |
| Code fence style    | [MDS010][mds010]               | [MD048][md048]                           | [fenced-code-flag][rl-fcf]      |
| Code block language | [MDS011][mds011]               | [MD040][md040]                           | [fenced-code-flag][rl-fcf]      |
| Bare URLs           | [MDS012][mds012]               | [MD034][md034]                           | [no-literal-urls][rl-nlu]       |
| Line length         | [MDS001][mds001]               | [MD013][md013]                           | [maximum-line-length][rl-mll]   |
| Trailing spaces     | [MDS006][mds006]               | [MD009][md009]                           | [hard-break-spaces][rl-hbs]     |

All three cover core structural rules. markdownlint has
the broadest rule set (~60 rules). mdsmith and remark-lint
are comparable in structural coverage.

### Rules mdsmith Lacks

markdownlint has ~30 rules without mdsmith equivalents.
Notable gaps:

| Rule area         | markdownlint                   | remark-lint                                       |
|-------------------|--------------------------------|---------------------------------------------------|
| Inline HTML       | [MD033][md033] no-inline-html  | [no-html][rl-nh]                                  |
| Image alt text    | [MD045][md045] no-alt-text     | [no-empty-image-alt-text][rl-neiat] (third-party) |
| OL numbering      | [MD029][md029] ol-prefix       | [ordered-list-marker-style][rl-olms]              |
| UL marker style   | [MD004][md004] ul-style        | [unordered-list-marker-style][rl-ulms]            |
| Emphasis style    | [MD049][md049], [MD050][md050] | [emphasis-marker][rl-em]                          |
| HR style          | [MD035][md035] hr-style        | [rule-style][rl-rs]                               |
| Space in emphasis | [MD037][md037]                 | no                                                |
| Space in code     | [MD038][md038]                 | no                                                |
| Space in links    | [MD039][md039]                 | no                                                |
| Proper names      | [MD044][md044]                 | no                                                |
| Required headings | [MD043][md043]                 | no                                                |
| Single H1         | [MD047][md047]                 | no                                                |
| Link fragments    | [MD051][md051]                 | no                                                |
| Reference links   | [MD052][md052], [MD053][md053] | no                                                |

mdsmith covers readability, tokens, and generated content
rather than formatting details. Teams that need full
coverage can pair mdsmith with markdownlint.

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

| Capability         | mdsmith          | Prettier                 | markdownlint |
|--------------------|------------------|--------------------------|--------------|
| Autofix CLI        | `fix`            | `--write`                | `--fix`      |
| Table alignment    | [MDS025][mds025] | yes                      | no           |
| Prose wrapping     | no               | [`proseWrap`][prosewrap] | no           |
| Embedded code fmt  | no               | JS/TS/CSS/JSON           | no           |
| Multi-pass fix     | yes              | single pass              | single pass  |
| Generated sections | catalog, include | no                       | no           |

Prose wrapping controls whether a tool reflows paragraph
line breaks. Prettier's [`proseWrap`][prosewrap] option
has three modes: `always` (wrap to print width), `never`
(unwrap to one line per paragraph), and `preserve` (leave
as-is, the default). Neither mdsmith nor markdownlint
reflow prose; they only diagnose long lines.

Prettier is the strongest pure formatter. mdsmith has
unique autofix for generated content (catalog, include).
markdownlint fixes structural violations.

### Cross-File and Project Features

| Capability           | mdsmith          | markdownlint | remark-lint                     |
|----------------------|------------------|--------------|---------------------------------|
| Link integrity       | [MDS027][mds027] | no           | [remark-validate-links][rl-vl]  |
| Include sections     | [MDS021][mds021] | no           | no                              |
| Catalog generation   | [MDS019][mds019] | no           | no                              |
| Required structure   | [MDS020][mds020] | no           | no                              |
| Git merge driver     | yes              | no           | no                              |
| Metrics ranking      | yes              | no           | no                              |
| Gitignore aware      | yes              | yes          | no                              |
| Front matter support | yes              | via plugin   | via [remark-frontmatter][rl-fm] |

mdsmith has the strongest cross-file and project-level
features. The merge driver and regenerable sections are
unique to mdsmith.

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

### Runtime and Integration

| Property       | mdsmith    | markdownlint   | remark-lint  | Prettier     | Vale       | textlint     | LLM         |
|----------------|------------|----------------|--------------|--------------|------------|--------------|-------------|
| Language       | Go         | Node.js        | Node.js      | Node.js      | Go         | Node.js      | API         |
| Runtime deps   | none       | Node 20+       | Node 16+     | Node 16+     | none       | Node 20+     | network     |
| Install        | binary     | npm            | npm          | npm          | binary     | npm          | varies      |
| Config format  | YAML       | JSONC/YAML/JS  | JSON/YAML/JS | JSON/YAML/JS | INI+YAML   | JSON/YAML/JS | prompt      |
| Output formats | text, JSON | text, JSON     | text         | none         | text, JSON | text, JSON   | text        |
| VS Code        | no         | yes            | yes          | yes          | yes        | yes          | varies      |
| GitHub Action  | no         | yes            | via npm      | via npm      | yes        | via npm      | custom      |
| Pre-commit     | lefthook   | husky/lefthook | husky        | husky        | hooks      | husky        | impractical |
| Offline        | yes        | yes            | yes          | yes          | yes        | yes          | no          |
| Deterministic  | yes        | yes            | yes          | yes          | yes        | yes          | no          |

Go-based tools (mdsmith, Vale) have zero runtime
dependencies. Node.js tools require a runtime but benefit
from the npm ecosystem. LLM-based linting requires network
access and is non-deterministic.

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

<!-- mdsmith rule links -->
[mds001]: ../../internal/rules/MDS001-line-length/README.md
[mds003]: ../../internal/rules/MDS003-heading-increment/README.md
[mds004]: ../../internal/rules/MDS004-first-line-heading/README.md
[mds005]: ../../internal/rules/MDS005-no-duplicate-headings/README.md
[mds006]: ../../internal/rules/MDS006-no-trailing-spaces/README.md
[mds010]: ../../internal/rules/MDS010-fenced-code-style/README.md
[mds011]: ../../internal/rules/MDS011-fenced-code-language/README.md
[mds012]: ../../internal/rules/MDS012-no-bare-urls/README.md
[mds013]: ../../internal/rules/MDS013-blank-line-around-headings/README.md
[mds015]: ../../internal/rules/MDS015-blank-line-around-fenced-code/README.md
[mds016]: ../../internal/rules/MDS016-list-indent/README.md
[mds019]: ../../internal/rules/MDS019-catalog/README.md
[mds020]: ../../internal/rules/MDS020-required-structure/README.md
[mds021]: ../../internal/rules/MDS021-include/README.md
[mds023]: ../../internal/rules/MDS023-paragraph-readability/README.md
[mds024]: ../../internal/rules/MDS024-paragraph-structure/README.md
[mds025]: ../../internal/rules/MDS025-table-format/README.md
[mds027]: ../../internal/rules/MDS027-cross-file-reference-integrity/README.md
[mds028]: ../../internal/rules/MDS028-token-budget/README.md
[mds029]: ../../internal/rules/MDS029-conciseness-scoring/README.md
[mds030]: ../../internal/rules/MDS030-empty-section-body/README.md
[mds035]: ../../internal/rules/MDS035-toc-directive/README.md
<!-- markdownlint links -->
[markdownlint]: https://github.com/DavidAnson/markdownlint
[markdownlint-cli2]: https://github.com/DavidAnson/markdownlint-cli2
[markdownlint-github]: https://github.com/github/markdownlint-github
[mdl-vscode]: https://marketplace.visualstudio.com/items?itemName=DavidAnson.vscode-markdownlint
[mdl-action]: https://github.com/DavidAnson/markdownlint-cli2-action
[md001]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md001.md
[md004]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md004.md
[md007]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md007.md
[md009]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md009.md
[md013]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md013.md
[md022]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md022.md
[md024]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md024.md
[md025]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md025.md
[md029]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md029.md
[md031]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md031.md
[md033]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md033.md
[md034]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md034.md
[md035]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md035.md
[md037]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md037.md
[md038]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md038.md
[md039]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md039.md
[md040]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md040.md
[md041]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md041.md
[md043]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md043.md
[md044]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md044.md
[md045]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md045.md
[md047]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md047.md
[md048]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md048.md
[md049]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md049.md
[md050]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md050.md
[md051]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md051.md
[md052]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md052.md
[md053]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md053.md
[md058]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md058.md
<!-- remark-lint links -->
[remark-lint]: https://github.com/remarkjs/remark-lint
[remark]: https://github.com/remarkjs/remark
[unified]: https://github.com/unifiedjs/unified
[rl-hi]: https://github.com/remarkjs/remark-lint/tree/main/packages/remark-lint-heading-increment
[rl-fhl]: https://github.com/remarkjs/remark-lint/tree/main/packages/remark-lint-first-heading-level
[rl-ndh]: https://github.com/remarkjs/remark-lint/tree/main/packages/remark-lint-no-duplicate-headings
[rl-lii]: https://github.com/remarkjs/remark-lint/tree/main/packages/remark-lint-list-item-indent
[rl-fcf]: https://github.com/remarkjs/remark-lint/tree/main/packages/remark-lint-fenced-code-flag
[rl-nlu]: https://github.com/remarkjs/remark-lint/tree/main/packages/remark-lint-no-literal-urls
[rl-mll]: https://github.com/remarkjs/remark-lint/tree/main/packages/remark-lint-maximum-line-length
[rl-hbs]: https://github.com/remarkjs/remark-lint/tree/main/packages/remark-lint-hard-break-spaces
[rl-nh]: https://github.com/remarkjs/remark-lint/tree/main/packages/remark-lint-no-html
[rl-neiat]: https://github.com/salesforce/remark-lint-no-empty-image-alt-text
[rl-olms]: https://github.com/remarkjs/remark-lint/tree/main/packages/remark-lint-ordered-list-marker-style
[rl-ulms]: https://github.com/remarkjs/remark-lint/tree/main/packages/remark-lint-unordered-list-marker-style
[rl-em]: https://github.com/remarkjs/remark-lint/tree/main/packages/remark-lint-emphasis-marker
[rl-rs]: https://github.com/remarkjs/remark-lint/tree/main/packages/remark-lint-rule-style
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
