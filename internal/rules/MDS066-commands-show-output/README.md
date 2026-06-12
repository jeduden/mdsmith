---
id: MDS066
name: commands-show-output
status: ready
description: >-
  A fenced code block whose every non-blank line begins
  with `$ ` and shows no output must drop the prompt.
nature: style
category: code
maintainability: null
markdownlint:
  - id: MD014
    name: commands-show-output
    partial: false
    default: true
rumdl:
  - id: MD014
    name: commands-show-output
    partial: false
    default: true
mado:
  - id: MD014
    name: commands-show-output
    partial: false
    default: true
panache: []
obsidian-linter: []
gomarklint: []
---
# MDS066: commands-show-output

A fenced code block whose every non-blank line begins
with `$ ` and shows no output must drop the prompt.

## Config

Enable (default):

```yaml
rules:
  commands-show-output: true
```

Disable:

```yaml
rules:
  commands-show-output: false
```

## Autofix

`mdsmith fix` strips the leading `$ ` from every non-blank
content line of the offending block. Blank lines pass through
unchanged. Only fenced code blocks are inspected; indented
blocks are out of scope.

## Examples

### Good — block shows output

<?include
file: good/with-output.md
wrap: markdown
?>

````markdown
# Title

A block that shows both commands and their output is fine:

```sh
$ ls
foo bar
$ pwd
/home/user
```
````

<?/include?>

### Good — no `$` prompts

<?include
file: good/no-prompts.md
wrap: markdown
?>

````markdown
# Title

A block that has no `$ ` prefixes is fine:

```sh
ls
pwd
```
````

<?/include?>

### Bad — only commands, no output

<?include
file: bad/default.md
wrap: markdown
?>

````markdown
# Title

Commands shown with `$` and no output:

```sh
$ ls
$ pwd
```
````

<?/include?>

## Meta-Information

- **ID**: MDS066
- **Name**: `commands-show-output`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: code
- **markdownlint**: [MD014][mdl-md014] (commands-show-output)
- **rumdl**: [MD014][rumdl-md014] (commands-show-output)
- **mado**: [MD014][mado-rules] (commands-show-output)

[mdl-md014]: https://github.com/DavidAnson/markdownlint/blob/main/doc/md014.md
[rumdl-md014]: https://rumdl.dev/md014/
[mado-rules]: https://github.com/akiomik/mado#supported-rules
