---
id: 206
title: Document cue/ in architecture layering map
status: "🔲"
summary: >-
  cue/types was added as a public Go package outside
  internal/ and pkg/, but is not in the layering map.
  Also add the required exemption comment to
  cue/types.Source().
model: ""
depends-on: []
---
# Document cue/ in architecture layering map

## Goal

[cue/types/types.go](../cue/types/types.go) was added and is
imported by [internal/schema/shortcuts.go](../internal/schema/shortcuts.go).
It sits outside both `internal/` and
`pkg/`, analogous to [pkg/markdown](../pkg/markdown) but
for CUE consumers. Two gaps need fixing:

1. `cue/` is absent from the layering
   diagram in
   `docs/development/architecture/index.md`.
2. `Source()` is a trivial accessor that
   qualifies for the test exemption but
   lacks the required comment.

## Tasks

1. Update the layering diagram in
   `docs/development/architecture/index.md`
   to show `cue/types` at the bottom
   layer alongside `pkg/markdown`.
2. Add one sentence in the layering
   prose explaining `cue/types`.
3. Add the comment
   `// no test by design: trivial embed accessor`
   to `func Source()` in `cue/types/types.go`.
4. Run `go run ./cmd/mdsmith check .`.

## Acceptance Criteria

- [ ] The layering diagram in
  `docs/development/architecture/index.md`
  shows `cue/types`.
- [ ] `Source()` has a "no test by design"
  comment.
- [ ] `go run ./cmd/mdsmith check .`
  reports no issues.
