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

Use these baseline examples for calibration.

### `verbose-actionable` examples

1. Onboarding guide

```markdown
Basically, we just need to make sure everyone can find the setup page,
and it seems like this is very important for new contributors.
```

Why: filler and hedge language with low added signal.
Cues: `basically`, `it seems`, `very`.

2. Release process note

```markdown
In order to make the release happen, we are going to proceed to run the
publish command at this point in time and then check it again.
```

Why: verbose phrases and repeated intent.
Cues: `in order to`, `at this point in time`, `then check it again`.

3. Incident runbook intro

```markdown
It appears that the service might be failing due to the fact that the
queue is kind of full, so we should maybe look into that first.
```

Why: weak hedging and unnecessary padding.
Cues: `it appears`, `due to the fact that`, `kind of`.

4. Architecture proposal summary

```markdown
For the purpose of clarity, this section is basically explaining that
the cache should probably be read before we read from storage.
```

Why: repeated framing and uncertain wording.
Cues: `for the purpose of`, `basically`, `probably`.

5. Policy change announcement

```markdown
We are really just trying to say that people should, in most cases,
avoid sharing screenshots because it is generally not ideal.
```

Why: filler terms and weak qualifiers obscure the action.
Cues: `really`, `in most cases`, `generally`.

### `acceptable` examples

6. API reference

```markdown
`POST /v1/releases` creates a release record and returns the immutable
release id and checksum metadata.
```

Why: concise and specific technical description.

7. Deployment runbook

```markdown
If canary error rate exceeds 2 percent for 5 minutes, roll back to the
previous image and page the on-call engineer.
```

Why: clear condition and clear action with required precision.

8. Security control

```markdown
Tokens expire after 15 minutes, and refresh tokens are revoked when a
password reset event is processed.
```

Why: short, concrete, and operationally complete.

9. Compliance note

```markdown
Audit logs must retain actor id, action, and timestamp for 365 days to
meet retention requirements.
```

Why: legal requirement with exact fields and period.

10. Migration guide

```markdown
Rename `config.timeout_ms` to `config.timeout`, keep values in
milliseconds, and run `mdsmith check .` before merging.
```

Why: direct instruction with no redundant framing.
