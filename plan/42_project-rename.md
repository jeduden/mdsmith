---
id: 42
title: Find a Better Project Name
status: 🔲
---
# Find a Better Project Name

## Goal

Look at other name options for the project and rename it
if a better one is found. The tool should have a catchy,
unique name before it ships to the public.

## Constraints

- Short (1--2 syllables preferred, 3 max)
- Easy to type as a CLI command
- Available as a Go module path on GitHub
- Not already a well-known tool or library name
- Conveys linting, checking, or tidying of markdown

## Tasks

1. Brainstorm candidate names. Directions to explore:

  - Compound words: lint + mark, prose + check, md + tidy
  - Short invented words: prosa, mklint, doclint, markcheck
  - Metaphors: plumb (plumb line), hone, burnish, proof

2. Check each candidate for availability:

  - GitHub org/repo availability
  - Go package name conflicts (`pkg.go.dev` search)
  - npm/PyPI/Homebrew conflicts (avoid confusion)
  - Domain availability (nice-to-have, not required)

3. Pick the winner and update:

  - Go module path in `go.mod`
  - All import paths in `internal/`, `cmd/`
  - Binary name in `cmd/<name>/`
  - Config file name (`.tidymark.yml` to `.<name>.yml`)
  - All references in `README.md`, `CLAUDE.md`, docs
  - Rule prefix: decide whether to keep `TM###` or change
  - GitHub repo name (manual step)

4. Add a `Makefile` or script to verify no stale references
   remain (`grep -r tidymark` returns only changelog/history
   entries).

5. Update CI/CD, release scripts, and lefthook config if
   present.

## Acceptance Criteria

- [ ] Candidate names evaluated for availability
- [ ] Name chosen and documented in a decision record
- [ ] All source files use the new module path
- [ ] Binary builds under the new name
- [ ] Config file uses the new name
- [ ] Docs updated with no stale references
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
