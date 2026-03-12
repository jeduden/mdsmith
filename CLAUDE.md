# CLAUDE.md

## Project

mdsmith — a Markdown linter written in Go.

## Docs

- [Build commands, project layout, code style, test fixtures, and merge conflicts](docs/development/index.md)
- [Plan template; see PLAN.md for status, plans live in plan/](plan/proto.md)

<?catalog
glob: "docs/**/*.md"
sort: path
header: ""
row: "- [{{.summary}}]({{.filename}})"
?>
- [Comparison of mdsmith with other Markdown linters and formatters.](docs/background/markdown-linters.md)
- [How generated sections work — markers, directives, and fix behavior.](docs/design/archetypes/generated-section/README.md)
- [Shared patterns (archetypes) reused across multiple linting rules.](docs/design/archetypes/README.md)
- [CLI commands, flags, exit codes, and output format.](docs/design/cli.md)
- [Build commands, project layout, code style, test fixtures, and merge conflicts.](docs/development/index.md)
- [PR fixup workflow for rebase, CI monitoring, review comment resolution, and gh CLI setup.](docs/development/pr-fixup-workflow.md)
- [Trade-offs and threshold guidance for readability, structure, length, and token budgets.](docs/guides/metrics-tradeoffs.md)
<?/catalog?>

## Development Workflow

- Any change follows Red / Green TDD: write a failing
  test (red), make it pass (green), commit
- Keep commits small and focused on one change
- Run `mdsmith check .` before committing to ensure all
  markdown files pass linting

## PR Workflow

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
[`.claude/settings.json`](.claude/settings.json).

## Writing Guidelines

When writing descriptions, state the concrete constraint:
what specific data must satisfy what condition. Name the
inputs (front matter fields, glob pattern, heading level)
not just the mechanism. Avoid vague verbs (match, sync,
reflect) without saying what is checked against what.
