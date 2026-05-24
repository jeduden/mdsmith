---
id: 211
title: "`<?include?>` projects any typed value of any kind via `extract`"
status: "🔲"
summary: >-
  Extend the include directive to pull a typed value out of any
  kind-typed Markdown file via the same projection
  `mdsmith extract` produces. Deletes the generated fragment
  layer under `docs/brand/fragments/` and the
  `messagingFragmentTargets` patcher; replaces hand-rolled
  shims with a generic feature.
model: opus
depends-on: [210]
---
# `<?include?>` projects any typed value of any kind via `extract`

## Goal

A README or in-repo doc should splice a single typed value
out of any kind-typed Markdown file. The lead paragraph from
a product-copy file. The version string from a release plan.
The H1 wordmark from the brand source.

Today `<?include?>` only takes `file:` and splices the whole
body. To embed one field, you generate a separate fragment
file holding it. That is what `mdsmith-release sync-messaging`
does for the README intros under `docs/brand/fragments/`. The
fragment layer is a workaround for a missing directive
parameter.

After this plan, the directive accepts an `extract:` parameter.
The value is a dotted path. It walks the JSON tree
[`mdsmith extract`][extract-ref] would produce from the file:

```markdown
<?include
file: docs/brand/messaging.md
extract: tagline.text
?>
```

…splices the rendered text of the `## Tagline` section of
`docs/brand/messaging.md`. Frontmatter is reachable through the
same path syntax:

```markdown
<?include
file: docs/brand/messaging.md
extract: frontmatter.title
?>
```

…splices `mdsmith product messaging`. Any kind-typed file
participates; the projection contract is the kind's schema,
which the extract reference already documents. The directive
runs the file through extract internally, decodes the JSON,
walks the dotted path, and splices the resulting scalar (or
the rendered text of an object).

[extract-ref]: ../docs/reference/cli/extract.md

## Tasks

1. **Directive parameter parsing.** Add `extract:` to the
   include directive validator in
   [`internal/rules/include/`](../internal/rules/include).
   Reject the parameter on files whose resolved kind set is
   empty (no extract contract). Reject `extract:` together with
   `strip-frontmatter:` or `heading-level:` for the first
   iteration — the value flow is scalar-only, so the
   frontmatter / heading-level params do not apply.
2. **Extract integration.** When the directive carries
   `extract:`, run the included file through the same projection
   `mdsmith extract` produces (re-use
   [`internal/extract`](../internal/extract) directly — no
   shell-out). Decode the JSON, walk the dotted path, and
   splice the leaf value. Object leaves with a single
   well-known content key (`text`, `code`, `items`, `rows`)
   splice the inner value; ambiguous objects are a lint error.
3. **Lint behavior.** A failing path lookup (`extract: nope.x`)
   is `MDS021 generated section is out of date` with the
   error message pointing at the missing key. A schema-non-
   conformant target file surfaces the same diagnostic that
   `mdsmith check` would surface for it, prefixed with the
   directive's call site.
4. **Auto-fix.** `mdsmith fix` regenerates the body of the
   include block from the current projection — the same model
   the existing file-include path uses. Round-trip stability
   matches the existing include behavior.
5. **Adopt in messaging.** Update [`README.md`](../README.md),
   [`npm/mdsmith/README.md`](../npm/mdsmith/README.md), and
   [`python/README.md`](../python/README.md) to use
   `<?include file: docs/brand/messaging.md extract: tagline.text ?>`.
6. **Delete the fragment layer.** Remove
   [`docs/brand/fragments/`](../docs/brand/fragments) and the
   `messagingFragmentTargets` entries in the release tooling
   under [`internal/release/`][rel]. Drop the
   `MarkdownFragment` patcher type if no other target
   references it.
7. **Documentation.** Add an "Include a typed value"
   subsection to the
   [generating-content guide][gen-content] with worked
   examples (text, code, frontmatter, nested bind). Update
   the [extract-markdown-as-data guide][extract-guide] to
   point at the new directive as the read-side companion to
   the projection rules.

[rel]: ../internal/release/messaging_targets.go
[gen-content]: ../docs/guides/directives/generating-content.md
[extract-guide]: ../docs/guides/extract-markdown-as-data.md

## Acceptance Criteria

- [ ] `<?include file: <f> extract: <path> ?>` resolves the
  dotted path in the kind's extract projection and splices the
  scalar (or the rendered text of an object) into the block
  body.
- [ ] `mdsmith check` flags a missing path, an ambiguous object
  target, or an `extract:` on a file with no resolved kind.
- [ ] `mdsmith fix` regenerates the block body from the
  projection; running twice is byte-stable.
- [ ] Every README that previously read a fragment file now
  reads from `docs/brand/messaging.md` directly via
  `extract: tagline.text`; `docs/brand/fragments/` is removed
  and `mdsmith check .` stays clean.
- [ ] `mdsmith-release sync-messaging --check` reports no drift
  (the JSON / TOML / YAML patcher targets continue working
  unchanged; only the fragment layer goes away).
- [ ] The new directive is documented in
  [generating-content.md](../docs/guides/directives/generating-content.md)
  with at least one worked example per value type (text, code,
  frontmatter scalar).
- [ ] All tests pass: `go test ./...`.
- [ ] `go tool golangci-lint run` reports no issues.
