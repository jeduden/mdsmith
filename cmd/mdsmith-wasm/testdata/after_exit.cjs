// Node harness for the mdsmith WASM engine after its Go program exits.
// It drops sessions without dispose(), lets Go release the JS values it
// held for them, forces a V8 collection so the FinalizationRegistry
// queues a cleanup callback per collected session, and then puts the Go
// runtime in the state runtime.wasmExit leaves (wasm_exec.js sets
// exited and deletes the value tables) before those callbacks run. A
// cleanup callback that calls into Go then throws "Go program has
// already exited" from a GC task, outside any caller's try.
//
// It prints "ok" and exits 0 when at least one cleanup callback ran
// after the exit and none threw; it exits 1 on an uncaught error, and 3
// when no callback ran after the exit, so the check cannot pass without
// exercising the path.
//
// Usage: node after_exit.cjs <wasm_exec.js> <mdsmith.wasm>
"use strict";

const fs = require("node:fs");
const v8 = require("node:v8");
const vm = require("node:vm");

const [, , wasmExecPath, wasmPath] = process.argv;
if (!wasmExecPath || !wasmPath) {
  console.error("usage: node after_exit.cjs <wasm_exec.js> <mdsmith.wasm>");
  process.exit(2);
}

globalThis.fs = fs;
v8.setFlagsFromString("--expose-gc");
const gc = vm.runInNewContext("gc");
require(wasmExecPath);

// Count cleanup callbacks, passing each through to the engine's own so
// whatever it throws still surfaces as uncaught.
let exited = false;
let firedAfterExit = 0;
const NativeRegistry = globalThis.FinalizationRegistry;
globalThis.FinalizationRegistry = class extends NativeRegistry {
  constructor(cleanup) {
    super((held) => {
      if (exited) firedAfterExit++;
      return cleanup(held);
    });
  }
};

const uncaught = [];
process.on("uncaughtException", (err) => uncaught.push(err));

const tick = () => new Promise((r) => setTimeout(r, 10));

// simulateExit mirrors wasm_exec.js's runtime.wasmExit import.
function simulateExit(go) {
  go.exited = true;
  delete go._inst;
  delete go._values;
  delete go._goRefCounts;
  delete go._ids;
  delete go._idPool;
}

async function main() {
  const go = new globalThis.Go();
  // Collect on nearly every allocation, so Go releases the values it
  // held for the dropped sessions within a few calls.
  go.env = { GOGC: "1" };
  const bytes = fs.readFileSync(wasmPath);
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  go.run(instance);

  const keeper = await globalThis.mdsmith.createSession({});
  for (let i = 0; i < 50; i++) {
    await globalThis.mdsmith.createSession({});
  }
  for (let i = 0; i < 50; i++) {
    await keeper.check("a.md", "# A\n\nSome text here.\n");
  }

  gc();
  exited = true;
  simulateExit(go);
  for (let i = 0; i < 20; i++) {
    await tick();
    gc();
  }

  if (uncaught.length > 0) {
    console.error(`uncaught after exit: ${uncaught[0] && uncaught[0].message}`);
    process.exit(1);
  }
  if (firedAfterExit === 0) {
    console.error("no cleanup callback ran after exit");
    process.exit(3);
  }
  console.log("ok");
  process.exit(0);
}

main().catch((err) => {
  console.error(err && err.stack ? err.stack : String(err));
  process.exit(1);
});
