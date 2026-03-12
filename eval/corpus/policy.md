# Collection Policy

## Licensing

Only sources with licenses in `license_allowlist` are eligible
for collection. Source-level `license` values must match the
allowlist in `config.yml`.

## Minimum Content Thresholds

A markdown file is kept only when it passes both thresholds:

- `min_words`
- `min_chars`

Files below either threshold are skipped.

## Provenance and Redistribution

Corpus outputs in `datasets/` store metadata, taxonomy labels,
and computed measurements. Raw source markdown content is not
committed to version control.

## Source Annotation

Each source entry in `config.yml` should include an
`annotations` map to document:

- why the source was selected
- what content it contributes
- the license reference URL
