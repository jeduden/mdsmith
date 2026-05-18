---
title: "Conventions and flavors"
summary: >-
  Pin a Markdown convention to get a curated rule preset and a target
  renderer flavor in one switch. `MDS034` flags syntax the flavor
  will not render; a placeholder vocabulary spares template tokens.
icon: book-type
link: "/reference/conventions/"
rules: ["MDS034"]
weight: 10
---
# Conventions and flavors

Markdown is not one language. GitHub renders things plain
CommonMark does not, and a portable doc must avoid both. mdsmith
makes the target explicit.

A **convention** is one config switch. It applies a curated
preset of style rules plus a renderer **flavor**. `MDS034` then
flags any syntax the flavor will not render. A portable doc
cannot smuggle in GitHub-only constructs.

Template files stay exempt. An opt-in placeholder vocabulary
marks tokens like `{name}` as opaque. A scaffold file is not
flagged as broken prose.

See the [conventions reference](../reference/conventions.md) and
the [flavor concept](../background/concepts/flavor-rule-convention-kind.md)
for the preset bundles and how layering works.
