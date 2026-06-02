// Drives the action surface: per-line palette commands, whole-buffer
// fix application, and the debounce used by fix-on-save.
//
// The WASM engine's fix returns the FULL rewritten document
// (FixResult.source), not edit ranges, so applying a fix is a single
// whole-buffer replace. These tests use a recording editor + plain
// data — no CM6 or Obsidian host.

import { describe, expect, test } from "bun:test";

import type { Diagnostic, FixResult } from "./wasm-runtime";
import {
  applyFixToEditor,
  buildLineCommands,
  debounce,
  type EditorLike,
} from "./actions";

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

const sleep = (ms: number): Promise<void> =>
  new Promise((r) => setTimeout(r, ms));

describe("buildLineCommands", () => {
  test("one command per unique rule on the cursor line (1-based)", () => {
    const diags = [
      diag({ line: 3, rule: "MDS001" }),
      diag({ line: 3, rule: "MDS009" }),
      diag({ line: 3, rule: "MDS001" }), // dup rule on same line → collapsed
      diag({ line: 4, rule: "MDS012" }), // other line → excluded
    ];
    const cmds = buildLineCommands(diags, 3);
    expect(cmds.map((c) => c.diagnostic.rule)).toEqual(["MDS001", "MDS009"]);
    expect(cmds[0].id).toBe("mdsmith-fix-line-MDS001");
    expect(cmds[0].name).toBe("mdsmith: Fix — MDS001");
  });

  test("returns nothing when no diagnostic sits on the cursor line", () => {
    expect(buildLineCommands([diag({ line: 1 })], 5)).toEqual([]);
  });
});

describe("applyFixToEditor", () => {
  test("replaces the whole buffer with the fixed source", () => {
    const dispatched: Array<{ from: number; to: number; insert: string }> = [];
    const editor: EditorLike = {
      length: 10,
      getValue: () => "old content",
      dispatch: (c) => dispatched.push(c),
    };
    const res: FixResult = {
      source: "new content\n",
      changed: true,
      diagnostics: [],
    };
    applyFixToEditor(editor, res);
    expect(dispatched).toEqual([
      { from: 0, to: 10, insert: "new content\n" },
    ]);
  });

  test("is a no-op when the fix changed nothing", () => {
    const dispatched: unknown[] = [];
    const editor: EditorLike = {
      length: 5,
      getValue: () => "same\n",
      dispatch: (c) => dispatched.push(c),
    };
    applyFixToEditor(editor, {
      source: "same\n",
      changed: false,
      diagnostics: [],
    });
    expect(dispatched.length).toBe(0);
  });

  test("does not dispatch when source equals the buffer despite changed=true", () => {
    // Defensive: if changed is somehow stale, comparing bytes avoids a
    // pointless dispatch that would push an undo step for no edit.
    const dispatched: unknown[] = [];
    const editor: EditorLike = {
      length: 5,
      getValue: () => "same\n",
      dispatch: (c) => dispatched.push(c),
    };
    applyFixToEditor(editor, {
      source: "same\n",
      changed: true,
      diagnostics: [],
    });
    expect(dispatched.length).toBe(0);
  });
});

describe("debounce", () => {
  test("collapses rapid calls into one trailing invocation", async () => {
    let calls = 0;
    let lastArg = "";
    const fn = debounce((arg: string) => {
      calls++;
      lastArg = arg;
    }, 40);
    fn("a");
    fn("b");
    fn("c");
    expect(calls).toBe(0);
    await sleep(70);
    expect(calls).toBe(1);
    expect(lastArg).toBe("c");
  });

  test("cancel() drops a pending call", async () => {
    let calls = 0;
    const fn = debounce(() => {
      calls++;
    }, 40);
    fn();
    fn.cancel();
    await sleep(70);
    expect(calls).toBe(0);
  });
});
