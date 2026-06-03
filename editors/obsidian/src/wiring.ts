// Host-agnostic wiring glue assembled by main.ts.
//
// main.ts is the thin Obsidian Plugin shell. The pieces that have logic
// worth testing without the host live here:
//   - makeAssetLoaders builds the wasm_exec.js + mdsmith.wasm loader
//     callbacks createRuntime expects, reading them from the plugin
//     directory through a vault adapter.
//   - editorAdapter wraps a CM6 EditorView into the EditorLike shape
//     applyFixToEditor drives.
//   - diagnosticRows flattens the per-uri diagnostics map into the
//     sortable rows the diagnostics view renders.

import type { EditorLike } from "./actions";
import type { Diagnostic } from "./wasm-runtime";

// AdapterLike is the slice of Obsidian's DataAdapter the WASM loader
// reads: read for the wasm_exec.js text, readBinary for the .wasm bytes.
export interface AdapterLike {
  read(normalizedPath: string): Promise<string>;
  readBinary(normalizedPath: string): Promise<ArrayBuffer>;
}

// AssetLoaders is the pair createRuntime needs to instantiate the
// engine from the plugin directory.
export interface AssetLoaders {
  loadWasmExec: () => Promise<string>;
  loadWasmBytes: () => Promise<ArrayBuffer>;
}

// makeAssetLoaders returns loaders that read the two WASM assets from
// the plugin directory (manifest.dir) through the vault adapter. The
// host has no fetch for plugin-bundled files; the adapter is the path.
export function makeAssetLoaders(
  adapter: AdapterLike,
  pluginDir: string | undefined,
): AssetLoaders {
  if (!pluginDir) {
    // manifest.dir is set by Obsidian for an installed plugin; its
    // absence means we cannot locate our own bundle.
    throw new Error(
      "mdsmith: plugin directory is unknown (manifest.dir unset); cannot " +
        "load the WASM engine",
    );
  }
  const join = (name: string): string => `${pluginDir}/${name}`;
  return {
    loadWasmExec: () => adapter.read(join("wasm_exec.js")),
    loadWasmBytes: () => adapter.readBinary(join("mdsmith.wasm")),
  };
}

// EditorViewLike is the slice of a CM6 EditorView editorAdapter wraps.
interface EditorViewLike {
  state: { doc: { length: number; toString(): string } };
  dispatch(tr: unknown): void;
}

// editorAdapter wraps a CM6 EditorView into the EditorLike shape
// applyFixToEditor expects. A whole-buffer replace becomes a single CM6
// transaction with one change.
export function editorAdapter(
  view: EditorViewLike,
  _uri: string,
): EditorLike {
  return {
    get length(): number {
      return view.state.doc.length;
    },
    getValue: () => view.state.doc.toString(),
    dispatch: (change) => view.dispatch({ changes: change }),
  };
}

// DiagnosticRow is one row of the workspace diagnostics view: a
// diagnostic plus the uri it belongs to, flattened for a sortable table.
export interface DiagnosticRow {
  uri: string;
  line: number;
  column: number;
  rule: string;
  severity: string;
  message: string;
}

// diagnosticRows flattens the per-uri diagnostics map into rows sorted
// by uri, then line, then column — the order the diagnostics view lists
// them. Files with no diagnostics are omitted.
export function diagnosticRows(
  byUri: Map<string, Diagnostic[]>,
): DiagnosticRow[] {
  const rows: DiagnosticRow[] = [];
  for (const [uri, diags] of byUri) {
    for (const d of diags) {
      rows.push({
        uri,
        line: d.line,
        column: d.column,
        rule: d.rule,
        severity: d.severity,
        message: d.message,
      });
    }
  }
  rows.sort(
    (a, b) =>
      a.uri.localeCompare(b.uri) || a.line - b.line || a.column - b.column,
  );
  return rows;
}
