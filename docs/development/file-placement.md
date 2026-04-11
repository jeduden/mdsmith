---
title: File Placement
summary: Where to place Markdown files and documentation types.
---
# File Placement

Every Markdown file checked by mdsmith must live in
one of the allowed directories. The
`directory-structure` rule (MDS033) enforces this for
linted files. When creating a new `.md` file, use the
decision list below — take the **first match**.

## Decision list

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
