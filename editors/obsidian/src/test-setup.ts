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
