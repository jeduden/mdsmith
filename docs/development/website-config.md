---
title: Website configuration
summary: >-
  Design rationale for `website/hugo.toml` — module mounts,
  Goldmark renderer flags, Chroma highlight style, and the
  version stamp shared with `mdsmith-release`. The TOML file
  itself is now machine-edited by `mdsmith-release
  sync-messaging`, which re-emits the file via the TOML
  library and drops inline comments. This page is the home
  for those comments.
---
# Website configuration

The `website/hugo.toml` file ships the Hugo site at
`mdsmith.dev`. Two surfaces edit it programmatically:

- `mdsmith-release stamp <version>` rewrites `[params].version`
  from the dev sentinel to the release tag before
  `hugo --minify`. See
  [release tooling](release-tooling.md).
- `mdsmith-release sync-messaging` rewrites
  `[params].description` from
  `docs/brand/messaging.md`. See
  [plan 210](../../plan/210_messaging-source-of-truth.md).

Both round-trip the file through a real TOML parser, so the
file does not carry inline rationale comments. The rationale
lives here.

## `[module]` mounts

Hugo treats `website/` as the site root. The canonical docs
live in the sibling `docs/` tree at the repo root. The two
trees are not mounted directly. Hugo parses shortcodes before
Markdown. Literal `{{< ... >}}` patterns in files like
`docs/background/markdown-linters.md` would break the build.

Before the Hugo build, `mdsmith-release build-website`
snapshots the source tree into `website/content/docs/`. The
snapshot escapes the shortcode patterns. It drops
`proto.md` templates. It renames `index.md` to `_index.md`.
The output directory is `.gitignore`d.

The synced tree lives on disk under `content/docs/` so the
`.mdsmith.yml` lint globs that target
`website/content/docs/**` keep matching. It is mounted at the
content root rather than under a `docs` segment: every doc
page is served at `/<section>/...`, not `/docs/<section>/...`.
The only index is the homepage
(`content/_index.md`, served at `/`).

The first mount excludes `docs/**` so the unsynced
`content/docs/` directory is not also exposed under `/docs/`;
the second mount re-roots it.

## `[markup.goldmark.renderer]` `unsafe = false`

Synced docs carry no raw HTML. `sync-docs` strips
`<?...?>` directive markers and the content uses Markdown
code spans (not literal `<code>` / `<…>` tags), so
Goldmark's default raw-HTML filtering loses nothing.

Keep `unsafe = false`. Raw HTML in any doc would silently
vanish, flagging the author to fix the source rather than
ship unsanitized markup to `mdsmith.dev`.

## `[markup.highlight]` style

`noClasses = true` emits inline styles rather than CSS
classes. The site does not ship a Chroma stylesheet under
`static/css/`, so the class-only form would render code
blocks unstyled. Switch to `noClasses = false` (and vendor
`chroma-<style>.css`) only when we want overridable token
classes.

`style = "github-dark"` is a dark style chosen because every
code surface on the site renders on the steel-900 panel (see
`pre` / `.codeblock pre` in CSS). The old light "github"
style emitted near-black token colors on that dark panel —
illegible (the "white/grey on grey" bug). `github-dark`'s
light token colors sit on the dark panel with full contrast.
CSS pins the panel background so Chroma's own background
does not introduce a second, mismatched shade.

## `[params].version`

Tracked by `mdsmith-release stamp` (see
[`internal/release/version.go`](../../internal/release/version.go)
`TrackedManifests`). Between releases the value is the dev
sentinel `0.0.0-dev`. The pages-deploy workflow rewrites it
to the cleaned tag (no leading `v`) before running
`hugo --minify`. Templates that display this should prefix
`v` in the rendered markup.

## `[params].description`

Tracked by `mdsmith-release sync-messaging` from the
`tagline` field of
[`docs/brand/messaging.md`](../brand/messaging.md). Hand-edits
to this field are reverted on the next sync. To change the
text, edit the source file and run the sync.

## Summary front-matter rendering

Each docs page carries a `summary` front-matter field.
The field holds inline Markdown. The templates render it
through Hugo's `.RenderString` so backticks become
`<code>` and `[text](url)` becomes `<a>`.

Two checks pin this contract. One walks the template
source. The other walks the rendered HTML.

### Template-source check

A Go test in [`template_summary_test.go`][tpl-test]
walks `website/layouts/**/*.html`. Each template action
that references `.Params.summary` must take one of three
forms. The forms are the predicate
`{{ if .Params.summary }}`, the negated predicate
`{{ if not .Params.summary }}`, or any action that calls
`.RenderString`.

Any other shape fails the test. The bug this guards
against is `{{ with .Params.summary }}...{{ . }}{{ end }}`.
The `with` rebinds the dot to the summary string. The
inner `{{ . }}` then emits the value raw. A value with
backticks ships as literal backticks instead of `<code>`
tags. The bare `{{ .Params.summary }}` form carries the
same defect without the rebinding.

`baseof.html` is exempt. Its meta-description fallback
emits plain text on purpose; the `<meta>` tag does not
accept HTML.

The test scans each file as one string, not line by
line. A multi-line action that spans newlines is still
caught.

### Rendered-HTML probe

`mdsmith-release verify-website-links` runs after
`hugo --minify`. One probe in [`verifylinks.go`][probe]
searches the output tree for a lead paragraph whose
body contains a `<code>` tag. The `<code>` must sit
inside the same paragraph as the lead, not in a sibling.
At least one rendered page must match. Otherwise a
template regressed to raw output, and the build fails.

The regex tolerates extra `<p>` attributes, additional
classes alongside `lead`, and inline tags before
`<code>`. It forbids `</p>` between the opening tag and
`<code>`. Code in a sibling paragraph does not satisfy
the probe.

[tpl-test]: ../../internal/release/template_summary_test.go
[probe]: ../../internal/release/verifylinks.go
