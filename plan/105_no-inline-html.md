---
id: 105
title: No inline HTML rule
status: "🔲"
summary: >-
  New rule MDS041 that flags raw HTML in Markdown
  (block and inline), with an allowlist for tags
  that have no Markdown equivalent. Closes the
  largest attack surface for XSS and the largest
  source of parser ambiguity, per the "Why are we
  using Markdown?" bgslabs.org rant.
model: sonnet
---
# No inline HTML rule

## Goal

Let users forbid raw HTML in their Markdown. Inline
HTML drives most XSS CVEs in Markdown renderers. It
also drives most parser ambiguity, since every
Markdown parser must also be a forgiving HTML parser.
Teams that adopt this rule keep their corpus in pure
Markdown plus the project's [directive
vocabulary](../docs/background/concepts/generated-section.md).

## Background

### What counts as inline HTML

Goldmark's CommonMark parser produces two AST node
types for raw HTML:

- `*ast.HTMLBlock` — block-level HTML (a `<div>` on
  its own line, an HTML comment paragraph, a `<table>`
  block).
- `*ast.RawHTML` — inline HTML inside a paragraph
  (`text <span>marked</span> text`, `<br>`, `<kbd>x</kbd>`).

The PI block parser
([internal/lint/pi.go](../internal/lint/pi.go))
already replaces block-level `<?name ... ?>`
directives with a distinct AST node, so block
directives are *not* HTMLBlocks. Inline
`<?name ... ?>` inside a paragraph lands as
`*ast.RawHTML`. The rule must skip both forms
unconditionally — see "What is *not* flagged"
below. HTML comments (`<!-- ... -->`) remain
`*ast.HTMLBlock`s — see the allowlist discussion
below.

### Why a separate rule from MDS034

MDS034 (markdown-flavor) restricts which *Markdown
extensions* a file may use. Raw HTML is part of
CommonMark itself, so MDS034 cannot flag it without
overloading its meaning. A dedicated rule keeps the
two policies independently togglable: a team can
allow GFM tables but still forbid raw `<table>`.

### Prior art

- markdownlint's MD033 (`no-inline-html`) takes the
  same shape: a single boolean plus an `allowed_elements`
  list. We follow that interface so users migrating
  from markdownlint get a familiar knob.
- markdownlint flags by tag name (string). We do the
  same — the AST node carries the raw bytes, from
  which we extract the tag name with a small regex
  (`<\/?([a-zA-Z][a-zA-Z0-9-]*)`).

## Design

### Configuration

```yaml
rules:
  no-inline-html:
    allow: []          # tag names that are permitted
    allow-comments: true   # <!-- ... --> stay legal by default
```

Category: `meta`. Disabled by default (opt-in) — the
mdsmith repo itself uses inline HTML in places (e.g.
`<sub>`, `<details>` in docs) and existing users
should not regress.

`allow` is a list-typed setting. It **replaces** by
default, matching the rest of the codebase except
`placeholders:`. A team that wants to extend the
default empty list does so by listing every tag they
need.

### Flavor activation

[Plan 112](112_flavor-profiles.md) ships profiles
that auto-enable this rule:

- `profile: portable` activates with empty
  `allow` and `allow-comments: true`.
- `profile: github` activates with
  `allow: [details, summary, sub, sup, kbd]`.
- `profile: plain` activates with empty `allow`
  and `allow-comments: false` (HTML comments leak
  through as literal text in plaintext readers).

User overrides on top of the profile still win via
deep-merge.

### Detection

Walk `f.AST`. For every `*ast.HTMLBlock` and every
`*ast.RawHTML`:

1. If the bytes start with `<?`, skip
   unconditionally. This covers inline mdsmith
   directives that the block PI parser does not
   reach. The allowance is not configurable.
2. Extract the tag name (lowercase) from the raw
   bytes. If the bytes start with `<!--`, treat as a
   comment and skip when `allow-comments` is true.
3. If the tag name is in `allow`, skip.
4. Otherwise emit a diagnostic at the node's start
   line/column: `inline HTML <{tag}> is not allowed;
   use a Markdown construct or an mdsmith directive
   instead`.

