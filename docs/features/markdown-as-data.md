---
title: "Markdown as a data source"
summary: >-
  `mdsmith extract` projects a schema-conformant Markdown file into a
  JSON, YAML, or msgpack data tree; `mdsmith export` writes a portable,
  directive-free copy that renders anywhere.
icon: braces
link: "/guides/extract-markdown-as-data/"
weight: 14
group: "Markdown as a single source of truth"
---
# Markdown as a data source

A schema-conformant Markdown file is already structured data: the
front matter is a map, and the headings beneath it form a tree.
`mdsmith extract` reads that structure with no extra
annotation, because the [kind schema](file-kinds-schemas.md) is
the extraction contract.

`mdsmith extract <kind> --format json|yaml|msgpack <file>` walks
the composed schema alongside the file and emits a tree.
Front matter goes under a `frontmatter` key, each heading becomes
an object keyed by its slug, and repeating sections become arrays.
A non-conformant file prints the same diagnostics as `mdsmith
check` and exits non-zero, never emitting partial data.

The read side lives on the `<?include?>` directive. Its
`extract:` parameter walks the same tree and splices one leaf into
another file's body. A README can quote a value from its source
of truth, and `mdsmith fix` keeps the copy current. No intermediate
fragment file is left to maintain.

For plain Markdown instead of data, `mdsmith export` writes a
portable, directive-free copy. It strips generated-section
markers, inlines `<?include?>` content recursively, and leaves a
file that renders on any Markdown tool with no mdsmith knowledge.
By default it refuses to export a stale directive body; `--fix`
regenerates first.

See the
[Extract Markdown as data guide](../guides/extract-markdown-as-data.md)
for when a value belongs in front matter versus a body section.
The [`mdsmith extract`](../reference/cli/extract.md) and
[`mdsmith export`](../reference/cli/export.md) references cover
flags, formats, and exit codes.
