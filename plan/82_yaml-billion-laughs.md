---
id: 82
title: 'YAML billion-laughs mitigation'
status: "✅"
summary: >-
  Reject YAML anchor/alias syntax in user-supplied
  content before unmarshalling to prevent exponential
  memory expansion.
---
# YAML billion-laughs mitigation

## Goal

Prevent OOM from YAML front matter or directive bodies
that use anchor/alias expansion. These create
exponential memory growth during `yaml.Unmarshal`.

## Background

`gopkg.in/yaml.v3` has no alias-expansion limit.
A 1 KB YAML with 8 levels of nested aliases can expand
to 10^8 strings. Byte-length caps on input do not
prevent this — the attack uses small input.

Prettier and remark-lint use `eemeli/yaml` which
defaults to `maxAliasCount: 100`. Vale uses
`yaml.v2` v2.4.0 which back-ported alias-depth fixes.
markdownlint avoids the issue by not parsing YAML at
all. textlint (js-yaml v4) is vulnerable like mdsmith.

Legitimate Markdown front matter virtually never uses
YAML anchors or aliases.

## Design

### Pre-scan approach

Before any `yaml.Unmarshal` call on user-supplied
content, scan the raw bytes for YAML anchor (`&`) or
alias (`*`) characters. If found, return an error
diagnostic rather than proceeding to unmarshal.

```go
func RejectYAMLAliases(data []byte) error {
    if bytes.ContainsAny(data, "&*") {
        return fmt.Errorf(
            "YAML anchors/aliases are not permitted")
    }
    return nil
}
```

### Refinement: context-aware scan

A bare `&` or `*` can appear in YAML string values
(e.g., `title: "Q&A"`). To reduce false positives,
only reject when `&` or `*` appears in a YAML
structural position:

- `&` followed by a YAML identifier (anchor
  definition): pattern `&\w`
- `*` followed by a YAML identifier (alias reference):
  pattern `\*\w` at the start of a value

Use a simple regex or byte scan, not a full YAML
parser.

### Call sites to guard

All `yaml.Unmarshal` sites processing user-supplied
content (13 total):

1. `internal/archetype/gensection/parse.go:189`
   (directive YAML body)
2. `internal/rules/catalog/rule.go:415`
   (per-file front matter)
3. `internal/rules/requiredstructure/rule.go:220,233,556,924`
   (schema front matter, require directives)
4. `cmd/mdsmith/main.go:352`
   (`query` subcommand front matter)
5. `internal/config/load.go:22,35`
   (config file — lower risk, operator-controlled)
6. `internal/corpus/config.go:35,82`
   (corpus config — internal tooling)

Sites 1–4 are high priority (user-supplied `.md`
content). Sites 5–6 are lower priority (operator-
controlled config files) but should be guarded for
defense in depth.

### Alternative considered

Use `goccy/go-yaml` with `yaml.WithMaxAliasesNum(100)`
to mirror `eemeli/yaml`. This is a larger dependency
change. Defer unless the pre-scan produces false
positives.

## Tasks

1. [x] Add `internal/lint/yamlsafe.go` with
   `RejectYAMLAliases(data []byte) error`
2. [x] Add `internal/lint/yamlsafe_test.go` with tests
   for clean YAML, anchor YAML, alias YAML, `Q&A`
   in string values (false positive check)
3. [x] Guard all 11 `yaml.Unmarshal` call sites with a
   `RejectYAMLAliases` check before unmarshalling
4. [x] Add integration test: `.md` file with YAML anchor
   front matter produces a clear error diagnostic

## Acceptance Criteria

- [x] YAML with `&anchor` / `*alias` is rejected
      before unmarshalling
- [x] Legitimate front matter with `&` in string
      values (e.g., `"Q&A"`) is accepted
- [x] All 11 `yaml.Unmarshal` sites are guarded
- [x] Error message clearly states anchors/aliases
      are not permitted
- [ ] All tests pass: `go test ./...`
- [ ] `go tool golangci-lint run` reports no issues
