---
title: Development
summary: Build commands, project layout, code style, test fixtures, coverage gate, and merge conflicts.
---

Build and test reference for mdsmith contributors.

## Build & Test Commands

Requires Go 1.24+.

- `go build ./...` — build all packages
- `go test ./...` — run all tests
- `go test -run TestName ./...` — run a specific test
- `go run ./cmd/mdsmith check .` — lint markdown
- `go run ./cmd/mdsmith fix .` — auto-fix markdown
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

## Code Style

- Follow standard Go conventions (gofmt, goimports)
- Use golangci-lint for linting
- Keep functions small and focused
- Error messages should be lowercase, no trailing
  punctuation
- Prefer returning errors over panicking

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

## Coverage Gate

Codecov blocks PRs that decrease per-file statement
coverage. Fork PRs skip the upload and are not gated.
Three status checks run on same-repo PRs:

- **project** — overall coverage must not drop below
  the base commit.
- **patch** — changed lines must have coverage at
  least equal to the project baseline.
- **changes** — no individual file's coverage may
  decrease vs the base commit.

If any check fails, Codecov posts a comment listing
the affected files with baseline, current, and delta
percentages. Fix regressions by adding tests for the
uncovered code paths before merging.

Configuration lives in `codecov.yml` at the repo
root. The `test` job in `.github/workflows/ci.yml`
uploads the merged coverage profile to Codecov after
each run.

To reproduce CI's merged coverage locally:

```bash
mkdir -p e2e-cover
E2E_COVERDIR=e2e-cover \
  go test -covermode=atomic \
  -coverprofile=unit.cov ./...
head -1 unit.cov > merged.cov
tail -n +2 unit.cov \
  | grep -v 'cmd/mdsmith/' >> merged.cov || true
tail -n +2 e2e-cover/e2e_coverage.txt \
  | grep 'cmd/mdsmith/' >> merged.cov
go tool cover -func=merged.cov
```

Unit tests cannot cover `cmd/mdsmith/` because those
functions run in a subprocess. The merge replaces
those zero-count unit lines with the e2e counts. CI
performs additional validation (mode header match,
file existence); see the `test` job in `ci.yml`.

## Generated Sections

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

## Where to Place Markdown Files

Every Markdown file checked by mdsmith must live in
one of the allowed directories. The
`directory-structure` rule (MDS033) enforces this for
linted files. When creating a new `.md` file, use the
decision list below — take the **first match**.

### Decision list

1. **Well-known root file?**
   (`README.md`, `CLAUDE.md`, `AGENTS.md`, `PLAN.md`)
   → Place in repo root (`.`)

2. **Plan file?** (has front matter with `id`, `title`,
   `status` matching plan schema)
   → Place in `plan/` as `<id>_<slug>.md`

3. **Rule documentation?** (front matter `id` starts
   with `MDS`)
   → Place in `internal/rules/<id>-<name>/README.md`

4. **Metric documentation?** (front matter `id` starts
   with `MET`)
   → Place in `internal/metrics/<id>-<name>/README.md`

5. **Agent skill?** (SKILL.md with skill front matter)
   → Place in `.claude/skills/<name>/SKILL.md`

6. **GitHub integration?** (copilot instructions,
   workflows)
   → Place in `.github/`

7. **Task-oriented: "how do I...?"** (steps, examples,
   practical guidance)
   → Place in `docs/guides/`

8. **Lookup-oriented: "what is the spec?"**
   (exhaustive, complete, for reference)
   → Place in `docs/reference/`

9. **Learning-oriented: "teach me"** (sequential
   tutorial, concrete outcome)
   → Place in `docs/tutorials/`

10. **Context-oriented: "why?"** (rationale,
    comparisons, trade-offs, design decisions)
    → Place in `docs/background/`

11. **Contributor workflow?** (build, test, CI, release
    procedures)
    → Place in `docs/development/`

12. **Security analysis?** (audits, threat models)
    → `docs/security/<YYYY-MM-DD>-<slug>.md`
    Front matter: `date`, `scope`, `method`.
    Schema: `docs/security/proto.md`.

13. **Research?** (spikes, experiments, corpus data,
    not user-facing)
    → Place in `docs/research/`

14. **Demo fixture?** (VHS terminal recording samples)
    → Place in `demo/`

15. **Rule test fixture?** (good/bad/fixed examples)
    → Place in `internal/rules/<id>-<name>/good/`,
    `bad/`, or `fixed/`

If a file does not match any of these, it does not
belong in the repo as a standalone Markdown file.
Consider whether it should be a section in an existing
document instead.

## Documentation Types

mdsmith documentation follows four types. Place each
file in the matching directory:

| Type       | Directory                                 | Purpose                              | Example                             |
|------------|-------------------------------------------|--------------------------------------|-------------------------------------|
| Guide      | `docs/guides/`                            | Task-oriented: how to achieve a goal | "How to enforce document structure" |
| Reference  | `docs/reference/`, `internal/rules/MDS*/` | Lookup-oriented: complete specs      | CLI flags, rule README              |
| Tutorial   | `docs/tutorials/`                         | Learning-oriented: step-by-step      | "Your first schema"                 |
| Background | `docs/background/`                        | Understanding-oriented: context      | Comparison with other linters       |

When writing documentation:

- **Guides** answer "how do I...?" — start with a
  use case, show examples, link to reference for
  full details
- **References** answer "what is...?" — complete,
  accurate, generated where possible (use catalog
  directives)
- **Tutorials** answer "teach me..." — sequential
  steps, minimal prerequisites, concrete outcome
- **Background** answers "why...?" — context,
  trade-offs, comparisons, design rationale
