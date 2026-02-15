# Conciseness Label Rubric

Use this rubric to assign paragraph labels.

## Labels

- `verbose-actionable`: wording is unnecessarily long and can be
  shortened without losing required precision.
- `acceptable`: paragraph length and phrasing are reasonable for the
  content and audience.

## Include as `verbose-actionable`

- Repeated filler terms that do not add meaning.
- Hedging that weakens statements without evidence needs.
- Verbose multi-word phrases with clear shorter alternatives.
- Circular explanation that repeats the same claim.

## Keep as `acceptable`

- Necessary precision for legal, safety, or protocol constraints.
- Technical qualifiers required for correctness.
- Dense but concise technical statements.
- Short paragraphs where edits would mostly change style, not signal.

## Tie-Break Rule

When uncertain, ask:

1. Can this paragraph be reduced by about 20 percent while preserving
   all required meaning?
2. Would most readers agree the shorter version is strictly better?

If both are yes, label `verbose-actionable`.

## Cue Annotation

When label is `verbose-actionable`, add up to 3 cues in
the record `cues` field. Prefer exact phrase spans.

- `filler`: low-information filler words.
- `hedge`: uncertain framing without required evidence.
- `verbose-phrase`: longer phrase with shorter alternative.
- `redundancy`: repeated idea with no added detail.

## Canonical Examples

Add at least 10 approved examples before model selection:

1. 5 `verbose-actionable` examples across doc types.
2. 5 `acceptable` examples including technical qualifiers.
