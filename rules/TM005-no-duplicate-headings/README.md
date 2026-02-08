# TM005: no-duplicate-headings

No two headings should have the same text.

- **ID**: TM005
- **Name**: `no-duplicate-headings`
- **Default**: enabled
- **Fixable**: no
- **Implementation**: [`internal/rules/noduplicateheadings/`](../../internal/rules/noduplicateheadings/)

## Config

```yaml
rules:
  no-duplicate-headings: true
```

## Examples

### Bad

```markdown
# Introduction

## Overview

## Overview
```

### Good

```markdown
# Introduction

## Project Overview

## API Overview
```
