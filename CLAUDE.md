# CLAUDE.md

## Project

mdsmith — a Markdown linter written in Go.

## Docs

- [Plan template; see PLAN.md for status, plans live in plan/](plan/proto.md)

<?catalog
glob:
  - "docs/**/*.md"
  - "!docs/research/**"
  - "!docs/security/**"
  - "!docs/brand/**"
  - "!**/proto.md"
sort: path
header: ""
row: "- [{summary}]({filename})"
?>
- [The public `pkg/mdsmith` engine API — a `Session` that owns workspace, compiled config, and parse caches — and how it mirrors one-to-one as WebAssembly JavaScript bindings, including the open method namespace, the cache contract, and the WASM size budgets and limits.](docs/background/concepts/engine-api.md)
- [How "flavor" (a property of the renderer), "rule" (a single check), "convention" (a project-wide bundle), and "kind" (a per-file role tag) differ in mdsmith, the cases where they overlap, and how the four concepts compose.](docs/background/concepts/flavor-rule-convention-kind.md)
- [How generated sections work — markers, directives, and fix behavior.](docs/background/concepts/generated-section.md)
- [How the placeholder vocabulary lets rules treat template tokens as opaque rather than flagging them as content violations.](docs/background/concepts/placeholder-grammar.md)
- [The mental model behind mdsmith — how flavor, rule, convention, and kind relate, how generated sections work, the placeholder grammar, and how it compares to other Markdown linters.](docs/background/index.md)
- [How mdsmith compares to other Markdown linters.](docs/background/markdown-linters.md)
- [How to wire a new peer Markdown linter into mdsmith's comparison docs, the per-rule coverage matrix, and the benchmark page.](docs/development/add-peer-linter.md)
- [Running log of SOLID and clean-architecture findings on origin/main. The solid-architecture skill (audit mode) appends here; blockers are also filed as plans.](docs/development/architecture-audit.md)
- [Checklist for sweeping origin/main for SOLID and boundary violations. Records findings in the audit log; schedules blockers as new plan files.](docs/development/architecture/audit-checklist.md)
- [External-surface contracts: LSP, CLI, .mdsmith.yml, generated markers, plugin manifest, distribution shims. Public APIs.](docs/development/architecture/cross-system.md)
- [Go-specific SOLID and clean architecture patterns for mdsmith's cmd/ and internal/ packages.](docs/development/architecture/go.md)
- [SOLID and clean-architecture rules for mdsmith's Go core, TypeScript extension, and cross-system surfaces. Canonical home for the solid-architecture skill.](docs/development/architecture/index.md)
- [Four-layer test pyramid (unit, contract, integration, e2e) and the rule that every function ships with a dedicated unit test. Included from the Go and TypeScript architecture pages.](docs/development/architecture/tests.md)
- [SOLID and clean architecture patterns for the mdsmith VS Code extension at editors/vscode/.](docs/development/architecture/typescript.md)
- [Codecov coverage gate and CI status checks.](docs/development/coverage.md)
- [Where to place Markdown files and documentation types.](docs/development/file-placement.md)
- [Process and patterns for keeping mdsmith's Go core fast: the benchmark→profile→fix loop, the patterns to reach for, and the anti-patterns that have already cost the project real CPU and GC time.](docs/development/high-performance-go.md)
- [Build commands, project layout, code style, test fixtures, coverage gate, and merge conflicts.](docs/development/index.md)
- [The pkg/markdown public package: parse, produce, and its compatibility policy.](docs/development/markdown-library.md)
- [Label-driven merge queue workflow using jeduden/merge-queue-action.](docs/development/merge-queue.md)
- [Rebase, CI monitoring, and review comment resolution.](docs/development/pr-fixup-workflow.md)
- [A single-file `.flatpak` bundle built in CI from the x86_64 Linux release binary and attached to each GitHub release, installed by file with host filesystem access for the linter.](docs/development/release-channels/flatpak.md)
- [Per-platform mdsmith binaries plus the .vsix, the checksum file, and a Sigstore signature, attached to a tag-named release.](docs/development/release-channels/github-releases.md)
- [Root `@mdsmith/cli` plus one platform-specific subpackage per supported host, all published via OIDC Trusted Publishing.](docs/development/release-channels/npm.md)
- [The same `.vsix` republished to Open VSX so VSCodium, Cursor, Theia, and Gitpod can install it.](docs/development/release-channels/open-vsx.md)
- [One platform-tagged wheel per supported host, published via OIDC Trusted Publishing.](docs/development/release-channels/pypi.md)
- [The mdsmith VS Code extension `.vsix`, published via a long-lived Marketplace publisher PAT.](docs/development/release-channels/visual-studio-marketplace.md)
- [Every GitHub Actions workflow that needs runtime logic invokes the `mdsmith-release` Go CLI rather than carrying inline shell or per-language scripts. This page captures the rule and the subcommands it applies to.](docs/development/release-tooling.md)
- [How a maintainer-dispatched workflow run publishes mdsmith to npm, PyPI, the Visual Studio Marketplace, Open VSX, a Flatpak bundle, and GitHub Releases — the workflow structure, the OIDC trusted publishers it relies on, the `release` environment that gates every publishing job, the separate website deploy, and the supply-chain hardening features baked into the pipeline.](docs/development/release.md)
- [Rotation cadence and procedure for the long-lived publisher tokens consumed by the release and merge-queue workflows. Each tracked secret has its own file under `secret-rotations/`; the catalog below enumerates them. The scheduled reminder workflow consumes the same files and opens a GitHub issue when any secret is within 30 days of expiry.](docs/development/secret-rotations.md)
- [GitHub fine-grained PAT for the merge-queue action. Plain repo secret — not gated by an environment.](docs/development/secret-rotations/merge-queue-token.md)
- [Open VSX publisher token. Drives the `ovsx publish` step.](docs/development/secret-rotations/ovsx-pat.md)
- [Visual Studio Marketplace publisher PAT issued by Azure DevOps. Drives the `vsce publish` step.](docs/development/secret-rotations/vsce-pat.md)
- [Design rationale for `website/hugo.toml` — module mounts, Goldmark renderer flags, Chroma highlight style, and the version stamp shared with `mdsmith-release`. The TOML file itself is now machine-edited by `mdsmith-release sync-messaging`, which re-emits the file via the TOML library and drops inline comments. This page is the home for those comments.](docs/development/website-config.md)
- [`mdsmith fix` rewrites whitespace, headings, code fences, bare URLs, list indentation, and table alignment in place, looping up to 10 passes and stopping when edits stabilize. `mdsmith check` is the read-only CI sibling.](docs/features/auto-fix.md)
- [The `<?build?>` directive declares an artifact and a recipe. `mdsmith fix` keeps the section body in sync with the recipe output; `MDS040` shell-safety-checks the recipe without running it.](docs/features/build-artifacts.md)
- [Config layers deep-merge rule by rule: defaults, convention, kinds, then overrides. `--explain` and `mdsmith kinds resolve` show which layer set each effective value, per leaf.](docs/features/config-transparency.md)
- [Built-in rules flag broken links and missing anchors, enforce per-file section schemas, and keep Markdown in the right folders. Schemas can be inline on a file kind or shared via `proto.md` files.](docs/features/cross-file-integrity.md)
- [`mdsmith deps` lists what a file pulls in — includes, catalogs, build sources, and links — or, with `--incoming`, every file that points at it. The LSP call-hierarchy walks the same graph in your editor.](docs/features/dependency-graph.md)
- [A bundled VS Code extension and Claude Code plugins drive the same `mdsmith lsp` server, so diagnostics, fix-on-save, and navigation reach your editor and your coding agent unchanged.](docs/features/editor-agent-integration.md)
- [Tag each file with a `kind`, then validate its headings and front matter against a schema declared inline on the kind or shared via a `proto.md` template — so a whole directory obeys one contract.](docs/features/file-kinds-schemas.md)
- [A Git merge driver auto-resolves conflicts inside generated blocks, and a pre-merge-commit hook re-runs `mdsmith fix` and re-stages the result, so generated content never blocks a merge.](docs/features/git-native.md)
- [The mdsmith feature overview shared by the repository README and the website. Each capability links to a fuller page with rules and examples.](docs/features/index.md)
- [One version-stamped Go binary ships through go install, npm, pip, uvx, mise, asdf, and GitHub Releases — with no postinstall network call, so locked-down CI installs offline.](docs/features/install-everywhere.md)
- [`mdsmith lsp` emits diagnostics, quick-fixes, and navigation — definition, references, symbol search, and a call-hierarchy over `<?include?>`, `<?catalog?>`, and cross-file links — consumed by any LSP-aware editor.](docs/features/live-diagnostics.md)
- [Pin a Markdown convention to get a curated rule preset and a target renderer flavor in one switch. `MDS034` flags syntax the flavor will not render; a placeholder vocabulary spares template tokens.](docs/features/markdown-conventions.md)
- [A single static Go binary, no runtime to start. The workspace walk runs in parallel, embeds are linted once, and `check` is built for the hot path — roughly 4x faster than Node markdownlint, with a CI gate against regression.](docs/features/performance.md)
- [CI badge, Go Report Card grade, and Codecov coverage badge report live project health. mdsmith lints its own docs with the rules it ships, and a coverage gate blocks any merge that drops below the line.](docs/features/quality.md)
- [`mdsmith list query 'status: "✅"' plan/` selects files by a CUE expression on front matter; `mdsmith metrics rank` ranks files by any shared metric — both ready to pipe into a release script.](docs/features/release-gating.md)
- [Rename a heading and every workspace anchor link that points at it is rewritten in one atomic edit. Link-reference labels rename with their uses. A colliding slug fails loudly instead of silently breaking cross-file links.](docs/features/rename.md)
- [On `mdsmith fix`, `<?toc?>` rebuilds a heading TOC, `<?catalog?>` generates an index from front matter, and `<?include?>` splices in another file. A Git merge driver auto-resolves conflicts inside those blocks.](docs/features/self-maintaining-sections.md)
- [Cap file, section, and token-budget size; enforce reading grade and sentence count; flag verbatim copy-paste across files.](docs/features/size-and-readability.md)
- [Run mdsmith alongside Prettier by ordering `mdsmith fix` before `prettier --write` in the same pre-commit hook.](docs/guides/coexist-with-prettier.md)
- [Vale owns brand voice and prose style; remark owns Markdown AST transformations; mdsmith owns formatting, cross-file integrity, and generated sections. They sit side by side in CI without overlap.](docs/guides/coexist-with-vale-and-remark.md)
- [How to use the build directive to declare artifact outputs, keep generated bodies in sync, and configure user-declared recipes.](docs/guides/directives/build.md)
- [How to use schemas, require, and allow-empty-section to validate headings, front matter, and filenames.](docs/guides/directives/enforcing-structure.md)
- [How to use catalog and include directives to generate and embed content in Markdown files.](docs/guides/directives/generating-content.md)
- [Key differences between Hugo templates and mdsmith directives for users familiar with Hugo.](docs/guides/directives/hugo-migration.md)
- [Wire `mdsmith lsp` into Neovim's built-in LSP client so diagnostics, code actions, and navigation work inline with no extra plugin.](docs/guides/editors/neovim.md)
- [Install the mdsmith VS Code extension and use its inline diagnostics, quick fixes, fix-on-save, and cross-file navigation — one bundled binary, no extra setup.](docs/guides/editors/vscode.md)
- [When a Markdown file's payload is prose, put it in the body under H2 sections — not in YAML frontmatter. `mdsmith extract` projects body structure into a JSON tree the same way it projects frontmatter, so the file stays editable as Markdown.](docs/guides/extract-markdown-as-data.md)
- [How to declare file kinds, assign files to them, and read the merged rule config that results.](docs/guides/file-kinds.md)
- [User guides for mdsmith directives, structure enforcement, and migration.](docs/guides/index.md)
- [Every channel that ships the mdsmith binary, the VS Code extension, or the Claude Code plugin — npm, PyPI, Homebrew, asdf, mise, a Flatpak bundle, the GitHub release, the Visual Studio Marketplace plus Open VSX, and the in-repository Claude Code marketplace — and which channel to pick for which workflow.](docs/guides/install.md)
- [Trade-offs and threshold guidance for readability, structure, length, and token budgets.](docs/guides/metrics-tradeoffs.md)
- [Move a project from markdownlint-cli or markdownlint-cli2 to mdsmith — the rule mapping, the config rewrite, and the markdownlint-to-mdsmith rule correspondence.](docs/guides/migrate-from-markdownlint.md)
- [Use `<?catalog?>` with a per-file `summary` front matter field to emit a one-line index of a directory, so AI coding agents read a few thousand tokens of metadata up front and only `Read` the files a task actually touches.](docs/guides/progressive-disclosure.md)
- [Declare a document-structure schema inline on a kind or in a proto.md file, validate headings and front matter, and tighten rule config per section.](docs/guides/schemas.md)
- [CLI commands, flags, exit codes, and output format.](docs/reference/cli.md)
- [List workspace links that point at a file.](docs/reference/cli/backlinks.md)
- [Lint Markdown files for style issues.](docs/reference/cli/check.md)
- [List a file's dependency-graph edges (includes, links, catalogs, builds).](docs/reference/cli/deps.md)
- [Write a portable, directive-free copy of a Markdown file.](docs/reference/cli/export.md)
- [Emit a schema-conformant Markdown file as a JSON/YAML/msgpack data tree.](docs/reference/cli/extract.md)
- [Auto-fix lint issues in Markdown files in place.](docs/reference/cli/fix.md)
- [Show built-in documentation for rules, metrics, and concept pages.](docs/reference/cli/help.md)
- [Generate a default `.mdsmith.yml` config in the current directory.](docs/reference/cli/init.md)
- [Inspect declared file kinds and resolve effective rule config per file.](docs/reference/cli/kinds.md)
- [Selection-style commands that walk the workspace and emit matches.](docs/reference/cli/list.md)
- [Run a Language Server Protocol server on stdio for editor integrations.](docs/reference/cli/lsp.md)
- [Git merge driver that resolves conflicts inside generated sections.](docs/reference/cli/merge-driver.md)
- [List and rank shared Markdown metrics (file length, token estimate, readability, …).](docs/reference/cli/metrics.md)
- [Install / manage a pre-merge-commit hook that runs `mdsmith fix` after a merge.](docs/reference/cli/pre-merge-commit.md)
- [Select Markdown files by a CUE expression on front matter.](docs/reference/cli/query.md)
- [Rename a heading or link-reference label and rewrite every dependent edit.](docs/reference/cli/rename.md)
- [Print the mdsmith build version and exit.](docs/reference/cli/version.md)
- [Each file under `.mdsmith/conventions/` declares one user convention. The basename is the convention name; the file body carries a `flavor:` plus a `rules:` map. Sits alongside inline `conventions.<name>:` in `.mdsmith.yml`.](docs/reference/convention-files.md)
- [Built-in Markdown conventions, the rule presets each one applies, and how user config layers on top via deep-merge.](docs/reference/conventions.md)
- [Glob pattern syntax across mdsmith config, directives, and CLI argument expansion, with the supported exclusion semantics for each surface.](docs/reference/globs.md)
- [Look up exact CLI commands, config glob and schema syntax, the built-in conventions, and the section-schema grammar.](docs/reference/index.md)
- [Each file under `.mdsmith/kinds/` declares one kind. The basename is the kind name; the file body carries the full `KindBody` — schema, rules, `path-pattern:`, `extends:`. Sits alongside inline `kinds.<name>:` in `.mdsmith.yml`.](docs/reference/kind-files.md)
- [Named field-type shortcuts for inline schema frontmatter values — the registered names, the canonical CUE each one resolves to, and example usage.](docs/reference/schema-types.md)
- [Section-schema reference for inline `kinds.<name>.schema:` blocks. Covers the `heading:` discriminator, the `regex:` matcher (a Go RE2 body with `\#(digits)` and `\#(fmvar(...))` helpers), the `repeat: {min, max}` cardinality field, and the matching algorithm. `proto.md` files are parsed into the same shape by the schema package, but MDS020's file-schema check still uses its legacy parser; see the proto.md section below for what is and is not migrated.](docs/reference/section-schema.md)
- [mdsmith collects no telemetry, no usage analytics, no error reports, and no identifiers. The CLI and the LSP server make no outbound network calls at runtime.](docs/reference/telemetry.md)
<?/catalog?>

## Development Workflow

- Any change follows Red/Green TDD: failing test, then pass, then commit
- Keep commits small and focused on one change
- Run `mdsmith check .` before committing; all markdown must pass
- Never modify `.mdsmith.yml` (linter configuration) without explicit user consent — this includes rule settings, overrides, ignore patterns, and file-length limits

## PR Workflow

Use the `/pr-fixup`, `/gh-resolve-threads`, and `/merge-queue`
skills for PR work — they cover rebases, CI monitoring, thread
resolution, and merge enqueuing. After every push, request a
Copilot re-review (the skills do this automatically).

## Plan Maintenance

When implementing work tracked by `plan/`:

- Update the plan file **as part of implementation**, not a follow-up
- Check off tasks and acceptance criteria as they're verified
- Move front-matter `status`: `🔲` → `🔳` on start, `✅` when done
- If implementation deviates, update plan text to match
- Run `mdsmith fix PLAN.md` after editing front matter

## Terminal Demo (`demo.tape`)

`demo.tape` records the demo GIF. Editing notes:

- Backtick-delimited strings for embedded quotes: `` Type `cmd 'status: "✅"'` ``. `\"` inside double-quoted Type strings crashes VHS
- A hidden `set +e` runs at start, so don't append `; true` to commands
- `demo/sample.md` is in the `.mdsmith.yml` ignore list; hidden setup copies it to a temp dir for check/fix
- Keep Sleep durations short (1–2 s) for fast CI renders
- Use only fixable rules in `demo/sample.md` (trailing spaces, long lines, bare URLs) so the fix→check flow works

## Writing Guidelines

When writing descriptions, state what specific data must satisfy
what condition. Name the inputs (front matter fields, glob pattern,
heading level), not just the mechanism. Avoid vague verbs (match,
sync, reflect) without saying what is checked against what.

## Development Reference

<?include
file: docs/development/index.md
strip-frontmatter: "true"
heading-level: "absolute"
?>
Build and test reference for mdsmith contributors.
See also:

<?catalog
source-dir: "docs/development"
glob:
  - "*.md"
  - "*/index.md"
  - "!index.md"
sort: title
row: "- [{title}](docs/development/{filename})"
?>
- [Adding a peer linter](docs/development/add-peer-linter.md)
- [Architecture audit log](docs/development/architecture-audit.md)
- [Architecture principles](docs/development/architecture/index.md)
- [Coverage Gate](docs/development/coverage.md)
- [File Placement](docs/development/file-placement.md)
- [High-Performance Go](docs/development/high-performance-go.md)
- [Merge Queue](docs/development/merge-queue.md)
- [PR Fixup Workflow](docs/development/pr-fixup-workflow.md)
- [Public Markdown Library](docs/development/markdown-library.md)
- [Release Pipeline](docs/development/release.md)
- [Release Tooling Architecture](docs/development/release-tooling.md)
- [Secret Rotations](docs/development/secret-rotations.md)
- [Website configuration](docs/development/website-config.md)
<?/catalog?>

### Build & Test Commands

Requires Go 1.24+.

- `go build ./...` — build all packages
- `go test ./...` — run all tests
- `go test -run TestName ./...` — run a specific test
- `go run ./cmd/mdsmith check .` — lint markdown
- `go run ./cmd/mdsmith fix .` — auto-fix markdown
- `go tool golangci-lint run` — run linter
- `go vet ./...` — run go vet

### Project Layout

Follows the [standard Go project layout][stdlayout]:

- `cmd/mdsmith/` — main entry point.
- `internal/` — private packages.
- `internal/rules/<rule-name>/` — rule code (e.g.
  `paragraphstructure/`).
- `internal/rules/MDS###-<rule-name>/` — rule README
  and good/bad fixtures (e.g.
  `MDS024-paragraph-structure/`).
- `testdata/` — shared markdown fixtures.
- `pkg/goldmark/` — vendored goldmark fork.

[stdlayout]: https://go.dev/doc/modules/layout

### Code Style

- Follow standard Go conventions (gofmt, goimports).
- Use golangci-lint for linting.
- Keep functions small and focused.
- Error messages: lowercase, no trailing punctuation.
- Prefer returning errors over panicking.

### Defensive Code

Add a defensive branch only when you can drive it
red/green. Write the failing test first. Then add
the code that takes the branch.

### Allocation Budget

**A rule's `Check` allocates ≤ 10 times per call on
representative input.** Enforced by
`internal/integration/alloc_budget_test.go`; most
rules allocate 0–6.

- Walk `f.Lines` / `f.AST` directly.
- Prefer `bytes.IndexByte` / `bytes.Contains` over
  `regexp` for fixed searches.
- Compile every `regexp.Regexp` at package scope.
- Pre-size slices with `make([]X, 0, n)`.
- Reuse loop-local buffers via `buf = buf[:0]`.
- Return `nil`, not an empty slice, on no diagnostics.

### Test Fixtures

Rule test fixtures live in
`internal/rules/MDS###-<rule-name>/` (e.g.
`MDS024-paragraph-structure/`). Each rule has `good/`
and `bad/` examples (or `good.md` / `bad.md`).

Good fixtures must pass **all default-enabled rules**
plus the rule under test. Opt-in rules are skipped:
a good MDS001 fixture need not also satisfy MDS043.
When a good fixture uses non-default settings,
override them in `.mdsmith.yml` so `mdsmith check .`
also passes. Bad fixtures are excluded via the
`ignore:` section.

When adding or changing a rule, add both:

1. **Unit tests** in `rule_test.go` (inline markdown,
   fast red/green). Use `require` for preconditions
   and `assert` for checks; `Same`/`NotSame` for
   pointer identity.
2. **Fixture tests** under
   `internal/rules/MDS###-<rule-name>/` with YAML
   frontmatter specifying expected diagnostics.
   Discovered automatically by
   `internal/integration/rules_test.go`.

### Config Merge Semantics

Layered config (defaults → kinds → overrides) is
**deep-merged** rule by rule:

- Maps merge key by key; siblings set in earlier
  layers survive partial overrides.
- Scalar leaves are replaced wholesale.
- List settings replace by default. Opt into
  `append` by implementing
  `rule.ListMerger.SettingMergeMode(key)`. The
  placeholder vocabulary is the canonical example.
- A bool-only layer (`rule-name: false`) toggles
  `enabled` without erasing inherited settings.

New list-typed settings must document the choice
next to their `ApplySettings` handler.

### Generated Sections

Content between `<?directive ... ?>` and
`<?/directive?>` markers is auto-generated. Edit
directive parameters or the source file, then run
`mdsmith fix <file>` — never the body by hand. Run
`mdsmith merge-driver install [files...]` once per
clone so generated-section conflicts resolve
automatically.
<?/include?>
