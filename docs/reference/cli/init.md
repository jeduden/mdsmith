---
command: init
summary: Generate a default `.mdsmith.yml` config in the current directory.
---
# `mdsmith init`

The generated file lists every rule with its built-in
default settings, so you can flip individual rules off or
override settings with a clear diff.

```text
mdsmith init
```

Refuses to overwrite an existing `.mdsmith.yml`. Takes
no arguments and no flags.

## Examples

```bash
mdsmith init
$EDITOR .mdsmith.yml
```

## Exit codes

| Code | Meaning                                 |
| ---- | --------------------------------------- |
| 0    | Config written                          |
| 2    | `.mdsmith.yml` already exists, or error |
