# Evals for mdsmith-security-review

These cases check that the skill stays calibrated. It must confirm
the project's existing defenses without inventing findings. It must
also still catch a real defect when one is introduced.

## How to run (manual)

There are no background agents in chat, so run each case yourself:

1. Clone the target so the skill reviews real source:

   ```bash
   git clone --depth 1 https://github.com/jeduden/mdsmith.git
   ```

2. Start a fresh session with this skill available. Give it the
   case `prompt`.
3. Follow the skill exactly: read `SKILL.md`, then
   `references/threat-model.md`, trace the code, record
   `findings.json`, and run `scripts/render_findings.py`.
4. Grade the result against the case's `expect.must` and
   `expect.must_not`.

A case passes only if every `must` holds and no `must_not` occurs.
One `must_not` matters most across all cases: do not report recipe
execution or RCE without a traced `exec` call path in the code
under review.

See `cases.yaml`.
