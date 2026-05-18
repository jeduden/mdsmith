---
title: "File kinds and schemas"
summary: >-
  Tag each file with a `kind`, then validate its headings and front
  matter against a schema declared inline on the kind or shared via a
  `proto.md` template — so a whole directory obeys one contract.
icon: shapes
link: "/guides/file-kinds/"
rules: ["MDS020"]
weight: 9
---
# File kinds and schemas

Not every Markdown file plays the same role. A plan is not a rule
README, and a release-channel page is not a guide. mdsmith lets
you model that.

A **kind** is a named bundle of rule config. You bind files to it
by front matter, a `kind-assignment` glob, or a `path-pattern`.
Each kind can attach a **schema** that constrains required
headings, section order, and front-matter fields.

Declare the schema inline on the kind or share it from a
`proto.md` template, so a whole directory validates against one
source of truth. Named field-type shortcuts keep the schema
short. `MDS020` reports a precise diagnostic when a file breaks
its contract.

See the [file-kinds guide](../guides/file-kinds.md) and the
[schemas guide](../guides/schemas.md) for the full vocabulary.
