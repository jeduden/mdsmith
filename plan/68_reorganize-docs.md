---
id: 68
title: Reorganize Documentation
status: "🔲"
---
# Reorganize Documentation

## Goal

Move reference content out of CLAUDE.md into dedicated
files. Make AGENTS.md use the include directive to keep
agent configs in sync.

## Background

CLAUDE.md mixes development workflow, CLI design spec,
background material, and agent-config housekeeping.
Moving each concern to its natural home makes CLAUDE.md
shorter. The include directive (`<?include` ...
`<?/include?>`) lets AGENTS.md and
`.github/copilot-instructions.md` pull content from
shared files at fix-time. A single edit then spreads
to every file.

## Tasks

### Create target files

1. Create `DEVELOPMENT.md` with sections moved from
   CLAUDE.md (see below).
2. Create `docs/design/` directory. Move CLI Design
   section to `docs/design/cli.md`.
3. Move `background/` to `docs/background/`.
4. Move `guides/` to `docs/guides/`.
5. Move `archetypes/` to `docs/design/archetypes/`.

### Populate DEVELOPMENT.md

6. Move "Build & Test Commands" from CLAUDE.md.
7. Move "Project Layout" from CLAUDE.md.
8. Move "Development Workflow" from CLAUDE.md.
   Reword: "New features are test-driven" becomes
   "Any change follows Red / Green TDD: write a
   failing test (red), make it pass (green), commit".
9. Move "Code Style" from CLAUDE.md.
10. Move "PR Workflow" from CLAUDE.md. Update the
    `gh api` example to use
    `gh repo view --json nameWithOwner` for
    dynamic owner/repo lookup and `--paginate`.

### Update CLAUDE.md

11. Add an include directive in CLAUDE.md that pulls
    DEVELOPMENT.md so agents still see the development
    info inline (`<?include` / `file: DEVELOPMENT.md` /
    `?>` ... `<?/include?>`).
12. Update "Merge Conflicts in PLAN.md and README.md"
    to reference processing-instruction markers
    (`<?name?>` / `<?/name?>`) instead of old
    HTML-comment markers (`<!-- name -->`).
13. Rewrite "Cross-Platform Agent Config" to state
    that CLAUDE.md is the primary doc and mdsmith
    keeps the others in sync via include directives.
14. Remove the "Config & Rules" section entirely.

### Update AGENTS.md

15. Replace the body of AGENTS.md with an include
    directive (`<?include` / `file: ...` / `?>` ...
    `<?/include?>`) that pulls relevant sections so
    it stays in sync automatically.

### Update copilot-instructions.md

16. Replace the body of
    `.github/copilot-instructions.md` with an include
    directive (same `<?include` / `<?/include?>`
    approach).

### Update README.md

17. Update README.md to reference the new docs layout:
    include DEVELOPMENT.md, and link to `docs/design/`,
    `docs/guides/`, `docs/background/`, and `plan/`.

### Fixups

18. Update internal links in moved files so they
    resolve from their new locations.
19. Update `.mdsmith.yml` overrides and ignore entries
    that reference old paths (`background/`, `guides/`,
    `archetypes/`).
20. Run `mdsmith fix .` to regenerate all include and
    catalog sections.
21. Run `mdsmith check .` and fix any diagnostics.

## Acceptance Criteria

- [ ] CLAUDE.md includes DEVELOPMENT.md via an
      `<?include` / `<?/include?>` directive
- [ ] CLAUDE.md no longer contains Build & Test,
      Project Layout, Development Workflow, Code Style,
      CLI Design, PR Workflow, or Config & Rules
      as hand-maintained sections
- [ ] CLAUDE.md "Merge Conflicts" section references
      `<?...?>` processing-instruction syntax
- [ ] CLAUDE.md "Cross-Platform Agent Config" says
      CLAUDE.md is the primary source and mdsmith
      keeps others in sync
- [ ] AGENTS.md uses an `<?include` / `<?/include?>`
      directive with a `file:` parameter
- [ ] `.github/copilot-instructions.md` uses an
      `<?include` / `<?/include?>` directive
- [ ] `background/` moved to `docs/background/`
- [ ] `guides/` moved to `docs/guides/`
- [ ] `archetypes/` moved to `docs/design/archetypes/`
- [ ] CLI Design lives in `docs/design/cli.md`
- [ ] DEVELOPMENT.md exists with the moved sections
- [ ] README.md includes DEVELOPMENT.md via an
      `<?include` / `<?/include?>` directive
- [ ] README.md links to `docs/design/`,
      `docs/guides/`, `docs/background/`, and `plan/`
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
- [ ] `mdsmith check .` reports zero diagnostics