Closing tags (`</div>`) emit no extra diagnostic —
the opening tag already produced one. Self-closing
tags (`<br/>`, `<img/>`) count once.

### What is *not* flagged

- Fenced and indented code blocks (the AST keeps
  these as `*ast.FencedCodeBlock` / `*ast.CodeBlock`,
  not HTML).
- Inline code spans (`*ast.CodeSpan`).
- Autolinks (`<https://example.com>`) and email
  autolinks — these are `*ast.AutoLink`, not RawHTML.
- mdsmith directives (`<?name ... ?>`) — block
  forms are carved out by the PI parser; inline
  forms are skipped explicitly by step 1 of
  Detection. The allowance is unconditional and not
  affected by `allow:` or `allow-comments:`.
- HTML entities in text (`&amp;`, `&#x2014;`) — these
  are `*ast.Text` after entity decoding; flagging
  them would be a separate rule.

### Auto-fix

No auto-fix in v1. The right replacement is
context-dependent: `<br>` may want `  \n`, `<b>` may
want `**`, `<details>` wants a future
`<?details?>` directive that does not exist yet.
Emitting a wrong fix is worse than emitting none.
Track a follow-up plan once a directive replacement
exists for the most common allowlisted tags.

### Error messages

Single message template, lowercase, no trailing
punctuation, per the project's code style:

```text
inline HTML <{tag}> is not allowed
```

When `allow-comments: false` flags a comment, the
tag is reported as `<!--` so the message stays
distinct.

## Tasks

1. Scaffold `internal/rules/noinlinehtml/` with
   `rule.go`, `rule_test.go`, and the `init()`
   `rule.Register` call.
2. Implement `Check()` walking the AST for
   `*ast.HTMLBlock` and `*ast.RawHTML`.
3. Add tag-name extraction helper with unit tests
   covering opening, closing, self-closing,
   uppercase, hyphenated (`<my-tag>`), and malformed
   inputs.
4. Implement `rule.Configurable` for `allow` and
   `allow-comments`; document `allow` as
   replace-mode in the rule's `ApplySettings`
   handler.
5. Implement `rule.Defaultable` returning `false`
   so the rule is opt-in.
6. Register as MDS041 in category `meta`.
7. Add fixture tests in
   `internal/rules/MDS041-no-inline-html/` with
   `good/` and `bad/` examples (paragraph with
   `<span>`, block with `<div>`, comment, allowlisted
   tag, directive, autolink — verify the directive
   and autolink fixtures stay clean).
8. Add rule README following the MDS012 template.
9. Update the docs catalog and the rule index so
   `<?catalog?>` regenerates the entry.
10. Run `mdsmith check .` and `go test ./...` until
    both pass.

## Acceptance Criteria

- [ ] `<div>...</div>` on its own line emits one
      diagnostic naming the `<div>` tag.
- [ ] `text <span>x</span> text` inside a paragraph
      emits one diagnostic naming the `<span>` tag.
- [ ] `<br/>` and `<br>` both emit exactly one
      diagnostic.
- [ ] A tag listed in `allow:` emits no diagnostic.
- [ ] `<!-- comment -->` emits no diagnostic when
      `allow-comments: true`, and one diagnostic
      naming `<!--` when `allow-comments: false`.
- [ ] `<?include file: foo.md ?>` and
      `<?catalog ... ?>` block directives emit no
      diagnostic.
- [ ] `text <?inline?> text` inline directive emits
      no diagnostic.
- [ ] An inline `<?...?>` directive emits no
      diagnostic even when `allow: []` and
      `allow-comments: false`.
- [ ] `<https://example.com>` autolink emits no
      diagnostic.
- [ ] Fenced code blocks containing HTML emit no
      diagnostic.
- [ ] Rule is disabled by default; enabling it via
      `rules.no-inline-html: true` activates checking
      with empty allowlist.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes on the repo with the
      rule disabled (no regression for existing
      docs).
