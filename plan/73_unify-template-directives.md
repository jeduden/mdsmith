---
id: 73
title: Unify template and processing directives
status: "🔲"
summary: >-
  Evaluate unifying catalog templates,
  required-structure patterns, and processing
  directives under a single user model with a
  simple mental model and a comprehensive guide.
---
# Unify template and processing directives

Addresses [#68](https://github.com/jeduden/mdsmith/issues/68)
(unify template syntax) and
[#70](https://github.com/jeduden/mdsmith/issues/70)
(central directive documentation).

## Problem

mdsmith exposes three separate mini-languages
for in-document directives:

1. **Go `text/template`** in catalog `row`/`header`
   /`footer` parameters
   (`{{.title}}`, `{{.filename}}`)
2. **Pattern placeholders** in required-structure
   heading templates (`{{.field}}` matched as
   regex wildcards)
3. **Processing-instruction markers** with YAML
   bodies (`<?catalog ... ?>`, `<?include ... ?>`,
   `<?require ... ?>`, `<?allow-empty-section?>`)

A user must learn all three to use the tool
confidently. The concepts overlap but the
behaviors diverge:

- `{{.title}}` in a catalog row *renders* a
  value; `{{.title}}` in a required-structure
  heading *matches* any text.
- `<?require?>` is a single marker with no
  closing tag; `<?catalog?>` is a marker pair
  whose body is regenerated.
- `<?allow-empty-section?>` takes no parameters;
  `<?catalog?>` takes up to seven.

## Goal

Give each directive a clear "if X then Y" rule
so users can predict behavior on sight. Write
one guide that covers all directives with
examples.

## Analysis of current user model

### What a user sees today

| Marker                         | X (trigger)          | Y (effect)                            |
|--------------------------------|----------------------|---------------------------------------|
| `<?catalog ...?>`                | Marker pair in file  | Body is regenerated from glob matches |
| `<?include ...?>`                | Marker pair in file  | Body is replaced with included file   |
| `<?require ...?>`                | Marker in template   | Filename must match glob              |
| `<?allow-empty-section?>`        | Marker after heading | Empty section is allowed              |
| `{{.field}}` in catalog row      | Inside `row:` param    | Replaced with front-matter value      |
| `{{.field}}` in template heading | Inside template `.md`  | Heading must contain that value       |

### Pain points

1. **Same syntax, different semantics** --
   `{{.field}}` means "insert value" in catalog
   but "match any text" in required-structure.
2. **Marker pairs vs single markers** -- no
   visual cue tells the user whether a marker
   needs a closing tag.
3. **Parameter count varies wildly** -- from
   zero (allow-empty-section) to seven (catalog).
4. **No single reference** -- rules are
   documented per-rule; no unified walkthrough
   exists.

### What works well

- The `<?name ... ?>` syntax is visually
  distinct from Markdown content.
- YAML inside markers is familiar to users who
  already write front matter.
- Catalog and include share the same
  open/generate/close lifecycle via the
  `gensection` archetype.

## Proposed user model

### Principle: two kinds of markers

Collapse all directives into two categories a
user can identify on sight:

| Kind       | Shape                         | Rule                                          | User prediction                                     |
|------------|-------------------------------|-----------------------------------------------|-----------------------------------------------------|
| **Generator**  | `<?name ...?>` ... `<?/name?>`    | Content between markers is managed by mdsmith | "If I see a marker pair, `fix` regenerates the body"  |
| **Constraint** | `<?name ...?>` (no closing tag) | The marker asserts a condition                | "If I see a lone marker, `check` validates something" |

The core rule: if a marker has a closing tag,
the body is auto-generated; if it does not, it
is a validation constraint.

This is already almost true today. The plan
makes it explicit and documents it as the
governing rule.

### Principle: one template language

Unify `{{.field}}` to always mean the same
thing: "insert the value of `field`."

- **Catalog** already uses Go `text/template` --
  keep as-is.
- **Required-structure** currently reuses the
  `{{.field}}` syntax for pattern matching, not
  insertion. This is the source of confusion.

**Proposal**: change required-structure to use a
different sigil for wildcard matching. Two
options:

| Option          | Syntax                | Meaning                                                    |
|-----------------|-----------------------|------------------------------------------------------------|
| A (recommended) | `{field}` single braces | "heading must contain the value of front-matter key `field`" |
| B               | `*`                     | "any text" (glob-style)                                    |

Option A keeps the link to front matter explicit
while removing the visual collision with Go
templates. Option B is simpler but loses the
field-name hint.

**Decision required from maintainer:** pick A
or B before implementation.

### Principle: consistent YAML parameters

All directives already use YAML for parameters.
No change needed, but the guide must present
them with a uniform layout:

```markdown
<?directive-name
param1: value
param2: value
?>
```

### What does NOT change

- The `<?...?>` marker syntax itself stays.
- The YAML parameter format stays.
- The `gensection` engine for generators stays.
- The `fix` / `check` CLI commands stay.
- `.mdsmith.yml` configuration stays.

This is intentionally conservative. The goal is
a clearer mental model and a guide, not a
rewrite.

## Tasks

### Phase 1: central directive reference (#70)

1. Write `docs/guides/directives.md` covering:

  - The two-kind model (generator vs
     constraint)
  - Placement constraints: must be at document
     root, at most 3-space indent, ignored
     inside fenced code blocks and HTML blocks
  - Every directive with: purpose, parameters,
     one good example, one bad example, what
     `check` reports, what `fix` does
  - A quick-reference table at the top
  - Classification by role: "generated-section
     markers" vs "rule modifiers / escape
     hatches" (per #70)

2. Add cross-links from each rule README to the
   guide.
3. Update `CLAUDE.md` catalog to include the new
   page.

### Phase 2: resolve `{{.field}}` ambiguity

4. Decide on Option A or B for
   required-structure patterns (needs maintainer
   input).
5. Implement the chosen syntax in
   `requiredstructure/rule.go`:

  - Update pattern extraction to use new sigil
  - Keep `{{.field}}` as a deprecated alias for
     one release cycle

6. Update `internal/rules/MDS020-required-structure/README.md`.
7. Update the directive guide with the new
   syntax.
8. Migrate existing template files
   (`plan/proto.md`,
   `internal/rules/proto.md`,
   `.claude/skills/proto.md`) to the new syntax.

### Phase 3: evaluate template engine (stretch)

9. Prototype replacing Go `text/template` with
   gonja or pongo2 in catalog rendering.

  - Measure: does Jinja syntax reduce user
     confusion vs Go templates?
  - Measure: does it enable filters/conditionals
     that users actually need?

10. If the prototype shows clear benefit, plan a
   migration. If not, keep Go `text/template` and
   document its quirks in the guide.

## Acceptance Criteria

- [ ] `docs/guides/directives.md` exists and
      covers every directive with examples
- [ ] The guide passes `mdsmith check
      docs/guides/`
- [ ] `{{.field}}` in required-structure
      templates uses a distinct syntax from
      catalog templates (Phase 2)
- [ ] Existing template files are migrated
      (Phase 2)
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no
      issues
