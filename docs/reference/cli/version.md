---
command: version
summary: Print the mdsmith build version and exit.
---
# `mdsmith version`

Print the mdsmith build version and exit.

```text
mdsmith version
```

The version is set via `-ldflags="-X main.version=..."`
at build time. When unset (e.g. `go install` or `go run`),
the value falls back to the Go build-info `Main.Version`
or `(devel)`.

## Examples

```bash
mdsmith version
# mdsmith v1.0.0
```

## Exit codes

| Code | Meaning |
|------|---------|
| 0    | Printed |
