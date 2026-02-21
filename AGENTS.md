# Agent Notes

Instructions for AI coding agents (Codex, Copilot, Claude).
See [CLAUDE.md](CLAUDE.md) for full project conventions.

## Quick Reference

- **Build:** `go build ./...`
- **Test:** `go test ./...`
- **Lint (Go):** `go tool golangci-lint run`
- **Lint (Markdown):** `mdsmith check .`
- **Fix Markdown:** `mdsmith fix .`

## PR Workflow

Use `gh` for all GitHub operations. Agents should run
these commands without prompting for confirmation:

```bash
# Pull PR comments
gh pr view <number> --comments

# List review comments
gh api repos/{owner}/{repo}/pulls/<number>/comments

# Resolve a review thread
gh api graphql -f query='...'

# Push updates
git push origin <branch>
```

## Merge Conflicts in PLAN.md and README.md

These files contain auto-generated sections (catalog,
include) delimited by `<!-- name -->` / `<!-- /name -->`
markers. When two branches add items, sections conflict.

Resolution: run `mdsmith fix <file>` after merging.
The fix command regenerates sections from front matter
or source files. Do not manually resolve section
conflicts — `mdsmith fix` overwrites the entire section.

A built-in merge driver automates this. Register it:

```bash
mdsmith merge-driver install [files...]
```

## Development Conventions

- Test-driven: write a failing test, make it pass, commit.
- Keep commits small and focused.
- Run `mdsmith check .` before every commit.
- Error messages: lowercase, no trailing punctuation.
- Plans live in [`plan/`](plan/) with status in
  [PLAN.md](PLAN.md).
