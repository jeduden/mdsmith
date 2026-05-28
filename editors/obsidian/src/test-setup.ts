// Test-only preload that registers stub `obsidian` and CodeMirror 6
// modules so unit tests can import the plugin's source without needing
// the real Obsidian host or the CM6 runtime packages installed.
//
// Wired in via bunfig.toml's [test] preload entry — bun loads it once
// before any test file, so the mocks are in place before ESM hoists
// the import graph.
//
// The stubs are intentionally minimal: each test only touches the
// surface its subject under test calls. New surface should land here
// alongside the test that needs it, not preemptively.

import { mock } from "bun:test";

// Minimal DOM stub for renderTooltip and any future component that
// builds DOM nodes. The plugin's UI runs in the Electron renderer
// where document is the real browser DOM, but bun's default test
// runtime has no DOM. Implement just the surface our code touches:
// createElement (returns a node with className, textContent, append-
// Child, querySelector, dispatchEvent), and addEventListener +
// click + querySelector resolution. This is dramatically smaller
// than pulling in jsdom or happy-dom for a handful of unit tests.

interface FakeEvent {
  defaultPrevented: boolean;
  preventDefault(): void;
}

interface FakeNode {
  nodeName: string;
  className: string;
  textContent: string;
  children: FakeNode[];
  parent: FakeNode | null;
  attrs: Record<string, string>;
  listeners: Record<string, Array<(e: FakeEvent) => void>>;
}

function makeNode(name: string): FakeNode {
  return {
    nodeName: name.toUpperCase(),
    className: "",
    textContent: "",
    children: [],
    parent: null,
    attrs: {},
    listeners: {},
  };
}

function walk(node: FakeNode): FakeNode[] {
  const out: FakeNode[] = [];
  const stack = [...node.children];
  while (stack.length > 0) {
    const n = stack.shift() as FakeNode;
    out.push(n);
    stack.unshift(...n.children);
  }
  return out;
}

function decorate(node: FakeNode): FakeNode & {
  appendChild(child: FakeNode): FakeNode;
  addEventListener(name: string, cb: (e: FakeEvent) => void): void;
  click(): void;
  querySelector(sel: string): (FakeNode & { click(): void }) | null;
  dispatchEvent(name: string): void;
  readonly textContent: string;
} {
  const d = node as unknown as FakeNode & {
    appendChild(child: FakeNode): FakeNode;
    addEventListener(name: string, cb: (e: FakeEvent) => void): void;
    click(): void;
    querySelector(sel: string): (FakeNode & { click(): void }) | null;
    dispatchEvent(name: string): void;
  };
  // Replace the prototype textContent (a plain string set/get) with
  // a getter that returns the node's own text plus every descendant's
  // text concatenated, matching DOM's read behavior. Assignment still
  // hits the underlying field so `node.textContent = "x"` works.
  const ownText = { value: "" };
  // Stash the previously-set value, then redefine the property.
  ownText.value = node.textContent ?? "";
  node.textContent = ""; // clear the plain field slot
  Object.defineProperty(node, "textContent", {
    get() {
      const parts: string[] = [ownText.value];
      for (const c of walk(node)) {
        const v = (c as unknown as { _ownText?: { value: string } })._ownText;
        if (v) parts.push(v.value);
      }
      return parts.join("");
    },
    set(v: string) {
      ownText.value = String(v ?? "");
    },
    configurable: true,
  });
  (node as unknown as { _ownText: { value: string } })._ownText = ownText;
  d.appendChild = (child: FakeNode) => {
    child.parent = node;
    node.children.push(child);
    return child;
  };
  d.addEventListener = (name: string, cb: (e: FakeEvent) => void) => {
    (node.listeners[name] ??= []).push(cb);
  };
  d.click = () => {
    const list = node.listeners["click"] ?? [];
    const ev: FakeEvent = {
      defaultPrevented: false,
      preventDefault() {
        this.defaultPrevented = true;
      },
    };
    for (const l of list) l(ev);
  };
  d.dispatchEvent = (name: string) => {
    const list = node.listeners[name] ?? [];
    const ev: FakeEvent = {
      defaultPrevented: false,
      preventDefault() {
        this.defaultPrevented = true;
      },
    };
    for (const l of list) l(ev);
  };
  d.querySelector = (sel: string) => {
    // Support only ".class" selectors — the only form our tests use.
    if (!sel.startsWith(".")) return null;
    const cls = sel.slice(1);
    for (const child of walk(node)) {
      const classes = child.className.split(/\s+/);
      if (classes.includes(cls)) {
        return decorate(child);
      }
    }
    return null;
  };
  return d;
}

