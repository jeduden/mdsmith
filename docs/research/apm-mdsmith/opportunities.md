---
summary: >-
  The merged APM opportunity catalogue: every place mdsmith can
  support, improve, or augment an APM workflow, tagged with status
  (exists / partial / new), effort, trigger, and a mini-plan.
---
# APM opportunity catalogue

This is the merged output of the two-axis sweep in
[workflows.md](workflows.md) (axis 1) and
[subcommands.md](subcommands.md) (axis 2), cross-referenced against
a full map of mdsmith's shipped features. Each opportunity carries:

- a **status** — `exists` (config alone, today), `partial` (a small
  tweak away), or `new` (a new mdsmith feature);
- an **effort** — S (a day), M (a week), L (a month);
- a **trigger** — the condition under which building it is worth
  the cost;
- a **mini-plan** — goal, sketch, and the surface a user sees.

The method mirrors
[learn-from-mdbase.md](../mdbase-vs-mdsmith/learn-from-mdbase.md):
this is a map of candidates, not a roadmap. Five of these have
plan files (linked inline); the rest wait on their triggers.

## Summary tables

### Author a package (A)

| ID  | Opportunity                                        | Status | Effort | Trigger                                              |
| --- | -------------------------------------------------- | ------ | ------ | ---------------------------------------------------- |
| A-1 | APM primitive kind pack (schemas for the 4 types)  | exists | S–M    | first author wants their `.apm/` tree linted         |
| A-2 | Numeric body budgets per primitive kind            | exists | S      | folds into A-1                                       |
| A-3 | `${input:name}` placeholder token                  | new    | S      | first prompt body false-positives on a content rule  |
| A-4 | Closed frontmatter (flag keys `apm compile` drops) | new    | M      | an author ships a key `apm compile` drops at compile |
| A-5 | Filename ↔ frontmatter-field agreement             | new    | M      | a `SKILL.md` `name` drifts from its directory        |
| A-6 | Prompt-input declaration/usage cross-check rule    | new    | M      | a typo'd `${input:Scope}` reaches an LLM verbatim    |
| A-7 | Dead `applyTo` glob detection                      | new    | M      | an instruction compiles to a rule no file activates  |

### Compile and coexist (C)

| ID  | Opportunity                                         | Status  | Effort | Trigger                                         |
| --- | --------------------------------------------------- | ------- | ------ | ----------------------------------------------- |
| C-1 | Foreign managed-region protection (`apm:start/end`) | new     | M      | `mdsmith fix` corrupts an APM-managed block     |
| C-2 | `mdsmith init --apm` template + coexist guide       | partial | S      | first repo runs both tools and hits fix-vs-hash |
| C-3 | Token/size budget canary on compiled context files  | exists  | S      | an assembled `AGENTS.md` blows a context window |
| C-4 | Wrap `apm compile` in a `<?build?>` recipe          | exists  | S      | a team wants edit-time compile freshness        |

### Consume and link (L)

| ID  | Opportunity                                          | Status | Effort | Trigger                                        |
| --- | ---------------------------------------------------- | ------ | ------ | ---------------------------------------------- |
| L-1 | Consumer-side content vetting of deployed primitives | exists | S      | folds into C-2                                 |
| L-2 | Uninstall/prune dangling-reference gate              | exists | S      | an `apm uninstall` orphans an incoming link    |
| L-3 | Drift-gated `<?catalog?>` index of installed skills  | exists | S      | a repo wants a live table of what it installed |

### Govern and gate in CI (G)

| ID  | Opportunity                            | Status  | Effort | Trigger                                                  |
| --- | -------------------------------------- | ------- | ------ | -------------------------------------------------------- |
| G-1 | mdsmith Action beside `apm audit --ci` | exists  | S      | a repo wants a content gate next to the integrity gate   |
| G-2 | SARIF output for `mdsmith check`       | partial | S–M    | a team wants mdsmith findings in Code Scanning           |
| G-3 | Hidden-Unicode rule                    | new     | M      | an author wants the APM install-block check at edit time |
| G-4 | Migration worklist via MDS037 + MDS033 | exists  | S      | a team migrates hand-copied instructions into `.apm/`    |

