# CLAUDE.md

## Project

mdsmith — a Markdown linter written in Go.

<?include
file: DEVELOPMENT.md
strip-frontmatter: "true"
?>
Build and test reference for mdsmith contributors.

## Build & Test Commands

Requires Go 1.24+.

- `go build ./...` — build all packages
- `go test ./...` — run all tests
- `go test -run TestName ./...` — run a specific test
- `go tool golangci-lint run` — run linter
- `go vet ./...` — run go vet

## Project Layout

Follow the [standard Go project
layout](https://go.dev/doc/modules/layout):

- `cmd/mdsmith/` — main application entry point
- `internal/` — private packages not importable by
  other modules
- `internal/rules/` — rule documentation
  (`internal/rules/<id>-<name>/README.md`)
- `testdata/` — test fixtures (markdown files for
  testing rules)

## Development Workflow

- Any change follows Red / Green TDD: write a failing
  test (red), make it pass (green), commit
- Keep commits small and focused on one change
- Run `mdsmith check .` before committing to ensure all
  markdown files pass linting

## Code Style

- Follow standard Go conventions (gofmt, goimports)
- Use golangci-lint for linting
- Keep functions small and focused
- Error messages should be lowercase, no trailing
  punctuation
- Prefer returning errors over panicking

## PR Workflow

Use `gh` for all GitHub PR operations:

```bash
# View PR comments
gh pr view <number> --comments

# List review comments on a PR
gh api "$(gh repo view --json nameWithOwner \
  -q '.nameWithOwner')/pulls/<number>/comments" \
  --paginate

# Push updates after addressing comments
git push origin <branch>
```

These commands are auto-approved in
`.claude/settings.json`.
<?/include?>

## Plans

Task plans live in [`plan/`](plan/). See
[`PLAN.md`](PLAN.md) for the current status of all
plans. Use [`plan/proto.md`](plan/proto.md) as a
template when creating new plans.

Each plan has acceptance criteria with behavioral tests.
Work test-driven: write a failing test (red), make it
pass (green), commit.

Plan files must pass `mdsmith check plan/` with zero
diagnostics.

Use Markdown links when referring to real repo paths in
docs and plans. Bare backticked paths are allowed in
commands, code blocks, and placeholders. This lets
link-integrity checks validate real targets.

## Test Fixtures

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

## Merge Conflicts in PLAN.md and README.md

[`PLAN.md`](PLAN.md) and [`README.md`](README.md)
contain auto-generated sections (catalog, include)
between `<?name?>` and `<?/name?>` markers. When two
branches add items, these sections conflict on merge.

**Resolution:** run `mdsmith fix <file>` after merging.
The fix command regenerates the content from front matter
or source files. Do not manually resolve section
conflicts — `mdsmith fix` overwrites the entire section
between the markers.

The built-in `merge-driver` command automates this. It
strips conflict markers inside regenerable sections, runs
`mdsmith fix`, and fails if unresolved markers remain.
Register it once per clone:

```bash
mdsmith merge-driver install [files...]
```

This adds `[merge "mdsmith"]` to `.git/config` and
updates [`.gitattributes`](.gitattributes) for the listed
files (default: PLAN.md, README.md).

## Cross-Platform Agent Config

[`CLAUDE.md`](CLAUDE.md) is the primary source of
project conventions. mdsmith keeps these agent config
files in sync via include directives:

- [`AGENTS.md`](AGENTS.md)
- [`.github/copilot-instructions.md`](.github/copilot-instructions.md)

Edit CLAUDE.md or the shared source files; run
`mdsmith fix .` to propagate changes.

When writing descriptions, state the concrete constraint:
what specific data must satisfy what condition. Name the
inputs (front matter fields, glob pattern, heading level)
not just the mechanism. Avoid vague verbs (match, sync,
reflect) without saying what is checked against what.
