// Node smoke harness for the mdsmith WASM engine. It loads the wasm
// artifact via Go's wasm_exec.js runtime, registers globalThis.mdsmith,
// creates a session over an in-memory workspace, calls session.check on
// a cross-file fixture, and prints the result as JSON on stdout. The Go
// smoke test (smoke_test.go) runs this and asserts the output equals the
// native engine on the same fixture.
//
// Usage: node smoke.cjs <wasm_exec.js> <mdsmith.wasm>
//
// CommonJS so wasm_exec.js (a plain IIFE script that assigns
// globalThis.Go) can be loaded with require for its side effect.
"use strict";

const fs = require("node:fs");

const [, , wasmExecPath, wasmPath] = process.argv;
if (!wasmExecPath || !wasmPath) {
  console.error("usage: node smoke.cjs <wasm_exec.js> <mdsmith.wasm>");
  process.exit(2);
}

// Node 22 provides TextEncoder/TextDecoder/crypto/performance as
// globals; the Go runtime references globalThis.fs for stdout/stderr
// writes, so wire it explicitly.
globalThis.fs = fs;

// wasm_exec.js is a script (not a module): requiring it runs the IIFE
// that defines globalThis.Go.
require(wasmExecPath);

async function main() {
  const go = new globalThis.Go();
  const bytes = fs.readFileSync(wasmPath);
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);

  // go.run never resolves (Go main blocks on select{}), so do not await
  // it; createSession is registered synchronously during startup.
  go.run(instance);

  if (typeof globalThis.mdsmith !== "object") {
    throw new Error("globalThis.mdsmith was not registered by the wasm module");
  }

  const workspace = {
    "docs/one.md": "---\nsummary: First doc\n---\n# One\n\nBody paragraph one here.\n",
    "docs/two.md": "---\nsummary: Second doc\n---\n# Two\n\nBody paragraph two here.\n",
  };
  const indexSrc =
    '# Index\n\n<?catalog\nglob:\n  - "docs/*.md"\n' +
    'row: "- [{summary}](docs/{filename})"\n?>\n<?/catalog?>\n';

  const session = await globalThis.mdsmith.createSession({
    workspace,
    configYAML: "",
  });

  const caps = session.capabilities();
  const diags = await session.check("index.md", indexSrc);
  session.dispose();

  const out = {
    version: globalThis.mdsmith.version,
    capabilities: [...caps].sort(),
    diagnostics: diags
      .map((d) => ({
        rule: d.rule,
        line: d.line,
        column: d.column,
        message: d.message,
      }))
      .sort(
        (a, b) =>
          a.line - b.line || a.column - b.column || a.rule.localeCompare(b.rule),
      ),
  };
  console.log(JSON.stringify(out));
}

main().then(
  () => process.exit(0),
  (err) => {
    console.error(err && err.stack ? err.stack : String(err));
    process.exit(1);
  },
);
