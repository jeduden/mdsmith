// CodeMirror 6 diagnostics layer for the Obsidian plugin.
//
// Obsidian's source and live-preview editors both run on CM6. This
// module:
//
//   - Maps LSP `textDocument/publishDiagnostics` payloads into CM6
//     primitives (severity → CSS class, range → document offsets,
//     decoration → underline).
//   - Exposes a `StateEffect` the LSP fan-out uses to push new
//     diagnostic sets into the editor.
//   - Owns a `StateField<DecorationSet>` that recomputes the
//     decorations on every effect dispatch.
//   - Renders a tooltip DOM node holding the rule code, the message,
//     and a "Fix" link the actions module wires up.
//
// The CM6 imports come from packages Obsidian provides at runtime;
// tests load test-setup.ts which stubs the surface we touch.

import { StateEffect, StateField } from "@codemirror/state";
import { Decoration, EditorView, hoverTooltip } from "@codemirror/view";

// LspDiagnostic is the wire shape we accept. Mirrors the LSP base
// protocol's Diagnostic — narrowed to the fields we render.
export interface LspDiagnostic {
  range: {
    start: { line: number; character: number };
    end: { line: number; character: number };
  };
  message: string;
  // LSP severity: 1=error, 2=warning, 3=info, 4=hint. Missing maps
  // to "warning" — see classForSeverity.
  severity?: number;
  // Rule id (e.g. "MDS001"). Optional because the LSP spec allows
  // string|number|undefined; mdsmith always sends strings.
  code?: string;
  source?: string;
}

// DocLike captures the subset of CM6's `EditorState.doc` the offset
// mapper reads. Used directly in unit tests so we don't have to spin
// up a full CM6 EditorState.
export interface DocLike {
  length: number;
  lineAt(pos: number): { from: number };
}

// classForSeverity maps the LSP severity scale to the CSS classes
// declared in styles.css. The CM6 decoration applies both the base
// "mdsmith-diagnostic" class (for the wavy underline) and the
// severity-specific one (for the color).
export function classForSeverity(sev: number | undefined): string {
  switch (sev) {
    case 1:
      return "mdsmith-diagnostic-error";
    case 3:
      return "mdsmith-diagnostic-info";
    case 4:
      return "mdsmith-diagnostic-hint";
    case 2:
    default:
      // Missing or unknown severity falls to "warning" so the user
      // still sees the diagnostic — silently dropping would hide
      // any rule that forgets to set severity.
      return "mdsmith-diagnostic-warning";
  }
}

// DecorationSpec is the structural shape buildDecorations returns.
// At runtime it's converted into a CM6 Decoration; in tests we
// inspect the shape directly.
export interface DecorationSpec {
  from: number;
  to: number;
  spec: {
    class: string;
    attributes: Record<string, string>;
  };
}

// docOffset converts an LSP {line, character} to an absolute document
// offset, using CM6's line-indexing API. Clamps both axes to the
// document length so a stale diagnostic (range past EOF after a fix
// in flight) does not throw.
function docOffset(
  doc: DocLike,
  pos: { line: number; character: number },
  lines: string[] | undefined,
): number {
  // The structural DocLike does not expose `line(n)` here; the test
  // helper passes its own `lines` array via the `lines` parameter
  // for the fake doc, and production code passes undefined so we
  // fall back to lineAt() math.
  if (lines) {
    // Line index past EOF clamps to doc.length so an out-of-bounds
    // range collapses to a zero-width span (dropped by the caller).
    if (pos.line < 0 || pos.line >= lines.length) {
      return doc.length;
    }
    let cursor = 0;
    for (let i = 0; i < pos.line; i++) {
      cursor += lines[i].length + 1;
    }
    const lineLen = lines[pos.line].length;
    cursor += Math.min(Math.max(pos.character, 0), lineLen);
    return Math.min(cursor, doc.length);
  }
  // Production path uses the CM6 EditorState.doc, which exposes
  // `line(number)` (1-indexed) — re-imported here as a structural
  // shape on DocLike via casting.
  type RealDoc = DocLike & {
    line: (n: number) => { from: number; length: number };
  };
  const real = doc as RealDoc;
  if (typeof real.line === "function") {
    const lineNum = Math.min(Math.max(pos.line + 1, 1), doc.length);
    let info: { from: number; length: number };
    try {
      info = real.line(lineNum);
    } catch {
      return doc.length;
    }
    return Math.min(info.from + Math.max(pos.character, 0), doc.length);
  }
  // Last-resort fallback: clamp to doc.length so we never throw.
  return Math.min(pos.character, doc.length);
}

