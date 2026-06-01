---
settings:
  schema: "../../internal/rules/MDS020-required-structure/bad/data/tmpl.md"
diagnostics:
  - line: 1
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
