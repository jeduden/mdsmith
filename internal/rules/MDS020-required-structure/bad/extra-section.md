---
settings:
  schema: "../../internal/rules/MDS020-required-structure/bad/data/tmpl.md"
diagnostics:
  - line: 3
    column: 1
    message: |-
      ## Extra: got <present>, expected not declared in schema
        (expected "## Goal" here instead)
---
# My Plan

## Extra

Extra description.

## Goal

Goal description.

## Tasks

Tasks description.
