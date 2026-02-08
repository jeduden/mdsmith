# 07: Code Block Rules (TM010-TM011, TM015)

## Goal

Implement three code-block-related rules using the goldmark AST
(`ast.FencedCodeBlock`).

## Rules

| Rule | Name | Fixable |
|------|------|---------|
| TM010 | fenced-code-style | yes |
| TM011 | fenced-code-language | no |
| TM015 | blank-line-around-fenced-code | yes |

## Tasks

1. Implement each rule in its own package under `internal/rules/`
2. Each rule inspects `ast.FencedCodeBlock` nodes from the AST
3. Create testdata fixtures for each rule
4. Write behavioral tests

## Acceptance Criteria

### TM010: fenced-code-style

- [ ] A backtick-fenced block reports nothing when `style: backtick` (default)
- [ ] A tilde-fenced block (`~~~`) reports a diagnostic when `style: backtick`
- [ ] A tilde-fenced block reports nothing when `style: tilde`
- [ ] A backtick-fenced block reports a diagnostic when `style: tilde`
- [ ] Fix replaces tilde fences with backtick fences (and vice versa),
      preserving the language tag and content
- [ ] Indented code blocks (4-space) are not flagged (they have no fence)
- [ ] Nested fenced code blocks (4+ backticks) are handled correctly

### TM011: fenced-code-language

- [ ] A fenced code block without a language tag reports a diagnostic
- [ ] A fenced code block with a language tag (e.g., ` ```go `) reports nothing
- [ ] Indented code blocks are not flagged
- [ ] Multiple fenced blocks: only those missing a language are flagged
- [ ] The diagnostic points to the opening fence line

### TM015: blank-line-around-fenced-code

- [ ] A fenced code block preceded by non-blank text reports a diagnostic
- [ ] A fenced code block followed by non-blank text reports a diagnostic
- [ ] A fenced code block with blank lines before and after reports nothing
- [ ] A fenced code block at the start of the file does not require a blank
      line before
- [ ] A fenced code block at the end of the file does not require a blank
      line after
- [ ] Fix inserts blank lines before/after fenced code blocks as needed
- [ ] The closing fence line is also checked (blank line after it)

### General

- [ ] Each rule is enabled by default
- [ ] Each rule can be disabled via config
- [ ] Configurable settings (`style`) are read from config and applied
- [ ] All tests pass: `go test ./...`
- [ ] `golangci-lint run` reports no issues
