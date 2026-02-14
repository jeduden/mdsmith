---
id: MDS017
name: no-trailing-punctuation-in-heading
description: Headings should not end with punctuation.
---
# MDS017: no-trailing-punctuation-in-heading

Headings should not end with punctuation.

- **ID**: MDS017
- **Name**: `no-trailing-punctuation-in-heading`
- **Default**: enabled
- **Fixable**: no
- **Implementation**:
  [source](./)
- **Category**: heading

Flags headings that end with `.`, `,`, `:`, `;`, or `!`.

## Config

Enable:

```yaml
rules:
  no-trailing-punctuation-in-heading: true
```

Disable:

```yaml
rules:
  no-trailing-punctuation-in-heading: false
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
