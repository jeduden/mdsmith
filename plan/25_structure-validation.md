---
id: 25
title: Markdown Structure Validation
status: 🔲
---
# Markdown Structure Validation

## Goal

Add three features that work together: template-based
structure checks, front matter sync with body text, and
file includes. These help keep rule and plan docs
consistent and avoid content being copied by hand.

## Prerequisites

Plan 27 (test case folders) provides the fixture paths the
include directive references. The rule and template work can
proceed independently.

## Tasks

### A. Template file format

1. Define the template file format: a plain markdown file
   where headings list the needed sections. Settings for
   the template go in its front matter:

   ```yaml
   ---
   template:
     allow-extra-sections: true
     heading-order: strict
   ---
   ```

  - Literal headings are required (`## Settings`)
  - `?` in heading text matches any text (`# ?` matches
     any H1)
  - `{{.field}}` references the checked document's
     frontmatter value for synchronization checking
  - Body content under headings is informational unless
     it contains `{{.field}}` sync points

2. Create `rules/proto.md` as the rule README template:

   ```markdown
   ---
   template:
     allow-extra-sections: true
   ---
   # {{.id}}: {{.name}}

   {{.description}}

   - **ID**: {{.id}}
   - **Name**: `{{.name}}`

   ## Settings

   ## Config

   ## Examples

   ### Bad

   ### Good
   ```

   The `allow-extra-sections: true` flag permits rules to
   add additional sections (e.g. TM019's Marker Syntax,
   Directives, Diagnostics sections).

3. Update `plan/proto.md` with template frontmatter:

   ```yaml
   ---
   template:
     allow-extra-sections: false
   ---
   ```

   Headings stay as-is: `# ?`, `## Goal`, `## Tasks`,
   `## Acceptance Criteria`. Plans must follow this
   structure exactly with no extra top-level sections.

### B. Implement TM020 `required-structure`

4. Create `internal/rules/requiredstructure/rule.go`
   implementing `rule.Rule` and `rule.Configurable`.
   Settings:

   | Setting | Type | Default | Description |
   |---------|------|---------|-------------|
   | `template` | string | `""` | Path to template file |

   When `template` is empty or unset the rule is a no-op
   (no diagnostics emitted). This lets users enable it
   only for specific file groups via overrides.

5. Template parsing: read the template file relative to
   the linted file's filesystem root. Extract an ordered
   list of required headings with their levels, and
   collect `{{.field}}` sync points with their expected
   positions (heading text or body line).

6. Structure checking: make sure the file has all needed
   headings in the right order and at the right level.
   Diagnostics:

  - `missing required section "## Settings"`
  - `section "## Examples" is out of order`
  - `heading level mismatch: expected h2, got h3`

7. Frontmatter-body sync: when the template contains
   `{{.field}}` references, substitute the target
   document's frontmatter values and compare against
   the actual body text. Diagnostics:

  - `heading does not match frontmatter: expected`
     `"TM001" (from id), got "TM002"`
  - `body does not match frontmatter field`
     `"description"`

8. Register rule in `init()`, add `required-structure`
   to the known-rules list in `config/load.go`, add
   blank import in `main.go`.

### C. Implement `include` directive for TM019

9. Add `include` as a recognized directive in the
   `generated-section` rule alongside `catalog`.
   Parameters:

   | Parameter | Required | Default | Meaning |
   |-----------|----------|---------|---------|
   | `file` | yes | -- | File path |
   | `strip-frontmatter` | no | `true` | Drop YAML |
   | `wrap` | no | -- | Code fence |

10. Include rendering logic:

  - Read the file relative to the linted file's
      directory (same resolution as catalog's glob)
  - If `strip-frontmatter: true` (default), remove the
      YAML frontmatter block from the included content
  - If `wrap` is set (e.g. `wrap: markdown`), wrap the
      content in a fenced code block with that language
  - Compare rendered content with the text between
      markers

11. Fix behaviour: replace content between markers with
    freshly rendered include. Same idempotency guarantees
    as the catalog directive.

### D. Wire up configuration

12. Update `.tidymark.yml` with overrides applying the
    template to the right file groups:

    ```yaml
    overrides:
      - files: ["rules/*/README.md"]
        rules:
          required-structure:
            template: rules/proto.md
      - files: ["plan/*.md"]
        rules:
          required-structure:
            template: plan/proto.md
    ```

13. Exclude template files from the structure check.
    Files containing `template:` frontmatter are
    templates, not documents. The rule skips them.

### E. Migrate rule READMEs to use includes

14. Update each rule README to replace the inline
    Bad/Good examples with `include` directives
    referencing the fixture files:

    ```text
    <!-- tidymark:gen:start include
    file: bad/default.md
    wrap: markdown
    -->
    ...
    <!-- tidymark:gen:end -->
    ```

15. Run `tidymark fix rules/` to populate the include
    sections and verify `tidymark check rules/` passes.

### F. Tests

16. Unit tests for template parsing: extract headings,
    `{{.field}}` sync points, and template options from
    a template file.

17. Unit tests for structure checking: missing heading,
    extra heading with `allow-extra-sections: false`,
    out-of-order heading, wrong level.

18. Unit tests for frontmatter-body sync: values match,
    values mismatch, missing frontmatter field, wildcard
    heading (`?`).

19. Unit tests for include directive: basic include,
    strip-frontmatter on/off, wrap in code fence, missing
    file, file outside repo.

20. Integration tests: validate existing rule READMEs
    against `rules/proto.md` and plan files against
    `plan/proto.md`.

21. Write `rules/TM020-required-structure/README.md`
    with examples for both template validation and
    frontmatter sync use cases.

## Acceptance Criteria

- [ ] Template file format supports required headings,
      `{{.field}}` sync, and `?` wildcards
- [ ] TM020 reports missing or out-of-order headings
- [ ] TM020 reports frontmatter-body value mismatches
- [ ] TM019 `include` directive embeds file content
- [ ] Include supports `strip-frontmatter` and `wrap`
- [ ] Rule READMEs use includes for Bad/Good examples
- [ ] `plan/*.md` validated against `plan/proto.md`
- [ ] `rules/*/README.md` validated against
      `rules/proto.md`
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
