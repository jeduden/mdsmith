---
weight: 30
summary: >-
  The top-level `foreign-regions:` config lists `{start, end}` marker
  pairs whose spanned bytes mdsmith treats as opaque — style rules skip
  diagnostics inside a matched pair and fixers never rewrite it, while
  whole-file rules still count the bytes. Glob-scopable via `overrides:`;
  a start with no matching end reports MDS073.
---
# Foreign regions

A **foreign region** is a span of a Markdown file that another
generator owns. The top-level `foreign-regions:` key lists the marker
pairs that bound such spans, so `mdsmith fix` leaves those bytes
byte-for-byte unchanged and the style rules raise no diagnostics inside
them. The first case is [APM][apm], whose `managed_section` mode writes
a block bounded by `<!-- apm:start -->` and `<!-- apm:end -->` and pins
a SHA-256 of the file; any reflow inside that block trips its drift
check.

This is the same exclusion the [generated-section engine][gensection]
applies to `<?include?>` and `<?catalog?>` bodies. The difference is
ownership: mdsmith regenerates its own directive bodies, but never
touches a foreign region — the owning tool regenerates it.

## Configuration

Declare each marker pair under `foreign-regions:`. mdsmith matches
`start` and `end` against whole lines with surrounding whitespace
trimmed. A marker may be indented, but it must sit on its own line:

```yaml
foreign-regions:
  - start: "<!-- apm:start -->"
    end: "<!-- apm:end -->"
```

Both markers must be non-empty and must differ from each other; a pair
that violates either rule is a config error.

### Scoping to a subtree

`foreign-regions:` is glob-scopable through
[`overrides:`](globs.md). An override's `foreign-regions:` list is
**appended** to the top-level list for every file its glob matches — it
never replaces the global pairs. Use this to protect an extra marker
pair that only appears under one path:

```yaml
foreign-regions:
  - start: "<!-- apm:start -->"
    end: "<!-- apm:end -->"
overrides:
  - glob: ["AGENTS.md"]
    foreign-regions:
      - start: "<!-- gen:start -->"
        end: "<!-- gen:end -->"
```

## What the region protects

mdsmith scans each file for every declared marker pair. It records the
span from a `start` line through its matching `end` line. The markers
themselves are part of the span.

| Surface                           | Behavior inside a matched region                                                           |
| --------------------------------- | ------------------------------------------------------------------------------------------ |
| `mdsmith fix`                     | Bytes round-trip unchanged, even otherwise-fixable trailing spaces and table misalignment. |
| Style and content rules           | Emit no diagnostics; the same violation outside the region still fires.                    |
| Whole-file rules (MDS022, MDS028) | Still count the region's bytes toward file length and token budget.                        |

Whole-file rules read the raw source. A large foreign region still
counts against `max-file-length` and `token-budget`. The region is
opaque to editing, not invisible to size accounting.

## Malformed regions (MDS073)

APM requires each marker exactly once. mdsmith reports **MDS073** on a
malformed region:

| Condition                                       | Reported on                | Bytes protected                              |
| ----------------------------------------------- | -------------------------- | -------------------------------------------- |
| A `start` marker with no matching `end`         | the `start` line           | none                                         |
| An `end` marker with no preceding `start`       | the `end` line             | none                                         |
| A second `start` before the first region closes | the duplicate `start` line | the first `start` through the matching `end` |

An unmatched `start` or `end` protects no bytes. A duplicate `start`
still pairs the first `start` with the next `end`, so that span is
protected while the extra marker only draws the diagnostic.

## Non-goals

- **Regenerating the region.** mdsmith treats it as opaque; the owning
  tool regenerates it.
- **Resolving merge conflicts inside it.** The
  [merge driver](cli/merge-driver.md) stays scoped to mdsmith's own
  directive blocks.
- **Auto-detecting markers.** Regions are declared in config, never
  inferred.

[apm]: ../research/apm-mdsmith/apm-model.md
[gensection]: ../background/concepts/generated-section.md
