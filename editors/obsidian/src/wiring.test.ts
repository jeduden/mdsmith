// Drives the host-agnostic wiring glue main.ts assembles.
//
// main.ts is the thin Obsidian Plugin shell; the testable pieces live
// in wiring.ts:
//   - makeAssetLoaders: reads wasm_exec.js + mdsmith.wasm from the
//     plugin directory through a vault adapter, returning the two
//     loader callbacks createRuntime expects.
//   - editorAdapter: wraps a CM6 EditorView into the EditorLike shape
//     applyFixToEditor drives.
//   - diagnosticRows: flattens the per-uri diagnostics map into the
//     sortable rows the diagnostics view renders.

import { describe, expect, test } from "bun:test";

import type { Diagnostic } from "./wasm-runtime";
import {
  diagnosticRows,
  editorAdapter,
  makeAssetLoaders,
} from "./wiring";

function diag(partial: Partial<Diagnostic>): Diagnostic {
  return {
    file: "a.md",
    line: 1,
    column: 1,
    rule: "MDS001",
    name: "line-length",
    severity: "warning",
    message: "m",
    ...partial,
  };
}

describe("makeAssetLoaders", () => {
  test("reads wasm_exec.js as text and mdsmith.wasm as bytes from the plugin dir", async () => {
    const reads: string[] = [];
    const adapter = {
      async read(p: string): Promise<string> {
        reads.push(p);
        return "// wasm_exec source";
      },
      async readBinary(p: string): Promise<ArrayBuffer> {
        reads.push(p);
        return new Uint8Array([1, 2, 3]).buffer;
      },
    };
    const { loadWasmExec, loadWasmBytes } = makeAssetLoaders(
      adapter,
      ".obsidian/plugins/mdsmith",
    );
    expect(await loadWasmExec()).toBe("// wasm_exec source");
    const bytes = await loadWasmBytes();
    expect(new Uint8Array(bytes as ArrayBuffer)).toEqual(
      new Uint8Array([1, 2, 3]),
    );
    expect(reads).toEqual([
      ".obsidian/plugins/mdsmith/wasm_exec.js",
      ".obsidian/plugins/mdsmith/mdsmith.wasm",
    ]);
  });

  test("throws a clear error when the plugin dir is unknown", () => {
    const adapter = {
      read: async () => "",
      readBinary: async () => new ArrayBuffer(0),
    };
    expect(() => makeAssetLoaders(adapter, undefined)).toThrow(
      /plugin directory/,
    );
  });
});

describe("editorAdapter", () => {
  test("exposes the buffer length and value and dispatches a replace", () => {
    const dispatched: unknown[] = [];
    const view = {
      state: { doc: { length: 7, toString: () => "content" } },
      dispatch: (tr: unknown) => dispatched.push(tr),
    };
    const ed = editorAdapter(view, "note.md");
    expect(ed.length).toBe(7);
    expect(ed.getValue()).toBe("content");
    ed.dispatch({ from: 0, to: 7, insert: "new" });
    expect(dispatched).toEqual([{ changes: { from: 0, to: 7, insert: "new" } }]);
  });
});

describe("diagnosticRows", () => {
  test("flattens the per-uri map and sorts by uri, then line, then column", () => {
    const byUri = new Map<string, Diagnostic[]>([
      [
        "b.md",
        [diag({ file: "b.md", line: 2, column: 1, rule: "MDS009" })],
      ],
      [
        "a.md",
        [
          diag({ file: "a.md", line: 5, column: 3, rule: "MDS001" }),
          diag({ file: "a.md", line: 1, column: 1, rule: "MDS023" }),
        ],
      ],
    ]);
    const rows = diagnosticRows(byUri);
    expect(rows.map((r) => [r.uri, r.line, r.column])).toEqual([
      ["a.md", 1, 1],
      ["a.md", 5, 3],
      ["b.md", 2, 1],
    ]);
    expect(rows[0].rule).toBe("MDS023");
    expect(rows[0].message).toBe("m");
  });

  test("omits files whose diagnostic list is empty", () => {
    const byUri = new Map<string, Diagnostic[]>([
      ["clean.md", []],
      ["bad.md", [diag({ file: "bad.md" })]],
    ]);
    const rows = diagnosticRows(byUri);
    expect(rows.map((r) => r.uri)).toEqual(["bad.md"]);
  });
});
