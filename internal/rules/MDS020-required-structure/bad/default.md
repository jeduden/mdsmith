---
settings:
  schema: "../../internal/rules/MDS020-required-structure/bad/data/tmpl.md"
diagnostics:
  # Missing section anchors at the preceding heading (## Goal, line 3),
  # not file line 1 (plan 230 body anchoring).
  - line: 3
    column: 1
    message: |-
      ## Tasks: got <missing>, expected section to be present
    related:
      - file: "../../internal/rules/MDS020-required-structure/bad/data/tmpl.md"
        message: "required by schema"
---
# My Plan

## Goal

Describe the goal here.
