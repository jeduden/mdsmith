// CodeMirror 6 diagnostics layer for the Obsidian plugin.
//
// Obsidian's source and live-preview editors both run on CM6. This
// module:
//
//   - Maps the mdsmith engine's diagnostic wire shape (1-based line +
//     1-based UTF-16 column, string severity, related_locations) into
//     CM6 primitives: a severity CSS class, a document-offset span, and
//     an underline decoration.
//   - Exposes a StateEffect the change fan-out dispatches to push a new
//     diagnostic set into an editor, and a StateField that holds the
//     current set.
//   - Renders the ISSUE-FIRST hover tooltip (plan 230): the message
//     leads, then the schema constraint from related_locations (a
//     navigable link when it has a file/line, else plain text), then
//     the rule code and a docs link, then a Fix link.
//
// The CM6 imports come from packages Obsidian provides at runtime;
// tests load test-setup.ts which stubs the surface we touch.

import { StateEffect, StateField } from "@codemirror/state";
import { Decoration, EditorView, hoverTooltip } from "@codemirror/view";

import type { Diagnostic, RelatedLocation } from "./wasm-runtime";

// DOCS_BASE is the published rule-docs host. The engine and every
// editor link rule pages under /rules/<id>-<name>/ (see
// editors/vscode/src/commands/rule-doc.ts). mdsmith never fetches it at
// runtime; the link is for the user's browser.
const DOCS_BASE = "https://mdsmith.dev/rules";

// DocLike is the slice of CM6's EditorState.doc the offset mapper reads.
// CM6 numbers lines from 1; line(n) returns the line's start/end
// offsets. Used directly in unit tests so we need no full EditorState.
export interface DocLike {
  length: number;
  line(n: number): { from: number; to: number };
  lines: number;
}

// severityClass maps the engine's string severity to the styles.css
// classes. The engine emits "error" | "warning"; an unknown or empty
// value falls to "warning" so a diagnostic is never silently dropped.
// "info" is accepted for forward-compatibility with a future severity.
export function severityClass(severity: string): string {
  switch (severity) {
    case "error":
      return "mdsmith-diagnostic-error";
    case "info":
      return "mdsmith-diagnostic-info";
    case "warning":
      return "mdsmith-diagnostic-warning";
    default:
      return "mdsmith-diagnostic-warning";
  }
}

// DecorationSpec is the structural shape buildDecorations returns. At
// runtime it becomes a CM6 Decoration.mark; tests inspect it directly.
export interface DecorationSpec {
  from: number;
  to: number;
  spec: {
    class: string;
    attributes: Record<string, string>;
  };
}

// lineColToOffset converts a 1-based line + 1-based UTF-16 column to an
// absolute document offset. Both axes clamp to the document so a stale
// diagnostic (range past EOF after an in-flight edit) never throws. A
// line past the end returns doc.length (a zero-width span the caller
// drops).
function lineColToOffset(
  doc: DocLike,
  line: number,
  column: number,
): number {
  if (line < 1 || line > doc.lines) return doc.length;
  let info: { from: number; to: number };
  try {
    info = doc.line(line);
  } catch {
    return doc.length;
  }
  // column is 1-based; column 1 is the line start. Clamp to the line so
  // a column past EOL does not spill into the next line.
  const col = Math.max(column - 1, 0);
  const at = info.from + Math.min(col, info.to - info.from);
  return Math.min(at, doc.length);
}

// buildDecorations turns a buffer's diagnostics into underline spans
// sorted by start offset. Each span runs from the diagnostic's column
// to end of line (the engine reports a point, not a range; underlining
// to EOL gives the user a visible target). Zero-length spans are
// dropped — CM6 marks need at least one character of width.
export function buildDecorations(
  doc: DocLike,
  diagnostics: Diagnostic[],
): DecorationSpec[] {
  const specs: DecorationSpec[] = [];
  for (const d of diagnostics) {
    const from = lineColToOffset(doc, d.line, d.column);
    // Underline from the column to end of the diagnostic's line.
    let to = from;
    if (d.line >= 1 && d.line <= doc.lines) {
      try {
        to = doc.line(d.line).to;
      } catch {
        to = from;
      }
    }
    // A point at EOL (or a clamped stale diagnostic) collapses; extend
    // by one so a real, last-column issue still shows a mark.
    if (to <= from) {
      if (from < doc.length) to = from + 1;
      else continue;
    }
    specs.push({
      from,
      to,
      spec: {
        class: `mdsmith-diagnostic ${severityClass(d.severity)}`,
        attributes: {
          "data-mdsmith-rule": d.rule,
          "data-mdsmith-severity": d.severity,
        },
      },
    });
  }
  specs.sort((a, b) => a.from - b.from || a.to - b.to);
  return specs;
}

