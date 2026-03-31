# 🔨 mdsmith

A fast, auto-fixing Markdown linter and formatter for docs, READMEs,
and AI-generated content. Checks style, readability, and structure.
Written in Go.

## ✨ Why mdsmith

**📋 Progressive disclosure with catalogs.**
The [`catalog`](internal/rules/MDS019-catalog/README.md) rule generates summary
tables from file front matter and keeps them in sync.
Link each row to the full document —
readers see the overview first and drill down on demand.
Run `mdsmith fix` and the table updates itself.

**🤖 Keep AI verbosity in check.**
AI tools produce walls of text.
[`max-file-length`](internal/rules/MDS022-max-file-length/README.md)
caps document size,
[`paragraph-readability`](internal/rules/MDS023-paragraph-readability/README.md)
enforces a reading-grade ceiling,
and [`paragraph-structure`](internal/rules/MDS024-paragraph-structure/README.md)
limits sentence count and length.
[`token-budget`](internal/rules/MDS028-token-budget/README.md)
adds a token-aware
budget with heuristic and tokenizer modes.
Set the thresholds in `.mdsmith.yml` and let CI enforce them.

**📖 AI-ready rule specs — no remote calls.**
`mdsmith help rule` lists every rule with its ID and description.
`mdsmith help rule <name>` prints the full spec: settings, examples,
diagnostics. All docs are compiled into the binary — works offline,
works in CI, works as a source for `.cursor/rules` or `AGENTS.md`.
`mdsmith help metrics` and `mdsmith help metrics <name>` do the same
for shared file metrics.

**🔧 Auto-fix.**
`mdsmith fix` corrects most rules in place.
Whitespace, heading style, code fences, bare URLs, list indentation,
table alignment, and generated sections — all handled.
Multi-pass fixing resolves cascading changes automatically.

## 📦 Installation

```bash
go install github.com/jeduden/mdsmith/cmd/mdsmith@latest
```

## 🚀 Usage

```text
mdsmith <command> [flags] [files...]
```

### Commands

| Command        | Description                                    |
|----------------|------------------------------------------------|
| `check`        | Lint files (default command)                   |
| `fix`          | Auto-fix issues in place                       |
| `query`        | Select files by CUE expression on front matter |
| `help`         | Show help for docs and topics                  |
| `metrics`      | List and rank Markdown metrics                 |
| `merge-driver` | Git merge driver for regenerable sections      |
| `init`         | Generate `.mdsmith.yml`                        |
| `version`      | Print version, exit                            |

Files can be paths, directories (walked recursively for `*.md`),
or glob patterns.
With no arguments and no piped input, mdsmith exits 0.

When walking directories, mdsmith respects `.gitignore` files by default.
Files matched by `.gitignore` patterns are skipped, including patterns from
nested `.gitignore` files in subdirectories and ancestor directories.
Explicitly named file paths are never filtered by gitignore.
Use `--no-gitignore` to disable this behavior and lint all files.

### Flags

| Flag             | Description      |
|------------------|------------------|
| `-c`, `--config` | Config path      |
| `-f`, `--format` | `text` or `json` |
| `--no-color`     | Plain output     |
| `--no-gitignore` | Skip gitignore   |
| `-q`, `--quiet`  | Quiet mode       |

### Examples

```bash
mdsmith check docs/            # lint a directory
mdsmith fix README.md          # auto-fix in place
mdsmith check -f json docs/    # JSON output
mdsmith metrics rank --by bytes --top 10 .
```

### Output

Diagnostics are printed to stderr with source context when available:

```text
README.md:10:81 MDS001 line too long (135 > 80)
 8 | Context lines appear above and below the diagnostic with line numbers.
 9 | They help you see the surrounding code at a glance.
10 | This line is deliberately made long so it exceeds the eighty character limit and keeps going and going.
·····················································································^
11 | A dot path runs from column 1 to the caret, marking the line and column.
12 | Up to two lines of context are shown on each side.
```

Each diagnostic shows a header (`file:line:col rule message`).
When source context is available, up to 5 surrounding lines appear
with a dot path (`····^`) pointing to the exact column.

### Exit codes

| Code | Meaning                        |
|------|--------------------------------|
| 0    | No lint issues found           |
| 1    | Lint issues found              |
| 2    | Runtime or configuration error |

## ⚙️ Configuration

Create a `.mdsmith.yml` in your project root, or run
`mdsmith init` to generate one with every rule and its
default settings.
Without a config file, rules run with their built-in
defaults.

```yaml
rules:
  line-length:
    max: 120
  fenced-code-language: false

ignore:
  - "vendor/**"

overrides:
  - files: ["CHANGELOG.md"]
    rules:
      no-duplicate-headings: false
```

Rules can be `true` (enable with defaults), `false` (disable),
or an object with settings.
The `overrides` list applies different rules per file pattern.
Later overrides take precedence.

Config is discovered by walking up from the current directory to the repo root.
Use `--config` to override.

### Bootstrapping with `mdsmith init`

Run `mdsmith init` to generate a `.mdsmith.yml` with every rule and its
default enablement and settings. This pins the config to the current defaults so
that future
mdsmith upgrades (which may change defaults) do not silently alter your
lint results. Review the generated file and adjust settings to match your
project's conventions.

```bash
mdsmith init
# creates .mdsmith.yml with all rule defaults
```

Commit the generated file to version control.
This ensures every contributor uses the same rule settings.
Upgrades become an explicit, reviewable change.

## 📚 Guides

<?catalog
glob: "docs/guides/*.md"
sort: title
header: |
  | Guide | Description |
  |-------|-------------|
row: "| [{title}]({filename}) | {summary} |"
empty: |
  | Guide         | Description                                                  |
  |---------------|--------------------------------------------------------------|
  | No guides yet | Add guide files under `docs/guides/` to populate this index. |
?>
| Guide                                                                                           | Description                                                                              |
|-------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------|
| [Choosing Readability, Conciseness, and Token Budget Metrics](docs/guides/metrics-tradeoffs.md) | Trade-offs and threshold guidance for readability, structure, length, and token budgets. |
| [Guides](docs/guides/index.md)                                                                  | User guides for mdsmith directives, structure enforcement, and migration.                |
<?/catalog?>

## 📏 Rules

See the [Rule Directory](docs/reference/rule-directory.md)
for the complete list with status and description.

## 🛠️ Development

Requires Go 1.24+. See
[`docs/development/index.md`](docs/development/index.md) for the full
contributor guide (build commands, project layout,
workflow, code style, and PR conventions).

## 📂 Documentation

- [CLI design](docs/design/cli.md)
- [Design archetypes](docs/design/archetypes/)
- [Guides](docs/guides/)
- [Background](docs/background/)
- [Plans](plan/)

## 📄 License

[MIT](LICENSE)