// buildDecorations turns the diagnostics for a buffer into structured
// decoration specs sorted by `from`. Zero-length spans are dropped —
// CM6 underlines need at least one character of width.
export function buildDecorations(
  doc: DocLike & { lines?: string[] },
  diagnostics: LspDiagnostic[],
): DecorationSpec[] {
  const lines = (doc as DocLike & { lines?: string[] }).lines;
  const specs: DecorationSpec[] = [];
  for (const d of diagnostics) {
    const from = docOffset(doc, d.range.start, lines);
    const to = docOffset(doc, d.range.end, lines);
    if (to <= from) continue;
    const severity = d.severity ?? 2;
    specs.push({
      from,
      to,
      spec: {
        class: `mdsmith-diagnostic ${classForSeverity(d.severity)}`,
        attributes: {
          "data-mdsmith-code": d.code ?? "",
          "data-mdsmith-severity": String(severity),
        },
      },
    });
  }
  specs.sort((a, b) => a.from - b.from || a.to - b.to);
  return specs;
}

// renderTooltip composes the DOM node CM6 renders next to a hovered
// diagnostic. The tooltip carries the rule code, the message, and —
// when the diagnostic has a code we can request a quick-fix for — a
// "Fix" link that fires the supplied callback.
//
// The callback shape lets actions.ts inject the
// textDocument/codeAction round-trip without coupling diagnostics.ts
// to the LSP client.
export function renderTooltip(
  diagnostic: LspDiagnostic,
  onFix: () => void,
): HTMLElement {
  const container = document.createElement("div");
  container.className = "mdsmith-tooltip";

  if (diagnostic.code) {
    const code = document.createElement("span");
    code.className = "mdsmith-tooltip-code";
    code.textContent = diagnostic.code;
    container.appendChild(code);
  }

  const msg = document.createElement("span");
  msg.className = "mdsmith-tooltip-message";
  msg.textContent = diagnostic.message;
  container.appendChild(msg);

  if (diagnostic.code) {
    const linkRow = document.createElement("div");
    const fix = document.createElement("a");
    fix.className = "mdsmith-tooltip-fix";
    fix.textContent = "Fix";
    fix.addEventListener("click", (e) => {
      e.preventDefault();
      onFix();
    });
    linkRow.appendChild(fix);
    container.appendChild(linkRow);
  }

  return container;
}

// setDiagnostics is the CM6 effect the LSP fan-out dispatches when a
// new `publishDiagnostics` notification arrives. The state field
// listens for it.
export const setDiagnostics = StateEffect.define<LspDiagnostic[]>();

// diagnosticsField holds the current diagnostics for the editor.
// Stored as the raw LSP payload; rendering happens in the view layer.
export const diagnosticsField: StateField<LspDiagnostic[]> =
  StateField.define<LspDiagnostic[]>({
    create() {
      return [];
    },
    update(value, tr) {
      let next = value;
      const trWithEffects = tr as unknown as {
        effects?: Array<{ is(t: unknown): boolean; value: LspDiagnostic[] }>;
      };
      const effects = trWithEffects.effects;
      if (effects) {
        for (const e of effects) {
          if (e.is(setDiagnostics)) {
            next = e.value;
          }
        }
      }
      return next;
    },
  });

// makeHoverTooltip wires CM6's hoverTooltip provider to the
// diagnostic at the cursor (if any). The `onFix` callback is invoked
// when the user clicks the Fix link in the rendered tooltip.
export function makeHoverTooltip(
  onFix: (d: LspDiagnostic) => void,
): unknown {
  return hoverTooltip((view, pos, _side) => {
    const stateWithField = view.state as unknown as {
      field(f: typeof diagnosticsField): LspDiagnostic[];
    };
    const list = stateWithField.field(diagnosticsField);
    if (!list || list.length === 0) return null;
    const doc = view.state.doc as DocLike;
    const hit = list.find((d) => {
      const from = docOffset(doc, d.range.start, undefined);
      const to = docOffset(doc, d.range.end, undefined);
      return pos >= from && pos <= to;
    });
    if (!hit) return null;
    return {
      pos,
      end: pos,
      above: true,
      create: () => ({
        dom: renderTooltip(hit, () => onFix(hit)),
      }),
    };
  });
}

// editorExtensions returns the CM6 extension array the plugin
// registers with Obsidian. Bundles the state field plus the hover
// tooltip so callers wire diagnostics with one call.
export function editorExtensions(onFix: (d: LspDiagnostic) => void): unknown[] {
  return [
    diagnosticsField,
    EditorView.decorations.from(diagnosticsField, () => Decoration.none),
    makeHoverTooltip(onFix),
  ];
}
