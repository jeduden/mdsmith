---
id: 52
title: Archetype / Template Library for Agentic Patterns
status: 🔲
---
# Archetype / Template Library for Agentic Patterns

## Goal

Provide a richer template system with pre-built archetypes
for common agentic Markdown artifacts.

## Tasks

1. Define supported archetype metadata and required-structure enforcement rules.
2. Add archetype templates for common agent patterns
   (story-file, prd, agent-definition, claude-md).
3. Update required-structure rule
   to validate archetype selection and required sections.
4. Document archetype usage and configuration.

## Acceptance Criteria

- [ ] Archetype templates are available for common agentic document types.
- [ ] Required-structure rule validates selected archetype sections.
- [ ] Documentation explains how to add and configure archetypes.
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
