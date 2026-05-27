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
The field holds inline Markdown. Templates render it
through Hugo's `.RenderString` so backticks become
`<code>` and `[text](url)` becomes `<a>`.

A Go test in [`template_summary_test.go`][tpl-test]
walks `website/layouts/**/*.html`. The scanner parses
each template with Go's `text/template/parse` package
(in `SkipFuncCheck` mode so undefined Hugo helpers
do not error) and walks the AST. Every reference to
`.Params.summary` is classified by its node context.
No regex tokenizing, no exemption list — comments,
string literals, and CRLF line endings are handled
by the parser.

Safe forms:

- A presence predicate — `{{ if .Params.summary }}`,
  the negated form, compound shapes
  (`{{ if and .Params.summary .X }}`,
  `{{ if or .Params.summary .Other }}`), the `else if`
  variant, subfield access
  (`{{ if .Params.summary.HTML }}`), or any other
  comparison that does not produce output.
- A `.RenderString` call with the summary as a
  positional argument:
  `{{ .RenderString (dict "display" "inline") .Params.summary }}`.
  Qualified receivers (`.Page.RenderString`,
  `$.RenderString`) are recognised.
- A pipeline that passes the summary through
  `.RenderString` and then any number of post-render
  filters: `{{ .Params.summary | .RenderString }}`,
  `{{ .Params.summary | strings.TrimSpace | .RenderString }}`,
  `{{ $.RenderString (dict "display" "inline") .Params.summary | plainify }}`.
  Once the value has rendered, downstream stages such
  as `plainify` or `safeHTML` are fine.
- A sub-pipeline argument whose output feeds
  `.RenderString`:
  `{{ .RenderString (dict) (printf "wrapper: %s" .Params.summary) }}`.

Forbidden forms:

- `{{ with .Params.summary }}` and
  `{{ else with .Params.summary }}` — these rebind
  `.` to the summary string and the body typically
  emits the value raw.
- `{{ range .Params.summary }}` — ranging over a
  string iterates rune-by-rune and emits each code
  point as an integer.
- The bare `{{ .Params.summary }}` action.
- Variable assignment in any context — `{{ $s := .Params.summary }}`,
  `{{ if $s := .Params.summary }}`,
  `{{ range $i, $v := .Params.summary }}`. The bound
  name escapes the per-action check.
- `.Params.summary` referenced in a value-emitting
  action whose pipe does not reach `.RenderString`:
  `{{ printf "%s" .Params.summary }}`,
  `{{ .Params.summary | print "x" .Page.RenderString }}`
  (the second example references `.RenderString` as
  a value passed to `print`, not as a method call).

`baseof.html` reuses the same projection. Each
branch of its meta-description chain runs the
source value through `$.RenderString` then
`plainify`. The sources are `.Description`,
`.Params.summary`, `.Params.description`, and
`.Site.Params.description`. Meta content cannot
carry HTML. Backticks become `<code>` then plain
text. SEO snippets see clean prose.

The rule applies only to `.Params.summary` today. If
other front-matter scalars (e.g. `lead`, `eyebrow`,
`tagline`) gain inline-Markdown content in the future,
extend the AST classifier in
[`template_summary_test.go`][tpl-test] to accept the
new field name set.

[tpl-test]: ../../internal/release/template_summary_test.go
