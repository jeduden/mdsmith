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
schema, the Go decoder, the matrix templates, every
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

Each rule README links its peer rules in its
Meta-Information section. The test
`internal/rules/peerlinks_test.go` builds that block.
Teach it the new peer's links:

- Add `{"newtool", info.Newtool}` to `peerLinkSpecs`
- Add a `peerRefID` case (a label prefix like
  `mdl-`, or the bare rule name for a shortcut peer)
- Add a `peerRefURL` case returning the per-rule doc
  URL; if the tool has no per-rule page, point every
  entry at one section anchor (as `mado` does)
- For a tool that pages rules by category (like
  obsidian-linter), add a name-to-category map and a
  `peerEntry` shortcut branch via `isShortcutPeer`

## 3. Extend the coverage matrix

The matrix lives in
`docs/research/markdownlint-coverage/README.md`. Most
sections are a `<?catalog?>` block per rule category,
each with a `header:` table and a `row-expr:` CUE
template that renders one cell per peer from the rule
README front matter. The `category: "directive"`
section is the exception: it is mdsmith-only, a
two-column `mdsmith | What it adds` table with no peer
cells. There is no dedicated Go coverage-matrix
generator to extend anymore; `mdsmith fix` renders
the `<?catalog?>` blocks from these templates.

Add the `newtool` column to every peer-coverage block
(all sections except the directive one), two edits
each:

- Add `newtool` to the `header:` block: extend both
  the column-names row and the alignment row of
  dashes (`| --- |`) below it.
- Append a peer cell to `row-expr:`. Copy an existing
  peer's cell and read the new key: `for m in
  newtool` when the key is a bare identifier, or the
  `fm["..."]` accessor when it has a hyphen — the way
  `obsidian-linter` is read as `for m in
  fm["obsidian-linter"]`.

A peer cell renders `—` for an empty list, otherwise
a comma-joined entry per mapping: the peer `id`, a
`✅`/`⚪` upstream-default marker, the `name` when it
differs from the `id`, and a ` (partial)` suffix.
Keep the new cell identical to the others so the
legend holds.

The page's `summary:` and opening paragraph name the
peers in prose; add `newtool` to both so they match
the columns.

Step 5 regenerates the tables, and `mdsmith check`
fails on any that drift.

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

## 5. Regenerate the matrix and rule README links

```bash
go run ./cmd/mdsmith fix docs/research/markdownlint-coverage/
MDSMITH_UPDATE_PEER_LINKS=1 go test ./internal/rules \
  -run TestRuleREADMEPeerLinks
go run ./cmd/mdsmith check internal/rules/ docs/research/
go test ./internal/release/ ./internal/rules/
```

`mdsmith fix` rebuilds the `<?catalog?>` tables in
place from the rule README front matter. The
`MDSMITH_UPDATE_PEER_LINKS` run rewrites the
peer-linter bullets in each rule README's
Meta-Information section from the same front matter;
a plain `go test ./internal/rules` fails when a
README has drifted. Commit both diffs in the same
change as the front-matter edits. `mdsmith check`
fails on a table left out of sync with the rule
READMEs.

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
- 67 rule READMEs
- the regenerated
  `docs/research/markdownlint-coverage/README.md`
- `docs/background/markdown-linters.md`
- `docs/research/benchmarks/README.md`
- this page
- `CLAUDE.md`
