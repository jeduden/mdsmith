# mdsmith Threat Model & Review Checklist

Read this in full before concluding a review. It is
**question-first**: each area states the trust boundary, the
questions to answer *from the source*, and grep targets to find the
code. It also records what the **current codebase does** at each
boundary (as of the last calibration). Do **not** treat those notes
as permanent truth — they are the expected-secure baseline, and your
job is to confirm they still hold and look for regressions or gaps.
Never report a defect you have not traced in the code in front of
you.

The central attacker is a **hostile Markdown file or repository**.
The victim clones it, opens it in an editor, or runs mdsmith against
it. Treat all of these as attacker-controlled:

- document content and front matter;
- file paths;
- `.mdsmith.yml`, `.vscode/settings.json`, and `.gitattributes`.

---

## 0. Known current defenses — confirm these haven't regressed

These are the load-bearing security properties of mdsmith's design.
A review's first job is to verify each still holds; a change that
breaks one is likely your highest-severity finding.

- **Recipes are not executed by mdsmith.** `<?build?>` renders a
  body *template*; the recipe command is run by external tooling
  (CI/Make), never by mdsmith. → confirm no new `exec` path.
- **Recipe commands come from config, not documents.** The directive
  supplies a recipe *name* and params; the command lives in
  `build.recipes`. → confirm document content can't supply a command.
- **Build `output` paths are validated** (relative only, no `..`, no
  absolute, no drive letter).
- **Symlinks are default-deny** during workspace
  traversal/resolution.
- **VS Code extension honors Workspace Trust**
  (`untrustedWorkspaces: "limited"`; `mdsmith.path` and
  `mdsmith.config` are restricted until the workspace is trusted).
- **npm/PyPI wrappers have no postinstall download-then-exec**
  (per-platform binaries ship as package dependencies).

If any of these is now false, that is the finding — rate it via the
ladder in the last section.

---

## 1. Directive engine

### 1a. `<?build?>` and recipe handling

Baseline: **mdsmith does not execute recipes.** The historical "fix
runs a shell recipe → RCE" fear does not apply to current code. So
the questions are about *regression* and *secondary* paths:

- Is there now **any** code path that executes a recipe command (a
  new `exec.Command`/`sh -c` reachable from `fix`, the engine, the
  LSP, or the merge driver)? If so, this is the headline finding —
  rate by interaction required (zero-interaction → Critical).
- Can **document content** now influence the recipe command (not
  just the recipe name/params)? A change that lets a directive
  supply or template the command string reopens injection.
- Is recipe **output** ever written back into files in a way that
  could enable secondary injection or path escape?
- What does MDS040 (`recipesafety`) actually check, and is it still
  advisory-only? Note for the report: MDS040 is a lint-time
  advisory, not a runtime sandbox — mdsmith's guarantee ends at the
  external runner. Flag if docs imply otherwise.

Grep targets: `exec.Command`, `exec.CommandContext`, `os/exec`,
`sh -c`, `/bin/sh`, `recipe`, `Recipe`, `MDS040`, `recipesafety`,
`generateBody`, `Generate(`, `Fix(`.

### 1b. `<?include?>` / `<?include extract:?>` / `<?catalog?>` path resolution

Baseline: symlinks are default-deny and build output is
containment-checked. Verify the same rigor for include/catalog:

- Is the include/catalog path constrained to the workspace root with
  a **containment check after `filepath.Clean`** (a prefix check
  alone is insufficient)? Confirm absolute paths are rejected.
- Is the symlink default-deny applied on **these** paths too, or
  only the top-level walk? A symlink reached *through* an include is
  the gap to look for.
- `<?catalog glob:?>`: can the glob escape the workspace or be
  steered to read sensitive files into generated output
  (disclosure)?
- Include depth/cycles: is there a recursion/depth limit? Self/mutual
  include → infinite loop/OOM.
- Does any include path accept a URL scheme (SSRF)? Confirm not.
- `extract:` source paths get the same containment treatment?

