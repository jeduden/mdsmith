---
title: Word-list files under `.mdsmith/wordlists/`
weight: 23
summary: >-
  Each file under `.mdsmith/wordlists/` declares one
  named word-list: an optional `extends:` parent and an
  `entries:` list of literal strings. A rule pulls a
  list in through its `lists:` setting and the entries
  union with the rule's own list. No lists ship compiled
  in; a project declares every list it uses.
---
# Word-list files under `.mdsmith/wordlists/`

A **word-list** is a named set of literal strings. A
rule names one or more lists in its `lists:` setting,
and the resolved entries union into the rule's own list.
A list lives in a YAML file under `.mdsmith/wordlists/`
whose basename is the list name. One file per list, no
nesting. The directory sits next to `.mdsmith.yml` at
the workspace root.

```text
.mdsmith.yml
.mdsmith/
  wordlists/
    house-banned.yaml
    product-names.yaml
```

## File shape

The body has an optional `extends:` parent and a
required `entries:` list of literal strings. A key
outside that set is a config error naming the key and
file.

```yaml
# .mdsmith/wordlists/house-banned.yaml
entries:
  - "synergy"
  - "circle back"
  - "it's important to note that"
```

Each entry is matched verbatim by the rule that reads
it. Quote any entry YAML would otherwise mangle — a
trailing comma, a leading symbol.

## Referencing a list from a rule

Every rule that takes a list setting also takes a
`lists:` key. The named lists resolve to entries that
union with the rule's own inline list:

```yaml
rules:
  forbidden-text:
    lists: [house-banned]
    contains: ["one-off phrase"]
```

`forbidden-text` then flags any paragraph that contains
a `house-banned` entry or the inline phrase. `lists:`
appends across config layers, so a convention's `lists:`
and a project's `lists:` combine rather than replace.

`lists:` fills one setting per rule:

| Rule                         | `lists:` fills   |
| ---------------------------- | ---------------- |
| `forbidden-text`             | `contains`       |
| `forbidden-paragraph-starts` | `starts`         |
| `proper-names`               | `names`          |
| `required-mentions`          | `mentions`       |
| `no-inline-html`             | `allow`          |
| `descriptive-link-text`      | `banned`         |
| `callout-type`               | `allow`          |
| `no-unused-link-definitions` | `ignored-labels` |
| the placeholder rules        | `placeholders`   |

The mechanism is neutral about how a rule reads its
list. `forbidden-text` bans each entry; `required-mentions`
requires each entry in every section.

No lists ship compiled into the binary. Every list a
rule names is one a project declares — in a file here or
inline in `.mdsmith.yml`. The [`no-llm-tells`
convention](conventions.md) is the home for the curated
anti-slop vocabulary; it ships those words as the rules'
inline presets, not as a named list you extend. To start
from that curated set as editable files, run [`mdsmith init
--wordlists`](cli/init.md). It scaffolds `ai-speak.yaml`
and `ai-openers.yaml` here for you to own.

## Extending a list

A list may name one parent with `extends:`. The parent's
entries come first, then the list's own, with duplicates
dropped. Chains resolve to the root; a cycle or a
missing parent is a config error. Use `extends:` to
share one base list across several denylists:

```yaml
# .mdsmith/wordlists/our-slop.yaml
extends: house-banned
entries:
  - "best-in-class"
  - "move the needle"
```

## Basename rule

The list name is the basename minus extension. It must
match `[a-z][a-z0-9-]*` — lower case, starting with a
letter, with optional hyphen-separated segments. Both
`*.yaml` and `*.yml` are scanned; the same basename
across both extensions is a config error naming both
files. Subdirectories and symlinks are rejected. A file
over 1 MB is rejected.

## Composition with `.mdsmith.yml`

An inline `wordlists:` block in `.mdsmith.yml` is a
first-class source — the inline twin of a file:

```yaml
wordlists:
  product-names:
    entries: ["mdsmith", "Goldmark"]
```

The same name declared in both a file and inline is a
config error naming both sources. The two do not merge.

The [`no-llm-tells` convention](conventions.md) ships a
curated anti-slop vocabulary as inline rule presets. A
project that wants those words selects the convention.
It does not name a list here. The [convention
files](convention-files.md) page covers the sibling
`.mdsmith/conventions/` directory.
