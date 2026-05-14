---
summary: mdsmith.dev — the docs-and-marketing site, built with Hugo from the repository's existing Markdown.
---
# mdsmith.dev

The single product surface that complements the `mdsmith` CLI:
homepage, docs, rules catalog, comparison page. Source content
is the repository itself — `README.md` and `docs/**/*.md` — so
the website cannot drift out of sync with the binary.

## Layout

| Path                           | Purpose                                                            |
|--------------------------------|--------------------------------------------------------------------|
| `hugo.toml`                    | Site config + module mounts (does NOT mount `../docs` directly).   |
| `content/_index.md`            | Homepage front matter and copy.                                    |
| `content/compare.md`           | "vs other linters" page.                                           |
| `content/docs/`                | **Synced** from `../docs/` by `scripts/sync-docs.sh` (gitignored). |
| `layouts/_default/baseof.html` | Page shell — `<head>`, top nav, footer.                            |
| `layouts/index.html`           | Homepage template (hero · feature grid · install tabs · terminal). |
| `layouts/_default/single.html` | Docs page template (sidebar + prose).                              |
| `layouts/_default/list.html`   | Section index template.                                            |
| `layouts/partials/`            | `topnav`, `footer`, `hero`, `feature-grid`, etc.                   |
| `layouts/shortcodes/`          | `callout`, `diag`, `pill`, `chip`, `install-cmd`.                  |
| `layouts/_default/_markup/`    | Goldmark render hooks (headings, code blocks).                     |
| `static/css/`                  | `colors_and_type.css` (tokens) + `app.css` (component styles).     |
| `static/fonts/`                | 0xProto Nerd Font TTF.                                             |
| `static/img/`                  | Logo SVGs.                                                         |
| `data/`                        | Reserved for generated data (rules catalog, etc.).                 |

## Develop

Hugo (non-extended is fine — no SCSS or image processing):

```bash
# install hugo — pin matches HUGO_VERSION in the pages-deploy
# workflow so local builds cannot drift from CI.
go install github.com/gohugoio/hugo@v0.161.1

# sync ../docs/ -> content/docs/ (mdsmith fix + snapshot
# via the mdsmith-release sync-docs subcommand).
# content/docs/ is .gitignore'd.
cd website && ./scripts/sync-docs.sh

# build into public/
hugo --minify

# or serve with live reload (re-run sync-docs after editing docs/)
hugo server -D
```

## Source layout

`docs/**/*.md` is the single source of truth. The website
never owns docs content. `scripts/sync-docs.sh` is a thin
wrapper that runs `mdsmith fix` over `../docs/` and then
delegates to `go run ./cmd/mdsmith-release sync-docs`. The
release workflow calls the same subcommand on every tag
push.

The subcommand applies the four transforms Hugo needs.
First, it snapshots `docs/` into `content/docs/`. Next,
it drops `proto.md` schema templates whose front matter
holds CUE expressions rather than real values. Then it
renames `index.md` to `_index.md` (Hugo's section-page
convention). Finally, it prunes non-markdown / non-image
files and escapes literal Hugo shortcode patterns
(`{{< … >}}` and `{{% … %}}`) so documentation about Hugo
renders as text instead of being parsed.

Pass `--no-fix` to skip the `mdsmith fix` step during
local preview.

## Deploy

`pages-deploy` in `.github/workflows/release.yml` builds
`website/` and publishes it to GitHub Pages on every `v*`
tag push.

The job installs Hugo via `go install` (sumdb verifies
the binary). It runs `mdsmith-release sync-docs`, then
`hugo --minify`. It hands the output to the
`actions/upload-pages-artifact` and `actions/deploy-pages`
pair.

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
