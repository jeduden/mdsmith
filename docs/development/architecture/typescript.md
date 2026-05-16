---
title: TypeScript architecture patterns
slug: ts
summary: >-
  SOLID and clean architecture patterns for
  the mdsmith VS Code extension at
  editors/vscode/.
---
# TypeScript architecture patterns

SOLID and clean-architecture rules for
mdsmith's VS Code extension at
`editors/vscode/`. This page is the
source of truth — the
[solid-architecture skill][skill-md]
reads it during design and audit modes
to check TypeScript changes. The layout
below is the target the extension is
converging on. "Current state" callouts
mark where the code differs.

[skill-md]: ../../../.claude/skills/solid-architecture/SKILL.md

## Layout

```text
editors/vscode/src/
├── extension.ts        (thin entry;
│                        delegates to
│                        wiring.ts)
├── wiring.ts           (compose commands +
│                        LSP client)
├── binary.ts           (locate mdsmith
│                        binary)
└── commands/
    ├── init.ts
    ├── kinds.ts
    ├── fix-workspace.ts
    ├── merge-driver.ts
    ├── runner.ts       (typed exec of
    │                    binary subcommands)
    └── virtual-doc.ts
```

Tests live next to source as `*.test.ts`.

**Current state**: `extension.ts` also
runs the LSP client today. It watches
`.mdsmith.yml` for config changes. It
calls `vscode.commands.registerCommand`
directly via `registerPaletteCommands`.
That work should move to `wiring.ts` and
`commands/*`. New code should not grow
`extension.ts` further — place it where
the target shape expects it.

## Single responsibility per module (SRP)

Each `commands/<name>.ts` holds one
command. Shared steps (subprocess spawn,
output capture, error formatting) live
in `commands/runner.ts`. We do not copy
those steps into each command; if a
command needs a variant, the variant
goes in `runner.ts` so every consumer
sees it.

A command past ~200 lines should split.
The natural split is three modules: one
picks the args, one runs the call, one
shows the result.

## Open/closed: adding a command (OCP)

The target is that adding a command does
not modify `extension.ts`:

1. Create `commands/<name>.ts` exporting
   a registration function.
2. Wire it from `wiring.ts`.
3. Add `commands/<name>.test.ts`
   alongside.
4. Add the matching entry to the
   `contributes.commands` section of
   `package.json`.

**Current state**: commands register in
`extension.ts` today via
`registerPaletteCommands`. Add new
entries to that helper. Do not call
`vscode.commands.registerCommand` from
the activation body. Keeping
registration in one helper makes the
eventual move to `wiring.ts` mechanical.

The `contributes` section of
`package.json` is the public surface for
VS Code commands and configuration.
Treat it as part of the contract;
changing the id or arguments of a
shipped command is a breaking change.

## Liskov: commands are interchangeable (LSP)

Every command takes the same wiring
shape — a typed dependency object or
`vscode.ExtensionContext` — and returns
`Promise<void>` or `vscode.Disposable`.
We do not add optional, command-specific
parameters at the registration site. If
a command needs extra configuration, it
reads it from workspace settings inside
the command.

A command commits to one return shape.
"Maybe done, maybe not" violates Liskov;
the caller should not need to know which
kind it received.

## Interface segregation at `binary.ts` (ISP)

`binary.ts` exposes a small interface
for locating and invoking the `mdsmith`
binary. Functions stay narrow:

- "Find the binary path" does not also
  warm a cache of subcommand outputs.
- "Run `mdsmith kinds --json`" does not
  also run `mdsmith fix .`.

Add a new function rather than widening
an existing one. Consumers import only
what they need.

## Dependency inversion through typed boundaries (DIP)

The extension talks to the Go core
through three typed boundaries; we never
thread raw, unparsed values across them:

- **LSP protocol** — typed via
  `vscode-languageclient`. Capabilities
  are the contract; additions are
  additive, removals are breaking.
- **Subprocess invocation** of the
  `mdsmith` binary — typed by the JSON
  schema each subcommand returns,
  parsed in `commands/runner.ts`.
  Today `runner.ts` owns subprocess
  spawn and returns
  `{stdout, stderr, exitCode}`. Target:
  parse those results into typed
  domain shapes that commands consume.
- **File system reads** — typed via
  narrow shape interfaces (e.g. a
  parsed `.mdsmith.yml` shape, not raw
  YAML).

If a binary subcommand returns a list,
parse it into a typed value in
`runner.ts`; consumers see the type,
not the raw JSON. The boundary between
strings and types should be exactly one
function deep.

## Clean wiring in `extension.ts` (target)

Target shape:

- Activate the extension.
- Construct the wiring object.
- Hand control to `wiring.ts`.

**Current state**: `extension.ts` also
owns the LSP client. It owns a
config-file watcher. It owns direct
command registrations. Those concerns
are slated to move to `wiring.ts` and
dedicated command modules. The
violations list flags new additions to
`extension.ts` outside the existing
`registerPaletteCommands` helper.

## Tests

<?include
file: tests.md
strip-frontmatter: "true"
heading-level: "absolute"
?>
### Test pyramid

mdsmith follows a four-layer test
pyramid. Each layer answers a
different question and sits in a
different place in the tree:

- **Unit** — one function or method
  per test. Lives next to source.
  No file I/O beyond inline string
  fixtures. Runs in milliseconds.