// ruleDocUrl builds the published docs URL for a diagnostic's rule. The
// slug is "<lowercased-id>-<name>" — the same form the LSP hover links
// use — so the link lands on the rule's page.
export function ruleDocUrl(d: Diagnostic): string {
  const slug = d.name
    ? `${d.rule.toLowerCase()}-${d.name}`
    : d.rule.toLowerCase();
  return `${DOCS_BASE}/${slug}/`;
}

// NavigateFn opens a related location (e.g. a schema constraint in a
// proto.md) in the editor. Supplied by actions.ts; absent in a tooltip
// that only needs the message + Fix link.
export type NavigateFn = (loc: RelatedLocation) => void;

// setAttr sets an attribute via a structural cast, mirroring the docs
// link below — the nodes this renderer builds are not statically typed
// as full HTMLElements.
function setAttr(el: HTMLElement, name: string, value: string): void {
  (el as unknown as { setAttribute(n: string, v: string): void }).setAttribute(
    name,
    value,
  );
}

// makeActivatable turns a non-form element (an <a>, used for styling)
// into a keyboard-accessible control: focusable via tabindex, announced
// as a button via role, and activated by click, Enter, or Space — so the
// tooltip's Fix and navigate actions work without a mouse.
function makeActivatable(el: HTMLElement, onActivate: () => void): void {
  setAttr(el, "role", "button");
  setAttr(el, "tabindex", "0");
  el.addEventListener("click", (e: Event) => {
    e.preventDefault();
    onActivate();
  });
  el.addEventListener("keydown", (e: Event) => {
    const key = (e as KeyboardEvent).key;
    if (key === "Enter" || key === " ") {
      e.preventDefault();
      onActivate();
    }
  });
}

// renderTooltip composes the issue-first tooltip DOM node. Order: the
// message (lead), each related location (the schema constraint — a
// navigable link when it has a file/line, else plain text), the rule
// code + docs link, then a Fix link wired to onFix.
export function renderTooltip(
  diagnostic: Diagnostic,
  onFix: () => void,
  onNavigate?: NavigateFn,
): HTMLElement {
  const container = document.createElement("div");
  container.className = "mdsmith-tooltip";

  // 1. Message leads.
  const msg = document.createElement("div");
  msg.className = "mdsmith-tooltip-message";
  msg.textContent = diagnostic.message;
  container.appendChild(msg);

  // 2. Schema constraints from related_locations.
  for (const loc of diagnostic.related_locations ?? []) {
    container.appendChild(renderRelated(loc, onNavigate));
  }

  // 3. Rule code + docs link.
  const meta = document.createElement("div");
  meta.className = "mdsmith-tooltip-meta";
  const code = document.createElement("span");
  code.className = "mdsmith-tooltip-code";
  code.textContent = diagnostic.rule;
  meta.appendChild(code);
  const docs = document.createElement("a");
  docs.className = "mdsmith-tooltip-docs";
  docs.textContent = "docs";
  (docs as unknown as { setAttribute(n: string, v: string): void }).setAttribute(
    "href",
    ruleDocUrl(diagnostic),
  );
  meta.appendChild(docs);
  container.appendChild(meta);

  // 4. Fix link — a keyboard-accessible control (see makeActivatable).
  const fixRow = document.createElement("div");
  const fix = document.createElement("a");
  fix.className = "mdsmith-tooltip-fix";
  fix.textContent = "Fix";
  makeActivatable(fix, onFix);
  fixRow.appendChild(fix);
  container.appendChild(fixRow);

  return container;
}

