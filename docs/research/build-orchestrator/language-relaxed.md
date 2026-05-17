---
title: External build orchestrator spike — language relaxed
description: Whether relaxing the Go-only constraint opens up a
  better external build orchestrator for mdsmith's `<?build?>`
  feature than the internal engine in plans 102-117.
---
# External build orchestrator spike — language relaxed

## Scope

The companion spike
([go-only.md](go-only.md)) found no Go-native
orchestrator that matches mdsmith's threat model. This
follow-up asks: if mdsmith users can be required to install
a non-Go binary, what becomes possible?

The audience matters. mdsmith users are tech writers and
docs-tooling devs. They run pandoc for ePubs, draw a
diagram with mermaid-cli, take a screenshot with
chromedp-headless. They are not Bazel-shop platform
engineers. Any orchestrator that demands a JVM, a Haskell
toolchain, or a 150 MB Python + Rust install raises the
floor of "what does it cost to lint Markdown."

## Verdict

**Even with the Go-only constraint dropped, the internal
engine remains the right call.** No non-Go orchestrator is
both (a) close enough to mdsmith's threat model and (b)
cheap enough to install for the docs-writer audience. The
candidates split cleanly:

- **Strongest correctness model**: [Shake][shake] (Haskell,
  Neil Mitchell). Content-hash native, early cutoff,
  dynamic dependencies, lint mode for invariants.
  Distribution requires GHC + Stack — hostile to the
  audience.
- **Strongest threat model**: [Tup][tup]. Its FUSE / PTRACE
  tracing literally implements output confinement and
  undeclared-write detection — exactly what plan 117's
  post-conditions reimplement. Distribution requires a
  kernel filesystem extension on every platform.
- **Closest to "everything plan 117 specifies"**: [Pants
  v2][pants]. Sandboxed by default, content-addressed,
  active. ~150 MB install of Python + bundled Rust engine
  for a Markdown linter audience is a non-starter.

The recommendation is unchanged from the Go-only spike:
**keep the internal engine; treat Tup as the inspiration
for plan 117's post-conditions, treat Shake as the
inspiration for plan 103's content-hash and early-cutoff
design**. Add the same `mdsmith targets --json` side door
proposed in the Go-only spike — it lets a power user wire
any of these in if they want.

## Candidates table

"Distribution" combines runtime + install medium so the
table fits within mdsmith's 8-column limit.

| Tool                  | Distribution                            | Content hash             | Atomic multi-output  | Output confinement    | Trust gate / sandbox   | Maintenance (May 2026)           | Cross-platform             |
|-----------------------|-----------------------------------------|--------------------------|----------------------|-----------------------|------------------------|----------------------------------|----------------------------|
| [GNU make][make]      | C; pre-installed everywhere on Unix     | No (mtime)               | No                   | No                    | No                     | Active; POSIX-standardised       | mac / Linux / Win (mingw)  |
| [ninja][ninja]        | C++ binary; apt / brew / choco          | No (mtime; `restat = 1`) | No                   | No                    | No                     | Active; widely shipped           | mac / Linux / Win          |
| [just][just]          | Rust binary; brew / apt / cargo / scoop | No                       | No                   | No                    | No                     | Very active (33 k+ stars)        | mac / Linux / Win          |
| [Shake][shake]        | Haskell library; `stack install`        | Yes (file content)       | User code            | Partial (lint mode)   | No                     | v0.19.9 Jan 2026; powers Hadrian | mac / Linux / Win          |
| [Tup][tup]            | C + Lua; needs FUSE / FUSE-T            | Yes                      | Per-rule isolation   | **Yes — auto-detect** | Partial (FUSE sandbox) | Active (commits Mar 2026)        | mac (FUSE-T) / Linux / Win |
| [redo][apenwarr-redo] | Python (apenwarr); C / C++ ports        | Yes (via `redo-stamp`)   | No                   | No                    | No                     | apenwarr active; ports mixed     | Unix-mostly                |
| [Bazel][bazel]        | Java + C++; `bazelisk` / brew           | Yes (SHA-256)            | Yes (sandboxed exec) | Yes (sandbox)         | Yes                    | Very active (Google)             | mac / Linux / Win          |
| [Buck2][buck2]        | Rust; GitHub release only               | Yes                      | Partial without RE   | Partial               | Partial                | Active (Meta)                    | mac / Linux                |
| [Pants v2][pants]     | Python + Rust; `pip install` (~150 MB)  | Yes                      | Yes (Rust engine)    | Yes                   | Yes                    | Active                           | mac / Linux                |
| [Please][please]      | Go binary; install script               | Yes (Bazel-style)        | Yes                  | Partial               | Some                   | Active                           | mac / Linux                |
| [SCons][scons]        | Python; pip                             | Yes (MD5 / SHA default)  | Partial              | No                    | No                     | Active Nov 2025                  | mac / Linux / Win          |
| [doit][doit]          | Python; pip                             | Yes (MD5 default)        | No                   | No                    | No                     | Active                           | mac / Linux / Win          |
| [Earthly][earthly]    | Go binary + BuildKit Docker; brew       | Yes (BuildKit CAS)       | Yes (containerised)  | Yes (containerised)   | Yes (containerised)    | Active                           | mac / Linux / Win + Docker |

