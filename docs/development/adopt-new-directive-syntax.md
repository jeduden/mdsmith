---
title: Ship and adopt new directive syntax
summary: >-
  Release new directive syntax and bump the pinned
  CI baseline before checked-in Markdown uses it, so
  the `mdsmith-fixed-version` job stays green.
---
# Ship and adopt new directive syntax

mdsmith lints its own repository. The
`mdsmith-fixed-version` job runs `mdsmith check .`
with the pinned release binary, not the branch
build. The job lives in `.github/workflows/ci.yml`;
the pin lives in the `setup-mdsmith-pinned-version`
action. So checked-in Markdown can use only
directive syntax that the pinned release already
parses. Adopt new syntax in three ordered steps.

## Steps

1. **Ship the feature in a tagged release.** Land
   the parser and renderer change, then cut a
   release so the new syntax exists in a published
   binary.
2. **Bump the pinned baseline.** Set
   `MDSMITH_VERSION` and `MDSMITH_SHA256` in
   `.github/actions/setup-mdsmith-pinned-version/action.yml`
   to that release. Now `mdsmith-fixed-version`
   runs a binary that parses the new syntax.
3. **Adopt the syntax.** Migrate the repository's
   own Markdown to the new directive. `mdsmith
   check .` then passes under both the branch
   binary and the pinned one.

Do step 3 last. If checked-in Markdown uses syntax
the pinned binary cannot parse,
`mdsmith-fixed-version` fails. The job is the
guardrail: it blocks adoption before the pin is
ready.

## Track the adoption

The guardrail blocks only early adoption. Nothing
forces step 3 after step 2, so a released feature
can sit unused. Track the gap in `PLAN.md`:

- Keep the feature's plan at `🔳` with the adoption
  task gated on the pin bump. The open plan is the
  reminder.
- When you bump the pin in step 2, scan `PLAN.md`
  for plans waiting to adopt the new syntax.
  Schedule them.