- **Contract** — locks a port-package
  interface or external surface
  shape. A contract test must fail
  loudly when the shape it pins
  drifts.
- **Integration** — multiple packages
  composed together against real
  Markdown fixtures.
- **E2E** — the built binary (or the
  packaged extension) against a
  fixture workspace.

The pyramid shape — many unit, fewer
contract, fewer integration, fewest
e2e — keeps the suite fast and the
feedback loop tight.

#### Every function has a dedicated unit test

A new function lands together with
its dedicated unit test by name.
Sub-behaviours of the same function
go in subtests under that parent.
The rule applies to exported and
unexported functions alike. The
audit flags every function in the
touched set that lacks a matching
test.

The language-specific page binds
this rule to concrete file and
symbol patterns. For Go, that is
`TestFunctionName`. For TypeScript,
that is a `describe("name")` block
with one or more `it("case")` cases.

#### Exemptions

A function may skip its dedicated
test only if one of these holds:

- It is generated code (file begins
  with a `// Code generated…` header,
  matches a generator file pattern
  such as `*_gen.go`, is a `*.d.ts`
  declaration, or is emitted under
  `dist/`).
- It is a trivial accessor with no
  branch — a one-line getter or a
  `String()`-style format method.

Add a one-line comment on the
function in either case so the audit
can distinguish "no test by design"
from "no test, forgotten".

#### Push down by default

A unit test on the same behaviour
is faster than the equivalent
integration test. It stays focused
on one function. It also survives
refactors better. The audit pushes
back on inverted pyramids:

- An integration test that exercises
  one function should move down to
  that function's own package as a
  unit test.
- An e2e test that exercises
  behaviour reachable through the
  integration layer should move down
  to integration.

Save e2e for the full process
boundary. Use it for exit codes.
Use it for signals. Use it for
subprocess lifecycle. Use it for
packaged-artifact tests.
<?/include?>

### TypeScript-specific bindings

- **Unit tests** in `xxx.test.ts`
  next to `xxx.ts`. The dedicated
  test for `function foo` (or a
  method `foo`) is a
  `describe("foo", () => { … })`
  block; one or more `it("case",
  …)` cases enumerate behaviours.
  Extract pure functions out of
  command bodies and unit-test
  those instead of mocking
  `vscode`.
- **Contract tests** pin the shape
  of the binary boundary — the JSON
  envelopes parsed in
  `commands/runner.ts` — the
  `contributes` section of
  `package.json`, and the LSP
  capability set the extension opts
  into. They live next to the
  module that owns the contract.
- **Integration tests** drive the
  real `mdsmith` binary against
  fixture workspaces rather than
  mocking subprocess calls. They
  live next to the command module
  they exercise.
- **E2E tests** spin up the VS Code
  extension host against a fixture
  workspace. Reserve them for
  behaviour that cannot be checked
  at the lower layers (activation
  order, command palette wiring,
  `onSave` handlers).
- Mock the `vscode` API only when
  unavoidable. If a function is
  hard to test without `vscode`,
  the function probably contains
  pure logic that should be
  extracted.
- Place test fixtures under
  `editors/vscode/test-fixtures/`
  or alongside the test that uses
  them. Do not import fixture data
  across command modules.

Severity for missing unit tests:
`tax` by default. Promote to
`blocker` if the function sits on
a public surface — an exported
command registration, a
`contributes`-backed entry point,
or a binary-boundary parser.

## Common violations to flag

These are the mdsmith-specific shapes
the audit flags:

- **A command that imports another
  command.** Share state through
  `wiring.ts` instead.
- **A command that constructs paths to
  another command's artifacts.** Same
  fix; go through wiring.
- **A type declared in `extension.ts`
  used by commands.** Belongs in
  `wiring.ts` or its own module.
- **Raw `child_process.exec` outside
  `binary.ts` or `commands/runner.ts`.**
  The subprocess boundary is one place.
- **A field added to `package.json`
  `contributes` without a corresponding
  module that owns it.** A contract
  drift bomb.
- **A test that spins up the full VS
  Code host** to test logic that could
  be unit-tested out of band.
- **A `vscode.commands.registerCommand`
  call inlined in `activate()` or
  anywhere outside
  `registerPaletteCommands`** (and,
  after the planned refactor, anywhere
  outside `wiring.ts`).
- **A `Util`, `Helpers`, or `Misc`
  module anywhere in `src/`.** Same
  smell, same fix.

## Match the npm shim

`npm/mdsmith/test/shim.test.ts` tests
the binary-launcher shim. The shim is
the only place that knows how to find
and exec the Go binary across
platforms. The extension's `binary.ts`
should consume that contract, not
reimplement it. If the extension needs
platform-specific behavior the shim
does not provide, the behavior goes
into the shim first.

## Refactor moves we have used

- A command with three responsibilities
  splits into three commands, or one
  command plus two helpers in
  `commands/runner.ts`.
- A growing `wiring.ts` extracts a
  registry module that commands enroll
  into; `wiring.ts` stays a thin call
  list.
- Raw JSON crossing module boundaries
  parses into a typed shape at the
  binary boundary; consumers see the
  shape, not the JSON.
- Scattered
  `vscode.workspace.getConfiguration`
  calls centralize into a config-reader
  module typed to the `.mdsmith.yml`
  and `package.json` `contributes`
  schemas.