Grep targets: `include`, `catalog`, `filepath.Join`,
`filepath.Clean`, `filepath.Abs`, `EvalSymlinks`, `filepath.Rel`,
`HasPrefix`, `Glob`, `WalkDir`, `ReadFile`, `os.Open`.

### 1c. CUE / YAML evaluation

- `list query` evaluates a **CUE expression on front matter**. Is
  evaluation bounded (time/memory)? Are filesystem/network CUE
  builtins disabled so a hostile expression can't read external
  files?
- YAML front matter: safe loader? Anchors/aliases enabling "billion
  laughs" → OOM? Custom tags → type confusion?

Grep targets: `cuelang`, `cue.`, `yaml.Unmarshal`,
`yaml.NewDecoder`, `msgpack`, `query`, `extract`.

---

## 2. CLI core, workspace walk & parser robustness

Baseline: symlink default-deny exists. Remaining questions are about
robustness and write-safety:

- **Workspace walk**: confirm the default-deny actually covers walk
  + resolve; check `--no-gitignore` (ignore rules are not a security
  boundary). Look for a symlinked dir causing an infinite walk.
- **In-place rewrites** (`fix`, `rename`): can a crafted heading/slug
  cause writes outside intended files? Is the atomic write safe
  against a symlinked target (writing through a symlink to clobber
  `~/.bashrc`)? Note rename rewrites workspace anchor links in one
  edit — check its path math.
- **Parser DoS**: readability/sentence/line-length rules are
  regex-heavy — look for nested quantifiers / catastrophic
  backtracking (ReDoS) on user input. Where is `recover()` — is a
  panic on one file contained or does it crash the whole run / LSP?
- **Resource limits**: any cap on file size, file count, include
  fan-out, token budget? Multi-GB input or include fan-out → OOM.

Grep targets: `regexp.MustCompile`, `WalkDir`, `filepath.Walk`,
`Lstat`, `recover()`, `panic(`, `io.ReadAll`, `os.WriteFile`,
`Rename`, `O_CREATE`.

---

## 3. LSP server (`internal/lsp`, `mdsmith lsp`)

Trust-on-open boundary — the editor opens a whole workspace before
any explicit command. **Largely unverified at last calibration —
treat as open.**

- On `initialize`/`didOpen`/`didChangeWatchedFiles`, what runs
  automatically? Diagnostics (read-only) only, or anything resolving
  includes/catalogs or writing files?
- Does **fix-on-save** / `workspace/executeCommand` quick-fix reach
  any file write outside the open document, or (post-regression)
  recipe execution? Trace the command handlers.
- Does a panic handling one document take down the whole server
  (editor-session DoS)?

Grep targets: `Initialize`, `DidOpen`, `DidSave`, `DidChange`,
`executeCommand`, `CodeAction`, `applyEdit`, `quickfix`.

---

## 4. VS Code extension (`editors/vscode`, TypeScript)

Baseline defenses here:

- the extension declares `untrustedWorkspaces: "limited"`;
- `mdsmith.path` and `mdsmith.config` are restricted until trust;
- mutating commands are disabled in restricted mode.

Verify each still holds and find the gaps:

- Does the restricted-mode boundary actually gate **every** mutating
  path (fix-workspace, merge driver install, init), or can one slip
  through before trust?
- **Binary resolution**: with `mdsmith.path` restricted, confirm a
  workspace `.vscode/settings.json` truly cannot redirect the binary
  pre-trust. Check how the path is read (`binary.ts`, `wiring.ts`).
- **Argument construction**: `spawn` with an argv array vs
  `exec`/`shell: true` with interpolated paths? Flag any
  shelling-out.
- Activation: does anything run on activation before the user acts?

Grep targets: `child_process`, `spawn`, `execFile`, `shell: true`,
`untrustedWorkspaces`, `isTrusted`, `workspace.getConfiguration`,
`mdsmith.path`, `onSave`, `activationEvents`.

