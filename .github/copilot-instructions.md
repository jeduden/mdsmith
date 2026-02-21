# Copilot Instructions

Instructions for GitHub Copilot. See
[CLAUDE.md](../CLAUDE.md) and [AGENTS.md](../AGENTS.md)
for full project conventions.

## Project

mdsmith is a Markdown linter written in Go (1.24+).

## Commands

- `go build ./...` — build
- `go test ./...` — test
- `go tool golangci-lint run` — Go lint
- `mdsmith check .` — Markdown lint
- `mdsmith fix .` — auto-fix Markdown

## PR Workflow

Use `gh` for GitHub operations without prompting:

- `gh pr view <number> --comments` — read comments
- `gh api repos/{owner}/{repo}/pulls/<n>/comments`
- `git push origin <branch>` — push changes

## Merge Conflicts

PLAN.md and README.md have auto-generated sections (catalog,
include). Run `mdsmith merge-driver install [files...]` to
register the git merge driver. It strips conflict markers
inside regenerable sections and runs `mdsmith fix` to
regenerate. Do not manually resolve section conflicts.

## Style

- Follow Go conventions (gofmt, goimports).
- Test-driven development.
- Run `mdsmith check .` before committing.
- Error messages: lowercase, no trailing punctuation.
