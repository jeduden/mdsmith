---
id: 80
title: "Terminal recording in README"
status: "🔲"
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
without a display server.

### Demo script (`demo.tape`)

A VHS tape file at the repo root that cycles through
key mdsmith features:

1. `mdsmith check` on a sample file with lint errors —
   shows diagnostic output with source context
2. `mdsmith fix` on the same file — shows auto-fix
3. `mdsmith check` again — clean pass, exit 0
4. `mdsmith help rule line-length` — shows built-in
   rule docs
5. `mdsmith metrics rank --by bytes --top 5 .` — shows
   metrics

Each step has a short pause so viewers can read the
output. The tape targets a 80x24 terminal at a
comfortable typing speed.

### Sample fixture

A small Markdown file `demo/sample.md` with intentional
lint issues (long line, trailing spaces, missing code
fence language). Kept outside the linted tree via an
`ignore` entry or directory placement so `mdsmith check
.` in CI does not flag it.

### README placement

The GIF is embedded immediately after the first
paragraph (the one-liner description), before the
"Why mdsmith" section:

```markdown
# mdsmith

A fast, auto-fixing Markdown linter ...

![mdsmith demo](assets/demo.gif)

## Why mdsmith
```

The `assets/` directory holds the generated GIF. It is
committed to the repo so the image renders on GitHub
without external hosting.

### Workflows

**Generate workflow** (`.github/workflows/demo.yml`):
runs on push to `main`. Steps:

1. Checkout repo
2. Build mdsmith (`go build -o mdsmith ./cmd/mdsmith`)
3. Install VHS
4. Run `vhs demo.tape`
5. If `assets/demo.gif` changed, commit and push it
   back to `main`

This keeps the GIF in sync with the latest CLI output.

**PR verification** (add a job to `.github/workflows/ci.yml`):
runs on pull requests. Steps:

1. Checkout repo
2. Build mdsmith
3. Install VHS
4. Run `vhs demo.tape`
5. Assert `assets/demo.gif` was produced and is a valid
   GIF (check file header bytes `GIF89a` or `GIF87a`)
6. Assert file size is within a reasonable range
   (> 10 KB, < 5 MB) to catch broken recordings

The PR job does **not** commit — it only verifies the
pipeline succeeds and the output is sane.

## Tasks

1. Create `demo/sample.md` with intentional lint issues
   for the demo
2. Write `demo.tape` VHS script that cycles through
   check, fix, help, and metrics commands
3. Create `assets/` directory with a `.gitkeep`
4. Add the demo GIF embed to `README.md` after the
   intro paragraph
5. Create `.github/workflows/demo.yml` that builds
   mdsmith, runs VHS, and commits the updated GIF on
   pushes to `main`
6. Add a `demo` job to `.github/workflows/ci.yml` that
   runs VHS and validates the output GIF on PRs
7. Add `demo/` to the ignore list in `.mdsmith.yml` so
   the intentionally broken sample file does not fail
   `mdsmith check .`
8. Test the full pipeline locally: run `vhs demo.tape`,
   verify the GIF renders correctly

## Acceptance Criteria

- [ ] `demo.tape` exists and defines a multi-step demo
  covering check, fix, help-rule, and metrics commands
- [ ] `demo/sample.md` contains intentional lint errors
  that produce visible diagnostics
- [ ] `README.md` embeds `assets/demo.gif` between the
  intro paragraph and the "Why mdsmith" section
- [ ] `.github/workflows/demo.yml` regenerates the GIF
  on push to `main` and commits it if changed
- [ ] CI job in `.github/workflows/ci.yml` runs VHS on
  PRs and asserts the GIF is valid (correct header,
  reasonable file size)
- [ ] `demo/` is excluded from mdsmith linting so the
  sample file does not cause CI failures
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
