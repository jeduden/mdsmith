// Drives the CodeMirror 6 diagnostics layer.
//
// The layer maps the mdsmith engine's diagnostic wire shape (1-based
// line + 1-based UTF-16 column, string severity, related_locations)
// into CM6 primitives:
//   - severityClass: "error"/"warning" → the styles.css class.
//   - buildDecorations: one underline span per diagnostic.
//   - renderTooltip: the ISSUE-FIRST tooltip (message, then schema
//     constraint, then code + docs link, then a Fix link).
//
// Tests assert the adapters without the full CM6 runtime; test-setup.ts
// stubs @codemirror/state and @codemirror/view and the DOM.

import { describe, expect, test } from "bun:test";

import type { Diagnostic } from "./wasm-runtime";
import {
  buildDecorations,
  type DocLike,
  renderTooltip,
  severityClass,
} from "./diagnostics";

// fakeDoc gives the line/length surface buildDecorations consumes. CM6
// doc lines are 1-indexed; offsets are absolute UTF-16 positions.
function fakeDoc(lines: string[]): DocLike {
  return {
    length: lines.reduce((acc, l) => acc + l.length + 1, 0) - 1,
    line(n: number) {
      let from = 0;
      for (let i = 0; i < n - 1; i++) from += lines[i].length + 1;
      return { from, to: from + (lines[n - 1]?.length ?? 0) };
    },
    lines: lines.length,
  };
}

describe("severityClass", () => {
  test("maps the engine's string severities to styles.css classes", () => {
    expect(severityClass("error")).toBe("mdsmith-diagnostic-error");
    expect(severityClass("warning")).toBe("mdsmith-diagnostic-warning");
  });

  test("treats an unknown severity as warning so it stays visible", () => {
    expect(severityClass("info")).toBe("mdsmith-diagnostic-info");
    expect(severityClass("")).toBe("mdsmith-diagnostic-warning");
    expect(severityClass("nonsense")).toBe("mdsmith-diagnostic-warning");
  });
});

function diag(partial: Partial<Diagnostic>): Diagnostic {
  return {
    file: "a.md",
    line: 1,
    column: 1,
    rule: "MDS001",
    name: "line-length",
    severity: "warning",
    message: "line too long",
    ...partial,
  };
}

describe("buildDecorations", () => {
  test("emits one underline span per diagnostic with the severity class", () => {
    const doc = fakeDoc(["hello world", "second line"]);
    const specs = buildDecorations(doc, [
      diag({ line: 1, column: 1, severity: "error", rule: "MDS001" }),
    ]);
    expect(specs.length).toBe(1);
    expect(specs[0].spec.class).toContain("mdsmith-diagnostic");
    expect(specs[0].spec.class).toContain("mdsmith-diagnostic-error");
    expect(specs[0].spec.attributes["data-mdsmith-rule"]).toBe("MDS001");
    // The span starts at the column and covers to end of line at least.
    expect(specs[0].from).toBe(0);
    expect(specs[0].to).toBeGreaterThan(0);
  });

  test("maps a 1-based column on line 2 to the right document offset", () => {
    const doc = fakeDoc(["abc", "defgh"]); // line 2 starts at offset 4
    const specs = buildDecorations(doc, [diag({ line: 2, column: 3 })]);
    // column 3 (1-based) on line 2 → offset 4 + 2 = 6.
    expect(specs[0].from).toBe(6);
  });

  test("clamps a stale out-of-range diagnostic instead of throwing", () => {
    const doc = fakeDoc(["short"]);
    const specs = buildDecorations(doc, [diag({ line: 99, column: 1 })]);
    // Past EOF collapses to a zero-width span, which is dropped.
    expect(specs.length).toBe(0);
  });

  test("sorts spans by start offset", () => {
    const doc = fakeDoc(["aaaa", "bbbb", "cccc"]);
    const specs = buildDecorations(doc, [
      diag({ line: 3, column: 1 }),
      diag({ line: 1, column: 1 }),
      diag({ line: 2, column: 1 }),
    ]);
    const froms = specs.map((s) => s.from);
    expect(froms).toEqual([...froms].sort((a, b) => a - b));
  });
});

