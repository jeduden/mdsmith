---
id: 168
title: Obsidian Flavored Markdown support
status: "🔲"
summary: >-
  Add an `obsidian` convention that validates wikilinks
  (`[[Page]]`) via extended MDS027 settings, checks callout
  types via new rule MDS059, and tolerates Dataview inline
  fields without surfacing false-positive diagnostics.
model: sonnet
---
# Obsidian Flavored Markdown support

## Goal

Teams that write docs in Obsidian vaults can run mdsmith on
their `.md` files. This plan adds three things: broken
wikilink detection, callout type validation, and no false
positives from OFM syntax that CommonMark parsers already
treat as plain text.

## Background

Obsidian Flavored Markdown (OFM) adds four constructs on
top of CommonMark/GFM that mdsmith currently ignores:

| Construct       | OFM syntax                                       | mdsmith today                   |
|-----------------|--------------------------------------------------|---------------------------------|
| Wikilink        | `[[Page]]`, `[[Page\|alias]]`, `[[Page#anchor]]` | silent (parsed as text)         |
| Embed           | `![[file.png]]`, `![[note]]`                     | silent (parsed as text)         |
| Callout         | `> [!note]` at blockquote start                  | silent (treated as prose)       |
| Dataview inline | `key:: value`                                    | silent (read as paragraph text) |

The [research sketches][L-1-sketch] a `wikilinks` setting on
MDS027 as the right surface for wikilink validation. The
[backlinks plan][plan138] explicitly defers wikilink coverage
to this plan. Dataview inline fields already pass all
existing rules without any diagnostic. The only gap is that
`require:`/`schema:` directives don't read them as front
matter — out of scope here.

### Wikilink resolution (Obsidian style)

Obsidian resolves `[[Page]]` by searching the entire vault
for any file whose stem matches `Page` (case-insensitive).
It prefers the shortest relative path when multiple files
match. `[[Page#Heading]]` also validates the heading anchor
in the resolved file. `[[Page|alias]]` uses `Page` as the
target; `alias` is display text only.

Embeds (`![[file.png]]`) follow the same resolution rules
but target any file type, not just `.md`.

### Callout types

Obsidian defines 13 base callout types, each with aliases:

- `note`
- `abstract` (aliases: `summary`, `tldr`)
- `info` (alias: `todo`)
- `tip` (aliases: `hint`, `important`)
- `success` (aliases: `check`, `done`)
- `question` (aliases: `help`, `faq`)
- `warning` (aliases: `caution`, `attention`)
- `failure` (aliases: `fail`, `missing`)
- `danger` (alias: `error`)
- `bug`
- `example`
- `quote` (alias: `cite`)

Type matching is case-insensitive. Custom types are valid in
Obsidian but opt-in here via the `allow` setting.

## Design

### 1. Wikilink extractor in `internal/linkgraph/`

Add `wikilinks.go` to the `linkgraph` package with a
source-level scanner. It must skip code spans, fenced code
blocks, and `<?...?>` PI blocks (same guards as MDS054's
bracket scanner). It returns a slice of `WikiLink`:

```go
type WikiLink struct {
    Target string // "Page", "Page#anchor", "Page.png"
    Anchor string // heading fragment, if present
    Alias  string // display alias, if present
    Embed  bool   // true for ![[...]]
    Line   int    // 1-based source line
    Col    int    // 1-based byte offset within line
}
```

Resolution helper `ResolveWikiLink(root fs.FS, from, target
string) (path string, ok bool)` implements the
shortest-path Obsidian algorithm:

1. If `target` ends in `.md` or has no extension, search
   for files whose stem matches `target` (case-insensitive).
2. Otherwise (embed with extension), search by exact name.
3. Return the match with the fewest path components. Ties
   break alphabetically.
4. Search is sandboxed to the workspace root (no `..`).

### 2. Extend MDS027 with wikilink settings

Add two new settings to
`internal/rules/crossfilereferenceintegrity/`:

```yaml
rules:
  cross-file-reference-integrity:
    wikilinks: false         # default; set true to enable
    wikilink-style: obsidian # only supported style initially
```

When `wikilinks: true`, `Check()` calls
`linkgraph.ExtractWikiLinks(f)` and runs each through
`ResolveWikiLink`. Unresolved targets emit:

```text
wikilink target "Missing Page" not found in workspace
```

Resolved targets with a missing heading anchor emit:

```text
wikilink "[[Notes#Old Heading]]": anchor "Old Heading"
not found in docs/notes.md
```

The existing `placeholders` setting applies to wikilink
targets the same way it applies to standard link
destinations.

### 3. New rule MDS059: `callout-type`

New package `internal/rules/callouttype/`.

```yaml
rules:
  callout-type:
    allow: []           # empty = use built-in Obsidian set
    allow-unknown: false
```

`allow: []` (default) permits only the 13 base types plus
aliases. `allow: [custom]` adds `custom` to the valid set.
`allow-unknown: true` disables all type validation.

Detection scans blockquote nodes. For each, check whether
the first non-whitespace line matches
`\[!([A-Za-z0-9_-]+)\]`. If the capture group is not in
the effective allow set, emit:

```text
unknown callout type "REVIEW"; valid types: note, abstract,
info, tip, success, question, warning, failure, danger, bug,
example, quote (or configure allow-unknown: true)
```

