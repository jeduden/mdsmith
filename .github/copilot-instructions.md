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

- [Build commands, project layout, code style, test fixtures, and merge conflicts](../docs/development/index.md)
- [Plan template; see PLAN.md for status, plans live in plan/](../plan/proto.md)

<?catalog
glob: "docs/**/*.md"
sort: path
header: ""
row: "- [{summary}](../{filename})"
?>
- [Comparison of mdsmith with other Markdown linters and formatters.](../docs/background/markdown-linters.md)
- [How generated sections work — markers, directives, and fix behavior.](../docs/design/archetypes/generated-section/README.md)
- [Shared patterns (archetypes) reused across multiple linting rules.](../docs/design/archetypes/README.md)
- [CLI commands, flags, exit codes, and output format.](../docs/design/cli.md)
- [Build commands, project layout, code style, test fixtures, and merge conflicts.](../docs/development/index.md)
- [PR fixup workflow for rebase, CI monitoring, review comment resolution, and gh CLI setup.](../docs/development/pr-fixup-workflow.md)
- [How to use schemas, require, and allow-empty-section to validate headings, front matter, and filenames.](../docs/guides/directives/enforcing-structure.md)
- [How to use catalog and include directives to generate and embed content in Markdown files.](../docs/guides/directives/generating-content.md)
- [Key differences between Hugo templates and mdsmith directives for users familiar with Hugo.](../docs/guides/directives/hugo-migration.md)
- [User guides for mdsmith directives, structure enforcement, and migration.](../docs/guides/index.md)
- [Trade-offs and threshold guidance for readability, structure, length, and token budgets.](../docs/guides/metrics-tradeoffs.md)
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
- End every shell command with `; true` so non-zero
  exits don't abort the recording
- `demo/sample.md` is in the `.mdsmith.yml` ignore
  list; copy it to `/tmp` for check/fix steps
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
<?/include?>
