// Entrypoint for the mdsmith Obsidian plugin.
//
// Obsidian loads `dist/main.js` and instantiates the default export
// as a Plugin subclass. This module is the wiring root: subsequent
// plan-214 tasks add the LSP client (lsp-client.ts), the binary
// resolver (binary.ts), the CodeMirror 6 diagnostics layer
// (diagnostics.ts), the palette commands (actions.ts), and the
// settings tab (settings.ts), and stitch them together here.
//
// During scaffolding the class is intentionally empty so the bundle
// builds and loads without behavior; subsequent commits replace the
// stub hooks with real lifecycle code.

import { Plugin } from "obsidian";

export default class MdsmithPlugin extends Plugin {
  override async onload(): Promise<void> {
    // Wiring lands in a follow-up commit under plan 214 task 7.
  }

  override async onunload(): Promise<void> {
    // Wiring lands in a follow-up commit under plan 214 task 7.
  }
}