[make]: https://www.gnu.org/software/make/
[ninja]: https://github.com/ninja-build/ninja
[just]: https://github.com/casey/just
[shake]: https://shakebuild.com/
[tup]: https://github.com/gittup/tup
[apenwarr-redo]: https://github.com/apenwarr/redo
[bazel]: https://bazel.build/
[buck2]: https://buck2.build/
[pants]: https://www.pantsbuild.org/
[please]: https://please.build/
[scons]: https://scons.org/
[doit]: https://pydoit.org/
[earthly]: https://earthly.dev/

## Top three deep dives

### Tup — best threat-model match

**How mdsmith integrates** (Model B, graph generator).
mdsmith emits a `Tupfile` per directory with one `:- rule`
per `<?build?>` directive. Inputs and outputs become
Tupfile inputs / outputs verbatim. Tup's FUSE / PTRACE
layer takes over file-access tracing, undeclared-write
rejection, and incremental rebuild against content hashes.
The user runs `tup` instead of `mdsmith fix --build-only`.

**Plan delta.** Plan 117 shrinks dramatically. Output
confinement, undeclared-write detection, and recipe
sandboxing are Tup's headline features — exactly what plan
117's post-conditions implement by hand. Plan 103's
ActionID becomes Tup's signature. Plan 102 stays as the
directive parser. Plans 115 / 116 shrink to "emit Tupfile
and shell out to `tup`." Net deletion: most of 103 and most
of 117.

**User burden.** Catastrophic. Tech writers must install
`tup` plus a FUSE kernel module — [macFUSE][macfuse], or
the kext-free [FUSE-T][fuse-t] on Apple Silicon, or libfuse
on Linux. Windows requires WinFsp. Recent Tup commits do
show macOS CI on FUSE-T (March 2026), so the platform is
not dying, but "install a kernel filesystem to lint
Markdown" is a non-starter. Tup also has no SemVer release
cadence and a small bus factor.

**Sources:** the Tup repo and manual; the [Lobsters
discussion of Tup][tup-lobsters]; the FUSE-T project page.

[macfuse]: https://macfuse.github.io/
[fuse-t]: https://www.fuse-t.org/
[tup-lobsters]: https://lobste.rs/s/3fnkyc/tup_file_based_build_system

### Shake — best correctness model, weakest distribution

**How mdsmith integrates** (Model A, mdsmith as verb, or
Model B with a generated Shakefile.hs). mdsmith ships a
stub `Shakefile.hs` template. The user copies it, calls
`mdsmith targets --json` from a Haskell rule, and Shake
takes over. Or mdsmith generates a Shakefile.hs file with
one `*>` rule per `<?build?>`.

**Plan delta.** Plan 103 (ActionID, early-cutoff caching)
is exactly Shake's wheelhouse. Plan 116's
`--build-explain` becomes Shake's `--lint` and `-VV`. Plan
117 still mostly needed — Shake leaves sandboxing to the
user. Net deletion: most of 103 and parts of 116.

**User burden.** Severe. Haskell Stack install plus `stack
install shake` is required; the [Shake quick-start][shake]
literally begins "install the Haskell Stack." There is no
canonical static-binary distribution; users would have to
compile their personal Shakefile.hs into a binary on every
machine. The audience for mdsmith — tech writers — does
not have GHC.