---

## 5. Obsidian plugin (`editors/obsidian`, TypeScript)

The plugin has full Node access inside the vault. In synced or
downloaded vaults, treat the vault contents as attacker-controlled.
This area is unverified at last calibration — treat it as open. The
plugin appears to run the engine via **WASM** (`wasm-runtime.ts`).
That shifts the model away from §4's binary-spawn:

- If execution is WASM in-process, the spawn/argv concerns shift to:
  what can the WASM module touch (filesystem adapter scope), and can
  vault content steer reads/writes outside the vault?
- Does the plugin auto-run fix on open/save without a trust prompt?
  Obsidian has no Workspace-Trust equivalent — so any mutating
  auto-run on untrusted vault content is worth scrutiny.
- Any outbound network (update check/telemetry) vs the stated
  policy?
- Manual-unzip install (no marketplace review) is a distribution
  note (§6), not plugin-code risk.

Grep targets: `wasm`, `WebAssembly`, `instantiate`, `adapter`,
`vault.`, `child_process`, `fetch(`, `onload`, `registerEvent`.

---

## 6. Distribution & supply chain

Baseline defenses here:

- the npm and PyPI wrappers have no postinstall download-then-exec;
- releases are Sigstore-signed and checksummed;
- npm and PyPI publish via OIDC Trusted Publishing.

Verify each, then probe CI:

- Confirm the npm wrapper still has **no** `postinstall`/`preinstall`
  script and ships binaries as package deps (not a runtime
  download). If a download is added, is it checksum+signature
  verified over TLS before exec?
- **Release pipeline** (`.github/workflows/release.yml`): does it
  actually sign/checksum? Are third-party Actions pinned by **SHA**
  (not floating tags)? Are workflow `permissions:` least-privilege
  (no broad `write-all`)?
- Any `pull_request_target` that checks out untrusted head **and**
  has a privileged token (classic CI RCE)?
- `go.sum` complete; no `replace` directives pointing at untrusted
  forks.

Grep targets: `postinstall`, `preinstall`, `package.json` scripts,
`setup.py`, `pyproject.toml`, `.github/workflows/*.yml`,
`pull_request_target`, `permissions:`, `uses:`, `cosign`,
`sigstore`, `sha256`.

---

## 7. Git integration (merge driver & hooks)

A hostile repo can ship `.gitattributes` to wire the merge driver.
Baseline: the merge driver re-runs the **directive** (template
regeneration), which — given recipes aren't executed — is text
regeneration, not command execution. Verify that remains true:

- Does the merge-driver / pre-merge-commit path reach **any**
  execution sink, or only the gensection template engine? If a
  regression makes directive regeneration execute something, a
  routine `git merge` becomes zero-interaction execution → Critical.
- The `exec.Command` calls in `mergedriver.go`/`premergecommit.go`
  should all be `git`/`go` with constant args — confirm none
  interpolate attacker content.
- Does any Git-path execution bypass the CLI's containment (symlink
  deny, output validation)?

Grep targets: `merge-driver`, `mergedriver`, `pre-merge-commit`,
`premergecommit`, `gitattributes`, `exec.Command`,
`BuildHookScript`.

---

## Cross-cutting: the severity ladder

For each candidate, the decisive questions are always: **is there a
trust gate, and is a dangerous sink actually reachable from
attacker-controlled input?** Trace the path before you rate it.

1. Execution/command sink reachable **without user interaction**
   (editor open, `git merge`) → Critical.
2. Same reachable via a routine command (`fix`, `merge`) on freshly
   obtained content → High.
3. File read/write **outside the workspace** (traversal/symlink
   escape) → High (write) / Med–High (read).
4. DoS on hostile input (panic/ReDoS/OOM) → Medium.
5. Weakened/absent supply-chain verification → Medium.

A regression that removes one of the §0 defenses inherits the
severity of the attack it re-enables.
