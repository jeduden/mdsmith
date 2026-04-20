---
id: 69
title: 'Include enhancements: link adjustment and heading-level'
status: "✅"
---
# Include enhancements

## Goal

Add two features to the include directive (MDS021):
automatic link-path rewriting and a `heading-level`
parameter for heading-level adjustment.

## Background

Relative links break when a file is included into a
document in a different directory. For example,
`docs/guide.md` includes `DEVELOPMENT.md` (a copy or
symlink placed alongside the including file, since
include `file:` paths may not contain `..` traversal
segments). A link like `[layout](internal/rules/)` in
DEVELOPMENT.md resolves from the repo root, but when
included from `docs/guide.md` it points to
`docs/internal/rules/` instead. The include directive
must rewrite each relative link target so it resolves
from the including file.

Heading levels also need adjustment. DEVELOPMENT.md
uses `##` headings. When included under `## Project`
in CLAUDE.md those headings appear as siblings, not
children. The `heading-level` parameter (set to `"absolute"`)
shifts included headings to nest under the parent.

## Design

### Link adjustment (always, automatic)

After reading the file and stripping frontmatter,
scan every Markdown link and image. A link looks like
`[text](target)` and an image like `![alt](target)`.
For each relative target (not `/`, `#`, `http://`,
or `https://`):

1. Get the included file's directory relative to the
   FS root (e.g. `DEVELOPMENT.md` → `.`).
2. Get the including file's directory relative to the
   FS root (e.g. `docs/guide.md` → `docs`).
3. Rewrite the target:
   `newTarget = relpath(includingDir,
   join(includedDir, target))`.

Skip the transformation when both files share the
same directory.

### `heading-level` parameter

New optional parameter `heading-level` (values:
`"absolute"` or omitted).

When `heading-level: "absolute"`:

1. Find the heading level of the section that contains
   the `<?include?>` marker (the "parent level"). Use
   0 when the marker sits at the document root.
2. Find the minimum heading level in the included
   content (the "source top level").
3. Compute `shift = parentLevel - sourceTopLevel + 1`
   so included top-level headings become children of
   the parent. Skip when shift is zero.
4. Add `shift` to every ATX heading (`#` prefix) and
   setext heading (underline). Cap at level 6.

Example: include under `## Project` (level 2), source
has `## Build` (level 2) and `### Sub` (level 3).
`shift = 2 - 2 + 1 = 1`. Result: `### Build` (3),
`#### Sub` (4).

## Tasks

1. [x] Add a helper `adjustLinks(content,
   includedFilePath, includingFilePath)` in
   [`internal/rules/include/`](../internal/rules/include/)
   that rewrites relative link/image targets
2. [x] Write unit tests for `adjustLinks`: same directory
   (no-op), different directories, anchors and
   absolute URLs left untouched, query strings
   preserved
3. [x] Call `adjustLinks` in `generateIncludeContent`
   after frontmatter stripping, before wrap
4. [x] Add a helper `adjustHeadings(content, parentLevel)`
   that shifts ATX and setext heading levels
5. [x] Write unit tests for `adjustHeadings`: shift up,
   shift down, cap at 6, no headings (no-op)
6. [x] Extend `validateIncludeDirective` to accept and
   validate the `heading-level` parameter (only
   `"absolute"` is valid)
7. [x] In `generateIncludeContent`, detect the parent
   heading level from the marker position and call
   `adjustHeadings` when `heading-level: "absolute"`
8. [x] Add test for parent-level detection (marker under
   h2, under h3, at document root)
9. [x] Update the rule README at
   [`MDS021-include/README.md`](../internal/rules/MDS021-include/README.md)
   to document both features
10. [x] Update existing fixtures and tests if link
    adjustment changes their expected output
11. [x] Run `go test ./...`, `go tool golangci-lint run`,
    and `mdsmith check .`

## Acceptance Criteria

- [x] Relative links in included content are rewritten
      so they resolve from the including file's
      directory, not the source file's directory
- [x] Absolute URLs, anchor-only links (`#foo`), and
      protocol links (`http://`, `https://`) are not
      modified
- [x] `heading-level: "absolute"` shifts headings so
      the included top-level headings appear one level
      below the enclosing section
- [x] When `heading-level` is omitted, heading levels
      stay unchanged
- [x] Heading level never exceeds 6
- [x] Invalid `heading-level` values produce a diagnostic
- [x] Link adjustment is always applied (no parameter
      needed)
- [x] All tests pass: `go test ./...`
- [x] `golangci-lint run` reports no issues
- [x] `mdsmith check .` reports zero diagnostics
