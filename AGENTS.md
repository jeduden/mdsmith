# Agent Notes

<!-- Included content comes from CLAUDE.md (which
     itself includes docs/development/index.md).
     Edit those files first, then run
     `mdsmith fix .` to propagate. -->

Instructions for AI coding agents (Codex, Copilot,
Claude).

<?include
file: CLAUDE.md
strip-frontmatter: "true"
?>
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
sort: path
header: ""
row: "- [{summary}]({filename})"
?>
- [How "flavor" (a property of the renderer), "rule" (a single check), "convention" (a project-wide bundle), and "kind" (a per-file role tag) differ in mdsmith, the cases where they overlap, and how the four concepts compose.](docs/background/concepts/flavor-rule-convention-kind.md)
- [How generated sections work — markers, directives, and fix behavior.](docs/background/concepts/generated-section.md)
- [How the placeholder vocabulary lets rules treat template tokens as opaque rather than flagging them as content violations.](docs/background/concepts/placeholder-grammar.md)
- [How mdsmith compares to other Markdown linters.](docs/background/markdown-linters.md)
- [Running log of SOLID and clean-architecture findings on origin/main. The solid-architecture skill (audit mode) appends here; blockers are also filed as plans.](docs/development/architecture-audit.md)
- [Checklist for sweeping origin/main for SOLID and boundary violations. Records findings in the audit log; schedules blockers as new plan files.](docs/development/architecture/audit-checklist.md)
- [External-surface contracts: LSP, CLI, .mdsmith.yml, generated markers, plugin manifest, distribution shims. Public APIs.](docs/development/architecture/cross-system.md)
- [Go-specific SOLID and clean architecture patterns for mdsmith's cmd/ and internal/ packages.](docs/development/architecture/go.md)
- [SOLID and clean-architecture rules for mdsmith's Go core, TypeScript extension, and cross-system surfaces. Canonical home for the solid-architecture skill.](docs/development/architecture/index.md)
- [SOLID and clean architecture patterns for the mdsmith VS Code extension at editors/vscode/.](docs/development/architecture/typescript.md)
- [Codecov coverage gate and CI status checks.](docs/development/coverage.md)
- [Where to place Markdown files and documentation types.](docs/development/file-placement.md)
- [Build commands, project layout, code style, test fixtures, coverage gate, and merge conflicts.](docs/development/index.md)
- [Label-driven merge queue workflow using jeduden/merge-queue-action.](docs/development/merge-queue.md)
- [Rebase, CI monitoring, and review comment resolution.](docs/development/pr-fixup-workflow.md)
- [Per-platform mdsmith binaries plus the .vsix, the checksum file, and a Sigstore signature, attached to a tag-named release.](docs/development/release-channels/github-releases.md)
- [Root `@mdsmith/cli` plus one platform-specific subpackage per supported host, all published via OIDC Trusted Publishing.](docs/development/release-channels/npm.md)
- [The same `.vsix` republished to Open VSX so VSCodium, Cursor, Theia, and Gitpod can install it.](docs/development/release-channels/open-vsx.md)
- [One platform-tagged wheel per supported host, published via OIDC Trusted Publishing.](docs/development/release-channels/pypi.md)
- [The mdsmith VS Code extension `.vsix`, published via a long-lived Marketplace publisher PAT.](docs/development/release-channels/visual-studio-marketplace.md)
- [Every GitHub Actions workflow that needs runtime logic invokes the `mdsmith-release` Go CLI rather than carrying inline shell or per-language scripts. This page captures the rule and the subcommands it applies to.](docs/development/release-tooling.md)
- [How tag pushes publish mdsmith to npm, PyPI, the Visual Studio Marketplace, Open VSX, and GitHub Releases — the workflow structure, the OIDC trusted publishers it relies on, the `release` environment that gates every publishing job, and the supply-chain hardening features baked into the pipeline.](docs/development/release.md)
- [Rotation cadence and procedure for the long-lived publisher tokens consumed by the release and merge-queue workflows. Each tracked secret has its own file under `secret-rotations/`; the catalog below enumerates them. The scheduled reminder workflow consumes the same files and opens a GitHub issue when any secret is within 30 days of expiry.](docs/development/secret-rotations.md)
- [GitHub fine-grained PAT for the merge-queue action. Plain repo secret — not gated by an environment.](docs/development/secret-rotations/merge-queue-token.md)
- [Open VSX publisher token. Drives the `ovsx publish` step.](docs/development/secret-rotations/ovsx-pat.md)
- [Visual Studio Marketplace publisher PAT issued by Azure DevOps. Drives the `vsce publish` step.](docs/development/secret-rotations/vsce-pat.md)
- [How to use the build directive to declare artifact outputs, keep generated bodies in sync, and configure user-declared recipes.](docs/guides/directives/build.md)
- [How to use schemas, require, and allow-empty-section to validate headings, front matter, and filenames.](docs/guides/directives/enforcing-structure.md)
- [How to use catalog and include directives to generate and embed content in Markdown files.](docs/guides/directives/generating-content.md)
- [Key differences between Hugo templates and mdsmith directives for users familiar with Hugo.](docs/guides/directives/hugo-migration.md)
- [Install the mdsmith VS Code extension, configure how it spawns `mdsmith lsp`, and read diagnostics inline as you edit Markdown files.](docs/guides/editors/vscode.md)
- [How to declare file kinds, assign files to them, and read the merged rule config that results.](docs/guides/file-kinds.md)
- [User guides for mdsmith directives, structure enforcement, and migration.](docs/guides/index.md)
- [Every channel that ships the mdsmith binary, the VS Code extension, or the Claude Code plugin — npm, PyPI, asdf, mise, the GitHub release, the Visual Studio Marketplace plus Open VSX, and the in-repository Claude Code marketplace — and which channel to pick for which workflow.](docs/guides/install.md)
- [Trade-offs and threshold guidance for readability, structure, length, and token budgets.](docs/guides/metrics-tradeoffs.md)
- [Declare a document-structure schema inline on a kind or in a proto.md file, validate headings and front matter, and tighten rule config per section.](docs/guides/schemas.md)
- [CLI commands, flags, exit codes, and output format.](docs/reference/cli.md)
- [List workspace links that point at a file.](docs/reference/cli/backlinks.md)
- [Lint Markdown files for style issues.](docs/reference/cli/check.md)
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
- [Print the mdsmith build version and exit.](docs/reference/cli/version.md)
- [Built-in Markdown conventions, the rule presets each one applies, and how user config layers on top via deep-merge.](docs/reference/conventions.md)
- [Glob pattern syntax across mdsmith config, directives, and CLI argument expansion, with the supported exclusion semantics for each surface.](docs/reference/globs.md)
<?/catalog?>

## Development Workflow

- Any change follows Red / Green TDD: write a failing
  test (red), make it pass (green), commit
- Keep commits small and focused on one change
- Run `mdsmith check .` before committing to ensure all
  markdown files pass linting
- Never modify `.mdsmith.yml` (linter configuration)
  without explicit user consent — this includes rule
  settings, overrides, ignore patterns, and file-length
  limits

## PR Workflow

Use the `/pr-fixup`, `/gh-resolve-threads`, and
`/merge-queue` skills for PR work — they cover
rebases, CI monitoring, thread resolution, and merge
enqueuing. After every push, request a Copilot
re-review (the skills do this automatically).

## Plan Maintenance

When implementing work tracked by `plan/`:

- Update the plan file **as part of the
  implementation**, not as a follow-up
- Check off each task and acceptance criterion as it
  is completed or verified
- Move front-matter `status` from `🔲` to `🔳` on
  start, then to `✅` when all criteria pass
- If implementation deviates, update plan text to
  match what was built
- Run `mdsmith fix PLAN.md` after editing plan front
  matter so the catalog table stays current

## Terminal Demo (`demo.tape`)

`demo.tape` is the VHS tape that records the demo
GIF. When editing it:

- Use backtick-delimited strings for embedded quotes:
  `` Type `cmd 'status: "✅"'` ``. Do NOT use `\"`
  inside double-quoted Type strings — VHS crashes
- A hidden `set +e` runs at start, so don't append
  `; true` to commands
- `demo/sample.md` is in the `.mdsmith.yml` ignore
  list; hidden setup copies it to a temp dir for
  check/fix steps
- Keep Sleep durations short (1–2 s) for fast CI
  renders
- Use only fixable rules in `demo/sample.md` (trailing
  spaces, long lines, bare URLs) so the fix→check
  flow works

## Writing Guidelines

When writing descriptions, state the concrete constraint:
what specific data must satisfy what condition. Name the
inputs (front matter fields, glob pattern, heading level)
not just the mechanism. Avoid vague verbs (match, sync,
reflect) without saying what is checked against what.

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
- [Architecture audit log](docs/development/architecture-audit.md)
- [Architecture principles](docs/development/architecture/index.md)
- [Coverage Gate](docs/development/coverage.md)
- [File Placement](docs/development/file-placement.md)
- [Merge Queue](docs/development/merge-queue.md)
- [PR Fixup Workflow](docs/development/pr-fixup-workflow.md)
- [Release Pipeline](docs/development/release.md)
- [Release Tooling Architecture](docs/development/release-tooling.md)
- [Secret Rotations](docs/development/secret-rotations.md)
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

Follow the [standard Go project
layout](https://go.dev/doc/modules/layout):

- `cmd/mdsmith/` — main application entry point
- `internal/` — private packages not importable by
  other modules
- `internal/rules/` — rule documentation
  (`internal/rules/<id>-<name>/README.md`)
- `testdata/` — test fixtures (markdown files for
  testing rules)

### Code Style

- Follow standard Go conventions (gofmt, goimports)
- Use golangci-lint for linting
- Keep functions small and focused
- Error messages should be lowercase, no trailing
  punctuation
- Prefer returning errors over panicking

### Test Fixtures

Rule test fixtures live in
`internal/rules/<id>-<name>/`. Each rule has `good/` and
`bad/` examples (or `good.md` / `bad.md`).

Good fixtures must pass **all default-enabled rules**
plus the rule under test. Default-disabled (opt-in)
rules are skipped for other rules' fixtures: a
good fixture for MDS001 is not required to also
satisfy MDS043, since MDS043 is opt-in and would not
fire in a default project. When a good fixture uses
non-default settings (e.g. setext headings, tilde
fences), add a matching override in `.mdsmith.yml`
so that `mdsmith check .` also passes.

Bad fixtures are excluded via the `ignore:` section in
`.mdsmith.yml`.

When adding or changing a rule feature, add both:

1. **Unit tests** in `rule_test.go` (inline markdown,
   fast red/green cycle). Use `require` from testify
   for preconditions that abort the test and `assert`
   for checks that continue. Use `Same`/`NotSame` for
   pointer identity.
2. **Fixture tests** under `internal/rules/<id>-<name>/`
   (`good/` and `bad/` markdown files with YAML
   frontmatter specifying expected diagnostics). These
   are discovered automatically by the integration test
   runner in `internal/integration/rules_test.go`.

### Config Merge Semantics

Layered config (defaults → kinds → overrides) is
**deep-merged** rule by rule:

- Maps merge key by key; sibling settings set in earlier
  layers survive when a later layer touches only one key.
- Scalars at a leaf are replaced wholesale by the later
  layer.
- List-typed settings replace by default. A rule opts a
  particular list setting into `append` by implementing
  `rule.ListMerger.SettingMergeMode(key)` and returning
  `rule.MergeAppend`. The placeholder vocabulary
  (`placeholders:`) is the canonical append-mode setting.
- A bool-only layer (`rule-name: false`) toggles
  `enabled` without erasing inherited settings.

When adding a new list-typed setting, decide whether
it should `append` or `replace`. Document the choice
next to its `ApplySettings` handler.

### Generated Sections

Content between `<?directive ... ?>` and
`<?/directive?>` markers is auto-generated. Do not
edit the body by hand — edit the directive parameters
or the source file it references, then run
`mdsmith fix <file>` to regenerate.

After merge conflicts in generated sections, run
`mdsmith fix <file>` to regenerate. The
`merge-driver` command automates this:

```bash
mdsmith merge-driver install [files...]
```

Run `mdsmith merge-driver install` once per clone.
<?/include?>
<?/include?>
