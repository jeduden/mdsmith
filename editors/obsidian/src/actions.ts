// Action surface for the Obsidian plugin: per-line palette commands,
// whole-buffer fix application, and the debounce fix-on-save uses.
//
// Unlike an LSP bridge, the WASM engine's fix returns the FULL
// rewritten document (FixResult.source) rather than a list of edit
// ranges. Applying a fix is therefore one whole-buffer replace, not a
// sorted edit splice — simpler and atomic.
//
// Obsidian/CM6 coupling stays behind the structural EditorLike
// interface so these helpers are testable with plain objects.

import type { Diagnostic, FixResult } from "./wasm-runtime";

// EditorLike is the structural subset of the active CM6 buffer
// applyFixToEditor drives. main.ts adapts the real EditorView to it.
export interface EditorLike {
  // length is the current document length in characters.
  length: number;
  // getValue returns the full current buffer text.
  getValue(): string;
  // dispatch applies one change. For a whole-buffer replace it receives
  // a single {from:0, to:length, insert:source} change.
  dispatch(change: { from: number; to: number; insert: string }): void;
}

// LineCommand is the transient palette entry derived from one active
// diagnostic on the cursor line. main.ts maps each to addCommand and
// clears the set on cursor move.
export interface LineCommand {
  id: string;
  name: string;
  diagnostic: Diagnostic;
}

// buildLineCommands returns one command per UNIQUE rule on the cursor
// line. Multiple diagnostics for the same rule on the same line
// collapse to one entry — a single fix pass handles all of them.
//
// cursorLine is 1-based to match the engine's diagnostic.line.
export function buildLineCommands(
  diagnostics: Diagnostic[],
  cursorLine: number,
): LineCommand[] {
  const seen = new Set<string>();
  const cmds: LineCommand[] = [];
  for (const d of diagnostics) {
    if (d.line !== cursorLine) continue;
    if (seen.has(d.rule)) continue;
    seen.add(d.rule);
    cmds.push({
      id: `mdsmith-fix-line-${d.rule}`,
      name: `mdsmith: Fix — ${d.rule}`,
      diagnostic: d,
    });
  }
  return cmds;
}

// applyFixToEditor replaces the whole buffer with the fixed source in
// one dispatch. It dispatches only when the fixed source actually
// differs from the current buffer: the changed flag is the engine's
// signal, but comparing bytes too avoids pushing an empty undo step if
// the flag is ever stale.
export function applyFixToEditor(
  editor: EditorLike,
  result: FixResult,
): void {
  if (!result.changed) return;
  const current = editor.getValue();
  if (result.source === current) return;
  editor.dispatch({ from: 0, to: editor.length, insert: result.source });
}

// DebouncedFn is debounce()'s return: a callable that schedules the
// wrapped fn, plus cancel() to drop a pending trailing call (used by
// onunload so a fix never fires after teardown).
export interface DebouncedFn<A extends unknown[]> {
  (...args: A): void;
  cancel(): void;
}

// debounce delays fn until delayMs has elapsed with no further call.
// Successive calls within the window reset the timer; only the trailing
// call runs, with the latest arguments. cancel() drops the pending
// timer. Used by fix-on-save to wait 200 ms after the last save.
export function debounce<A extends unknown[]>(
  fn: (...args: A) => void,
  delayMs: number,
): DebouncedFn<A> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const wrapped = (...args: A): void => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = undefined;
      fn(...args);
    }, delayMs);
  };
  wrapped.cancel = (): void => {
    if (timer) {
      clearTimeout(timer);
      timer = undefined;
    }
  };
  return wrapped;
}