// renderRelated builds one related-location row. With a file or line it
// is a clickable link that calls onNavigate; otherwise it is plain text
// (a bare constraint note with nowhere to jump to).
function renderRelated(
  loc: RelatedLocation,
  onNavigate?: NavigateFn,
): HTMLElement {
  const row = document.createElement("div");
  row.className = "mdsmith-tooltip-related";
  const navigable = (loc.file || loc.line) && onNavigate;
  if (navigable) {
    const link = document.createElement("a");
    link.className = "mdsmith-tooltip-related-link";
    link.textContent = loc.message;
    makeActivatable(link, () => onNavigate(loc));
    row.appendChild(link);
  } else {
    row.textContent = loc.message;
  }
  return row;
}

// setDiagnostics is the CM6 effect the change fan-out dispatches when a
// new check result arrives for the active buffer.
export const setDiagnostics = StateEffect.define<Diagnostic[]>();

// diagnosticsField holds the current diagnostics for the editor, stored
// as the raw engine payload; the view layer renders them.
export const diagnosticsField: StateField<Diagnostic[]> = StateField.define<
  Diagnostic[]
>({
  create() {
    return [];
  },
  update(value, tr) {
    let next = value;
    const trWithEffects = tr as unknown as {
      effects?: Array<{ is(t: unknown): boolean; value: Diagnostic[] }>;
    };
    for (const e of trWithEffects.effects ?? []) {
      if (e.is(setDiagnostics)) next = e.value;
    }
    return next;
  },
});

// diagnosticAt returns the diagnostic whose underline covers pos, or
// undefined. Used by the hover tooltip to pick the issue under the
// cursor.
export function diagnosticAt(
  doc: DocLike,
  diagnostics: Diagnostic[],
  pos: number,
): Diagnostic | undefined {
  for (const d of diagnostics) {
    const from = lineColToOffset(doc, d.line, d.column);
    let to = from;
    if (d.line >= 1 && d.line <= doc.lines) {
      try {
        to = doc.line(d.line).to;
      } catch {
        to = from;
      }
    }
    if (pos >= from && pos <= Math.max(to, from + 1)) return d;
  }
  return undefined;
}

// makeHoverTooltip wires CM6's hoverTooltip provider to the diagnostic
// at the cursor. onFix runs the quick-fix; onNavigate (optional) jumps
// to a related location.
export function makeHoverTooltip(
  onFix: (d: Diagnostic) => void,
  onNavigate?: NavigateFn,
): unknown {
  return hoverTooltip((view, pos) => {
    const state = view.state as unknown as {
      field(f: typeof diagnosticsField): Diagnostic[];
      doc: DocLike;
    };
    const list = state.field(diagnosticsField);
    if (!list || list.length === 0) return null;
    const hit = diagnosticAt(state.doc, list, pos);
    if (!hit) return null;
    return {
      pos,
      end: pos,
      above: true,
      create: () => ({
        dom: renderTooltip(
          hit,
          () => onFix(hit),
          onNavigate,
        ),
      }),
    };
  });
}

// decorationSetFor turns a buffer's diagnostics into a CM6
// DecorationSet, mapping each spec's offsets to a Decoration.mark range.
// Exported so the editor extension and any future caller share one
// builder. doc is CM6's Text (which satisfies DocLike).
export function decorationSetFor(
  doc: DocLike,
  diagnostics: Diagnostic[],
): unknown {
  const specs = buildDecorations(doc, diagnostics);
  if (specs.length === 0) return Decoration.none;
  return Decoration.set(
    specs.map((s) => Decoration.mark(s.spec).range(s.from, s.to)),
  );
}

// editorExtensions returns the CM6 extension array the plugin registers
// with Obsidian: the diagnostics state field, a decoration provider
// that recomputes spans whenever the field changes, and the hover
// tooltip. onFix and onNavigate are wired by actions.ts.
export function editorExtensions(
  onFix: (d: Diagnostic) => void,
  onNavigate?: NavigateFn,
): unknown[] {
  // EditorView.decorations.compute recomputes the set from the current
  // state whenever the diagnostics field changes. state.doc is CM6's
  // Text, which exposes the line()/lines surface DocLike needs.
  const decorations = (
    EditorView.decorations as unknown as {
      compute(
        deps: unknown[],
        get: (state: { doc: DocLike; field(f: unknown): Diagnostic[] }) => unknown,
      ): unknown;
    }
  ).compute([diagnosticsField], (state) =>
    decorationSetFor(state.doc, state.field(diagnosticsField)),
  );
  return [diagnosticsField, decorations, makeHoverTooltip(onFix, onNavigate)];
}
