---
title: External build orchestrator spike — Go-only
description: Whether mdsmith should outsource its build engine to
  an existing Go-based orchestrator instead of implementing the
  internal engine described in plans 102-117.
---
# External build orchestrator spike — Go-only

## Scope

mdsmith's `<?build?>` directive turns Markdown into a build
graph: every directive declares `inputs`, `outputs`, and a
recipe. The current plan set (102, 103, 115, 116, 117)
implements an internal engine for this graph: ActionID
content-hash cache, atomic multi-output write, trust gate,
hermetic execution environment, stdout/stderr capture, log
retention, parallel execution.

That is roughly 1 500 lines of plan describing several
hundred lines of Go.

This spike asks: instead of writing all that, can we lean on
an existing Go-based build orchestrator? mdsmith would
expose subcommands the orchestrator drives. The orchestrator
would own staleness, parallelism, and execution.

The spike was constrained to Go-language tools so the
mdsmith user does not have to install a foreign-language
runtime. A separate spike
([language-relaxed.md](language-relaxed.md)) revisits this
constraint.

## Verdict

Stick with the internal engine. Add a single side door.

No Go-native orchestrator offers a meaningfully better deal
than what plans 102-117 already specify. The closest
single-binary candidate (go-task) matches at the surface but
not on the threat model: it has no per-config trust gate, no
hermetic environment, no atomic multi-output write, no
output-confinement post-conditions, and its checksum scope
excludes `generates` (see [go-task issue #238][issue-238]).
Outsourcing the engine moves four of the five gotchas from
the
[gotchas spike](#gotchas-summary) back onto the user.

Recommendation: keep the internal engine; add one new
subcommand (`mdsmith targets --json`) that lets users who
already drive cross-tool DAGs with go-task / make / just
fold mdsmith into their outer orchestration.

[issue-238]: https://github.com/go-task/task/issues/238

## Candidates evaluated

| Tool                                | Maintenance                              | Content-hash                                                        | Parallel                 | Dynamic graph                | Format / API               | Distribution                                                                |
|-------------------------------------|------------------------------------------|---------------------------------------------------------------------|--------------------------|------------------------------|----------------------------|-----------------------------------------------------------------------------|
| [go-task / Taskfile][go-task]       | Active (15.5 k stars, v3.50 Apr 2026)    | Yes (`method: checksum`); scope is sources only ([#238][issue-238]) | Yes (`deps`, --parallel) | Static YAML; can read stdin  | `Taskfile.yml`             | Single binary, brew/apt                                                     |
| [magefile/mage][mage]               | Active (4.7 k stars, v1.17.2)            | Function-binary hash only; file deps via `target` package are mtime | Yes (`mg.Deps`)          | Yes (Go code)                | Go function calls          | `go install`                                                                |
| [goyek/goyek][goyek]                | Active (686 stars, v3.0.1 Dec 2025)      | No                                                                  | Yes (`Task.Parallel`)    | Yes (Go code)                | Go function calls          | `go install` (no separate binary)                                           |
| [ejholmes/walk][walk]               | Dormant (138 stars, v0.3.3 Sep 2017)     | **None — outsourced to the Walkfile script**                        | Yes (semaphore)          | Yes (Walkfile is executable) | Walkfile (executable)      | `go install`                                                                |
| [evmar/n2][n2]                      | Hobby — author labels it "hobbyist code" | No (mtime, ninja-compatible)                                        | Yes                      | Partial                      | `build.ninja`              | `cargo install` (Rust, included for completeness — fails the Go constraint) |
| [benchkram/bob][bob]                | Stale (last commit Mar 2024)             | Yes                                                                 | Yes                      | Static                       | YAML                       | Single binary + Nix                                                         |
| [thought-machine/please][please]    | Active                                   | Yes (Bazel-style)                                                   | Yes                      | Yes (BUILD files)            | BUILD files (Starlark-ish) | Single binary; standalone CLI, not a library                                |
| [dagger][dagger]                    | Active                                   | Yes (BuildKit)                                                      | Yes                      | Yes (CUE / Go)               | CUE / Go SDK               | Pulls Docker; heavy                                                         |
| [buildbarn / bb-storage][buildbarn] | Distributed-build focused                | Yes                                                                 | Yes                      | Yes                          | proto                      | Heavy infrastructure                                                        |

[go-task]: https://github.com/go-task/task
[mage]: https://github.com/magefile/mage
[goyek]: https://github.com/goyek/goyek
[walk]: https://github.com/ejholmes/walk
[n2]: https://github.com/evmar/n2
[bob]: https://github.com/benchkram/bob
[please]: https://please.build/
[dagger]: https://github.com/dagger/dagger
[buildbarn]: https://github.com/buildbarn/bb-storage

### Why each was rejected as the primary engine

- **n2** — Rust, not Go; the [README][n2] explicitly calls
  it hobbyist code from a single author.
- **bob** — last commit March 2024; not a safe runtime
  dependency for a tool we are still planning.
- **please** — standalone CLI, not a library. mdsmith
  cannot embed it; the user would have to install please
  separately and learn its BUILD-file dialect.
- **dagger / buildbarn** — container or distributed build
  focus. Wrong audience for a Markdown linter whose users
  are tech writers and devs.
- **mage / goyek** — both are programmable in Go, both run
  parallel deps, but neither tracks file content hashes.
  mage's `target` helper is mtime-only ([dependencies
  doc][mage-deps]); goyek does no file tracking.
- **walk** — interesting design (no DSL; the Walkfile is
  an executable script that responds to `deps` and `exec`
  phases) and it gives parallel DAG execution via a
  semaphore. But [`plan.go`][walk-plan] does no staleness
  detection at all — that is delegated to the Walkfile
  script. So walk is a parallel DAG runner, not a build
  engine; mdsmith would still own staleness, atomic
  write, trust gate, and post-conditions. The repo is
  also dormant: 138 stars, last release v0.3.3 in
  September 2017. Adopting walk would replace 50 lines
  of `errgroup`-based concurrency with a third-party
  dependency that has not been updated in 8+ years.
- **go-task** — the only serious match. Single binary,
  built-in checksum mode, parallel deps. Detailed below.

[walk-plan]: https://github.com/ejholmes/walk/blob/master/plan.go

[mage-deps]: https://magefile.org/dependencies/

## Why not go-task as the primary engine

go-task gets close but loses on five specific points:

1. **Checksum scope.** go-task hashes `sources:` only;
   `generates:` is excluded ([issue #238][issue-238]).
   Plan 103's staleness step 5 ("hand-edited artifact
   triggers rebuild") cannot be implemented inside
   go-task.
2. **No trust gate.** A fresh `git clone` of a hostile repo
   running `task` invokes every declared command. Plan 117
   exists because mdsmith cannot ship that default.
3. **No hermetic environment.** go-task inherits the
   parent shell environment. Plan 117 specifies an
   allowlisted `PATH` and explicit env pass-through.
4. **No atomic multi-output write.** A failing recipe in
   go-task leaves partial outputs. Plan 115 plus plan 117
   stage every output in a per-recipe temp directory and
   rename atomically; on failure no declared output is
   touched.
5. **No output-confinement post-conditions.** go-task does
   not detect when a recipe writes a file outside its
   declared `generates:`. Plan 117 snapshots the staging
   dir and the output-paths' parents and diffs after the
   recipe.

These are not nice-to-haves. Each addresses a specific
class of gotcha that surfaced in the prior gotchas spike
(see [Gotchas summary](#gotchas-summary)).

## Recommended architecture: internal engine + side door

```text
┌─────────────────────────┐
│ outer orchestrator      │  go-task / make / just / mage
│ (cross-tool DAG)        │  Optional. The user already runs
└──────────┬──────────────┘  this for their docs site.
           │
           ▼
┌─────────────────────────┐
│ mdsmith fix --build-only│  mdsmith owns the inner DAG:
│ (inner DAG)             │  staleness, recipe dispatch,
└──────────┬──────────────┘  trust gate, atomic write.
           │
           ▼
┌─────────────────────────┐
│ user-declared recipes   │  pandoc / mmdc / scripts / …
└─────────────────────────┘
```

mdsmith owns:

- Parsing `<?build?>` directives.
- Resolving `inputs:` and `outputs:` against the project.
- Computing the ActionID and consulting the cache.
- Dispatching recipes via `os/exec` with the trust gate,
  hermetic env, atomic multi-output write, and post-
  conditions.
- The lint-fix pass (rules + generated sections).

The outer orchestrator (optional) owns:

- Coordination across non-Markdown tools (npm, pandoc,
  pytest, etc.).
- Whichever node is named "mdsmith fix" — usually one node
  for `--no-build` (lint-fix) and one for `--build-only`
  (build).

### New mdsmith subcommand: `mdsmith targets --json`

One new subcommand opens the side door. Approximately
50 lines of Go on top of the existing parser:

```bash
$ mdsmith targets --json
[
  {
    "recipe": "pandoc",
    "inputs": ["chapters/intro.md", "chapters/01-prologue.md"],
    "outputs": ["book.html", "book.epub"],
    "source": "README.md",
    "line": 12
  },
  …
]
```

That output is enough for any orchestrator to generate a
Taskfile.yml, a Makefile, or a Justfile that invokes
`mdsmith fix --build-only --build-recipe NAME` per
recipe. Per-target invocation (running a single
directive in isolation) would need a path-based
selector that mdsmith does not yet expose; the side
door treats one recipe as the smallest unit.

### User-facing flow

```yaml
# Taskfile.yml
tasks:
  docs:
    deps: [docs:lint, docs:build]
  docs:lint:
    cmds: [mdsmith fix --no-build .]
    sources: ["**/*.md"]
  docs:build:
    cmds: [mdsmith fix --build-only .]
    sources: ["**/*.md", ".mdsmith.yml"]
    deps: [docs:lint]
```

go-task handles the *outer* DAG (mdsmith vs npm vs pandoc).
mdsmith handles the *inner* DAG (which `<?build?>` directive
is stale). Both use content hashing within their scope.

## Plan delta if we adopt this recommendation

| Plan                                             | Status                                                                                            |
|--------------------------------------------------|---------------------------------------------------------------------------------------------------|
| [102][p102] — multi-output directive             | Stays in full                                                                                     |
| [103][p103] — staleness + ActionID cache         | Stays in full (content-hash beats every Go-native alternative)                                    |
| [115][p115] — builder execution in fix           | Stays in full (Model A literally calls `mdsmith fix --build-only`)                                |
| [116][p116] — UX (logs, `--build-jobs`, explain) | Minor shrink: `--build-jobs N` could be deprioritised if users prefer go-task `deps:` parallelism |
| [117][p117] — hardening                          | Stays in full (no external tool ships any of these)                                               |
| **NEW** — `mdsmith targets --json`               | Add (~30 line plan)                                                                               |

[p102]: ../../../plan/102_build-subcommand.md
[p103]: ../../../plan/103_build-staleness-and-deps.md
[p115]: ../../../plan/115_builder-execution-in-fix.md
[p116]: ../../../plan/116_build-execution-ux.md
[p117]: ../../../plan/117_build-execution-hardening.md

## Risks and unknowns

- **API stability of `mdsmith targets --json`.** Once the
  side door exists, downstream Taskfile / Makefile
  generators will rely on the JSON shape. Treat it as a
  versioned API.
- **Two trust surfaces.** If a user wires the side door,
  the outer orchestrator runs *its own* commands too. The
  trust gate (plan 117) only protects mdsmith's spawn,
  not the orchestrator's. Document this explicitly.
- **`mdsmith fix` vs `mdsmith fix --build-only` vs
  `--no-build` ergonomics.** Three modes is one too many
  if users get them wrong. Document a decision tree.
- **Generated sections + builds in one DAG.** mdsmith's
  lint-fix pass already runs both in the right order
  internally (plan 115). The external split is for cross-
  tool DAGs, not intra-mdsmith ones — the side door does
  not let an orchestrator interleave them.

## Gotchas summary

The previous "build system gotchas" research distilled
five lessons from Bazel, Buck, Gradle, Ninja, and others:

1. Buffered stdout hides hangs (Ninja [issue 545][ninja-545]).
2. Failure messages without argv, cwd, exit code, and
   log path are useless.
3. Stale-cache surprises drive `make clean` muscle memory
   unless mdsmith can explain freshness
   (Gradle [issue 14773][gradle-14773]).
4. Non-deterministic recipes silently defeat caching
   (Bazel determinism notes;
   [Buck2 issue #573][buck2-573]).
5. Parallel builds collide on undeclared shared state
   (CMake / zlib race-condition reports).

Plan 116 covers gotchas 1, 2, 3, and 4 (logs, failure
diagnostic, `--build-explain`, `--build-verify`). Plan 117
covers gotcha 5 (post-conditions catch undeclared writes
even under parallel execution). go-task as the engine
covers gotcha 5 only (its `deps:` parallelism does not
enforce output confinement).

[ninja-545]: https://github.com/ninja-build/ninja/issues/545
[gradle-14773]: https://github.com/gradle/gradle/issues/14773
[buck2-573]: https://github.com/facebook/buck2/issues/573

## Sources

### Tool homes and docs

- [go-task / Taskfile][go-task]
- [Taskfile schema reference][taskfile-schema]
- [Taskfile usage and fingerprinting][taskfile-usage]
- [magefile/mage][mage]
- [magefile/mage `target` package (mtime-only)][mage-deps]
- [goyek/goyek][goyek]
- [ejholmes/walk][walk] and [walk's `plan.go`][walk-plan]
- [evmar/n2][n2]
- [ninja-build manual][ninja-manual]
- [thought-machine/please][please]
- [dagger / dagger][dagger]
- [buildbarn / bb-storage][buildbarn]
- [benchkram/bob][bob]

[taskfile-schema]: https://taskfile.dev/docs/reference/schema/
[taskfile-usage]: https://taskfile.dev/usage/
[ninja-manual]: https://ninja-build.org/manual.html

### Issue threads cited

- [go-task #238 — `generates` not in checksum scope][issue-238]
- [Ninja #545 — show subcommand output as it happens][ninja-545]
- [Gradle #14773 — cache corruption on daemon kill][gradle-14773]
- [Buck2 #573 — custom caching for non-reproducible actions][buck2-573]

### Plan files referenced

- [Plan 102 — multi-output `<?build?>` directive][p102]
- [Plan 103 — build target staleness and dependency tracking][p103]
- [Plan 115 — builder execution wired into `mdsmith fix`][p115]
- [Plan 116 — build execution UX][p116]
- [Plan 117 — build execution hardening][p117]
