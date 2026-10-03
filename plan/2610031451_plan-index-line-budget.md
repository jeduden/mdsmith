---
id: 2610031451
title: Keep PLAN.md under its file-length budget
status: "🔲"
model: sonnet
summary: >-
  PLAN.md holds one catalog row per plan and sits a line or
  two under the 300-line max-file-length budget. Its catalog
  YAML has been compacted three times to make room, and
  nothing is left to compact. Split completed plans into
  their own index so new plans do not break CI.
---
# Keep PLAN.md under its file-length budget

## Goal

Adding a plan row to [PLAN.md](../PLAN.md) never pushes it
over the MDS022 `max-file-length` budget.

## Background

The PR #895 review counted 299 of 300 lines in PLAN.md
after this plan's row. To stay under the budget, three
changes squeezed the catalog directive. The `glob:` list
went to flow style, `footer: ""` was added, and the
`header:` block became a one-line quoted string. Each
change keeps the generated table byte for byte, so none
hides a violation, but no YAML is left to compact. The
next new plan fails `mdsmith check .`, the
`mdsmith-fixed-version` job, and `bench-fragments`.

Two fixes exist:

- Raise the PLAN.md `max-file-length` in `.mdsmith.yml`.
  CLAUDE.md requires the user's consent for any edit to
  that file.
- Move completed plans (`status: "✅"` and `"⛔"`) into a
  second catalog file, so PLAN.md lists only open work.
  The [pick-plan skill](../.claude/skills/pick-plan/SKILL.md)
  reads PLAN.md, so it needs a matching update.

## Tasks

1. Ask the user which fix to take, and stop if neither is
   approved.
2. For the split: add a catalog file for completed plans
   that filters on `status`, and narrow the PLAN.md
   catalog to open plans. Keep the same columns.
3. Update the pick-plan skill and any doc that says
   PLAN.md lists every plan.
4. Run `go run ./cmd/mdsmith fix PLAN.md` and the new
   file, then `go run ./cmd/mdsmith check .`.

## Acceptance Criteria

- [ ] PLAN.md has at least 50 lines of headroom under its
      `max-file-length` budget
- [ ] Every plan file appears in exactly one catalog
- [ ] The pick-plan skill still finds every open plan
- [ ] `go run ./cmd/mdsmith check .` passes
- [ ] All tests pass: `go test ./...`
- [ ] `go tool -modfile=tools/go.mod golangci-lint run`
      reports no issues