describe("renderTooltip — issue first (plan 230)", () => {
  test("orders message, then code + docs link, then Fix", () => {
    const node = renderTooltip(
      diag({ rule: "MDS001", message: "line too long (99 > 80)" }),
      () => {},
    ) as unknown as {
      querySelector(s: string): { textContent: string } | null;
      children: Array<{ className: string }>;
    };
    const message = node.querySelector(".mdsmith-tooltip-message");
    const meta = node.querySelector(".mdsmith-tooltip-meta");
    expect(message?.textContent).toBe("line too long (99 > 80)");
    expect(meta?.textContent).toContain("MDS001");

    // The message row comes before the meta row in DOM order.
    const order = node.children.map((c) => c.className);
    const msgIdx = order.findIndex((c) => c.includes("tooltip-message"));
    const metaIdx = order.findIndex((c) => c.includes("tooltip-meta"));
    expect(msgIdx).toBeGreaterThanOrEqual(0);
    expect(metaIdx).toBeGreaterThan(msgIdx);
  });

  test("renders a schema constraint from related_locations between message and meta", () => {
    const node = renderTooltip(
      diag({
        rule: "MDS020",
        message: "section out of order",
        related_locations: [
          { file: "proto.md", line: 12, message: "expected ## Tasks here" },
        ],
      }),
      () => {},
    ) as unknown as {
      querySelector(s: string): { textContent: string } | null;
      children: Array<{ className: string }>;
    };
    const related = node.querySelector(".mdsmith-tooltip-related");
    expect(related?.textContent).toContain("expected ## Tasks here");

    const order = node.children.map((c) => c.className);
    const msgIdx = order.findIndex((c) => c.includes("tooltip-message"));
    const relIdx = order.findIndex((c) => c.includes("tooltip-related"));
    const metaIdx = order.findIndex((c) => c.includes("tooltip-meta"));
    expect(relIdx).toBeGreaterThan(msgIdx);
    expect(metaIdx).toBeGreaterThan(relIdx);
  });

  test("a related location with a file/line renders as a navigable link", () => {
    let navigated: { file?: string; line?: number } | undefined;
    const node = renderTooltip(
      diag({
        related_locations: [
          { file: "proto.md", line: 12, message: "schema says X" },
        ],
      }),
      () => {},
      (loc) => {
        navigated = { file: loc.file, line: loc.line };
      },
    ) as unknown as {
      querySelector(s: string): { click(): void } | null;
    };
    const link = node.querySelector(".mdsmith-tooltip-related-link");
    expect(link).not.toBeNull();
    link?.click();
    expect(navigated).toEqual({ file: "proto.md", line: 12 });
  });

  test("a related location with no file/line renders as plain text, not a link", () => {
    const node = renderTooltip(
      diag({ related_locations: [{ message: "just a note" }] }),
      () => {},
    ) as unknown as {
      querySelector(s: string): unknown | null;
    };
    expect(node.querySelector(".mdsmith-tooltip-related-link")).toBeNull();
    const related = node.querySelector(".mdsmith-tooltip-related") as {
      textContent: string;
    } | null;
    expect(related?.textContent).toContain("just a note");
  });

  test("the Fix link fires the supplied callback", () => {
    let fixed = false;
    const node = renderTooltip(diag({}), () => {
      fixed = true;
    }) as unknown as { querySelector(s: string): { click(): void } | null };
    const fix = node.querySelector(".mdsmith-tooltip-fix");
    expect(fix).not.toBeNull();
    fix?.click();
    expect(fixed).toBe(true);
  });

  test("the meta row links to the rule's docs page", () => {
    const node = renderTooltip(
      diag({ rule: "MDS001" }),
      () => {},
    ) as unknown as {
      querySelector(s: string): { attrs?: Record<string, string> } | null;
    };
    const docs = node.querySelector(".mdsmith-tooltip-docs");
    expect(docs).not.toBeNull();
    // The href points at the rule's published page; the docs slug is
    // the lower-cased id + name (mirrors the LSP hover-link convention).
    expect(docs?.attrs?.href).toBe("https://mdsmith.dev/rules/mds001-line-length/");
  });

  test("the Fix control is keyboard-accessible: role, tabindex, and Enter activates it", () => {
    let fixes = 0;
    const node = renderTooltip(diag({}), () => {
      fixes++;
    }) as unknown as {
      querySelector(s: string): {
        attrs?: Record<string, string>;
        dispatchEvent(name: string, init?: Record<string, unknown>): void;
      } | null;
    };
    const fix = node.querySelector(".mdsmith-tooltip-fix");
    expect(fix?.attrs?.role).toBe("button");
    expect(fix?.attrs?.tabindex).toBe("0");
    fix?.dispatchEvent("keydown", { key: "Enter" });
    expect(fixes).toBe(1);
  });

  test("the related-location link is keyboard-accessible: role, tabindex, and Enter navigates", () => {
    let navigated: { file?: string; line?: number } | undefined;
    const node = renderTooltip(
      diag({
        related_locations: [
          { file: "proto.md", line: 12, message: "schema says X" },
        ],
      }),
      () => {},
      (loc) => {
        navigated = { file: loc.file, line: loc.line };
      },
    ) as unknown as {
      querySelector(s: string): {
        attrs?: Record<string, string>;
        dispatchEvent(name: string, init?: Record<string, unknown>): void;
      } | null;
    };
    const link = node.querySelector(".mdsmith-tooltip-related-link");
    expect(link?.attrs?.role).toBe("button");
    expect(link?.attrs?.tabindex).toBe("0");
    link?.dispatchEvent("keydown", { key: "Enter" });
    expect(navigated).toEqual({ file: "proto.md", line: 12 });
  });
});
