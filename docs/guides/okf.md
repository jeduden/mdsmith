---
title: Open Knowledge Format (OKF) bundles
weight: 82
summary: >-
  Author and validate Open Knowledge Format bundles with mdsmith:
  scaffold the config with `mdsmith init --starter okf` to require a
  non-empty `type` on every concept document, validate bundle-relative
  cross-links with `links.site-root`, and generate the `index.md`
  listing with `<?catalog?>`.
---
# Open Knowledge Format (OKF) bundles

The [Open Knowledge Format][okf] (OKF) is a vendor-neutral
specification for representing the knowledge an AI agent needs as a
plain directory of Markdown files with YAML front matter. Each file is
one concept — a table, a dataset, a metric, a playbook, an API. The
format is "just files": no SDK, no runtime, no registry.

mdsmith is a natural authoring and validation tool for OKF, because the
two share a model — Markdown plus front matter, cross-linked files, and
a progressive-disclosure `index.md`. This guide shows how to keep a
bundle conformant as you edit it: require the one field OKF mandates,
catch broken cross-links, and generate the index from front matter.

## What an OKF bundle looks like

A bundle is a directory tree. Every `.md` file except the two reserved
names — `index.md` and `log.md` — is a concept document with a
non-empty `type` in its front matter.

```text
sales/
├── index.md          # reserved: a listing for progressive disclosure
├── log.md            # reserved: a chronological change history
├── datasets/
│   └── sales.md      # concept (type: BigQuery Dataset)
└── tables/
    ├── orders.md     # concept (type: BigQuery Table)
    └── customers.md  # concept (type: BigQuery Table)
```

A concept document carries a small block of front matter for the
structured fields OKF names — `type` (required), and the recommended
`title`, `description`, `resource`, `tags`, and `timestamp` — and a
Markdown body for everything else:

```markdown
---
type: BigQuery Table
title: Orders
description: One row per completed customer order.
resource: https://console.cloud.google.com/bigquery?d=sales&t=orders
tags: [sales, orders]
timestamp: 2026-05-28T00:00:00Z
---
# Schema

| Column     | Type   | Description                              |
| ---------- | ------ | ---------------------------------------- |
| order_id   | STRING | Unique order identifier.                 |
| total_usd  | NUMERIC | Order total in USD.                     |

Part of the [sales dataset](/datasets/sales.md).
```

## Scaffold the config

Run this once at the bundle root:

```bash
mdsmith init --starter okf
```

It writes a ready-to-edit `.mdsmith.yml` — a plain mdsmith config, no
special OKF runtime — that does two things.

First, it requires a non-empty `type` on every concept document:

```yaml
rules:
  required-frontmatter:
    fields: [type]
    exclude: [index.md, log.md]
```

The check is [MDS071 `required-frontmatter`][mds071], scoped to skip
the reserved files. A file with no `type`, an empty `type`, or no
front matter at all fails `mdsmith check`:

```text
sales/tables/orders.md:1:1 MDS071 front-matter "type" is required but missing
```

Second, it turns off the prose- and size-opinion rules that suit a
documentation site but not a data bundle. An OKF concept body may open
with prose, so the "first line must be a heading" rule steps aside.
Long lines, large files, dense tables, and tight token budgets are all
fine in a knowledge bundle, so those checks stand down too:

```yaml
rules:
  first-line-heading: false
  line-length: false
  max-file-length: false
  token-budget: false
  paragraph-readability: false
  paragraph-structure: false
  table-readability: false
```

