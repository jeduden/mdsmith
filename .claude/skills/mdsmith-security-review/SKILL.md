---
name: mdsmith-security-review
description: >-
  Security-review methodology specialized for mdsmith - the Go
  Markdown linter/formatter and its surfaces (CLI binary, the mdsmith
  lsp language server, the VS Code extension, the Obsidian plugin, the
  npm/PyPI/Homebrew/Flatpak distribution wrappers, and the Git
  merge-driver/pre-merge-commit hooks). Use this skill whenever the
  user asks for a security review, threat assessment, vulnerability
  audit, or PR security sign-off on mdsmith or any of its components -
  even when they name only one surface (e.g. review the VS Code
  extension, audit the LSP server, check this PR for security
  problems). Also use when reviewing code that parses untrusted
  Markdown, resolves the include and catalog path directives,
  statically lints the build recipe directive, or wires mdsmith into
  an editor or CI pipeline. Prefer this over a generic code review for
  anything mdsmith-related: it encodes the project's attack surface
  and current defenses so reviews stay consistent and accurate.
---

# mdsmith Security Review

A repeatable methodology for finding real security defects in
mdsmith. mdsmith is unusual for a linter:

- it **resolves file paths from document content** (`<?include?>`,
  `<?catalog?>`);
- it **statically lints shell recipes** declared in config
  (`<?build?>` / MDS040);
- it **runs the same engine inside editors and Git hooks**.

So a hostile Markdown file or repository — not just hostile network
input — is a real threat actor. Most interesting bugs live at those
trust boundaries, not in the lint rules.

Note up front — and verify, don't trust. In current mdsmith,
**recipes are *not* executed by the tool**. `<?build?>` renders a
body template, and MDS040 only lints the recipe string. The actual
build runs in external tooling.

The review's job is to confirm that the §0 baseline defenses in
`references/threat-model.md` still hold. Look for regressions or
gaps — do not presume an RCE that the design currently prevents.

## When to use which mode

This skill runs in two modes. Decide first; if unclear, ask.

- **Repo audit** — review the whole codebase (or a named surface).
  Broad, methodical, every threat area in scope. Use for "audit
  mdsmith", "review the Obsidian plugin", a release sign-off.
- **PR / diff review** — review only what changed. Map the diff to
  the threat areas it touches, then review those in depth plus their
  blast radius. Use for "is this PR safe to merge?".

## Workflow

1. **Establish scope and get the code.** Confirm mode. For a PR, get
   the diff (`git diff <base>...<head>` or the PR files); for an
   audit, list the tree (`git ls-files`). Never review from the
   README alone — read the actual source.

2. **Map scope to surfaces.** Identify which of the surfaces below
   are in scope. For a PR, map each changed file to a surface; a
   one-line change in recipe-execution code outweighs a thousand
   lines of rule tweaks.

   | Surface               | Where it lives (verify, don't assume)                  | Primary concern                                                 |
   | --------------------- | ------------------------------------------------------ | --------------------------------------------------------------- |
   | Directive engine      | `internal/` (directive/include/build/catalog handling) | Recipe-execution *regression*, path traversal, SSRF-via-include |
   | CLI core & parser     | `cmd/`, `internal/`, `pkg/`                            | Panic/DoS, ReDoS, OOM, path handling                            |
   | LSP server            | `mdsmith lsp` impl in `internal/`/`pkg/`               | Trust-on-open, fix-on-save side effects                         |
   | VS Code extension     | `editors/` (TypeScript)                                | Workspace Trust, binary spawn, arg/setting injection            |
   | Obsidian plugin       | `editors/` (TS, full Node access)                      | Vault trust, command spawn, network                             |
   | Distribution wrappers | `npm/`, `python/`, Homebrew/Flatpak, `.github/`        | Supply chain, postinstall, signing/verification                 |
   | Git integration       | `merge-driver`, `pre-merge-commit`, `.gitattributes`   | Hostile-repo delivery of recipe execution                       |

3. **Walk the threat model.** Read `references/threat-model.md` and
   work through every threat area that touches your scope. It lists
   the concrete questions to answer and the code patterns to grep
   for, per surface. **Read this file before forming conclusions** —
   it is the core of the skill.

4. **Record findings in the structured model.** Capture each finding
   as a JSON object per the schema in `references/output-formats.md`.
   One finding = one defect. Assign severity using the rubric below
   and record `confidence` honestly (`confirmed` only if you traced
   the code path or built a repro; otherwise `likely` / `tentative`).

5. **Emit the three outputs.** From the single `findings.json`,
   render the report, the SARIF, and the inline annotations. Use the
   script — do not hand-write SARIF:

   ```bash
   go run ./cmd/mdsmith-secreview render findings.json --out-dir <dir>
   ```

   It writes `security-review.md`, `findings.sarif`, and
   `inline-annotations.json`; see `references/output-formats.md` for
   the schema and what each output is for.

6. **Summarize honestly.** Lead with the highest-severity confirmed
   findings. Separate confirmed defects from hardening suggestions.
   If you could not reach a conclusion on an in-scope area (e.g. you
   couldn't find the recipe-execution code), say so explicitly
   rather than implying it's clean.

## Severity rubric

Anchor severity to mdsmith's real-world trust boundaries, not to
abstract CVSS alone.

- **Critical** — code execution or equivalent with **zero or
  near-zero interaction**, e.g. *if a regression introduced*
  recipe/command execution reachable from editor fix-on-save on
  opening an untrusted repo/vault without a trust prompt, or from
  the merge driver during a routine `git merge`. (Current code
  executes no recipes — a finding here means that property broke.)
- **High** — code execution requiring a normal action the user
  wouldn't see as dangerous (e.g. a regression making `mdsmith fix`
  execute something on a freshly cloned repo); arbitrary file
  read/write **outside the workspace** via include/catalog traversal
  or a symlink escape that gets past the default-deny.
- **Medium** — denial of service (parser panic, ReDoS, unbounded
  memory) on attacker-supplied Markdown; information disclosure of
  in-workspace data the user didn't intend to expose; weakened
  supply-chain verification.
- **Low** — issues needing unusual config or yielding limited
  impact; missing defense-in-depth on an already-guarded path.
- **Informational** — hardening, missing tests around a security
  control, unsafe-by-default options that are documented.

When unsure between two levels, state both and explain the deciding
factor (interaction required, trust prompt present, escape
confirmed).

## Reference files

- `references/threat-model.md` — the mdsmith-specific attack
  surface, organized by surface, with concrete review questions and
  grep targets. **Always read before concluding.**
- `references/output-formats.md` — finding JSON schema,
  severity→SARIF mapping, report layout, and inline-annotation
  format.

## Operating principle

Be a careful auditor, not a vibes-based one. Trace the actual code
path before calling something exploitable; mark confidence
accordingly. A precise "here is the call chain from `fix` to
`exec.Command`, with no trust gate" is worth more than a list of
speculative worries. And never write a working exploit for
distribution — a short repro sketch sufficient to prove the defect
is the goal.
