---
id: 80
title: "Terminal recording in README"
status: "✅"
summary: "Auto-generate a terminal demo GIF via GitHub Actions and embed it in README.md"
---
# Terminal recording in README

## Context

The README describes mdsmith's features in text, but a
short terminal recording showing the tool in action is
more compelling. The recording must stay current — if
commands or output change, the GIF should update
automatically.

## Goal

Embed an auto-generated terminal demo GIF in the root
README, placed right after the intro paragraph. A
GitHub Actions workflow regenerates the recording on
every push to `main`, and PR CI verifies the recording
pipeline works without pushing artifacts.

## Design

### Recording tool

Use [VHS](https://github.com/charmbracelet/vhs) from
Charm. VHS reads a declarative `.tape` file, drives a
headless terminal, and renders to GIF. It runs in CI
without a display server. Add VHS as a Go tool
dependency in `go.mod` so it is invoked via
`go tool vhs` — no separate install step needed.

### Demo script (`demo.tape`)

A VHS tape file at the repo root that cycles through
key mdsmith features:

1. `./mdsmith init` in a temporary directory — shows
   config generation without conflicting with the
   repo's existing `.mdsmith.yml`
2. `./mdsmith check` on a sample file with lint
   errors — shows diagnostic output with source context
3. `./mdsmith fix` on the same file — shows auto-fix
4. `./mdsmith check` again — clean pass, exit 0
5. `./mdsmith help rule line-length` — shows built-in
   rule docs
6. `./mdsmith help rule catalog` — shows catalog rule
7. `./mdsmith help rule directory-structure` — shows
   directory-structure rule
8. `./mdsmith help rule required-structure` — shows
   required-structure rule
9. `./mdsmith query 'status: "✅"' plan/` — shows
   front-matter filtering
10. `./mdsmith metrics rank --by bytes --top 5 .` —
    shows metrics

Each step has a short pause so viewers can read the
output. The tape targets an 80x24 terminal at a
comfortable typing speed.

### Sample fixture

A small Markdown file `demo/sample.md` with intentional
lint issues (long line, trailing spaces, missing code
fence language). Kept out of normal lint runs via an
`ignore` entry in `.mdsmith.yml` so `./mdsmith check .`
in CI does not flag it.

### README placement

The GIF is embedded immediately after the first
paragraph (the one-liner description), before the
"Why mdsmith" section:

```markdown
# 🔨 mdsmith

A fast, auto-fixing Markdown linter ...

![mdsmith demo](assets/demo.gif)

## ✨ Why mdsmith
```

The `assets/` directory holds the generated GIF. It is
committed to the repo so the image renders on GitHub
without external hosting.

### Workflows

**Generate workflow** (`.github/workflows/demo.yml`):
runs on push to `main`. Steps:

1. Checkout repo
2. Build mdsmith (`go build -o mdsmith ./cmd/mdsmith`)
3. Run `go tool vhs demo.tape` (VHS added as a tool
   dependency in `go.mod`)
4. Configure git `user.name` / `user.email` for the
   CI bot. If `assets/demo.gif` changed, commit with a
   `[skip ci]` marker and push it back to `main`. Add
   a loop guard (e.g. skip when `github.actor` is
   `github-actions[bot]`) to avoid retriggering the
   workflow. Request `permissions: contents: write` so
   the `GITHUB_TOKEN` can push.

This keeps the GIF in sync with the latest CLI output.

**PR verification** (add a job to `.github/workflows/ci.yml`):
runs on pull requests. Steps:

1. Checkout repo
2. Build mdsmith
3. Run `go tool vhs demo.tape`
4. Assert `assets/demo.gif` was produced and is a valid
   GIF (check file header bytes `GIF89a` or `GIF87a`)
5. Assert file size is within a reasonable range
   (> 10 KB, < 5 MB) to catch broken recordings
6. Analyze the GIF content: extract frames, verify
   expected command output appears (e.g. grep rendered
   text for key strings like `MDS001`, `./mdsmith check`,
   `0 issues found`). Use a frame-to-text tool or compare
   against a set of reference screenshots to catch
   regressions where the GIF renders but shows wrong
   or empty output

The PR job does **not** commit — it only verifies the
pipeline succeeds and the output is sane.

## Tasks

1. Create `demo/sample.md` with intentional lint issues
   for the demo
2. Write `demo.tape` VHS script that cycles through
   init, check, fix, help-rule, query, and metrics
   commands
3. Create `assets/` directory with a `.gitkeep`
4. Add the demo GIF embed to `README.md` after the
   intro paragraph
5. Create `.github/workflows/demo.yml` that builds
   mdsmith, runs VHS, and commits the updated GIF on
   pushes to `main`
6. Add a `demo` job to `.github/workflows/ci.yml` that
   runs VHS and validates the output GIF on PRs
7. Add `demo/` to the ignore list in `.mdsmith.yml`
   (requires explicit user consent per CLAUDE.md) so
   the intentionally broken sample file does not fail
   `./mdsmith check .`
8. Test the full pipeline locally: run
   `go tool vhs demo.tape`, verify the GIF renders
   correctly

## Acceptance Criteria

- [x] `demo.tape` exists and defines a multi-step demo
  covering init, check, fix, help-rule (line-length,
  catalog, directory-structure, required-structure),
  query, and metrics commands
- [x] `demo/sample.md` contains intentional lint errors
  that produce visible diagnostics
- [x] `README.md` embeds `assets/demo.gif` between the
  intro paragraph and the "Why mdsmith" section
- [x] `.github/workflows/demo.yml` regenerates the GIF
  on push to `main` and commits it if changed
- [x] CI job in `.github/workflows/ci.yml` runs VHS on
  PRs and asserts the GIF is valid (file exists, has a
  correct GIF header, and falls within a reasonable file
  size range)
- [x] `demo/` is excluded from mdsmith linting so the
  sample file does not cause CI failures
- [x] All tests pass: `go test ./...`
- [x] `go tool golangci-lint run` reports no issues
