---
id: 125
title: No space inside link text rule
status: "🔲"
summary: >-
  New rule MDS049 that flags Markdown links and
  images whose visible text has leading or trailing
  whitespace inside the brackets. Closes the gap with
  markdownlint MD039.
model: sonnet
---
# No space inside link text rule

## Goal

Let users forbid stray whitespace inside the visible
text of links and images. `[ click here ](url)`
renders the spaces as part of the underlined link
text in most renderers, which looks broken. The same
applies to image alt text. markdownlint covers this
as [MD039][md039]; mdsmith does not.

## Background

### What goldmark exposes

Inline links are `*ast.Link`. Inline images are
`*ast.Image`. The visible text is the node's child
sequence, but the rule needs the *source bytes*
inside the `[...]` brackets to detect whitespace
flanking the text — the AST already trims to text
nodes that may have lost the whitespace.

The rule reads `f.Source` between the opening `[` and
the closing `]` for each link/image node and inspects
the boundary bytes.

### Why a separate rule

MDS012 (no-bare-urls) and MDS027 (cross-file
reference integrity) handle URLs and link targets.
Neither looks at the *text* between the brackets. A
dedicated rule keeps text-formatting policy
independent of URL policy.

Reference-style links are out of scope here —
[plan 107](107_no-reference-style.md) forbids them
entirely. Until that rule is enabled, MDS049 also
applies to reference-link text (`[ text ][ref]`)
because the text portion is still visible.

## Design

### Configuration

```yaml
rules:
  no-space-in-link-text:
    enabled: true
    check-images: true
```

Category: `link`. Disabled by default (opt-in).
`check-images` allows opting out of image alt-text
checking when MDS032 (no-empty-alt-text) is doing
heavier alt-text work.

### Detection

Walk `*ast.Link` and `*ast.Image`. For each node:

1. Locate the bytes between the opening `[` and the
   matching closing `]` in `f.Source`.
2. If the first byte is whitespace (space, tab),
   emit `link text has leading whitespace` (or
   `image alt text has leading whitespace`).
3. If the last byte is whitespace, emit the
   trailing-whitespace variant.
4. Skip when `check-images: false` and the node is
   `*ast.Image`.

Newlines inside the brackets are *not* flagged —
they often reflect intentional wrapping of long
link text.

### Auto-fix

Trim leading and trailing whitespace inside the
brackets. Keep the brackets and any trailing
reference label or URL parenthetical untouched.

### Error messages

```text
link text has leading whitespace
link text has trailing whitespace
image alt text has leading whitespace
image alt text has trailing whitespace
```

## Tasks

1. Scaffold `internal/rules/nospaceinlinktext/` with
   `rule.go`, `rule_test.go`, and the `init()`
   `rule.Register` call.
2. Implement `Check()` walking `*ast.Link` and
   `*ast.Image`, locating the bracket span in
   `f.Source`.
3. Implement `rule.Configurable` for `check-images`.
4. Implement `Fix()` that trims whitespace inside
   the brackets.
5. Implement `rule.Defaultable` returning `false`.
6. Register as MDS049 in category `link`.
7. Add fixture tests in
   `internal/rules/MDS049-no-space-in-link-text/`
   covering: clean link, leading space, trailing
   space, both, image alt with leading space,
   reference link with whitespace text, and a link
   whose text wraps across a newline (not flagged).
8. Add rule README following the MDS012 template.

## Acceptance Criteria

- [ ] `[text](url)` emits no diagnostic.
- [ ] `[ text ](url)` emits leading and trailing
      diagnostics and fixes to `[text](url)`.
- [ ] `[text ](url)` emits one trailing diagnostic.
- [ ] `![ alt ](img.png)` emits two diagnostics with
      `image alt text` wording and fixes to
      `![alt](img.png)`.
- [ ] `![ alt ](img.png)` emits no diagnostic when
      `check-images: false`.
- [ ] A link whose text spans two source lines
      (newline between words) emits no diagnostic.
- [ ] `[ text ][ref]` emits diagnostics on the text
      portion only.
- [ ] Rule is disabled by default.
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
- [ ] `mdsmith check .` passes on the repo with the
      rule disabled (no regression for existing
      docs).
