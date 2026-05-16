---
title: "Cross-file integrity"
summary: >-
  Built-in rules flag broken links and missing anchors, enforce
  per-file section schemas, and keep Markdown in the right folders.
  Schemas can be inline on a file kind or shared via `proto.md` files.
icon: link
link: "/docs/guides/directives/enforcing-structure/"
rules: ["MDS027", "MDS020", "MDS033"]
weight: 3
---
# Cross-file integrity

A linter that only sees one file at a time cannot catch a link
that points at a heading another file just renamed. mdsmith
resolves links and anchors across the whole workspace.

[`MDS027`](../../internal/rules/MDS027-cross-file-reference-integrity/README.md)
flags broken links and missing anchors.
[`MDS020`](../../internal/rules/MDS020-required-structure/README.md)
enforces a per-file section schema: required headings, front-matter
fields, and ordering.
[`MDS033`](../../internal/rules/MDS033-directory-structure/README.md)
keeps each Markdown file in an allowed folder.

A schema can be declared inline on a [file
kind](../guides/file-kinds.md) or shared across files via a
`proto.md` template, so a whole directory validates against one
source of truth.