## Mini-plans

### A-1 — APM primitive kind pack (exists)

**Goal.** Ship a documented set of mdsmith kinds so an APM author
runs `mdsmith check .apm/` and gets the front-matter and size
contracts APM's own docs state but never enforce.

**Sketch.** Four kinds keyed by `path-pattern`: `apm-skill`
(`.apm/skills/*/SKILL.md` — `name: nonEmpty`, `description:
nonEmpty`), `apm-prompt` (`.apm/prompts/*.prompt.md` —
`description: nonEmpty`, optional `input`, `allowed-tools`,
`model`, `argument-hint`), `apm-instruction`
(`.apm/instructions/*.instructions.md` — `description` + `applyTo`),
`apm-agent` (`.apm/agents/*.agent.md` — `name`, `description`,
`model`, `tools`). This is the exact pattern the repo already runs
for its own `skill` and `plan` kinds. A-2 layers `max-file-length`
and `token-budget` per kind (`SKILL.md` 500 lines / 5000 tokens,
agents 300 lines).

**Surface.** The `.mdsmith/kinds/apm-*` files that `mdsmith init
--apm` writes (C-2). Ships together with C-2's template and guide as
[plan 2607082050](../../../plan/2607082050_apm-coexist-guide-and-kind-pack.md).

### A-3 — `${input:name}` placeholder token (new)

**Goal.** Let content rules treat APM's `${input:name}` prompt
parameters as opaque, the way `var-token` already handles
`{identifier}`.

**Sketch.** `var-token` matches only `{identifier}` (verified at
`internal/placeholders/placeholders.go`), so `${input:pr_url}` is
opaque to nothing: MDS023, MDS024, MDS018, and MDS004 all see it as
prose. Add one token to the closed vocabulary matching
`${input:NAME}` with `NAME` on APM's documented grammar
`[A-Za-z][\w-]{0,63}`, wired into `ContainsBodyToken` and
`MaskBodyTokens`. Small, self-contained, TDD-shaped.

**Surface.** A new token name in `placeholders:` and a row in
[placeholder-grammar.md](../../background/concepts/placeholder-grammar.md).
Scheduled as
[plan 2607082048](../../../plan/2607082048_apm-input-placeholder-token.md).

### A-4 / A-5 — schema grammar extensions (new)

**Goal.** Express two APM contracts the schema engine cannot today:
frontmatter that rejects unknown keys, and a filename that must
match a frontmatter field.

**Sketch.** (a) A `frontmatter-closed: true` schema flag that makes
MDS020 diagnose any frontmatter key not declared in the
`frontmatter:` map — `closed:` exists but applies only to sections
(`docs/reference/section-schema.md:243`), and on a frontmatter-only
kind it is dropped. This catches the sixth `.prompt.md` key at edit
time, where `apm compile` flags it only at compile time, after the
file ships. (b) `\#(fmvar(name))` interpolation inside the
schema `path-pattern:` / `filename:` matcher, reusing the existing
`fmvar` resolver that already works in heading regexes — so
`path-pattern: ".apm/skills/\#(fmvar(name))/SKILL.md"` enforces
APM's "`name` equals directory name" rule.

**Surface.** Two schema keys; both extend MDS020. Scheduled as
[plan 2607082051](../../../plan/2607082051_apm-schema-extensions.md).

### C-1 — foreign managed-region protection (new)

**Goal.** Make `mdsmith fix` and the style rules safe inside a
region another generator owns — starting with APM's
`<!-- apm:start -->` … `<!-- apm:end -->` managed section.

**Sketch.** mdsmith's generated-section engine recognizes only its
own `<?directive?>` markers. When APM writes a managed section into
`AGENTS.md`, `mdsmith fix` reflows the bytes inside it (table
alignment, trailing whitespace), which breaks the SHA-256 APM
pins in `apm.lock.yaml` and trips `apm audit --ci`. Add a
`foreign-regions:` config listing `{start, end}` marker pairs
(glob-scopable via overrides): fixers never rewrite bytes inside a
declared region, style/content rules skip diagnostics there, but
whole-file rules (MDS022, MDS028) still count the bytes — mirroring
how generated bodies are already handled. A companion check
diagnoses malformed pairs (zero or multiple markers).

