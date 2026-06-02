---
settings:
  inline-schema:
    sections:
      - heading: "Goal"
      - heading: "Tasks"
diagnostics:
  # Missing section anchors at the preceding heading (## Goal, line 3).
  - line: 3
    column: 1
    message: |-
      ## Tasks: got <missing>, expected section to be present
    related:
      - message: "inline kind schema"
---
# My Plan

## Goal

Describe the goal here.