Category: `structure`. Disabled by default (opt-in). No
auto-fix — renaming a callout type changes meaning.

### 4. Update `backlinks` to include wikilinks

Extend `mdsmith list backlinks` to call
`linkgraph.ExtractWikiLinks` unconditionally. Extraction is
cheap and read-only. The command already has a `--json`
flag; wikilink sources appear there with
`"kind": "wikilink"`.

### 5. `obsidian` convention

Add to `internal/convention/convention.go`:

```go
"obsidian": {
    Name:   "obsidian",
    Flavor: FlavorGFM,
    Rules: map[string]RulePreset{
        "markdown-flavor": {
            Enabled:  true,
            Settings: map[string]any{"flavor": "gfm"},
        },
        "cross-file-reference-integrity": {
            Enabled: true,
            Settings: map[string]any{
                "wikilinks":      true,
                "wikilink-style": "obsidian",
            },
        },
        "callout-type": {Enabled: true},
    },
},
```

`convention: obsidian` activates both rules with one config
line. Standard Markdown link settings stay at their
defaults.

### 6. Docs updates

Add `obsidian` to the built-in convention table in
`docs/reference/conventions.md`.

Update `docs/background/markdown-linters.md`. The Obsidian
comparison table rows for wikilinks and callouts change from
"no validation" to cite the new rules.

## Tasks

1. Add `internal/linkgraph/wikilinks.go` with `WikiLink`,
   `ExtractWikiLinks(f *lint.File)`, and
   `ResolveWikiLink(root fs.FS, from, target string)`. Add
   unit tests covering: bare page, page with anchor, page
   with alias, embed, code-span skipping, fenced-code
   skipping, PI-block skipping, case-insensitive match,
   shortest-path tie-break, not-found, root-escape rejected.
2. Extend `internal/rules/crossfilereferenceintegrity/` with
   `wikilinks bool` and `wikilink-style string` fields.
   Update `ApplySettings`, `DefaultSettings`, and `Check()`
   to call `ExtractWikiLinks` when enabled. Add tests for
   resolved, unresolved, anchor-broken, and
   placeholder-suppressed wikilink diagnostics. Add wikilink
   fixtures in `internal/rules/MDS027-cross-file-reference-integrity/`.
3. Scaffold `internal/rules/callouttype/` with `rule.go`
   and `rule_test.go`. Implement `Check()`, `ApplySettings`,
   `DefaultSettings`, and `EnabledByDefault`. Register as
   MDS059 in category `structure`. Add fixtures in
   `internal/rules/MDS059-callout-type/`: `good/` covers
   standard types and `allow-unknown: true`; `bad/` covers
   unknown type flagging.
4. Add `obsidian` convention entry in
   `internal/convention/convention.go`.
5. Extend `mdsmith list backlinks` to call
   `ExtractWikiLinks`; add `"kind": "wikilink"` to JSON.
6. Update `docs/reference/conventions.md` and
   `docs/background/markdown-linters.md`.
7. Run `go run ./cmd/mdsmith fix .` and confirm
   `go run ./cmd/mdsmith check .` passes.

## Acceptance Criteria

- [ ] `[[Missing]]` with `wikilinks: true` emits one
      diagnostic naming the unresolved target.
- [ ] `[[Present]]` where `present.md` exists emits no
      diagnostic.
- [ ] `[[Notes#Old Heading]]` with a missing anchor emits
      one anchor-not-found diagnostic.
- [ ] `[[Page|Alias]]` resolves `Page`, not `Alias`.
- [ ] `![[image.png]]` resolves as an embed (any file type).
- [ ] Wikilinks inside fenced code, code spans, and PI
      blocks are never flagged.
- [ ] A wikilink target in `placeholders` is never flagged.
- [ ] `wikilinks: false` (default) emits no wikilink
      diagnostics.
- [ ] `> [!note]` (and all 13 base types + aliases)
      emits no diagnostic from MDS059 with default settings.
- [ ] `> [!REVIEW]` (unknown type) emits one diagnostic
      naming the type and listing valid options.
- [ ] `> [!custom]` with `allow: [custom]` emits no
      diagnostic.
- [ ] `> [!anything]` with `allow-unknown: true` emits no
      diagnostic.
- [ ] MDS059 is disabled by default.
- [ ] `convention: obsidian` activates both rules with no
      other config.
- [ ] `mdsmith list backlinks docs/page.md` lists files
      with `[[page]]` wikilinks pointing at `page.md`.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes on the repo.

## Non-Goals

- Dataview inline field recognition in `require:`/`schema:`
  — those lines already pass without errors.
- Auto-fix to rewrite wikilinks as standard Markdown links
  — that breaks Obsidian for users who want wikilinks.
- Non-Obsidian wikilink styles (Foam, Logseq) — the
  `wikilink-style` key is extensible but only `obsidian`
  ships here.
- LSP hover or completion for wikilinks — follow-up work.

## See also

- [MDS027 cross-file-reference-integrity][mds027]
- [Plan 138: backlinks subcommand][plan138]
- [Research sketch L-1][L-1-sketch]

[mds027]: ../internal/rules/MDS027-cross-file-reference-integrity/README.md
[plan138]: 138_backlinks-subcommand.md
[L-1-sketch]: ../docs/research/mdbase-vs-mdsmith/learn-from-mdbase.md
