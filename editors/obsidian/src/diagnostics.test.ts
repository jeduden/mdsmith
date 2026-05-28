// Drives the CodeMirror 6 diagnostics layer.
//
// The layer owns four pieces: a `StateEffect` carrying the latest
// LSP `publishDiagnostics` payload, a `StateField` that holds the
// current diagnostic set per file, a `RangeSetBuilder` invocation
// that emits one Decoration per diagnostic, and a `hoverTooltip` that
// renders the rule code, the message, and a "Fix" link.
//
// The tests verify the *adapters* — the transform from LSP wire shape
// into CM6 primitives — without needing the full CM6 runtime. The
// test-setup preload registers shim modules for @codemirror/state and
// @codemirror/view so source imports resolve.

import { describe, expect, test } from "bun:test";

import {
  buildDecorations,
  classForSeverity,
  type LspDiagnostic,
  renderTooltip,
} from "./diagnostics";

// fakeDoc just gives lineAt/length on the structural CM6 EditorState
// shape buildDecorations consumes. Each LSP position maps to a doc
// offset; the simplest mapping is "byte-per-line × line + character".
function fakeDoc(lines: string[]): {
  lines: string[];
  lineAt(pos: number): { from: number; to: number; number: number };
  length: number;
} {
  return {
    lines,
    length: lines.reduce((acc, l) => acc + l.length + 1, 0),
    lineAt(pos: number) {
      let cursor = 0;
      for (let i = 0; i < lines.length; i++) {
        const end = cursor + lines[i].length;
        if (pos <= end) {
          return { from: cursor, to: end, number: i + 1 };
        }
        cursor = end + 1; // newline
      }
      const last = lines.length - 1;
      const lastFrom = cursor - lines[last].length - 1;
      return { from: lastFrom, to: cursor - 1, number: lines.length };
    },
  };
}

describe("classForSeverity", () => {
  test("maps LSP severities 1-4 to the styles.css classes", () => {
    expect(classForSeverity(1)).toBe("mdsmith-diagnostic-error");
    expect(classForSeverity(2)).toBe("mdsmith-diagnostic-warning");
    expect(classForSeverity(3)).toBe("mdsmith-diagnostic-info");
    expect(classForSeverity(4)).toBe("mdsmith-diagnostic-hint");
  });

  test("treats an unknown or missing severity as warning", () => {
    // The LSP spec says missing severity falls to the client. Other
    // mdsmith surfaces default to "warning" for missing/unknown.
    expect(classForSeverity(undefined)).toBe("mdsmith-diagnostic-warning");
    expect(classForSeverity(0)).toBe("mdsmith-diagnostic-warning");
    expect(classForSeverity(99)).toBe("mdsmith-diagnostic-warning");
  });
});

describe("buildDecorations", () => {
  test("emits one decoration per diagnostic with the severity class", () => {
    const doc = fakeDoc(["hello world", "second line"]);
    const diagnostics: LspDiagnostic[] = [
      {
        range: {
          start: { line: 0, character: 0 },
          end: { line: 0, character: 5 },
        },
        message: "boom",
        severity: 1,
        code: "MDS001",
      },
      {
        range: {
          start: { line: 1, character: 7 },
          end: { line: 1, character: 11 },
        },
        message: "ohno",
        severity: 2,
        code: "MDS002",
      },
    ];
    const decorations = buildDecorations(doc, diagnostics);
    // Expect one decoration per diagnostic, mapped to absolute
    // document offsets. "hello world\n" → newline at offset 11, so
    // line 1 char 7 lands at 12 + 7 = 19. End is 12 + 11 = 23.
    expect(decorations).toEqual([
      {
        from: 0,
        to: 5,
        spec: {
          class: "mdsmith-diagnostic mdsmith-diagnostic-error",
          attributes: {
            "data-mdsmith-code": "MDS001",
            "data-mdsmith-severity": "1",
          },
        },
      },
      {
        from: 19,
        to: 23,
        spec: {
          class: "mdsmith-diagnostic mdsmith-diagnostic-warning",
          attributes: {
            "data-mdsmith-code": "MDS002",
            "data-mdsmith-severity": "2",
          },
        },
      },
    ]);
  });

  test("drops zero-length ranges (cannot underline an empty span)", () => {
    const doc = fakeDoc(["hello"]);
    const decorations = buildDecorations(doc, [
      {
        range: {
          start: { line: 0, character: 2 },
          end: { line: 0, character: 2 },
        },
        message: "empty",
        severity: 2,
      },
    ]);
    expect(decorations).toEqual([]);
  });

  test("clamps an out-of-bounds range to the document length", () => {
    // The LSP server occasionally emits a range past the end of the
    // buffer (e.g. when a fix narrowed the file mid-flight). CM6
    // throws if we hand it an offset past doc.length; clamp so the
    // diagnostic at least renders at the last character.
    const doc = fakeDoc(["short"]);
    const decorations = buildDecorations(doc, [
      {
        range: {
          start: { line: 5, character: 0 },
          end: { line: 5, character: 99 },
        },
        message: "stale",
        severity: 2,
      },
    ]);
    // Both clamped to doc.length; that's a zero-width range, which
    // we drop. The point is no throw.
    expect(decorations).toEqual([]);
  });
});

describe("renderTooltip", () => {
  test("returns a DOM node with the code, message, and Fix link", () => {
    const diagnostic: LspDiagnostic = {
      range: {
        start: { line: 0, character: 0 },
        end: { line: 0, character: 4 },
      },
      message: "trailing whitespace",
      severity: 2,
      code: "MDS017",
    };
    const fixes: string[] = [];
    const dom = renderTooltip(diagnostic, () => fixes.push("ran"));
    // The tooltip should hold the code in a styled span and the
    // message verbatim. Click on the Fix link calls back into the
    // handler we provided so the actions module can wire it up to
    // textDocument/codeAction.
    const text = dom.textContent ?? "";
    expect(text).toContain("MDS017");
    expect(text).toContain("trailing whitespace");
    const fixLink = dom.querySelector(".mdsmith-tooltip-fix");
    expect(fixLink).not.toBeNull();
    (fixLink as HTMLElement)?.click();
    expect(fixes).toEqual(["ran"]);
  });

  test("omits the Fix link when no fix is wired (diagnostic without code)", () => {
    const diagnostic: LspDiagnostic = {
      range: {
        start: { line: 0, character: 0 },
        end: { line: 0, character: 4 },
      },
      message: "informational",
      severity: 3,
    };
    const dom = renderTooltip(diagnostic, () => {});
    // No `code`, no Fix link — there's nothing to ask the server for
    // since the user has no rule id to act on.
    expect(dom.querySelector(".mdsmith-tooltip-fix")).toBeNull();
    expect(dom.textContent).toContain("informational");
  });
});