const docStub = {
  createElement(name: string) {
    return decorate(makeNode(name));
  },
};

(globalThis as unknown as { document: typeof docStub }).document = docStub;
(globalThis as unknown as { HTMLElement: unknown }).HTMLElement = class {};

// Plugin: Obsidian's base class. Test subjects extend it and call
// `addCommand`, `registerEvent`, `loadData`, `saveData`, etc. The
// stub mirrors that shape so a `new MdsmithPlugin()` works in unit
// tests and individual methods can be spied on per-test.
class PluginStub {
  app: unknown = {};
  manifest: unknown = {};
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  addCommand(_cmd: unknown): unknown {
    return _cmd;
  }
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  registerEvent(_evt: unknown): void {}
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  registerEditorExtension(_ext: unknown): void {}
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  registerView(_args: unknown, _factory: unknown): void {}
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  addSettingTab(_tab: unknown): void {}
  async loadData(): Promise<unknown> {
    return undefined;
  }
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  async saveData(_data: unknown): Promise<void> {}
  async onload(): Promise<void> {}
  async onunload(): Promise<void> {}
}

class NoticeStub {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  constructor(_message: string, _timeout?: number) {}
}

class PluginSettingTabStub {
  containerEl: unknown = {};
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  constructor(_app: unknown, _plugin: unknown) {}
  display(): void {}
  hide(): void {}
}

class SettingStub {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  constructor(_containerEl: unknown) {}
  setName(_: string): this {
    return this;
  }
  setDesc(_: string): this {
    return this;
  }
  addText(_: unknown): this {
    return this;
  }
  addToggle(_: unknown): this {
    return this;
  }
  addDropdown(_: unknown): this {
    return this;
  }
}

mock.module("obsidian", () => ({
  Plugin: PluginStub,
  Notice: NoticeStub,
  PluginSettingTab: PluginSettingTabStub,
  Setting: SettingStub,
  ItemView: PluginStub,
  Editor: class {},
  MarkdownView: class {},
  TFile: class {},
}));

// CodeMirror 6 stubs. The diagnostics layer uses StateField,
// StateEffect, RangeSetBuilder, Decoration, EditorView, and
// hoverTooltip. Provide structural placeholders so the source
// imports resolve in tests; tests assert behaviors at the layer
// above these primitives.
mock.module("@codemirror/state", () => ({
  StateEffect: {
    define: () => ({ of: (v: unknown) => ({ value: v }) }),
  },
  StateField: {
    define: <T>(spec: {
      create: () => T;
      update: (value: T, tr: unknown) => T;
    }) => ({
      create: spec.create,
      update: spec.update,
    }),
  },
  RangeSetBuilder: class {
    private items: Array<{ from: number; to: number; value: unknown }> = [];
    add(from: number, to: number, value: unknown): void {
      this.items.push({ from, to, value });
    }
    finish(): { items: Array<{ from: number; to: number; value: unknown }> } {
      return { items: this.items };
    }
  },
}));

mock.module("@codemirror/view", () => ({
  Decoration: {
    mark: (spec: unknown) => ({ spec }),
    none: { items: [] },
  },
  EditorView: {
    decorations: { from: (_: unknown) => ({}) },
  },
  hoverTooltip: (cb: unknown) => ({ cb }),
}));
