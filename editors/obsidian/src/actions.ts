// Code-action surface for the Obsidian plugin.
//
// Obsidian's command palette and `vault.on('modify')` event are the
// two surfaces this module wires:
//
//   - buildLineCommands derives transient palette commands from the
//     diagnostics on the cursor line. main.ts re-registers them on
//     every cursor move so the palette only shows the rules the user
//     can act on right now.
//   - applyWorkspaceEdit translates an LSP `WorkspaceEdit` (either
//     `changes` or `documentChanges`) into CM6 dispatch operations
//     on the active editor. Used by the Fix file command and the
//     hover tooltip's Fix link.
//   - debounce wraps a function so consecutive calls collapse into
//     one trailing invocation. Used by `fixOnSave` to wait 200 ms
//     after the last save before sending Fix file.

import type { LspDiagnostic } from "./diagnostics";
export type { LspDiagnostic } from "./diagnostics";

// LspTextEdit mirrors the LSP TextEdit shape. Server returns these
// inside a WorkspaceEdit; we translate to CM6 change descriptors.
export interface LspTextEdit {
  range: {
    start: { line: number; character: number };
    end: { line: number; character: number };
  };
  newText: string;
}

// WorkspaceEdit is the union of the two LSP shapes mdsmith may emit.
// `changes` is the legacy uri→edits map; `documentChanges` carries
// optional version stamps. mdsmith currently emits `changes`, but
// accepting both keeps us compatible if the server is upgraded.
export interface WorkspaceEdit {
  changes?: Record<string, LspTextEdit[]>;
  documentChanges?: Array<{
    textDocument: { uri: string; version?: number };
    edits: LspTextEdit[];
  }>;
}

// EditorLike is the structural subset of the active buffer adapter
// applyWorkspaceEdit operates on. main.ts constructs one of these
// per buffer (wrapping the real CM6 EditorView), and tests pass a
// plain object — see actions.test.ts.
export interface EditorLike {
  uri: string;
  offsetAt(pos: { line: number; character: number }): number;
  dispatch(
    changes: Array<{ from: number; to: number; insert: string }>,
  ): void;
}

// LineCommand is the palette entry derived from one active diagnostic
// on the cursor line. main.ts maps each to addCommand({ id, name,
// callback }) on every cursor move.
export interface LineCommand {
  id: string;
  name: string;
  diagnostic: LspDiagnostic;
}

// buildLineCommands returns one command per *unique rule code* on
// the cursor line. Multiple diagnostics for the same rule on the
// same line collapse to one entry — running the fix once handles
// all of them.
export function buildLineCommands(
  diagnostics: LspDiagnostic[],
  cursorLine: number,
): LineCommand[] {
  const seen = new Set<string>();
  const cmds: LineCommand[] = [];
  for (const d of diagnostics) {
    if (d.range.start.line !== cursorLine) continue;
    if (!d.code) continue;
    if (seen.has(d.code)) continue;
    seen.add(d.code);
    cmds.push({
      id: `mdsmith-fix-line-${d.code}`,
      name: `mdsmith: Fix — ${d.code}`,
      diagnostic: d,
    });
  }
  return cmds;
}

// applyWorkspaceEdit walks a WorkspaceEdit, picks out the edits
// targeted at `editor.uri`, sorts them bottom-up (so earlier offsets
// stay valid as later ones land), and dispatches them as one CM6
// change set. Edits for other URIs are silently dropped — Fix file
// targets exactly one buffer.
export function applyWorkspaceEdit(
  editor: EditorLike,
  edit: WorkspaceEdit,
): void {
  const edits: LspTextEdit[] = [];
  if (edit.changes && edit.changes[editor.uri]) {
    edits.push(...edit.changes[editor.uri]);
  }
  if (edit.documentChanges) {
    for (const dc of edit.documentChanges) {
      if (dc.textDocument.uri === editor.uri) {
        edits.push(...dc.edits);
      }
    }
  }
  if (edits.length === 0) return;

  // Translate to CM6 changes and sort bottom-up so earlier offsets
  // do not shift when later edits land.
  const changes = edits.map((e) => ({
    from: editor.offsetAt(e.range.start),
    to: editor.offsetAt(e.range.end),
    insert: e.newText,
  }));
  changes.sort((a, b) => b.from - a.from);
  editor.dispatch(changes);
}

// DebouncedFn is the public surface of debounce(): a callable that
// schedules the wrapped function, plus `cancel()` to drop a pending
// trailing call (used by onunload so we don't fire after the plugin
// is torn down).
export interface DebouncedFn<A extends unknown[]> {
  (...args: A): void;
  cancel(): void;
}

// debounce delays `fn` until `delayMs` has elapsed without another
// call. Successive calls within the window reset the timer; only the
// trailing call runs, and it receives the latest arguments. Cancel
// drops the pending timer.
export function debounce<A extends unknown[]>(
  fn: (...args: A) => void,
  delayMs: number,
): DebouncedFn<A> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  let lastArgs: A | undefined;

  const wrapped = ((...args: A) => {
    lastArgs = args;
    if (timer !== undefined) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = undefined;
      if (lastArgs) fn(...lastArgs);
    }, delayMs);
  }) as DebouncedFn<A>;

  wrapped.cancel = () => {
    if (timer !== undefined) {
      clearTimeout(timer);
      timer = undefined;
    }
    lastArgs = undefined;
  };

  return wrapped;
}
