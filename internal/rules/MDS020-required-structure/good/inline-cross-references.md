---
settings:
  inline-schema:
    cross-references:
      - pattern: "\\bStep (\\d+)\\b"
        must-match: "Step {n}"
        skip-lines-matching: "^> "
---
# Runbook

## Diagnosis

The user-facing summary: see Step 1 and Step 2 for the workflow.

> Historical note: Step 9 was removed in v1 — this blockquote is exempt.

## Step 1

Inspect the queue.

## Step 2

Drain the queue.
