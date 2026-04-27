---
id: 99
title: Screenshot directive and capture command
status: "🔲"
summary: >-
  Add a `<?screenshot?>...<?/screenshot?>` generated
  section that keeps a Markdown image link in sync
  with its parameters (URL, selector, output path,
  alt text), plus a separate `mdsmith screenshot`
  command that drives a headless browser to
  regenerate the image bytes. Splits the
  always-cheap linting layer from the
  occasionally-expensive capture step so
  `mdsmith fix` does not need a dev server.
---
# Screenshot directive and capture command

## Goal

Make documentation screenshots a build artifact
of the source they describe, the same way
[`<?catalog?>`][catalog] makes the file index a
build artifact of the directory it indexes.
Authors declare what to capture. Rebuilding
refreshes the image so docs do not silently
diverge from the UI.

## Context

[interblah.net/self-updating-screenshots][post]
describes a Rake task that scans Markdown
help-centre articles for
`<!-- SCREENSHOT: ... -->` comments. It runs
Capybara plus Cuprite against the Rails app
and writes PNGs to disk. The author reports
the biggest win is killing drift: once capture
is automatic, screenshots stop diverging from
the UI.

mdsmith already ships the moving parts. The
[generated-section archetype][gensection]
gives us markers and body regeneration. The
multi-pass `fix` engine handles fixable rules.
Two pieces are missing: a directive whose body
is a Markdown image link, and a way to drive
a browser without folding it into
`mdsmith fix`.

[post]: https://interblah.net/self-updating-screenshots
[catalog]: ../internal/rules/MDS019-catalog/README.md
[gensection]: ../docs/background/archetypes/generated-section/README.md

### Why split linting from capture

Existing directives generate text from cheap
inputs on disk. Capture needs a running
target, a browser, network, and time. Folding
it into `mdsmith fix` would either make the
linter slow and stateful, or silently no-op
when the browser is unavailable.

The split:

- **Lint layer (always on).** Rule MDS039
  keeps `![alt](output)` in sync with the
  directive params. `mdsmith fix` rewrites
  it. No network, no browser.
- **Capture layer (opt-in command).** A new
  `mdsmith screenshot` subcommand walks
  files, finds `<?screenshot?>` directives,
  drives a headless browser, and writes the
  image bytes to `output`. Never a side
  effect of `check`/`fix`.

## Design

### Directive syntax

```text
<?screenshot
url: /inbox
selector: "#inbox-brand-new-section"
output: screenshots/inbox-brand-new.png
alt: The Brand New section of the inbox
?>
![The Brand New section of the inbox](screenshots/inbox-brand-new.png)
<?/screenshot?>
```

Parameters:

| Name       | Type           | Required | Default    | Description                                                               |
|------------|----------------|----------|------------|---------------------------------------------------------------------------|
| `url`      | string         | yes      | —          | Target URL. Absolute or path-only (joined with `--base-url` at capture)   |
| `output`   | string         | yes      | —          | Image path relative to the Markdown file. Must be inside the project root |
| `alt`      | string         | no       | derived    | Alt text. Defaults to a slug of the file's H1 plus selector, see below    |
| `selector` | string         | no       | full page  | CSS selector of the element to capture                                    |
| `viewport` | `WIDTHxHEIGHT` | no       | `1280x800` | Browser viewport size                                                     |
| `wait`     | int (ms)       | no       | `0`        | Delay after navigation/click before capture                               |
| `click`    | string         | no       | —          | CSS selector to click before capture                                      |
| `hide`     | list of string | no       | `[]`       | CSS selectors hidden via `display: none` for the capture                  |

Out of scope here: `crop`, `torn`, auth,
scripted multi-step interactions. The
`Capturer` interface (below) absorbs them
later without re-shaping the directive
surface.

### Generated body (lint layer)

The body is exactly one line:

```text
![{alt}]({output})
```

`{alt}` and `{output}` come from the params.
MDS039's `Check` compares this against the
actual body and reports a stale-section
diagnostic. `Fix` rewrites the body. Reuses
[`internal/archetype/gensection`][gensection]
unchanged — same plumbing as `catalog`,
`include`, and `toc`.

When `alt` is omitted, derive it as
`Screenshot of {selector or "page"} at {url}`
so the body always passes MDS032
(no-empty-alt-text). The validator emits a
warning suggesting an explicit `alt:`.

### Rule: MDS039 (screenshot)

- ID: `MDS039`
- Name: `screenshot`
- Category: `meta`
- Default: enabled
- Fixable: yes (lint layer only)

Validation:

- `url` and `output` required
- `output` is a relative path inside the
  project root, no `..`, extension in
  `{.png, .jpg, .jpeg, .webp}`
- `viewport` matches `^\d+x\d+$`
- `wait` is a non-negative integer
- `selector` and `click`, when present, are
  non-empty strings (no AST-level CSS check)

Lives in `internal/rules/screenshot/`.
Fixtures under
`internal/rules/MDS039-screenshot/` with
`good/`, `bad/`, and `fixed/` cases mirroring
plan 89.

