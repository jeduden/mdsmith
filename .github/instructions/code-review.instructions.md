---
title: Code Review Instructions
applyTo: "**/*.go"
excludeAgent: "coding-agent"
---
# Code Review Instructions

## Formatting and Import Style

Do not flag import grouping or formatting in Go files.
This project uses golangci-lint with goimports as the
formatter (see `.golangci.yml`). The goimports config
produces three import groups with blank lines between:

1. Standard library
2. Project-internal (`github.com/jeduden/mdsmith/...`)
3. Third-party (`github.com/stretchr/testify/...`)

If golangci-lint CI passes, the formatting is correct.
Do not suggest collapsing groups 2 and 3 into one.

## Focus Areas

Focus reviews on correctness, logic errors, security,
test coverage gaps, and API misuse.

Skip formatting, whitespace, import ordering, and any
style already enforced by linters.
