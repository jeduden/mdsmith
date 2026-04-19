# Copilot Instructions

<?include
file: ../CLAUDE.md
strip-frontmatter: "true"
heading-level: "absolute"
?>
## CLAUDE.md

### Project

mdsmith — a Markdown linter written in Go.

### Docs

- [Plan template; see PLAN.md for status, plans live in plan/](../plan/proto.md)

<?catalog
source-dir: "."
glob:
  - "docs/**/*.md"
  - "!docs/research/**"
  - "!docs/security/**"
sort: path
header: ""
row: "- [{summary}](../{filename})"
?>
- [How generated sections work — markers, directives, and fix behavior.](../docs/background/archetypes/generated-section/README.md)
- [Shared patterns (archetypes) reused across multiple linting rules.](../docs/background/archetypes/README.md)
- [Comparison of mdsmith with other Markdown linters and formatters.](../docs/background/markdown-linters.md)
- [Codecov coverage gate and CI status checks.](../docs/development/coverage.md)
- [Where to place Markdown files and documentation types.](../docs/development/file-placement.md)
- [Build commands, project layout, code style, test fixtures, coverage gate, and merge conflicts.](../docs/development/index.md)
- [Label-driven merge queue workflow using jeduden/merge-queue-action.](../docs/development/merge-queue.md)
- [Rebase, CI monitoring, and review comment resolution.](../docs/development/pr-fixup-workflow.md)
- [How to use schemas, require, and allow-empty-section to validate headings, front matter, and filenames.](../docs/guides/directives/enforcing-structure.md)
- [How to use catalog and include directives to generate and embed content in Markdown files.](../docs/guides/directives/generating-content.md)
- [Key differences between Hugo templates and mdsmith directives for users familiar with Hugo.](../docs/guides/directives/hugo-migration.md)
- [User guides for mdsmith directives, structure enforcement, and migration.](../docs/guides/index.md)
- [Trade-offs and threshold guidance for readability, structure, length, and token budgets.](../docs/guides/metrics-tradeoffs.md)
- [CLI commands, flags, exit codes, and output format.](../docs/reference/cli.md)
<?/catalog?>

### Development Workflow

- Any change follows Red / Green TDD: write a failing
  test (red), make it pass (green), commit
- Keep commits small and focused on one change
- Run `mdsmith check .` before committing to ensure all
  markdown files pass linting
- Never modify `.mdsmith.yml` (linter configuration)
  without explicit user consent — this includes rule
  settings, overrides, ignore patterns, and file-length
  limits

### PR Workflow

Use `gh` for all GitHub PR operations:

```bash
# View PR comments
gh pr view <number> --comments

# List review comments on a PR
gh api repos/"$(gh repo view --json nameWithOwner \
  -q '.nameWithOwner')"/pulls/<number>/comments \
  --paginate

# Resolve a review thread after addressing it
gh api graphql -f query='mutation {
  resolveReviewThread(input: {threadId: "ID"}) {
    thread { id isResolved }
  }
}'

# Push updates after addressing comments
git push origin <branch>
```

These commands are auto-approved in
[`.claude/settings.json`](../.claude/settings.json).

### Plan Maintenance

When implementing work tracked by a plan file in
`plan/`:

- Update the plan file **as part of the
  implementation**, not as a separate follow-up
- Check off each task (`- [x]`) as it is completed
- Check off each acceptance criterion when verified
- When all acceptance criteria are met, change the
  front-matter `status` from `🔲` or `🔳` to `✅`
- When work begins on a not-started plan, change
  `status` from `🔲` to `🔳`
- If the implementation deviates from the plan
  (e.g. a parameter name changes), update the plan
  text to match what was actually built
- Run `mdsmith fix PLAN.md` after changing a plan's
  front matter so the catalog table stays current

### Terminal Demo (`demo.tape`)

The repo includes a VHS tape file (`demo.tape`) that
records a terminal demo GIF. When editing this file:

- VHS uses backtick-delimited strings to embed quotes:
  `` Type `cmd 'status: "✅"'` `` — do NOT use `\"`
  inside double-quoted Type strings (VHS crashes)
- The tape runs `set +e` (hidden) at the start so
  non-zero exits don't abort the recording — no need
  to append `; true` to commands
- `demo/sample.md` is in the `.mdsmith.yml` ignore
  list; the hidden setup copies it to a temp dir
  for check/fix steps
- Keep Sleep durations short (1–2 s) so VHS renders
  quickly in CI
- Only use fixable lint rules in `demo/sample.md`
  (e.g. trailing spaces, long lines, bare URLs) so
  the "fix then clean check" flow works

### Writing Guidelines

When writing descriptions, state the concrete constraint:
what specific data must satisfy what condition. Name the
inputs (front matter fields, glob pattern, heading level)
not just the mechanism. Avoid vague verbs (match, sync,
reflect) without saying what is checked against what.

### Development Reference

<?include
source-dir: "."
file: docs/development/index.md
strip-frontmatter: "true"
heading-level: "absolute"
?>
Build and test reference for mdsmith contributors.
See also:

- [Coverage gate](../docs/development/coverage.md)
- [File placement](../docs/development/file-placement.md)
- [Merge queue](../docs/development/merge-queue.md)
- [PR fixup workflow](../docs/development/pr-fixup-workflow.md)

#### Build & Test Commands

Requires Go 1.24+.

- `go build ./...` — build all packages
- `go test ./...` — run all tests
- `go test -run TestName ./...` — run a specific test
- `go run ./cmd/mdsmith check .` — lint markdown
- `go run ./cmd/mdsmith fix .` — auto-fix markdown
- `go tool golangci-lint run` — run linter
- `go vet ./...` — run go vet

#### Project Layout

Follow the [standard Go project
layout](https://go.dev/doc/modules/layout):

- `cmd/mdsmith/` — main application entry point
- `internal/` — private packages not importable by
  other modules
- `internal/rules/` — rule documentation
  (`internal/rules/<id>-<name>/README.md`)
- `testdata/` — test fixtures (markdown files for
  testing rules)

#### Code Style

- Follow standard Go conventions (gofmt, goimports)
- Use golangci-lint for linting
- Keep functions small and focused
- Error messages should be lowercase, no trailing
  punctuation
- Prefer returning errors over panicking

#### Test Fixtures

Rule test fixtures live in
`internal/rules/<id>-<name>/`. Each rule has `good/` and
`bad/` examples (or `good.md` / `bad.md`).

Good fixtures must pass **all** rules, not just their
own. When a good fixture uses non-default settings
(e.g. setext headings, tilde fences), add a matching
override in `.mdsmith.yml` so that `mdsmith check .`
also passes.

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

#### Generated Sections

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
