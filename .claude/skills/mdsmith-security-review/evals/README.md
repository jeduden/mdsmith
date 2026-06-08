# Evals for mdsmith-security-review

These cases check that the skill stays calibrated. It must confirm
the project's existing defenses without inventing findings. It must
also still catch a real defect when one is introduced.

Everything is pinned to one commit — `cases.yaml`'s `baseline_ref`.
So a run is reproducible. The code paths, the §0 defenses, and the
regression patch all match that commit.

## Two layers

The eval has a mechanical layer and a human-judged layer.

- **Mechanical** — the Go eval tests (`internal/secreview`) enforce
  what is derivable from `findings.json`: forbidden severities, a
  required finding at the sink, the renderer round-trip, and that the
  regression patch still applies. CI runs this on every PR.
- **Human-judged** — the prose `expect.must` / `expect.must_not` in
  `cases.yaml` (for example "reads threat-model.md first", "names
  residual un-traced areas"). These still need a person or an LLM to
  grade.

## Deterministic harness (what CI runs)

The eval is a set of Go tests in `internal/secreview`. Run them in
strict mode from the repo root:

```bash
SECREVIEW_STRICT=1 go test ./internal/secreview/... ./cmd/mdsmith-secreview/...
```

They run these checks, all pinned to `baseline_ref`:

- the `cases.yaml` schema;
- the render round-trip and severity-to-SARIF mapping on every
  fixture;
- that the grader passes each golden and rejects the complacent
  negative one;
- that `regressions/build-exec.patch` still applies and compiles at
  `baseline_ref`.

The `skill-eval` CI job (`.github/workflows/skill-eval.yml`) runs
exactly this.

## Running a case by hand (the human-judged layer)

Run these from the repo root.

1. Clone the target and check out the pinned baseline:

   ```bash
   git clone https://github.com/jeduden/mdsmith.git
   cd mdsmith
   git checkout <baseline_ref from cases.yaml>
   ```

2. For `pr-regression-introduces-exec`, apply the regression so the
   skill has a real diff to review:

   ```bash
   git apply .claude/skills/mdsmith-security-review/evals/regressions/build-exec.patch
   ```

3. Start a fresh session with the skill available and give it the
   case `prompt`.
4. Follow the skill: read `SKILL.md`, then
   `references/threat-model.md`, trace the code, record
   `findings.json`, and render it:

   ```bash
   go run ./cmd/mdsmith-secreview render findings.json --out-dir out
   ```

5. Score the mechanical bar with the grader, then judge the prose
   `must` / `must_not` yourself:

   ```bash
   go run ./cmd/mdsmith-secreview grade --findings findings.json \
     --cases .claude/skills/mdsmith-security-review/evals/cases.yaml \
     --case <case-id>
   ```

A case passes only if the grader passes and every prose `must` holds
with no `must_not`. One `must_not` matters most: do not report
recipe execution or RCE without a traced `exec` call path.

## Recalibrating

When the skill is recalibrated against newer code, bump
`baseline_ref`, refresh the fixtures, and regenerate
`regressions/build-exec.patch` against the new commit. The eval fails
if the patch no longer applies — that is the signal to do this.

See `cases.yaml`.
