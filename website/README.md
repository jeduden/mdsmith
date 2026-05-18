---
summary: mdsmith.dev — the docs-and-marketing site, built with Hugo from the repository's existing Markdown.
---
# mdsmith.dev

The single product surface that complements the `mdsmith` CLI:
homepage, docs, rules catalog, comparison page. Source content
is the repository itself — `README.md` and `docs/**/*.md` — so
the website cannot drift out of sync with the binary.

## Layout

| Path                           | Purpose                                                                     |
|--------------------------------|-----------------------------------------------------------------------------|
| `hugo.toml`                    | Site config + module mounts (does NOT mount `../docs` directly).            |
| `content/_index.md`            | Homepage front matter and copy.                                             |
| `content/docs/`                | **Synced** from `../docs/` by `mdsmith-release build-website` (gitignored). |
| `layouts/_default/baseof.html` | Page shell — `<head>`, top nav, footer.                                     |
| `layouts/index.html`           | Homepage template (hero · feature grid · install tabs · terminal).          |
| `layouts/_default/single.html` | Docs page template (sidebar + prose).                                       |
| `layouts/_default/list.html`   | Section index template.                                                     |
| `layouts/partials/`            | `topnav`, `footer`, `hero`, `feature-grid`, etc.                            |
| `layouts/shortcodes/`          | `callout`, `diag`, `pill`, `chip`, `install-cmd`.                           |
| `layouts/_default/_markup/`    | Goldmark render hooks (headings, code blocks).                              |
| `static/css/`                  | `colors_and_type.css` (tokens) + `app.css` (component styles).              |
| `static/fonts/`                | Self-hosted WOFF2: 0xProto (mono) + IBM Plex Sans/Serif.                    |
| `static/img/`                  | Logo SVGs.                                                                  |

## Develop

Hugo (non-extended is fine — no SCSS or image processing):

```bash
# install hugo — pin matches HUGO_VERSION in the pages-deploy
# workflow so local builds cannot drift from CI.
go install github.com/gohugoio/hugo@v0.161.1

# sync ../docs/ -> content/docs/ (mdsmith fix + snapshot).
# Run from the repo root; content/docs/ is .gitignore'd.
go run ./cmd/mdsmith-release build-website

# build into public/
cd website && hugo --minify

# or serve with live reload (re-run build-website after
# editing docs/; pass --no-fix to skip the mdsmith fix pass)
hugo server -D
```

## Source layout

`docs/**/*.md` is the single source of truth. The website
never owns docs content. `mdsmith-release build-website`
runs `mdsmith fix` over `../docs/` (skippable with
`--no-fix`) and then snapshots it into `content/docs/` via
the same `sync-docs` logic. The release workflow calls
`build-website --no-fix` on every tag push.

The subcommand snapshots `docs/` into `content/docs/`.
It then adapts the tree for Hugo. The synced output
differs from the source docs. These transforms cause
the difference:

- Drop `proto.md` schema templates. Their front matter
  holds CUE expressions, not real values.
- Rename `index.md` to `_index.md` (Hugo's section-page
  convention).
- Prune non-markdown / non-image files.
- Escape literal Hugo shortcode patterns (`{{< … >}}`
  and `{{% … %}}`) so docs about Hugo render as text.
- Promote the first body H1 to front-matter `title:`
  and remove it from the body. Hugo themes render the
  title from front matter, so a body H1 would double it.
- Strip mdsmith `<?…?>` directive markers. They are
  source syntax with no meaning to Hugo. The same syntax
  inside code fences or inline code is kept, so
  directive documentation still renders.

Pass `--no-fix` to skip the `mdsmith fix` step during
local preview.

After the docs snapshot, `build-website` also publishes
the rule directory. `../internal/rules/index.md` is a
sibling of `docs/`, not inside it, so `sync-docs` never
sees it. The build copies it to
`content/docs/rules/_index.md` with the same H1-lift and
directive-strip transforms. It then rewrites each
`MDSxxx-name/README.md` link to its absolute GitHub URL.
The per-rule READMEs are not snapshotted into the Hugo
tree, so a relative link would 404. The result is a
browsable **Rules** section in the top nav and docs
sidebar.

## Homepage content sources

The homepage has **no** Hugo data file. Every block is
sourced from Markdown so a copy edit lands in one place:

- **Hero + install quickstart** — front matter (`hero:`,
  `install:`) on the homepage itself,
  `content/_index.md`, read by `hero.html` and
  `install-list.html`.
- **"Distributed via" strip** — the release-channel docs
  `../docs/development/release-channels/*.md`. Each
  carries a canonical `channelurl:` and an ordering
  `weight:`; `logos-strip.html` ranges the synced
  section. (`channelurl`, not `url`: Hugo treats a
  front-matter `url` as a page-URL override.)
- **Feature cards** — the shared Markdown described
  below.

## Shared feature copy

Feature copy lives once, as Markdown, in
`../docs/features/`:

- `index.md` is the shared "Why mdsmith" overview. The
  repository `README.md` splices it in with `<?include?>`,
  and the website renders the same file as the
  `/features/` landing page — so the README and the
  site cannot drift.
- One page per feature (`auto-fix.md`, `performance.md`,
  `quality.md`, …) carries a short `summary:` plus an
  `icon:`, `weight:`, optional `rules:`, and a fuller
  body. `feature-grid.html` builds the homepage cards
  from these pages; each card links to the full page,
  which has room for the longer write-up the README
  cannot fit.

Requirement: every feature (or feature category) has a
page here, including non-CLI capabilities such as
performance and the quality badges. The README and the
website must reuse the same Markdown; the website may
present more, never different, content.

## Deploy

`pages-deploy` in `.github/workflows/release.yml` builds
`website/` and publishes it to GitHub Pages on every `v*`
tag push.

A push to `main` also deploys, via the path filter in
`.github/workflows/pages.yml`. That filter watches
`docs/**`, `website/**`, the workflow itself, and
`internal/rules/index.md`. The rule index is on the
list because `build-website` publishes it as the
`/rules/` section. Editing a rule README and
running `mdsmith fix` regenerates the tracked
`internal/rules/index.md` catalog. That regenerated
file is the change that triggers the deploy.

The job installs Hugo via `go install` (sumdb verifies
the binary). It runs `mdsmith-release build-website
--no-fix`, then `hugo --minify`. It hands the output to
the `actions/upload-pages-artifact` and
`actions/deploy-pages` pair.

Set the Pages source to **GitHub Actions** in repository
settings.

## Design system origin

The CSS in `static/css/` is the Claude Design export for the
project. See:

- `static/css/colors_and_type.css` — tokens, base typography
- `static/css/app.css` — component styles (class-based, no
  React or Tailwind required)
- `static/img/logo-*.svg` — hammer mark + `md`/`smith` wordmark
  (lockup and inverse variants)

The CSS classes (`.topnav`, `.hero`, `.feature-grid`,
`.install`, `.term`, `.diag`, `.docs-grid`, `.chip`, `.pill`,
`.pg`, `.tbl`, `.footer`, …) drive the partials and shortcodes
in `layouts/`. To restyle, edit the CSS; markup stays put.