**Surface.** A `foreign-regions:` top-level key. Scheduled as
[plan 2607082049](../../../plan/2607082049_foreign-managed-regions.md).

### C-2 — coexist-with-APM guide + preset (partial)

**Goal.** One guide that tells a repo running both tools exactly
which globs mdsmith must leave alone and which kinds to apply to
the `.apm/` source it *should* lint.

**Sketch.** Model it on
[coexist-with-prettier.md](../../guides/coexist-with-prettier.md).
Ship the canonical `ignore:` list naming APM's deploy dirs
(`.github/prompts/**`, `.github/instructions/**`, `.claude/rules/**`,
`.agents/skills/**`, `.kiro/steering/**`, `.windsurf/rules/**`,
`apm_modules/**`) so `mdsmith fix` never rewrites a hash-pinned
file, plus `overrides:` disabling fix-capable rules on the compiled
root files, plus the A-1 kind pack for the `.apm/` source tree. The
config exists today; the command that assembles it does not. So the
plan ships it as an `mdsmith init --apm` template — additive and
non-clobbering, on the `--wordlists` model — that writes the
`.mdsmith/kinds/apm-*` pack and the coexistence posture, scoped to
the harness directories the repo actually has. The guide documents
what the template writes.

**Surface.** An `mdsmith init --apm` flag plus
`docs/guides/coexist-with-apm.md`. Scheduled as
[plan 2607082050](../../../plan/2607082050_apm-coexist-guide-and-kind-pack.md).

### G-2 — SARIF output for `mdsmith check` (partial)

**Goal.** Let mdsmith diagnostics land in the same GitHub Code
Scanning dashboard APM's `apm audit --ci -f sarif` already feeds.

**Sketch.** `mdsmith check` emits text or JSON only
(`docs/reference/cli/check.md`). mdsmith already renders SARIF in
`internal/release/auditsarif.go` and `internal/secreview/sarif.go`
for the security-review engine, so the emission machinery exists —
it is not wired to the check command's diagnostic stream. Add
`-f sarif`: one `reportingDescriptor` per fired rule (`ruleId =
MDS###`, `helpUri` to the rule doc), one result per diagnostic with
a `physicalLocation` region. Generalize the existing SARIF structs
into a shared internal package.

**Surface.** A new `-f sarif` value on `check` (and `fix
--dry-run`). Scheduled as
[plan 2607082052](../../../plan/2607082052_check-sarif-output.md).

### G-3 — hidden-Unicode rule (new)

**Goal.** Catch the bidi-override, tag-character, and zero-width
payloads APM blocks at install time — but at edit time, in the
editor, before the file is ever committed.

**Sketch.** APM's only content check is the install/audit-time
hidden-Unicode scan; `apm compile` does not run it after hand-edits,
and detection happens only when an APM command runs. mdsmith has no
rule here (verified: nothing under `internal/rules/` scans for
invisible characters). A new default-enabled rule mirroring APM's
three severity tiers (critical: tag chars, bidi overrides,
variation selectors 17–256; warning: zero-width, VS 1–15; info:
NBSP, ZWJ), with a `Fix()` that strips flagged characters —
delivered live through `mdsmith lsp` and the autofix hook. This
overlaps APM's scanner deliberately: the value is edit-time
delivery, not novelty. Not yet scheduled; awaits the trigger.

**Surface.** A new `no-hidden-unicode` rule (next free id, MDS071).

## What does not fit

Some APM surfaces have no mdsmith angle and are recorded here so
the sweep is complete: the `apm.yml` / `apm.lock.yaml` /
`apm-policy.yml` manifests (YAML, not Markdown — outside mdsmith's
domain), executable-trust lists (`apm approve`/`deny`), lifecycle
scripts, runtime-CLI management (`apm runtime`), the SBOM export,
and the cache/config/self-update plumbing. The marketplace
`packages[]` block is YAML too; only the package `README.md` it
points at is mdsmith's concern (covered by the repo's existing
docs-quality stack).
