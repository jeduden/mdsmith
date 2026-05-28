---
id: 218
title: Add internal/punkt to the architecture layering map
status: "🔲"
summary: >-
  internal/punkt was vendored in commit a1aa6c5 as
  an allocation-clean Punkt sentence tokenizer,
  imported only by internal/mdtext. It is not
  listed in the SRP section of go.md. Add a
  one-line entry so the package is discoverable.
model: ""
depends-on: []
---
# Add internal/punkt to the architecture layering map

## Goal

[internal/punkt](../internal/punkt/) is a
vendored fork of the Punkt sentence
tokenizer added by commit `a1aa6c5`. Its
only importer is
[internal/mdtext](../internal/mdtext/).

The SRP bullet list in
[`go.md`](../docs/development/architecture/go.md)
lists core packages but omits it. One
line added there anchors the rule that
only `internal/mdtext` may import it.

## Tasks

1. Add a bullet to the SRP section of
   `docs/development/architecture/go.md`
   after the `internal/mdtext` entry:

   ```markdown
   - `internal/punkt` — segment a byte
     sequence into sentences (vendored
     Punkt fork); imported only by
     `internal/mdtext`.
   ```

2. Run `go run ./cmd/mdsmith fix .`
   to refresh generated sections.
3. Run `go run ./cmd/mdsmith check .`
   to confirm no lint violations.

## Acceptance Criteria

- [ ] `internal/punkt` appears in the SRP
  table in `go.md`, after
  `internal/mdtext`.
- [ ] Entry states what it answers and
  that only `internal/mdtext` imports it.
- [ ] `go run ./cmd/mdsmith check .`
  passes.
