---
id: MDS038
name: toc
status: ready
description: Keep toc generated heading lists in sync with document headings.
---
# MDS038: toc

Keep toc generated heading lists in sync with
document headings.

## What it detects

A `<?toc?>...<?/toc?>` block contains a
generated list of headings linked to their
GitHub-style anchors. When the block body
does not match what the directive would
produce from the current document headings,
MDS038 reports a "generated section is out of
date" diagnostic.

## Parameters

| Name        | Type | Default | Description                                   |
|-------------|------|---------|-----------------------------------------------|
| `min-level` | int  | `2`     | Lowest heading level to include (1–6)         |
| `max-level` | int  | `6`     | Highest heading level to include (1–6, ≥ min) |

`min-level: 2` skips the document title (H1)
by default, matching Python-Markdown's `[TOC]`
default output.

## Generated content

A nested unordered list, one item per heading
in source order. Each item links to the
GitHub-style slug for that heading. Duplicate
headings get `-1`, `-2`, … suffixes.

Indentation is 2 spaces per depth level.
The list structure follows the heading
hierarchy, not raw levels. Given H2 → H4 → H2,
the H4 is indented under the preceding H2.

## Config

Disable:

```yaml
rules:
  toc: false
```

## Examples

### Good

```markdown
# Guide

<?toc?>

- [Introduction](#introduction)
- [Usage](#usage)
  - [Basic Usage](#basic-usage)

<?/toc?>

## Introduction

…

## Usage

### Basic Usage

…
```

### Bad

```markdown
# Guide

<?toc?>

- [Old Section](#old-section)

<?/toc?>

## Introduction

…
```

MDS038 reports a "generated section is out of
date" diagnostic on the `<?toc?>` line.

## Meta-Information

- **ID**: MDS038
- **Name**: `toc`
- **Status**: ready
- **Default**: enabled
- **Fixable**: yes
- **Implementation**:
  [source](./)
- **Category**: meta
