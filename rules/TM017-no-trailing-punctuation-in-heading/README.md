---
id: TM017
name: no-trailing-punctuation-in-heading
description: Headings should not end with punctuation.
---
# TM017: no-trailing-punctuation-in-heading

Headings should not end with punctuation.

- **ID**: TM017
- **Name**: `no-trailing-punctuation-in-heading`
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](../../internal/rules/notrailingpunctuationinheading/)
- **Category**: heading

## Details

Flags headings that end with `.`, `,`, `:`, `;`, or `!`.

## Config

```yaml
rules:
  no-trailing-punctuation-in-heading: true
```

## Examples

### Bad

```markdown
# Introduction.

## Overview:
```

### Good

```markdown
# Introduction

## Overview
```
