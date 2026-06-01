---
settings:
  schema: "../../internal/rules/MDS020-required-structure/bad/data/tmpl.md"
diagnostics:
  - line: 3
    column: 1
    message: |-
      ## Tasks: got <out of order>, expected in declared order
        (expected after "## Goal")
---
# My Plan

## Tasks

Tasks description.

## Goal

Goal description.
