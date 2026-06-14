---
summary: >-
  Reproducible evidence for the gomarklint-versus-mdsmith rule
  divergences — one fixture per claim, the gomarklint 3.2.3 and
  mdsmith commands, and both tools' verbatim output.
---
# gomarklint equivalence evidence

The [linter comparison][cmp] states three places where
gomarklint's line scanner diverges from mdsmith's AST checks,
plus one where gomarklint reports less. Each claim here is one
minimal fixture run through both tools, with verbatim output.
Source-reading alone does not settle a behavioural claim, so
every divergence below is a command you can re-run.

## Versions

- **gomarklint 3.2.3** — built from the `v3.2.3` tag of
  [`shinagawa-web/gomarklint`][gml] (commit `4c3dc17`) with
  `go build`. Its only direct dependencies are a glob library
  and cobra; it links no Markdown parser, so it scans lines
  rather than building a CommonMark AST.
- **mdsmith `v0.0.0-20260614184107-53bee322dcd6`** — this
  repository at the head of the change that adds this note.

## Method

Each case is one Markdown fixture plus a minimal
`.gomarklint.json` that enables only the rule under test
(`"default": false`). Both tools lint the same file:
gomarklint as `gomarklint <file>`, mdsmith as
`mdsmith check <file>`. Output is shown verbatim with color
disabled; gomarklint's trailing "Checked … in Nms" timing line
is elided because it varies run to run.

## max-line-length counts bytes, not characters

`.gomarklint.json`:

```json
{"default": false, "rules": {"max-line-length": {"enabled": true, "lineLength": 30}}}
```

`bytes.md` — line 3 is 20 CJK characters, which is 20 runes
but 60 bytes:

```markdown
# H

ああああああああああああああああああああ
```

gomarklint flags line 3 at a 30-unit limit, and its message
names the unit:

```text
Errors in bytes.md:
  bytes.md:3: [error] max-line-length: line exceeds 30 bytes (60)

✖ 1 issues found
```

mdsmith, with `line-length.max: 30` on the same file, counts
20 runes and stays silent:

```text
stats: checked=1 fixed=0 failures=0 unfixed=0
```

So at one shared limit the two tools disagree on a line of
multi-byte text. mdsmith counts characters
(`utf8.RuneCount`); gomarklint counts bytes.

## duplicate-heading counts headings inside code fences

`.gomarklint.json`:

```json
{"default": false, "rules": {"duplicate-heading": true}}
```

`dupfence.md` — the second `# Build` is inside a fenced code
block, so it is code, not a heading:

````markdown
# Build

```text
# Build
```
````

gomarklint has no fenced-block state in this rule, so it reads
line 4 as a heading and reports a duplicate:

```text
Errors in dupfence.md:
  dupfence.md:4: [error] duplicate heading: "build"

✖ 1 issues found
```

mdsmith resolves headings from the AST, where line 4 is a code
line, so there is no duplicate:

```text
stats: checked=1 fixed=0 failures=0 unfixed=0
```

## link-fragments resolves anchors in one file only

`.gomarklint.json`:

```json
{"default": false, "rules": {"link-fragments": {"enabled": true, "slug-algorithm": "github"}}}
```

`links.md` — one broken same-file anchor and one broken
cross-file anchor:

```markdown
# Title

[same-file broken](#nope)

[cross-file broken](other.md#nope)
```

gomarklint reports the same-file fragment only; its message
says "in this document". The cross-file `other.md#nope` is not
checked:

```text
Errors in links.md:
  links.md:3: [error] link-fragments: fragment #nope not found in this document

✖ 1 issues found
```

mdsmith's whole-repo graph (MDS027) reports both — including
the cross-file target gomarklint passed over:

```text
links.md:3:2 MDS027 broken link target "#nope" has no matching heading anchor
links.md:5:2 MDS027 broken link target "other.md#nope" not found
stats: checked=1 fixed=0 failures=2 unfixed=2
```

## no-bare-urls skips a blank-line-fenced URL

`.gomarklint.json`:

```json
{"default": false, "rules": {"no-bare-urls": true}}
```

`urls.md` — one URL inline in a sentence, one URL alone
between blank lines:

```markdown
# Links

Inline https://example.com/a here.

https://example.com/b
```

gomarklint reports the inline URL only. A URL alone between
blank lines is treated as a renderer link-card and left
unflagged:

```text
Errors in urls.md:
  urls.md:3: [error] no-bare-urls: bare URL found, use angle brackets or a Markdown link: https://example.com/a

✖ 1 issues found
```

mdsmith (MDS012) reports both, including the standalone URL on
line 5:

```text
urls.md:3:8 MDS012 bare URL — wrap in angle brackets or add link text
urls.md:5:1 MDS012 bare URL — wrap in angle brackets or add link text
stats: checked=1 fixed=0 failures=2 unfixed=2
```

This case runs the other way from the first three: here
gomarklint reports less, by design, than mdsmith.

[cmp]: ../../background/markdown-linters.md
[gml]: https://github.com/shinagawa-web/gomarklint