**Sources:** Shake homepage, [Hackage entry for
shake-0.19.9][shake-hackage], [Mitchell's "Reflecting on
the Shake Build System" retrospective][shake-retro], the
[non-recursive Make paper][shake-paper] by Mokhov,
Mitchell, Marlow.

[shake-hackage]: https://hackage.haskell.org/package/shake
[shake-retro]: https://neilmitchell.blogspot.com/2021/09/
[shake-paper]: https://simonmar.github.io/bib/papers/shake.pdf

### make, redo, ninja, just — popular side-doors

None of these is a real build engine for mdsmith's
threat model, but each is a sane "side-door target" if
the user already drives a docs site with one of them.
Walked through individually because the user explicitly
asked about make and redo.

**make.** POSIX-standardised, pre-installed on every
Unix, every developer recognises a Makefile. mdsmith
emits a Makefile (Model B) with one rule per
`<?build?>` directive. Wins: zero install friction, one
familiar entry point per project. Losses: mtime-only,
which is exactly the same CI cache-restore problem as
ninja (see below); no atomic multi-output (a recipe
that fails halfway leaves partial files in place); no
trust gate; classic concurrent-write hazards under
`-j N` if two rules touch a shared directory ([Miller's
*Recursive Make Considered Harmful*][miller-make]
remains the foundational reading). The "every dev
knows make" advantage is real but does not buy mdsmith
any of the threat-model defenses plan 117 covers. Plan
delta if adopted: same as ninja.

**redo** (Avery Pennarun's Python implementation, the
active fork; also [DJB's original sketch][djb-redo] and
ports in C, C++, and Go). Each `<?build?>` becomes a
`target.do` shell script that calls `redo-ifchange
input1 input2 ...` to declare dependencies, then runs
the recipe. redo records sha hashes (`redo-stamp`) so
re-runs detect actual content change, not mtime. mdsmith
generates `.do` files (Model B with shell scripts).
Wins: content-hash native, parallel `-j N`, no DSL —
just shell scripts the user can read and debug. Losses:
no atomic multi-output (still the user's problem); no
trust gate; Python install or build-from-source for the
C ports; uneven cross-platform story (Windows is
poorly supported by every redo implementation). Plan
delta if adopted: plan 103 mostly absorbed (redo handles
content hashing); plans 115 / 116 / 117 stay.

**ninja.** Single C++ binary, ubiquitous, fast. mdsmith
emits `build.ninja` (Model B). The mtime problem is
real: docs CIs that cache `_site/` between runs see
mtimes reset to the cache-restore moment, and ninja
then either spuriously rebuilds everything or skips
legitimate rebuilds. `restat = 1` only helps when an
output's content matches its previous content; it does
not fix CI cache restore. Open issues [#2740][ninja-2740]
and [#1459][ninja-1459] confirm the problem is
unresolved upstream. For hundreds-of-Markdown-files
docs the practical impact is occasional spurious
rebuilds — annoying, not catastrophic. Workaround:
content-hash sidecar (mdsmith already has the cache
from plan 103) plus timestamp pinning on cache restore
(the [`mtime_cache` gem][mtime-cache]). Plan delta if
adopted: ninja replaces some of 116's parallel work;
everything else stays.

**just.** Rust single binary, 33 k+ stars. **It
explicitly says it is not a build system and does no
file tracking.** Adopting it deletes nothing from plans
102 / 103 / 115 / 117 — it would only replace the
orchestration shell of plan 116. Net value: low.

[miller-make]: https://aegis.sourceforge.net/auug97.pdf
[djb-redo]: https://cr.yp.to/redo.html
[ninja-2740]: https://github.com/ninja-build/ninja/issues/2740
[ninja-1459]: https://github.com/ninja-build/ninja/issues/1459
[mtime-cache]: https://github.com/iboB/mtime_cache

## The orchestrator that gets closest to mdsmith's threat model

**Pants v2** — Python frontend, Rust engine, sandboxed by
default, content-addressed, daemon-mode change watching.
Pants implements every threat-model defense plan 117
specifies — sandbox, hermetic env, content-addressed
action cache, fine-grained invalidation — and its
sandboxing is on by default rather than gated behind
remote execution. It is actively maintained and used by
real organisations.

**Why it is still not a slam-dunk.** Pants demands `pip
install pantsbuild.pants` plus a Python environment plus
the bundled Rust engine native module. The minimal install
footprint is roughly 150 MB. Configuration is in BUILD
files written in a Pants-flavored Python dialect. mdsmith's
audience runs pandoc for ePubs; asking them to install
Pants to lint Markdown inverts the value ratio. Pants is
also designed for source-code monorepos, not docs trees;
its value-add for hundreds of pandoc invocations versus a
few thousand lines of in-house Go is small.

**Buck2** is Pants's nearest peer in Rust. Reading the
Buck2 docs makes the trade-off explicit:

> Purely local builds are currently not sandboxed in
> Buck2 and therefore hermeticity cannot be enforced
> [without remote execution].

This is a deliberate Meta-internal design choice — Meta
runs Buck2 against an RE cluster — and it sinks Buck2 as
a sandbox for mdsmith. ([Buck2 bootstrapping
docs][buck2-boot], [Buck2 install
docs][buck2-install], [Tweag tour of Buck2][buck2-tweag].)

[buck2-boot]: https://buck2.build/docs/about/bootstrapping/
[buck2-install]: https://buck2.build/docs/getting_started/install/
[buck2-tweag]: https://www.tweag.io/blog/2023-07-06-buck2/

## Plan delta if any of these were adopted

| Plan                                                          | Tup                           | Shake              | ninja             | Pants v2       |
|---------------------------------------------------------------|-------------------------------|--------------------|-------------------|----------------|
| [102][p102] — multi-output directive                          | Stays in full                 | Stays in full      | Stays in full     | Stays in full  |
| [103][p103] — staleness + ActionID cache                      | Mostly deleted                | Mostly deleted     | Stays (mtime gap) | Mostly deleted |
| [115][p115] — builder execution in fix                        | Shrinks to "emit + shell out" | Shrinks            | Shrinks           | Shrinks        |
| [116][p116] — UX (logs, `--build-jobs`, explain)              | Shrinks (Tup logs)            | Shrinks (`--lint`) | Minor shrink      | Stays          |
| [117][p117] — hardening                                       | Mostly deleted                | Stays in full      | Stays in full     | Mostly deleted |
| **NEW** — `mdsmith targets --json` side door                  | Add                           | Add                | Add               | Add            |
| **NEW** — graph emitter (Tupfile / Shakefile / ninja / BUILD) | Add                           | Add                | Add               | Add            |
| **NEW** — install / dependency docs                           | Add                           | Add                | Add               | Add            |

[p102]: ../../../plan/102_build-subcommand.md
[p103]: ../../../plan/103_build-staleness-and-deps.md
[p115]: ../../../plan/115_builder-execution-in-fix.md
[p116]: ../../../plan/116_build-execution-ux.md
[p117]: ../../../plan/117_build-execution-hardening.md

In every column, mdsmith *gains* a graph emitter and an
install-docs chore. Tup and Pants v2 delete the most plan
content, but each pays back the deletion in user-install
friction.

## Risks and unknowns

- **FUSE-T on macOS is the only viable Tup story.**
  macFUSE (kext) is increasingly restricted on Apple
  Silicon. FUSE-T is kext-free and recent Tup CI commits
  track it. If FUSE-T stalls, Tup is effectively Linux-only.
- **Buck2's local-sandbox gap may close.** Meta has
  signalled local sandboxing is on the roadmap. If it
  lands, Buck2 jumps to first place on the threat-model
  axis with a plausible install story (single Rust
  binary). Worth re-evaluating in twelve months.
- **Pants for docs is unprecedented.** No public examples
  of Pants driving a Markdown-only docs build. Tooling
  fit is unknown.
- **Shake static binary.** Mitchell has discussed it; if a
  `shake` standalone CLI shipped with bundled rules
  (similar to how Hadrian could ship a Shake binary for
  GHC), the install story would change overnight. Watch
  the shake repo.
- **CI cache mtime is hostile to mtime engines
  generally.** The ninja issues are a cautionary tale
  generalisable to any mtime-based engine. mdsmith should
  treat git-checkout and cache-restore mtimes as untrusted
  even with the internal engine.
- **Outer-orchestrator trust.** Any hand-off to a foreign
  orchestrator means the trust gate (plan 117) only
  protects mdsmith's own spawn. The outer orchestrator
  runs its own commands in its own threat envelope.
  Document this asymmetry.
- **Cross-platform Windows.** ninja and just are first-
  class. Tup needs WinFsp. Shake / Pants Windows support
  is real but lightly tested. The internal engine in Go
  remains the only candidate that works equivalently
  everywhere.

## Lessons folded back into the internal engine

The spike validated several internal-engine design
choices and surfaced two that could be tightened:

- **Plan 103's content-hash cache** is borrowed from the
  same model that Shake, Bazel, Buck2, and Pants all
  arrived at. Confirmed.
- **Plan 117's output-confinement post-condition** is
  Tup's headline feature, implemented by mdsmith without
  the FUSE dependency. Confirmed.
- **Plan 116's `--build-explain`** maps directly to
  Shake's `--lint` and `-VV`. Naming and feature scope
  confirmed.
- **Open question for plan 103**: should the cache
  invalidate on git-checkout / cache-restore-style mtime
  resets? mdsmith already content-hashes inputs, so the
  answer is "no" — but the doc should call this out
  explicitly as a deliberate choice borrowed from
  ninja's pain.
- **Open question for plan 117**: Pants and Tup both
  sandbox the recipe's `cwd` to a per-invocation
  directory. Plan 117 already does this via the staging
  dir; worth restating that the staging dir is the
  recipe's `Cmd.Dir`, full stop.

## Sources

### Tool homes and primary docs

- [ninja][ninja] and [ninja manual][ninja-manual]
- [casey/just][just]
- [Shake][shake] and [Hackage shake][shake-hackage]
- [gittup/tup][tup] and [tup manual][tup-manual]
- [apenwarr/redo][apenwarr-redo]
- [facebook/buck2][buck2-repo] and [buck2.build][buck2]
- [Pants v2 — How does Pants work?][pants-how] and
  [Introducing Pants v2][pants-intro]
- [SCons docs][scons-docs]
- [pydoit/doit][doit-repo] and [pydoit.org][doit]
- [thought-machine/please][please-repo] and [please.build][please]
- [Earthly docs — BuildKit standalone][earthly-bk]
- [Bazel sandboxing][bazel-sb] and
  [Bazel hermeticity][bazel-h]
- [FUSE-T][fuse-t] and [macFUSE][macfuse]

[ninja-manual]: https://ninja-build.org/manual.html
[tup-manual]: https://gittup.org/tup/manual.html
[buck2-repo]: https://github.com/facebook/buck2
[pants-how]: https://www.pantsbuild.org/stable/docs/
[pants-intro]: https://blog.pantsbuild.org/
[scons-docs]: https://scons.org/doc/production/HTML/scons-user/
[doit-repo]: https://github.com/pydoit/doit
[please-repo]: https://github.com/thought-machine/please
[earthly-bk]: https://docs.earthly.dev/
[bazel-sb]: https://bazel.build/docs/sandboxing
[bazel-h]: https://bazel.build/versions/6.1.0/basics/hermeticity

### Issue threads and primary writing

- [ninja #2740 — wrong mtime in `.ninja_log`][ninja-2740]
- [ninja #1459 — file characteristics vs timestamps][ninja-1459]
- [ninja #1972 — restat problem][ninja-1972]
- [Mitchell — Reflecting on the Shake Build System][shake-retro]
- [Mokhov / Mitchell / Marlow — non-recursive Make paper][shake-paper]
- [evmar — n2: revisiting Ninja][n2-blog]
- [Tweag — A Tour Around Buck2][buck2-tweag]
- [Engineering at Meta — Buck2 announcement][buck2-meta]
- [Buck2 — bootstrapping docs][buck2-boot]
- [Buck2 — install docs][buck2-install]
- [Lobsters — tup discussion][tup-lobsters]
- [iboB/mtime_cache — CI artifact mtime restore][mtime-cache]

[ninja-1972]: https://github.com/ninja-build/ninja/issues/1972
[n2-blog]: https://neugierig.org/software/blog/2022/03/n2.html
[buck2-meta]: https://engineering.fb.com/2023/04/06/open-source/

### Companion plans and spike

- [Plan 102 — multi-output `<?build?>` directive][p102]
- [Plan 103 — build target staleness and dependency tracking][p103]
- [Plan 115 — builder execution wired into `mdsmith fix`][p115]
- [Plan 116 — build execution UX][p116]
- [Plan 117 — build execution hardening][p117]
- [Earlier Go-only spike](go-only.md)
