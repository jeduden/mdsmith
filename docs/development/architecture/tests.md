---
title: Test pyramid
slug: tests
summary: >-
  Four-layer test pyramid (unit,
  contract, integration, e2e) and the
  rule that every function ships with
  a dedicated unit test. Included from
  the Go and TypeScript architecture
  pages.
---
# Test pyramid

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

## Every function has a dedicated unit test

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

## Exemptions

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

## Push down by default

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