### `mdsmith screenshot` subcommand

```text
mdsmith screenshot [paths...] [flags]
```

Flags:

- `--base-url URL` — prefix joined to relative
  `url` params. Required when any directive
  uses a path-only URL.
- `--dry-run` — print captures; do not launch
  a browser or write files.
- `--browser PATH` — override the chromium
  binary (defaults to autodetect).
- `--timeout DURATION` — per-screenshot
  timeout; default `30s`.

Behaviour:

1. Discover Markdown files via the same
   walker `check`/`fix` use (respects
   `.mdsmith.yml` ignores).
2. Parse with the existing AST and directive
   parser, collect every `<?screenshot?>`
   block. Validate via MDS039; on validation
   error, abort that file.
3. Group by `url` host so a single browser
   context handles all shots for a target,
   in declaration order.
4. For each: navigate, optionally click and
   wait, optionally hide selectors, capture
   element-or-page, write atomically to
   `output`.
5. Exit non-zero if any capture failed; print
   a per-file `OK | FAIL` summary.

Capture backend: pure-Go [`chromedp`][chromedp].
No Node toolchain. Wrapped behind a
`Capturer` interface in `internal/screenshot/`
so tests can swap in a fake that writes a 1×1
PNG. Future backends (Playwright, remote
service) plug in without touching the rule or
command.

[chromedp]: https://github.com/chromedp/chromedp

### Interaction with existing rules

- **MDS032 (no-empty-alt-text)**: derived alt
  text is non-empty.
- **MDS012 (no-bare-urls)**: image link is
  formatted, not bare.
- **MDS027 (cross-file-reference-integrity)**:
  the generated `![...](path)` participates in
  the existing image-target check. A missing
  image fires MDS027 and points the user at
  `mdsmith screenshot`. No special-case
  needed.
- **MDS019/MDS021/MDS038**: orthogonal
  directives, different names.
- **`merge-driver`**: regenerates the body on
  conflict via the shared gensection engine.
  Image bytes are not regenerated by the
  merge driver.

### Configuration

A new top-level `screenshot:` block holds
defaults (`base-url`, `viewport`, `wait`)
that would otherwise repeat on every
directive. Per-directive params override
config; `--base-url` overrides both. Reuses
the deep-merge rules from plan 97.

### Out of scope

Animations or video, authenticated sessions,
image diffing, git LFS / CDN storage, and a
`--watch` mode.

## Tasks

1. Add a `Capturer` interface and `chromedp`
   implementation in `internal/screenshot/`.
   Unit-test against `httptest.Server`. Skip
   when no chromium binary resolves.
2. Create the `<?screenshot?>` directive in
   `internal/rules/screenshot/` using
   `gensection.Engine`. Register as MDS039,
   category `meta`, `FixableRule`. `Generate`
   only renders the image line; never calls
   the capturer.
3. Add MDS039 fixtures under
   `internal/rules/MDS039-screenshot/`:
   `good/`, `bad/`, `fixed/`. Mirror plan 89.
4. Wire the rule into
   [`cmd/mdsmith/main.go`][main].
5. Add `mdsmith screenshot` subcommand.
   Reuse the file walker and AST parser.
   Iterate captures sequentially per host.
   Wire the flags listed above.
6. Add the `screenshot:` config block in
   `internal/config/`. Surface defaults to
   MDS039 and to the capture command.
7. Integration test in
   `internal/integration/`: a
   `<?screenshot?>` whose `url` points at an
   `httptest.Server` produces a non-empty
   PNG with stable MD5 across runs. Skipped
   without chromium.
8. Document MDS039 README and a guide
   `docs/guides/directives/screenshots.md`.
   Update the gensection archetype doc to
   list `screenshot`.
9. Add a demo using a static HTML file in
   the repo so it runs without a dev server.

[main]: ../cmd/mdsmith/main.go

## Acceptance Criteria

- [ ] `<?screenshot url:... output:... ?>`
      regenerates its body to `![alt](output)`
      on `mdsmith fix`
- [ ] MDS039 reports a stale-section
      diagnostic on `check` when the body
      diverges from the params
- [ ] MDS039 rejects missing `url`, missing
      `output`, paths with `..`, and
      non-image extensions
- [ ] `mdsmith screenshot` against an
      `httptest.Server` writes a non-empty
      PNG to the path declared in `output`
- [ ] `mdsmith screenshot --dry-run` lists
      every capture without launching a
      browser or writing files
- [ ] `mdsmith screenshot` exits non-zero
      when any capture fails and prints a
      per-file summary
- [ ] `mdsmith check` does **not** launch a
      browser or attempt network access for
      `<?screenshot?>` blocks
- [ ] `screenshot:` config defaults apply to
      directives that omit them and are
      overridden by per-directive params
- [ ] CI without a chromium binary still
      passes; capture-dependent tests are
      skipped, not failed
- [ ] Merge driver regenerates
      `<?screenshot?>` bodies on conflict,
      same as `<?catalog?>` (inherited from
      the shared gensection engine)
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports
      no issues
