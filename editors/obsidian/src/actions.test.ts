// Drives the code-action surface.
//
// Obsidian has no lightbulb. The plugin substitutes three entry
// points (see plan 214 task 5):
//   1. The hover tooltip "Fix" link (wired in diagnostics.ts).
//   2. Per-line palette commands derived from the active diagnostics
//      on the cursor line. The set rebuilds on cursor move.
//   3. The "mdsmith: Fix file" command that asks the server for the
//      `source.fixAll.mdsmith` action and applies the returned
//      WorkspaceEdit to the active buffer.
//
// fixOnSave debounces vault.on('modify') and triggers the same
// Fix file command 200 ms after the last save.

import { describe, expect, test } from "bun:test";

import {
  applyWorkspaceEdit,
  buildLineCommands,
  debounce,
  type LspDiagnostic,
  type WorkspaceEdit,
} from "./actions";

describe("buildLineCommands", () => {
  test("emits one entry per diagnostic on the cursor line", () => {
    const cursorLine = 3;
    const diagnostics: LspDiagnostic[] = [
      {
        range: { start: { line: 3, character: 0 }, end: { line: 3, character: 4 } },
        message: "no trailing whitespace",
        code: "MDS017",
        severity: 2,
      },
      {
        range: { start: { line: 3, character: 6 }, end: { line: 3, character: 10 } },
        message: "line too long",
        code: "MDS013",
        severity: 2,
      },
      // Off-line diagnostic — must not contribute.
      {
        range: { start: { line: 5, character: 0 }, end: { line: 5, character: 4 } },
        message: "elsewhere",
        code: "MDS001",
        severity: 2,
      },
    ];
    const cmds = buildLineCommands(diagnostics, cursorLine);
    expect(cmds).toEqual([
      { id: "mdsmith-fix-line-MDS017", name: "mdsmith: Fix — MDS017", diagnostic: diagnostics[0] },
      { id: "mdsmith-fix-line-MDS013", name: "mdsmith: Fix — MDS013", diagnostic: diagnostics[1] },
    ]);
  });

  test("dedupes per code so the same rule on a line shows once", () => {
    const diagnostics: LspDiagnostic[] = [
      {
        range: { start: { line: 0, character: 0 }, end: { line: 0, character: 2 } },
        message: "first",
        code: "MDS017",
        severity: 2,
      },
      {
        range: { start: { line: 0, character: 5 }, end: { line: 0, character: 7 } },
        message: "second",
        code: "MDS017",
        severity: 2,
      },
    ];
    const cmds = buildLineCommands(diagnostics, 0);
    expect(cmds.length).toBe(1);
    expect(cmds[0].id).toBe("mdsmith-fix-line-MDS017");
  });

  test("skips diagnostics that have no code (no rule to ask for)", () => {
    const cmds = buildLineCommands(
      [
        {
          range: { start: { line: 0, character: 0 }, end: { line: 0, character: 2 } },
          message: "no code",
          severity: 2,
        },
      ],
      0,
    );
    expect(cmds).toEqual([]);
  });
});

describe("applyWorkspaceEdit", () => {
  test("applies every text edit for the matching uri in order", () => {
    const ops: Array<{ from: number; to: number; insert: string }> = [];
    const editor = {
      uri: "file:///vault/a.md",
      // CM6-flavored interface: offsetAt / lineAt / dispatch
      offsetAt: ({ line, character }: { line: number; character: number }) => {
        // Two-line doc with newline at 5: line 0 chars 0-5, line 1 starts at 6
        return line * 6 + character;
      },
      dispatch(changes: Array<{ from: number; to: number; insert: string }>) {
        ops.push(...changes);
      },
    };
    const edit: WorkspaceEdit = {
      changes: {
        "file:///vault/a.md": [
          {
            range: { start: { line: 0, character: 0 }, end: { line: 0, character: 5 } },
            newText: "hello",
          },
          {
            range: { start: { line: 1, character: 0 }, end: { line: 1, character: 5 } },
            newText: "world",
          },
        ],
        "file:///vault/other.md": [
          {
            range: { start: { line: 0, character: 0 }, end: { line: 0, character: 1 } },
            newText: "nope",
          },
        ],
      },
    };
    applyWorkspaceEdit(editor, edit);
    // Only the matching uri's edits should reach dispatch, in the
    // bottom-up order LSP TextEdit application requires (later
    // positions first so earlier offsets don't shift).
    expect(ops).toEqual([
      { from: 6, to: 11, insert: "world" },
      { from: 0, to: 5, insert: "hello" },
    ]);
  });

  test("is a no-op when the edit has no entry for this buffer's uri", () => {
    const ops: Array<unknown> = [];
    const editor = {
      uri: "file:///vault/a.md",
      offsetAt: () => 0,
      dispatch(changes: unknown[]) {
        ops.push(...changes);
      },
    };
    applyWorkspaceEdit(editor, {
      changes: {
        "file:///vault/elsewhere.md": [],
      },
    });
    expect(ops).toEqual([]);
  });

  test("accepts the documentChanges shape too", () => {
    const ops: Array<{ from: number; to: number; insert: string }> = [];
    const editor = {
      uri: "file:///vault/a.md",
      offsetAt: ({ character }: { line: number; character: number }) => character,
      dispatch(changes: Array<{ from: number; to: number; insert: string }>) {
        ops.push(...changes);
      },
    };
    applyWorkspaceEdit(editor, {
      documentChanges: [
        {
          textDocument: { uri: "file:///vault/a.md", version: 1 },
          edits: [
            {
              range: {
                start: { line: 0, character: 1 },
                end: { line: 0, character: 4 },
              },
              newText: "xx",
            },
          ],
        },
      ],
    });
    expect(ops).toEqual([{ from: 1, to: 4, insert: "xx" }]);
  });
});

describe("debounce", () => {
  test("calls the wrapped function only after the delay elapses", async () => {
    let calls = 0;
    const fn = debounce(() => {
      calls++;
    }, 50);
    fn();
    fn();
    fn();
    // Immediately after the burst, nothing has run yet.
    expect(calls).toBe(0);
    await new Promise((r) => setTimeout(r, 80));
    // Exactly one trailing call after the burst.
    expect(calls).toBe(1);
  });

  test("forwards the latest arguments to the wrapped function", async () => {
    let received: number | undefined;
    const fn = debounce((n: number) => {
      received = n;
    }, 20);
    fn(1);
    fn(2);
    fn(3);
    await new Promise((r) => setTimeout(r, 60));
    expect(received).toBe(3);
  });

  test("cancel() drops the pending call", async () => {
    let calls = 0;
    const fn = debounce(() => {
      calls++;
    }, 20);
    fn();
    fn.cancel();
    await new Promise((r) => setTimeout(r, 60));
    expect(calls).toBe(0);
  });
});