Mechanical hygiene — trailing whitespace, code fences, blank lines,
link integrity — stays on. The starter pins no Markdown flavor, so GFM
tables in concept bodies are never flagged. Everything past this point
is the rest of what the starter writes (or what you would add by hand);
edit any line to fit your bundle. If you want a stricter `type`
vocabulary — say, an enum of allowed types — layer a
[kind](#tighten-per-type-structure-with-a-kind) on top, as shown below.

## Validate cross-links

OKF concept documents link to each other with ordinary Markdown links.
The spec supports two forms, and mdsmith handles each:

- **Relative links** (`[orders](../tables/orders.md)`) are resolved
  against the linking file and validated out of the box. A typo or a
  moved target fails `mdsmith check`.
- **Bundle-relative links** (`[orders](/tables/orders.md)`), which OKF
  recommends for stability, begin with `/` and are read from the
  bundle root. mdsmith treats a leading-`/` link as absolute and, by
  default, skips it — so it is never a false positive, but it is also
  not checked.

To validate bundle-relative links, point mdsmith at the bundle root
with `links.site-root`. The starter already sets this — `"."` is the
directory you run `mdsmith check` from:

```yaml
rules:
  cross-file-reference-integrity:
    links:
      site-root: "."
```

With `site-root` set, `[orders](/tables/orders.md)` resolves to
`tables/orders.md` under the root and a missing target is reported.
Anchor checks are skipped for bundle-relative links, so use them for
whole-file references and relative links when you target a heading.

## Generate `index.md` with a catalog

An OKF `index.md` is a bullet list that lets an agent scan a directory
before opening any single file — the same progressive-disclosure idea
covered in the
[progressive disclosure guide](progressive-disclosure.md). The OKF
shape is one bullet per concept, `[Title](link) - description`, with
the description taken from each concept's front matter.

A [`<?catalog?>` directive](directives/generating-content.md) generates
exactly that. Put it in each directory's `index.md` and let
`mdsmith fix` keep it in sync:

```markdown
<?catalog
glob:
  - "*.md"
  - "!index.md"
  - "!log.md"
sort: title
row: "* [{title}]({filename}) - {description}"
?>
<?/catalog?>
```

On `mdsmith fix`, the directive reads `title` and `description` from
every concept's front matter and rewrites the bullet list between the
markers; the reserved files are excluded by the `!` globs. The bundle
root's `index.md` is also the one place OKF allows front matter — an
`okf_version: "0.1"` line declaring the target spec version — and the
starter's `required-frontmatter` excludes `index.md`, so it needs no
`type` there.

The directive markers are mdsmith-specific. To hand a consumer a
pristine, directive-free copy of the bundle, run
[`mdsmith export`](../reference/cli/export.md), which writes the
generated bodies without the surrounding markers.

## Track changes in `log.md`

OKF's other reserved file, `log.md`, records a bundle's history with
date-grouped entries, newest first, under `YYYY-MM-DD` headings:

```markdown
# Update log

## 2026-05-22

* **Update**: Added the [orders table](/tables/orders.md) reference.
```

mdsmith does not generate `log.md` — it is authored by hand or by your
producer — but the starter excludes it from the `type` requirement and
lints it like any other Markdown file, so its headings, lists, and
links stay well formed.

## Tighten per-type structure with a kind

The starter enforces OKF's single hard requirement. To go further —
make the recommended fields mandatory for your project, pin `type` to
an enum, or require a heading structure per concept type — declare a
[kind with a schema](file-kinds.md). A kind matches files by a path
pattern and validates their front matter and headings. The starter
ships this block commented out; uncomment and adapt it:

```yaml
kinds:
  bq-table:
    path-pattern: "tables/*.md"
    schema:
      frontmatter:
        type: '"BigQuery Table"'
        title: nonEmpty
        description: nonEmpty
        timestamp: string
      sections:
        - heading: null
        - heading: { regex: '^Schema$' }
kind-assignment:
  - glob: ["tables/*.md"]
    kinds: [bq-table]
```

Now a `tables/*.md` file whose `type` is not exactly `BigQuery Table`,
or that omits `title`, or that lacks a `Schema` section, fails
`mdsmith check` with a [schema](schemas.md) diagnostic. The kind layers
over the base config: the starter's `type` requirement still applies to
every other directory.

## Run it in CI

The same two commands serve editing and CI:

```bash
mdsmith fix .     # rebuild every index.md, normalize formatting
mdsmith check .   # read-only: non-zero exit on any violation
```

`mdsmith check .` is the gate. It walks the bundle, applies your
config, and exits non-zero if any concept document is missing its
`type`, any cross-link is broken, or any `index.md` is stale relative
to the front matter it indexes. Wire it into the same pre-commit hook
or CI step you use for the rest of the repository.

## Full example

`mdsmith init --starter okf` writes this complete `.mdsmith.yml` for a
bundle whose root is the repository:

```yaml
front-matter: true

rules:
  required-frontmatter:
    fields: [type]
    exclude: [index.md, log.md]
  cross-file-reference-integrity:
    links:
      site-root: "."
  first-line-heading: false
  line-length: false
  max-file-length: false
  token-budget: false
  paragraph-readability: false
  paragraph-structure: false
  table-readability: false
```

That is the whole configuration — ordinary, editable mdsmith config.
`required-frontmatter` carries the `type` requirement, `site-root`
turns on bundle-relative link validation, and the disabled rules keep
mdsmith's prose opinions off a data bundle. Everything else — index
generation, formatting, change-log linting — follows from the
directives and reserved-file conventions in the bundle itself.

[okf]: https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md
[mds071]: https://mdsmith.dev/rules/mds071-required-frontmatter/
