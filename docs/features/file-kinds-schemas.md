---
title: "File kinds and schemas"
summary: >-
  Tag each file with a `kind`, then validate its headings and front
  matter against a schema declared inline on the kind or shared via a
  `proto.md` template — so a whole directory obeys one contract.
icon: shapes
link: "/guides/file-kinds/"
rules: ["MDS020"]
weight: 11
group: "A connected docs tree"
---
# File kinds and schemas

Not every Markdown file plays the same role. A plan is not a rule
README, and a release-channel page is not a guide. A per-file
style linter treats them all alike: it catches a long line, but
not a missing Decision section or an invented `status` value. A
directory of similar files drifts as it grows.

A **kind** gives each file a role; a **schema** gives that role a
contract. Bind files to a kind by a front-matter `kinds:` field or
a `kind-assignment` glob. The kind's schema then constrains
required headings, section order, and front-matter fields. Declare
it inline on the kind, or share it from a `proto.md` template so a
whole directory validates against one source of truth.

For example, an `rfc` kind can declare its schema inline in
`.mdsmith.yml`:

```yaml
kinds:
  rfc:
    schema:
      frontmatter:
        status: '"draft" | "ratified" | "deprecated"'
      sections:
        - heading: "Context"
        - heading: "Decision"
```

Tag a file with that kind:

```markdown
---
kinds: [rfc]
status: approved
---
# RFC-0007: Adopt structured logging

## Context

We keep losing requests in unstructured log lines.
```

This file breaks the contract twice. `approved` is not an allowed
`status`. The required `Decision` section is missing. `MDS020`
reports both:

```text
RFC-0007.md:3:1 MDS020 status: got "approved", expected one of: "draft", "ratified", "deprecated"
RFC-0007.md:7:1 MDS020 ## Decision: got <missing>, expected section to be present
```

Schemas go further: they nest sections, repeat them, and inherit
from a parent. See the [file-kinds guide](../guides/file-kinds.md)
and the [schemas guide](../guides/schemas.md) for the full
vocabulary.
