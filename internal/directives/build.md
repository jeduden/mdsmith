---
title: build directive
summary: >-
  Declare a build artifact and recipe; mdsmith fix
  syncs the section body to the recipe output.
---
# `<?build?>` directive

Declares a build artifact — a file produced by a
recipe configured in `build.recipes`. `mdsmith fix`
renders the section body from the recipe
`body-template`. mdsmith never executes the recipe
itself; MDS040 statically checks recipe commands.

```text
<?build
recipe: RECIPE-NAME
output: path/to/artifact.ext
?>
RENDERED BODY
<?/build?>
```

Required params:

- `recipe` — a name declared in `build.recipes`
- `output` — a relative path, no `..`, no absolute
  paths

MDS039 validates the directive and reports stale
bodies. MDS040 shell-safety-checks the recipe at
lint time. Run `mdsmith fix <file>` to regenerate a
stale body.

See the full
[build directive guide](../../docs/guides/directives/build.md)
for recipe declaration, body templates, and rule
interactions.
