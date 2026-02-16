# Manual QA

Manual QA uses a stratified sample from
`datasets/<version>/qa-sample.jsonl`.

## Procedure

1. Label each sampled record with `actual_category`.
2. Save annotations to `annotations.csv` using columns:
   `record_id,actual_category`.
3. Run `corpusctl qa` to compute agreement and
   precision/recall by category.
4. Review confusion cases and update taxonomy rules.

## Refinement Logged In This Version

Observed confusion in early samples:

- Some `plan/*.md` docs were labeled `reference` instead of
  `design-proposal`.

Refinement applied:

- Classification now maps `plan/` paths to
  `design-proposal` by default.

This change is intended to reduce ambiguity between
implementation plans and static lookup references.
