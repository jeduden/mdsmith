---
title: Adding a peer linter
summary: How to wire a new peer Markdown linter into mdsmith's comparison docs, the per-rule coverage matrix, and the benchmark page.
---
# Adding a peer linter

mdsmith ships a peer-linter coverage matrix and a
prose comparison page. Both stay accurate because
every rule README owns its own peer-mapping front
matter; the matrix is regenerated from those blocks.
Adding a new peer — say `newtool` — touches the
schema, the Go decoder, the matrix generator, every
rule README, the prose comparison page, and the
benchmark page.

This page is the checklist. Each step lists the
files to touch and the validation that proves the
step landed.

## 1. Extend the front-matter schema

Two proto files declare what front-matter keys a
rule README is allowed to carry:

- `internal/rules/proto.md` — applied to most MDS
  rules
- `internal/rules/directive-proto.md` — applied to
  the directive rules (catalog, include, toc,
  build)

Add a `newtool:` line to both. The schema fragment
matches the existing peers: an open list of
`{id, name, partial?, default}` objects. Pick the
ID regex that matches the peer's naming:

- markdownlint-derived tools: `^MD[0-9]{3}$`
- kebab-case schemes (panache, obsidian-linter,
  most plugins): `^[a-z][a-z0-9-]*$`

Update the contributor comment block in
`proto.md` so it names the new key alongside the
existing four.

## 2. Wire the Go decoder

`internal/rules/ruledocs.go` parses the front
matter into `RuleInfo`. Three edits:

- Add `Newtool []RuleMapping` to the `RuleInfo`
  struct
- Add the matching field to the anonymous struct
  inside `parseFrontMatter` with the YAML tag
  `yaml:"newtool"`
- Copy `meta.Newtool` into `info.Newtool` at the
  end of `parseFrontMatter`

`go build ./...` should pass.

## 3. Extend the coverage generator

`internal/release/coverage.go` renders the matrix.
Four edits:

- Append `"newtool"` to the `headers` slice in
  `renderPeerTable`
- Append `renderPeerCell(r.Newtool)` to the row
  builder in the same function
- Add `len(r.Newtool) > 0` to the disjunction in
  `categoryIsMdsmithOnly`
- Update the page summary and intro paragraph that
  `RenderCoverageMatrix` writes — both currently
  enumerate the linters by name

`go test ./internal/release/` should pass. The
existing tests do not assert column count, so they
keep working with one extra column; add a new test
case if you want the assertion explicit.

## 4. Add per-rule mappings

Every README under `internal/rules/MDS*/` needs a
`newtool:` block in its front matter. Most get
`newtool: []`; only the rules that genuinely cover
a peer rule get a populated list.

A one-off Python script is the cheapest way to seed
the empties. Walk `internal/rules/MDS*/README.md`,
insert the block before the closing `---`, and skip
files that already declare the key. Then hand-edit
the rules that need a real mapping. Record the
peer's upstream default-enabled state on each
entry.

Two facts about defaults are easy to miss:

- markdownlint, rumdl, and mado mostly ship their
  rules enabled by default. Check the upstream
  config doc per rule.
- obsidian-linter ships every rule disabled by
  default — the plugin's `BooleanOption` sets
  `enabled: false` for all 65 rules. New plugin
  tools often follow the same opt-in pattern.

`mdsmith check internal/rules/` validates each rule
README against the proto schema. Run it after every
batch of edits.

## 5. Regenerate the matrix

```bash
go run ./cmd/mdsmith-release sync-coverage-matrix
go run ./cmd/mdsmith check internal/rules/ docs/research/
go test ./internal/release/ ./internal/rules/
```

The generator rewrites the matrix in place. Commit
the diff in the same change as the front-matter
edits. CI runs a drift check that catches a
matrix out of sync with the rule READMEs.

## 6. Update the comparison page

`docs/background/markdown-linters.md` is the
prose-style overview. Add a `### [newtool][]`
subsection between the existing entries:

- One-line characterisation of the tool
- 4–6 bullets covering rule count, config format,
  autofix model, and CLI / CI / LSP availability
- A two-column comparison table against mdsmith
- A "When to Use What" entry near the bottom of the
  page
- The link reference at the very bottom

If the new tool covers ground no other peer
touches, add a `#### newtool rules with no mdsmith
equivalent` subsection. Group the rules by theme
and keep each bullet short — MDS023 flags long
paragraphs, and a comma-separated list of 20
rule names is one long sentence.

## 7. Decide on the benchmark

The harness in `docs/research/benchmarks/run.sh`
expects a static binary that hyperfine can invoke
N times against a corpus directory. If `newtool`
ships that way, add it to the `tools` list in
`run.sh` and re-run the harness; commit the
refreshed `data/*.json`.

If it does not — e.g. an editor plugin without a
CLI — skip the harness. Write a short "Why newtool
is not benchmarked" subsection in
`docs/research/benchmarks/README.md` instead. Name
the structural reason: a plugin runtime that is
not AOT-compiled, defaults that produce a no-op
run, or a corpus shape (Quarto, MDX) that doesn't
match the harness.

## 8. Link from agent instructions

Add a bullet to `CLAUDE.md` under the project
docs catalog. Future agent runs find this page
when asked to wire in another peer linter.

## Reference: obsidian-linter as the worked example

The repo is the worked example. The PR that added
obsidian-linter touched, in order:

- `internal/rules/proto.md` and
  `directive-proto.md`
- `internal/rules/ruledocs.go`
- `internal/release/coverage.go`
- 67 rule READMEs
- the regenerated
  `docs/research/markdownlint-coverage/README.md`
- `docs/background/markdown-linters.md`
- `docs/research/benchmarks/README.md`
- this page
- `CLAUDE.md`
