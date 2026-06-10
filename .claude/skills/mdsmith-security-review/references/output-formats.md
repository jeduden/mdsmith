# Output Formats

All three outputs are rendered from one **findings file** by the
`mdsmith-secreview render` command. Author that one file; never
hand-write SARIF.

## File layout

Each review owns a directory `docs/security/<YYYY-MM-DD-slug>/`
holding four fixed-name files:

```text
docs/security/<stem>/findings.json             # authored input
docs/security/<stem>/report.md                 # rendered report
docs/security/<stem>/findings.sarif            # rendered SARIF
docs/security/<stem>/inline-annotations.json   # rendered annotations
```

Render with `--out-dir docs/security/<stem>/`. The directory — not a
filename stem — namespaces each review, so the basenames are fixed
and a later review never overwrites an earlier one. `SECURITY.md`'s
`<?catalog?>` over `docs/security/*/report.md` indexes the report;
run `mdsmith fix SECURITY.md` after rendering. The `security-note`
kind validates `report.md` against `docs/security/proto.md`.

## The finding object

The findings file is `{"target": {...}, "findings": [ <finding>, ... ]}`.

```json
{
  "target": {
    "mode": "pr",                         // "pr" or "audit"
    "repo": "jeduden/mdsmith",
    "ref": "abc1234",                     // commit / PR head, for traceability
    "scope": "VS Code extension + LSP"    // free text
  },
  "coverage": "what was and wasn't reviewed; any in-scope area left inconclusive. Optional top-level string; renders as the report's Coverage note. Omit it and the note falls back to a placeholder.",
  "findings": [
    {
      "id": "S001",                       // stable within this review; S001, S002, ...
      "title": "fix-on-save runs <?build?> recipes in untrusted workspace",
      "severity": "critical",             // critical | high | medium | low | info
      "confidence": "confirmed",          // confirmed | likely | tentative
      "surface": "vscode",                // directive|cli|lsp|vscode|obsidian|supplychain|git
      "cwe": "CWE-94",                    // optional; one or more, comma-separated string
      "location": {
        "file": "editors/vscode/src/extension.ts",
        "startLine": 142,
        "endLine": 168                    // optional; defaults to startLine
      },
      "related_locations": [              // optional, for multi-site call chains
        {"file": "internal/build/run.go", "startLine": 51}
      ],
      "description": "Plain-language explanation of the defect and the code path.",
      "impact": "What an attacker achieves (e.g. RCE on victim machine when they open the repo).",
      "repro": "Minimal sketch proving it — NOT a weaponized exploit. e.g. a repo layout + the one fix that triggers it.",
      "remediation": "Concrete fix (e.g. gate behind Workspace Trust; pass argv array, never shell)."
    }
  ]
}
```

Rules:

- One defect per finding. Don't merge "traversal in include" and
  "traversal in catalog" unless they are literally the same code
  path.
- `confidence: confirmed` only if you traced the call chain or
  built a repro. Otherwise use `likely` (strong code evidence, one
  gap) or `tentative` (worth flagging, unverified).
- Keep `repro` a sketch — enough to prove the bug, never a drop-in
  attack.

## Output 1 (canonical, machine-readable): `findings.sarif`

SARIF 2.1.0, compatible with GitHub code scanning. The renderer
maps:

| severity | SARIF `level` | `security-severity` (GitHub) |
| -------- | ------------- | ---------------------------- |
| critical | error         | 9.5                          |
| high     | error         | 8.0                          |
| medium   | warning       | 5.5                          |
| low      | note          | 3.0                          |
| info     | note          | 0.0                          |

Each finding becomes a `result` with a `ruleId` (its `id`), a
message, and a physical location (file + region). Each distinct
`id` becomes a `rule` in `tool.driver.rules`. That rule carries the
title, the CWE (as a `properties.tags` entry), and the
`security-severity`. `confidence` and `severity` are recorded in
result `properties`.

## Output 2 (human report): `report.md`

Layout the renderer produces:

1. **Header** — target, mode, ref, scope, date.
2. **Summary table** — counts by severity, then one row per
   finding (id, severity, confidence, title, surface, location).
3. **Findings** — one section each, ordered by severity then
   confidence. Each carries the title, the
   severity/confidence/CWE/surface line, the location(s), the
   description, the impact, the repro sketch, and the remediation.
4. **Hardening / informational** — `info`-severity items grouped
   separately so they don't dilute real findings.
5. **Coverage note** — what was and wasn't reviewed, plus any
   in-scope area left inconclusive.

## Output 3 (PR review): `inline-annotations.json`

A flat list for posting as PR review comments, keyed to file +
line:

```json
[
  {"path": "editors/vscode/src/extension.ts", "line": 142, "side": "RIGHT",
   "severity": "critical", "id": "S001",
   "body": "**[S001 · critical] <title>**\n\n<description>\n\n**Fix:** <remediation>"}
]
```

Only findings with a concrete `location` produce annotations. In PR
mode these map directly to GitHub review comments. In audit mode
they are still useful as an editor-navigable list. Keep each `body`
self-contained — severity, what, why, fix — since reviewers read
them out of context.
