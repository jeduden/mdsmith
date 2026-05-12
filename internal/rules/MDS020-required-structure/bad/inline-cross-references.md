---
settings:
  inline-schema:
    cross-references:
      - pattern: "\\bStep (\\d+)\\b"
        must-match: "Step {n}"
diagnostics:
  - line: 3
    column: 1
    message: 'cross-reference "Step 7" does not resolve to a heading (looked for "Step 7")'
---
# Runbook

Follow Step 7 to continue.

## Step 1

Only Step 1 exists.
